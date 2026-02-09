// Package storage provides wallet UTXO persistence for multi-address spending.
package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// =============================================================================
// Wallet Address Types and Operations
// =============================================================================

// WalletAddress represents a derived wallet address with its derivation path.
type WalletAddress struct {
	Address      string `json:"address"`
	Chain        string `json:"chain"`
	Account      uint32 `json:"account"`
	Change       uint32 `json:"change"`       // 0=external, 1=change
	AddressIndex uint32 `json:"address_index"`
	AddressType  string `json:"address_type"` // p2wpkh, p2tr, p2pkh

	// Usage stats
	TxCount       int64 `json:"tx_count"`
	TotalReceived int64 `json:"total_received"`
	TotalSent     int64 `json:"total_sent"`

	// Timestamps
	CreatedAt   int64 `json:"created_at"`
	FirstSeenAt int64 `json:"first_seen_at,omitempty"`
	LastSeenAt  int64 `json:"last_seen_at,omitempty"`
}

// SaveWalletAddress saves or updates a wallet address.
func (s *Storage) SaveWalletAddress(addr *WalletAddress) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if addr.CreatedAt == 0 {
		addr.CreatedAt = now
	}

	query := `
		INSERT INTO wallet_addresses (
			address, chain, account, change, address_index, address_type,
			tx_count, total_received, total_sent,
			created_at, first_seen_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(address) DO UPDATE SET
			tx_count = excluded.tx_count,
			total_received = excluded.total_received,
			total_sent = excluded.total_sent,
			last_seen_at = excluded.last_seen_at
	`

	_, err := s.db.Exec(query,
		addr.Address, addr.Chain, addr.Account, addr.Change, addr.AddressIndex, addr.AddressType,
		addr.TxCount, addr.TotalReceived, addr.TotalSent,
		addr.CreatedAt, addr.FirstSeenAt, addr.LastSeenAt,
	)
	return err
}

// GetWalletAddress retrieves a wallet address by its string representation.
func (s *Storage) GetWalletAddress(address string) (*WalletAddress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT address, chain, account, change, address_index, address_type,
			   tx_count, total_received, total_sent,
			   created_at, first_seen_at, last_seen_at
		FROM wallet_addresses WHERE address = ?
	`

	var addr WalletAddress
	var firstSeen, lastSeen sql.NullInt64

	err := s.db.QueryRow(query, address).Scan(
		&addr.Address, &addr.Chain, &addr.Account, &addr.Change, &addr.AddressIndex, &addr.AddressType,
		&addr.TxCount, &addr.TotalReceived, &addr.TotalSent,
		&addr.CreatedAt, &firstSeen, &lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if firstSeen.Valid {
		addr.FirstSeenAt = firstSeen.Int64
	}
	if lastSeen.Valid {
		addr.LastSeenAt = lastSeen.Int64
	}

	return &addr, nil
}

// GetWalletAddressByPath retrieves a wallet address by derivation path.
func (s *Storage) GetWalletAddressByPath(chain string, account, change, index uint32) (*WalletAddress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT address, chain, account, change, address_index, address_type,
			   tx_count, total_received, total_sent,
			   created_at, first_seen_at, last_seen_at
		FROM wallet_addresses
		WHERE chain = ? AND account = ? AND change = ? AND address_index = ?
	`

	var addr WalletAddress
	var firstSeen, lastSeen sql.NullInt64

	err := s.db.QueryRow(query, chain, account, change, index).Scan(
		&addr.Address, &addr.Chain, &addr.Account, &addr.Change, &addr.AddressIndex, &addr.AddressType,
		&addr.TxCount, &addr.TotalReceived, &addr.TotalSent,
		&addr.CreatedAt, &firstSeen, &lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if firstSeen.Valid {
		addr.FirstSeenAt = firstSeen.Int64
	}
	if lastSeen.Valid {
		addr.LastSeenAt = lastSeen.Int64
	}

	return &addr, nil
}

// ListWalletAddresses returns all addresses for a chain.
func (s *Storage) ListWalletAddresses(chain string) ([]*WalletAddress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT address, chain, account, change, address_index, address_type,
			   tx_count, total_received, total_sent,
			   created_at, first_seen_at, last_seen_at
		FROM wallet_addresses
		WHERE chain = ?
		ORDER BY account, change, address_index
	`

	rows, err := s.db.Query(query, chain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var addresses []*WalletAddress
	for rows.Next() {
		var addr WalletAddress
		var firstSeen, lastSeen sql.NullInt64

		err := rows.Scan(
			&addr.Address, &addr.Chain, &addr.Account, &addr.Change, &addr.AddressIndex, &addr.AddressType,
			&addr.TxCount, &addr.TotalReceived, &addr.TotalSent,
			&addr.CreatedAt, &firstSeen, &lastSeen,
		)
		if err != nil {
			return nil, err
		}

		if firstSeen.Valid {
			addr.FirstSeenAt = firstSeen.Int64
		}
		if lastSeen.Valid {
			addr.LastSeenAt = lastSeen.Int64
		}

		addresses = append(addresses, &addr)
	}

	return addresses, rows.Err()
}

// GetMaxAddressIndex returns the highest address index for a given chain and change type.
func (s *Storage) GetMaxAddressIndex(chain string, account, change uint32) (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT COALESCE(MAX(address_index), -1)
		FROM wallet_addresses
		WHERE chain = ? AND account = ? AND change = ?
	`

	var maxIndex int64
	err := s.db.QueryRow(query, chain, account, change).Scan(&maxIndex)
	if err != nil {
		return 0, err
	}

	if maxIndex < 0 {
		return 0, nil
	}
	return uint32(maxIndex), nil
}

// GetNextAddressIndex returns the next available address index for a given chain.
// This is the highest used index + 1, or 0 if no addresses exist.
func (s *Storage) GetNextAddressIndex(chain string, account, change uint32) (uint32, error) {
	maxIndex, err := s.GetMaxAddressIndex(chain, account, change)
	if err != nil {
		return 0, err
	}

	// If max is 0 and we need to check if any addresses exist
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	countQuery := `SELECT COUNT(*) FROM wallet_addresses WHERE chain = ? AND account = ? AND change = ?`
	if err := s.db.QueryRow(countQuery, chain, account, change).Scan(&count); err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil // No addresses yet, start at 0
	}

	return maxIndex + 1, nil
}

// =============================================================================
// Wallet UTXO Types and Operations
// =============================================================================

// UTXOStatus represents the status of a UTXO.
type UTXOStatus string

const (
	UTXOStatusUnconfirmed  UTXOStatus = "unconfirmed"
	UTXOStatusConfirmed    UTXOStatus = "confirmed"
	UTXOStatusPendingSpend UTXOStatus = "pending_spend"
	UTXOStatusSpent        UTXOStatus = "spent"
)

// WalletUTXO represents a UTXO with its derivation path for signing.
type WalletUTXO struct {
	TxID string `json:"txid"`
	Vout uint32 `json:"vout"`

	// Amount in smallest units
	Amount uint64 `json:"amount"`

	// Address info
	Address     string `json:"address"`
	Chain       string `json:"chain"`
	AddressType string `json:"address_type"`

	// Derivation path (for key derivation during signing)
	Account      uint32 `json:"account"`
	Change       uint32 `json:"change"`
	AddressIndex uint32 `json:"address_index"`

	// Script
	ScriptPubKey string `json:"script_pubkey,omitempty"`

	// Status
	Status        UTXOStatus `json:"status"`
	BlockHeight   int64      `json:"block_height,omitempty"`
	BlockHash     string     `json:"block_hash,omitempty"`
	Confirmations int64      `json:"confirmations"`

	// Spending info
	SpentTxID string `json:"spent_txid,omitempty"`
	SpentAt   int64  `json:"spent_at,omitempty"`

	// Timestamps
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// SaveWalletUTXO saves or updates a UTXO.
func (s *Storage) SaveWalletUTXO(utxo *WalletUTXO) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if utxo.CreatedAt == 0 {
		utxo.CreatedAt = now
	}
	utxo.UpdatedAt = now

	query := `
		INSERT INTO wallet_utxos (
			txid, vout, amount, address, chain,
			account, change, address_index,
			script_pubkey, address_type, status,
			block_height, block_hash, confirmations,
			spent_txid, spent_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(txid, vout) DO UPDATE SET
			status = excluded.status,
			block_height = excluded.block_height,
			block_hash = excluded.block_hash,
			confirmations = excluded.confirmations,
			spent_txid = excluded.spent_txid,
			spent_at = excluded.spent_at,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(query,
		utxo.TxID, utxo.Vout, utxo.Amount, utxo.Address, utxo.Chain,
		utxo.Account, utxo.Change, utxo.AddressIndex,
		utxo.ScriptPubKey, utxo.AddressType, utxo.Status,
		utxo.BlockHeight, utxo.BlockHash, utxo.Confirmations,
		utxo.SpentTxID, utxo.SpentAt,
		utxo.CreatedAt, utxo.UpdatedAt,
	)
	return err
}

// GetWalletUTXO retrieves a specific UTXO by txid and vout.
func (s *Storage) GetWalletUTXO(txid string, vout uint32) (*WalletUTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT txid, vout, amount, address, chain,
			   account, change, address_index,
			   script_pubkey, address_type, status,
			   block_height, block_hash, confirmations,
			   spent_txid, spent_at,
			   created_at, updated_at
		FROM wallet_utxos WHERE txid = ? AND vout = ?
	`

	return s.scanWalletUTXO(s.db.QueryRow(query, txid, vout))
}

// GetSpendableUTXOs returns all confirmed, unspent UTXOs for a chain.
func (s *Storage) GetSpendableUTXOs(chain string) ([]*WalletUTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT txid, vout, amount, address, chain,
			   account, change, address_index,
			   script_pubkey, address_type, status,
			   block_height, block_hash, confirmations,
			   spent_txid, spent_at,
			   created_at, updated_at
		FROM wallet_utxos
		WHERE chain = ? AND status = ?
		ORDER BY amount DESC
	`

	return s.queryWalletUTXOs(query, chain, UTXOStatusConfirmed)
}

// GetUTXOsByAddress returns all UTXOs for a specific address.
func (s *Storage) GetUTXOsByAddress(address string) ([]*WalletUTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT txid, vout, amount, address, chain,
			   account, change, address_index,
			   script_pubkey, address_type, status,
			   block_height, block_hash, confirmations,
			   spent_txid, spent_at,
			   created_at, updated_at
		FROM wallet_utxos
		WHERE address = ?
		ORDER BY status, amount DESC
	`

	return s.queryWalletUTXOs(query, address)
}

// GetAllUTXOs returns all UTXOs for a chain (including unconfirmed and pending).
func (s *Storage) GetAllUTXOs(chain string) ([]*WalletUTXO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT txid, vout, amount, address, chain,
			   account, change, address_index,
			   script_pubkey, address_type, status,
			   block_height, block_hash, confirmations,
			   spent_txid, spent_at,
			   created_at, updated_at
		FROM wallet_utxos
		WHERE chain = ? AND status IN (?, ?, ?)
		ORDER BY amount DESC
	`

	return s.queryWalletUTXOs(query, chain, UTXOStatusConfirmed, UTXOStatusUnconfirmed, UTXOStatusPendingSpend)
}

// MarkUTXOPendingSpend marks a UTXO as pending spend (used in a transaction).
func (s *Storage) MarkUTXOPendingSpend(txid string, vout uint32, spendTxID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	query := `
		UPDATE wallet_utxos
		SET status = ?, spent_txid = ?, updated_at = ?
		WHERE txid = ? AND vout = ?
	`

	result, err := s.db.Exec(query, UTXOStatusPendingSpend, spendTxID, now, txid, vout)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("UTXO not found: %s:%d", txid, vout)
	}

	return nil
}

// MarkUTXOSpent marks a UTXO as spent (confirmed in a block).
func (s *Storage) MarkUTXOSpent(txid string, vout uint32, spendTxID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	query := `
		UPDATE wallet_utxos
		SET status = ?, spent_txid = ?, spent_at = ?, updated_at = ?
		WHERE txid = ? AND vout = ?
	`

	result, err := s.db.Exec(query, UTXOStatusSpent, spendTxID, now, now, txid, vout)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("UTXO not found: %s:%d", txid, vout)
	}

	return nil
}

// RevertUTXOPendingSpend reverts a pending spend back to confirmed (if tx failed).
func (s *Storage) RevertUTXOPendingSpend(txid string, vout uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	query := `
		UPDATE wallet_utxos
		SET status = ?, spent_txid = NULL, updated_at = ?
		WHERE txid = ? AND vout = ? AND status = ?
	`

	_, err := s.db.Exec(query, UTXOStatusConfirmed, now, txid, vout, UTXOStatusPendingSpend)
	return err
}

// UpdateUTXOConfirmations updates confirmation count for a UTXO.
func (s *Storage) UpdateUTXOConfirmations(txid string, vout uint32, confirmations int64, blockHeight int64, blockHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// If confirmations > 0 and status is unconfirmed, mark as confirmed
	query := `
		UPDATE wallet_utxos
		SET confirmations = ?,
			block_height = CASE WHEN ? > 0 THEN ? ELSE block_height END,
			block_hash = CASE WHEN ? > 0 THEN ? ELSE block_hash END,
			status = CASE WHEN ? > 0 AND status = ? THEN ? ELSE status END,
			updated_at = ?
		WHERE txid = ? AND vout = ?
	`

	_, err := s.db.Exec(query,
		confirmations,
		confirmations, blockHeight,
		confirmations, blockHash,
		confirmations, UTXOStatusUnconfirmed, UTXOStatusConfirmed,
		now,
		txid, vout,
	)
	return err
}

// DeleteSpentUTXOs removes all spent UTXOs older than the given duration.
func (s *Storage) DeleteSpentUTXOs(olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan).Unix()

	query := `DELETE FROM wallet_utxos WHERE status = ? AND spent_at < ?`

	result, err := s.db.Exec(query, UTXOStatusSpent, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// GetTotalBalance returns the total balance for a chain (confirmed UTXOs only).
func (s *Storage) GetTotalBalance(chain string) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT COALESCE(SUM(amount), 0)
		FROM wallet_utxos
		WHERE chain = ? AND status = ?
	`

	var total int64
	err := s.db.QueryRow(query, chain, UTXOStatusConfirmed).Scan(&total)
	if err != nil {
		return 0, err
	}

	return uint64(total), nil
}

// GetBalanceByStatus returns balances grouped by status.
func (s *Storage) GetBalanceByStatus(chain string) (confirmed, unconfirmed, pending uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT status, COALESCE(SUM(amount), 0)
		FROM wallet_utxos
		WHERE chain = ?
		GROUP BY status
	`

	rows, err := s.db.Query(query, chain)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var amount int64
		if err := rows.Scan(&status, &amount); err != nil {
			return 0, 0, 0, err
		}

		switch UTXOStatus(status) {
		case UTXOStatusConfirmed:
			confirmed = uint64(amount)
		case UTXOStatusUnconfirmed:
			unconfirmed = uint64(amount)
		case UTXOStatusPendingSpend:
			pending = uint64(amount)
		}
	}

	return confirmed, unconfirmed, pending, rows.Err()
}

// =============================================================================
// Wallet Sync State Operations
// =============================================================================

// WalletSyncState represents the sync state for a chain.
type WalletSyncState struct {
	Chain             string `json:"chain"`
	LastExternalIndex uint32 `json:"last_external_index"`
	LastChangeIndex   uint32 `json:"last_change_index"`
	GapLimit          uint32 `json:"gap_limit"`
	LastSyncAt        int64  `json:"last_sync_at,omitempty"`
	LastBlockHeight   int64  `json:"last_block_height,omitempty"`
	SyncStatus        string `json:"sync_status"`
}

// SaveWalletSyncState saves or updates sync state for a chain.
func (s *Storage) SaveWalletSyncState(state *WalletSyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		INSERT INTO wallet_sync_state (
			chain, last_external_index, last_change_index, gap_limit,
			last_sync_at, last_block_height, sync_status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chain) DO UPDATE SET
			last_external_index = excluded.last_external_index,
			last_change_index = excluded.last_change_index,
			gap_limit = excluded.gap_limit,
			last_sync_at = excluded.last_sync_at,
			last_block_height = excluded.last_block_height,
			sync_status = excluded.sync_status
	`

	_, err := s.db.Exec(query,
		state.Chain, state.LastExternalIndex, state.LastChangeIndex, state.GapLimit,
		state.LastSyncAt, state.LastBlockHeight, state.SyncStatus,
	)
	return err
}

// GetWalletSyncState retrieves sync state for a chain.
func (s *Storage) GetWalletSyncState(chain string) (*WalletSyncState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT chain, last_external_index, last_change_index, gap_limit,
			   last_sync_at, last_block_height, sync_status
		FROM wallet_sync_state WHERE chain = ?
	`

	var state WalletSyncState
	var lastSync, lastBlock sql.NullInt64

	err := s.db.QueryRow(query, chain).Scan(
		&state.Chain, &state.LastExternalIndex, &state.LastChangeIndex, &state.GapLimit,
		&lastSync, &lastBlock, &state.SyncStatus,
	)
	if err == sql.ErrNoRows {
		// Return default state
		return &WalletSyncState{
			Chain:      chain,
			GapLimit:   20,
			SyncStatus: "pending",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if lastSync.Valid {
		state.LastSyncAt = lastSync.Int64
	}
	if lastBlock.Valid {
		state.LastBlockHeight = lastBlock.Int64
	}

	return &state, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func (s *Storage) scanWalletUTXO(row *sql.Row) (*WalletUTXO, error) {
	var utxo WalletUTXO
	var scriptPubKey, blockHash, spentTxID sql.NullString
	var blockHeight, spentAt sql.NullInt64

	err := row.Scan(
		&utxo.TxID, &utxo.Vout, &utxo.Amount, &utxo.Address, &utxo.Chain,
		&utxo.Account, &utxo.Change, &utxo.AddressIndex,
		&scriptPubKey, &utxo.AddressType, &utxo.Status,
		&blockHeight, &blockHash, &utxo.Confirmations,
		&spentTxID, &spentAt,
		&utxo.CreatedAt, &utxo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if scriptPubKey.Valid {
		utxo.ScriptPubKey = scriptPubKey.String
	}
	if blockHash.Valid {
		utxo.BlockHash = blockHash.String
	}
	if blockHeight.Valid {
		utxo.BlockHeight = blockHeight.Int64
	}
	if spentTxID.Valid {
		utxo.SpentTxID = spentTxID.String
	}
	if spentAt.Valid {
		utxo.SpentAt = spentAt.Int64
	}

	return &utxo, nil
}

func (s *Storage) queryWalletUTXOs(query string, args ...interface{}) ([]*WalletUTXO, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var utxos []*WalletUTXO
	for rows.Next() {
		var utxo WalletUTXO
		var scriptPubKey, blockHash, spentTxID sql.NullString
		var blockHeight, spentAt sql.NullInt64

		err := rows.Scan(
			&utxo.TxID, &utxo.Vout, &utxo.Amount, &utxo.Address, &utxo.Chain,
			&utxo.Account, &utxo.Change, &utxo.AddressIndex,
			&scriptPubKey, &utxo.AddressType, &utxo.Status,
			&blockHeight, &blockHash, &utxo.Confirmations,
			&spentTxID, &spentAt,
			&utxo.CreatedAt, &utxo.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if scriptPubKey.Valid {
			utxo.ScriptPubKey = scriptPubKey.String
		}
		if blockHash.Valid {
			utxo.BlockHash = blockHash.String
		}
		if blockHeight.Valid {
			utxo.BlockHeight = blockHeight.Int64
		}
		if spentTxID.Valid {
			utxo.SpentTxID = spentTxID.String
		}
		if spentAt.Valid {
			utxo.SpentAt = spentAt.Int64
		}

		utxos = append(utxos, &utxo)
	}

	return utxos, rows.Err()
}
