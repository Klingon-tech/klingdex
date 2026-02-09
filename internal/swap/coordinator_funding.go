// Package swap - Funding operations for the Coordinator.
package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/internal/config"
	"github.com/Klingon-tech/klingdex/internal/wallet"
)

// =============================================================================
// Funding
// =============================================================================

// CreateFundingTx creates a funding transaction for our side of the swap.
func (c *Coordinator) CreateFundingTx(ctx context.Context, tradeID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", ErrSwapNotFound
	}

	if active.Swap.LocalFundingTxID != "" {
		return "", ErrAlreadyFunded
	}

	// Determine which chain we're funding based on role
	var chainSymbol string
	var amount uint64
	var chainData *ChainMuSig2Data
	if active.Swap.Role == RoleInitiator {
		chainSymbol = active.Swap.Offer.OfferChain
		amount = active.Swap.Offer.OfferAmount
		chainData = active.MuSig2.OfferChain
	} else {
		chainSymbol = active.Swap.Offer.RequestChain
		amount = active.Swap.Offer.RequestAmount
		chainData = active.MuSig2.RequestChain
	}

	if chainData == nil || chainData.TaprootAddress == "" {
		return "", errors.New("taproot address not set - exchange pubkeys first")
	}

	// Get backend for fee estimation
	b, ok := c.backends[chainSymbol]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoBackend, chainSymbol)
	}

	// Get wallet address for change
	walletAddr, err := c.getWalletAddress(chainSymbol)
	if err != nil {
		return "", fmt.Errorf("failed to get wallet address: %w", err)
	}

	// Calculate DAO fee
	isMaker := active.Swap.Role == RoleInitiator
	daoFee := CalculateDAOFee(amount, isMaker)

	// Get DAO address from config based on network
	exchangeCfg := config.NewExchangeConfig(config.NetworkType(c.network))
	daoAddress := exchangeCfg.GetDAOAddress(chainSymbol)

	// Get dynamic fee rate from backend
	feeRate := uint64(10) // Default fallback
	if feeEstimate, err := b.GetFeeEstimates(ctx); err == nil && feeEstimate != nil {
		if feeEstimate.HalfHourFee > 0 {
			feeRate = feeEstimate.HalfHourFee
		} else if feeEstimate.HourFee > 0 {
			feeRate = feeEstimate.HourFee
		}
	}

	// Convert backend UTXOs to wallet AddressUTXOs for signing
	if c.walletService == nil {
		return "", errors.New("wallet service not available for signing")
	}

	// Use the wallet service to list UTXOs with derivation paths
	walletUTXOs, err := c.walletService.ListAllUTXOs(ctx, chainSymbol, c.store)
	if err != nil {
		return "", fmt.Errorf("failed to list wallet UTXOs: %w", err)
	}

	// Calculate total needed including DAO fee
	totalNeeded := amount + daoFee

	// Build and sign the funding transaction using wallet
	txResult, _, err := c.buildAndSignFundingTx(ctx, &fundingBuildParams{
		symbol:      chainSymbol,
		utxos:       walletUTXOs,
		escrowAddr:  chainData.TaprootAddress,
		escrowAmt:   amount,
		daoAddr:     daoAddress,
		daoFee:      daoFee,
		changeAddr:  walletAddr,
		feeRate:     feeRate,
		totalNeeded: totalNeeded,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build and sign funding tx: %w", err)
	}

	txHex := txResult.TxHex

	c.emitEvent(tradeID, "funding_tx_created", map[string]interface{}{
		"chain":  chainSymbol,
		"amount": amount,
		"tx_hex": txHex,
	})

	return txHex, nil
}

// SetFundingTx records that a funding transaction was broadcast.
func (c *Coordinator) SetFundingTx(tradeID string, txID string, vout uint32, isLocal bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if isLocal {
		active.Swap.LocalFundingTxID = txID
		active.Swap.LocalFundingVout = vout
	} else {
		active.Swap.RemoteFundingTxID = txID
		active.Swap.RemoteFundingVout = vout
	}

	// Transition to funding state if this is our first funding info
	if active.Swap.State == StateInit {
		if err := active.Swap.TransitionTo(StateFunding); err != nil {
			return err
		}
	}

	// Save swap state to database
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("SetFundingTx: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "funding_set", map[string]interface{}{
		"txid":     txID,
		"vout":     vout,
		"is_local": isLocal,
	})

	return nil
}

// =============================================================================
// Confirmation Tracking
// =============================================================================

// UpdateConfirmations updates confirmation counts for funding transactions.
func (c *Coordinator) UpdateConfirmations(ctx context.Context, tradeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	// Update local funding confirmations
	if active.Swap.LocalFundingTxID != "" {
		var chainSymbol string
		if active.Swap.Role == RoleInitiator {
			chainSymbol = active.Swap.Offer.OfferChain
		} else {
			chainSymbol = active.Swap.Offer.RequestChain
		}

		confirms, err := c.getConfirmations(ctx, chainSymbol, active.Swap.LocalFundingTxID)
		if err == nil {
			active.Swap.UpdateLocalConfirmations(confirms)
		}
	}

	// Update remote funding confirmations
	if active.Swap.RemoteFundingTxID != "" {
		var chainSymbol string
		if active.Swap.Role == RoleInitiator {
			chainSymbol = active.Swap.Offer.RequestChain
		} else {
			chainSymbol = active.Swap.Offer.OfferChain
		}

		confirms, err := c.getConfirmations(ctx, chainSymbol, active.Swap.RemoteFundingTxID)
		if err == nil {
			active.Swap.UpdateRemoteConfirmations(confirms)
		}
	}

	// Check if we should transition to funded state
	if active.Swap.State == StateFunding && active.Swap.IsFundingConfirmed() {
		if err := active.Swap.TransitionTo(StateFunded); err != nil {
			return err
		}
		c.emitEvent(tradeID, "funding_confirmed", nil)
	}

	return nil
}

// =============================================================================
// Auto-Funding (Sign + Broadcast + Set)
// =============================================================================

// FundSwapResult contains the result of funding a swap.
type FundSwapResult struct {
	TxID         string `json:"txid"`
	Chain        string `json:"chain"`
	Amount       uint64 `json:"amount"`
	Fee          uint64 `json:"fee"`
	EscrowVout   uint32 `json:"escrow_vout"`
	EscrowAddr   string `json:"escrow_address"`
	InputCount   int    `json:"input_count"`
	TotalInput   uint64 `json:"total_input"`
	Change       uint64 `json:"change"`
}

// FundSwap automatically funds the swap escrow address by:
// 1. Scanning wallet UTXOs
// 2. Building and signing a funding transaction
// 3. Broadcasting to the network
// 4. Setting the funding info on the swap
func (c *Coordinator) FundSwap(ctx context.Context, tradeID string) (*FundSwapResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	if active.Swap.LocalFundingTxID != "" {
		return nil, ErrAlreadyFunded
	}

	if c.walletService == nil {
		return nil, errors.New("wallet service not available")
	}

	// Determine which chain we're funding and get escrow address
	var chainSymbol string
	var amount uint64
	var escrowAddr string

	if active.IsMuSig2() {
		var chainData *ChainMuSig2Data
		if active.Swap.Role == RoleInitiator {
			chainSymbol = active.Swap.Offer.OfferChain
			amount = active.Swap.Offer.OfferAmount
			chainData = active.MuSig2.OfferChain
		} else {
			chainSymbol = active.Swap.Offer.RequestChain
			amount = active.Swap.Offer.RequestAmount
			chainData = active.MuSig2.RequestChain
		}
		if chainData == nil || chainData.TaprootAddress == "" {
			return nil, errors.New("taproot address not set - exchange pubkeys first")
		}
		escrowAddr = chainData.TaprootAddress
	} else if active.IsHTLC() {
		var chainData *ChainHTLCData
		if active.Swap.Role == RoleInitiator {
			chainSymbol = active.Swap.Offer.OfferChain
			amount = active.Swap.Offer.OfferAmount
			chainData = active.HTLC.OfferChain
		} else {
			chainSymbol = active.Swap.Offer.RequestChain
			amount = active.Swap.Offer.RequestAmount
			chainData = active.HTLC.RequestChain
		}
		if chainData == nil || chainData.HTLCAddress == "" {
			return nil, errors.New("HTLC address not set - exchange secret hash first")
		}
		escrowAddr = chainData.HTLCAddress
	} else {
		return nil, errors.New("unknown swap method")
	}

	// Get backend for the chain
	b, ok := c.backends[chainSymbol]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoBackend, chainSymbol)
	}

	// Calculate DAO fee
	isMaker := active.Swap.Role == RoleInitiator
	daoFee := CalculateDAOFee(amount, isMaker)

	// Get DAO address from config
	exchangeCfg := config.NewExchangeConfig(config.NetworkType(c.network))
	daoAddress := exchangeCfg.GetDAOAddress(chainSymbol)

	// Total amount needed: escrow + DAO fee
	totalNeeded := amount + daoFee

	// Get fee rate with minimum floor to ensure relay
	const minFeeRate = uint64(2) // Minimum 2 sat/vB to ensure relay
	feeRate := uint64(10) // Default
	if feeEstimate, err := b.GetFeeEstimates(ctx); err == nil && feeEstimate != nil {
		if feeEstimate.HalfHourFee > 0 {
			feeRate = feeEstimate.HalfHourFee
		} else if feeEstimate.HourFee > 0 {
			feeRate = feeEstimate.HourFee
		}
	}
	// Ensure minimum fee rate
	if feeRate < minFeeRate {
		feeRate = minFeeRate
	}

	// Scan wallet UTXOs
	utxos, err := c.walletService.ListAllUTXOs(ctx, chainSymbol, c.store)
	if err != nil {
		return nil, fmt.Errorf("failed to scan wallet UTXOs: %w", err)
	}
	if len(utxos) == 0 {
		return nil, errors.New("no spendable UTXOs found in wallet")
	}

	// Get change address
	changeAddr, err := c.getWalletAddress(chainSymbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get change address: %w", err)
	}

	// Build and sign the funding transaction
	// We need to create outputs: 1) escrow, 2) DAO fee (if > 0), 3) change
	txResult, escrowVout, err := c.buildAndSignFundingTx(ctx, &fundingBuildParams{
		symbol:      chainSymbol,
		utxos:       utxos,
		escrowAddr:  escrowAddr,
		escrowAmt:   amount,
		daoAddr:     daoAddress,
		daoFee:      daoFee,
		changeAddr:  changeAddr,
		feeRate:     feeRate,
		totalNeeded: totalNeeded,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build funding tx: %w", err)
	}

	// Broadcast the transaction
	txid, err := b.BroadcastTransaction(ctx, txResult.TxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast funding tx: %w", err)
	}

	// Set funding info on the swap
	active.Swap.LocalFundingTxID = txid
	active.Swap.LocalFundingVout = escrowVout

	// Transition to funding state
	if active.Swap.State == StateInit {
		if err := active.Swap.TransitionTo(StateFunding); err != nil {
			c.log.Warn("FundSwap: failed to transition state", "error", err)
		}
	}

	// Save swap state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("FundSwap: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	result := &FundSwapResult{
		TxID:       txid,
		Chain:      chainSymbol,
		Amount:     amount,
		Fee:        txResult.Fee,
		EscrowVout: escrowVout,
		EscrowAddr: escrowAddr,
		InputCount: txResult.InputCount,
		TotalInput: txResult.TotalInput,
		Change:     txResult.Change,
	}

	c.emitEvent(tradeID, "funding_broadcast", map[string]interface{}{
		"txid":        txid,
		"chain":       chainSymbol,
		"amount":      amount,
		"escrow_vout": escrowVout,
	})

	return result, nil
}

// fundingBuildParams holds parameters for building a funding transaction.
type fundingBuildParams struct {
	symbol      string
	utxos       []*wallet.AddressUTXO
	escrowAddr  string
	escrowAmt   uint64
	daoAddr     string
	daoFee      uint64
	changeAddr  string
	feeRate     uint64
	totalNeeded uint64
}

// buildAndSignFundingTx builds and signs a funding transaction with escrow and DAO outputs.
// Output order: escrow (vout 0), DAO fee (vout 1 if present), change (last vout)
func (c *Coordinator) buildAndSignFundingTx(ctx context.Context, params *fundingBuildParams) (*wallet.MultiAddressTxResult, uint32, error) {
	// Get chain params for script generation
	chainParams, ok := chain.Get(params.symbol, c.network)
	if !ok {
		return nil, 0, fmt.Errorf("unsupported chain: %s", params.symbol)
	}

	// Select UTXOs to cover escrow + DAO + fees
	selectedUTXOs, totalInput, err := selectUTXOsForFunding(params.utxos, params.totalNeeded, params.feeRate, params.daoFee > 0 && params.daoAddr != "")
	if err != nil {
		return nil, 0, err
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range selectedUTXOs {
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid txid %s: %w", utxo.TxID, err)
		}
		outpoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		txIn.Sequence = wire.MaxTxInSequenceNum - 2 // Enable RBF
		tx.AddTxIn(txIn)
	}

	// Parse escrow address script
	escrowScript, err := wallet.ParseAddressToScript(params.escrowAddr, chainParams)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid escrow address: %w", err)
	}

	// Add escrow output (vout 0)
	tx.AddTxOut(wire.NewTxOut(int64(params.escrowAmt), escrowScript))
	outputCount := 1
	hasDAOOutput := false

	// Add DAO fee output (vout 1) if present
	if params.daoFee > 0 && params.daoAddr != "" {
		daoScript, err := wallet.ParseAddressToScript(params.daoAddr, chainParams)
		if err != nil {
			c.log.Warn("Invalid DAO address, skipping DAO output", "address", params.daoAddr, "error", err)
		} else {
			tx.AddTxOut(wire.NewTxOut(int64(params.daoFee), daoScript))
			outputCount++
			hasDAOOutput = true
			c.log.Info("Added DAO fee output", "amount", params.daoFee, "address", params.daoAddr)
		}
	}

	// Calculate fee
	estimatedVSize := estimateFundingVSize(selectedUTXOs, outputCount+1) // +1 for potential change
	fee := uint64(estimatedVSize) * params.feeRate

	// Calculate change
	totalOutput := params.escrowAmt
	if hasDAOOutput {
		totalOutput += params.daoFee
	}
	change := totalInput - totalOutput - fee
	dustThreshold := uint64(546)

	if change > dustThreshold {
		changeScript, err := wallet.ParseAddressToScript(params.changeAddr, chainParams)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid change address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))
		outputCount++
	} else {
		// Add dust to fee
		fee += change
		change = 0
	}

	// Build prevout fetcher for signing
	prevOuts := make(map[wire.OutPoint]*wire.TxOut)
	for i, utxo := range selectedUTXOs {
		script, err := wallet.ParseAddressToScript(utxo.Address, chainParams)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid UTXO address %s: %w", utxo.Address, err)
		}
		prevOuts[tx.TxIn[i].PreviousOutPoint] = wire.NewTxOut(int64(utxo.Amount), script)
	}
	prevOutFetcher := txscript.NewMultiPrevOutFetcher(prevOuts)

	// Sign each input
	for i, utxo := range selectedUTXOs {
		// Derive private key for this UTXO
		privKey, err := c.walletService.DerivePrivateKeyWithChange(
			params.symbol,
			utxo.Account,
			utxo.Change,
			utxo.AddressIndex,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to derive key for input %d: %w", i, err)
		}

		// Sign based on address type
		if err := signFundingInput(tx, i, privKey, prevOutFetcher, utxo, chainParams); err != nil {
			return nil, 0, fmt.Errorf("failed to sign input %d: %w", i, err)
		}
	}

	// Serialize transaction
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, 0, fmt.Errorf("failed to serialize tx: %w", err)
	}

	txHex := hex.EncodeToString(buf.Bytes())
	txID := tx.TxHash().String()

	// Build list of used UTXOs
	usedUTXOs := make([]string, len(selectedUTXOs))
	for i, utxo := range selectedUTXOs {
		usedUTXOs[i] = fmt.Sprintf("%s:%d", utxo.TxID, utxo.Vout)
	}

	result := &wallet.MultiAddressTxResult{
		TxHex:       txHex,
		TxID:        txID,
		Fee:         fee,
		TotalInput:  totalInput,
		TotalOutput: totalOutput + change,
		Change:      change,
		InputCount:  len(selectedUTXOs),
		OutputCount: outputCount,
		UsedUTXOs:   usedUTXOs,
		VirtualSize: int64(estimatedVSize),
	}

	// Escrow is always at vout 0
	return result, 0, nil
}

// selectUTXOsForFunding selects UTXOs to cover target amount plus fees.
func selectUTXOsForFunding(utxos []*wallet.AddressUTXO, targetAmount, feeRate uint64, hasDAOOutput bool) ([]*wallet.AddressUTXO, uint64, error) {
	if len(utxos) == 0 {
		return nil, 0, errors.New("no UTXOs provided")
	}

	// Sort by amount descending
	sorted := make([]*wallet.AddressUTXO, len(utxos))
	copy(sorted, utxos)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Amount > sorted[j-1].Amount; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var selected []*wallet.AddressUTXO
	var totalSelected uint64

	// Base outputs: escrow + DAO (if present) + change
	numOutputs := 2 // escrow + change
	if hasDAOOutput {
		numOutputs = 3 // escrow + DAO + change
	}
	baseFee := uint64(10+numOutputs*34) * feeRate // tx overhead + outputs

	for _, utxo := range sorted {
		selected = append(selected, utxo)
		totalSelected += utxo.Amount

		// Calculate fee with current inputs
		inputFee := calculateInputsFeeForFunding(selected, feeRate)
		totalFee := baseFee + inputFee

		if totalSelected >= targetAmount+totalFee {
			return selected, totalSelected, nil
		}
	}

	// Final check
	inputFee := calculateInputsFeeForFunding(selected, feeRate)
	totalFee := baseFee + inputFee
	if totalSelected < targetAmount+totalFee {
		return nil, 0, fmt.Errorf("insufficient funds: need %d, have %d", targetAmount+totalFee, totalSelected)
	}

	return selected, totalSelected, nil
}

// calculateInputsFeeForFunding calculates total fee for inputs.
func calculateInputsFeeForFunding(utxos []*wallet.AddressUTXO, feeRate uint64) uint64 {
	var totalVBytes uint64
	for _, utxo := range utxos {
		switch utxo.AddressType {
		case "p2tr":
			totalVBytes += 58
		case "p2pkh":
			totalVBytes += 148
		default: // p2wpkh
			totalVBytes += 68
		}
	}
	return totalVBytes * feeRate
}

// estimateFundingVSize estimates vsize for funding transaction.
func estimateFundingVSize(utxos []*wallet.AddressUTXO, numOutputs int) int64 {
	vsize := int64(10) // tx overhead
	for _, utxo := range utxos {
		switch utxo.AddressType {
		case "p2tr":
			vsize += 58
		case "p2pkh":
			vsize += 148
		default:
			vsize += 68
		}
	}
	vsize += int64(numOutputs * 34) // average output size
	return vsize
}

// signFundingInput signs a single input based on address type.
func signFundingInput(tx *wire.MsgTx, inputIndex int, privKey *btcec.PrivateKey, prevOutFetcher txscript.PrevOutputFetcher, utxo *wallet.AddressUTXO, chainParams *chain.Params) error {
	outpoint := tx.TxIn[inputIndex].PreviousOutPoint
	prevOut := prevOutFetcher.FetchPrevOutput(outpoint)
	if prevOut == nil {
		return errors.New("previous output not found")
	}

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

	switch utxo.AddressType {
	case "p2wpkh":
		witness, err := txscript.WitnessSignature(
			tx, sigHashes, inputIndex,
			prevOut.Value, prevOut.PkScript,
			txscript.SigHashAll, privKey, true,
		)
		if err != nil {
			return err
		}
		tx.TxIn[inputIndex].Witness = witness

	case "p2tr":
		sig, err := txscript.RawTxInTaprootSignature(
			tx, sigHashes, inputIndex,
			prevOut.Value, prevOut.PkScript,
			nil, txscript.SigHashDefault, privKey,
		)
		if err != nil {
			return err
		}
		tx.TxIn[inputIndex].Witness = wire.TxWitness{sig}

	case "p2pkh":
		sigScript, err := txscript.SignatureScript(
			tx, inputIndex, prevOut.PkScript,
			txscript.SigHashAll, privKey, true,
		)
		if err != nil {
			return err
		}
		tx.TxIn[inputIndex].SignatureScript = sigScript

	default:
		return fmt.Errorf("unsupported address type: %s", utxo.AddressType)
	}

	return nil
}
