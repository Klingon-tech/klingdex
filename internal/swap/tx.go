// Package swap - Transaction building for atomic swaps.
// This file contains the logic for constructing swap transactions.
// It uses backend.UTXO and backend.Transaction types but does NOT wrap backend methods.
package swap

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/config"
)

// Transaction errors
var (
	ErrNoUTXOs         = errors.New("no UTXOs available")
	ErrInvalidTxID     = errors.New("invalid transaction ID")
	ErrOutputNotFound  = errors.New("output not found")
)

// TxOutput represents an output to create in a transaction.
type TxOutput struct {
	Address string
	Amount  uint64 // in satoshis
}

// FundingTxParams contains parameters for creating a funding transaction.
type FundingTxParams struct {
	// Chain parameters
	Symbol  string
	Network chain.Network

	// Inputs (UTXOs to spend) - caller fetches these via backend.Backend.GetAddressUTXOs()
	UTXOs []backend.UTXO

	// Change address for leftover funds
	ChangeAddress string

	// P2TR output (the swap address)
	SwapAddress string
	SwapAmount  uint64

	// DAO fee output
	DAOAddress string
	DAOFee     uint64

	// Fee rate in sat/vB
	FeeRate uint64
}

// BuildFundingTx creates a funding transaction for a MuSig2 swap.
// Returns the raw transaction bytes ready for signing.
func BuildFundingTx(params *FundingTxParams) (*wire.MsgTx, error) {
	if len(params.UTXOs) == 0 {
		return nil, ErrNoUTXOs
	}

	// Get chain params for address decoding
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, params.Symbol)
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Calculate total input amount
	var totalInput uint64
	for _, utxo := range params.UTXOs {
		totalInput += utxo.Amount
	}

	// Add inputs
	for _, utxo := range params.UTXOs {
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidTxID, utxo.TxID)
		}
		outpoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		txIn.Sequence = wire.MaxTxInSequenceNum - 2 // Enable RBF
		tx.AddTxIn(txIn)
	}

	// Add swap output (P2TR)
	swapScript, err := addressToScript(params.SwapAddress, chainParams)
	if err != nil {
		return nil, fmt.Errorf("invalid swap address: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(params.SwapAmount), swapScript))

	// Add DAO fee output if non-zero
	if params.DAOFee > 0 && params.DAOAddress != "" {
		daoScript, err := addressToScript(params.DAOAddress, chainParams)
		if err != nil {
			return nil, fmt.Errorf("invalid DAO address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(params.DAOFee), daoScript))
	}

	// Estimate transaction size for fee calculation
	// P2TR input: ~58 vbytes, P2TR output: 43 vbytes
	estimatedVSize := int64(10) // Base tx overhead
	estimatedVSize += int64(len(params.UTXOs) * 58)
	estimatedVSize += 43 // Swap output
	if params.DAOFee > 0 {
		estimatedVSize += 43 // DAO output
	}
	estimatedVSize += 43 // Change output (assume we need change)

	// Calculate fee
	fee := uint64(estimatedVSize) * params.FeeRate

	// Calculate change
	totalOutput := params.SwapAmount + params.DAOFee + fee
	if totalInput < totalOutput {
		return nil, fmt.Errorf("%w: need %d, have %d", ErrInsufficientFunds, totalOutput, totalInput)
	}

	change := totalInput - totalOutput

	// Add change output if above dust threshold
	dustThreshold := uint64(546) // Standard dust threshold
	if change > dustThreshold {
		changeScript, err := addressToScript(params.ChangeAddress, chainParams)
		if err != nil {
			return nil, fmt.Errorf("invalid change address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))
	}

	return tx, nil
}

// SpendingTxParams contains parameters for creating a spending transaction.
type SpendingTxParams struct {
	// Chain parameters
	Symbol  string
	Network chain.Network

	// Input (the P2TR output to spend)
	FundingTxID    string
	FundingVout    uint32
	FundingAmount  uint64
	TaprootAddress string // The P2TR address we're spending FROM (needed for sighash)

	// Output address
	DestAddress string

	// DAO fee output
	DAOAddress string
	DAOFee     uint64

	// Fee rate in sat/vB
	FeeRate uint64
}

// BuildSpendingTx creates a transaction to spend from a MuSig2 P2TR output.
// Returns the unsigned transaction and the sighash to sign.
func BuildSpendingTx(params *SpendingTxParams) (*wire.MsgTx, *chainhash.Hash, error) {
	// Get chain params
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, params.Symbol)
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add input
	txHash, err := chainhash.NewHashFromStr(params.FundingTxID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidTxID, params.FundingTxID)
	}
	outpoint := wire.NewOutPoint(txHash, params.FundingVout)
	txIn := wire.NewTxIn(outpoint, nil, nil)
	txIn.Sequence = wire.MaxTxInSequenceNum
	tx.AddTxIn(txIn)

	// Estimate fee (P2TR input ~58 vbytes, P2TR output 43 vbytes each)
	outputCount := 1
	if params.DAOFee > 0 && params.DAOAddress != "" {
		outputCount = 2
	}
	estimatedVSize := int64(10 + 58 + 43*outputCount)
	fee := uint64(estimatedVSize) * params.FeeRate

	// Calculate total required
	totalRequired := fee + params.DAOFee
	if params.FundingAmount <= totalRequired {
		return nil, nil, fmt.Errorf("%w: funding %d <= fee+dao %d", ErrInsufficientFunds, params.FundingAmount, totalRequired)
	}
	outputAmount := params.FundingAmount - fee - params.DAOFee

	// Add DAO fee output first (if present)
	if params.DAOFee > 0 && params.DAOAddress != "" {
		daoScript, err := addressToScript(params.DAOAddress, chainParams)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid DAO address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(params.DAOFee), daoScript))
	}

	// Add destination output
	destScript, err := addressToScript(params.DestAddress, chainParams)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid destination address: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(outputAmount), destScript))

	// Compute sighash for Taproot key-path spend
	// We need the previous output's scriptPubKey (the P2TR output we're spending FROM)
	if params.TaprootAddress == "" {
		return nil, nil, fmt.Errorf("TaprootAddress is required for sighash computation")
	}
	prevOutScript, err := addressToScript(params.TaprootAddress, chainParams)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid taproot address: %w", err)
	}
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		prevOutScript,
		int64(params.FundingAmount),
	)

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

	sighash, err := txscript.CalcTaprootSignatureHash(
		sigHashes,
		txscript.SigHashDefault,
		tx,
		0, // Input index
		prevOutFetcher,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute sighash: %w", err)
	}

	hashBytes, err := chainhash.NewHash(sighash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create hash: %w", err)
	}

	return tx, hashBytes, nil
}

// AddWitness adds a Schnorr signature to a transaction input.
func AddWitness(tx *wire.MsgTx, inputIndex int, sig *schnorr.Signature) {
	// Taproot key-path witness: just the 64-byte signature
	tx.TxIn[inputIndex].Witness = wire.TxWitness{sig.Serialize()}
}

// SerializeTx serializes a transaction to hex.
func SerializeTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

// DeserializeTx deserializes a transaction from hex.
func DeserializeTx(hexStr string) (*wire.MsgTx, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("failed to deserialize: %w", err)
	}

	return tx, nil
}

// MinDAOFee is the minimum DAO fee to avoid dust outputs (546 sats).
const MinDAOFee = uint64(546)

// CalculateDAOFee calculates the DAO fee for a swap amount.
// Uses config.DefaultFeeConfig() for fee rates.
// Returns at least MinDAOFee to avoid dust outputs.
func CalculateDAOFee(amount uint64, isMaker bool) uint64 {
	feeCfg := config.DefaultFeeConfig()
	tradeFee := feeCfg.CalculateFee(amount, isMaker)
	daoFee := feeCfg.CalculateDAOShare(tradeFee)
	if daoFee < MinDAOFee {
		return MinDAOFee
	}
	return daoFee
}

// SelectUTXOs selects UTXOs to cover a target amount plus estimated fees.
// Returns selected UTXOs and total selected amount.
// This is a simple greedy algorithm - select largest UTXOs first.
func SelectUTXOs(utxos []backend.UTXO, targetAmount, feeRate uint64) ([]backend.UTXO, uint64, error) {
	if len(utxos) == 0 {
		return nil, 0, ErrNoUTXOs
	}

	// Sort UTXOs by amount (descending)
	sorted := make([]backend.UTXO, len(utxos))
	copy(sorted, utxos)
	sortUTXOs(sorted)

	var selected []backend.UTXO
	var totalSelected uint64

	// Estimate base fee (tx overhead + outputs)
	baseFee := uint64(10+43+43) * feeRate // overhead + swap output + change output

	for _, utxo := range sorted {
		selected = append(selected, utxo)
		totalSelected += utxo.Amount

		// Estimate fee with current inputs
		inputFee := uint64(len(selected)*58) * feeRate
		totalFee := baseFee + inputFee

		if totalSelected >= targetAmount+totalFee {
			return selected, totalSelected, nil
		}
	}

	// Check if we have enough
	inputFee := uint64(len(selected)*58) * feeRate
	totalFee := baseFee + inputFee
	if totalSelected < targetAmount+totalFee {
		return nil, 0, fmt.Errorf("%w: need %d, have %d", ErrInsufficientFunds, targetAmount+totalFee, totalSelected)
	}

	return selected, totalSelected, nil
}

// sortUTXOs sorts UTXOs by amount in descending order (largest first).
func sortUTXOs(utxos []backend.UTXO) {
	// Simple insertion sort (good enough for typical UTXO counts)
	for i := 1; i < len(utxos); i++ {
		for j := i; j > 0 && utxos[j].Amount > utxos[j-1].Amount; j-- {
			utxos[j], utxos[j-1] = utxos[j-1], utxos[j]
		}
	}
}

// addressToScript converts an address string to a scriptPubKey.
// Supports bech32 (P2WPKH) and bech32m (P2TR) addresses for any chain.
func addressToScript(address string, params *chain.Params) ([]byte, error) {
	// Get the chaincfg.Params for this chain
	netParams := getChaincfgParams(params)
	if netParams == nil {
		return nil, fmt.Errorf("unsupported chain for address decoding: %s", params.Symbol)
	}

	// Try standard btcutil first
	addr, err := btcutil.DecodeAddress(address, netParams)
	if err == nil {
		script, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create script: %w", err)
		}
		return script, nil
	}

	// Handle non-BTC bech32/bech32m addresses (LTC, etc.)
	// btcutil.DecodeAddress doesn't fully support non-BTC bech32m addresses
	if params != nil && params.Bech32HRP != "" {
		// Try to decode as bech32/bech32m manually
		hrp, data, bech32Err := bech32.Decode(address)
		if bech32Err == nil && len(data) > 0 {
			// Check if this is bech32m by trying both decodings
			isBech32m := false
			if strings.HasSuffix(address[len(address)-6:], "stje9pm") ||
				strings.Contains(address, "1p") { // P2TR addresses have "1p" after HRP
				// Try bech32m decoding
				hrpM, dataM, errM := bech32.DecodeNoLimit(address)
				if errM == nil && hrpM == params.Bech32HRP {
					hrp = hrpM
					data = dataM
					isBech32m = true
				}
			}

			if hrp == params.Bech32HRP {
				witVer := data[0]
				witnessProgram, convErr := bech32.ConvertBits(data[1:], 5, 8, false)
				if convErr != nil {
					return nil, fmt.Errorf("invalid bech32 witness program: %w", convErr)
				}

				// P2WPKH - witness version 0, 20-byte hash
				if witVer == 0 && len(witnessProgram) == 20 && !isBech32m {
					return append([]byte{txscript.OP_0, txscript.OP_DATA_20}, witnessProgram...), nil
				}

				// P2TR - witness version 1, 32-byte pubkey
				if witVer == 1 && len(witnessProgram) == 32 {
					return append([]byte{txscript.OP_1, txscript.OP_DATA_32}, witnessProgram...), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("failed to decode address: %w", err)
}

// getChaincfgParams returns the chaincfg.Params for a chain.
// Returns nil if the chain is not supported for transaction building.
func getChaincfgParams(params *chain.Params) *chaincfg.Params {
	// Map our chain params to btcd's chaincfg.Params
	// This is necessary because btcutil.DecodeAddress requires *chaincfg.Params
	switch params.Symbol {
	case "BTC":
		if params.Bech32HRP == "bc" {
			return &chaincfg.MainNetParams
		}
		return &chaincfg.TestNet3Params
	case "LTC":
		// For LTC we need to create custom params since btcd doesn't have them
		return createLTCParams(params)
	default:
		return nil
	}
}

// createLTCParams creates chaincfg.Params for Litecoin.
func createLTCParams(params *chain.Params) *chaincfg.Params {
	// Clone mainnet params and modify for LTC
	ltcParams := chaincfg.MainNetParams
	ltcParams.Name = "litecoin"
	ltcParams.Bech32HRPSegwit = params.Bech32HRP
	ltcParams.PubKeyHashAddrID = params.PubKeyHashAddrID
	ltcParams.ScriptHashAddrID = params.ScriptHashAddrID
	ltcParams.HDPrivateKeyID = params.HDPrivateKeyID
	ltcParams.HDPublicKeyID = params.HDPublicKeyID
	return &ltcParams
}

// RefundTxParams contains parameters for creating a refund transaction.
// Used when spending via the Taproot script path after CSV timelock expires.
type RefundTxParams struct {
	// Chain parameters
	Symbol  string
	Network chain.Network

	// Input (the P2TR output to spend)
	FundingTxID     string
	FundingVout     uint32
	FundingAmount   uint64
	FundingScript   []byte // The P2TR scriptPubKey (OP_1 <32-byte-pubkey>)

	// Taproot script path data
	RefundScript []byte // The refund leaf script
	ControlBlock []byte // Merkle proof for script path

	// CSV timelock in blocks (must match what's in RefundScript)
	TimeoutBlocks uint32

	// Output address for refunded funds
	DestAddress string

	// Fee rate in sat/vB
	FeeRate uint64

	// Local private key for signing (NOT the MuSig2 aggregated key)
	LocalPrivKey *btcec.PrivateKey
}

// BuildRefundTx creates a transaction to spend from a P2TR output via the script path.
// This is used when the counterparty disappears and the CSV timelock has expired.
// The transaction spends using the refund script with a single signature.
//
// Witness structure: [signature, refund_script, control_block]
func BuildRefundTx(params *RefundTxParams) (*wire.MsgTx, error) {
	if params.LocalPrivKey == nil {
		return nil, fmt.Errorf("local private key required for refund")
	}
	if len(params.RefundScript) == 0 {
		return nil, fmt.Errorf("refund script required")
	}
	if len(params.ControlBlock) == 0 {
		return nil, fmt.Errorf("control block required")
	}
	if params.TimeoutBlocks == 0 {
		return nil, fmt.Errorf("timeout blocks must be > 0")
	}

	// Get chain params
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, params.Symbol)
	}

	// Create transaction with version 2 (required for BIP 68 / CSV relative timelocks)
	tx := wire.NewMsgTx(2)

	// Add input with CSV sequence number
	txHash, err := chainhash.NewHashFromStr(params.FundingTxID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTxID, params.FundingTxID)
	}
	outpoint := wire.NewOutPoint(txHash, params.FundingVout)
	txIn := wire.NewTxIn(outpoint, nil, nil)
	// Set sequence for CSV - the timelock value enables relative lock-time
	// This tells miners the tx can only be included after timeoutBlocks confirmations
	txIn.Sequence = params.TimeoutBlocks
	tx.AddTxIn(txIn)

	// Estimate fee for script path spend
	// Script path is larger than key path due to witness data
	// Base: 10 vbytes, Input: ~58 vbytes (minimal), Output: 43 vbytes
	// Witness: signature (65) + script (~36) + control block (33+) = ~134 bytes
	// With witness discount (1/4): ~34 vbytes additional
	estimatedVSize := int64(10 + 58 + 43 + 34)
	fee := uint64(estimatedVSize) * params.FeeRate

	// Calculate output amount
	if params.FundingAmount <= fee {
		return nil, fmt.Errorf("%w: funding %d <= fee %d", ErrInsufficientFunds, params.FundingAmount, fee)
	}
	outputAmount := params.FundingAmount - fee

	// Add output
	destScript, err := addressToScript(params.DestAddress, chainParams)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(outputAmount), destScript))

	// Get or construct the funding scriptPubKey
	fundingScript := params.FundingScript
	if len(fundingScript) == 0 {
		return nil, fmt.Errorf("funding script (P2TR scriptPubKey) required")
	}

	// Create prevout fetcher for sighash computation
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		fundingScript,
		int64(params.FundingAmount),
	)

	// Create TapLeaf for the refund script
	refundLeaf := txscript.NewBaseTapLeaf(params.RefundScript)

	// Compute sighash for Taproot script path spend
	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

	sighash, err := txscript.CalcTapscriptSignaturehash(
		sigHashes,
		txscript.SigHashDefault,
		tx,
		0, // Input index
		prevOutFetcher,
		refundLeaf,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute tapscript sighash: %w", err)
	}

	// Sign with local private key
	sig, err := schnorr.Sign(params.LocalPrivKey, sighash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refund transaction: %w", err)
	}

	// Build witness: [signature, refund_script, control_block]
	// For SigHashDefault, signature is 64 bytes (no sighash byte appended)
	tx.TxIn[0].Witness = wire.TxWitness{
		sig.Serialize(),
		params.RefundScript,
		params.ControlBlock,
	}

	return tx, nil
}

// BuildRefundTxFromTree is a convenience function that builds a refund transaction
// using a TaprootScriptTree directly. This extracts the necessary data from the tree.
func BuildRefundTxFromTree(
	tree *TaprootScriptTree,
	symbol string,
	network chain.Network,
	fundingTxID string,
	fundingVout uint32,
	fundingAmount uint64,
	destAddress string,
	feeRate uint64,
	localPrivKey *btcec.PrivateKey,
) (*wire.MsgTx, error) {
	if tree == nil {
		return nil, fmt.Errorf("taproot script tree required")
	}

	// Get the P2TR scriptPubKey from the tree
	fundingScript, err := tree.ScriptPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get funding script: %w", err)
	}

	params := &RefundTxParams{
		Symbol:        symbol,
		Network:       network,
		FundingTxID:   fundingTxID,
		FundingVout:   fundingVout,
		FundingAmount: fundingAmount,
		FundingScript: fundingScript,
		RefundScript:  tree.RefundScript,
		ControlBlock:  tree.ControlBlock,
		TimeoutBlocks: tree.TimeoutBlocks,
		DestAddress:   destAddress,
		FeeRate:       feeRate,
		LocalPrivKey:  localPrivKey,
	}

	return BuildRefundTx(params)
}

// =============================================================================
// HTLC Transaction Building (P2WSH)
// =============================================================================

// HTLCClaimTxParams contains parameters for creating an HTLC claim transaction.
type HTLCClaimTxParams struct {
	// Chain parameters
	Symbol  string
	Network chain.Network

	// Input (the P2WSH HTLC output to spend)
	FundingTxID   string
	FundingVout   uint32
	FundingAmount uint64

	// HTLC script (the witness script)
	HTLCScript []byte

	// Secret for claiming (32 bytes)
	Secret []byte

	// Output address for claimed funds
	DestAddress string

	// DAO fee output
	DAOAddress string
	DAOFee     uint64

	// Fee rate in sat/vB
	FeeRate uint64

	// Private key for signing (the receiver's key in the HTLC)
	PrivKey *btcec.PrivateKey
}

// BuildHTLCClaimTx creates a transaction to claim from an HTLC P2WSH output.
// This is used when the receiver knows the secret and claims the funds.
//
// Witness structure: [signature, secret, 0x01, htlc_script]
func BuildHTLCClaimTx(params *HTLCClaimTxParams) (*wire.MsgTx, error) {
	if params.PrivKey == nil {
		return nil, fmt.Errorf("private key required for claim")
	}
	if len(params.HTLCScript) == 0 {
		return nil, fmt.Errorf("HTLC script required")
	}
	if len(params.Secret) != 32 {
		return nil, fmt.Errorf("secret must be 32 bytes, got %d", len(params.Secret))
	}

	// Get chain params
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, params.Symbol)
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add input
	txHash, err := chainhash.NewHashFromStr(params.FundingTxID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTxID, params.FundingTxID)
	}
	outpoint := wire.NewOutPoint(txHash, params.FundingVout)
	txIn := wire.NewTxIn(outpoint, nil, nil)
	txIn.Sequence = wire.MaxTxInSequenceNum
	tx.AddTxIn(txIn)

	// Estimate fee for P2WSH spend
	// Witness: sig (~73) + secret (32) + branch selector (1) + script (~100) = ~206 bytes
	// With witness discount (1/4): ~52 vbytes additional
	// Base: 10 vbytes, Input overhead: ~41 vbytes, Output: 43 vbytes
	outputCount := 1
	if params.DAOFee > 0 && params.DAOAddress != "" {
		outputCount = 2
	}
	estimatedVSize := int64(10 + 41 + 43*outputCount + 52)
	fee := uint64(estimatedVSize) * params.FeeRate

	// Calculate total required
	totalRequired := fee + params.DAOFee
	if params.FundingAmount <= totalRequired {
		return nil, fmt.Errorf("%w: funding %d <= fee+dao %d", ErrInsufficientFunds, params.FundingAmount, totalRequired)
	}
	outputAmount := params.FundingAmount - fee - params.DAOFee

	// Add DAO fee output first (if present)
	if params.DAOFee > 0 && params.DAOAddress != "" {
		daoScript, err := addressToScript(params.DAOAddress, chainParams)
		if err != nil {
			return nil, fmt.Errorf("invalid DAO address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(params.DAOFee), daoScript))
	}

	// Add destination output
	destScript, err := addressToScript(params.DestAddress, chainParams)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(outputAmount), destScript))

	// Build the P2WSH scriptPubKey from the HTLC script
	p2wshScript := BuildP2WSHScriptPubKey(params.HTLCScript)

	// Create prevout fetcher for sighash computation
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		p2wshScript,
		int64(params.FundingAmount),
	)

	// Compute sighash for P2WSH (BIP 143)
	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)
	sighash, err := txscript.CalcWitnessSigHash(
		params.HTLCScript,
		sigHashes,
		txscript.SigHashAll,
		tx,
		0, // Input index
		int64(params.FundingAmount),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sighash: %w", err)
	}

	// Sign with ECDSA (not Schnorr, since this is P2WSH)
	sig := btcecdsa.Sign(params.PrivKey, sighash)
	// Append SIGHASH_ALL byte
	sigBytes := append(sig.Serialize(), byte(txscript.SigHashAll))

	// Build witness: [signature, secret, 0x01 (OP_TRUE for IF branch), htlc_script]
	tx.TxIn[0].Witness = BuildHTLCClaimWitness(sigBytes, params.Secret, params.HTLCScript)

	return tx, nil
}

// HTLCRefundTxParams contains parameters for creating an HTLC refund transaction.
type HTLCRefundTxParams struct {
	// Chain parameters
	Symbol  string
	Network chain.Network

	// Input (the P2WSH HTLC output to spend)
	FundingTxID   string
	FundingVout   uint32
	FundingAmount uint64

	// HTLC script (the witness script)
	HTLCScript []byte

	// CSV timelock in blocks (must match what's in HTLCScript)
	TimeoutBlocks uint32

	// Output address for refunded funds
	DestAddress string

	// Fee rate in sat/vB
	FeeRate uint64

	// Private key for signing (the sender's key in the HTLC)
	PrivKey *btcec.PrivateKey
}

// BuildHTLCRefundTx creates a transaction to refund from an HTLC P2WSH output.
// This is used when the sender refunds after the CSV timelock has expired.
//
// Witness structure: [signature, 0x00, htlc_script]
func BuildHTLCRefundTx(params *HTLCRefundTxParams) (*wire.MsgTx, error) {
	if params.PrivKey == nil {
		return nil, fmt.Errorf("private key required for refund")
	}
	if len(params.HTLCScript) == 0 {
		return nil, fmt.Errorf("HTLC script required")
	}
	if params.TimeoutBlocks == 0 {
		return nil, fmt.Errorf("timeout blocks must be > 0")
	}

	// Get chain params
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, params.Symbol)
	}

	// Create transaction with version 2 (required for BIP 68 / CSV)
	tx := wire.NewMsgTx(2)

	// Add input with CSV sequence number
	txHash, err := chainhash.NewHashFromStr(params.FundingTxID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTxID, params.FundingTxID)
	}
	outpoint := wire.NewOutPoint(txHash, params.FundingVout)
	txIn := wire.NewTxIn(outpoint, nil, nil)
	// Set sequence for CSV - the timelock value enables relative lock-time
	txIn.Sequence = params.TimeoutBlocks
	tx.AddTxIn(txIn)

	// Estimate fee for P2WSH refund spend
	// Witness: sig (~73) + empty (1) + script (~100) = ~174 bytes
	// With witness discount (1/4): ~44 vbytes additional
	estimatedVSize := int64(10 + 41 + 43 + 44)
	fee := uint64(estimatedVSize) * params.FeeRate

	// Calculate output amount
	if params.FundingAmount <= fee {
		return nil, fmt.Errorf("%w: funding %d <= fee %d", ErrInsufficientFunds, params.FundingAmount, fee)
	}
	outputAmount := params.FundingAmount - fee

	// Add output
	destScript, err := addressToScript(params.DestAddress, chainParams)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(outputAmount), destScript))

	// Build the P2WSH scriptPubKey from the HTLC script
	p2wshScript := BuildP2WSHScriptPubKey(params.HTLCScript)

	// Create prevout fetcher for sighash computation
	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		p2wshScript,
		int64(params.FundingAmount),
	)

	// Compute sighash for P2WSH (BIP 143)
	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)
	sighash, err := txscript.CalcWitnessSigHash(
		params.HTLCScript,
		sigHashes,
		txscript.SigHashAll,
		tx,
		0, // Input index
		int64(params.FundingAmount),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sighash: %w", err)
	}

	// Sign with ECDSA
	sig := btcecdsa.Sign(params.PrivKey, sighash)
	// Append SIGHASH_ALL byte
	sigBytes := append(sig.Serialize(), byte(txscript.SigHashAll))

	// Build witness: [signature, empty (for ELSE branch), htlc_script]
	tx.TxIn[0].Witness = BuildHTLCRefundWitness(sigBytes, params.HTLCScript)

	return tx, nil
}
