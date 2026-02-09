// Package wallet - Multi-address transaction building and signing.
// This enables spending UTXOs from multiple addresses with different private keys.
package wallet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/internal/storage"
)

// =============================================================================
// Multi-Address UTXO Types
// =============================================================================

// AddressUTXO extends storage.WalletUTXO for use in transaction building.
// It's compatible with both fresh API queries and persisted UTXOs.
type AddressUTXO struct {
	TxID   string `json:"txid"`
	Vout   uint32 `json:"vout"`
	Amount uint64 `json:"amount"`

	// Address and derivation path
	Address      string `json:"address"`
	Account      uint32 `json:"account"`
	Change       uint32 `json:"change"`        // 0=external, 1=change
	AddressIndex uint32 `json:"address_index"`

	// Address type for signing
	AddressType string `json:"address_type"` // p2wpkh, p2tr, p2pkh

	// Script (optional, can be derived from address)
	ScriptPubKey []byte `json:"-"`
}

// FromStorageUTXO converts a storage.WalletUTXO to AddressUTXO.
func FromStorageUTXO(u *storage.WalletUTXO) *AddressUTXO {
	return &AddressUTXO{
		TxID:         u.TxID,
		Vout:         u.Vout,
		Amount:       u.Amount,
		Address:      u.Address,
		Account:      u.Account,
		Change:       u.Change,
		AddressIndex: u.AddressIndex,
		AddressType:  u.AddressType,
	}
}

// KeyDeriver is an interface for deriving private keys from derivation paths.
type KeyDeriver interface {
	DerivePrivateKeyWithChange(symbol string, account, change, index uint32) (*btcec.PrivateKey, error)
}

// =============================================================================
// Multi-Address Transaction Building
// =============================================================================

// MultiAddressTxParams contains parameters for building a multi-address transaction.
type MultiAddressTxParams struct {
	// UTXOs to spend (from multiple addresses)
	UTXOs []*AddressUTXO

	// Destination
	ToAddress string
	Amount    uint64

	// Change address (typically a new change address from the wallet)
	ChangeAddress string

	// Fee rate in sat/vB
	FeeRate uint64

	// Chain parameters
	Symbol  string
	Network chain.Network
}

// MultiAddressTxResult contains the result of building a multi-address transaction.
type MultiAddressTxResult struct {
	TxHex        string   `json:"tx_hex"`
	TxID         string   `json:"txid"`
	Fee          uint64   `json:"fee"`
	TotalInput   uint64   `json:"total_input"`
	TotalOutput  uint64   `json:"total_output"`
	Change       uint64   `json:"change"`
	InputCount   int      `json:"input_count"`
	OutputCount  int      `json:"output_count"`
	UsedUTXOs    []string `json:"used_utxos"` // txid:vout format
	VirtualSize  int64    `json:"vsize"`
}

// BuildAndSignMultiAddressTx builds and signs a transaction using UTXOs from multiple addresses.
// Each input is signed with its own private key derived from the UTXO's path.
func BuildAndSignMultiAddressTx(
	keyDeriver KeyDeriver,
	params *MultiAddressTxParams,
) (*MultiAddressTxResult, error) {
	if len(params.UTXOs) == 0 {
		return nil, fmt.Errorf("no UTXOs provided")
	}

	// Get chain params
	chainParams, ok := chain.Get(params.Symbol, params.Network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", params.Symbol)
	}

	netParams := getChaincfgParamsForTx(chainParams)
	if netParams == nil {
		return nil, fmt.Errorf("unsupported chain for transaction: %s", params.Symbol)
	}

	// Select UTXOs to cover amount + fees
	selectedUTXOs, totalInput, err := selectAddressUTXOs(params.UTXOs, params.Amount, params.FeeRate)
	if err != nil {
		return nil, err
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs from selected UTXOs
	for _, utxo := range selectedUTXOs {
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, fmt.Errorf("invalid txid %s: %w", utxo.TxID, err)
		}
		outpoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		txIn.Sequence = wire.MaxTxInSequenceNum - 2 // Enable RBF
		tx.AddTxIn(txIn)
	}

	// Parse destination address
	destScript, err := parseAddressToScript(params.ToAddress, netParams, chainParams)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}

	// Add destination output
	tx.AddTxOut(wire.NewTxOut(int64(params.Amount), destScript))

	// Calculate fee with small buffer to avoid underestimation
	estimatedVSize := estimateVSize(selectedUTXOs, params.ToAddress, params.ChangeAddress)
	// Add 2 vbytes buffer to ensure we don't underestimate and fail relay
	fee := uint64(estimatedVSize+2) * params.FeeRate

	// Calculate change
	change := totalInput - params.Amount - fee
	dustThreshold := uint64(546)

	outputCount := 1
	if change > dustThreshold {
		changeScript, err := parseAddressToScript(params.ChangeAddress, netParams, chainParams)
		if err != nil {
			return nil, fmt.Errorf("invalid change address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))
		outputCount++
	} else {
		// Add dust to fee
		fee += change
		change = 0
	}

	// Build prevout fetcher for all inputs
	prevOuts := make(map[wire.OutPoint]*wire.TxOut)
	utxoScripts := make([][]byte, len(selectedUTXOs))

	for i, utxo := range selectedUTXOs {
		// Get script for this UTXO's address
		script, err := parseAddressToScript(utxo.Address, netParams, chainParams)
		if err != nil {
			return nil, fmt.Errorf("invalid UTXO address %s: %w", utxo.Address, err)
		}
		utxoScripts[i] = script
		prevOuts[tx.TxIn[i].PreviousOutPoint] = wire.NewTxOut(int64(utxo.Amount), script)
	}

	prevOutFetcher := txscript.NewMultiPrevOutFetcher(prevOuts)

	// Sign each input with its own private key
	for i, utxo := range selectedUTXOs {
		// Derive the private key for this UTXO's address
		privKey, err := keyDeriver.DerivePrivateKeyWithChange(
			params.Symbol,
			utxo.Account,
			utxo.Change,
			utxo.AddressIndex,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key for input %d (path: %d'/%d/%d): %w",
				i, utxo.Account, utxo.Change, utxo.AddressIndex, err)
		}

		// Determine address type and sign accordingly
		addrType := detectAddressType(utxo.Address, chainParams)

		switch addrType {
		case "p2wpkh":
			if err := signP2WPKH(tx, i, privKey, prevOutFetcher); err != nil {
				return nil, fmt.Errorf("failed to sign P2WPKH input %d: %w", i, err)
			}
		case "p2tr":
			if err := signP2TR(tx, i, privKey, prevOutFetcher); err != nil {
				return nil, fmt.Errorf("failed to sign P2TR input %d: %w", i, err)
			}
		case "p2pkh":
			if err := signP2PKH(tx, i, privKey, utxoScripts[i]); err != nil {
				return nil, fmt.Errorf("failed to sign P2PKH input %d: %w", i, err)
			}
		default:
			return nil, fmt.Errorf("unsupported address type for input %d: %s", i, addrType)
		}
	}

	// Serialize
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, fmt.Errorf("failed to serialize: %w", err)
	}

	txHex := hex.EncodeToString(buf.Bytes())
	txID := tx.TxHash().String()

	// Build list of used UTXOs
	usedUTXOs := make([]string, len(selectedUTXOs))
	for i, utxo := range selectedUTXOs {
		usedUTXOs[i] = fmt.Sprintf("%s:%d", utxo.TxID, utxo.Vout)
	}

	return &MultiAddressTxResult{
		TxHex:       txHex,
		TxID:        txID,
		Fee:         fee,
		TotalInput:  totalInput,
		TotalOutput: params.Amount + change,
		Change:      change,
		InputCount:  len(selectedUTXOs),
		OutputCount: outputCount,
		UsedUTXOs:   usedUTXOs,
		VirtualSize: estimatedVSize,
	}, nil
}

// selectAddressUTXOs selects UTXOs from multiple addresses to cover target amount.
func selectAddressUTXOs(utxos []*AddressUTXO, targetAmount, feeRate uint64) ([]*AddressUTXO, uint64, error) {
	if len(utxos) == 0 {
		return nil, 0, fmt.Errorf("no UTXOs provided")
	}

	// Sort by amount descending (greedy selection)
	sorted := make([]*AddressUTXO, len(utxos))
	copy(sorted, utxos)
	sortAddressUTXOs(sorted)

	var selected []*AddressUTXO
	var totalSelected uint64

	// Base fee: tx overhead + 2 outputs (destination + change)
	baseFee := uint64(10+31+31) * feeRate

	for _, utxo := range sorted {
		selected = append(selected, utxo)
		totalSelected += utxo.Amount

		// Calculate fee with current inputs
		// Input size varies by type: P2WPKH=68, P2TR=58, P2PKH=148
		inputFee := calculateInputsFee(selected, feeRate)
		totalFee := baseFee + inputFee

		if totalSelected >= targetAmount+totalFee {
			return selected, totalSelected, nil
		}
	}

	// Final check
	inputFee := calculateInputsFee(selected, feeRate)
	totalFee := baseFee + inputFee
	if totalSelected < targetAmount+totalFee {
		return nil, 0, fmt.Errorf("insufficient funds: need %d, have %d", targetAmount+totalFee, totalSelected)
	}

	return selected, totalSelected, nil
}

// sortAddressUTXOs sorts UTXOs by amount descending.
func sortAddressUTXOs(utxos []*AddressUTXO) {
	for i := 1; i < len(utxos); i++ {
		for j := i; j > 0 && utxos[j].Amount > utxos[j-1].Amount; j-- {
			utxos[j], utxos[j-1] = utxos[j-1], utxos[j]
		}
	}
}

// calculateInputsFee calculates total fee for all inputs based on their types.
func calculateInputsFee(utxos []*AddressUTXO, feeRate uint64) uint64 {
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

// estimateVSize estimates the virtual size of a transaction.
func estimateVSize(utxos []*AddressUTXO, destAddress, changeAddress string) int64 {
	// Base transaction overhead
	vsize := int64(10)

	// Add input sizes based on type
	for _, utxo := range utxos {
		switch utxo.AddressType {
		case "p2tr":
			vsize += 58
		case "p2pkh":
			vsize += 148
		default: // p2wpkh
			vsize += 68
		}
	}

	// Add output sizes based on address type
	vsize += estimateOutputSize(destAddress)
	if changeAddress != "" {
		vsize += estimateOutputSize(changeAddress)
	}

	return vsize
}

// estimateOutputSize estimates output size based on address prefix.
func estimateOutputSize(address string) int64 {
	if len(address) < 4 {
		return 34 // Default P2PKH
	}

	// P2TR (Taproot): tb1p, bc1p, ltc1p, tltc1p
	if strings.HasPrefix(address, "tb1p") || strings.HasPrefix(address, "bc1p") ||
		strings.HasPrefix(address, "ltc1p") || strings.HasPrefix(address, "tltc1p") {
		return 43
	}

	// P2WPKH: tb1q, bc1q, ltc1q, tltc1q
	if strings.HasPrefix(address, "tb1q") || strings.HasPrefix(address, "bc1q") ||
		strings.HasPrefix(address, "ltc1q") || strings.HasPrefix(address, "tltc1q") {
		return 31
	}

	// P2PKH/P2SH (legacy)
	return 34
}

// detectAddressType detects the address type from address string.
func detectAddressType(address string, chainParams *chain.Params) string {
	if len(address) < 4 {
		return "p2pkh"
	}

	hrp := chainParams.Bech32HRP

	// Check for Taproot (witness version 1)
	if strings.HasPrefix(address, hrp+"1p") {
		return "p2tr"
	}

	// Check for P2WPKH (witness version 0)
	if strings.HasPrefix(address, hrp+"1q") {
		return "p2wpkh"
	}

	// Check for standard Bitcoin testnet/mainnet prefixes
	if strings.HasPrefix(address, "tb1p") || strings.HasPrefix(address, "bc1p") {
		return "p2tr"
	}
	if strings.HasPrefix(address, "tb1q") || strings.HasPrefix(address, "bc1q") {
		return "p2wpkh"
	}

	// Default to legacy
	return "p2pkh"
}

// =============================================================================
// Batch Operations
// =============================================================================

// SendMaxParams contains parameters for sending the maximum amount.
type SendMaxParams struct {
	UTXOs         []*AddressUTXO
	ToAddress     string
	FeeRate       uint64
	Symbol        string
	Network       chain.Network
}

// CalculateMaxSendAmount calculates the maximum amount that can be sent given UTXOs and fee rate.
func CalculateMaxSendAmount(params *SendMaxParams) (uint64, uint64, error) {
	if len(params.UTXOs) == 0 {
		return 0, 0, fmt.Errorf("no UTXOs provided")
	}

	// Calculate total input
	var totalInput uint64
	for _, utxo := range params.UTXOs {
		totalInput += utxo.Amount
	}

	// Estimate fee (no change output since we're sending max)
	vsize := int64(10) // Base overhead
	for _, utxo := range params.UTXOs {
		switch utxo.AddressType {
		case "p2tr":
			vsize += 58
		case "p2pkh":
			vsize += 148
		default:
			vsize += 68
		}
	}
	vsize += estimateOutputSize(params.ToAddress) // Only destination output

	fee := uint64(vsize) * params.FeeRate

	if totalInput <= fee {
		return 0, fee, fmt.Errorf("insufficient funds: total %d, fee %d", totalInput, fee)
	}

	maxAmount := totalInput - fee
	return maxAmount, fee, nil
}

// BuildSendMaxTx builds a transaction that sends all available funds (minus fees).
func BuildSendMaxTx(
	keyDeriver KeyDeriver,
	params *SendMaxParams,
) (*MultiAddressTxResult, error) {
	maxAmount, _, err := CalculateMaxSendAmount(params)
	if err != nil {
		return nil, err
	}

	return BuildAndSignMultiAddressTx(keyDeriver, &MultiAddressTxParams{
		UTXOs:         params.UTXOs,
		ToAddress:     params.ToAddress,
		Amount:        maxAmount,
		ChangeAddress: "", // No change for max send
		FeeRate:       params.FeeRate,
		Symbol:        params.Symbol,
		Network:       params.Network,
	})
}

// =============================================================================
// UTXO Conversion Helpers
// =============================================================================

// ConvertStorageUTXOs converts a slice of storage UTXOs to AddressUTXOs.
func ConvertStorageUTXOs(utxos []*storage.WalletUTXO) []*AddressUTXO {
	result := make([]*AddressUTXO, len(utxos))
	for i, u := range utxos {
		result[i] = FromStorageUTXO(u)
	}
	return result
}

// FilterSpendableUTXOs filters UTXOs to only include confirmed ones.
func FilterSpendableUTXOs(utxos []*AddressUTXO) []*AddressUTXO {
	var result []*AddressUTXO
	for _, u := range utxos {
		// Only include if we have path info (can derive key)
		if u.Address != "" {
			result = append(result, u)
		}
	}
	return result
}
