// Package node - Peer-to-peer message encryption using NaCl box.
package node

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// EncryptedEnvelope wraps an encrypted message for PubSub delivery.
type EncryptedEnvelope struct {
	// RecipientPeerID is the intended recipient (only they can decrypt)
	RecipientPeerID string `json:"recipient"`

	// SenderPeerID identifies the sender for reply routing
	SenderPeerID string `json:"sender"`

	// EphemeralPubKey is the sender's ephemeral X25519 public key (32 bytes, base64)
	EphemeralPubKey []byte `json:"ephemeral_key"`

	// Nonce is the 24-byte nonce used for encryption (base64)
	Nonce []byte `json:"nonce"`

	// Ciphertext is the encrypted message (base64)
	Ciphertext []byte `json:"ciphertext"`

	// MessageID for deduplication and ACK matching
	MessageID string `json:"message_id"`

	// TradeID for routing to correct swap handler
	TradeID string `json:"trade_id"`
}

// MessageEncryptor handles encryption/decryption of P2P messages.
type MessageEncryptor struct {
	// localPrivKey is our Ed25519 private key
	localPrivKey crypto.PrivKey

	// localX25519Priv is our X25519 private key derived from Ed25519
	localX25519Priv [32]byte

	// localPeerID is our peer ID
	localPeerID peer.ID
}

// NewMessageEncryptor creates a new encryptor using the node's identity key.
func NewMessageEncryptor(privKey crypto.PrivKey, peerID peer.ID) (*MessageEncryptor, error) {
	// Convert Ed25519 private key to X25519
	x25519Priv, err := ed25519PrivToX25519(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive X25519 key: %w", err)
	}

	return &MessageEncryptor{
		localPrivKey:    privKey,
		localX25519Priv: x25519Priv,
		localPeerID:     peerID,
	}, nil
}

// Encrypt encrypts a message for a specific peer.
// Uses ephemeral key + recipient's public key for forward secrecy.
func (e *MessageEncryptor) Encrypt(recipientPeerID peer.ID, msg *SwapMessage) (*EncryptedEnvelope, error) {
	// Serialize the message
	plaintext, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get recipient's X25519 public key from their peer ID
	recipientX25519Pub, err := peerIDToX25519Pub(recipientPeerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get recipient public key: %w", err)
	}

	// Generate ephemeral key pair for forward secrecy
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Generate random nonce
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt: box.Seal uses X25519 + XSalsa20-Poly1305
	ciphertext := box.Seal(nil, plaintext, &nonce, &recipientX25519Pub, ephemeralPriv)

	return &EncryptedEnvelope{
		RecipientPeerID: recipientPeerID.String(),
		SenderPeerID:    e.localPeerID.String(),
		EphemeralPubKey: ephemeralPub[:],
		Nonce:           nonce[:],
		Ciphertext:      ciphertext,
		MessageID:       msg.MessageID,
		TradeID:         msg.TradeID,
	}, nil
}

// Decrypt decrypts an encrypted envelope intended for us.
func (e *MessageEncryptor) Decrypt(envelope *EncryptedEnvelope) (*SwapMessage, error) {
	// Verify we're the intended recipient
	if envelope.RecipientPeerID != e.localPeerID.String() {
		return nil, fmt.Errorf("message not intended for us")
	}

	// Validate envelope fields
	if len(envelope.EphemeralPubKey) != 32 {
		return nil, fmt.Errorf("invalid ephemeral public key length")
	}
	if len(envelope.Nonce) != 24 {
		return nil, fmt.Errorf("invalid nonce length")
	}

	// Convert ephemeral public key
	var ephemeralPub [32]byte
	copy(ephemeralPub[:], envelope.EphemeralPubKey)

	// Convert nonce
	var nonce [24]byte
	copy(nonce[:], envelope.Nonce)

	// Decrypt
	plaintext, ok := box.Open(nil, envelope.Ciphertext, &nonce, &ephemeralPub, &e.localX25519Priv)
	if !ok {
		return nil, fmt.Errorf("decryption failed")
	}

	// Unmarshal message
	var msg SwapMessage
	if err := json.Unmarshal(plaintext, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// IsForUs checks if an encrypted envelope is intended for us.
func (e *MessageEncryptor) IsForUs(envelope *EncryptedEnvelope) bool {
	return envelope.RecipientPeerID == e.localPeerID.String()
}

// =============================================================================
// Key conversion utilities
// =============================================================================

// ed25519PrivToX25519 converts an Ed25519 private key to X25519 format.
// This uses the standard conversion: hash the seed with SHA-512, clamp, use as X25519 private key.
func ed25519PrivToX25519(privKey crypto.PrivKey) ([32]byte, error) {
	var x25519Priv [32]byte

	// Get raw Ed25519 private key bytes
	raw, err := privKey.Raw()
	if err != nil {
		return x25519Priv, fmt.Errorf("failed to get raw private key: %w", err)
	}

	// Ed25519 private key is 64 bytes: 32-byte seed + 32-byte public key
	// We need the seed (first 32 bytes)
	if len(raw) < 32 {
		return x25519Priv, fmt.Errorf("invalid private key length: %d", len(raw))
	}

	// Hash the seed with SHA-512 and use first 32 bytes as X25519 private key
	h := sha512.Sum512(raw[:32])

	// Clamp the key (as per X25519 spec)
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	copy(x25519Priv[:], h[:32])
	return x25519Priv, nil
}

// peerIDToX25519Pub extracts and converts a peer's Ed25519 public key to X25519.
func peerIDToX25519Pub(peerID peer.ID) ([32]byte, error) {
	var x25519Pub [32]byte

	// Extract public key from peer ID
	pubKey, err := peerID.ExtractPublicKey()
	if err != nil {
		return x25519Pub, fmt.Errorf("failed to extract public key: %w", err)
	}

	// Get raw Ed25519 public key bytes
	raw, err := pubKey.Raw()
	if err != nil {
		return x25519Pub, fmt.Errorf("failed to get raw public key: %w", err)
	}

	if len(raw) != 32 {
		return x25519Pub, fmt.Errorf("invalid public key length: %d", len(raw))
	}

	// Convert Ed25519 public key to X25519 public key
	// This is done by interpreting the Ed25519 point on the Edwards curve
	// and converting it to the Montgomery curve used by X25519
	edPoint, err := new(edwards25519.Point).SetBytes(raw)
	if err != nil {
		return x25519Pub, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	// Convert Edwards point to Montgomery u-coordinate
	copy(x25519Pub[:], edPoint.BytesMontgomery())

	return x25519Pub, nil
}

// ed25519PubToX25519 converts a raw Ed25519 public key to X25519 format.
func ed25519PubToX25519(ed25519Pub []byte) ([32]byte, error) {
	var x25519Pub [32]byte

	if len(ed25519Pub) != 32 {
		return x25519Pub, fmt.Errorf("invalid public key length: %d", len(ed25519Pub))
	}

	edPoint, err := new(edwards25519.Point).SetBytes(ed25519Pub)
	if err != nil {
		return x25519Pub, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	copy(x25519Pub[:], edPoint.BytesMontgomery())
	return x25519Pub, nil
}

// deriveSharedSecret derives a shared secret using X25519 ECDH.
// Used for verification that encryption would work.
func deriveSharedSecret(privKey [32]byte, pubKey [32]byte) ([]byte, error) {
	shared, err := curve25519.X25519(privKey[:], pubKey[:])
	if err != nil {
		return nil, err
	}
	return shared, nil
}
