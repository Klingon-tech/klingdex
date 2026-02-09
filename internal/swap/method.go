// Package swap - Method abstraction for atomic swap protocols.
// This file defines the interfaces that swap methods (MuSig2, HTLC, etc.) must implement.
package swap

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/Klingon-tech/klingdex/internal/chain"
)

// ErrUnsupportedMethod is returned when an unsupported swap method is requested.
var ErrUnsupportedMethod = errors.New("unsupported swap method")

// SwapMethodHandler is the common interface for all swap methods.
// Both MuSig2Session and HTLCSession implement this interface.
type SwapMethodHandler interface {
	// Identity returns the method type and chain symbol
	Method() Method
	Symbol() string
	Network() chain.Network

	// Key exchange
	GetLocalPubKey() *btcec.PublicKey
	GetLocalPubKeyBytes() []byte
	SetRemotePubKey(pubkey *btcec.PublicKey) error

	// Address generation - returns the swap address for funding
	// For MuSig2: P2TR (Taproot) address
	// For HTLC: P2WSH (SegWit) address
	GenerateSwapAddress(timeoutBlocks uint32, refundPubKey *btcec.PublicKey) (string, error)
	GetSwapAddress() string

	// Serialization for storage persistence
	MarshalStorageData() ([]byte, error)
}

// MuSig2MethodHandler extends SwapMethodHandler with MuSig2-specific operations.
type MuSig2MethodHandler interface {
	SwapMethodHandler

	// Nonce exchange
	GenerateNoncesBytes() ([]byte, error)
	SetRemoteNonceBytes(nonce []byte) error
	HasRemoteNonce() bool

	// Signing
	InitSigningSession() error
	SignSighash(sighash []byte) (*PartialSignature, error)
	CombinePartialSignatures(localSig, remoteSig *PartialSignature) ([]byte, error)

	// Taproot-specific
	GetAggregatedPubKey() *btcec.PublicKey
	GetScriptTree() *TaprootScriptTree
}

// HTLCMethodHandler extends SwapMethodHandler with HTLC-specific operations.
type HTLCMethodHandler interface {
	SwapMethodHandler

	// Secret handling
	// Initiator calls GenerateSecret() to create secret + hash
	// Responder calls SetSecretHash() with hash received from initiator
	GenerateSecret() (secret, hash []byte, err error)
	SetSecretHash(hash []byte) error
	GetSecretHash() []byte

	// Set secret after it's revealed on-chain (for responder to claim)
	SetSecret(secret []byte) error
	GetSecret() []byte
	HasSecret() bool

	// Witness building for claim/refund
	// Claim: requires secret + signature
	// Refund: requires signature only (after timeout)
	BuildClaimWitness(secret []byte, sig []byte) ([][]byte, error)
	BuildRefundWitness(sig []byte) ([][]byte, error)

	// Script access
	GetHTLCScript() []byte
	GetRedeemScript() []byte

	// Signing (ECDSA for HTLC)
	SignHash(hash []byte) ([]byte, error)
}

// PartialSignature wraps a MuSig2 partial signature for storage and transport.
type PartialSignature struct {
	Bytes []byte // 32-byte partial signature
}

// NewMuSig2MethodHandler creates a new MuSig2 method handler.
// This is the factory function for creating MuSig2 handlers.
func NewMuSig2MethodHandler(symbol string, network chain.Network, privKey *btcec.PrivateKey) (MuSig2MethodHandler, error) {
	return NewMuSig2Session(symbol, network, privKey)
}

// NewHTLCMethodHandler creates a new HTLC method handler.
// This is the factory function for creating HTLC handlers.
func NewHTLCMethodHandler(symbol string, network chain.Network) (HTLCMethodHandler, error) {
	return NewHTLCSession(symbol, network)
}

// NewHTLCMethodHandlerWithKey creates an HTLC handler with a pre-existing private key.
// Used when recovering from storage.
func NewHTLCMethodHandlerWithKey(symbol string, network chain.Network, privKey *btcec.PrivateKey) (HTLCMethodHandler, error) {
	return NewHTLCSessionWithKey(symbol, network, privKey)
}
