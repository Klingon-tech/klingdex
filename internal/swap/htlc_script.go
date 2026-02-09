// Package swap - HTLC script building for atomic swaps.
// This file contains functions for building HTLC (Hash Time-Locked Contract) scripts
// and deriving P2WSH addresses for Bitcoin-family chains.
package swap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/pkg/helpers"
)

// HTLCScriptData contains all data needed to spend an HTLC output.
type HTLCScriptData struct {
	// The full HTLC script (used in witness)
	Script []byte

	// P2WSH address derived from script
	Address string

	// Script hash (SHA256 of script, used in output scriptPubKey)
	ScriptHash []byte

	// Components
	SecretHash     []byte // SHA256 hash that must be revealed to claim
	ReceiverPubKey []byte // Who can claim with secret
	SenderPubKey   []byte // Who can refund after timeout
	TimeoutBlocks  uint32 // CSV relative timelock for refund
}

// BuildHTLCScript creates an HTLC script for atomic swaps.
//
// Script structure:
//
//	OP_IF
//	    OP_SHA256 <secret_hash> OP_EQUALVERIFY
//	    <receiver_pubkey> OP_CHECKSIG
//	OP_ELSE
//	    <timeout_blocks> OP_CHECKSEQUENCEVERIFY OP_DROP
//	    <sender_pubkey> OP_CHECKSIG
//	OP_ENDIF
//
// Claim path (OP_IF branch): Requires secret + receiver signature
// Refund path (OP_ELSE branch): Requires sender signature after timeout
//
// Parameters:
//   - secretHash: 32-byte SHA256 hash of the secret
//   - receiverPubKey: 33-byte compressed public key of the receiver (claims with secret)
//   - senderPubKey: 33-byte compressed public key of the sender (can refund after timeout)
//   - timeoutBlocks: relative timelock in blocks (CSV)
func BuildHTLCScript(secretHash, receiverPubKey, senderPubKey []byte, timeoutBlocks uint32) ([]byte, error) {
	// Validate inputs
	if len(secretHash) != 32 {
		return nil, fmt.Errorf("secret hash must be 32 bytes, got %d", len(secretHash))
	}
	if len(receiverPubKey) != 33 {
		return nil, fmt.Errorf("receiver pubkey must be 33 bytes (compressed), got %d", len(receiverPubKey))
	}
	if len(senderPubKey) != 33 {
		return nil, fmt.Errorf("sender pubkey must be 33 bytes (compressed), got %d", len(senderPubKey))
	}
	if timeoutBlocks == 0 {
		return nil, fmt.Errorf("timeout blocks must be greater than 0")
	}
	if timeoutBlocks > 0xFFFF {
		return nil, fmt.Errorf("timeout blocks exceeds maximum CSV value (65535)")
	}

	// Build the script using txscript builder
	builder := txscript.NewScriptBuilder()

	// OP_IF branch (claim with secret)
	builder.AddOp(txscript.OP_IF)
	builder.AddOp(txscript.OP_SHA256)
	builder.AddData(secretHash)
	builder.AddOp(txscript.OP_EQUALVERIFY)
	builder.AddData(receiverPubKey)
	builder.AddOp(txscript.OP_CHECKSIG)

	// OP_ELSE branch (refund after timeout)
	builder.AddOp(txscript.OP_ELSE)
	builder.AddInt64(int64(timeoutBlocks))
	builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)
	builder.AddOp(txscript.OP_DROP)
	builder.AddData(senderPubKey)
	builder.AddOp(txscript.OP_CHECKSIG)

	// OP_ENDIF
	builder.AddOp(txscript.OP_ENDIF)

	return builder.Script()
}

// BuildHTLCScriptData creates complete HTLC data including script and address.
func BuildHTLCScriptData(
	secretHash []byte,
	receiverPubKey, senderPubKey *btcec.PublicKey,
	timeoutBlocks uint32,
	symbol string,
	network chain.Network,
) (*HTLCScriptData, error) {
	// Get compressed pubkey bytes
	receiverBytes := receiverPubKey.SerializeCompressed()
	senderBytes := senderPubKey.SerializeCompressed()

	// Build the script
	script, err := BuildHTLCScript(secretHash, receiverBytes, senderBytes, timeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to build HTLC script: %w", err)
	}

	// Calculate script hash (SHA256 of script for P2WSH)
	scriptHash := sha256.Sum256(script)

	// Get chain params
	chainParams, err := getHTLCChainParams(symbol, network)
	if err != nil {
		return nil, err
	}

	// Create P2WSH address
	address, err := btcutil.NewAddressWitnessScriptHash(scriptHash[:], chainParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2WSH address: %w", err)
	}

	return &HTLCScriptData{
		Script:         script,
		Address:        address.EncodeAddress(),
		ScriptHash:     scriptHash[:],
		SecretHash:     secretHash,
		ReceiverPubKey: receiverBytes,
		SenderPubKey:   senderBytes,
		TimeoutBlocks:  timeoutBlocks,
	}, nil
}

// BuildHTLCClaimWitness creates the witness stack for claiming an HTLC with the secret.
//
// Witness stack (bottom to top):
//
//	<signature>
//	<secret>
//	<1> (selects OP_IF branch)
//	<script>
func BuildHTLCClaimWitness(signature, secret, script []byte) [][]byte {
	return [][]byte{
		signature,
		secret,
		{0x01}, // OP_TRUE to select OP_IF branch
		script,
	}
}

// BuildHTLCRefundWitness creates the witness stack for refunding an HTLC after timeout.
//
// Witness stack (bottom to top):
//
//	<signature>
//	<0> (selects OP_ELSE branch)
//	<script>
func BuildHTLCRefundWitness(signature, script []byte) [][]byte {
	return [][]byte{
		signature,
		{}, // Empty to select OP_ELSE branch
		script,
	}
}

// BuildP2WSHScriptPubKey creates the scriptPubKey for a P2WSH output.
// Format: OP_0 <32-byte-script-hash>
func BuildP2WSHScriptPubKey(script []byte) []byte {
	scriptHash := sha256.Sum256(script)
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_0)
	builder.AddData(scriptHash[:])
	scriptPubKey, _ := builder.Script()
	return scriptPubKey
}

// GenerateSecret generates a cryptographically secure 32-byte secret
// and returns both the secret and its SHA256 hash.
func GenerateSecret() (secret, hash []byte, err error) {
	secret, err = helpers.GenerateSecureRandom(32)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	hashArray := sha256.Sum256(secret)
	return secret, hashArray[:], nil
}

// VerifySecret checks if a secret matches the expected hash.
func VerifySecret(secret, expectedHash []byte) bool {
	if len(secret) != 32 || len(expectedHash) != 32 {
		return false
	}
	actualHash := sha256.Sum256(secret)
	return helpers.ConstantTimeCompare(actualHash[:], expectedHash)
}

// getHTLCChainParams returns btcd chaincfg params for HTLC address generation.
func getHTLCChainParams(symbol string, network chain.Network) (*chaincfg.Params, error) {
	chainParams, ok := chain.Get(symbol, network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}

	switch symbol {
	case "BTC":
		if network == chain.Testnet {
			return &chaincfg.TestNet3Params, nil
		}
		return &chaincfg.MainNetParams, nil

	case "LTC":
		// Create LTC params
		ltcParams := createLTCParams(chainParams)
		return ltcParams, nil

	default:
		return nil, fmt.Errorf("HTLC not supported for chain: %s", symbol)
	}
}

// HTLCAddressFromScript derives a P2WSH address from an HTLC script.
func HTLCAddressFromScript(script []byte, symbol string, network chain.Network) (string, error) {
	chainParams, err := getHTLCChainParams(symbol, network)
	if err != nil {
		return "", err
	}

	scriptHash := sha256.Sum256(script)
	address, err := btcutil.NewAddressWitnessScriptHash(scriptHash[:], chainParams)
	if err != nil {
		return "", fmt.Errorf("failed to create P2WSH address: %w", err)
	}

	return address.EncodeAddress(), nil
}

// ParseHTLCScript parses an HTLC script and extracts its components.
// Returns secretHash, receiverPubKey, senderPubKey, timeoutBlocks, or error.
func ParseHTLCScript(script []byte) (secretHash, receiverPubKey, senderPubKey []byte, timeoutBlocks uint32, err error) {
	// Tokenize the script
	tokenizer := txscript.MakeScriptTokenizer(0, script)

	// Expected structure:
	// OP_IF OP_SHA256 <secretHash> OP_EQUALVERIFY <receiverPubKey> OP_CHECKSIG
	// OP_ELSE <timeoutBlocks> OP_CSV OP_DROP <senderPubKey> OP_CHECKSIG
	// OP_ENDIF

	// OP_IF
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_IF {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_IF")
	}

	// OP_SHA256
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_SHA256 {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_SHA256")
	}

	// <secretHash> - 32 bytes
	if !tokenizer.Next() {
		return nil, nil, nil, 0, fmt.Errorf("expected secret hash")
	}
	secretHash = tokenizer.Data()
	if len(secretHash) != 32 {
		return nil, nil, nil, 0, fmt.Errorf("secret hash must be 32 bytes")
	}

	// OP_EQUALVERIFY
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_EQUALVERIFY {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_EQUALVERIFY")
	}

	// <receiverPubKey> - 33 bytes
	if !tokenizer.Next() {
		return nil, nil, nil, 0, fmt.Errorf("expected receiver pubkey")
	}
	receiverPubKey = tokenizer.Data()
	if len(receiverPubKey) != 33 {
		return nil, nil, nil, 0, fmt.Errorf("receiver pubkey must be 33 bytes")
	}

	// OP_CHECKSIG
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_CHECKSIG {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_CHECKSIG")
	}

	// OP_ELSE
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_ELSE {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_ELSE")
	}

	// <timeoutBlocks> - variable length integer
	if !tokenizer.Next() {
		return nil, nil, nil, 0, fmt.Errorf("expected timeout blocks")
	}
	op := tokenizer.Opcode()
	if txscript.IsSmallInt(op) {
		// Small int (0-16) - use AsSmallInt to get the value
		timeoutBlocks = uint32(txscript.AsSmallInt(op))
	} else {
		// Not a small int, parse from data push
		data := tokenizer.Data()
		if len(data) == 0 {
			return nil, nil, nil, 0, fmt.Errorf("invalid timeout blocks: expected data push")
		}
		timeoutBlocks = 0
		for i := 0; i < len(data); i++ {
			timeoutBlocks |= uint32(data[i]) << (8 * i)
		}
	}

	// OP_CHECKSEQUENCEVERIFY
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_CHECKSEQUENCEVERIFY {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_CHECKSEQUENCEVERIFY")
	}

	// OP_DROP
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_DROP {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_DROP")
	}

	// <senderPubKey> - 33 bytes
	if !tokenizer.Next() {
		return nil, nil, nil, 0, fmt.Errorf("expected sender pubkey")
	}
	senderPubKey = tokenizer.Data()
	if len(senderPubKey) != 33 {
		return nil, nil, nil, 0, fmt.Errorf("sender pubkey must be 33 bytes")
	}

	// OP_CHECKSIG
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_CHECKSIG {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_CHECKSIG")
	}

	// OP_ENDIF
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_ENDIF {
		return nil, nil, nil, 0, fmt.Errorf("expected OP_ENDIF")
	}

	return secretHash, receiverPubKey, senderPubKey, timeoutBlocks, nil
}

// HTLCScriptHex returns the script as a hex string.
func (h *HTLCScriptData) HTLCScriptHex() string {
	return hex.EncodeToString(h.Script)
}
