// Package wallet - UTXO synchronization service for multi-address wallet.
// Implements gap limit scanning and UTXO persistence.
package wallet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// =============================================================================
// UTXO Sync Service
// =============================================================================

// UTXOSyncService manages UTXO synchronization across all wallet addresses.
type UTXOSyncService struct {
	wallet   *Wallet
	storage  *storage.Storage
	backends *backend.Registry
	network  chain.Network

	// Configuration
	gapLimit uint32

	// Sync state
	syncMu     sync.RWMutex
	syncing    map[string]bool // chain -> is syncing
	lastSync   map[string]time.Time

	// Stop channel for background sync
	stopCh chan struct{}
	wg     sync.WaitGroup

	logger *logging.Logger
}

// UTXOSyncConfig holds configuration for the UTXO sync service.
type UTXOSyncConfig struct {
	Wallet   *Wallet
	Storage  *storage.Storage
	Backends *backend.Registry
	Network  chain.Network
	GapLimit uint32
	Logger   *logging.Logger
}

// NewUTXOSyncService creates a new UTXO sync service.
func NewUTXOSyncService(cfg *UTXOSyncConfig) *UTXOSyncService {
	gapLimit := cfg.GapLimit
	if gapLimit == 0 {
		gapLimit = 20 // Default BIP44 gap limit
	}

	logger := cfg.Logger
	if logger == nil {
		logger = logging.GetDefault().Component("utxo-sync")
	}

	return &UTXOSyncService{
		wallet:   cfg.Wallet,
		storage:  cfg.Storage,
		backends: cfg.Backends,
		network:  cfg.Network,
		gapLimit: gapLimit,
		syncing:  make(map[string]bool),
		lastSync: make(map[string]time.Time),
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
}

// =============================================================================
// Sync Operations
// =============================================================================

// SyncChain synchronizes all addresses and UTXOs for a specific chain.
// Uses gap limit to determine how far to scan.
func (s *UTXOSyncService) SyncChain(ctx context.Context, symbol string) error {
	s.syncMu.Lock()
	if s.syncing[symbol] {
		s.syncMu.Unlock()
		return fmt.Errorf("sync already in progress for %s", symbol)
	}
	s.syncing[symbol] = true
	s.syncMu.Unlock()

	defer func() {
		s.syncMu.Lock()
		s.syncing[symbol] = false
		s.lastSync[symbol] = time.Now()
		s.syncMu.Unlock()
	}()

	s.logger.Info("starting UTXO sync", "chain", symbol)

	// Get backend for this chain
	b, ok := s.backends.Get(symbol)
	if !ok {
		return fmt.Errorf("no backend configured for chain: %s", symbol)
	}

	// Connect if needed
	if err := b.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect backend: %w", err)
	}

	// Get current sync state
	state, err := s.storage.GetWalletSyncState(symbol)
	if err != nil {
		return fmt.Errorf("failed to get sync state: %w", err)
	}

	// Scan external addresses (change=0)
	externalIndex, err := s.scanAddresses(ctx, symbol, b, 0, state.LastExternalIndex)
	if err != nil {
		return fmt.Errorf("failed to scan external addresses: %w", err)
	}

	// Scan change addresses (change=1)
	changeIndex, err := s.scanAddresses(ctx, symbol, b, 1, state.LastChangeIndex)
	if err != nil {
		return fmt.Errorf("failed to scan change addresses: %w", err)
	}

	// Get current block height
	blockHeight, err := b.GetBlockHeight(ctx)
	if err != nil {
		s.logger.Warn("failed to get block height", "error", err)
		blockHeight = 0
	}

	// Update sync state
	state.LastExternalIndex = externalIndex
	state.LastChangeIndex = changeIndex
	state.LastSyncAt = time.Now().Unix()
	state.LastBlockHeight = int64(blockHeight)
	state.SyncStatus = "synced"
	state.GapLimit = s.gapLimit

	if err := s.storage.SaveWalletSyncState(state); err != nil {
		return fmt.Errorf("failed to save sync state: %w", err)
	}

	s.logger.Info("UTXO sync complete",
		"chain", symbol,
		"external_addresses", externalIndex+1,
		"change_addresses", changeIndex+1,
	)

	return nil
}

// scanAddresses scans addresses starting from startIndex using gap limit.
// Returns the highest index with activity.
func (s *UTXOSyncService) scanAddresses(
	ctx context.Context,
	symbol string,
	b backend.Backend,
	change uint32,
	startIndex uint32,
) (uint32, error) {
	consecutiveEmpty := uint32(0)
	lastUsedIndex := startIndex
	currentIndex := uint32(0)

	// If we have previous state, start from there
	if startIndex > 0 {
		// Re-scan from beginning to catch any new UTXOs on existing addresses
		// but we can be smarter about the gap limit
		lastUsedIndex = 0
	}

	for {
		select {
		case <-ctx.Done():
			return lastUsedIndex, ctx.Err()
		default:
		}

		// Derive address at this index
		address, err := s.wallet.DeriveAddressWithChange(symbol, 0, change, currentIndex)
		if err != nil {
			return lastUsedIndex, fmt.Errorf("failed to derive address at index %d: %w", currentIndex, err)
		}

		// Determine address type
		chainParams, _ := chain.Get(symbol, s.network)
		addrType := detectAddressType(address, chainParams)

		// Save address to storage
		walletAddr := &storage.WalletAddress{
			Address:      address,
			Chain:        symbol,
			Account:      0,
			Change:       change,
			AddressIndex: currentIndex,
			AddressType:  addrType,
		}
		if err := s.storage.SaveWalletAddress(walletAddr); err != nil {
			s.logger.Warn("failed to save address", "address", address, "error", err)
		}

		// Fetch UTXOs for this address
		utxos, err := b.GetAddressUTXOs(ctx, address)
		if err != nil {
			s.logger.Warn("failed to get UTXOs", "address", address, "error", err)
			// Continue scanning, don't fail completely
			currentIndex++
			consecutiveEmpty++
			if consecutiveEmpty >= s.gapLimit {
				break
			}
			continue
		}

		// Check if address has any UTXOs or history
		hasActivity := len(utxos) > 0

		// Also check address info for past transactions
		if !hasActivity {
			info, err := b.GetAddressInfo(ctx, address)
			if err == nil && info != nil {
				// Address has had transactions even if no current UTXOs
				hasActivity = info.TxCount > 0
			}
		}

		if hasActivity {
			lastUsedIndex = currentIndex
			consecutiveEmpty = 0

			// Update address stats
			var totalReceived uint64
			for _, utxo := range utxos {
				totalReceived += utxo.Amount
			}
			walletAddr.TxCount = int64(len(utxos))
			walletAddr.TotalReceived = int64(totalReceived)
			walletAddr.FirstSeenAt = time.Now().Unix()
			walletAddr.LastSeenAt = time.Now().Unix()
			if err := s.storage.SaveWalletAddress(walletAddr); err != nil {
				s.logger.Warn("failed to update address", "address", address, "error", err)
			}

			// Save UTXOs
			for _, utxo := range utxos {
				walletUTXO := &storage.WalletUTXO{
					TxID:          utxo.TxID,
					Vout:          utxo.Vout,
					Amount:        utxo.Amount,
					Address:       address,
					Chain:         symbol,
					Account:       0,
					Change:        change,
					AddressIndex:  currentIndex,
					AddressType:   addrType,
					Status:        storage.UTXOStatusConfirmed,
					Confirmations: int64(utxo.Confirmations),
				}

				// Mark as unconfirmed if no confirmations
				if utxo.Confirmations == 0 {
					walletUTXO.Status = storage.UTXOStatusUnconfirmed
				}

				if err := s.storage.SaveWalletUTXO(walletUTXO); err != nil {
					s.logger.Warn("failed to save UTXO",
						"txid", utxo.TxID,
						"vout", utxo.Vout,
						"error", err,
					)
				}
			}

			s.logger.Debug("found UTXOs",
				"address", address,
				"index", currentIndex,
				"change", change,
				"utxo_count", len(utxos),
			)
		} else {
			consecutiveEmpty++
		}

		currentIndex++

		// Stop if we've hit the gap limit
		if consecutiveEmpty >= s.gapLimit {
			break
		}
	}

	return lastUsedIndex, nil
}

// =============================================================================
// UTXO Retrieval
// =============================================================================

// GetSpendableUTXOs returns all spendable UTXOs for a chain.
// Combines persisted UTXOs with any specified minimum confirmations.
func (s *UTXOSyncService) GetSpendableUTXOs(ctx context.Context, symbol string, minConfirmations int) ([]*AddressUTXO, error) {
	// Get confirmed UTXOs from storage
	storageUTXOs, err := s.storage.GetSpendableUTXOs(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs from storage: %w", err)
	}

	// Convert to AddressUTXO
	result := make([]*AddressUTXO, 0, len(storageUTXOs))
	for _, u := range storageUTXOs {
		// Filter by confirmations if needed
		if minConfirmations > 0 && u.Confirmations < int64(minConfirmations) {
			continue
		}

		result = append(result, &AddressUTXO{
			TxID:         u.TxID,
			Vout:         u.Vout,
			Amount:       u.Amount,
			Address:      u.Address,
			Account:      u.Account,
			Change:       u.Change,
			AddressIndex: u.AddressIndex,
			AddressType:  u.AddressType,
		})
	}

	return result, nil
}

// GetAllUTXOs returns all UTXOs for a chain (including unconfirmed).
func (s *UTXOSyncService) GetAllUTXOs(ctx context.Context, symbol string) ([]*AddressUTXO, error) {
	storageUTXOs, err := s.storage.GetAllUTXOs(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs from storage: %w", err)
	}

	result := make([]*AddressUTXO, 0, len(storageUTXOs))
	for _, u := range storageUTXOs {
		// Skip pending spend UTXOs
		if u.Status == storage.UTXOStatusPendingSpend {
			continue
		}

		result = append(result, &AddressUTXO{
			TxID:         u.TxID,
			Vout:         u.Vout,
			Amount:       u.Amount,
			Address:      u.Address,
			Account:      u.Account,
			Change:       u.Change,
			AddressIndex: u.AddressIndex,
			AddressType:  u.AddressType,
		})
	}

	return result, nil
}

// GetTotalBalance returns the total spendable balance for a chain.
func (s *UTXOSyncService) GetTotalBalance(ctx context.Context, symbol string) (uint64, error) {
	return s.storage.GetTotalBalance(symbol)
}

// GetBalanceBreakdown returns balance broken down by status.
func (s *UTXOSyncService) GetBalanceBreakdown(ctx context.Context, symbol string) (confirmed, unconfirmed, pending uint64, err error) {
	return s.storage.GetBalanceByStatus(symbol)
}

// =============================================================================
// UTXO Status Management
// =============================================================================

// MarkUTXOsSpending marks UTXOs as pending spend when creating a transaction.
func (s *UTXOSyncService) MarkUTXOsSpending(utxos []*AddressUTXO, spendTxID string) error {
	for _, u := range utxos {
		if err := s.storage.MarkUTXOPendingSpend(u.TxID, u.Vout, spendTxID); err != nil {
			return fmt.Errorf("failed to mark UTXO %s:%d as pending: %w", u.TxID, u.Vout, err)
		}
	}
	return nil
}

// ConfirmUTXOsSpent marks UTXOs as fully spent after transaction confirms.
func (s *UTXOSyncService) ConfirmUTXOsSpent(utxos []*AddressUTXO, spendTxID string) error {
	for _, u := range utxos {
		if err := s.storage.MarkUTXOSpent(u.TxID, u.Vout, spendTxID); err != nil {
			s.logger.Warn("failed to mark UTXO spent", "txid", u.TxID, "vout", u.Vout, "error", err)
		}
	}
	return nil
}

// RevertPendingSpend reverts UTXOs from pending back to confirmed (if tx failed).
func (s *UTXOSyncService) RevertPendingSpend(utxos []*AddressUTXO) error {
	for _, u := range utxos {
		if err := s.storage.RevertUTXOPendingSpend(u.TxID, u.Vout); err != nil {
			s.logger.Warn("failed to revert UTXO pending spend", "txid", u.TxID, "vout", u.Vout, "error", err)
		}
	}
	return nil
}

// =============================================================================
// Fresh Scan (No Persistence)
// =============================================================================

// FreshScanUTXOs performs a fresh UTXO scan without using persistence.
// Useful for one-time operations or when storage isn't available.
func (s *UTXOSyncService) FreshScanUTXOs(ctx context.Context, symbol string) ([]*AddressUTXO, error) {
	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend configured for chain: %s", symbol)
	}

	if err := b.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect backend: %w", err)
	}

	chainParams, _ := chain.Get(symbol, s.network)
	var allUTXOs []*AddressUTXO

	// Scan both external and change addresses
	for _, change := range []uint32{0, 1} {
		consecutiveEmpty := uint32(0)
		index := uint32(0)

		for consecutiveEmpty < s.gapLimit {
			select {
			case <-ctx.Done():
				return allUTXOs, ctx.Err()
			default:
			}

			address, err := s.wallet.DeriveAddressWithChange(symbol, 0, change, index)
			if err != nil {
				index++
				consecutiveEmpty++
				continue
			}

			utxos, err := b.GetAddressUTXOs(ctx, address)
			if err != nil {
				index++
				consecutiveEmpty++
				continue
			}

			if len(utxos) > 0 {
				consecutiveEmpty = 0
				addrType := detectAddressType(address, chainParams)

				for _, u := range utxos {
					allUTXOs = append(allUTXOs, &AddressUTXO{
						TxID:         u.TxID,
						Vout:         u.Vout,
						Amount:       u.Amount,
						Address:      address,
						Account:      0,
						Change:       change,
						AddressIndex: index,
						AddressType:  addrType,
					})
				}
			} else {
				consecutiveEmpty++
			}

			index++
		}
	}

	return allUTXOs, nil
}

// =============================================================================
// Background Sync
// =============================================================================

// StartBackgroundSync starts periodic background synchronization.
func (s *UTXOSyncService) StartBackgroundSync(interval time.Duration, chains []string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Do initial sync
		for _, chain := range chains {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := s.SyncChain(ctx, chain); err != nil {
				s.logger.Warn("initial sync failed", "chain", chain, "error", err)
			}
			cancel()
		}

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				for _, chain := range chains {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					if err := s.SyncChain(ctx, chain); err != nil {
						s.logger.Warn("background sync failed", "chain", chain, "error", err)
					}
					cancel()
				}
			}
		}
	}()
}

// StopBackgroundSync stops the background sync goroutine.
func (s *UTXOSyncService) StopBackgroundSync() {
	close(s.stopCh)
	s.wg.Wait()
}

// GetNextChangeAddress returns the next unused change address.
func (s *UTXOSyncService) GetNextChangeAddress(symbol string) (string, uint32, error) {
	state, err := s.storage.GetWalletSyncState(symbol)
	if err != nil {
		return "", 0, err
	}

	// Use next index after last known change address
	nextIndex := state.LastChangeIndex + 1

	address, err := s.wallet.DeriveAddressWithChange(symbol, 0, 1, nextIndex)
	if err != nil {
		return "", 0, err
	}

	return address, nextIndex, nil
}

// GetNextReceiveAddress returns the next unused external address.
func (s *UTXOSyncService) GetNextReceiveAddress(symbol string) (string, uint32, error) {
	state, err := s.storage.GetWalletSyncState(symbol)
	if err != nil {
		return "", 0, err
	}

	nextIndex := state.LastExternalIndex + 1

	address, err := s.wallet.DeriveAddressWithChange(symbol, 0, 0, nextIndex)
	if err != nil {
		return "", 0, err
	}

	return address, nextIndex, nil
}
