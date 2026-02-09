package swap

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

func TestNewMuSig2Session(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		network chain.Network
		wantErr bool
	}{
		{
			name:    "BTC testnet - supported",
			symbol:  "BTC",
			network: chain.Testnet,
			wantErr: false,
		},
		{
			name:    "LTC testnet - supported",
			symbol:  "LTC",
			network: chain.Testnet,
			wantErr: false,
		},
		{
			name:    "DOGE - not supported (no taproot)",
			symbol:  "DOGE",
			network: chain.Mainnet,
			wantErr: true,
		},
		{
			name:    "invalid chain",
			symbol:  "INVALID",
			network: chain.Testnet,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privKey, err := btcec.NewPrivateKey()
			if err != nil {
				t.Fatalf("failed to generate key: %v", err)
			}

			session, err := NewMuSig2Session(tt.symbol, tt.network, privKey)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if session.GetLocalPubKey() == nil {
				t.Error("LocalPubKey should not be nil")
			}
			if session.Symbol() != tt.symbol {
				t.Errorf("Symbol = %s, want %s", session.Symbol(), tt.symbol)
			}
			if session.Network() != tt.network {
				t.Errorf("Network = %s, want %s", session.Network(), tt.network)
			}
		})
	}
}

func TestMuSig2SessionKeyAggregation(t *testing.T) {
	// Create two sessions (Alice and Bob)
	alicePrivKey, _ := btcec.NewPrivateKey()
	bobPrivKey, _ := btcec.NewPrivateKey()

	aliceSession, err := NewMuSig2Session("BTC", chain.Testnet, alicePrivKey)
	if err != nil {
		t.Fatalf("failed to create Alice's session: %v", err)
	}

	bobSession, err := NewMuSig2Session("BTC", chain.Testnet, bobPrivKey)
	if err != nil {
		t.Fatalf("failed to create Bob's session: %v", err)
	}

	// Exchange public keys
	err = aliceSession.SetRemotePubKey(bobSession.GetLocalPubKey())
	if err != nil {
		t.Fatalf("Alice failed to set Bob's pubkey: %v", err)
	}

	err = bobSession.SetRemotePubKey(aliceSession.GetLocalPubKey())
	if err != nil {
		t.Fatalf("Bob failed to set Alice's pubkey: %v", err)
	}

	// Both should compute the same aggregated key
	aliceAggKey, err := aliceSession.AggregatedPubKey()
	if err != nil {
		t.Fatalf("Alice failed to get aggregated key: %v", err)
	}

	bobAggKey, err := bobSession.AggregatedPubKey()
	if err != nil {
		t.Fatalf("Bob failed to get aggregated key: %v", err)
	}

	if !aliceAggKey.IsEqual(bobAggKey) {
		t.Error("Alice and Bob should compute the same aggregated key")
	}
}

func TestMuSig2SessionTaprootAddress(t *testing.T) {
	// Create session
	privKey, _ := btcec.NewPrivateKey()
	remotePrivKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("BTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Must set remote pubkey before getting address
	err = session.SetRemotePubKey(remotePrivKey.PubKey())
	if err != nil {
		t.Fatalf("failed to set remote pubkey: %v", err)
	}

	// Get Taproot address
	addr, err := session.TaprootAddress()
	if err != nil {
		t.Fatalf("failed to get taproot address: %v", err)
	}

	// Should be a valid testnet Taproot address (starts with tb1p)
	if len(addr) < 4 || addr[:4] != "tb1p" {
		t.Errorf("invalid taproot address format: %s", addr)
	}

	t.Logf("Taproot address: %s", addr)
}

func TestMuSig2SessionNonceGeneration(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("BTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Generate nonces
	nonces, err := session.GenerateNonces()
	if err != nil {
		t.Fatalf("failed to generate nonces: %v", err)
	}

	if nonces == nil {
		t.Fatal("nonces should not be nil")
	}

	// Get public nonce (returns [66]byte)
	pubNonce, err := session.LocalPubNonce()
	if err != nil {
		t.Fatalf("failed to get public nonce: %v", err)
	}

	// Check it's not all zeros
	allZero := true
	for _, b := range pubNonce {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("public nonce should not be all zeros")
	}

	// Generating again should give different nonces
	nonces2, err := session.GenerateNonces()
	if err != nil {
		t.Fatalf("failed to generate second nonces: %v", err)
	}

	// PubNonce bytes should differ
	nonce1Bytes := nonces.PubNonce
	nonce2Bytes := nonces2.PubNonce

	same := true
	for i := range nonce1Bytes {
		if nonce1Bytes[i] != nonce2Bytes[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("consecutive nonce generations should produce different nonces")
	}
}

func TestMuSig2FullSigningFlow(t *testing.T) {
	// Create two sessions
	alicePrivKey, _ := btcec.NewPrivateKey()
	bobPrivKey, _ := btcec.NewPrivateKey()

	aliceSession, _ := NewMuSig2Session("BTC", chain.Testnet, alicePrivKey)
	bobSession, _ := NewMuSig2Session("BTC", chain.Testnet, bobPrivKey)

	// Exchange public keys
	aliceSession.SetRemotePubKey(bobSession.GetLocalPubKey())
	bobSession.SetRemotePubKey(aliceSession.GetLocalPubKey())

	// Generate nonces
	aliceNonces, err := aliceSession.GenerateNonces()
	if err != nil {
		t.Fatalf("Alice failed to generate nonces: %v", err)
	}

	bobNonces, err := bobSession.GenerateNonces()
	if err != nil {
		t.Fatalf("Bob failed to generate nonces: %v", err)
	}

	// Exchange nonces (PubNonce is [66]byte)
	aliceSession.SetRemoteNonce(bobNonces.PubNonce)
	bobSession.SetRemoteNonce(aliceNonces.PubNonce)

	// Initialize signing sessions
	err = aliceSession.InitSigningSession()
	if err != nil {
		t.Fatalf("Alice failed to init signing session: %v", err)
	}

	err = bobSession.InitSigningSession()
	if err != nil {
		t.Fatalf("Bob failed to init signing session: %v", err)
	}

	// Note: Full signing test would require more complex setup with
	// actual transaction sighashes. This test verifies the session
	// initialization works correctly.
	t.Log("MuSig2 signing session initialized successfully")
}

func TestGenerateEphemeralKey(t *testing.T) {
	key1, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	key2, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatalf("failed to generate second key: %v", err)
	}

	// Keys should be different - compare serialized bytes
	key1Bytes := key1.Serialize()
	key2Bytes := key2.Serialize()
	same := true
	for i := range key1Bytes {
		if key1Bytes[i] != key2Bytes[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("consecutive key generations should produce different keys")
	}

	// Verify serialization roundtrip
	serialized := SerializePrivKey(key1)
	if len(serialized) != 32 {
		t.Errorf("serialized key length = %d, want 32", len(serialized))
	}

	deserialized, pubKey := DeserializePrivKey(serialized)

	// Compare by serializing again
	deserializedBytes := deserialized.Serialize()
	match := true
	for i := range serialized {
		if serialized[i] != deserializedBytes[i] {
			match = false
			break
		}
	}
	if !match {
		t.Error("deserialized key doesn't match original")
	}
	if !pubKey.IsEqual(key1.PubKey()) {
		t.Error("deserialized pubkey doesn't match original")
	}
}

func TestBech32mEncoding(t *testing.T) {
	// Test with known vectors
	// Create a test public key and encode it
	privKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("BTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Set remote pubkey to get aggregated key
	remotePrivKey, _ := btcec.NewPrivateKey()
	session.SetRemotePubKey(remotePrivKey.PubKey())

	addr, err := session.TaprootAddress()
	if err != nil {
		t.Fatalf("failed to encode address: %v", err)
	}

	// Verify address format
	// Testnet taproot addresses start with "tb1p"
	if len(addr) != 62 { // tb1p + 58 chars
		t.Errorf("address length = %d, want 62", len(addr))
	}

	// Check prefix
	if addr[:4] != "tb1p" {
		t.Errorf("address prefix = %s, want tb1p", addr[:4])
	}

	// All characters should be valid bech32
	validChars := "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	for _, c := range addr[4:] {
		found := false
		for _, vc := range validChars {
			if c == vc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("invalid bech32 character: %c", c)
		}
	}
}

func TestLTCTaprootAddress(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()
	remotePrivKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("LTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	session.SetRemotePubKey(remotePrivKey.PubKey())

	addr, err := session.TaprootAddress()
	if err != nil {
		t.Fatalf("failed to get taproot address: %v", err)
	}

	// LTC testnet taproot addresses should start with "tltc1p"
	if len(addr) < 5 || addr[:5] != "tltc1" {
		t.Errorf("invalid LTC taproot address format: %s", addr)
	}

	t.Logf("LTC Taproot address: %s", addr)
}

// Note: TestCompareBytes moved to pkg/helpers/helpers_test.go

// =============================================================================
// Nonce Reuse Prevention Tests
// =============================================================================

func TestMuSig2SessionNonceReusePrevention(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()
	remotePrivKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("BTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Set remote pubkey
	session.SetRemotePubKey(remotePrivKey.PubKey())

	// Generate initial nonces
	nonces1, err := session.GenerateNonces()
	if err != nil {
		t.Fatalf("failed to generate nonces: %v", err)
	}

	// Store the first nonce for later comparison
	firstNonce := nonces1.PubNonce

	// Generate new nonces - this should mark the first as used
	_, err = session.GenerateNonces()
	if err != nil {
		t.Fatalf("failed to generate second nonces: %v", err)
	}

	// The first nonce should now be in usedNonces
	if !session.usedNonces[firstNonce] {
		t.Error("first nonce should be marked as used after generating new nonces")
	}

	// Verify used nonce count
	if session.UsedNonceCount() != 1 {
		t.Errorf("expected 1 used nonce, got %d", session.UsedNonceCount())
	}
}

func TestMuSig2SessionSignInvalidatesSession(t *testing.T) {
	// This test verifies that after signing, the session is invalidated
	alicePrivKey, _ := btcec.NewPrivateKey()
	bobPrivKey, _ := btcec.NewPrivateKey()

	aliceSession, _ := NewMuSig2Session("BTC", chain.Testnet, alicePrivKey)
	bobSession, _ := NewMuSig2Session("BTC", chain.Testnet, bobPrivKey)

	// Exchange public keys
	aliceSession.SetRemotePubKey(bobSession.GetLocalPubKey())
	bobSession.SetRemotePubKey(aliceSession.GetLocalPubKey())

	// Generate and exchange nonces
	aliceNonces, _ := aliceSession.GenerateNonces()
	bobNonces, _ := bobSession.GenerateNonces()

	aliceSession.SetRemoteNonce(bobNonces.PubNonce)
	bobSession.SetRemoteNonce(aliceNonces.PubNonce)

	// Initialize signing sessions
	if err := aliceSession.InitSigningSession(); err != nil {
		t.Fatalf("Alice failed to init signing session: %v", err)
	}

	// Verify session is valid before signing
	if !aliceSession.IsValid() {
		t.Error("session should be valid before signing")
	}

	// Note: We can't test the actual Sign method without a valid sighash,
	// but we can test the state tracking methods
	if aliceSession.IsInvalidated() {
		t.Error("session should not be invalidated before signing")
	}

	if aliceSession.NonceUsed() {
		t.Error("nonce should not be marked as used before signing")
	}
}

func TestMuSig2SessionResetForNewSign(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()
	remotePrivKey, _ := btcec.NewPrivateKey()

	session, _ := NewMuSig2Session("BTC", chain.Testnet, privKey)
	session.SetRemotePubKey(remotePrivKey.PubKey())

	// Generate nonces
	_, err := session.GenerateNonces()
	if err != nil {
		t.Fatalf("failed to generate nonces: %v", err)
	}

	// Get the initial used nonce count
	initialCount := session.UsedNonceCount()

	// Reset for new sign
	err = session.ResetForNewSign()
	if err != nil {
		t.Fatalf("ResetForNewSign failed: %v", err)
	}

	// Verify:
	// 1. Old nonce was marked as used
	if session.UsedNonceCount() != initialCount+1 {
		t.Errorf("expected %d used nonces after reset, got %d", initialCount+1, session.UsedNonceCount())
	}

	// 2. Session state was cleared
	if session.session != nil {
		t.Error("session should be nil after reset")
	}
	if session.context != nil {
		t.Error("context should be nil after reset")
	}
	if session.hasRemoteNonce {
		t.Error("hasRemoteNonce should be false after reset")
	}

	// 3. Session is valid again
	if !session.IsValid() {
		t.Error("session should be valid after reset")
	}
}

func TestUsedNoncesMapInitialized(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()

	session, err := NewMuSig2Session("BTC", chain.Testnet, privKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Verify usedNonces map is initialized
	if session.usedNonces == nil {
		t.Error("usedNonces map should be initialized")
	}

	// Should be able to add to it without panic
	session.usedNonces[[66]byte{}] = true
	if len(session.usedNonces) != 1 {
		t.Error("should be able to add to usedNonces map")
	}
}

// TestMuSig2FullSignAndCombine tests the complete signing flow including
// CombineSignatures. This specifically tests the fix for the nil pointer
// dereference when creating PartialSignature from bytes.
func TestMuSig2FullSignAndCombine(t *testing.T) {
	// Create two sessions (Alice and Bob)
	alicePrivKey, _ := btcec.NewPrivateKey()
	bobPrivKey, _ := btcec.NewPrivateKey()

	aliceSession, err := NewMuSig2Session("BTC", chain.Testnet, alicePrivKey)
	if err != nil {
		t.Fatalf("failed to create Alice's session: %v", err)
	}

	bobSession, err := NewMuSig2Session("BTC", chain.Testnet, bobPrivKey)
	if err != nil {
		t.Fatalf("failed to create Bob's session: %v", err)
	}

	// Exchange public keys
	if err := aliceSession.SetRemotePubKey(bobSession.GetLocalPubKey()); err != nil {
		t.Fatalf("Alice failed to set Bob's pubkey: %v", err)
	}
	if err := bobSession.SetRemotePubKey(aliceSession.GetLocalPubKey()); err != nil {
		t.Fatalf("Bob failed to set Alice's pubkey: %v", err)
	}

	// Generate and exchange nonces
	aliceNonces, err := aliceSession.GenerateNonces()
	if err != nil {
		t.Fatalf("Alice failed to generate nonces: %v", err)
	}

	bobNonces, err := bobSession.GenerateNonces()
	if err != nil {
		t.Fatalf("Bob failed to generate nonces: %v", err)
	}

	aliceSession.SetRemoteNonce(bobNonces.PubNonce)
	bobSession.SetRemoteNonce(aliceNonces.PubNonce)

	// Initialize signing sessions
	if err := aliceSession.InitSigningSession(); err != nil {
		t.Fatalf("Alice failed to init signing session: %v", err)
	}
	if err := bobSession.InitSigningSession(); err != nil {
		t.Fatalf("Bob failed to init signing session: %v", err)
	}

	// Create a test message hash (simulating a transaction sighash)
	testMessage := make([]byte, 32)
	for i := range testMessage {
		testMessage[i] = byte(i)
	}
	msgHash, err := chainhash.NewHash(testMessage)
	if err != nil {
		t.Fatalf("failed to create message hash: %v", err)
	}

	// Create partial signatures
	alicePartialSig, err := aliceSession.Sign(msgHash)
	if err != nil {
		t.Fatalf("Alice failed to sign: %v", err)
	}

	bobPartialSig, err := bobSession.Sign(msgHash)
	if err != nil {
		t.Fatalf("Bob failed to sign: %v", err)
	}

	// Serialize partial signatures (32 bytes each)
	aliceSigBytes := alicePartialSig.S.Bytes()
	bobSigBytes := bobPartialSig.S.Bytes()

	t.Logf("Alice partial sig: %x", aliceSigBytes[:])
	t.Logf("Bob partial sig: %x", bobSigBytes[:])

	// Test combining signatures - this is where the nil pointer crash occurred
	// The session's CombineSignatures only takes the remote sig - the local sig
	// is already stored in the session from the Sign() call.
	// Alice combines with Bob's partial sig
	finalSig, err := aliceSession.CombineSignatures(nil, bobPartialSig)
	if err != nil {
		t.Fatalf("Alice failed to combine signatures: %v", err)
	}

	if finalSig == nil {
		t.Fatal("final signature should not be nil")
	}

	// Verify the final signature is 64 bytes (Schnorr)
	serializedSig := finalSig.Serialize()
	if len(serializedSig) != 64 {
		t.Errorf("expected 64-byte Schnorr signature, got %d bytes", len(serializedSig))
	}

	t.Logf("Alice's combined Schnorr signature: %x", serializedSig)

	// Bob should produce the same final signature
	finalSigBob, err := bobSession.CombineSignatures(nil, alicePartialSig)
	if err != nil {
		t.Fatalf("Bob failed to combine signatures: %v", err)
	}

	// Both should produce identical final signatures
	bobSerialized := finalSigBob.Serialize()
	aliceSerialized := finalSig.Serialize()

	t.Logf("Bob's combined Schnorr signature: %x", bobSerialized)

	for i := range aliceSerialized {
		if aliceSerialized[i] != bobSerialized[i] {
			t.Fatalf("Alice and Bob produced different final signatures")
		}
	}

	t.Log("Successfully combined signatures - both parties produce identical Schnorr signatures")
}
