// Package wallet - Transaction building and signing for wallet operations.
package wallet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/Klingon-tech/klingdex/internal/backend"
	"github.com/Klingon-tech/klingdex/internal/chain"
)

// BuildAndSignTx builds and signs a transaction sending to toAddress.
// All UTXOs are assumed to belong to the senderAddress (used as change address).
// Returns the serialized transaction hex ready for broadcast.
func BuildAndSignTx(
	privKey *btcec.PrivateKey,
	utxos []backend.UTXO,
	toAddress string,
	senderAddress string,
	amount uint64,
	feeRate uint64,
	params *chain.Params,
) (string, error) {
	if len(utxos) == 0 {
		return "", fmt.Errorf("no UTXOs provided")
	}

	// Get chaincfg params
	netParams := getChaincfgParamsForTx(params)
	if netParams == nil {
		return "", fmt.Errorf("unsupported chain for transaction: %s", params.Symbol)
	}

	// Select UTXOs
	selectedUTXOs, totalInput, err := selectUTXOsForAmount(utxos, amount, feeRate)
	if err != nil {
		return "", err
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range selectedUTXOs {
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", fmt.Errorf("invalid txid %s: %w", utxo.TxID, err)
		}
		outpoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		txIn.Sequence = wire.MaxTxInSequenceNum - 2 // Enable RBF
		tx.AddTxIn(txIn)
	}

	// Parse destination address (with Taproot support for all chains)
	destScript, err := parseAddressToScript(toAddress, netParams, params)
	if err != nil {
		return "", fmt.Errorf("invalid destination address: %w", err)
	}

	// Add destination output
	tx.AddTxOut(wire.NewTxOut(int64(amount), destScript))

	// Calculate fee
	// Estimate vsize based on address types
	// Input sizes: P2WPKH=68, P2PKH=148, P2TR=58
	// Output sizes: P2WPKH=31, P2PKH=34, P2TR=43
	inputSize := 68  // Assume P2WPKH inputs (most common)
	baseSize := 10   // tx overhead

	// Estimate destination output size based on address prefix
	// P2TR (Taproot): tb1p, bc1p, ltc1p, tltc1p - 43 vbytes
	// P2WSH: tb1q..(62+ chars), bc1q.., ltc1q.., tltc1q.. - 43 vbytes
	// P2WPKH: tb1q, bc1q, ltc1q, tltc1q - 31 vbytes
	destOutputSize := 31 // P2WPKH default
	if len(toAddress) > 4 {
		// Check for Taproot P2TR addresses (witness version 1)
		if strings.HasPrefix(toAddress, "tb1p") || strings.HasPrefix(toAddress, "bc1p") ||
			strings.HasPrefix(toAddress, "ltc1p") || strings.HasPrefix(toAddress, "tltc1p") {
			destOutputSize = 43
		}
		// Check for P2WSH addresses (longer than P2WPKH due to 32-byte hash vs 20-byte)
		// P2WPKH is ~42 chars, P2WSH is ~62 chars
		if len(toAddress) > 50 && (strings.HasPrefix(toAddress, "tb1q") || strings.HasPrefix(toAddress, "bc1q") ||
			strings.HasPrefix(toAddress, "ltc1q") || strings.HasPrefix(toAddress, "tltc1q")) {
			destOutputSize = 43
		}
	}
	changeOutputSize := 31 // P2WPKH change (from same wallet)

	// Add 2 vbytes margin to handle rounding differences
	estimatedVSize := baseSize + len(selectedUTXOs)*inputSize + destOutputSize + changeOutputSize + 2
	fee := uint64(estimatedVSize) * feeRate

	// Calculate change
	change := totalInput - amount - fee
	dustThreshold := uint64(546)

	if change > dustThreshold {
		changeScript, err := parseAddressToScript(senderAddress, netParams, params)
		if err != nil {
			return "", fmt.Errorf("invalid change address: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))
	}

	// Decode sender address to determine script type for signing
	senderAddr, senderScript, err := decodeAnyAddress(senderAddress, netParams, params)
	if err != nil {
		return "", fmt.Errorf("invalid sender address: %w", err)
	}

	// Build prevout fetcher for all inputs (all from same sender address)
	prevOuts := make(map[wire.OutPoint]*wire.TxOut)
	for i, utxo := range selectedUTXOs {
		prevOuts[tx.TxIn[i].PreviousOutPoint] = wire.NewTxOut(int64(utxo.Amount), senderScript)
	}

	prevOutFetcher := txscript.NewMultiPrevOutFetcher(prevOuts)

	// Sign each input based on address type
	for i := range selectedUTXOs {
		switch senderAddr.(type) {
		case *btcutil.AddressWitnessPubKeyHash:
			// P2WPKH - Native SegWit
			if err := signP2WPKH(tx, i, privKey, prevOutFetcher); err != nil {
				return "", fmt.Errorf("failed to sign P2WPKH input %d: %w", i, err)
			}
		case *btcutil.AddressTaproot:
			// P2TR - Taproot
			if err := signP2TR(tx, i, privKey, prevOutFetcher); err != nil {
				return "", fmt.Errorf("failed to sign P2TR input %d: %w", i, err)
			}
		case *btcutil.AddressPubKeyHash:
			// P2PKH - Legacy
			if err := signP2PKH(tx, i, privKey, senderScript); err != nil {
				return "", fmt.Errorf("failed to sign P2PKH input %d: %w", i, err)
			}
		default:
			return "", fmt.Errorf("unsupported address type for input %d: %T", i, senderAddr)
		}
	}

	// Serialize
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize: %w", err)
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

// signP2WPKH signs a P2WPKH (native SegWit) input.
func signP2WPKH(tx *wire.MsgTx, inputIndex int, privKey *btcec.PrivateKey, prevOutFetcher txscript.PrevOutputFetcher) error {
	outpoint := tx.TxIn[inputIndex].PreviousOutPoint
	prevOut := prevOutFetcher.FetchPrevOutput(outpoint)
	if prevOut == nil {
		return fmt.Errorf("previous output not found")
	}

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

	witness, err := txscript.WitnessSignature(
		tx,
		sigHashes,
		inputIndex,
		prevOut.Value,
		prevOut.PkScript,
		txscript.SigHashAll,
		privKey,
		true, // compressed
	)
	if err != nil {
		return err
	}

	tx.TxIn[inputIndex].Witness = witness
	return nil
}

// signP2TR signs a P2TR (Taproot) input using key-path spend.
func signP2TR(tx *wire.MsgTx, inputIndex int, privKey *btcec.PrivateKey, prevOutFetcher txscript.PrevOutputFetcher) error {
	outpoint := tx.TxIn[inputIndex].PreviousOutPoint
	prevOut := prevOutFetcher.FetchPrevOutput(outpoint)
	if prevOut == nil {
		return fmt.Errorf("previous output not found for taproot signing")
	}

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

	sig, err := txscript.RawTxInTaprootSignature(
		tx,
		sigHashes,
		inputIndex,
		prevOut.Value,
		prevOut.PkScript,
		nil, // No tapLeaf for key-path
		txscript.SigHashDefault,
		privKey,
	)
	if err != nil {
		return err
	}

	// Taproot key-path witness is just the signature
	tx.TxIn[inputIndex].Witness = wire.TxWitness{sig}
	return nil
}

// signP2PKH signs a P2PKH (legacy) input.
func signP2PKH(tx *wire.MsgTx, inputIndex int, privKey *btcec.PrivateKey, pkScript []byte) error {
	sig, err := txscript.SignatureScript(
		tx,
		inputIndex,
		pkScript,
		txscript.SigHashAll,
		privKey,
		true, // compressed
	)
	if err != nil {
		return err
	}

	tx.TxIn[inputIndex].SignatureScript = sig
	return nil
}

// selectUTXOsForAmount selects UTXOs to cover target amount plus fees.
func selectUTXOsForAmount(utxos []backend.UTXO, targetAmount, feeRate uint64) ([]backend.UTXO, uint64, error) {
	// Sort by amount descending (simple greedy selection)
	sorted := make([]backend.UTXO, len(utxos))
	copy(sorted, utxos)

	// Simple insertion sort
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Amount > sorted[j-1].Amount; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var selected []backend.UTXO
	var totalSelected uint64

	// Base fee for tx overhead + 2 outputs
	baseFee := uint64(10+31+31) * feeRate

	for _, utxo := range sorted {
		selected = append(selected, utxo)
		totalSelected += utxo.Amount

		// Add per-input fee (assuming P2WPKH)
		inputFee := uint64(len(selected)*68) * feeRate
		totalFee := baseFee + inputFee

		if totalSelected >= targetAmount+totalFee {
			return selected, totalSelected, nil
		}
	}

	// Final check
	inputFee := uint64(len(selected)*68) * feeRate
	totalFee := baseFee + inputFee
	if totalSelected < targetAmount+totalFee {
		return nil, 0, fmt.Errorf("insufficient funds: need %d, have %d", targetAmount+totalFee, totalSelected)
	}

	return selected, totalSelected, nil
}

// getChaincfgParamsForTx returns chaincfg.Params for transaction building.
func getChaincfgParamsForTx(params *chain.Params) *chaincfg.Params {
	switch params.Symbol {
	case "BTC":
		if params.Bech32HRP == "bc" {
			return &chaincfg.MainNetParams
		}
		return &chaincfg.TestNet3Params
	case "LTC":
		return createLTCParamsForTx(params)
	default:
		return nil
	}
}

// createLTCParamsForTx creates chaincfg.Params for Litecoin transactions.
func createLTCParamsForTx(params *chain.Params) *chaincfg.Params {
	ltcParams := chaincfg.MainNetParams
	ltcParams.Name = "litecoin"
	ltcParams.Bech32HRPSegwit = params.Bech32HRP
	ltcParams.PubKeyHashAddrID = params.PubKeyHashAddrID
	ltcParams.ScriptHashAddrID = params.ScriptHashAddrID
	ltcParams.HDPrivateKeyID = params.HDPrivateKeyID
	ltcParams.HDPublicKeyID = params.HDPublicKeyID
	return &ltcParams
}

// ParseAddressToScript parses an address and returns its output script.
// Supports bech32 (P2WPKH) and bech32m (Taproot) addresses for any chain.
// This is the exported version that takes only chain.Params.
func ParseAddressToScript(address string, chainParams *chain.Params) ([]byte, error) {
	if chainParams == nil {
		return nil, fmt.Errorf("chain params required")
	}
	netParams := getChaincfgParamsForTx(chainParams)
	if netParams == nil {
		return nil, fmt.Errorf("unsupported chain: %s", chainParams.Symbol)
	}
	return parseAddressToScript(address, netParams, chainParams)
}

// parseAddressToScript parses an address and returns its output script.
// Supports bech32 (P2WPKH) and bech32m (Taproot) addresses for any chain.
func parseAddressToScript(address string, netParams *chaincfg.Params, chainParams *chain.Params) ([]byte, error) {
	// Try standard btcutil first
	decoded, err := btcutil.DecodeAddress(address, netParams)
	if err == nil {
		return txscript.PayToAddrScript(decoded)
	}

	// Handle non-BTC bech32/bech32m addresses (LTC, etc.)
	if chainParams != nil {
		hrp, data, spec, bErr := decodeBech32(address)
		if bErr == nil && len(data) > 0 {
			expectedHRP := chainParams.Bech32HRP
			if hrp == expectedHRP {
				witVer := data[0]
				witnessProgram, err := bech32ConvertBits(data[1:], 5, 8, false)
				if err != nil {
					return nil, fmt.Errorf("invalid bech32 witness program: %w", err)
				}

				// P2WPKH - witness version 0, 20-byte hash
				if witVer == 0 && len(witnessProgram) == 20 && spec == bech32 {
					return append([]byte{txscript.OP_0, txscript.OP_DATA_20}, witnessProgram...), nil
				}

				// P2WSH - witness version 0, 32-byte script hash
				if witVer == 0 && len(witnessProgram) == 32 && spec == bech32 {
					return append([]byte{txscript.OP_0, txscript.OP_DATA_32}, witnessProgram...), nil
				}

				// P2TR - witness version 1, 32-byte pubkey
				if witVer == 1 && len(witnessProgram) == 32 && spec == bech32m {
					return append([]byte{txscript.OP_1, txscript.OP_DATA_32}, witnessProgram...), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("decoded address is of unknown format")
}

// decodeAnyAddress decodes an address and returns both the address object and its script.
// This handles addresses for any supported chain including LTC bech32 (P2WPKH).
func decodeAnyAddress(address string, netParams *chaincfg.Params, chainParams *chain.Params) (btcutil.Address, []byte, error) {
	// Try standard btcutil first
	decoded, err := btcutil.DecodeAddress(address, netParams)
	if err == nil {
		script, err := txscript.PayToAddrScript(decoded)
		if err != nil {
			return nil, nil, err
		}
		return decoded, script, nil
	}

	// Handle non-BTC bech32 addresses (like LTC tltc1q... P2WPKH or tltc1p... P2TR)
	if chainParams != nil {
		hrp, data, spec, bErr := decodeBech32(address)
		if bErr == nil && len(data) > 0 {
			expectedHRP := chainParams.Bech32HRP
			if hrp == expectedHRP {
				witVer := data[0]
				witnessProgram, err := bech32ConvertBits(data[1:], 5, 8, false)
				if err != nil {
					return nil, nil, fmt.Errorf("invalid bech32 witness program: %w", err)
				}

				if witVer == 0 && len(witnessProgram) == 20 && spec == bech32 {
					// P2WPKH - witness version 0, 20-byte hash
					addr, err := btcutil.NewAddressWitnessPubKeyHash(witnessProgram, netParams)
					if err != nil {
						return nil, nil, err
					}
					// P2WPKH script: OP_0 <20-byte hash>
					script := append([]byte{txscript.OP_0, txscript.OP_DATA_20}, witnessProgram...)
					return addr, script, nil
				}

				if witVer == 0 && len(witnessProgram) == 32 && spec == bech32 {
					// P2WSH - witness version 0, 32-byte script hash
					addr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram, netParams)
					if err != nil {
						return nil, nil, err
					}
					// P2WSH script: OP_0 <32-byte hash>
					script := append([]byte{txscript.OP_0, txscript.OP_DATA_32}, witnessProgram...)
					return addr, script, nil
				}

				if witVer == 1 && len(witnessProgram) == 32 && spec == bech32m {
					// P2TR - witness version 1, 32-byte pubkey
					addr, err := btcutil.NewAddressTaproot(witnessProgram, netParams)
					if err != nil {
						return nil, nil, err
					}
					// P2TR script: OP_1 <32-byte pubkey>
					script := append([]byte{txscript.OP_1, txscript.OP_DATA_32}, witnessProgram...)
					return addr, script, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("decoded address is of unknown format: %s", address)
}

// bech32 decoding constants
const (
	bech32  = 1
	bech32m = 2
)

// decodeBech32 decodes a bech32/bech32m string.
func decodeBech32(str string) (string, []byte, int, error) {
	if len(str) < 8 {
		return "", nil, 0, fmt.Errorf("invalid bech32 string length")
	}

	// Find separator
	sepPos := -1
	for i := len(str) - 1; i >= 0; i-- {
		if str[i] == '1' {
			sepPos = i
			break
		}
	}
	if sepPos < 1 || sepPos+7 > len(str) {
		return "", nil, 0, fmt.Errorf("invalid bech32 separator position")
	}

	hrp := str[:sepPos]
	dataStr := str[sepPos+1:]

	// Decode charset
	charset := "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	data := make([]byte, len(dataStr))
	for i, c := range dataStr {
		idx := -1
		for j, cc := range charset {
			if byte(c) == byte(cc) {
				idx = j
				break
			}
		}
		if idx == -1 {
			return "", nil, 0, fmt.Errorf("invalid character in data")
		}
		data[i] = byte(idx)
	}

	// Verify checksum
	spec := bech32VerifyChecksum(hrp, data)
	if spec == 0 {
		return "", nil, 0, fmt.Errorf("invalid checksum")
	}

	// Remove checksum (last 6 bytes)
	return hrp, data[:len(data)-6], spec, nil
}

// bech32VerifyChecksum verifies the checksum and returns the encoding type.
func bech32VerifyChecksum(hrp string, data []byte) int {
	polymod := bech32Polymod(append(bech32HRPExpand(hrp), data...))
	if polymod == 1 {
		return bech32
	}
	if polymod == 0x2bc830a3 {
		return bech32m
	}
	return 0
}

func bech32HRPExpand(hrp string) []byte {
	result := make([]byte, len(hrp)*2+1)
	for i, c := range hrp {
		result[i] = byte(c >> 5)
		result[i+len(hrp)+1] = byte(c & 31)
	}
	result[len(hrp)] = 0
	return result
}

func bech32Polymod(values []byte) uint32 {
	gen := []uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		b := chk >> 25
		chk = ((chk & 0x1ffffff) << 5) ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (b>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32ConvertBits(data []byte, fromBits, toBits uint, pad bool) ([]byte, error) {
	acc := uint32(0)
	bits := uint(0)
	var result []byte
	maxv := uint32((1 << toBits) - 1)

	for _, b := range data {
		acc = (acc << fromBits) | uint32(b)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((acc>>bits)&maxv))
		}
	}

	if pad {
		if bits > 0 {
			result = append(result, byte((acc<<(toBits-bits))&maxv))
		}
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	return result, nil
}
