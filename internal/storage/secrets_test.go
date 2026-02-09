package storage

import (
	"os"
	"testing"
	"time"
)

func TestSecretCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now()
	secret := &Secret{
		ID:         "secret-123",
		TradeID:    "trade-456",
		SecretHash: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", // 64 hex chars = 32 bytes
		Secret:     "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", // The preimage
		CreatedBy:  SecretCreatorUs,
		CreatedAt:  now,
	}

	// Create
	if err := store.CreateSecret(secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	// Get by ID
	got, err := store.GetSecret(secret.ID)
	if err != nil {
		t.Fatalf("GetSecret() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetSecret() returned nil")
	}

	if got.ID != secret.ID {
		t.Errorf("ID = %s, want %s", got.ID, secret.ID)
	}
	if got.SecretHash != secret.SecretHash {
		t.Errorf("SecretHash = %s, want %s", got.SecretHash, secret.SecretHash)
	}
	if got.Secret != secret.Secret {
		t.Errorf("Secret = %s, want %s", got.Secret, secret.Secret)
	}
	if got.CreatedBy != SecretCreatorUs {
		t.Errorf("CreatedBy = %s, want %s", got.CreatedBy, SecretCreatorUs)
	}

	// Get by hash
	byHash, err := store.GetSecretByHash(secret.SecretHash)
	if err != nil {
		t.Fatalf("GetSecretByHash() error = %v", err)
	}
	if byHash.ID != secret.ID {
		t.Errorf("GetSecretByHash() ID = %s, want %s", byHash.ID, secret.ID)
	}

	// Get by trade ID
	byTrade, err := store.GetSecretByTradeID(secret.TradeID)
	if err != nil {
		t.Fatalf("GetSecretByTradeID() error = %v", err)
	}
	if byTrade.ID != secret.ID {
		t.Errorf("GetSecretByTradeID() ID = %s, want %s", byTrade.ID, secret.ID)
	}

	// Delete
	if err := store.DeleteSecret(secret.ID); err != nil {
		t.Fatalf("DeleteSecret() error = %v", err)
	}

	got, err = store.GetSecret(secret.ID)
	if err != ErrSecretNotFound {
		t.Errorf("GetSecret after delete should return ErrSecretNotFound, got %v", err)
	}
}

func TestSecretNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	_, err = store.GetSecret("nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("GetSecret(nonexistent) should return ErrSecretNotFound, got %v", err)
	}

	_, err = store.GetSecretByHash("nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("GetSecretByHash(nonexistent) should return ErrSecretNotFound, got %v", err)
	}

	err = store.DeleteSecret("nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("DeleteSecret(nonexistent) should return ErrSecretNotFound, got %v", err)
	}
}

func TestSecretWithoutPreimage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create secret without preimage (counterparty's secret)
	secret := &Secret{
		ID:         "secret-nopre",
		TradeID:    "trade-456",
		SecretHash: "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
		Secret:     "", // Empty - we don't know the preimage yet
		CreatedBy:  SecretCreatorThem,
		CreatedAt:  time.Now(),
	}

	if err := store.CreateSecret(secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	got, _ := store.GetSecret(secret.ID)
	if got.Secret != "" {
		t.Errorf("Secret should be empty, got %s", got.Secret)
	}

	// Check if we have preimage
	hasPreimage, err := store.HasSecretPreimage(secret.SecretHash)
	if err != nil {
		t.Fatalf("HasSecretPreimage() error = %v", err)
	}
	if hasPreimage {
		t.Error("HasSecretPreimage() should return false for empty secret")
	}
}

func TestRevealSecret(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create secret without preimage
	secret := &Secret{
		ID:         "secret-reveal",
		TradeID:    "trade-456",
		SecretHash: "aabbccdd0987654321fedcba0987654321fedcba0987654321fedcba09876543",
		Secret:     "",
		CreatedBy:  SecretCreatorThem,
		CreatedAt:  time.Now(),
	}

	if err := store.CreateSecret(secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	// Reveal by ID
	preimage := "1122334455667788990011223344556677889900112233445566778899001122"
	if err := store.RevealSecret(secret.ID, preimage); err != nil {
		t.Fatalf("RevealSecret() error = %v", err)
	}

	got, _ := store.GetSecret(secret.ID)
	if got.Secret != preimage {
		t.Errorf("Secret = %s, want %s", got.Secret, preimage)
	}
	if got.RevealedAt == nil {
		t.Error("RevealedAt should be set after reveal")
	}

	// Check if we have preimage now
	hasPreimage, _ := store.HasSecretPreimage(secret.SecretHash)
	if !hasPreimage {
		t.Error("HasSecretPreimage() should return true after reveal")
	}

	// Reveal again should be idempotent
	if err := store.RevealSecret(secret.ID, "different-preimage"); err != nil {
		t.Fatalf("RevealSecret() second call should not error, got %v", err)
	}

	// Secret should still have original preimage
	got, _ = store.GetSecret(secret.ID)
	if got.Secret != preimage {
		t.Errorf("Secret after second reveal = %s, want original %s", got.Secret, preimage)
	}
}

func TestRevealSecretByHash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	secretHash := "deadbeef0987654321fedcba0987654321fedcba0987654321fedcba09876543"
	secret := &Secret{
		ID:         "secret-reveal-hash",
		TradeID:    "trade-456",
		SecretHash: secretHash,
		Secret:     "",
		CreatedBy:  SecretCreatorThem,
		CreatedAt:  time.Now(),
	}

	if err := store.CreateSecret(secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	// Reveal by hash
	preimage := "cafebabe55667788990011223344556677889900112233445566778899001122"
	if err := store.RevealSecretByHash(secretHash, preimage); err != nil {
		t.Fatalf("RevealSecretByHash() error = %v", err)
	}

	got, _ := store.GetSecretByHash(secretHash)
	if got.Secret != preimage {
		t.Errorf("Secret = %s, want %s", got.Secret, preimage)
	}
}

func TestListSecretsByTrade(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now()
	tradeID := "trade-multi-secrets"

	// Create multiple secrets for one trade (uncommon but possible)
	secrets := []*Secret{
		{
			ID:         "secret-1",
			TradeID:    tradeID,
			SecretHash: "hash1111111111111111111111111111111111111111111111111111111111",
			Secret:     "pre11111111111111111111111111111111111111111111111111111111111",
			CreatedBy:  SecretCreatorUs,
			CreatedAt:  now,
		},
		{
			ID:         "secret-2",
			TradeID:    tradeID,
			SecretHash: "hash2222222222222222222222222222222222222222222222222222222222",
			Secret:     "",
			CreatedBy:  SecretCreatorThem,
			CreatedAt:  now.Add(time.Second),
		},
	}

	for _, s := range secrets {
		if err := store.CreateSecret(s); err != nil {
			t.Fatalf("CreateSecret() error = %v", err)
		}
	}

	list, err := store.ListSecretsByTrade(tradeID)
	if err != nil {
		t.Fatalf("ListSecretsByTrade() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListSecretsByTrade() returned %d secrets, want 2", len(list))
	}
}

func TestGetUnrevealedSecrets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now()

	// Create our unrevealed secret (we have preimage but haven't revealed on-chain)
	ourSecret := &Secret{
		ID:         "secret-ours-unrevealed",
		TradeID:    "trade-1",
		SecretHash: "ourhash11111111111111111111111111111111111111111111111111111111",
		Secret:     "ourpre111111111111111111111111111111111111111111111111111111111",
		CreatedBy:  SecretCreatorUs,
		CreatedAt:  now,
		RevealedAt: nil, // Not revealed yet
	}

	// Create their secret (we don't have preimage)
	theirSecret := &Secret{
		ID:         "secret-theirs",
		TradeID:    "trade-2",
		SecretHash: "theirhash1111111111111111111111111111111111111111111111111111111",
		Secret:     "", // We don't know it
		CreatedBy:  SecretCreatorThem,
		CreatedAt:  now,
	}

	// Create our revealed secret
	revealedAt := now.Add(time.Hour)
	ourRevealedSecret := &Secret{
		ID:         "secret-ours-revealed",
		TradeID:    "trade-3",
		SecretHash: "revhash111111111111111111111111111111111111111111111111111111111",
		Secret:     "revpre1111111111111111111111111111111111111111111111111111111111",
		CreatedBy:  SecretCreatorUs,
		CreatedAt:  now,
		RevealedAt: &revealedAt,
	}

	for _, s := range []*Secret{ourSecret, theirSecret, ourRevealedSecret} {
		if err := store.CreateSecret(s); err != nil {
			t.Fatalf("CreateSecret() error = %v", err)
		}
	}

	unrevealed, err := store.GetUnrevealedSecrets()
	if err != nil {
		t.Fatalf("GetUnrevealedSecrets() error = %v", err)
	}
	if len(unrevealed) != 1 {
		t.Errorf("GetUnrevealedSecrets() returned %d secrets, want 1", len(unrevealed))
	}
	if len(unrevealed) > 0 && unrevealed[0].ID != "secret-ours-unrevealed" {
		t.Errorf("GetUnrevealedSecrets() returned wrong secret: %s", unrevealed[0].ID)
	}
}

func TestDeleteSecretsByTradeID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	tradeID := "trade-delete"
	secret := &Secret{
		ID:         "secret-del",
		TradeID:    tradeID,
		SecretHash: "delhash11111111111111111111111111111111111111111111111111111111",
		CreatedBy:  SecretCreatorUs,
		CreatedAt:  time.Now(),
	}

	if err := store.CreateSecret(secret); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	if err := store.DeleteSecretsByTradeID(tradeID); err != nil {
		t.Fatalf("DeleteSecretsByTradeID() error = %v", err)
	}

	list, _ := store.ListSecretsByTrade(tradeID)
	if len(list) != 0 {
		t.Errorf("ListSecretsByTrade() after delete returned %d secrets, want 0", len(list))
	}
}

func TestHasSecretPreimage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Nonexistent hash
	has, err := store.HasSecretPreimage("nonexistent")
	if err != nil {
		t.Fatalf("HasSecretPreimage() error = %v", err)
	}
	if has {
		t.Error("HasSecretPreimage(nonexistent) should return false")
	}

	// Create secret with preimage
	secretWithPre := &Secret{
		ID:         "secret-with",
		TradeID:    "trade-1",
		SecretHash: "withhash1111111111111111111111111111111111111111111111111111111",
		Secret:     "withpre11111111111111111111111111111111111111111111111111111111",
		CreatedBy:  SecretCreatorUs,
		CreatedAt:  time.Now(),
	}
	if err := store.CreateSecret(secretWithPre); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	has, _ = store.HasSecretPreimage(secretWithPre.SecretHash)
	if !has {
		t.Error("HasSecretPreimage(with preimage) should return true")
	}

	// Create secret without preimage
	secretWithout := &Secret{
		ID:         "secret-without",
		TradeID:    "trade-2",
		SecretHash: "withouthash111111111111111111111111111111111111111111111111111",
		Secret:     "",
		CreatedBy:  SecretCreatorThem,
		CreatedAt:  time.Now(),
	}
	if err := store.CreateSecret(secretWithout); err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	has, _ = store.HasSecretPreimage(secretWithout.SecretHash)
	if has {
		t.Error("HasSecretPreimage(without preimage) should return false")
	}
}

func TestContainsHelper(t *testing.T) {
	if !contains("hello world", "world") {
		t.Error("contains should find 'world' in 'hello world'")
	}
	if contains("hello world", "foo") {
		t.Error("contains should not find 'foo' in 'hello world'")
	}
	if !contains("UNIQUE constraint failed", "UNIQUE constraint") {
		t.Error("contains should find 'UNIQUE constraint' in error")
	}
}
