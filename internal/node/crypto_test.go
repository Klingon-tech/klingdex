package node

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestMessageEncryptorRoundTrip(t *testing.T) {
	// Generate two Ed25519 key pairs (simulating two peers)
	senderPriv, senderPub, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("Failed to generate sender key: %v", err)
	}

	recipientPriv, recipientPub, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("Failed to generate recipient key: %v", err)
	}

	// Create peer IDs
	senderPeerID, err := peer.IDFromPublicKey(senderPub)
	if err != nil {
		t.Fatalf("Failed to create sender peer ID: %v", err)
	}

	recipientPeerID, err := peer.IDFromPublicKey(recipientPub)
	if err != nil {
		t.Fatalf("Failed to create recipient peer ID: %v", err)
	}

	// Create encryptors
	senderEncryptor, err := NewMessageEncryptor(senderPriv, senderPeerID)
	if err != nil {
		t.Fatalf("Failed to create sender encryptor: %v", err)
	}

	recipientEncryptor, err := NewMessageEncryptor(recipientPriv, recipientPeerID)
	if err != nil {
		t.Fatalf("Failed to create recipient encryptor: %v", err)
	}

	// Create test message
	originalMsg := &SwapMessage{
		Type:      SwapMsgPubKeyExchange,
		TradeID:   "test-trade-123",
		MessageID: "msg-456",
		FromPeer:  senderPeerID.String(),
		Payload:   []byte(`{"pubkey":"abc123"}`),
	}

	// Encrypt message for recipient
	envelope, err := senderEncryptor.Encrypt(recipientPeerID, originalMsg)
	if err != nil {
		t.Fatalf("Failed to encrypt message: %v", err)
	}

	// Verify envelope fields
	if envelope.RecipientPeerID != recipientPeerID.String() {
		t.Errorf("Wrong recipient: got %s, want %s", envelope.RecipientPeerID, recipientPeerID.String())
	}
	if envelope.SenderPeerID != senderPeerID.String() {
		t.Errorf("Wrong sender: got %s, want %s", envelope.SenderPeerID, senderPeerID.String())
	}
	if len(envelope.EphemeralPubKey) != 32 {
		t.Errorf("Invalid ephemeral key length: %d", len(envelope.EphemeralPubKey))
	}
	if len(envelope.Nonce) != 24 {
		t.Errorf("Invalid nonce length: %d", len(envelope.Nonce))
	}

	// Verify IsForUs
	if !recipientEncryptor.IsForUs(envelope) {
		t.Error("IsForUs returned false for recipient")
	}
	if senderEncryptor.IsForUs(envelope) {
		t.Error("IsForUs returned true for sender (should be false)")
	}

	// Decrypt message
	decryptedMsg, err := recipientEncryptor.Decrypt(envelope)
	if err != nil {
		t.Fatalf("Failed to decrypt message: %v", err)
	}

	// Verify decrypted message matches original
	if decryptedMsg.Type != originalMsg.Type {
		t.Errorf("Type mismatch: got %s, want %s", decryptedMsg.Type, originalMsg.Type)
	}
	if decryptedMsg.TradeID != originalMsg.TradeID {
		t.Errorf("TradeID mismatch: got %s, want %s", decryptedMsg.TradeID, originalMsg.TradeID)
	}
	if decryptedMsg.MessageID != originalMsg.MessageID {
		t.Errorf("MessageID mismatch: got %s, want %s", decryptedMsg.MessageID, originalMsg.MessageID)
	}
	if string(decryptedMsg.Payload) != string(originalMsg.Payload) {
		t.Errorf("Payload mismatch: got %s, want %s", string(decryptedMsg.Payload), string(originalMsg.Payload))
	}
}

func TestMessageEncryptorWrongRecipient(t *testing.T) {
	// Generate three Ed25519 key pairs
	senderPriv, senderPub, _ := crypto.GenerateEd25519Key(nil)
	recipientPriv, recipientPub, _ := crypto.GenerateEd25519Key(nil)
	wrongPriv, wrongPub, _ := crypto.GenerateEd25519Key(nil)

	senderPeerID, _ := peer.IDFromPublicKey(senderPub)
	recipientPeerID, _ := peer.IDFromPublicKey(recipientPub)
	wrongPeerID, _ := peer.IDFromPublicKey(wrongPub)

	senderEncryptor, _ := NewMessageEncryptor(senderPriv, senderPeerID)
	wrongEncryptor, _ := NewMessageEncryptor(wrongPriv, wrongPeerID)
	_, _ = NewMessageEncryptor(recipientPriv, recipientPeerID)

	// Create and encrypt message
	msg := &SwapMessage{
		Type:    SwapMsgNonceExchange,
		TradeID: "test-trade",
	}

	envelope, err := senderEncryptor.Encrypt(recipientPeerID, msg)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Wrong recipient should not be able to decrypt
	if wrongEncryptor.IsForUs(envelope) {
		t.Error("IsForUs should return false for wrong recipient")
	}

	_, err = wrongEncryptor.Decrypt(envelope)
	if err == nil {
		t.Error("Decrypt should fail for wrong recipient")
	}
}

func TestMessageEncryptorMultipleMessages(t *testing.T) {
	// Generate keys
	senderPriv, senderPub, _ := crypto.GenerateEd25519Key(nil)
	recipientPriv, recipientPub, _ := crypto.GenerateEd25519Key(nil)

	senderPeerID, _ := peer.IDFromPublicKey(senderPub)
	recipientPeerID, _ := peer.IDFromPublicKey(recipientPub)

	senderEncryptor, _ := NewMessageEncryptor(senderPriv, senderPeerID)
	recipientEncryptor, _ := NewMessageEncryptor(recipientPriv, recipientPeerID)

	// Encrypt multiple messages
	for i := 0; i < 10; i++ {
		msg := &SwapMessage{
			Type:      SwapMsgPartialSig,
			TradeID:   "test-trade",
			MessageID: "msg-" + string(rune('0'+i)),
		}

		envelope, err := senderEncryptor.Encrypt(recipientPeerID, msg)
		if err != nil {
			t.Fatalf("Failed to encrypt message %d: %v", i, err)
		}

		// Each message should have unique ephemeral key and nonce
		decrypted, err := recipientEncryptor.Decrypt(envelope)
		if err != nil {
			t.Fatalf("Failed to decrypt message %d: %v", i, err)
		}

		if decrypted.MessageID != msg.MessageID {
			t.Errorf("Message %d: MessageID mismatch", i)
		}
	}
}

func TestEd25519ToX25519Conversion(t *testing.T) {
	// Generate Ed25519 key pair
	priv, pub, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Convert private key
	x25519Priv, err := ed25519PrivToX25519(priv)
	if err != nil {
		t.Fatalf("Failed to convert private key: %v", err)
	}

	// Verify key is non-zero
	allZero := true
	for _, b := range x25519Priv {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("X25519 private key is all zeros")
	}

	// Get peer ID and convert public key
	peerID, _ := peer.IDFromPublicKey(pub)
	x25519Pub, err := peerIDToX25519Pub(peerID)
	if err != nil {
		t.Fatalf("Failed to convert public key: %v", err)
	}

	// Verify public key is non-zero
	allZero = true
	for _, b := range x25519Pub {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("X25519 public key is all zeros")
	}
}

func TestEncryptedEnvelopeValidation(t *testing.T) {
	priv, pub, _ := crypto.GenerateEd25519Key(nil)
	peerID, _ := peer.IDFromPublicKey(pub)
	encryptor, _ := NewMessageEncryptor(priv, peerID)

	tests := []struct {
		name      string
		envelope  *EncryptedEnvelope
		wantError bool
	}{
		{
			name: "invalid ephemeral key length",
			envelope: &EncryptedEnvelope{
				RecipientPeerID: peerID.String(),
				EphemeralPubKey: []byte{1, 2, 3}, // Wrong length
				Nonce:           make([]byte, 24),
				Ciphertext:      []byte{1, 2, 3},
			},
			wantError: true,
		},
		{
			name: "invalid nonce length",
			envelope: &EncryptedEnvelope{
				RecipientPeerID: peerID.String(),
				EphemeralPubKey: make([]byte, 32),
				Nonce:           []byte{1, 2, 3}, // Wrong length
				Ciphertext:      []byte{1, 2, 3},
			},
			wantError: true,
		},
		{
			name: "wrong recipient",
			envelope: &EncryptedEnvelope{
				RecipientPeerID: "12D3KooWDummyPeerID",
				EphemeralPubKey: make([]byte, 32),
				Nonce:           make([]byte, 24),
				Ciphertext:      []byte{1, 2, 3},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encryptor.Decrypt(tt.envelope)
			if (err != nil) != tt.wantError {
				t.Errorf("Decrypt() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
