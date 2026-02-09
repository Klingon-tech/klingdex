// Package storage - Trade storage operations.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Trade errors
var (
	ErrTradeNotFound = errors.New("trade not found")
)

// TradeState represents the current state of a trade.
type TradeState string

const (
	TradeStateInit      TradeState = "init"      // Trade initiated
	TradeStateAccepted  TradeState = "accepted"  // Both parties agreed
	TradeStateFunding   TradeState = "funding"   // Funding in progress
	TradeStateFunded    TradeState = "funded"    // Both sides funded
	TradeStateRedeemed  TradeState = "redeemed"  // Successfully completed
	TradeStateRefunded  TradeState = "refunded"  // Refunded due to timeout
	TradeStateFailed    TradeState = "failed"    // Failed for other reasons
	TradeStateAborted   TradeState = "aborted"   // Aborted before funding
)

// TradeRole indicates our role in the trade.
type TradeRole string

const (
	TradeRoleMaker TradeRole = "maker" // We created the order
	TradeRoleTaker TradeRole = "taker" // We took the order
)

// Trade represents a trade in the database.
type Trade struct {
	ID          string
	OrderID     string
	MakerPeerID string
	TakerPeerID string
	MakerPubKey string // Hex-encoded compressed pubkey
	TakerPubKey string // Hex-encoded compressed pubkey
	OurRole     TradeRole
	Method      string // musig2, htlc_bitcoin, htlc_evm, etc.
	State       TradeState

	// Actual amounts for this trade
	OfferChain    string
	OfferAmount   uint64
	RequestChain  string
	RequestAmount uint64

	// Timing
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	CompletedAt *time.Time

	// Failure tracking
	FailureReason string
}

// CreateTrade creates a new trade in the database.
func (s *Storage) CreateTrade(trade *Trade) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO trades (
			id, order_id, maker_peer_id, taker_peer_id, our_role, method, state,
			offer_chain, offer_amount, request_chain, request_amount, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		trade.ID, trade.OrderID, trade.MakerPeerID, trade.TakerPeerID,
		trade.OurRole, trade.Method, trade.State,
		trade.OfferChain, trade.OfferAmount,
		trade.RequestChain, trade.RequestAmount,
		trade.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to create trade: %w", err)
	}

	return nil
}

// GetTrade retrieves a trade by ID.
func (s *Storage) GetTrade(id string) (*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var trade Trade
	var createdAt, updatedAt, completedAt sql.NullInt64
	var failureReason, makerPubKey, takerPubKey sql.NullString

	err := s.db.QueryRow(`
		SELECT id, order_id, maker_peer_id, taker_peer_id, maker_pubkey, taker_pubkey,
			our_role, method, state,
			offer_chain, offer_amount, request_chain, request_amount,
			created_at, updated_at, completed_at, failure_reason
		FROM trades WHERE id = ?
	`, id).Scan(
		&trade.ID, &trade.OrderID, &trade.MakerPeerID, &trade.TakerPeerID,
		&makerPubKey, &takerPubKey,
		&trade.OurRole, &trade.Method, &trade.State,
		&trade.OfferChain, &trade.OfferAmount,
		&trade.RequestChain, &trade.RequestAmount,
		&createdAt, &updatedAt, &completedAt, &failureReason,
	)

	if err == sql.ErrNoRows {
		return nil, ErrTradeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trade: %w", err)
	}

	// Convert timestamps
	trade.CreatedAt = time.Unix(createdAt.Int64, 0)
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0)
		trade.UpdatedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		trade.CompletedAt = &t
	}
	if failureReason.Valid {
		trade.FailureReason = failureReason.String
	}
	if makerPubKey.Valid {
		trade.MakerPubKey = makerPubKey.String
	}
	if takerPubKey.Valid {
		trade.TakerPubKey = takerPubKey.String
	}

	return &trade, nil
}

// GetTradeByOrderID retrieves a trade by its order ID.
func (s *Storage) GetTradeByOrderID(orderID string) (*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var trade Trade
	var createdAt, updatedAt, completedAt sql.NullInt64
	var failureReason, makerPubKey, takerPubKey sql.NullString

	err := s.db.QueryRow(`
		SELECT id, order_id, maker_peer_id, taker_peer_id, maker_pubkey, taker_pubkey,
			our_role, method, state,
			offer_chain, offer_amount, request_chain, request_amount,
			created_at, updated_at, completed_at, failure_reason
		FROM trades WHERE order_id = ?
	`, orderID).Scan(
		&trade.ID, &trade.OrderID, &trade.MakerPeerID, &trade.TakerPeerID,
		&makerPubKey, &takerPubKey,
		&trade.OurRole, &trade.Method, &trade.State,
		&trade.OfferChain, &trade.OfferAmount,
		&trade.RequestChain, &trade.RequestAmount,
		&createdAt, &updatedAt, &completedAt, &failureReason,
	)

	if err == sql.ErrNoRows {
		return nil, ErrTradeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trade by order: %w", err)
	}

	trade.CreatedAt = time.Unix(createdAt.Int64, 0)
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0)
		trade.UpdatedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		trade.CompletedAt = &t
	}
	if failureReason.Valid {
		trade.FailureReason = failureReason.String
	}
	if makerPubKey.Valid {
		trade.MakerPubKey = makerPubKey.String
	}
	if takerPubKey.Valid {
		trade.TakerPubKey = takerPubKey.String
	}

	return &trade, nil
}

// UpdateTradeState updates the state of a trade.
func (s *Storage) UpdateTradeState(id string, state TradeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	var completedAt *int64

	// Set completed_at for terminal states
	if state == TradeStateRedeemed || state == TradeStateRefunded ||
		state == TradeStateFailed || state == TradeStateAborted {
		completedAt = &now
	}

	result, err := s.db.Exec(`
		UPDATE trades SET state = ?, updated_at = ?, completed_at = COALESCE(?, completed_at)
		WHERE id = ?
	`, state, now, completedAt, id)

	if err != nil {
		return fmt.Errorf("failed to update trade state: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTradeNotFound
	}

	return nil
}

// UpdateTradeFailure marks a trade as failed with a reason.
func (s *Storage) UpdateTradeFailure(id string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	result, err := s.db.Exec(`
		UPDATE trades SET state = ?, failure_reason = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`, TradeStateFailed, reason, now, now, id)

	if err != nil {
		return fmt.Errorf("failed to update trade failure: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTradeNotFound
	}

	return nil
}

// UpdateTradePubKey updates a maker or taker pubkey for a trade.
func (s *Storage) UpdateTradePubKey(id string, isMaker bool, pubkey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	var query string
	if isMaker {
		query = "UPDATE trades SET maker_pubkey = ?, updated_at = ? WHERE id = ?"
	} else {
		query = "UPDATE trades SET taker_pubkey = ?, updated_at = ? WHERE id = ?"
	}

	result, err := s.db.Exec(query, pubkey, now, id)
	if err != nil {
		return fmt.Errorf("failed to update trade pubkey: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTradeNotFound
	}

	return nil
}

// TradeFilter defines filters for listing trades.
type TradeFilter struct {
	State       *TradeState
	OurRole     *TradeRole
	Method      string
	MakerPeerID string
	TakerPeerID string
	Limit       int
	Offset      int
}

// ListTrades returns trades matching the filter.
func (s *Storage) ListTrades(filter TradeFilter) ([]*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, order_id, maker_peer_id, taker_peer_id, maker_pubkey, taker_pubkey,
			our_role, method, state,
			offer_chain, offer_amount, request_chain, request_amount,
			created_at, updated_at, completed_at, failure_reason
		FROM trades WHERE 1=1
	`
	args := []interface{}{}

	if filter.State != nil {
		query += " AND state = ?"
		args = append(args, *filter.State)
	}
	if filter.OurRole != nil {
		query += " AND our_role = ?"
		args = append(args, *filter.OurRole)
	}
	if filter.Method != "" {
		query += " AND method = ?"
		args = append(args, filter.Method)
	}
	if filter.MakerPeerID != "" {
		query += " AND maker_peer_id = ?"
		args = append(args, filter.MakerPeerID)
	}
	if filter.TakerPeerID != "" {
		query += " AND taker_peer_id = ?"
		args = append(args, filter.TakerPeerID)
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list trades: %w", err)
	}
	defer rows.Close()

	var trades []*Trade
	for rows.Next() {
		var trade Trade
		var createdAt, updatedAt, completedAt sql.NullInt64
		var failureReason, makerPubKey, takerPubKey sql.NullString

		err := rows.Scan(
			&trade.ID, &trade.OrderID, &trade.MakerPeerID, &trade.TakerPeerID,
			&makerPubKey, &takerPubKey,
			&trade.OurRole, &trade.Method, &trade.State,
			&trade.OfferChain, &trade.OfferAmount,
			&trade.RequestChain, &trade.RequestAmount,
			&createdAt, &updatedAt, &completedAt, &failureReason,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trade: %w", err)
		}

		trade.CreatedAt = time.Unix(createdAt.Int64, 0)
		if updatedAt.Valid {
			t := time.Unix(updatedAt.Int64, 0)
			trade.UpdatedAt = &t
		}
		if completedAt.Valid {
			t := time.Unix(completedAt.Int64, 0)
			trade.CompletedAt = &t
		}
		if failureReason.Valid {
			trade.FailureReason = failureReason.String
		}
		if makerPubKey.Valid {
			trade.MakerPubKey = makerPubKey.String
		}
		if takerPubKey.Valid {
			trade.TakerPubKey = takerPubKey.String
		}

		trades = append(trades, &trade)
	}

	return trades, nil
}

// GetActiveTrades returns all trades in non-terminal states.
func (s *Storage) GetActiveTrades() ([]*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, order_id, maker_peer_id, taker_peer_id, maker_pubkey, taker_pubkey,
			our_role, method, state,
			offer_chain, offer_amount, request_chain, request_amount,
			created_at, updated_at, completed_at, failure_reason
		FROM trades
		WHERE state NOT IN (?, ?, ?, ?)
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query,
		TradeStateRedeemed, TradeStateRefunded, TradeStateFailed, TradeStateAborted)
	if err != nil {
		return nil, fmt.Errorf("failed to get active trades: %w", err)
	}
	defer rows.Close()

	var trades []*Trade
	for rows.Next() {
		var trade Trade
		var createdAt, updatedAt, completedAt sql.NullInt64
		var failureReason, makerPubKey, takerPubKey sql.NullString

		err := rows.Scan(
			&trade.ID, &trade.OrderID, &trade.MakerPeerID, &trade.TakerPeerID,
			&makerPubKey, &takerPubKey,
			&trade.OurRole, &trade.Method, &trade.State,
			&trade.OfferChain, &trade.OfferAmount,
			&trade.RequestChain, &trade.RequestAmount,
			&createdAt, &updatedAt, &completedAt, &failureReason,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trade: %w", err)
		}

		trade.CreatedAt = time.Unix(createdAt.Int64, 0)
		if updatedAt.Valid {
			t := time.Unix(updatedAt.Int64, 0)
			trade.UpdatedAt = &t
		}
		if completedAt.Valid {
			t := time.Unix(completedAt.Int64, 0)
			trade.CompletedAt = &t
		}
		if failureReason.Valid {
			trade.FailureReason = failureReason.String
		}
		if makerPubKey.Valid {
			trade.MakerPubKey = makerPubKey.String
		}
		if takerPubKey.Valid {
			trade.TakerPubKey = takerPubKey.String
		}

		trades = append(trades, &trade)
	}

	return trades, nil
}

// CountTrades returns the count of trades by state.
func (s *Storage) CountTrades(state *TradeState) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	var err error

	if state != nil {
		err = s.db.QueryRow("SELECT COUNT(*) FROM trades WHERE state = ?", *state).Scan(&count)
	} else {
		err = s.db.QueryRow("SELECT COUNT(*) FROM trades").Scan(&count)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to count trades: %w", err)
	}

	return count, nil
}

// DeleteTrade deletes a trade (use with caution - prefer state updates).
func (s *Storage) DeleteTrade(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM trades WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete trade: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTradeNotFound
	}

	return nil
}
