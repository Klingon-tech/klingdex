// Package wallet provides wallet service for managing wallet lifecycle.
package wallet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
)

// Service manages wallet operations and lifecycle.
type Service struct {
	wallet  *Wallet
	dataDir string
	network chain.Network

	// Backend registry for blockchain queries
	backends *backend.Registry

	mu sync.RWMutex
}

// ServiceConfig holds configuration for the wallet service.
type ServiceConfig struct {
	DataDir  string
	Network  chain.Network
	Backends *backend.Registry
}

// NewService creates a new wallet service.
func NewService(cfg *ServiceConfig) *Service {
	if cfg == nil {
		cfg = &ServiceConfig{
			DataDir: ".",
			Network: chain.Mainnet,
		}
	}

	// Default to mainnet if network not specified
	network := cfg.Network
	if network == "" {
		network = chain.Mainnet
	}

	// Default to current directory if dataDir not specified
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "."
	}

	return &Service{
		dataDir:  dataDir,
		network:  network,
		backends: cfg.Backends,
	}
}

// GenerateMnemonic generates a new 24-word mnemonic.
func (s *Service) GenerateMnemonic() (string, error) {
	return GenerateMnemonic()
}

// ValidateMnemonic checks if a mnemonic is valid.
func (s *Service) ValidateMnemonic(mnemonic string) bool {
	return ValidateMnemonic(mnemonic)
}

// CreateWallet creates a new wallet from a mnemonic and encrypts it.
func (s *Service) CreateWallet(mnemonic, passphrase, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !ValidateMnemonic(mnemonic) {
		return fmt.Errorf("invalid mnemonic")
	}

	if err := ValidatePassword(password); err != nil {
		return fmt.Errorf("weak password: %w", err)
	}

	wallet, err := NewFromMnemonic(mnemonic, passphrase, s.network)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}
	s.wallet = wallet

	// Encrypt with Argon2id
	encrypted, err := EncryptMnemonic(mnemonic, password)
	if err != nil {
		return fmt.Errorf("failed to encrypt seed: %w", err)
	}

	seedPath := filepath.Join(s.dataDir, "wallet.seed")
	if err := SaveEncryptedSeed(encrypted, seedPath); err != nil {
		return fmt.Errorf("failed to save seed: %w", err)
	}

	return nil
}

// LoadWallet loads an existing wallet using the password.
func (s *Service) LoadWallet(password, passphrase string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	seedPath := filepath.Join(s.dataDir, "wallet.seed")

	encrypted, err := LoadEncryptedSeed(seedPath)
	if err != nil {
		return fmt.Errorf("failed to load encrypted seed: %w", err)
	}

	mnemonic, err := DecryptMnemonic(encrypted, password)
	if err != nil {
		return fmt.Errorf("failed to decrypt seed: %w", err)
	}

	wallet, err := NewFromMnemonic(mnemonic, passphrase, s.network)
	if err != nil {
		// Clear mnemonic from memory before returning
		SecureClear([]byte(mnemonic))
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	// Clear mnemonic from memory
	SecureClear([]byte(mnemonic))

	s.wallet = wallet
	return nil
}

// IsUnlocked returns true if the wallet is loaded.
func (s *Service) IsUnlocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wallet != nil
}

// HasWallet returns true if a wallet file exists.
func (s *Service) HasWallet() bool {
	seedPath := filepath.Join(s.dataDir, "wallet.seed")
	_, err := os.Stat(seedPath)
	return err == nil
}

// Lock locks the wallet (clears from memory).
func (s *Service) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wallet != nil {
		s.wallet.ClearCache()
		s.wallet = nil
	}
}

// Network returns the wallet network.
func (s *Service) Network() chain.Network {
	return s.network
}

// GetWallet returns the underlying wallet if unlocked, nil otherwise.
// This is needed for components like the swap coordinator that need direct wallet access.
func (s *Service) GetWallet() *Wallet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wallet
}

// GetAddress returns an address for a chain at the given account and index.
func (s *Service) GetAddress(symbol string, account, index uint32) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return "", fmt.Errorf("wallet not loaded")
	}

	return s.wallet.DeriveAddress(symbol, account, index)
}

// GetAddressWithType returns a specific address type for Bitcoin-family chains.
func (s *Service) GetAddressWithType(symbol string, account, index uint32, addrType chain.AddressType) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return "", fmt.Errorf("wallet not loaded")
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return "", fmt.Errorf("unsupported chain: %s", symbol)
	}

	// EVM chains only have one address type
	if params.Type == chain.ChainTypeEVM {
		return s.wallet.DeriveAddress(symbol, account, index)
	}

	// Bitcoin-family chains
	pubKey, err := s.wallet.DerivePublicKey(symbol, account, index)
	if err != nil {
		return "", err
	}

	chainParams := toChainCfgParams(params)

	switch addrType {
	case chain.AddressP2PKH:
		return deriveP2PKH(pubKey, chainParams)
	case chain.AddressP2WPKH:
		if !params.SupportsSegWit {
			return "", fmt.Errorf("chain %s does not support SegWit", symbol)
		}
		return deriveP2WPKH(pubKey, chainParams)
	case chain.AddressP2SH_P2WPKH:
		if !params.SupportsSegWit {
			return "", fmt.Errorf("chain %s does not support SegWit", symbol)
		}
		return DeriveP2SH_P2WPKH(pubKey, chainParams)
	case chain.AddressP2TR:
		if !params.SupportsTaproot {
			return "", fmt.Errorf("chain %s does not support Taproot", symbol)
		}
		return deriveP2TR(pubKey, chainParams)
	default:
		return s.wallet.DeriveAddress(symbol, account, index)
	}
}

// GetAllAddresses returns all supported address types for a chain.
func (s *Service) GetAllAddresses(symbol string, account, index uint32) (map[chain.AddressType]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}

	// EVM chains only have one address type
	if params.Type == chain.ChainTypeEVM {
		addr, err := s.wallet.DeriveAddress(symbol, account, index)
		if err != nil {
			return nil, err
		}
		return map[chain.AddressType]string{chain.AddressEVM: addr}, nil
	}

	// Bitcoin-family chains
	pubKey, err := s.wallet.DerivePublicKey(symbol, account, index)
	if err != nil {
		return nil, err
	}

	return AllAddressTypes(pubKey, params)
}

// GetDerivationPath returns the derivation path for a chain.
func (s *Service) GetDerivationPath(symbol string, account, index uint32) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return "", fmt.Errorf("wallet not loaded")
	}

	return s.wallet.GetDerivationPath(symbol, account, index)
}

// GetPublicKey returns the public key for a chain at the given account and index.
func (s *Service) GetPublicKey(symbol string, account, index uint32) (*btcec.PublicKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	return s.wallet.DerivePublicKey(symbol, account, index)
}

// GetPrivateKey returns the private key for a chain at the given account and index.
// WARNING: Handle private keys with care!
func (s *Service) GetPrivateKey(symbol string, account, index uint32) (*btcec.PrivateKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	return s.wallet.DerivePrivateKey(symbol, account, index)
}

// GetBalance returns the balance for an address using the configured backend.
func (s *Service) GetBalance(ctx context.Context, symbol, address string) (uint64, error) {
	if s.backends == nil {
		return 0, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return 0, fmt.Errorf("no backend for chain: %s", symbol)
	}

	info, err := b.GetAddressInfo(ctx, address)
	if err != nil {
		return 0, err
	}

	return info.Balance, nil
}

// GetUTXOs returns UTXOs for an address using the configured backend.
func (s *Service) GetUTXOs(ctx context.Context, symbol, address string) ([]backend.UTXO, error) {
	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	return b.GetAddressUTXOs(ctx, address)
}

// BroadcastTx broadcasts a raw transaction.
func (s *Service) BroadcastTx(ctx context.Context, symbol, rawTxHex string) (string, error) {
	if s.backends == nil {
		return "", fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return "", fmt.Errorf("no backend for chain: %s", symbol)
	}

	return b.BroadcastTransaction(ctx, rawTxHex)
}

// GetFeeEstimates returns fee estimates for a chain.
func (s *Service) GetFeeEstimates(ctx context.Context, symbol string) (*backend.FeeEstimate, error) {
	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	return b.GetFeeEstimates(ctx)
}

// SetBackends sets the backend registry.
func (s *Service) SetBackends(backends *backend.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends = backends
}

// SupportedChains returns the list of supported chain symbols.
func (s *Service) SupportedChains() []string {
	return chain.List()
}

// DefaultGapLimit is the standard BIP44 gap limit for address scanning.
const DefaultGapLimit = 20

// AddressBalance holds balance information for a single address.
type AddressBalance struct {
	Address  string `json:"address"`
	Path     string `json:"path"`
	Balance  uint64 `json:"balance"`
	IsChange bool   `json:"is_change"`
	Index    uint32 `json:"index"`
}

// ScanResult holds the result of a wallet balance scan.
type ScanResult struct {
	Symbol          string           `json:"symbol"`
	TotalBalance    uint64           `json:"total_balance"`
	ExternalBalance uint64           `json:"external_balance"`
	ChangeBalance   uint64           `json:"change_balance"`
	Addresses       []AddressBalance `json:"addresses"`
	ScannedExternal uint32           `json:"scanned_external"`
	ScannedChange   uint32           `json:"scanned_change"`
}

// ScanBalance scans all addresses (external and change) for a chain to find total balance.
// Uses a gap limit to stop scanning when no activity is found for consecutive addresses.
func (s *Service) ScanBalance(ctx context.Context, symbol string, account uint32, gapLimit uint32) (*ScanResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}

	if gapLimit == 0 {
		gapLimit = DefaultGapLimit
	}

	result := &ScanResult{
		Symbol:    symbol,
		Addresses: make([]AddressBalance, 0),
	}

	// Scan external addresses (change=0)
	externalAddrs, scannedExt := s.scanAddressChain(ctx, b, params, symbol, account, 0, gapLimit)
	result.ScannedExternal = scannedExt
	for _, addr := range externalAddrs {
		result.ExternalBalance += addr.Balance
		result.Addresses = append(result.Addresses, addr)
	}

	// Scan change addresses (change=1)
	changeAddrs, scannedChg := s.scanAddressChain(ctx, b, params, symbol, account, 1, gapLimit)
	result.ScannedChange = scannedChg
	for _, addr := range changeAddrs {
		result.ChangeBalance += addr.Balance
		result.Addresses = append(result.Addresses, addr)
	}

	result.TotalBalance = result.ExternalBalance + result.ChangeBalance

	return result, nil
}

// scanAddressChain scans addresses for a specific change path (0=external, 1=change).
func (s *Service) scanAddressChain(ctx context.Context, b backend.Backend, params *chain.Params, symbol string, account, change, gapLimit uint32) ([]AddressBalance, uint32) {
	var addresses []AddressBalance
	var index uint32
	emptyCount := uint32(0)

	for emptyCount < gapLimit {
		address, err := s.wallet.DeriveAddressWithChange(symbol, account, change, index)
		if err != nil {
			break
		}

		// Get balance for this address
		info, err := b.GetAddressInfo(ctx, address)
		if err != nil {
			// If we can't get info, assume empty and continue
			emptyCount++
			index++
			continue
		}

		// Build path string
		path := fmt.Sprintf("m/%d'/%d'/%d'/%d/%d", params.DefaultPurpose, params.CoinType, account, change, index)

		if info.Balance > 0 || info.TxCount > 0 {
			// Address has activity - reset gap counter
			emptyCount = 0
			addresses = append(addresses, AddressBalance{
				Address:  address,
				Path:     path,
				Balance:  info.Balance,
				IsChange: change == 1,
				Index:    index,
			})
		} else {
			emptyCount++
		}

		index++
	}

	return addresses, index
}

// GetAddressWithChange returns an address with explicit change path.
func (s *Service) GetAddressWithChange(symbol string, account, change, index uint32) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return "", fmt.Errorf("wallet not loaded")
	}

	return s.wallet.DeriveAddressWithChange(symbol, account, change, index)
}

// DerivePrivateKeyWithChange returns the private key for a specific derivation path.
func (s *Service) DerivePrivateKeyWithChange(symbol string, account, change, index uint32) (*btcec.PrivateKey, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	key, err := s.wallet.DeriveKeyForChainWithChange(symbol, account, change, index)
	if err != nil {
		return nil, err
	}

	return key.ECPrivKey()
}

// SendTransaction builds, signs, and broadcasts a transaction.
// Returns the transaction ID on success.
// This uses change=0 (external addresses). Use SendTransactionFromPath for change addresses.
func (s *Service) SendTransaction(ctx context.Context, symbol string, toAddress string, amount uint64, account, index uint32) (string, error) {
	return s.SendTransactionFromPath(ctx, symbol, toAddress, amount, account, 0, index)
}

// SendTransactionFromPath builds, signs, and broadcasts a transaction from a specific derivation path.
// change=0 for external addresses, change=1 for internal/change addresses.
func (s *Service) SendTransactionFromPath(ctx context.Context, symbol string, toAddress string, amount uint64, account, change, index uint32) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return "", fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return "", fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return "", fmt.Errorf("no backend for chain: %s", symbol)
	}

	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return "", fmt.Errorf("unsupported chain: %s", symbol)
	}

	// Get sender address and private key with explicit change path
	fromAddress, err := s.wallet.DeriveAddressWithChange(symbol, account, change, index)
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}

	privKey, err := s.DerivePrivateKeyWithChange(symbol, account, change, index)
	if err != nil {
		return "", fmt.Errorf("failed to derive private key: %w", err)
	}

	// Get UTXOs
	utxos, err := b.GetAddressUTXOs(ctx, fromAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	if len(utxos) == 0 {
		return "", fmt.Errorf("no UTXOs available for address %s", fromAddress)
	}

	// Get fee estimate
	feeEst, err := b.GetFeeEstimates(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get fee estimates: %w", err)
	}
	feeRate := feeEst.HalfHourFee
	if feeRate == 0 {
		feeRate = 10 // Default to 10 sat/vB
	}

	// Build and sign transaction
	txHex, err := BuildAndSignTx(privKey, utxos, toAddress, fromAddress, amount, feeRate, params)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Broadcast
	txid, err := b.BroadcastTransaction(ctx, txHex)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast: %w", err)
	}

	return txid, nil
}

// =============================================================================
// Multi-Address Sending (aggregates UTXOs from all addresses)
// =============================================================================

// SendFromAllAddresses builds, signs, and broadcasts a transaction using UTXOs
// from all wallet addresses. This enables spending when funds are spread across
// multiple addresses.
func (s *Service) SendFromAllAddresses(ctx context.Context, symbol string, toAddress string, amount uint64, storage *storage.Storage) (*MultiAddressTxResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	// Create UTXO sync service for fresh scan
	syncService := NewUTXOSyncService(&UTXOSyncConfig{
		Wallet:   s.wallet,
		Storage:  storage,
		Backends: s.backends,
		Network:  s.network,
		GapLimit: 20,
	})

	// Get all spendable UTXOs via fresh scan
	utxos, err := syncService.FreshScanUTXOs(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to scan UTXOs: %w", err)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("no spendable UTXOs found")
	}

	// Get fee estimate
	feeEst, err := b.GetFeeEstimates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee estimates: %w", err)
	}
	feeRate := feeEst.HalfHourFee
	if feeRate == 0 {
		feeRate = 10
	}

	// Get next change address
	changeAddr, _, err := syncService.GetNextChangeAddress(symbol)
	if err != nil {
		// Fallback to external address if change derivation fails
		changeAddr, err = s.wallet.DeriveAddress(symbol, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to derive change address: %w", err)
		}
	}

	// Build and sign multi-address transaction
	result, err := BuildAndSignMultiAddressTx(s, &MultiAddressTxParams{
		UTXOs:         utxos,
		ToAddress:     toAddress,
		Amount:        amount,
		ChangeAddress: changeAddr,
		FeeRate:       feeRate,
		Symbol:        symbol,
		Network:       s.network,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Broadcast
	txid, err := b.BroadcastTransaction(ctx, result.TxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	// Update result with actual broadcast txid (should match)
	result.TxID = txid

	return result, nil
}

// SendMaxFromAllAddresses sends the maximum possible amount from all addresses.
func (s *Service) SendMaxFromAllAddresses(ctx context.Context, symbol string, toAddress string, storage *storage.Storage) (*MultiAddressTxResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	// Create UTXO sync service for fresh scan
	syncService := NewUTXOSyncService(&UTXOSyncConfig{
		Wallet:   s.wallet,
		Storage:  storage,
		Backends: s.backends,
		Network:  s.network,
		GapLimit: 20,
	})

	// Get all spendable UTXOs
	utxos, err := syncService.FreshScanUTXOs(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to scan UTXOs: %w", err)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("no spendable UTXOs found")
	}

	// Get fee estimate
	feeEst, err := b.GetFeeEstimates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee estimates: %w", err)
	}
	feeRate := feeEst.HalfHourFee
	if feeRate == 0 {
		feeRate = 10
	}

	// Build max send transaction
	result, err := BuildSendMaxTx(s, &SendMaxParams{
		UTXOs:     utxos,
		ToAddress: toAddress,
		FeeRate:   feeRate,
		Symbol:    symbol,
		Network:   s.network,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Broadcast
	txid, err := b.BroadcastTransaction(ctx, result.TxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	result.TxID = txid

	return result, nil
}

// GetAggregatedBalance returns the total balance across all wallet addresses.
func (s *Service) GetAggregatedBalance(ctx context.Context, symbol string, storage *storage.Storage) (confirmed, unconfirmed uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return 0, 0, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return 0, 0, fmt.Errorf("no backends configured")
	}

	// Create UTXO sync service
	syncService := NewUTXOSyncService(&UTXOSyncConfig{
		Wallet:   s.wallet,
		Storage:  storage,
		Backends: s.backends,
		Network:  s.network,
		GapLimit: 20,
	})

	// Fresh scan all UTXOs
	utxos, err := syncService.FreshScanUTXOs(ctx, symbol)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to scan UTXOs: %w", err)
	}

	// Sum up balances
	for _, u := range utxos {
		confirmed += u.Amount
	}

	return confirmed, 0, nil
}

// ScanAndPersistUTXOs performs a full UTXO scan and persists results to storage.
func (s *Service) ScanAndPersistUTXOs(ctx context.Context, symbol string, storage *storage.Storage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return fmt.Errorf("no backends configured")
	}

	syncService := NewUTXOSyncService(&UTXOSyncConfig{
		Wallet:   s.wallet,
		Storage:  storage,
		Backends: s.backends,
		Network:  s.network,
		GapLimit: 20,
	})

	return syncService.SyncChain(ctx, symbol)
}

// ListAllUTXOs returns all UTXOs across all addresses for a chain.
func (s *Service) ListAllUTXOs(ctx context.Context, symbol string, storage *storage.Storage) ([]*AddressUTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	syncService := NewUTXOSyncService(&UTXOSyncConfig{
		Wallet:   s.wallet,
		Storage:  storage,
		Backends: s.backends,
		Network:  s.network,
		GapLimit: 20,
	})

	return syncService.FreshScanUTXOs(ctx, symbol)
}
