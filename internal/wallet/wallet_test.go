package wallet

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

// Test mnemonic (DO NOT USE FOR REAL FUNDS)
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestGenerateMnemonic(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error = %v", err)
	}

	words := strings.Fields(mnemonic)
	if len(words) != 24 {
		t.Errorf("expected 24 words, got %d", len(words))
	}

	if !ValidateMnemonic(mnemonic) {
		t.Error("generated mnemonic should be valid")
	}
}

func TestValidateMnemonic(t *testing.T) {
	tests := []struct {
		mnemonic string
		valid    bool
	}{
		{testMnemonic, true},
		{"invalid mnemonic words", false},
		{"", false},
		{"abandon", false}, // Too short
	}

	for _, tc := range tests {
		result := ValidateMnemonic(tc.mnemonic)
		if result != tc.valid {
			t.Errorf("ValidateMnemonic(%q) = %v, want %v", tc.mnemonic, result, tc.valid)
		}
	}
}

func TestNewFromMnemonic(t *testing.T) {
	wallet, err := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	if err != nil {
		t.Fatalf("NewFromMnemonic() error = %v", err)
	}

	if wallet.Network() != chain.Mainnet {
		t.Errorf("Network() = %s, want mainnet", wallet.Network())
	}
}

func TestNewFromMnemonicInvalid(t *testing.T) {
	_, err := NewFromMnemonic("invalid mnemonic", "", chain.Mainnet)
	if err == nil {
		t.Error("expected error for invalid mnemonic")
	}
}

func TestDeriveKeyForChain(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	// Test BTC derivation
	key, err := wallet.DeriveKeyForChain("BTC", 0, 0)
	if err != nil {
		t.Fatalf("DeriveKeyForChain(BTC) error = %v", err)
	}
	if key == nil {
		t.Error("key should not be nil")
	}

	// Test ETH derivation
	key, err = wallet.DeriveKeyForChain("ETH", 0, 0)
	if err != nil {
		t.Fatalf("DeriveKeyForChain(ETH) error = %v", err)
	}
	if key == nil {
		t.Error("key should not be nil")
	}

	// Test unsupported chain
	_, err = wallet.DeriveKeyForChain("INVALID", 0, 0)
	if err == nil {
		t.Error("expected error for unsupported chain")
	}
}

func TestDeriveAddress(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	tests := []struct {
		symbol string
		wantOk bool
	}{
		{"BTC", true},
		{"LTC", true},
		{"ETH", true},
		{"DOGE", true},
		{"INVALID", false},
	}

	for _, tc := range tests {
		addr, err := wallet.DeriveAddress(tc.symbol, 0, 0)
		if tc.wantOk {
			if err != nil {
				t.Errorf("DeriveAddress(%s) error = %v", tc.symbol, err)
			}
			if addr == "" {
				t.Errorf("DeriveAddress(%s) returned empty address", tc.symbol)
			}
		} else {
			if err == nil {
				t.Errorf("DeriveAddress(%s) should return error", tc.symbol)
			}
		}
	}
}

func TestDeriveAddressBTC(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	addr, err := wallet.DeriveAddress("BTC", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(BTC) error = %v", err)
	}

	// BTC mainnet native SegWit addresses start with bc1q
	if !strings.HasPrefix(addr, "bc1q") {
		t.Errorf("BTC address should start with bc1q, got %s", addr)
	}
}

func TestDeriveAddressLTC(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	addr, err := wallet.DeriveAddress("LTC", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(LTC) error = %v", err)
	}

	// LTC mainnet native SegWit addresses start with ltc1q
	if !strings.HasPrefix(addr, "ltc1q") {
		t.Errorf("LTC address should start with ltc1q, got %s", addr)
	}
}

func TestDeriveAddressDOGE(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	addr, err := wallet.DeriveAddress("DOGE", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(DOGE) error = %v", err)
	}

	// DOGE mainnet addresses start with D
	if !strings.HasPrefix(addr, "D") {
		t.Errorf("DOGE address should start with D, got %s", addr)
	}
}

func TestDeriveAddressETH(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	addr, err := wallet.DeriveAddress("ETH", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(ETH) error = %v", err)
	}

	// ETH addresses start with 0x
	if !strings.HasPrefix(addr, "0x") {
		t.Errorf("ETH address should start with 0x, got %s", addr)
	}

	// ETH addresses are 42 characters (0x + 40 hex)
	if len(addr) != 42 {
		t.Errorf("ETH address should be 42 chars, got %d", len(addr))
	}
}

func TestDeriveAddressTestnet(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Testnet)

	// BTC testnet
	btcAddr, err := wallet.DeriveAddress("BTC", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(BTC testnet) error = %v", err)
	}
	if !strings.HasPrefix(btcAddr, "tb1q") {
		t.Errorf("BTC testnet address should start with tb1q, got %s", btcAddr)
	}

	// LTC testnet
	ltcAddr, err := wallet.DeriveAddress("LTC", 0, 0)
	if err != nil {
		t.Fatalf("DeriveAddress(LTC testnet) error = %v", err)
	}
	if !strings.HasPrefix(ltcAddr, "tltc1q") {
		t.Errorf("LTC testnet address should start with tltc1q, got %s", ltcAddr)
	}
}

func TestDerivePrivateKey(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	privKey, err := wallet.DerivePrivateKey("BTC", 0, 0)
	if err != nil {
		t.Fatalf("DerivePrivateKey() error = %v", err)
	}

	if privKey == nil {
		t.Error("private key should not be nil")
	}

	// Verify key is 32 bytes
	keyBytes := privKey.Serialize()
	if len(keyBytes) != 32 {
		t.Errorf("private key should be 32 bytes, got %d", len(keyBytes))
	}
}

func TestDerivePublicKey(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	pubKey, err := wallet.DerivePublicKey("BTC", 0, 0)
	if err != nil {
		t.Fatalf("DerivePublicKey() error = %v", err)
	}

	if pubKey == nil {
		t.Error("public key should not be nil")
	}

	// Compressed public key is 33 bytes
	keyBytes := pubKey.SerializeCompressed()
	if len(keyBytes) != 33 {
		t.Errorf("compressed public key should be 33 bytes, got %d", len(keyBytes))
	}
}

func TestGetDerivationPath(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	tests := []struct {
		symbol   string
		account  uint32
		index    uint32
		expected string
	}{
		{"BTC", 0, 0, "m/84'/0'/0'/0/0"},
		{"BTC", 0, 5, "m/84'/0'/0'/0/5"},
		{"BTC", 1, 0, "m/84'/0'/1'/0/0"},
		{"ETH", 0, 0, "m/44'/60'/0'/0/0"},
		{"LTC", 0, 0, "m/84'/2'/0'/0/0"},
		{"DOGE", 0, 0, "m/44'/3'/0'/0/0"},
	}

	for _, tc := range tests {
		path, err := wallet.GetDerivationPath(tc.symbol, tc.account, tc.index)
		if err != nil {
			t.Errorf("GetDerivationPath(%s) error = %v", tc.symbol, err)
			continue
		}
		if path != tc.expected {
			t.Errorf("GetDerivationPath(%s, %d, %d) = %s, want %s",
				tc.symbol, tc.account, tc.index, path, tc.expected)
		}
	}
}

func TestWalletCache(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	// Derive same key twice
	key1, _ := wallet.DeriveKeyForChain("BTC", 0, 0)
	key2, _ := wallet.DeriveKeyForChain("BTC", 0, 0)

	// Should be same key (cached)
	if key1 != key2 {
		t.Error("cache should return same key instance")
	}

	// Clear cache
	wallet.ClearCache()

	// Derive again
	key3, _ := wallet.DeriveKeyForChain("BTC", 0, 0)

	// Should be different instance but same value
	pub1, _ := key1.ECPubKey()
	pub3, _ := key3.ECPubKey()
	if hex.EncodeToString(pub1.SerializeCompressed()) != hex.EncodeToString(pub3.SerializeCompressed()) {
		t.Error("keys should have same value after cache clear")
	}
}

func TestDeterministicDerivation(t *testing.T) {
	// Same mnemonic should produce same addresses
	wallet1, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	wallet2, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)

	addr1, _ := wallet1.DeriveAddress("BTC", 0, 0)
	addr2, _ := wallet2.DeriveAddress("BTC", 0, 0)

	if addr1 != addr2 {
		t.Error("same mnemonic should produce same addresses")
	}

	// Different passphrase should produce different addresses
	wallet3, _ := NewFromMnemonic(testMnemonic, "passphrase", chain.Mainnet)
	addr3, _ := wallet3.DeriveAddress("BTC", 0, 0)

	if addr1 == addr3 {
		t.Error("different passphrase should produce different addresses")
	}
}

// ============ Crypto Tests ============

func TestEncryptDecryptMnemonic(t *testing.T) {
	password := "TestPassword123!"

	encrypted, err := EncryptMnemonic(testMnemonic, password)
	if err != nil {
		t.Fatalf("EncryptMnemonic() error = %v", err)
	}

	if encrypted.Version != 1 {
		t.Errorf("version = %d, want 1", encrypted.Version)
	}

	decrypted, err := DecryptMnemonic(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptMnemonic() error = %v", err)
	}

	if decrypted != testMnemonic {
		t.Error("decrypted mnemonic doesn't match original")
	}
}

func TestEncryptMnemonicWeakPassword(t *testing.T) {
	_, err := EncryptMnemonic(testMnemonic, "weak")
	if err == nil {
		t.Error("should reject weak password")
	}
}

func TestDecryptMnemonicWrongPassword(t *testing.T) {
	encrypted, _ := EncryptMnemonic(testMnemonic, "TestPassword123!")

	_, err := DecryptMnemonic(encrypted, "WrongPassword123!")
	if err == nil {
		t.Error("should fail with wrong password")
	}
}

func TestSaveLoadEncryptedSeed(t *testing.T) {
	password := "TestPassword123!"
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "wallet.json")

	encrypted, _ := EncryptMnemonic(testMnemonic, password)

	err := SaveEncryptedSeed(encrypted, path)
	if err != nil {
		t.Fatalf("SaveEncryptedSeed() error = %v", err)
	}

	loaded, err := LoadEncryptedSeed(path)
	if err != nil {
		t.Fatalf("LoadEncryptedSeed() error = %v", err)
	}

	decrypted, err := DecryptMnemonic(loaded, password)
	if err != nil {
		t.Fatalf("DecryptMnemonic() error = %v", err)
	}

	if decrypted != testMnemonic {
		t.Error("loaded and decrypted mnemonic doesn't match")
	}
}

func TestSaveEncryptedSeedPermissions(t *testing.T) {
	password := "TestPassword123!"
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "wallet.json")

	encrypted, _ := EncryptMnemonic(testMnemonic, password)
	SaveEncryptedSeed(encrypted, path)

	info, _ := os.Stat(path)
	perm := info.Mode().Perm()

	// Should be 0600 (owner read/write only)
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		password string
		valid    bool
	}{
		{"TestPassword123!", true},  // Has all 4 types
		{"TestPassword123", true},   // Has 3 of 4 (upper, lower, number)
		{"TestPassword!", true},     // Has 3 of 4 (upper, lower, special)
		{"Test123!", true},          // Has all 4 types
		{"Testpass1", true},         // Has 3 of 4 (upper, lower, number)
		{"short", false},            // Too short
		{"testpassword", false},     // Only lowercase
		{"12345678", false},         // Only numbers
		{"TESTPASSWORD", false},     // Only uppercase
		{"testpassword123", false},  // Only 2 types (lower + number)
		{"TESTPASSWORD123", false},  // Only 2 types (upper + number)
		{strings.Repeat("a", 257), false}, // Too long
	}

	for _, tc := range tests {
		err := ValidatePassword(tc.password)
		if tc.valid && err != nil {
			t.Errorf("ValidatePassword(%q) should be valid, got error: %v", tc.password, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidatePassword(%q) should be invalid", tc.password)
		}
	}
}

func TestSecureClear(t *testing.T) {
	data := []byte("sensitive data")
	SecureClear(data)

	for _, b := range data {
		if b != 0 {
			t.Error("data should be cleared to zeros")
			break
		}
	}
}

func TestConstantTimeCompare(t *testing.T) {
	a := []byte("test")
	b := []byte("test")
	c := []byte("diff")

	if !ConstantTimeCompare(a, b) {
		t.Error("equal slices should compare true")
	}
	if ConstantTimeCompare(a, c) {
		t.Error("different slices should compare false")
	}
}

// ============ EVM Tests ============

func TestPublicKeyToEVMAddress(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	pubKey, _ := wallet.DerivePublicKey("ETH", 0, 0)

	addr := PublicKeyToEVMAddress(pubKey)

	// Should start with 0x
	if !strings.HasPrefix(addr, "0x") {
		t.Errorf("EVM address should start with 0x, got %s", addr)
	}

	// Should be 42 characters
	if len(addr) != 42 {
		t.Errorf("EVM address should be 42 chars, got %d", len(addr))
	}

	// Should pass validation
	if !ValidateEVMAddress(addr) {
		t.Error("generated address should be valid")
	}
}

func TestValidateEVMAddress(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{
		{"0x742d35Cc6634C0532925a3b844Bc9e7595f1a3d4", true},
		{"742d35Cc6634C0532925a3b844Bc9e7595f1a3d4", true}, // Without 0x
		{"0x742d35Cc6634C0532925a3b844Bc9e7595f1a3", false}, // Too short
		{"0x742d35Cc6634C0532925a3b844Bc9e7595f1a3d4ff", false}, // Too long
		{"0xGGGd35Cc6634C0532925a3b844Bc9e7595f1a3d4", false}, // Invalid hex
		{"", false},
	}

	for _, tc := range tests {
		result := ValidateEVMAddress(tc.address)
		if result != tc.valid {
			t.Errorf("ValidateEVMAddress(%q) = %v, want %v", tc.address, result, tc.valid)
		}
	}
}

func TestChecksumAddress(t *testing.T) {
	// Test EIP-55 checksum
	addr := "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	checksummed := ChecksumAddress(strings.TrimPrefix(addr, "0x"))

	// The checksummed version should have mixed case
	if checksummed == strings.ToLower(checksummed) {
		t.Error("checksummed address should have mixed case")
	}

	// Should be valid checksum
	if !IsChecksumValid(checksummed) {
		t.Error("checksummed address should pass validation")
	}
}

func TestIsChecksumValid(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{
		// All lowercase is valid (no checksum)
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", true},
		// All uppercase is valid (no checksum)
		{"0x5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED", true},
		// Invalid mixed case
		{"0x5aAeb6053f3e94c9b9a09f33669435e7Ef1beaed", false},
	}

	for _, tc := range tests {
		result := IsChecksumValid(tc.address)
		if result != tc.valid {
			t.Errorf("IsChecksumValid(%q) = %v, want %v", tc.address, result, tc.valid)
		}
	}
}

func TestEVMSign(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	privKey, _ := wallet.DerivePrivateKey("ETH", 0, 0)

	hash := Keccak256([]byte("test message"))

	sig, err := EVMSign(privKey, hash)
	if err != nil {
		t.Fatalf("EVMSign() error = %v", err)
	}

	// Signature should be 65 bytes (r || s || v)
	if len(sig) != 65 {
		t.Errorf("signature length = %d, want 65", len(sig))
	}

	// v should be 0 or 1
	v := sig[64]
	if v != 0 && v != 1 {
		t.Errorf("v = %d, want 0 or 1", v)
	}
}

func TestPersonalSign(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	privKey, _ := wallet.DerivePrivateKey("ETH", 0, 0)

	sig, err := PersonalSign(privKey, []byte("Hello World"))
	if err != nil {
		t.Fatalf("PersonalSign() error = %v", err)
	}

	if len(sig) != 65 {
		t.Errorf("signature length = %d, want 65", len(sig))
	}
}

func TestFormatWei(t *testing.T) {
	// Test with 1 ETH (10^18 wei)
	oneETH, _ := ParseETH("1")
	result := FormatWei(oneETH)
	if !strings.Contains(result, "ETH") {
		t.Error("FormatWei should contain ETH")
	}
	if !strings.Contains(result, "1.") {
		t.Errorf("FormatWei(1 ETH) should contain '1.', got %s", result)
	}

	// Test with 0.5 ETH
	halfETH, _ := ParseETH("0.5")
	result = FormatWei(halfETH)
	if !strings.Contains(result, "0.5") {
		t.Errorf("FormatWei(0.5 ETH) should contain '0.5', got %s", result)
	}
}

func TestPrivateKeyFromHex(t *testing.T) {
	wallet, _ := NewFromMnemonic(testMnemonic, "", chain.Mainnet)
	privKey, _ := wallet.DerivePrivateKey("ETH", 0, 0)

	// Convert to hex and back
	hexStr := PrivateKeyHex(privKey)
	restored, err := PrivateKeyFromHex(hexStr)
	if err != nil {
		t.Fatalf("PrivateKeyFromHex() error = %v", err)
	}

	// Should produce same address
	addr1 := PrivateKeyToEVMAddress(privKey)
	addr2 := PrivateKeyToEVMAddress(restored)

	if addr1 != addr2 {
		t.Error("restored key should produce same address")
	}
}
