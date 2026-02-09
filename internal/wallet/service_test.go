package wallet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Klingon-tech/klingdex/internal/chain"
)

func TestNewService(t *testing.T) {
	tmpDir := t.TempDir()

	svc := NewService(&ServiceConfig{
		DataDir: tmpDir,
		Network: chain.Testnet,
	})

	if svc == nil {
		t.Fatal("NewService() returned nil")
	}

	if svc.Network() != chain.Testnet {
		t.Errorf("Network() = %s, want testnet", svc.Network())
	}
}

func TestNewServiceDefaults(t *testing.T) {
	svc := NewService(nil)

	if svc == nil {
		t.Fatal("NewService(nil) returned nil")
	}

	if svc.Network() != chain.Mainnet {
		t.Errorf("Network() = %s, want mainnet", svc.Network())
	}
}

func TestServiceGenerateMnemonic(t *testing.T) {
	svc := NewService(nil)

	mnemonic, err := svc.GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error = %v", err)
	}

	if !svc.ValidateMnemonic(mnemonic) {
		t.Error("generated mnemonic should be valid")
	}
}

func TestServiceValidateMnemonic(t *testing.T) {
	svc := NewService(nil)

	if !svc.ValidateMnemonic(testMnemonic) {
		t.Error("test mnemonic should be valid")
	}

	if svc.ValidateMnemonic("invalid words") {
		t.Error("invalid mnemonic should not be valid")
	}
}

func TestServiceCreateAndLoadWallet(t *testing.T) {
	tmpDir := t.TempDir()
	password := "TestPassword123!"

	svc := NewService(&ServiceConfig{
		DataDir: tmpDir,
		Network: chain.Testnet,
	})

	// Initially no wallet
	if svc.HasWallet() {
		t.Error("HasWallet() should be false initially")
	}
	if svc.IsUnlocked() {
		t.Error("IsUnlocked() should be false initially")
	}

	// Create wallet
	err := svc.CreateWallet(testMnemonic, "", password)
	if err != nil {
		t.Fatalf("CreateWallet() error = %v", err)
	}

	// Should have wallet and be unlocked
	if !svc.HasWallet() {
		t.Error("HasWallet() should be true after creation")
	}
	if !svc.IsUnlocked() {
		t.Error("IsUnlocked() should be true after creation")
	}

	// Verify wallet file exists
	seedPath := filepath.Join(tmpDir, "wallet.seed")
	if _, err := os.Stat(seedPath); os.IsNotExist(err) {
		t.Error("wallet.seed file should exist")
	}

	// Lock wallet
	svc.Lock()
	if svc.IsUnlocked() {
		t.Error("IsUnlocked() should be false after lock")
	}

	// Load wallet
	err = svc.LoadWallet(password, "")
	if err != nil {
		t.Fatalf("LoadWallet() error = %v", err)
	}

	if !svc.IsUnlocked() {
		t.Error("IsUnlocked() should be true after load")
	}
}

func TestServiceCreateWalletInvalidMnemonic(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	err := svc.CreateWallet("invalid mnemonic", "", "TestPassword123!")
	if err == nil {
		t.Error("CreateWallet() should fail with invalid mnemonic")
	}
}

func TestServiceCreateWalletWeakPassword(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	err := svc.CreateWallet(testMnemonic, "", "weak")
	if err == nil {
		t.Error("CreateWallet() should fail with weak password")
	}
}

func TestServiceLoadWalletWrongPassword(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	// Create wallet
	svc.CreateWallet(testMnemonic, "", "TestPassword123!")
	svc.Lock()

	// Try to load with wrong password
	err := svc.LoadWallet("WrongPassword123!", "")
	if err == nil {
		t.Error("LoadWallet() should fail with wrong password")
	}
}

func TestServiceGetAddress(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{
		DataDir: tmpDir,
		Network: chain.Testnet,
	})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	// Get BTC address
	addr, err := svc.GetAddress("BTC", 0, 0)
	if err != nil {
		t.Fatalf("GetAddress(BTC) error = %v", err)
	}

	// Should be testnet address
	if addr[:4] != "tb1q" {
		t.Errorf("BTC testnet address should start with tb1q, got %s", addr)
	}
}

func TestServiceGetAddressLocked(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")
	svc.Lock()

	_, err := svc.GetAddress("BTC", 0, 0)
	if err == nil {
		t.Error("GetAddress() should fail when wallet is locked")
	}
}

func TestServiceGetAddressWithType(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{
		DataDir: tmpDir,
		Network: chain.Mainnet,
	})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	// Get P2PKH address (legacy)
	addr, err := svc.GetAddressWithType("BTC", 0, 0, chain.AddressP2PKH)
	if err != nil {
		t.Fatalf("GetAddressWithType(P2PKH) error = %v", err)
	}
	if addr[0] != '1' {
		t.Errorf("P2PKH address should start with 1, got %s", addr)
	}

	// Get P2WPKH address (native segwit)
	addr, err = svc.GetAddressWithType("BTC", 0, 0, chain.AddressP2WPKH)
	if err != nil {
		t.Fatalf("GetAddressWithType(P2WPKH) error = %v", err)
	}
	if addr[:4] != "bc1q" {
		t.Errorf("P2WPKH address should start with bc1q, got %s", addr)
	}

	// Get Taproot address
	addr, err = svc.GetAddressWithType("BTC", 0, 0, chain.AddressP2TR)
	if err != nil {
		t.Fatalf("GetAddressWithType(P2TR) error = %v", err)
	}
	if addr[:4] != "bc1p" {
		t.Errorf("P2TR address should start with bc1p, got %s", addr)
	}
}

func TestServiceGetAllAddresses(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{
		DataDir: tmpDir,
		Network: chain.Mainnet,
	})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	addrs, err := svc.GetAllAddresses("BTC", 0, 0)
	if err != nil {
		t.Fatalf("GetAllAddresses() error = %v", err)
	}

	// Should have at least P2PKH and P2WPKH
	if len(addrs) < 2 {
		t.Errorf("expected at least 2 address types, got %d", len(addrs))
	}

	// Check P2WPKH is included
	if _, ok := addrs[chain.AddressP2WPKH]; !ok {
		t.Error("should include P2WPKH address")
	}
}

func TestServiceGetDerivationPath(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	path, err := svc.GetDerivationPath("BTC", 0, 0)
	if err != nil {
		t.Fatalf("GetDerivationPath() error = %v", err)
	}

	if path != "m/84'/0'/0'/0/0" {
		t.Errorf("path = %s, want m/84'/0'/0'/0/0", path)
	}
}

func TestServiceGetPublicKey(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	pubKey, err := svc.GetPublicKey("BTC", 0, 0)
	if err != nil {
		t.Fatalf("GetPublicKey() error = %v", err)
	}

	// Compressed public key should be 33 bytes
	if len(pubKey.SerializeCompressed()) != 33 {
		t.Errorf("compressed public key should be 33 bytes")
	}
}

func TestServiceGetPrivateKey(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	privKey, err := svc.GetPrivateKey("BTC", 0, 0)
	if err != nil {
		t.Fatalf("GetPrivateKey() error = %v", err)
	}

	// Private key should be 32 bytes
	if len(privKey.Serialize()) != 32 {
		t.Errorf("private key should be 32 bytes")
	}
}

func TestServiceSupportedChains(t *testing.T) {
	svc := NewService(nil)

	chains := svc.SupportedChains()
	if len(chains) == 0 {
		t.Error("should have supported chains")
	}

	// Check for expected chains
	hasETH := false
	hasBTC := false
	for _, c := range chains {
		if c == "ETH" {
			hasETH = true
		}
		if c == "BTC" {
			hasBTC = true
		}
	}

	if !hasETH {
		t.Error("should support ETH")
	}
	if !hasBTC {
		t.Error("should support BTC")
	}
}

func TestServiceGetBalanceNoBackend(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	_, err := svc.GetBalance(context.Background(), "BTC", "tb1qtest")
	if err == nil {
		t.Error("GetBalance() should fail without backends configured")
	}
}

func TestServiceGetFeeEstimatesNoBackend(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	_, err := svc.GetFeeEstimates(context.Background(), "BTC")
	if err == nil {
		t.Error("GetFeeEstimates() should fail without backends configured")
	}
}

func TestServiceEVMAddress(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(&ServiceConfig{DataDir: tmpDir})

	svc.CreateWallet(testMnemonic, "", "TestPassword123!")

	// ETH address should work
	addr, err := svc.GetAddress("ETH", 0, 0)
	if err != nil {
		t.Fatalf("GetAddress(ETH) error = %v", err)
	}

	if addr[:2] != "0x" {
		t.Errorf("ETH address should start with 0x, got %s", addr)
	}

	// All EVM chains should return same address type
	addrs, err := svc.GetAllAddresses("ETH", 0, 0)
	if err != nil {
		t.Fatalf("GetAllAddresses(ETH) error = %v", err)
	}

	if len(addrs) != 1 {
		t.Errorf("EVM should have exactly 1 address type, got %d", len(addrs))
	}

	if _, ok := addrs[chain.AddressEVM]; !ok {
		t.Error("EVM addresses should have AddressEVM type")
	}
}
