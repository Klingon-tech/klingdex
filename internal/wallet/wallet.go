// Package wallet provides HD wallet functionality with BIP39/BIP44 support.
// Uses the chain package for network parameters and only Argon2id for encryption.
package wallet

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/tyler-smith/go-bip39"
)

// Wallet manages HD keys derived from a BIP39 seed.
// Supports multiple chains with per-chain derivation paths.
type Wallet struct {
	masterKey *hdkeychain.ExtendedKey
	network   chain.Network
	mu        sync.RWMutex

	// Cached derived keys (purpose -> coinType -> account -> change -> index -> key)
	cache map[uint32]map[uint32]map[uint32]map[uint32]map[uint32]*hdkeychain.ExtendedKey
}

// GenerateMnemonic generates a new 24-word BIP39 mnemonic.
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(256) // 256 bits = 24 words
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return mnemonic, nil
}

// ValidateMnemonic checks if a mnemonic is valid.
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// NewFromMnemonic creates a wallet from a BIP39 mnemonic.
// The passphrase is optional (can be empty string).
func NewFromMnemonic(mnemonic, passphrase string, network chain.Network) (*Wallet, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// Generate seed from mnemonic (with optional passphrase)
	seed := bip39.NewSeed(mnemonic, passphrase)

	return NewFromSeed(seed, network)
}

// NewFromSeed creates a wallet from a raw 64-byte seed.
func NewFromSeed(seed []byte, network chain.Network) (*Wallet, error) {
	// Use Bitcoin mainnet params for master key derivation
	// The actual chain params are used later when generating addresses
	params := &chaincfg.MainNetParams
	if network == chain.Testnet {
		params = &chaincfg.TestNet3Params
	}

	masterKey, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %w", err)
	}

	return &Wallet{
		masterKey: masterKey,
		network:   network,
		cache:     make(map[uint32]map[uint32]map[uint32]map[uint32]map[uint32]*hdkeychain.ExtendedKey),
	}, nil
}

// Network returns the wallet's network (mainnet/testnet).
func (w *Wallet) Network() chain.Network {
	return w.network
}

// DeriveKey derives a key at the full BIP44 path: m/purpose'/coin'/account'/change/index
func (w *Wallet) DeriveKey(purpose, coinType, account, change, index uint32) (*hdkeychain.ExtendedKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check cache
	if key := w.getCachedKey(purpose, coinType, account, change, index); key != nil {
		return key, nil
	}

	// m/purpose' (hardened)
	purposeKey, err := w.masterKey.Derive(hdkeychain.HardenedKeyStart + purpose)
	if err != nil {
		return nil, fmt.Errorf("failed to derive purpose: %w", err)
	}

	// m/purpose'/coin' (hardened)
	coinKey, err := purposeKey.Derive(hdkeychain.HardenedKeyStart + coinType)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin: %w", err)
	}

	// m/purpose'/coin'/account' (hardened)
	accountKey, err := coinKey.Derive(hdkeychain.HardenedKeyStart + account)
	if err != nil {
		return nil, fmt.Errorf("failed to derive account: %w", err)
	}

	// m/purpose'/coin'/account'/change (non-hardened)
	changeKey, err := accountKey.Derive(change)
	if err != nil {
		return nil, fmt.Errorf("failed to derive change: %w", err)
	}

	// m/purpose'/coin'/account'/change/index (non-hardened)
	addressKey, err := changeKey.Derive(index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive address: %w", err)
	}

	// Cache the key
	w.setCachedKey(purpose, coinType, account, change, index, addressKey)

	return addressKey, nil
}

// DeriveKeyForChain derives a key for a specific chain using its default derivation path.
// This always uses change=0 (external addresses).
func (w *Wallet) DeriveKeyForChain(symbol string, account, index uint32) (*hdkeychain.ExtendedKey, error) {
	return w.DeriveKeyForChainWithChange(symbol, account, 0, index)
}

// DeriveKeyForChainWithChange derives a key for a specific chain with explicit change path.
// change=0 for external (receiving) addresses, change=1 for internal (change) addresses.
func (w *Wallet) DeriveKeyForChainWithChange(symbol string, account, change, index uint32) (*hdkeychain.ExtendedKey, error) {
	params, ok := chain.Get(symbol, w.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}

	return w.DeriveKey(params.DefaultPurpose, params.CoinType, account, change, index)
}

// DerivePrivateKey derives a private key for a chain at the given account and index.
func (w *Wallet) DerivePrivateKey(symbol string, account, index uint32) (*btcec.PrivateKey, error) {
	key, err := w.DeriveKeyForChain(symbol, account, index)
	if err != nil {
		return nil, err
	}

	privKey, err := key.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	return privKey, nil
}

// DerivePublicKey derives a public key for a chain at the given account and index.
func (w *Wallet) DerivePublicKey(symbol string, account, index uint32) (*btcec.PublicKey, error) {
	key, err := w.DeriveKeyForChain(symbol, account, index)
	if err != nil {
		return nil, err
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	return pubKey, nil
}

// DeriveAddress derives an address for a chain at the given account and index.
// Returns the default address type for that chain. Uses change=0 (external addresses).
func (w *Wallet) DeriveAddress(symbol string, account, index uint32) (string, error) {
	return w.DeriveAddressWithChange(symbol, account, 0, index)
}

// DeriveAddressWithChange derives an address with explicit change path.
// change=0 for external (receiving) addresses, change=1 for internal (change) addresses.
func (w *Wallet) DeriveAddressWithChange(symbol string, account, change, index uint32) (string, error) {
	params, ok := chain.Get(symbol, w.network)
	if !ok {
		return "", fmt.Errorf("unsupported chain: %s", symbol)
	}

	// EVM chains use keccak256 address derivation
	if params.Type == chain.ChainTypeEVM {
		key, err := w.DeriveKeyForChainWithChange(symbol, account, change, index)
		if err != nil {
			return "", err
		}
		pubKey, err := key.ECPubKey()
		if err != nil {
			return "", fmt.Errorf("failed to get public key: %w", err)
		}
		return PublicKeyToEVMAddress(pubKey), nil
	}

	// Solana uses ed25519, which requires different handling
	if params.Type == chain.ChainTypeSolana {
		return "", fmt.Errorf("Solana address derivation not yet implemented")
	}

	// Monero uses different cryptography
	if params.Type == chain.ChainTypeMonero {
		return "", fmt.Errorf("Monero address derivation not yet implemented")
	}

	// Bitcoin-family chains
	key, err := w.DeriveKeyForChainWithChange(symbol, account, change, index)
	if err != nil {
		return "", err
	}

	return DeriveAddressFromKey(key, params)
}

// GetDerivationPath returns the derivation path string for a chain.
func (w *Wallet) GetDerivationPath(symbol string, account, index uint32) (string, error) {
	params, ok := chain.Get(symbol, w.network)
	if !ok {
		return "", fmt.Errorf("unsupported chain: %s", symbol)
	}

	return params.DerivationPathString(account, 0, index), nil
}

// getCachedKey returns a cached key or nil if not found.
func (w *Wallet) getCachedKey(purpose, coinType, account, change, index uint32) *hdkeychain.ExtendedKey {
	if w.cache[purpose] == nil {
		return nil
	}
	if w.cache[purpose][coinType] == nil {
		return nil
	}
	if w.cache[purpose][coinType][account] == nil {
		return nil
	}
	if w.cache[purpose][coinType][account][change] == nil {
		return nil
	}
	return w.cache[purpose][coinType][account][change][index]
}

// setCachedKey stores a key in the cache.
func (w *Wallet) setCachedKey(purpose, coinType, account, change, index uint32, key *hdkeychain.ExtendedKey) {
	if w.cache[purpose] == nil {
		w.cache[purpose] = make(map[uint32]map[uint32]map[uint32]map[uint32]*hdkeychain.ExtendedKey)
	}
	if w.cache[purpose][coinType] == nil {
		w.cache[purpose][coinType] = make(map[uint32]map[uint32]map[uint32]*hdkeychain.ExtendedKey)
	}
	if w.cache[purpose][coinType][account] == nil {
		w.cache[purpose][coinType][account] = make(map[uint32]map[uint32]*hdkeychain.ExtendedKey)
	}
	if w.cache[purpose][coinType][account][change] == nil {
		w.cache[purpose][coinType][account][change] = make(map[uint32]*hdkeychain.ExtendedKey)
	}
	w.cache[purpose][coinType][account][change][index] = key
}

// ClearCache clears the key cache (useful for memory management).
func (w *Wallet) ClearCache() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cache = make(map[uint32]map[uint32]map[uint32]map[uint32]map[uint32]*hdkeychain.ExtendedKey)
}
