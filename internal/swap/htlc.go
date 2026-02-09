// Package swap - HTLC session implementation for atomic swaps.
// This file contains the HTLCSession struct which implements the HTLCMethodHandler interface.
package swap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

// HTLCSession manages the HTLC protocol for one chain in an atomic swap.
// It handles key management, secret handling, and witness construction.
type HTLCSession struct {
	mu sync.RWMutex

	// Chain identity
	symbol  string
	network chain.Network

	// Keys
	localPrivKey *btcec.PrivateKey
	localPubKey  *btcec.PublicKey
	remotePubKey *btcec.PublicKey

	// Secret (initiator has both, responder only has hash until revealed)
	secret     []byte // 32 bytes, nil for responder until revealed
	secretHash []byte // SHA256(secret)

	// Script data (populated after GenerateSwapAddress)
	htlcData *HTLCScriptData

	// Role tracking
	isInitiator bool // true if we generated the secret
}

// HTLCStorageData is the serializable form of HTLCSession for persistence.
type HTLCStorageData struct {
	Symbol       string `json:"symbol"`
	Network      string `json:"network"`
	LocalPrivKey string `json:"local_priv_key"` // Hex-encoded
	LocalPubKey  string `json:"local_pub_key"`
	RemotePubKey string `json:"remote_pub_key,omitempty"`
	Secret       string `json:"secret,omitempty"`      // Hex, only for initiator
	SecretHash   string `json:"secret_hash,omitempty"` // Hex
	HTLCScript   string `json:"htlc_script,omitempty"` // Hex
	HTLCAddress  string `json:"htlc_address,omitempty"`
	IsInitiator  bool   `json:"is_initiator"`
}

// NewHTLCSession creates a new HTLC session with a fresh ephemeral key.
func NewHTLCSession(symbol string, network chain.Network) (*HTLCSession, error) {
	// Validate chain supports HTLC
	chainParams, ok := chain.Get(symbol, network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if chainParams.Type != chain.ChainTypeBitcoin {
		return nil, fmt.Errorf("HTLC only supported for Bitcoin-family chains, got %s", chainParams.Type)
	}

	// Generate ephemeral private key
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	return &HTLCSession{
		symbol:       symbol,
		network:      network,
		localPrivKey: privKey,
		localPubKey:  privKey.PubKey(),
	}, nil
}

// NewHTLCSessionWithKey creates an HTLC session with a pre-existing private key.
// Used when recovering from storage.
func NewHTLCSessionWithKey(symbol string, network chain.Network, privKey *btcec.PrivateKey) (*HTLCSession, error) {
	if privKey == nil {
		return nil, fmt.Errorf("private key cannot be nil")
	}

	return &HTLCSession{
		symbol:       symbol,
		network:      network,
		localPrivKey: privKey,
		localPubKey:  privKey.PubKey(),
	}, nil
}

// =============================================================================
// SwapMethodHandler Implementation
// =============================================================================

// Method returns the swap method type.
func (h *HTLCSession) Method() Method {
	return MethodHTLC
}

// Symbol returns the chain symbol.
func (h *HTLCSession) Symbol() string {
	return h.symbol
}

// Network returns the network (mainnet/testnet).
func (h *HTLCSession) Network() chain.Network {
	return h.network
}

// GetLocalPubKey returns the local public key.
func (h *HTLCSession) GetLocalPubKey() *btcec.PublicKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.localPubKey
}

// GetLocalPubKeyBytes returns the local public key as compressed bytes.
func (h *HTLCSession) GetLocalPubKeyBytes() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.localPubKey == nil {
		return nil
	}
	return h.localPubKey.SerializeCompressed()
}

// SetRemotePubKey sets the counterparty's public key.
func (h *HTLCSession) SetRemotePubKey(pubkey *btcec.PublicKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if pubkey == nil {
		return fmt.Errorf("remote public key cannot be nil")
	}
	h.remotePubKey = pubkey
	return nil
}

// GenerateSwapAddress generates the P2WSH HTLC address for funding.
// For HTLC, the receiver can claim with secret, sender can refund after timeout.
//
// In an atomic swap:
//   - Initiator is sender on their chain (can refund), receiver on counterparty's chain (claims with secret)
//   - Responder is sender on their chain (can refund), receiver on initiator's chain (claims with secret)
//
// Parameters:
//   - timeoutBlocks: CSV relative timelock for refund path
//   - refundPubKey: ignored for HTLC (we use localPubKey as sender, remotePubKey as receiver)
func (h *HTLCSession) GenerateSwapAddress(timeoutBlocks uint32, _ *btcec.PublicKey) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.remotePubKey == nil {
		return "", fmt.Errorf("remote public key not set")
	}
	if len(h.secretHash) == 0 {
		return "", fmt.Errorf("secret hash not set")
	}

	// In HTLC atomic swap:
	// - Local party is the SENDER (can refund after timeout)
	// - Remote party is the RECEIVER (can claim with secret)
	htlcData, err := BuildHTLCScriptData(
		h.secretHash,
		h.remotePubKey, // Receiver (claims with secret)
		h.localPubKey,  // Sender (can refund)
		timeoutBlocks,
		h.symbol,
		h.network,
	)
	if err != nil {
		return "", fmt.Errorf("failed to build HTLC script: %w", err)
	}

	h.htlcData = htlcData
	return htlcData.Address, nil
}

// GenerateSwapAddressWithRoles generates the P2WSH HTLC address with explicit sender/receiver.
// This is needed because in an atomic swap, the roles are different per chain:
//   - Offer chain: Maker=sender, Taker=receiver
//   - Request chain: Taker=sender, Maker=receiver
func (h *HTLCSession) GenerateSwapAddressWithRoles(senderPubKey, receiverPubKey *btcec.PublicKey, timeoutBlocks uint32) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.secretHash) == 0 {
		return "", fmt.Errorf("secret hash not set")
	}

	htlcData, err := BuildHTLCScriptData(
		h.secretHash,
		receiverPubKey, // Receiver (claims with secret)
		senderPubKey,   // Sender (can refund after timeout)
		timeoutBlocks,
		h.symbol,
		h.network,
	)
	if err != nil {
		return "", fmt.Errorf("failed to build HTLC script: %w", err)
	}

	h.htlcData = htlcData
	return htlcData.Address, nil
}

// GetSwapAddress returns the previously generated HTLC address.
func (h *HTLCSession) GetSwapAddress() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.htlcData == nil {
		return ""
	}
	return h.htlcData.Address
}

// MarshalStorageData serializes the session for storage.
func (h *HTLCSession) MarshalStorageData() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data := HTLCStorageData{
		Symbol:      h.symbol,
		Network:     string(h.network),
		IsInitiator: h.isInitiator,
	}

	if h.localPrivKey != nil {
		data.LocalPrivKey = hex.EncodeToString(h.localPrivKey.Serialize())
	}
	if h.localPubKey != nil {
		data.LocalPubKey = hex.EncodeToString(h.localPubKey.SerializeCompressed())
	}
	if h.remotePubKey != nil {
		data.RemotePubKey = hex.EncodeToString(h.remotePubKey.SerializeCompressed())
	}
	if len(h.secret) > 0 {
		data.Secret = hex.EncodeToString(h.secret)
	}
	if len(h.secretHash) > 0 {
		data.SecretHash = hex.EncodeToString(h.secretHash)
	}
	if h.htlcData != nil {
		data.HTLCScript = hex.EncodeToString(h.htlcData.Script)
		data.HTLCAddress = h.htlcData.Address
	}

	return json.Marshal(data)
}

// UnmarshalHTLCStorageData deserializes an HTLCSession from storage.
func UnmarshalHTLCStorageData(data []byte) (*HTLCSession, error) {
	var stored HTLCStorageData
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("failed to unmarshal HTLC storage data: %w", err)
	}

	session := &HTLCSession{
		symbol:      stored.Symbol,
		network:     chain.Network(stored.Network),
		isInitiator: stored.IsInitiator,
	}

	// Restore private key
	if stored.LocalPrivKey != "" {
		privKeyBytes, err := hex.DecodeString(stored.LocalPrivKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode private key: %w", err)
		}
		privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)
		session.localPrivKey = privKey
		session.localPubKey = privKey.PubKey()
	}

	// Restore remote pubkey
	if stored.RemotePubKey != "" {
		remotePubKeyBytes, err := hex.DecodeString(stored.RemotePubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode remote pubkey: %w", err)
		}
		remotePubKey, err := btcec.ParsePubKey(remotePubKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse remote pubkey: %w", err)
		}
		session.remotePubKey = remotePubKey
	}

	// Restore secret/hash
	if stored.Secret != "" {
		secret, err := hex.DecodeString(stored.Secret)
		if err != nil {
			return nil, fmt.Errorf("failed to decode secret: %w", err)
		}
		session.secret = secret
	}
	if stored.SecretHash != "" {
		secretHash, err := hex.DecodeString(stored.SecretHash)
		if err != nil {
			return nil, fmt.Errorf("failed to decode secret hash: %w", err)
		}
		session.secretHash = secretHash
	}

	// Restore HTLC data if available
	if stored.HTLCScript != "" && stored.HTLCAddress != "" {
		script, err := hex.DecodeString(stored.HTLCScript)
		if err != nil {
			return nil, fmt.Errorf("failed to decode HTLC script: %w", err)
		}
		session.htlcData = &HTLCScriptData{
			Script:     script,
			Address:    stored.HTLCAddress,
			SecretHash: session.secretHash,
		}
	}

	return session, nil
}

// =============================================================================
// HTLCMethodHandler Implementation
// =============================================================================

// GenerateSecret generates a new 32-byte secret and its SHA256 hash.
// Only the initiator should call this.
func (h *HTLCSession) GenerateSecret() (secret, hash []byte, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.secret) > 0 {
		return nil, nil, fmt.Errorf("secret already generated")
	}

	secret, hash, err = GenerateSecret()
	if err != nil {
		return nil, nil, err
	}

	h.secret = secret
	h.secretHash = hash
	h.isInitiator = true

	return secret, hash, nil
}

// SetSecretHash sets the secret hash (for responder who receives it from initiator).
func (h *HTLCSession) SetSecretHash(hash []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(hash) != 32 {
		return fmt.Errorf("secret hash must be 32 bytes")
	}
	if len(h.secretHash) > 0 {
		return fmt.Errorf("secret hash already set")
	}

	h.secretHash = make([]byte, 32)
	copy(h.secretHash, hash)
	h.isInitiator = false

	return nil
}

// GetSecretHash returns the secret hash.
func (h *HTLCSession) GetSecretHash() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.secretHash
}

// SetSecret sets the secret (for responder after it's revealed on-chain).
func (h *HTLCSession) SetSecret(secret []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(secret) != 32 {
		return fmt.Errorf("secret must be 32 bytes")
	}

	// Verify it matches the hash
	if !VerifySecret(secret, h.secretHash) {
		return fmt.Errorf("secret does not match hash")
	}

	h.secret = make([]byte, 32)
	copy(h.secret, secret)
	return nil
}

// GetSecret returns the secret (nil if not known).
func (h *HTLCSession) GetSecret() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.secret
}

// HasSecret returns true if the secret is known.
func (h *HTLCSession) HasSecret() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.secret) == 32
}

// BuildClaimWitness builds the witness for claiming with the secret.
func (h *HTLCSession) BuildClaimWitness(secret []byte, sig []byte) ([][]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.htlcData == nil {
		return nil, fmt.Errorf("HTLC script not generated")
	}
	if len(secret) != 32 {
		return nil, fmt.Errorf("secret must be 32 bytes")
	}
	if !VerifySecret(secret, h.secretHash) {
		return nil, fmt.Errorf("secret does not match hash")
	}

	return BuildHTLCClaimWitness(sig, secret, h.htlcData.Script), nil
}

// BuildRefundWitness builds the witness for refunding after timeout.
func (h *HTLCSession) BuildRefundWitness(sig []byte) ([][]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.htlcData == nil {
		return nil, fmt.Errorf("HTLC script not generated")
	}

	return BuildHTLCRefundWitness(sig, h.htlcData.Script), nil
}

// GetHTLCScript returns the HTLC script bytes.
func (h *HTLCSession) GetHTLCScript() []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.htlcData == nil {
		return nil
	}
	return h.htlcData.Script
}

// GetRedeemScript returns the redeem script (same as HTLC script for P2WSH).
func (h *HTLCSession) GetRedeemScript() []byte {
	return h.GetHTLCScript()
}

// SignHash signs a hash using ECDSA with the local private key.
// Used for signing claim or refund transactions.
func (h *HTLCSession) SignHash(hash []byte) ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.localPrivKey == nil {
		return nil, fmt.Errorf("local private key not available")
	}

	sig := btcecdsa.Sign(h.localPrivKey, hash)
	return sig.Serialize(), nil
}

// GetLocalPrivKey returns the local private key (for signing).
func (h *HTLCSession) GetLocalPrivKey() *btcec.PrivateKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.localPrivKey
}

// IsInitiator returns true if this session generated the secret.
func (h *HTLCSession) IsInitiator() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isInitiator
}

// Ensure HTLCSession implements the interfaces
var (
	_ SwapMethodHandler = (*HTLCSession)(nil)
	_ HTLCMethodHandler = (*HTLCSession)(nil)
)
