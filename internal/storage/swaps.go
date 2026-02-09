// Package storage - Swap state persistence for atomic swaps.
// This file provides CRUD operations for persisting swap state to SQLite,
// enabling recovery after node restart.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Swap persistence errors
var (
	ErrSwapNotFound     = errors.New("swap not found")
	ErrSwapExists       = errors.New("swap already exists")
	ErrInvalidSwapState = errors.New("invalid swap state")
)

// SwapState represents the current state of a swap.
type SwapState string

const (
	SwapStateInit      SwapState = "init"
	SwapStateFunding   SwapState = "funding"
	SwapStateFunded    SwapState = "funded"
	SwapStateSigning   SwapState = "signing"
	SwapStateRedeemed  SwapState = "redeemed"
	SwapStateRefunded  SwapState = "refunded"
	SwapStateFailed    SwapState = "failed"
	SwapStateCancelled SwapState = "cancelled"
)

// SwapRecord represents a persisted swap in the database.
// This contains all data needed to recover a swap after restart.
type SwapRecord struct {
	// Identity
	TradeID string `json:"trade_id"`
	OrderID string `json:"order_id"`

	// Participants
	MakerPeerID string `json:"maker_peer_id"`
	TakerPeerID string `json:"taker_peer_id"`

	// Our role in this swap
	OurRole string `json:"our_role"` // "maker" or "taker"
	IsMaker bool   `json:"is_maker"`

	// Swap details
	OfferChain    string `json:"offer_chain"`
	OfferAmount   uint64 `json:"offer_amount"`
	RequestChain  string `json:"request_chain"`
	RequestAmount uint64 `json:"request_amount"`

	// State
	State SwapState `json:"state"`

	// MuSig2 data (JSON blob - contains keys, nonces, etc.)
	// This is the critical data for recovery
	MethodData json.RawMessage `json:"method_data"`

	// Funding info
	LocalFundingTxID  string `json:"local_funding_txid,omitempty"`
	LocalFundingVout  uint32 `json:"local_funding_vout"`
	RemoteFundingTxID string `json:"remote_funding_txid,omitempty"`
	RemoteFundingVout uint32 `json:"remote_funding_vout"`

	// Timeout tracking (separate for each chain)
	TimeoutHeight        uint32 `json:"timeout_height"`         // Offer chain timeout
	RequestTimeoutHeight uint32 `json:"request_timeout_height"` // Request chain timeout
	TimeoutTimestamp     int64  `json:"timeout_timestamp"`

	// Result
	RedeemTxID    string `json:"redeem_txid,omitempty"`
	RefundTxID    string `json:"refund_txid,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`

	// Timing
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// SaveSwap saves or updates a swap record.
// Uses UPSERT pattern - creates if not exists, updates if exists.
func (s *Storage) SaveSwap(swap *SwapRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if swap.CreatedAt.IsZero() {
		swap.CreatedAt = now
	}
	swap.UpdatedAt = now

	query := `
		INSERT INTO active_swaps (
			trade_id, order_id, maker_peer_id, taker_peer_id,
			our_role, is_maker, offer_chain, offer_amount,
			request_chain, request_amount, state, method_data,
			local_funding_txid, local_funding_vout,
			remote_funding_txid, remote_funding_vout,
			timeout_height, request_timeout_height, timeout_timestamp,
			redeem_txid, refund_txid, failure_reason,
			created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trade_id) DO UPDATE SET
			state = excluded.state,
			method_data = excluded.method_data,
			local_funding_txid = excluded.local_funding_txid,
			local_funding_vout = excluded.local_funding_vout,
			remote_funding_txid = excluded.remote_funding_txid,
			remote_funding_vout = excluded.remote_funding_vout,
			timeout_height = excluded.timeout_height,
			request_timeout_height = excluded.request_timeout_height,
			timeout_timestamp = excluded.timeout_timestamp,
			redeem_txid = excluded.redeem_txid,
			refund_txid = excluded.refund_txid,
			failure_reason = excluded.failure_reason,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at
	`

	_, err := s.db.Exec(query,
		swap.TradeID,
		swap.OrderID,
		swap.MakerPeerID,
		swap.TakerPeerID,
		swap.OurRole,
		boolToInt(swap.IsMaker),
		swap.OfferChain,
		swap.OfferAmount,
		swap.RequestChain,
		swap.RequestAmount,
		string(swap.State),
		string(swap.MethodData),
		swap.LocalFundingTxID,
		swap.LocalFundingVout,
		swap.RemoteFundingTxID,
		swap.RemoteFundingVout,
		swap.TimeoutHeight,
		swap.RequestTimeoutHeight,
		swap.TimeoutTimestamp,
		swap.RedeemTxID,
		swap.RefundTxID,
		swap.FailureReason,
		swap.CreatedAt.Unix(),
		swap.UpdatedAt.Unix(),
		timeToUnixOrZero(swap.CompletedAt),
	)
	return err
}

// GetSwap retrieves a swap by trade ID.
func (s *Storage) GetSwap(tradeID string) (*SwapRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
			our_role, is_maker, offer_chain, offer_amount,
			request_chain, request_amount, state, method_data,
			local_funding_txid, local_funding_vout,
			remote_funding_txid, remote_funding_vout,
			timeout_height, request_timeout_height, timeout_timestamp,
			redeem_txid, refund_txid, failure_reason,
			created_at, updated_at, completed_at
		FROM active_swaps WHERE trade_id = ?
	`

	row := s.db.QueryRow(query, tradeID)
	return scanSwapRecord(row)
}

// GetPendingSwaps returns all swaps that are not in a terminal state.
// These are swaps that need to be recovered on startup.
func (s *Storage) GetPendingSwaps() ([]*SwapRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
			our_role, is_maker, offer_chain, offer_amount,
			request_chain, request_amount, state, method_data,
			local_funding_txid, local_funding_vout,
			remote_funding_txid, remote_funding_vout,
			timeout_height, request_timeout_height, timeout_timestamp,
			redeem_txid, refund_txid, failure_reason,
			created_at, updated_at, completed_at
		FROM active_swaps
		WHERE state NOT IN ('redeemed', 'refunded', 'failed', 'cancelled')
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swaps []*SwapRecord
	for rows.Next() {
		swap, err := scanSwapRecordRows(rows)
		if err != nil {
			return nil, err
		}
		swaps = append(swaps, swap)
	}

	return swaps, rows.Err()
}

// GetSwapsNearingTimeout returns swaps that are close to timeout.
// safetyMargin is the number of blocks before timeout to start looking.
func (s *Storage) GetSwapsNearingTimeout(currentHeight uint32, safetyMargin uint32) ([]*SwapRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	thresholdHeight := currentHeight + safetyMargin

	query := `
		SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
			our_role, is_maker, offer_chain, offer_amount,
			request_chain, request_amount, state, method_data,
			local_funding_txid, local_funding_vout,
			remote_funding_txid, remote_funding_vout,
			timeout_height, request_timeout_height, timeout_timestamp,
			redeem_txid, refund_txid, failure_reason,
			created_at, updated_at, completed_at
		FROM active_swaps
		WHERE state IN ('funded', 'signing')
		AND timeout_height > 0
		AND timeout_height <= ?
		ORDER BY timeout_height ASC
	`

	rows, err := s.db.Query(query, thresholdHeight)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swaps []*SwapRecord
	for rows.Next() {
		swap, err := scanSwapRecordRows(rows)
		if err != nil {
			return nil, err
		}
		swaps = append(swaps, swap)
	}

	return swaps, rows.Err()
}

// GetSwapsPastTimeout returns swaps that have passed their timeout.
// These are candidates for automatic refund.
func (s *Storage) GetSwapsPastTimeout(currentHeight uint32) ([]*SwapRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
			our_role, is_maker, offer_chain, offer_amount,
			request_chain, request_amount, state, method_data,
			local_funding_txid, local_funding_vout,
			remote_funding_txid, remote_funding_vout,
			timeout_height, request_timeout_height, timeout_timestamp,
			redeem_txid, refund_txid, failure_reason,
			created_at, updated_at, completed_at
		FROM active_swaps
		WHERE state IN ('funded', 'signing')
		AND timeout_height > 0
		AND timeout_height < ?
		ORDER BY timeout_height ASC
	`

	rows, err := s.db.Query(query, currentHeight)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swaps []*SwapRecord
	for rows.Next() {
		swap, err := scanSwapRecordRows(rows)
		if err != nil {
			return nil, err
		}
		swaps = append(swaps, swap)
	}

	return swaps, rows.Err()
}

// UpdateSwapState updates the state of a swap.
func (s *Storage) UpdateSwapState(tradeID string, state SwapState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	var completedAt int64
	if isTerminalState(state) {
		completedAt = now
	}

	query := `
		UPDATE active_swaps
		SET state = ?, updated_at = ?, completed_at = CASE WHEN ? > 0 THEN ? ELSE completed_at END
		WHERE trade_id = ?
	`

	result, err := s.db.Exec(query, string(state), now, completedAt, completedAt, tradeID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSwapNotFound
	}

	return nil
}

// UpdateSwapMethodData updates the method_data JSON blob for a swap.
func (s *Storage) UpdateSwapMethodData(tradeID string, methodData json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE active_swaps SET method_data = ?, updated_at = ? WHERE trade_id = ?`

	result, err := s.db.Exec(query, string(methodData), time.Now().Unix(), tradeID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSwapNotFound
	}

	return nil
}

// UpdateSwapFunding updates funding transaction info for a swap.
func (s *Storage) UpdateSwapFunding(tradeID string, isLocal bool, txid string, vout uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var query string
	if isLocal {
		query = `UPDATE active_swaps SET local_funding_txid = ?, local_funding_vout = ?, updated_at = ? WHERE trade_id = ?`
	} else {
		query = `UPDATE active_swaps SET remote_funding_txid = ?, remote_funding_vout = ?, updated_at = ? WHERE trade_id = ?`
	}

	result, err := s.db.Exec(query, txid, vout, time.Now().Unix(), tradeID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSwapNotFound
	}

	return nil
}

// DeleteSwap removes a swap from the database.
// Only use for terminal states or cleanup.
func (s *Storage) DeleteSwap(tradeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM active_swaps WHERE trade_id = ?", tradeID)
	return err
}

// ListSwaps returns all swaps with optional filtering.
func (s *Storage) ListSwaps(limit int, includeCompleted bool) ([]*SwapRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var query string
	if includeCompleted {
		query = `
			SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
				our_role, is_maker, offer_chain, offer_amount,
				request_chain, request_amount, state, method_data,
				local_funding_txid, local_funding_vout,
				remote_funding_txid, remote_funding_vout,
				timeout_height, request_timeout_height, timeout_timestamp,
				redeem_txid, refund_txid, failure_reason,
				created_at, updated_at, completed_at
			FROM active_swaps
			ORDER BY updated_at DESC
		`
	} else {
		query = `
			SELECT trade_id, order_id, maker_peer_id, taker_peer_id,
				our_role, is_maker, offer_chain, offer_amount,
				request_chain, request_amount, state, method_data,
				local_funding_txid, local_funding_vout,
				remote_funding_txid, remote_funding_vout,
				timeout_height, request_timeout_height, timeout_timestamp,
				redeem_txid, refund_txid, failure_reason,
				created_at, updated_at, completed_at
			FROM active_swaps
			WHERE state NOT IN ('redeemed', 'refunded', 'failed', 'cancelled')
			ORDER BY updated_at DESC
		`
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swaps []*SwapRecord
	for rows.Next() {
		swap, err := scanSwapRecordRows(rows)
		if err != nil {
			return nil, err
		}
		swaps = append(swaps, swap)
	}

	return swaps, rows.Err()
}

// SwapCount returns count of swaps by state.
func (s *Storage) SwapCount() (pending, completed int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err = s.db.QueryRow(
		"SELECT COUNT(*) FROM active_swaps WHERE state NOT IN ('redeemed', 'refunded', 'failed', 'cancelled')",
	).Scan(&pending)
	if err != nil {
		return
	}

	err = s.db.QueryRow(
		"SELECT COUNT(*) FROM active_swaps WHERE state IN ('redeemed', 'refunded', 'failed', 'cancelled')",
	).Scan(&completed)
	return
}

// Helper functions

func isTerminalState(state SwapState) bool {
	switch state {
	case SwapStateRedeemed, SwapStateRefunded, SwapStateFailed, SwapStateCancelled:
		return true
	}
	return false
}

func scanSwapRecord(row *sql.Row) (*SwapRecord, error) {
	var swap SwapRecord
	var isMaker int
	var methodData, localFundingTxID, remoteFundingTxID, redeemTxID, refundTxID, failureReason sql.NullString
	var createdAt, updatedAt, completedAt int64

	err := row.Scan(
		&swap.TradeID,
		&swap.OrderID,
		&swap.MakerPeerID,
		&swap.TakerPeerID,
		&swap.OurRole,
		&isMaker,
		&swap.OfferChain,
		&swap.OfferAmount,
		&swap.RequestChain,
		&swap.RequestAmount,
		&swap.State,
		&methodData,
		&localFundingTxID,
		&swap.LocalFundingVout,
		&remoteFundingTxID,
		&swap.RemoteFundingVout,
		&swap.TimeoutHeight,
		&swap.RequestTimeoutHeight,
		&swap.TimeoutTimestamp,
		&redeemTxID,
		&refundTxID,
		&failureReason,
		&createdAt,
		&updatedAt,
		&completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSwapNotFound
		}
		return nil, err
	}

	swap.IsMaker = isMaker == 1
	if methodData.Valid {
		swap.MethodData = json.RawMessage(methodData.String)
	}
	if localFundingTxID.Valid {
		swap.LocalFundingTxID = localFundingTxID.String
	}
	if remoteFundingTxID.Valid {
		swap.RemoteFundingTxID = remoteFundingTxID.String
	}
	if redeemTxID.Valid {
		swap.RedeemTxID = redeemTxID.String
	}
	if refundTxID.Valid {
		swap.RefundTxID = refundTxID.String
	}
	if failureReason.Valid {
		swap.FailureReason = failureReason.String
	}

	swap.CreatedAt = time.Unix(createdAt, 0)
	swap.UpdatedAt = time.Unix(updatedAt, 0)
	if completedAt > 0 {
		swap.CompletedAt = time.Unix(completedAt, 0)
	}

	return &swap, nil
}

func scanSwapRecordRows(rows *sql.Rows) (*SwapRecord, error) {
	var swap SwapRecord
	var isMaker int
	var methodData, localFundingTxID, remoteFundingTxID, redeemTxID, refundTxID, failureReason sql.NullString
	var createdAt, updatedAt, completedAt int64

	err := rows.Scan(
		&swap.TradeID,
		&swap.OrderID,
		&swap.MakerPeerID,
		&swap.TakerPeerID,
		&swap.OurRole,
		&isMaker,
		&swap.OfferChain,
		&swap.OfferAmount,
		&swap.RequestChain,
		&swap.RequestAmount,
		&swap.State,
		&methodData,
		&localFundingTxID,
		&swap.LocalFundingVout,
		&remoteFundingTxID,
		&swap.RemoteFundingVout,
		&swap.TimeoutHeight,
		&swap.RequestTimeoutHeight,
		&swap.TimeoutTimestamp,
		&redeemTxID,
		&refundTxID,
		&failureReason,
		&createdAt,
		&updatedAt,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}

	swap.IsMaker = isMaker == 1
	if methodData.Valid {
		swap.MethodData = json.RawMessage(methodData.String)
	}
	if localFundingTxID.Valid {
		swap.LocalFundingTxID = localFundingTxID.String
	}
	if remoteFundingTxID.Valid {
		swap.RemoteFundingTxID = remoteFundingTxID.String
	}
	if redeemTxID.Valid {
		swap.RedeemTxID = redeemTxID.String
	}
	if refundTxID.Valid {
		swap.RefundTxID = refundTxID.String
	}
	if failureReason.Valid {
		swap.FailureReason = failureReason.String
	}

	swap.CreatedAt = time.Unix(createdAt, 0)
	swap.UpdatedAt = time.Unix(updatedAt, 0)
	if completedAt > 0 {
		swap.CompletedAt = time.Unix(completedAt, 0)
	}

	return &swap, nil
}
