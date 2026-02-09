// Package swap - Taproot script building for atomic swaps.
// This file contains logic for building Taproot script trees with timelock refunds.
package swap

import (
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// Default timelock values in blocks
const (
	// DefaultMakerTimeoutBlocks is the default timeout for the maker (initiator)
	// ~24 hours on Bitcoin (~10 min blocks)
	DefaultMakerTimeoutBlocks = 144

	// DefaultTakerTimeoutBlocks is the default timeout for the taker (responder)
	// ~12 hours on Bitcoin - must be shorter than maker's timeout
	DefaultTakerTimeoutBlocks = 72

	// LTCMakerTimeoutBlocks is the default timeout for LTC maker
	// ~24 hours on Litecoin (~2.5 min blocks)
	LTCMakerTimeoutBlocks = 576

	// LTCTakerTimeoutBlocks is the default timeout for LTC taker
	// ~12 hours on Litecoin
	LTCTakerTimeoutBlocks = 288
)

// TaprootScriptTree represents a Taproot output with both key path and script path.
type TaprootScriptTree struct {
	// InternalKey is the untweaked aggregated MuSig2 public key
	InternalKey *btcec.PublicKey

	// TweakedKey is the final output key (tweaked with script tree root)
	TweakedKey *btcec.PublicKey

	// RefundScript is the raw refund script bytes
	RefundScript []byte

	// RefundLeaf is the TapLeaf containing the refund script
	RefundLeaf txscript.TapLeaf

	// MerkleRoot is the root hash of the script tree
	MerkleRoot []byte

	// ControlBlock is the proof needed for script path spending
	ControlBlock []byte

	// TimeoutBlocks is the CSV relative timelock in blocks
	TimeoutBlocks uint32
}

// BuildRefundScript creates a timelock refund script using OP_CHECKSEQUENCEVERIFY.
// Script: <timeout_blocks> OP_CHECKSEQUENCEVERIFY OP_DROP <pubkey> OP_CHECKSIG
//
// This script can only be spent after `timeoutBlocks` blocks have passed
// since the funding transaction was confirmed.
func BuildRefundScript(pubKey *btcec.PublicKey, timeoutBlocks uint32) ([]byte, error) {
	if pubKey == nil {
		return nil, fmt.Errorf("pubkey cannot be nil")
	}
	if timeoutBlocks == 0 {
		return nil, fmt.Errorf("timeout blocks must be > 0")
	}
	if timeoutBlocks > 0xFFFF {
		// CSV only supports 16-bit values for relative lock
		return nil, fmt.Errorf("timeout blocks too large (max 65535)")
	}

	// Build the script using script builder
	builder := txscript.NewScriptBuilder()

	// Push the timeout value (CSV interprets this as relative block height)
	builder.AddInt64(int64(timeoutBlocks))

	// OP_CHECKSEQUENCEVERIFY - verify relative timelock
	builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)

	// OP_DROP - remove the timeout value from stack
	builder.AddOp(txscript.OP_DROP)

	// Push the public key (x-only for Taproot, 32 bytes)
	xOnlyKey := schnorr.SerializePubKey(pubKey)
	builder.AddData(xOnlyKey)

	// OP_CHECKSIG - verify signature (Schnorr for Taproot)
	builder.AddOp(txscript.OP_CHECKSIG)

	return builder.Script()
}

// BuildTaprootScriptTree creates a Taproot output with key path (MuSig2) and script path (refund).
//
// The resulting address can be spent via:
// 1. Key path: MuSig2 aggregated signature (happy path - both parties agree)
// 2. Script path: Single signature after timeout (refund path)
func BuildTaprootScriptTree(
	aggregatedKey *btcec.PublicKey,
	refundPubKey *btcec.PublicKey,
	timeoutBlocks uint32,
) (*TaprootScriptTree, error) {
	if aggregatedKey == nil {
		return nil, fmt.Errorf("aggregated key cannot be nil")
	}
	if refundPubKey == nil {
		return nil, fmt.Errorf("refund pubkey cannot be nil")
	}

	// Build the refund script
	refundScript, err := BuildRefundScript(refundPubKey, timeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to build refund script: %w", err)
	}

	// Create a TapLeaf from the refund script
	// Using the default leaf version (0xC0)
	refundLeaf := txscript.NewBaseTapLeaf(refundScript)

	// Build the Taproot script tree with a single leaf
	// For a single leaf, the tree is just the leaf itself
	tapScriptTree := txscript.AssembleTaprootScriptTree(refundLeaf)

	// Get the merkle root
	merkleRoot := tapScriptTree.RootNode.TapHash()

	// Compute the tweaked output key
	// outputKey = internalKey + H(internalKey || merkleRoot) * G
	tweakedKey := txscript.ComputeTaprootOutputKey(aggregatedKey, merkleRoot[:])

	// Build the control block for script path spending
	// Control block = <leaf_version + parity> || <internal_key> || <merkle_path>
	ctrlBlock := tapScriptTree.LeafMerkleProofs[0].ToControlBlock(aggregatedKey)
	ctrlBlockBytes, err := ctrlBlock.ToBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize control block: %w", err)
	}

	return &TaprootScriptTree{
		InternalKey:   aggregatedKey,
		TweakedKey:    tweakedKey,
		RefundScript:  refundScript,
		RefundLeaf:    refundLeaf,
		MerkleRoot:    merkleRoot[:],
		ControlBlock:  ctrlBlockBytes,
		TimeoutBlocks: timeoutBlocks,
	}, nil
}

// TaprootAddress returns the bech32m P2TR address for this script tree.
func (t *TaprootScriptTree) TaprootAddress(hrp string) (string, error) {
	if t.TweakedKey == nil {
		return "", fmt.Errorf("tweaked key not set")
	}

	// Use the existing encodeTaprootAddress helper from musig2.go
	return encodeTaprootAddress(t.TweakedKey, hrp)
}

// ScriptPubKey returns the P2TR scriptPubKey for this tree.
func (t *TaprootScriptTree) ScriptPubKey() ([]byte, error) {
	if t.TweakedKey == nil {
		return nil, fmt.Errorf("tweaked key not set")
	}

	// P2TR script: OP_1 <32-byte-x-only-pubkey>
	xOnlyKey := schnorr.SerializePubKey(t.TweakedKey)

	script := make([]byte, 34)
	script[0] = txscript.OP_1
	script[1] = txscript.OP_DATA_32
	copy(script[2:], xOnlyKey)

	return script, nil
}

// RefundScriptHex returns the hex-encoded refund script.
func (t *TaprootScriptTree) RefundScriptHex() string {
	return hex.EncodeToString(t.RefundScript)
}

// ControlBlockHex returns the hex-encoded control block.
func (t *TaprootScriptTree) ControlBlockHex() string {
	return hex.EncodeToString(t.ControlBlock)
}

// BuildRefundWitness creates the witness data for spending via the refund script path.
// Witness: [signature, refund_script, control_block]
func (t *TaprootScriptTree) BuildRefundWitness(sig *schnorr.Signature) wire.TxWitness {
	return wire.TxWitness{
		sig.Serialize(),
		t.RefundScript,
		t.ControlBlock,
	}
}

// GetTimeoutBlocks returns the timeout in blocks for a given chain.
func GetTimeoutBlocks(symbol string, isMaker bool) uint32 {
	switch symbol {
	case "LTC":
		if isMaker {
			return LTCMakerTimeoutBlocks
		}
		return LTCTakerTimeoutBlocks
	default: // BTC and others
		if isMaker {
			return DefaultMakerTimeoutBlocks
		}
		return DefaultTakerTimeoutBlocks
	}
}

// ValidateTimeoutRelationship ensures maker timeout > taker timeout.
// This is critical for atomic swap security.
func ValidateTimeoutRelationship(makerBlocks, takerBlocks uint32) error {
	if makerBlocks <= takerBlocks {
		return fmt.Errorf("maker timeout (%d) must be greater than taker timeout (%d)", makerBlocks, takerBlocks)
	}
	// Ensure there's enough safety margin (at least 10% difference)
	minDiff := takerBlocks / 10
	if minDiff < 6 {
		minDiff = 6 // At least 6 blocks (~1 hour BTC)
	}
	if makerBlocks-takerBlocks < minDiff {
		return fmt.Errorf("insufficient timeout difference: maker=%d, taker=%d, need at least %d blocks difference",
			makerBlocks, takerBlocks, minDiff)
	}
	return nil
}

// Note: bech32 encoding functions (bech32mEncode, bech32CreateChecksum, etc.)
// are defined in musig2.go to avoid duplication
