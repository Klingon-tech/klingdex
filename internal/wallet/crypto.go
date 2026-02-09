// Package wallet provides cryptographic utilities for secure seed storage.
// Only Argon2id + AES-256-GCM is supported (no legacy scrypt).
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2 parameters (OWASP recommended for password hashing)
const (
	argon2Time        = 3         // Number of iterations
	argon2Memory      = 64 * 1024 // 64 MB memory
	argon2Parallelism = 4         // Parallel threads
	argon2KeyLen      = 32        // Output key length for AES-256
	argon2SaltLen     = 32        // Salt length
)

// EncryptedSeed represents an encrypted mnemonic seed for storage.
type EncryptedSeed struct {
	Version     int    `json:"version"`
	Ciphertext  []byte `json:"ciphertext"`
	Salt        []byte `json:"salt"`
	Nonce       []byte `json:"nonce"`
	Time        uint32 `json:"time"`
	Memory      uint32 `json:"memory"`
	Parallelism uint8  `json:"parallelism"`
}

// EncryptMnemonic encrypts a mnemonic using Argon2id + AES-256-GCM.
func EncryptMnemonic(mnemonic, password string) (*EncryptedSeed, error) {
	// Validate inputs
	if err := ValidatePassword(password); err != nil {
		return nil, fmt.Errorf("invalid password: %w", err)
	}
	if !ValidateMnemonic(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// Generate salt
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id (resistant to side-channel and GPU attacks)
	key := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Parallelism,
		argon2KeyLen,
	)
	defer SecureClear(key)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, []byte(mnemonic), nil)

	return &EncryptedSeed{
		Version:     1,
		Ciphertext:  ciphertext,
		Salt:        salt,
		Nonce:       nonce,
		Time:        argon2Time,
		Memory:      argon2Memory,
		Parallelism: argon2Parallelism,
	}, nil
}

// DecryptMnemonic decrypts an encrypted seed.
func DecryptMnemonic(encrypted *EncryptedSeed, password string) (string, error) {
	// Use stored parameters or defaults
	time := encrypted.Time
	if time == 0 {
		time = argon2Time
	}
	memory := encrypted.Memory
	if memory == 0 {
		memory = argon2Memory
	}
	parallelism := encrypted.Parallelism
	if parallelism == 0 {
		parallelism = argon2Parallelism
	}

	// Derive key using Argon2id
	key := argon2.IDKey(
		[]byte(password),
		encrypted.Salt,
		time,
		memory,
		parallelism,
		argon2KeyLen,
	)
	defer SecureClear(key)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, encrypted.Nonce, encrypted.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt (wrong password?): %w", err)
	}
	defer SecureClear(plaintext)

	return string(plaintext), nil
}

// SaveEncryptedSeed saves an encrypted seed to a file.
func SaveEncryptedSeed(encrypted *EncryptedSeed, path string) error {
	if err := ValidateFilePath(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.Marshal(encrypted)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// LoadEncryptedSeed loads an encrypted seed from a file.
func LoadEncryptedSeed(path string) (*EncryptedSeed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var encrypted EncryptedSeed
	if err := json.Unmarshal(data, &encrypted); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &encrypted, nil
}

// SecureClear overwrites a byte slice with zeros.
func SecureClear(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// ConstantTimeCompare compares two byte slices in constant time.
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Password validation constants
const (
	MinPasswordLength = 8
	MaxPasswordLength = 256
)

// ValidatePassword validates password strength.
// Requires at least 8 characters and 3 of 4 character types.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("password must be at most %d characters", MaxPasswordLength)
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	// Require at least 3 of 4 character types
	complexity := 0
	if hasUpper {
		complexity++
	}
	if hasLower {
		complexity++
	}
	if hasNumber {
		complexity++
	}
	if hasSpecial {
		complexity++
	}

	if complexity < 3 {
		return fmt.Errorf("password must contain at least 3 of: uppercase, lowercase, number, special character")
	}

	return nil
}

// ValidateFilePath validates a file path for safety.
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Check for path traversal
	clean := filepath.Clean(path)
	if clean != path && !filepath.IsAbs(path) {
		return fmt.Errorf("suspicious path (potential traversal): %s", path)
	}

	// Ensure it's a valid UTF-8 string
	if !utf8.ValidString(path) {
		return fmt.Errorf("path contains invalid UTF-8")
	}

	return nil
}

// ValidateAccountIndex validates a BIP44 account index.
func ValidateAccountIndex(index uint32) error {
	// BIP44 accounts use hardened derivation, max is 2^31 - 1
	const maxAccount = 1<<31 - 1
	if index > maxAccount {
		return fmt.Errorf("account index %d exceeds maximum %d", index, maxAccount)
	}
	return nil
}

// ValidateAddressIndex validates a BIP44 address index.
func ValidateAddressIndex(index uint32) error {
	// Reasonable limit to prevent abuse
	const maxIndex = 100000
	if index > maxIndex {
		return fmt.Errorf("address index %d exceeds reasonable maximum %d", index, maxIndex)
	}
	return nil
}
