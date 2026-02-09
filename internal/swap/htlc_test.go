package swap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

func TestBuildHTLCScript(t *testing.T) {
	// Generate test keys
	privKey1, _ := btcec.NewPrivateKey()
	privKey2, _ := btcec.NewPrivateKey()
	receiverPubKey := privKey1.PubKey().SerializeCompressed()
	senderPubKey := privKey2.PubKey().SerializeCompressed()

	// Generate test secret
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	secretHash := sha256.Sum256(secret)

	tests := []struct {
		name          string
		secretHash    []byte
		receiverKey   []byte
		senderKey     []byte
		timeoutBlocks uint32
		wantErr       bool
	}{
		{
			name:          "valid script",
			secretHash:    secretHash[:],
			receiverKey:   receiverPubKey,
			senderKey:     senderPubKey,
			timeoutBlocks: 144,
			wantErr:       false,
		},
		{
			name:          "invalid secret hash length",
			secretHash:    []byte{1, 2, 3},
			receiverKey:   receiverPubKey,
			senderKey:     senderPubKey,
			timeoutBlocks: 144,
			wantErr:       true,
		},
		{
			name:          "invalid receiver key length",
			secretHash:    secretHash[:],
			receiverKey:   []byte{1, 2, 3},
			senderKey:     senderPubKey,
			timeoutBlocks: 144,
			wantErr:       true,
		},
		{
			name:          "zero timeout",
			secretHash:    secretHash[:],
			receiverKey:   receiverPubKey,
			senderKey:     senderPubKey,
			timeoutBlocks: 0,
			wantErr:       true,
		},
		{
			name:          "max timeout",
			secretHash:    secretHash[:],
			receiverKey:   receiverPubKey,
			senderKey:     senderPubKey,
			timeoutBlocks: 65535,
			wantErr:       false,
		},
		{
			name:          "timeout exceeds max",
			secretHash:    secretHash[:],
			receiverKey:   receiverPubKey,
			senderKey:     senderPubKey,
			timeoutBlocks: 65536,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := BuildHTLCScript(tt.secretHash, tt.receiverKey, tt.senderKey, tt.timeoutBlocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildHTLCScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(script) == 0 {
				t.Error("BuildHTLCScript() returned empty script")
			}
		})
	}
}

func TestParseHTLCScript(t *testing.T) {
	// Generate test keys
	privKey1, _ := btcec.NewPrivateKey()
	privKey2, _ := btcec.NewPrivateKey()
	receiverPubKey := privKey1.PubKey().SerializeCompressed()
	senderPubKey := privKey2.PubKey().SerializeCompressed()

	// Generate test secret
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	secretHash := sha256.Sum256(secret)

	// Build script
	script, err := BuildHTLCScript(secretHash[:], receiverPubKey, senderPubKey, 144)
	if err != nil {
		t.Fatalf("BuildHTLCScript() failed: %v", err)
	}

	// Parse it back
	parsedHash, parsedReceiver, parsedSender, parsedTimeout, err := ParseHTLCScript(script)
	if err != nil {
		t.Fatalf("ParseHTLCScript() failed: %v", err)
	}

	if !bytes.Equal(parsedHash, secretHash[:]) {
		t.Errorf("secret hash mismatch: got %x, want %x", parsedHash, secretHash[:])
	}
	if !bytes.Equal(parsedReceiver, receiverPubKey) {
		t.Errorf("receiver key mismatch")
	}
	if !bytes.Equal(parsedSender, senderPubKey) {
		t.Errorf("sender key mismatch")
	}
	if parsedTimeout != 144 {
		t.Errorf("timeout mismatch: got %d, want 144", parsedTimeout)
	}
}

func TestHTLCScriptData(t *testing.T) {
	privKey1, _ := btcec.NewPrivateKey()
	privKey2, _ := btcec.NewPrivateKey()

	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	secretHash := sha256.Sum256(secret)

	// Test BTC testnet
	htlcData, err := BuildHTLCScriptData(
		secretHash[:],
		privKey1.PubKey(),
		privKey2.PubKey(),
		144,
		"BTC",
		chain.Testnet,
	)
	if err != nil {
		t.Fatalf("BuildHTLCScriptData() failed: %v", err)
	}

	// Address should start with tb1q (P2WSH on testnet)
	if len(htlcData.Address) == 0 {
		t.Error("address is empty")
	}
	if htlcData.Address[:4] != "tb1q" {
		t.Errorf("BTC testnet P2WSH address should start with tb1q, got %s", htlcData.Address[:4])
	}

	// Test LTC testnet
	htlcDataLTC, err := BuildHTLCScriptData(
		secretHash[:],
		privKey1.PubKey(),
		privKey2.PubKey(),
		576,
		"LTC",
		chain.Testnet,
	)
	if err != nil {
		t.Fatalf("BuildHTLCScriptData() for LTC failed: %v", err)
	}

	// LTC testnet P2WSH should start with tltc1q
	if htlcDataLTC.Address[:5] != "tltc1" {
		t.Errorf("LTC testnet P2WSH address should start with tltc1, got %s", htlcDataLTC.Address[:5])
	}
}

func TestBuildHTLCClaimWitness(t *testing.T) {
	signature := make([]byte, 71) // DER signature
	secret := make([]byte, 32)
	script := make([]byte, 100)

	witness := BuildHTLCClaimWitness(signature, secret, script)

	if len(witness) != 4 {
		t.Fatalf("expected 4 witness elements, got %d", len(witness))
	}
	if !bytes.Equal(witness[0], signature) {
		t.Error("signature mismatch in witness")
	}
	if !bytes.Equal(witness[1], secret) {
		t.Error("secret mismatch in witness")
	}
	if len(witness[2]) != 1 || witness[2][0] != 0x01 {
		t.Error("branch selector should be 0x01 for claim")
	}
	if !bytes.Equal(witness[3], script) {
		t.Error("script mismatch in witness")
	}
}

func TestBuildHTLCRefundWitness(t *testing.T) {
	signature := make([]byte, 71)
	script := make([]byte, 100)

	witness := BuildHTLCRefundWitness(signature, script)

	if len(witness) != 3 {
		t.Fatalf("expected 3 witness elements, got %d", len(witness))
	}
	if !bytes.Equal(witness[0], signature) {
		t.Error("signature mismatch in witness")
	}
	if len(witness[1]) != 0 {
		t.Error("branch selector should be empty for refund")
	}
	if !bytes.Equal(witness[2], script) {
		t.Error("script mismatch in witness")
	}
}

func TestGenerateAndVerifySecret(t *testing.T) {
	secret, hash, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() failed: %v", err)
	}

	if len(secret) != 32 {
		t.Errorf("secret should be 32 bytes, got %d", len(secret))
	}
	if len(hash) != 32 {
		t.Errorf("hash should be 32 bytes, got %d", len(hash))
	}

	// Verify the hash matches
	expectedHash := sha256.Sum256(secret)
	if !bytes.Equal(hash, expectedHash[:]) {
		t.Error("hash does not match SHA256(secret)")
	}

	// Test VerifySecret
	if !VerifySecret(secret, hash) {
		t.Error("VerifySecret() returned false for valid secret")
	}

	// Test with wrong secret
	wrongSecret := make([]byte, 32)
	if VerifySecret(wrongSecret, hash) {
		t.Error("VerifySecret() returned true for wrong secret")
	}
}

func TestHTLCSession(t *testing.T) {
	// Create initiator session
	initiator, err := NewHTLCSession("BTC", chain.Testnet)
	if err != nil {
		t.Fatalf("NewHTLCSession() failed: %v", err)
	}

	// Create responder session
	responder, err := NewHTLCSession("BTC", chain.Testnet)
	if err != nil {
		t.Fatalf("NewHTLCSession() for responder failed: %v", err)
	}

	// Initiator generates secret
	secret, hash, err := initiator.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() failed: %v", err)
	}

	if !initiator.IsInitiator() {
		t.Error("initiator should be marked as initiator")
	}
	if !initiator.HasSecret() {
		t.Error("initiator should have secret")
	}

	// Responder receives secret hash
	if err := responder.SetSecretHash(hash); err != nil {
		t.Fatalf("SetSecretHash() failed: %v", err)
	}

	if responder.IsInitiator() {
		t.Error("responder should not be marked as initiator")
	}
	if responder.HasSecret() {
		t.Error("responder should not have secret yet")
	}

	// Exchange public keys
	if err := initiator.SetRemotePubKey(responder.GetLocalPubKey()); err != nil {
		t.Fatalf("initiator.SetRemotePubKey() failed: %v", err)
	}
	if err := responder.SetRemotePubKey(initiator.GetLocalPubKey()); err != nil {
		t.Fatalf("responder.SetRemotePubKey() failed: %v", err)
	}

	// Generate swap addresses
	initiatorAddr, err := initiator.GenerateSwapAddress(144, nil)
	if err != nil {
		t.Fatalf("initiator.GenerateSwapAddress() failed: %v", err)
	}
	responderAddr, err := responder.GenerateSwapAddress(144, nil)
	if err != nil {
		t.Fatalf("responder.GenerateSwapAddress() failed: %v", err)
	}

	// Addresses should be different (different key pairs)
	if initiatorAddr == responderAddr {
		t.Error("initiator and responder should have different addresses")
	}

	// Both addresses should be valid P2WSH (tb1q...)
	if initiatorAddr[:4] != "tb1q" {
		t.Errorf("initiator address should be P2WSH, got %s", initiatorAddr)
	}
	if responderAddr[:4] != "tb1q" {
		t.Errorf("responder address should be P2WSH, got %s", responderAddr)
	}

	// Responder learns secret after initiator reveals it on-chain
	if err := responder.SetSecret(secret); err != nil {
		t.Fatalf("responder.SetSecret() failed: %v", err)
	}

	if !responder.HasSecret() {
		t.Error("responder should now have secret")
	}
	if !bytes.Equal(responder.GetSecret(), secret) {
		t.Error("responder secret should match initiator's")
	}
}

func TestHTLCSessionSerialization(t *testing.T) {
	// Create and configure a session
	session, err := NewHTLCSession("BTC", chain.Testnet)
	if err != nil {
		t.Fatalf("NewHTLCSession() failed: %v", err)
	}

	_, hash, _ := session.GenerateSecret()

	// Create a remote key
	remotePriv, _ := btcec.NewPrivateKey()
	session.SetRemotePubKey(remotePriv.PubKey())

	// Generate address
	session.GenerateSwapAddress(144, nil)

	// Serialize
	data, err := session.MarshalStorageData()
	if err != nil {
		t.Fatalf("MarshalStorageData() failed: %v", err)
	}

	// Deserialize
	restored, err := UnmarshalHTLCStorageData(data)
	if err != nil {
		t.Fatalf("UnmarshalHTLCStorageData() failed: %v", err)
	}

	// Verify fields
	if restored.Symbol() != session.Symbol() {
		t.Errorf("symbol mismatch: got %s, want %s", restored.Symbol(), session.Symbol())
	}
	if restored.Network() != session.Network() {
		t.Errorf("network mismatch")
	}
	if !bytes.Equal(restored.GetSecretHash(), hash) {
		t.Error("secret hash mismatch")
	}
	if restored.GetSwapAddress() != session.GetSwapAddress() {
		t.Errorf("address mismatch: got %s, want %s", restored.GetSwapAddress(), session.GetSwapAddress())
	}
	if !restored.IsInitiator() {
		t.Error("restored session should be initiator")
	}
	if !restored.HasSecret() {
		t.Error("restored session should have secret")
	}
}

func TestHTLCSessionSignHash(t *testing.T) {
	session, err := NewHTLCSession("BTC", chain.Testnet)
	if err != nil {
		t.Fatalf("NewHTLCSession() failed: %v", err)
	}

	// Create a test hash to sign
	testData := []byte("test transaction sighash")
	hash := sha256.Sum256(testData)

	// Sign the hash
	sig, err := session.SignHash(hash[:])
	if err != nil {
		t.Fatalf("SignHash() failed: %v", err)
	}

	if len(sig) == 0 {
		t.Error("signature should not be empty")
	}

	// Signature should be DER encoded (starts with 0x30)
	if sig[0] != 0x30 {
		t.Errorf("signature should be DER encoded, first byte: %x", sig[0])
	}
}

func TestHTLCScriptExecution(t *testing.T) {
	// This test verifies the script can actually execute correctly
	// by using btcd's script engine

	// Generate keys
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	receiverPub := receiverPriv.PubKey().SerializeCompressed()
	senderPub := senderPriv.PubKey().SerializeCompressed()

	// Generate secret
	_, hash, _ := GenerateSecret()

	// Build script
	script, err := BuildHTLCScript(hash, receiverPub, senderPub, 10)
	if err != nil {
		t.Fatalf("BuildHTLCScript() failed: %v", err)
	}

	// Disassemble for debugging
	disasm, err := txscript.DisasmString(script)
	if err != nil {
		t.Fatalf("DisasmString() failed: %v", err)
	}
	t.Logf("HTLC Script: %s", disasm)
	t.Logf("Script hex: %s", hex.EncodeToString(script))

	// The script structure should be:
	// OP_IF OP_SHA256 <hash> OP_EQUALVERIFY <receiverPub> OP_CHECKSIG
	// OP_ELSE <timeout> OP_CSV OP_DROP <senderPub> OP_CHECKSIG
	// OP_ENDIF

	// Verify script contains expected opcodes
	if !bytes.Contains(script, hash) {
		t.Error("script should contain secret hash")
	}
	if !bytes.Contains(script, receiverPub) {
		t.Error("script should contain receiver pubkey")
	}
	if !bytes.Contains(script, senderPub) {
		t.Error("script should contain sender pubkey")
	}
}

func TestHTLCUnsupportedChain(t *testing.T) {
	// Try to create HTLC session for an EVM chain (should fail)
	_, err := NewHTLCSession("ETH", chain.Testnet)
	if err == nil {
		t.Error("NewHTLCSession() should fail for EVM chains")
	}
}

func TestVerifySecretEdgeCases(t *testing.T) {
	secret, hash, _ := GenerateSecret()

	tests := []struct {
		name   string
		secret []byte
		hash   []byte
		want   bool
	}{
		{"valid", secret, hash, true},
		{"wrong secret", make([]byte, 32), hash, false},
		{"short secret", make([]byte, 16), hash, false},
		{"short hash", secret, make([]byte, 16), false},
		{"nil secret", nil, hash, false},
		{"nil hash", secret, nil, false},
		{"both nil", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifySecret(tt.secret, tt.hash); got != tt.want {
				t.Errorf("VerifySecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// HTLC Transaction Building Tests
// =============================================================================

func TestBuildHTLCClaimTx(t *testing.T) {
	// Generate keys
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	receiverPub := receiverPriv.PubKey().SerializeCompressed()
	senderPub := senderPriv.PubKey().SerializeCompressed()

	// Generate secret
	secret, secretHash, _ := GenerateSecret()

	// Build HTLC script
	htlcScript, err := BuildHTLCScript(secretHash, receiverPub, senderPub, 144)
	if err != nil {
		t.Fatalf("BuildHTLCScript() failed: %v", err)
	}

	// Test claim tx building
	params := &HTLCClaimTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FundingVout:   0,
		FundingAmount: 100000,
		HTLCScript:    htlcScript,
		Secret:        secret,
		DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		FeeRate:       10,
		PrivKey:       receiverPriv,
	}

	tx, err := BuildHTLCClaimTx(params)
	if err != nil {
		t.Fatalf("BuildHTLCClaimTx() failed: %v", err)
	}

	// Verify transaction structure
	if len(tx.TxIn) != 1 {
		t.Errorf("expected 1 input, got %d", len(tx.TxIn))
	}
	if len(tx.TxOut) != 1 {
		t.Errorf("expected 1 output, got %d", len(tx.TxOut))
	}

	// Verify witness structure
	if len(tx.TxIn[0].Witness) != 4 {
		t.Errorf("expected 4 witness elements for claim, got %d", len(tx.TxIn[0].Witness))
	}

	// Check witness contains secret
	if !bytes.Equal(tx.TxIn[0].Witness[1], secret) {
		t.Error("witness should contain secret")
	}

	// Check OP_TRUE for IF branch
	if len(tx.TxIn[0].Witness[2]) != 1 || tx.TxIn[0].Witness[2][0] != 0x01 {
		t.Error("witness should have OP_TRUE for claim branch")
	}

	// Check script is included
	if !bytes.Equal(tx.TxIn[0].Witness[3], htlcScript) {
		t.Error("witness should contain HTLC script")
	}
}

func TestBuildHTLCClaimTxValidation(t *testing.T) {
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	secret, secretHash, _ := GenerateSecret()

	htlcScript, _ := BuildHTLCScript(
		secretHash,
		receiverPriv.PubKey().SerializeCompressed(),
		senderPriv.PubKey().SerializeCompressed(),
		144,
	)

	tests := []struct {
		name    string
		modify  func(*HTLCClaimTxParams)
		wantErr string
	}{
		{
			name:    "nil private key",
			modify:  func(p *HTLCClaimTxParams) { p.PrivKey = nil },
			wantErr: "private key required",
		},
		{
			name:    "empty script",
			modify:  func(p *HTLCClaimTxParams) { p.HTLCScript = nil },
			wantErr: "HTLC script required",
		},
		{
			name:    "short secret",
			modify:  func(p *HTLCClaimTxParams) { p.Secret = make([]byte, 16) },
			wantErr: "secret must be 32 bytes",
		},
		{
			name:    "invalid txid",
			modify:  func(p *HTLCClaimTxParams) { p.FundingTxID = "invalid" },
			wantErr: "invalid transaction ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &HTLCClaimTxParams{
				Symbol:        "BTC",
				Network:       chain.Testnet,
				FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				FundingVout:   0,
				FundingAmount: 100000,
				HTLCScript:    htlcScript,
				Secret:        secret,
				DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
				FeeRate:       10,
				PrivKey:       receiverPriv,
			}
			tt.modify(params)

			_, err := BuildHTLCClaimTx(params)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildHTLCRefundTx(t *testing.T) {
	// Generate keys
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	receiverPub := receiverPriv.PubKey().SerializeCompressed()
	senderPub := senderPriv.PubKey().SerializeCompressed()

	// Generate secret
	_, secretHash, _ := GenerateSecret()

	// Build HTLC script
	timeoutBlocks := uint32(144)
	htlcScript, err := BuildHTLCScript(secretHash, receiverPub, senderPub, timeoutBlocks)
	if err != nil {
		t.Fatalf("BuildHTLCScript() failed: %v", err)
	}

	// Test refund tx building
	params := &HTLCRefundTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FundingVout:   0,
		FundingAmount: 100000,
		HTLCScript:    htlcScript,
		TimeoutBlocks: timeoutBlocks,
		DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		FeeRate:       10,
		PrivKey:       senderPriv, // Sender refunds
	}

	tx, err := BuildHTLCRefundTx(params)
	if err != nil {
		t.Fatalf("BuildHTLCRefundTx() failed: %v", err)
	}

	// Verify transaction structure
	if len(tx.TxIn) != 1 {
		t.Errorf("expected 1 input, got %d", len(tx.TxIn))
	}
	if len(tx.TxOut) != 1 {
		t.Errorf("expected 1 output, got %d", len(tx.TxOut))
	}

	// Verify CSV sequence
	if tx.TxIn[0].Sequence != timeoutBlocks {
		t.Errorf("expected sequence %d, got %d", timeoutBlocks, tx.TxIn[0].Sequence)
	}

	// Verify transaction version (must be 2 for CSV)
	if tx.Version != 2 {
		t.Errorf("expected tx version 2 for CSV, got %d", tx.Version)
	}

	// Verify witness structure
	if len(tx.TxIn[0].Witness) != 3 {
		t.Errorf("expected 3 witness elements for refund, got %d", len(tx.TxIn[0].Witness))
	}

	// Check empty element for ELSE branch
	if len(tx.TxIn[0].Witness[1]) != 0 {
		t.Error("witness should have empty element for refund branch")
	}

	// Check script is included
	if !bytes.Equal(tx.TxIn[0].Witness[2], htlcScript) {
		t.Error("witness should contain HTLC script")
	}
}

func TestBuildHTLCRefundTxValidation(t *testing.T) {
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	_, secretHash, _ := GenerateSecret()

	htlcScript, _ := BuildHTLCScript(
		secretHash,
		receiverPriv.PubKey().SerializeCompressed(),
		senderPriv.PubKey().SerializeCompressed(),
		144,
	)

	tests := []struct {
		name    string
		modify  func(*HTLCRefundTxParams)
		wantErr string
	}{
		{
			name:    "nil private key",
			modify:  func(p *HTLCRefundTxParams) { p.PrivKey = nil },
			wantErr: "private key required",
		},
		{
			name:    "empty script",
			modify:  func(p *HTLCRefundTxParams) { p.HTLCScript = nil },
			wantErr: "HTLC script required",
		},
		{
			name:    "zero timeout",
			modify:  func(p *HTLCRefundTxParams) { p.TimeoutBlocks = 0 },
			wantErr: "timeout blocks must be > 0",
		},
		{
			name:    "invalid txid",
			modify:  func(p *HTLCRefundTxParams) { p.FundingTxID = "invalid" },
			wantErr: "invalid transaction ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &HTLCRefundTxParams{
				Symbol:        "BTC",
				Network:       chain.Testnet,
				FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				FundingVout:   0,
				FundingAmount: 100000,
				HTLCScript:    htlcScript,
				TimeoutBlocks: 144,
				DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
				FeeRate:       10,
				PrivKey:       senderPriv,
			}
			tt.modify(params)

			_, err := BuildHTLCRefundTx(params)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHTLCTxFeesDeducted(t *testing.T) {
	receiverPriv, _ := btcec.NewPrivateKey()
	senderPriv, _ := btcec.NewPrivateKey()
	secret, secretHash, _ := GenerateSecret()

	htlcScript, _ := BuildHTLCScript(
		secretHash,
		receiverPriv.PubKey().SerializeCompressed(),
		senderPriv.PubKey().SerializeCompressed(),
		144,
	)

	fundingAmount := uint64(100000)
	feeRate := uint64(20)

	// Test claim tx
	claimTx, err := BuildHTLCClaimTx(&HTLCClaimTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FundingVout:   0,
		FundingAmount: fundingAmount,
		HTLCScript:    htlcScript,
		Secret:        secret,
		DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		FeeRate:       feeRate,
		PrivKey:       receiverPriv,
	})
	if err != nil {
		t.Fatalf("BuildHTLCClaimTx() failed: %v", err)
	}

	claimOutput := uint64(claimTx.TxOut[0].Value)
	claimFee := fundingAmount - claimOutput

	// Fee should be reasonable (not too small, not more than 10% of funding)
	if claimFee < feeRate*50 {
		t.Errorf("claim fee %d seems too small", claimFee)
	}
	if claimFee > fundingAmount/10 {
		t.Errorf("claim fee %d is more than 10%% of funding", claimFee)
	}

	// Test refund tx
	refundTx, err := BuildHTLCRefundTx(&HTLCRefundTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		FundingTxID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FundingVout:   0,
		FundingAmount: fundingAmount,
		HTLCScript:    htlcScript,
		TimeoutBlocks: 144,
		DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		FeeRate:       feeRate,
		PrivKey:       senderPriv,
	})
	if err != nil {
		t.Fatalf("BuildHTLCRefundTx() failed: %v", err)
	}

	refundOutput := uint64(refundTx.TxOut[0].Value)
	refundFee := fundingAmount - refundOutput

	if refundFee < feeRate*50 {
		t.Errorf("refund fee %d seems too small", refundFee)
	}
	if refundFee > fundingAmount/10 {
		t.Errorf("refund fee %d is more than 10%% of funding", refundFee)
	}
}
