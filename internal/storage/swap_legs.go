// Package storage - Swap leg storage operations.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Swap leg errors
var (
	ErrSwapLegNotFound = errors.New("swap leg not found")
)

// SwapLegState represents the current state of a swap leg.
type SwapLegState string

const (
	SwapLegStateInit      SwapLegState = "init"      // Leg initialized
	SwapLegStatePending   SwapLegState = "pending"   // Waiting for funding
	SwapLegStateFunding   SwapLegState = "funding"   // Funding tx broadcast
	SwapLegStateFunded    SwapLegState = "funded"    // Funding confirmed
	SwapLegStateRedeemed  SwapLegState = "redeemed"  // Successfully redeemed
	SwapLegStateRefunded  SwapLegState = "refunded"  // Refunded after timeout
	SwapLegStateFailed    SwapLegState = "failed"    // Failed
)

// SwapLegType identifies which side of the swap.
type SwapLegType string

const (
	SwapLegTypeOffer   SwapLegType = "offer"   // The offer chain leg
	SwapLegTypeRequest SwapLegType = "request" // The request chain leg
)

// SwapLegRole indicates our role on this specific leg.
type SwapLegRole string

const (
	SwapLegRoleSender   SwapLegRole = "sender"   // We send funds on this leg
	SwapLegRoleReceiver SwapLegRole = "receiver" // We receive funds on this leg
)

// SwapLeg represents a swap leg in the database.
type SwapLeg struct {
	ID      string
	TradeID string

	// Leg identification
	LegType SwapLegType
	Chain   string
	Amount  uint64

	// Our role on this leg
	OurRole SwapLegRole

	// Leg state
	State SwapLegState

	// Funding transaction
	FundingTxID      string
	FundingVout      uint32
	FundingConfirms  uint32
	FundingAddress   string

	// Redeem/Refund transactions
	RedeemTxID string
	RefundTxID string

	// Timeout
	TimeoutHeight    uint32
	TimeoutTimestamp int64

	// Method-specific data (JSON blob)
	MethodData json.RawMessage

	// Timing
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// CreateSwapLeg creates a new swap leg in the database.
func (s *Storage) CreateSwapLeg(leg *SwapLeg) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var methodData *string
	if leg.MethodData != nil {
		md := string(leg.MethodData)
		methodData = &md
	}

	_, err := s.db.Exec(`
		INSERT INTO swap_legs (
			id, trade_id, leg_type, chain, amount, our_role, state,
			funding_address, timeout_height, timeout_timestamp,
			method_data, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		leg.ID, leg.TradeID, leg.LegType, leg.Chain, leg.Amount,
		leg.OurRole, leg.State, leg.FundingAddress,
		leg.TimeoutHeight, leg.TimeoutTimestamp,
		methodData, leg.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to create swap leg: %w", err)
	}

	return nil
}

// GetSwapLeg retrieves a swap leg by ID.
func (s *Storage) GetSwapLeg(id string) (*SwapLeg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getSwapLegLocked(id)
}

// getSwapLegLocked retrieves a swap leg without acquiring lock (internal use).
func (s *Storage) getSwapLegLocked(id string) (*SwapLeg, error) {
	var leg SwapLeg
	var fundingTxID, fundingAddress, redeemTxID, refundTxID, methodData sql.NullString
	var fundingVout, fundingConfirms, timeoutHeight sql.NullInt64
	var timeoutTimestamp sql.NullInt64
	var createdAt, updatedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, trade_id, leg_type, chain, amount, our_role, state,
			funding_txid, funding_vout, funding_confirms, funding_address,
			redeem_txid, refund_txid, timeout_height, timeout_timestamp,
			method_data, created_at, updated_at
		FROM swap_legs WHERE id = ?
	`, id).Scan(
		&leg.ID, &leg.TradeID, &leg.LegType, &leg.Chain, &leg.Amount,
		&leg.OurRole, &leg.State,
		&fundingTxID, &fundingVout, &fundingConfirms, &fundingAddress,
		&redeemTxID, &refundTxID, &timeoutHeight, &timeoutTimestamp,
		&methodData, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSwapLegNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get swap leg: %w", err)
	}

	// Convert nullable fields
	if fundingTxID.Valid {
		leg.FundingTxID = fundingTxID.String
	}
	if fundingVout.Valid {
		leg.FundingVout = uint32(fundingVout.Int64)
	}
	if fundingConfirms.Valid {
		leg.FundingConfirms = uint32(fundingConfirms.Int64)
	}
	if fundingAddress.Valid {
		leg.FundingAddress = fundingAddress.String
	}
	if redeemTxID.Valid {
		leg.RedeemTxID = redeemTxID.String
	}
	if refundTxID.Valid {
		leg.RefundTxID = refundTxID.String
	}
	if timeoutHeight.Valid {
		leg.TimeoutHeight = uint32(timeoutHeight.Int64)
	}
	if timeoutTimestamp.Valid {
		leg.TimeoutTimestamp = timeoutTimestamp.Int64
	}
	if methodData.Valid {
		leg.MethodData = json.RawMessage(methodData.String)
	}

	leg.CreatedAt = time.Unix(createdAt.Int64, 0)
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0)
		leg.UpdatedAt = &t
	}

	return &leg, nil
}

// GetSwapLegsByTradeID retrieves all swap legs for a trade.
func (s *Storage) GetSwapLegsByTradeID(tradeID string) ([]*SwapLeg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trade_id, leg_type, chain, amount, our_role, state,
			funding_txid, funding_vout, funding_confirms, funding_address,
			redeem_txid, refund_txid, timeout_height, timeout_timestamp,
			method_data, created_at, updated_at
		FROM swap_legs WHERE trade_id = ?
		ORDER BY leg_type
	`, tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap legs: %w", err)
	}
	defer rows.Close()

	return s.scanSwapLegs(rows)
}

// GetSwapLegByTradeAndType retrieves a specific swap leg by trade ID and leg type.
func (s *Storage) GetSwapLegByTradeAndType(tradeID string, legType SwapLegType) (*SwapLeg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var leg SwapLeg
	var fundingTxID, fundingAddress, redeemTxID, refundTxID, methodData sql.NullString
	var fundingVout, fundingConfirms, timeoutHeight sql.NullInt64
	var timeoutTimestamp sql.NullInt64
	var createdAt, updatedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, trade_id, leg_type, chain, amount, our_role, state,
			funding_txid, funding_vout, funding_confirms, funding_address,
			redeem_txid, refund_txid, timeout_height, timeout_timestamp,
			method_data, created_at, updated_at
		FROM swap_legs WHERE trade_id = ? AND leg_type = ?
	`, tradeID, legType).Scan(
		&leg.ID, &leg.TradeID, &leg.LegType, &leg.Chain, &leg.Amount,
		&leg.OurRole, &leg.State,
		&fundingTxID, &fundingVout, &fundingConfirms, &fundingAddress,
		&redeemTxID, &refundTxID, &timeoutHeight, &timeoutTimestamp,
		&methodData, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSwapLegNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get swap leg: %w", err)
	}

	// Convert nullable fields
	if fundingTxID.Valid {
		leg.FundingTxID = fundingTxID.String
	}
	if fundingVout.Valid {
		leg.FundingVout = uint32(fundingVout.Int64)
	}
	if fundingConfirms.Valid {
		leg.FundingConfirms = uint32(fundingConfirms.Int64)
	}
	if fundingAddress.Valid {
		leg.FundingAddress = fundingAddress.String
	}
	if redeemTxID.Valid {
		leg.RedeemTxID = redeemTxID.String
	}
	if refundTxID.Valid {
		leg.RefundTxID = refundTxID.String
	}
	if timeoutHeight.Valid {
		leg.TimeoutHeight = uint32(timeoutHeight.Int64)
	}
	if timeoutTimestamp.Valid {
		leg.TimeoutTimestamp = timeoutTimestamp.Int64
	}
	if methodData.Valid {
		leg.MethodData = json.RawMessage(methodData.String)
	}

	leg.CreatedAt = time.Unix(createdAt.Int64, 0)
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0)
		leg.UpdatedAt = &t
	}

	return &leg, nil
}

// UpdateSwapLegState updates the state of a swap leg.
func (s *Storage) UpdateSwapLegState(id string, state SwapLegState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs SET state = ?, updated_at = ? WHERE id = ?
	`, state, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg state: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// UpdateSwapLegFunding updates the funding information for a swap leg.
func (s *Storage) UpdateSwapLegFunding(id string, txID string, vout uint32, address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs
		SET funding_txid = ?, funding_vout = ?, funding_address = ?,
		    state = ?, updated_at = ?
		WHERE id = ?
	`, txID, vout, address, SwapLegStateFunding, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg funding: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// UpdateSwapLegConfirmations updates the funding confirmation count.
func (s *Storage) UpdateSwapLegConfirmations(id string, confirms uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs SET funding_confirms = ?, updated_at = ? WHERE id = ?
	`, confirms, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg confirmations: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// UpdateSwapLegRedeemed marks a swap leg as redeemed.
func (s *Storage) UpdateSwapLegRedeemed(id string, redeemTxID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs SET state = ?, redeem_txid = ?, updated_at = ? WHERE id = ?
	`, SwapLegStateRedeemed, redeemTxID, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg redeemed: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// UpdateSwapLegRefunded marks a swap leg as refunded.
func (s *Storage) UpdateSwapLegRefunded(id string, refundTxID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs SET state = ?, refund_txid = ?, updated_at = ? WHERE id = ?
	`, SwapLegStateRefunded, refundTxID, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg refunded: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// UpdateSwapLegMethodData updates the method-specific data for a swap leg.
func (s *Storage) UpdateSwapLegMethodData(id string, methodData json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE swap_legs SET method_data = ?, updated_at = ? WHERE id = ?
	`, string(methodData), time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update swap leg method data: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// SwapLegFilter defines filters for listing swap legs.
type SwapLegFilter struct {
	TradeID string
	Chain   string
	State   *SwapLegState
	OurRole *SwapLegRole
	LegType *SwapLegType
	Limit   int
	Offset  int
}

// ListSwapLegs returns swap legs matching the filter.
func (s *Storage) ListSwapLegs(filter SwapLegFilter) ([]*SwapLeg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, trade_id, leg_type, chain, amount, our_role, state,
			funding_txid, funding_vout, funding_confirms, funding_address,
			redeem_txid, refund_txid, timeout_height, timeout_timestamp,
			method_data, created_at, updated_at
		FROM swap_legs WHERE 1=1
	`
	args := []interface{}{}

	if filter.TradeID != "" {
		query += " AND trade_id = ?"
		args = append(args, filter.TradeID)
	}
	if filter.Chain != "" {
		query += " AND chain = ?"
		args = append(args, filter.Chain)
	}
	if filter.State != nil {
		query += " AND state = ?"
		args = append(args, *filter.State)
	}
	if filter.OurRole != nil {
		query += " AND our_role = ?"
		args = append(args, *filter.OurRole)
	}
	if filter.LegType != nil {
		query += " AND leg_type = ?"
		args = append(args, *filter.LegType)
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
		return nil, fmt.Errorf("failed to list swap legs: %w", err)
	}
	defer rows.Close()

	return s.scanSwapLegs(rows)
}

// GetPendingSwapLegs returns all swap legs that need monitoring.
func (s *Storage) GetPendingSwapLegs() ([]*SwapLeg, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trade_id, leg_type, chain, amount, our_role, state,
			funding_txid, funding_vout, funding_confirms, funding_address,
			redeem_txid, refund_txid, timeout_height, timeout_timestamp,
			method_data, created_at, updated_at
		FROM swap_legs
		WHERE state NOT IN (?, ?, ?)
		ORDER BY created_at ASC
	`, SwapLegStateRedeemed, SwapLegStateRefunded, SwapLegStateFailed)

	if err != nil {
		return nil, fmt.Errorf("failed to get pending swap legs: %w", err)
	}
	defer rows.Close()

	return s.scanSwapLegs(rows)
}

// scanSwapLegs scans rows into swap leg structs.
func (s *Storage) scanSwapLegs(rows *sql.Rows) ([]*SwapLeg, error) {
	var legs []*SwapLeg

	for rows.Next() {
		var leg SwapLeg
		var fundingTxID, fundingAddress, redeemTxID, refundTxID, methodData sql.NullString
		var fundingVout, fundingConfirms, timeoutHeight sql.NullInt64
		var timeoutTimestamp sql.NullInt64
		var createdAt, updatedAt sql.NullInt64

		err := rows.Scan(
			&leg.ID, &leg.TradeID, &leg.LegType, &leg.Chain, &leg.Amount,
			&leg.OurRole, &leg.State,
			&fundingTxID, &fundingVout, &fundingConfirms, &fundingAddress,
			&redeemTxID, &refundTxID, &timeoutHeight, &timeoutTimestamp,
			&methodData, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan swap leg: %w", err)
		}

		if fundingTxID.Valid {
			leg.FundingTxID = fundingTxID.String
		}
		if fundingVout.Valid {
			leg.FundingVout = uint32(fundingVout.Int64)
		}
		if fundingConfirms.Valid {
			leg.FundingConfirms = uint32(fundingConfirms.Int64)
		}
		if fundingAddress.Valid {
			leg.FundingAddress = fundingAddress.String
		}
		if redeemTxID.Valid {
			leg.RedeemTxID = redeemTxID.String
		}
		if refundTxID.Valid {
			leg.RefundTxID = refundTxID.String
		}
		if timeoutHeight.Valid {
			leg.TimeoutHeight = uint32(timeoutHeight.Int64)
		}
		if timeoutTimestamp.Valid {
			leg.TimeoutTimestamp = timeoutTimestamp.Int64
		}
		if methodData.Valid {
			leg.MethodData = json.RawMessage(methodData.String)
		}

		leg.CreatedAt = time.Unix(createdAt.Int64, 0)
		if updatedAt.Valid {
			t := time.Unix(updatedAt.Int64, 0)
			leg.UpdatedAt = &t
		}

		legs = append(legs, &leg)
	}

	return legs, nil
}

// DeleteSwapLeg deletes a swap leg.
func (s *Storage) DeleteSwapLeg(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM swap_legs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete swap leg: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSwapLegNotFound
	}

	return nil
}

// DeleteSwapLegsByTradeID deletes all swap legs for a trade.
func (s *Storage) DeleteSwapLegsByTradeID(tradeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM swap_legs WHERE trade_id = ?", tradeID)
	if err != nil {
		return fmt.Errorf("failed to delete swap legs: %w", err)
	}

	return nil
}
