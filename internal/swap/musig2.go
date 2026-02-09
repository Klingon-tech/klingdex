// Package swap - MuSig2 protocol implementation for atomic swaps.
// This file contains MuSig2-specific logic: key aggregation, nonce handling, and signing.
package swap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/pkg/helpers"
)

// MuSig2 errors
var (
	ErrNonceNotSet       = errors.New("nonce not set")
	ErrSessionNotReady   = errors.New("session not ready for signing")
	ErrSigningFailed     = errors.New("signing failed")
	ErrNonceAlreadyUsed  = errors.New("nonce already used - generate new nonces")
	ErrNonceReuse        = errors.New("attempted nonce reuse detected")
	ErrSessionInvalidated = errors.New("session invalidated after signing")
)

// MuSig2Session holds the state for a MuSig2 signing session.
// One session per swap, per chain (each side of the swap has its own session).
//
// SECURITY: This session tracks nonce usage to prevent catastrophic nonce reuse.
// If a nonce is used for signing, the session is invalidated and new nonces must
// be generated before signing again. Reusing MuSig2 nonces LEAKS THE PRIVATE KEY.
type MuSig2Session struct {
	// Chain this session is for
	symbol  string
	network chain.Network

	// Generated swap address (stored after GenerateSwapAddress)
	swapAddress string

	// Keys
	localPrivKey *btcec.PrivateKey
	localPubKey  *btcec.PublicKey
	remotePubKey *btcec.PublicKey

	// Aggregated key (computed after both pubkeys are known)
	aggregatedKey *musig2.AggregateKey

	// Nonce state with security tracking
	localNonces    *musig2.Nonces
	remoteNonces   [musig2.PubNonceSize]byte
	hasRemoteNonce bool

	// SECURITY: Track used nonces to prevent reuse
	// Key is the serialized public nonce bytes
	usedNonces map[[musig2.PubNonceSize]byte]bool

	// SECURITY: Flag if current nonce has been used for signing
	// Once true, session must not sign again without new nonces
	nonceUsed bool

	// SECURITY: Session invalidation flag
	// Once a session signs, it's invalidated to prevent accidental reuse
	invalidated bool

	// MuSig2 context and session
	context *musig2.Context
	session *musig2.Session

	// BIP-86 tweak for key-path spending
	tweakApplied bool

	// Taproot script tree (for refund path)
	// This is populated when TaprootAddressWithRefund is called
	scriptTree *TaprootScriptTree
}

// NewMuSig2Session creates a new MuSig2 session for a swap.
// The localPrivKey is the ephemeral key for this swap (NOT the HD wallet key).
func NewMuSig2Session(symbol string, network chain.Network, localPrivKey *btcec.PrivateKey) (*MuSig2Session, error) {
	// Verify chain supports Taproot
	params, ok := chain.Get(symbol, network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, symbol)
	}
	if !params.SupportsTaproot {
		return nil, fmt.Errorf("%w: %s", ErrTaprootNotSupported, symbol)
	}

	return &MuSig2Session{
		symbol:       symbol,
		network:      network,
		localPrivKey: localPrivKey,
		localPubKey:  localPrivKey.PubKey(),
		usedNonces:   make(map[[musig2.PubNonceSize]byte]bool), // SECURITY: Initialize nonce tracking
	}, nil
}

// =============================================================================
// SwapMethodHandler Interface Implementation
// =============================================================================

// Method returns the swap method type.
func (s *MuSig2Session) Method() Method {
	return MethodMuSig2
}

// Symbol returns the chain symbol.
func (s *MuSig2Session) Symbol() string {
	return s.symbol
}

// Network returns the network (mainnet/testnet).
func (s *MuSig2Session) Network() chain.Network {
	return s.network
}

// GetLocalPubKey returns the local public key.
func (s *MuSig2Session) GetLocalPubKey() *btcec.PublicKey {
	return s.localPubKey
}

// GetLocalPubKeyBytes returns the local public key as compressed bytes.
func (s *MuSig2Session) GetLocalPubKeyBytes() []byte {
	if s.localPubKey == nil {
		return nil
	}
	return s.localPubKey.SerializeCompressed()
}

// GenerateSwapAddress generates the P2TR swap address with refund capability.
// For MuSig2, this creates a Taproot address with a CSV refund script path.
func (s *MuSig2Session) GenerateSwapAddress(timeoutBlocks uint32, refundPubKey *btcec.PublicKey) (string, error) {
	if refundPubKey == nil {
		refundPubKey = s.localPubKey
	}
	addr, err := s.TaprootAddressWithRefund(refundPubKey, timeoutBlocks)
	if err != nil {
		return "", err
	}
	s.swapAddress = addr
	return addr, nil
}

// GetSwapAddress returns the previously generated swap address.
func (s *MuSig2Session) GetSwapAddress() string {
	return s.swapAddress
}


// SetRemotePubKey sets the counterparty's public key and computes the aggregated key.
func (s *MuSig2Session) SetRemotePubKey(remotePubKey *btcec.PublicKey) error {
	if remotePubKey == nil {
		return ErrInvalidPubKey
	}
	s.remotePubKey = remotePubKey

	// Compute aggregated key
	if err := s.computeAggregatedKey(); err != nil {
		return fmt.Errorf("failed to compute aggregated key: %w", err)
	}

	return nil
}

// computeAggregatedKey computes the MuSig2 aggregated public key.
func (s *MuSig2Session) computeAggregatedKey() error {
	if s.localPubKey == nil || s.remotePubKey == nil {
		return ErrInvalidPubKey
	}

	// Keys for aggregation
	keys := []*btcec.PublicKey{s.localPubKey, s.remotePubKey}

	// KeyAgg returns the aggregated key
	// sort=true ensures both parties compute the same key regardless of local/remote ordering
	aggKey, _, _, err := musig2.AggregateKeys(keys, true)
	if err != nil {
		return fmt.Errorf("key aggregation failed: %w", err)
	}

	s.aggregatedKey = aggKey
	return nil
}

// AggregatedPubKey returns the aggregated public key.
func (s *MuSig2Session) AggregatedPubKey() (*btcec.PublicKey, error) {
	if s.aggregatedKey == nil {
		return nil, ErrKeyAggregationFailed
	}
	return s.aggregatedKey.FinalKey, nil
}

// TweakedPubKey returns the BIP-86 tweaked public key for key-path spending.
// This is what goes into the Taproot output (P2TR address).
func (s *MuSig2Session) TweakedPubKey() (*btcec.PublicKey, error) {
	aggPubKey, err := s.AggregatedPubKey()
	if err != nil {
		return nil, err
	}

	// BIP-86: tweak = TaggedHash("TapTweak", pubkey_x)
	// For key-path-only spending, there's no script tree, so merkle_root is empty
	tweakedKey := txscript.ComputeTaprootOutputKey(aggPubKey, nil)
	return tweakedKey, nil
}

// TaprootAddress returns the P2TR address for the aggregated key.
func (s *MuSig2Session) TaprootAddress() (string, error) {
	tweakedKey, err := s.TweakedPubKey()
	if err != nil {
		return "", err
	}

	params, _ := chain.Get(s.symbol, s.network)

	// Create P2TR address
	// Format: witness version (1) + 32-byte x-only pubkey
	addr, err := encodeTaprootAddress(tweakedKey, params.Bech32HRP)
	if err != nil {
		return "", fmt.Errorf("failed to encode taproot address: %w", err)
	}

	return addr, nil
}

// TaprootAddressForChain returns the P2TR address for the aggregated key
// but encoded for a different chain. This is useful for atomic swaps where
// both parties use the same aggregated key but on different chains.
func (s *MuSig2Session) TaprootAddressForChain(symbol string) (string, error) {
	tweakedKey, err := s.TweakedPubKey()
	if err != nil {
		return "", err
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return "", fmt.Errorf("chain not found: %s", symbol)
	}
	if !params.SupportsTaproot {
		return "", fmt.Errorf("chain %s does not support Taproot", symbol)
	}

	addr, err := encodeTaprootAddress(tweakedKey, params.Bech32HRP)
	if err != nil {
		return "", fmt.Errorf("failed to encode taproot address: %w", err)
	}

	return addr, nil
}

// TaprootAddressWithRefund returns the P2TR address with a refund script path.
// This creates a Taproot output that can be spent either:
// 1. Key path: MuSig2 aggregated signature (cooperative spend)
// 2. Script path: Single signature after timeout (refund)
//
// refundPubKey is our public key for the refund path.
// timeoutBlocks is the CSV relative timelock in blocks.
func (s *MuSig2Session) TaprootAddressWithRefund(refundPubKey *btcec.PublicKey, timeoutBlocks uint32) (string, error) {
	aggPubKey, err := s.AggregatedPubKey()
	if err != nil {
		return "", err
	}

	// Build the Taproot script tree with refund script
	scriptTree, err := BuildTaprootScriptTree(aggPubKey, refundPubKey, timeoutBlocks)
	if err != nil {
		return "", fmt.Errorf("failed to build taproot script tree: %w", err)
	}

	// Store the script tree for later use (refund tx building)
	s.scriptTree = scriptTree

	// Get chain params for bech32 encoding
	params, _ := chain.Get(s.symbol, s.network)

	// Return the address using the tweaked key from the script tree
	return scriptTree.TaprootAddress(params.Bech32HRP)
}

// TaprootAddressWithRefundForChain creates a refund-enabled Taproot address for a different chain.
func (s *MuSig2Session) TaprootAddressWithRefundForChain(symbol string, refundPubKey *btcec.PublicKey, timeoutBlocks uint32) (string, error) {
	aggPubKey, err := s.AggregatedPubKey()
	if err != nil {
		return "", err
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return "", fmt.Errorf("chain not found: %s", symbol)
	}
	if !params.SupportsTaproot {
		return "", fmt.Errorf("chain %s does not support Taproot", symbol)
	}

	// Build the Taproot script tree with refund script
	scriptTree, err := BuildTaprootScriptTree(aggPubKey, refundPubKey, timeoutBlocks)
	if err != nil {
		return "", fmt.Errorf("failed to build taproot script tree: %w", err)
	}

	// Store the script tree for later use
	s.scriptTree = scriptTree

	return scriptTree.TaprootAddress(params.Bech32HRP)
}

// GetScriptTree returns the Taproot script tree if one was created.
// Returns nil if TaprootAddressWithRefund has not been called.
func (s *MuSig2Session) GetScriptTree() *TaprootScriptTree {
	return s.scriptTree
}

// SetScriptTree sets the Taproot script tree (for recovery from persistence).
func (s *MuSig2Session) SetScriptTree(tree *TaprootScriptTree) {
	s.scriptTree = tree
}

// encodeTaprootAddress encodes a public key as a bech32m P2TR address.
func encodeTaprootAddress(pubKey *btcec.PublicKey, hrp string) (string, error) {
	// Get x-only public key (32 bytes)
	xOnlyKey := schnorr.SerializePubKey(pubKey)

	// Encode as bech32m with witness version 1
	conv, err := bech32ConvertBits(xOnlyKey, 8, 5, true)
	if err != nil {
		return "", err
	}

	// Prepend witness version (1 for Taproot)
	data := append([]byte{0x01}, conv...)

	return bech32mEncode(hrp, data)
}

// GenerateNonces generates local nonces for a signing session.
//
// SECURITY: If previous nonces exist, they are marked as used to prevent reuse.
// This is critical because reusing MuSig2 nonces LEAKS THE PRIVATE KEY.
func (s *MuSig2Session) GenerateNonces() (*musig2.Nonces, error) {
	// SECURITY: Mark previous nonce as used if it exists
	if s.localNonces != nil {
		s.usedNonces[s.localNonces.PubNonce] = true
	}

	// Reset nonce usage flags for the new nonce
	s.nonceUsed = false
	s.invalidated = false

	nonces, err := musig2.GenNonces(
		musig2.WithPublicKey(s.localPubKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonces: %w", err)
	}

	// SECURITY: Check if we somehow generated a previously used nonce (extremely unlikely)
	if s.usedNonces[nonces.PubNonce] {
		return nil, fmt.Errorf("%w: regenerated a previously used nonce", ErrNonceReuse)
	}

	s.localNonces = nonces
	return nonces, nil
}

// LocalPubNonce returns our public nonce to share with the counterparty.
// Returns the 66-byte public nonce.
func (s *MuSig2Session) LocalPubNonce() ([musig2.PubNonceSize]byte, error) {
	if s.localNonces == nil {
		return [musig2.PubNonceSize]byte{}, ErrNonceNotSet
	}
	return s.localNonces.PubNonce, nil
}

// SetRemoteNonce sets the counterparty's public nonce.
func (s *MuSig2Session) SetRemoteNonce(nonce [musig2.PubNonceSize]byte) {
	s.remoteNonces = nonce
	s.hasRemoteNonce = true
}

// InitSigningSession initializes the MuSig2 signing session.
// Must be called after both nonces are set.
func (s *MuSig2Session) InitSigningSession() error {
	if s.localNonces == nil || !s.hasRemoteNonce {
		return ErrNonceNotSet
	}
	if s.aggregatedKey == nil {
		return ErrKeyAggregationFailed
	}

	// Create MuSig2 context
	// IMPORTANT: Keys must be in sorted order to match computeAggregatedKey
	allPubKeys := []*btcec.PublicKey{s.localPubKey, s.remotePubKey}
	// Sort keys lexicographically to ensure both parties have the same order
	if helpers.CompareBytes(s.localPubKey.SerializeCompressed(), s.remotePubKey.SerializeCompressed()) > 0 {
		allPubKeys = []*btcec.PublicKey{s.remotePubKey, s.localPubKey}
	}

	// Build context options
	ctxOpts := []musig2.ContextOption{
		musig2.WithKnownSigners(allPubKeys),
	}

	// CRITICAL: Apply the correct taproot tweak
	// If we have a script tree (from TaprootAddressWithRefund), we MUST apply the
	// taproot tweak with the Merkle root. Otherwise the signature won't match
	// the tweaked key used in the P2TR address.
	if s.scriptTree != nil && len(s.scriptTree.MerkleRoot) > 0 {
		// Apply taproot tweak with the script tree's Merkle root
		ctxOpts = append(ctxOpts, musig2.WithTaprootTweakCtx(s.scriptTree.MerkleRoot))
	}

	ctx, err := musig2.NewContext(
		s.localPrivKey,
		false, // Don't auto-tweak, we apply it via options above
		ctxOpts...,
	)
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}
	s.context = ctx

	// Create session with pre-generated nonces
	session, err := ctx.NewSession(musig2.WithPreGeneratedNonce(s.localNonces))
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Register remote nonce
	_, err = session.RegisterPubNonce(s.remoteNonces)
	if err != nil {
		return fmt.Errorf("failed to register remote nonce: %w", err)
	}

	s.session = session
	s.tweakApplied = true
	return nil
}

// Sign creates a partial signature for the given message hash.
//
// SECURITY: This method enforces nonce usage rules:
// - Returns error if nonce was already used
// - Marks nonce as used after signing
// - Invalidates session to prevent accidental reuse
//
// WARNING: After calling Sign, you MUST call GenerateNonces before signing again!
func (s *MuSig2Session) Sign(msgHash *chainhash.Hash) (*musig2.PartialSignature, error) {
	if s.session == nil {
		return nil, ErrSessionNotReady
	}

	// SECURITY: Check if session was invalidated
	if s.invalidated {
		return nil, ErrSessionInvalidated
	}

	// SECURITY: Check if nonce was already used
	if s.nonceUsed {
		return nil, ErrNonceAlreadyUsed
	}

	// SECURITY: Verify current nonce wasn't used before (belt and suspenders)
	if s.localNonces != nil && s.usedNonces[s.localNonces.PubNonce] {
		return nil, ErrNonceReuse
	}

	partialSig, err := s.session.Sign(*msgHash)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSigningFailed, err)
	}

	// SECURITY: Mark nonce as used IMMEDIATELY after signing
	s.nonceUsed = true
	if s.localNonces != nil {
		s.usedNonces[s.localNonces.PubNonce] = true
	}

	// SECURITY: Invalidate session to prevent any accidental reuse
	s.invalidated = true

	return partialSig, nil
}

// CombineSignatures combines partial signatures into a final Schnorr signature.
func (s *MuSig2Session) CombineSignatures(localSig, remoteSig *musig2.PartialSignature) (*schnorr.Signature, error) {
	if s.session == nil {
		return nil, ErrSessionNotReady
	}

	// Combine partial signatures
	haveFinal, err := s.session.CombineSig(remoteSig)
	if err != nil {
		return nil, fmt.Errorf("failed to combine signatures: %w", err)
	}

	if !haveFinal {
		return nil, errors.New("not enough signatures to finalize")
	}

	return s.session.FinalSig(), nil
}

// VerifySignature verifies a Schnorr signature against the tweaked public key.
func (s *MuSig2Session) VerifySignature(sig *schnorr.Signature, msgHash *chainhash.Hash) bool {
	tweakedKey, err := s.TweakedPubKey()
	if err != nil {
		return false
	}
	return sig.Verify(msgHash[:], tweakedKey)
}

// IsValid returns true if the session can be used for signing.
// Returns false if the session is invalidated or the nonce has been used.
func (s *MuSig2Session) IsValid() bool {
	return !s.invalidated && !s.nonceUsed
}

// IsInvalidated returns true if the session has been invalidated after signing.
func (s *MuSig2Session) IsInvalidated() bool {
	return s.invalidated
}

// NonceUsed returns true if the current nonce has been used for signing.
func (s *MuSig2Session) NonceUsed() bool {
	return s.nonceUsed
}

// UsedNonceCount returns the number of nonces that have been used in this session.
// This is useful for auditing/debugging.
func (s *MuSig2Session) UsedNonceCount() int {
	return len(s.usedNonces)
}

// ResetForNewSign prepares the session for a new signing operation.
// This generates new nonces and resets the invalidation state.
// MUST be called before signing again after a previous sign operation.
func (s *MuSig2Session) ResetForNewSign() error {
	// Generate new nonces (this marks old ones as used)
	_, err := s.GenerateNonces()
	if err != nil {
		return fmt.Errorf("failed to generate new nonces for reset: %w", err)
	}

	// Clear the session - it needs to be reinitialized with new nonces
	s.session = nil
	s.context = nil
	s.hasRemoteNonce = false
	s.remoteNonces = [musig2.PubNonceSize]byte{}

	return nil
}

// GenerateEphemeralKey generates a new ephemeral private key for a swap.
// This should be used instead of HD-derived keys for privacy.
func GenerateEphemeralKey() (*btcec.PrivateKey, error) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	return privKey, nil
}

// =============================================================================
// Persistence helpers for MuSig2Session
// =============================================================================

// IsNonceUsed returns true if the current nonce has been used for signing.
// Alias for NonceUsed() for consistency with coordinator storage interface.
func (s *MuSig2Session) IsNonceUsed() bool {
	return s.nonceUsed
}

// GetUsedNoncesHex returns all used nonces as hex-encoded strings.
// SECURITY CRITICAL: This must be persisted to prevent nonce reuse across restarts.
func (s *MuSig2Session) GetUsedNoncesHex() []string {
	result := make([]string, 0, len(s.usedNonces))
	for nonce := range s.usedNonces {
		result = append(result, fmt.Sprintf("%x", nonce[:]))
	}
	return result
}

// SetUsedNonces restores used nonces from hex-encoded strings.
// Call this when recovering a session from persistence.
func (s *MuSig2Session) SetUsedNonces(hexNonces []string) error {
	for _, hexNonce := range hexNonces {
		nonceBytes, err := hex.DecodeString(hexNonce)
		if err != nil {
			return fmt.Errorf("invalid hex nonce: %w", err)
		}
		if len(nonceBytes) != musig2.PubNonceSize {
			return fmt.Errorf("invalid nonce size: expected %d, got %d", musig2.PubNonceSize, len(nonceBytes))
		}
		var nonce [musig2.PubNonceSize]byte
		copy(nonce[:], nonceBytes)
		s.usedNonces[nonce] = true
	}
	return nil
}

// SetNonceUsed sets the nonceUsed flag (for recovery).
func (s *MuSig2Session) SetNonceUsed(used bool) {
	s.nonceUsed = used
}

// SetInvalidated sets the invalidated flag (for recovery).
func (s *MuSig2Session) SetInvalidated(invalid bool) {
	s.invalidated = invalid
}

// GetLocalPrivKey returns the local private key (for persistence).
func (s *MuSig2Session) GetLocalPrivKey() *btcec.PrivateKey {
	return s.localPrivKey
}

// SerializePrivKey serializes a private key to 32 bytes.
func SerializePrivKey(privKey *btcec.PrivateKey) []byte {
	return privKey.Serialize()
}

// DeserializePrivKey deserializes a 32-byte private key.
func DeserializePrivKey(data []byte) (*btcec.PrivateKey, *btcec.PublicKey) {
	privKey, pubKey := btcec.PrivKeyFromBytes(data)
	return privKey, pubKey
}

// ComputeSwapID computes a unique swap ID from both parties' public keys.
// This ensures both parties compute the same ID.
func ComputeSwapID(pubKey1, pubKey2 *btcec.PublicKey) string {
	// Sort keys to ensure deterministic ordering
	key1 := pubKey1.SerializeCompressed()
	key2 := pubKey2.SerializeCompressed()

	var combined []byte
	// Lexicographic ordering
	if helpers.CompareBytes(key1, key2) < 0 {
		combined = append(key1, key2...)
	} else {
		combined = append(key2, key1...)
	}

	hash := sha256.Sum256(combined)
	return fmt.Sprintf("%x", hash[:16]) // 16 bytes = 32 hex chars
}

// bech32 encoding helpers (minimal implementation for P2TR addresses)
const bech32mConst = 0x2bc830a3

var bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

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
		return nil, errors.New("invalid padding")
	}

	return result, nil
}

func bech32mEncode(hrp string, data []byte) (string, error) {
	// Compute checksum
	values := append(bech32HRPExpand(hrp), data...)
	polymod := bech32Polymod(append(values, []byte{0, 0, 0, 0, 0, 0}...)) ^ bech32mConst

	checksum := make([]byte, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = byte((polymod >> (5 * (5 - i))) & 31)
	}

	// Encode
	result := hrp + "1"
	for _, d := range append(data, checksum...) {
		result += string(bech32Charset[d])
	}

	return result, nil
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

// =============================================================================
// MuSig2MethodHandler Interface Implementation
// =============================================================================

// MuSig2SessionData is the serializable form of MuSig2Session for persistence.
// This is separate from MuSig2StorageData in coordinator.go which handles swap-level data.
type MuSig2SessionData struct {
	Symbol        string   `json:"symbol"`
	Network       string   `json:"network"`
	LocalPrivKey  string   `json:"local_priv_key"`  // Hex-encoded
	LocalPubKey   string   `json:"local_pub_key"`   // Hex-encoded
	RemotePubKey  string   `json:"remote_pub_key,omitempty"`
	SwapAddress   string   `json:"swap_address,omitempty"`
	NonceUsed     bool     `json:"nonce_used"`
	Invalidated   bool     `json:"invalidated"`
	UsedNonces    []string `json:"used_nonces,omitempty"` // Hex-encoded used nonces
	LocalPubNonce string   `json:"local_pub_nonce,omitempty"`
}

// MarshalStorageData serializes the session for storage.
func (s *MuSig2Session) MarshalStorageData() ([]byte, error) {
	data := MuSig2SessionData{
		Symbol:      s.symbol,
		Network:     string(s.network),
		SwapAddress: s.swapAddress,
		NonceUsed:   s.nonceUsed,
		Invalidated: s.invalidated,
	}

	if s.localPrivKey != nil {
		data.LocalPrivKey = hex.EncodeToString(s.localPrivKey.Serialize())
	}
	if s.localPubKey != nil {
		data.LocalPubKey = hex.EncodeToString(s.localPubKey.SerializeCompressed())
	}
	if s.remotePubKey != nil {
		data.RemotePubKey = hex.EncodeToString(s.remotePubKey.SerializeCompressed())
	}
	if s.localNonces != nil {
		data.LocalPubNonce = hex.EncodeToString(s.localNonces.PubNonce[:])
	}
	data.UsedNonces = s.GetUsedNoncesHex()

	return json.Marshal(data)
}

// GenerateNoncesBytes generates local nonces and returns the public nonce as bytes.
// This is the MuSig2MethodHandler interface method.
func (s *MuSig2Session) GenerateNoncesBytes() ([]byte, error) {
	nonces, err := s.GenerateNonces()
	if err != nil {
		return nil, err
	}
	return nonces.PubNonce[:], nil
}

// SetRemoteNonceBytes sets the counterparty's public nonce from bytes.
// This is the MuSig2MethodHandler interface method.
func (s *MuSig2Session) SetRemoteNonceBytes(nonce []byte) error {
	if len(nonce) != musig2.PubNonceSize {
		return fmt.Errorf("invalid nonce size: expected %d, got %d", musig2.PubNonceSize, len(nonce))
	}
	var nonceArr [musig2.PubNonceSize]byte
	copy(nonceArr[:], nonce)
	s.SetRemoteNonce(nonceArr)
	return nil
}

// HasRemoteNonce returns true if the remote nonce has been set.
func (s *MuSig2Session) HasRemoteNonce() bool {
	return s.hasRemoteNonce
}

// SignSighash creates a partial signature for the given sighash bytes.
// This is the MuSig2MethodHandler interface method.
func (s *MuSig2Session) SignSighash(sighash []byte) (*PartialSignature, error) {
	if len(sighash) != 32 {
		return nil, fmt.Errorf("invalid sighash length: expected 32, got %d", len(sighash))
	}
	msgHash, err := chainhash.NewHash(sighash)
	if err != nil {
		return nil, fmt.Errorf("failed to create hash: %w", err)
	}
	partialSig, err := s.Sign(msgHash)
	if err != nil {
		return nil, err
	}
	// Serialize the partial signature
	var buf bytes.Buffer
	if err := partialSig.Encode(&buf); err != nil {
		return nil, fmt.Errorf("failed to encode partial signature: %w", err)
	}
	return &PartialSignature{Bytes: buf.Bytes()}, nil
}

// CombinePartialSignatures combines partial signatures into a final Schnorr signature.
// This is the MuSig2MethodHandler interface method.
func (s *MuSig2Session) CombinePartialSignatures(localSig, remoteSig *PartialSignature) ([]byte, error) {
	// Convert PartialSignature to musig2.PartialSignature
	localPartial := new(musig2.PartialSignature)
	if err := localPartial.Decode(bytes.NewReader(localSig.Bytes)); err != nil {
		return nil, fmt.Errorf("failed to decode local partial sig: %w", err)
	}

	remotePartial := new(musig2.PartialSignature)
	if err := remotePartial.Decode(bytes.NewReader(remoteSig.Bytes)); err != nil {
		return nil, fmt.Errorf("failed to decode remote partial sig: %w", err)
	}

	finalSig, err := s.CombineSignatures(localPartial, remotePartial)
	if err != nil {
		return nil, err
	}
	return finalSig.Serialize(), nil
}

// GetAggregatedPubKey returns the MuSig2 aggregated public key.
// This is the MuSig2MethodHandler interface method.
func (s *MuSig2Session) GetAggregatedPubKey() *btcec.PublicKey {
	if s.aggregatedKey == nil {
		return nil
	}
	return s.aggregatedKey.FinalKey
}

// Ensure MuSig2Session implements the interfaces
var (
	_ SwapMethodHandler   = (*MuSig2Session)(nil)
	_ MuSig2MethodHandler = (*MuSig2Session)(nil)
)
