// Package storage - Order storage operations.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Order errors
var (
	ErrOrderNotFound = errors.New("order not found")
	ErrOrderExpired  = errors.New("order expired")
)

// OrderStatus represents the status of an order.
type OrderStatus string

const (
	OrderStatusOpen      OrderStatus = "open"
	OrderStatusMatched   OrderStatus = "matched"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusExpired   OrderStatus = "expired"
	OrderStatusFailed    OrderStatus = "failed"
)

// Order represents a trade order in the database.
// Price is implicit: offer_amount/request_amount ratio.
type Order struct {
	ID        string
	PeerID    string
	Status    OrderStatus
	IsLocal   bool // True if this is our order

	// Trading pair (price is implicit ratio)
	OfferChain    string
	OfferAmount   uint64
	RequestChain  string
	RequestAmount uint64

	// Preferred swap methods in priority order
	PreferredMethods []string

	// Timing
	CreatedAt time.Time
	ExpiresAt *time.Time
	UpdatedAt *time.Time

	// Ownership proof
	Signature string
}

// CreateOrder creates a new order in the database.
func (s *Storage) CreateOrder(order *Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	methodsJSON, err := json.Marshal(order.PreferredMethods)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred methods: %w", err)
	}

	var expiresAt *int64
	if order.ExpiresAt != nil {
		ts := order.ExpiresAt.Unix()
		expiresAt = &ts
	}

	isLocal := 0
	if order.IsLocal {
		isLocal = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO orders (
			id, peer_id, status, offer_chain, offer_amount,
			request_chain, request_amount, preferred_methods,
			created_at, expires_at, is_local, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		order.ID, order.PeerID, order.Status,
		order.OfferChain, order.OfferAmount,
		order.RequestChain, order.RequestAmount,
		string(methodsJSON),
		order.CreatedAt.Unix(), expiresAt,
		isLocal, order.Signature,
	)

	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// SaveOrder saves an order (insert or update).
// This is used for syncing orders from other peers.
func (s *Storage) SaveOrder(order *Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	methodsJSON, err := json.Marshal(order.PreferredMethods)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred methods: %w", err)
	}

	var expiresAt *int64
	if order.ExpiresAt != nil {
		ts := order.ExpiresAt.Unix()
		expiresAt = &ts
	}

	isLocal := 0
	if order.IsLocal {
		isLocal = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO orders (
			id, peer_id, status, offer_chain, offer_amount,
			request_chain, request_amount, preferred_methods,
			created_at, expires_at, is_local, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			expires_at = excluded.expires_at
	`,
		order.ID, order.PeerID, order.Status,
		order.OfferChain, order.OfferAmount,
		order.RequestChain, order.RequestAmount,
		string(methodsJSON),
		order.CreatedAt.Unix(), expiresAt,
		isLocal, order.Signature,
	)

	if err != nil {
		return fmt.Errorf("failed to save order: %w", err)
	}

	return nil
}

// GetOrder retrieves an order by ID.
func (s *Storage) GetOrder(id string) (*Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var order Order
	var methodsJSON string
	var createdAt, expiresAt, updatedAt sql.NullInt64
	var isLocal int

	err := s.db.QueryRow(`
		SELECT id, peer_id, status, offer_chain, offer_amount,
			request_chain, request_amount, preferred_methods,
			created_at, expires_at, updated_at, is_local, signature
		FROM orders WHERE id = ?
	`, id).Scan(
		&order.ID, &order.PeerID, &order.Status,
		&order.OfferChain, &order.OfferAmount,
		&order.RequestChain, &order.RequestAmount,
		&methodsJSON,
		&createdAt, &expiresAt, &updatedAt,
		&isLocal, &order.Signature,
	)

	if err == sql.ErrNoRows {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Parse JSON fields
	if err := json.Unmarshal([]byte(methodsJSON), &order.PreferredMethods); err != nil {
		return nil, fmt.Errorf("failed to parse preferred methods: %w", err)
	}

	// Convert timestamps
	order.CreatedAt = time.Unix(createdAt.Int64, 0)
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		order.ExpiresAt = &t
	}
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0)
		order.UpdatedAt = &t
	}
	order.IsLocal = isLocal == 1

	return &order, nil
}

// UpdateOrderStatus updates the status of an order.
func (s *Storage) UpdateOrderStatus(id string, status OrderStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE orders SET status = ?, updated_at = ? WHERE id = ?
	`, status, time.Now().Unix(), id)

	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}

	return nil
}

// ListOrders lists orders matching the given filters.
type OrderFilter struct {
	Status       *OrderStatus
	OfferChain   string
	RequestChain string
	PeerID       string
	IsLocal      *bool
	Limit        int
	Offset       int
}

// ListOrders returns orders matching the filter.
func (s *Storage) ListOrders(filter OrderFilter) ([]*Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, peer_id, status, offer_chain, offer_amount,
			request_chain, request_amount, preferred_methods,
			created_at, expires_at, updated_at, is_local, signature
		FROM orders WHERE 1=1
	`
	args := []interface{}{}

	if filter.Status != nil {
		query += " AND status = ?"
		args = append(args, *filter.Status)
	}
	if filter.OfferChain != "" {
		query += " AND offer_chain = ?"
		args = append(args, filter.OfferChain)
	}
	if filter.RequestChain != "" {
		query += " AND request_chain = ?"
		args = append(args, filter.RequestChain)
	}
	if filter.PeerID != "" {
		query += " AND peer_id = ?"
		args = append(args, filter.PeerID)
	}
	if filter.IsLocal != nil {
		isLocal := 0
		if *filter.IsLocal {
			isLocal = 1
		}
		query += " AND is_local = ?"
		args = append(args, isLocal)
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
		return nil, fmt.Errorf("failed to list orders: %w", err)
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		var order Order
		var methodsJSON string
		var createdAt, expiresAt, updatedAt sql.NullInt64
		var isLocal int

		err := rows.Scan(
			&order.ID, &order.PeerID, &order.Status,
			&order.OfferChain, &order.OfferAmount,
			&order.RequestChain, &order.RequestAmount,
			&methodsJSON,
			&createdAt, &expiresAt, &updatedAt,
			&isLocal, &order.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}

		if err := json.Unmarshal([]byte(methodsJSON), &order.PreferredMethods); err != nil {
			return nil, fmt.Errorf("failed to parse preferred methods: %w", err)
		}

		order.CreatedAt = time.Unix(createdAt.Int64, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			order.ExpiresAt = &t
		}
		if updatedAt.Valid {
			t := time.Unix(updatedAt.Int64, 0)
			order.UpdatedAt = &t
		}
		order.IsLocal = isLocal == 1

		orders = append(orders, &order)
	}

	return orders, nil
}

// GetOpenOrders returns all open orders for a trading pair.
func (s *Storage) GetOpenOrders(offerChain, requestChain string) ([]*Order, error) {
	status := OrderStatusOpen
	return s.ListOrders(OrderFilter{
		Status:       &status,
		OfferChain:   offerChain,
		RequestChain: requestChain,
	})
}

// GetMyOrders returns all orders created by us.
func (s *Storage) GetMyOrders() ([]*Order, error) {
	isLocal := true
	return s.ListOrders(OrderFilter{
		IsLocal: &isLocal,
	})
}

// DeleteOrder deletes an order (use with caution).
func (s *Storage) DeleteOrder(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM orders WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete order: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrOrderNotFound
	}

	return nil
}

// ExpireOldOrders marks expired orders as expired.
func (s *Storage) ExpireOldOrders() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE orders
		SET status = ?, updated_at = ?
		WHERE status = ? AND expires_at IS NOT NULL AND expires_at < ?
	`, OrderStatusExpired, time.Now().Unix(), OrderStatusOpen, time.Now().Unix())

	if err != nil {
		return 0, fmt.Errorf("failed to expire orders: %w", err)
	}

	return result.RowsAffected()
}

// CountOrders returns the count of orders by status.
func (s *Storage) CountOrders(status *OrderStatus) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	var err error

	if status != nil {
		err = s.db.QueryRow("SELECT COUNT(*) FROM orders WHERE status = ?", *status).Scan(&count)
	} else {
		err = s.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to count orders: %w", err)
	}

	return count, nil
}
