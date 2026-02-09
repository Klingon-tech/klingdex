// Package storage - Secret storage operations for HTLC swaps.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Secret errors
var (
	ErrSecretNotFound      = errors.New("secret not found")
	ErrSecretAlreadyExists = errors.New("secret already exists for this trade and hash")
)

// SecretCreator indicates who created the secret.
type SecretCreator string

const (
	SecretCreatorUs   SecretCreator = "us"   // We created the secret
	SecretCreatorThem SecretCreator = "them" // Counterparty created the secret
)

// Secret represents an HTLC secret in the database.
type Secret struct {
	ID        string
	TradeID   string

	// The secret hash (always known - SHA256 of secret)
	SecretHash string // 32 bytes, hex-encoded

	// The secret itself (only after reveal)
	Secret string // 32 bytes, hex-encoded

	// Who created this secret
	CreatedBy SecretCreator

	// Remote wallet addresses (received with secret hash in P2P message)
	// These are the counterparty's addresses for receiving funds
	RemoteOfferWalletAddr   string // Counterparty's address on offer chain
	RemoteRequestWalletAddr string // Counterparty's address on request chain

	// Timing
	CreatedAt  time.Time
	RevealedAt *time.Time
}

// CreateSecret creates a new secret entry in the database.
// The Secret field may be empty if we don't know the preimage yet.
func (s *Storage) CreateSecret(secret *Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var secretValue *string
	if secret.Secret != "" {
		secretValue = &secret.Secret
	}

	var revealedAt *int64
	if secret.RevealedAt != nil {
		ts := secret.RevealedAt.Unix()
		revealedAt = &ts
	}

	var remoteOfferAddr, remoteRequestAddr *string
	if secret.RemoteOfferWalletAddr != "" {
		remoteOfferAddr = &secret.RemoteOfferWalletAddr
	}
	if secret.RemoteRequestWalletAddr != "" {
		remoteRequestAddr = &secret.RemoteRequestWalletAddr
	}

	_, err := s.db.Exec(`
		INSERT INTO secrets (
			id, trade_id, secret_hash, secret, created_by,
			remote_offer_wallet_addr, remote_request_wallet_addr,
			created_at, revealed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		secret.ID, secret.TradeID, secret.SecretHash, secretValue,
		secret.CreatedBy, remoteOfferAddr, remoteRequestAddr,
		secret.CreatedAt.Unix(), revealedAt,
	)

	if err != nil {
		// Check for unique constraint violation
		if isUniqueConstraintError(err) {
			return ErrSecretAlreadyExists
		}
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// GetSecret retrieves a secret by ID.
func (s *Storage) GetSecret(id string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secret Secret
	var secretValue, remoteOfferAddr, remoteRequestAddr sql.NullString
	var createdAt, revealedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, trade_id, secret_hash, secret, created_by,
			   remote_offer_wallet_addr, remote_request_wallet_addr,
			   created_at, revealed_at
		FROM secrets WHERE id = ?
	`, id).Scan(
		&secret.ID, &secret.TradeID, &secret.SecretHash, &secretValue,
		&secret.CreatedBy, &remoteOfferAddr, &remoteRequestAddr,
		&createdAt, &revealedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	if secretValue.Valid {
		secret.Secret = secretValue.String
	}
	if remoteOfferAddr.Valid {
		secret.RemoteOfferWalletAddr = remoteOfferAddr.String
	}
	if remoteRequestAddr.Valid {
		secret.RemoteRequestWalletAddr = remoteRequestAddr.String
	}
	secret.CreatedAt = time.Unix(createdAt.Int64, 0)
	if revealedAt.Valid {
		t := time.Unix(revealedAt.Int64, 0)
		secret.RevealedAt = &t
	}

	return &secret, nil
}

// GetSecretByHash retrieves a secret by its hash.
func (s *Storage) GetSecretByHash(secretHash string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secret Secret
	var secretValue, remoteOfferAddr, remoteRequestAddr sql.NullString
	var createdAt, revealedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, trade_id, secret_hash, secret, created_by,
			   remote_offer_wallet_addr, remote_request_wallet_addr,
			   created_at, revealed_at
		FROM secrets WHERE secret_hash = ?
	`, secretHash).Scan(
		&secret.ID, &secret.TradeID, &secret.SecretHash, &secretValue,
		&secret.CreatedBy, &remoteOfferAddr, &remoteRequestAddr,
		&createdAt, &revealedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get secret by hash: %w", err)
	}

	if secretValue.Valid {
		secret.Secret = secretValue.String
	}
	if remoteOfferAddr.Valid {
		secret.RemoteOfferWalletAddr = remoteOfferAddr.String
	}
	if remoteRequestAddr.Valid {
		secret.RemoteRequestWalletAddr = remoteRequestAddr.String
	}
	secret.CreatedAt = time.Unix(createdAt.Int64, 0)
	if revealedAt.Valid {
		t := time.Unix(revealedAt.Int64, 0)
		secret.RevealedAt = &t
	}

	return &secret, nil
}

// GetSecretByTradeID retrieves the secret for a trade.
func (s *Storage) GetSecretByTradeID(tradeID string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secret Secret
	var secretValue, remoteOfferAddr, remoteRequestAddr sql.NullString
	var createdAt, revealedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, trade_id, secret_hash, secret, created_by,
			   remote_offer_wallet_addr, remote_request_wallet_addr,
			   created_at, revealed_at
		FROM secrets WHERE trade_id = ?
	`, tradeID).Scan(
		&secret.ID, &secret.TradeID, &secret.SecretHash, &secretValue,
		&secret.CreatedBy, &remoteOfferAddr, &remoteRequestAddr,
		&createdAt, &revealedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get secret by trade: %w", err)
	}

	if secretValue.Valid {
		secret.Secret = secretValue.String
	}
	if remoteOfferAddr.Valid {
		secret.RemoteOfferWalletAddr = remoteOfferAddr.String
	}
	if remoteRequestAddr.Valid {
		secret.RemoteRequestWalletAddr = remoteRequestAddr.String
	}
	secret.CreatedAt = time.Unix(createdAt.Int64, 0)
	if revealedAt.Valid {
		t := time.Unix(revealedAt.Int64, 0)
		secret.RevealedAt = &t
	}

	return &secret, nil
}

// RevealSecret updates the secret preimage when it's discovered.
func (s *Storage) RevealSecret(id string, preimage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	result, err := s.db.Exec(`
		UPDATE secrets SET secret = ?, revealed_at = ? WHERE id = ? AND secret IS NULL
	`, preimage, now, id)

	if err != nil {
		return fmt.Errorf("failed to reveal secret: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Either not found or already revealed
		var existing sql.NullString
		err := s.db.QueryRow("SELECT secret FROM secrets WHERE id = ?", id).Scan(&existing)
		if err == sql.ErrNoRows {
			return ErrSecretNotFound
		}
		// Already has a secret - that's fine, operation is idempotent
		return nil
	}

	return nil
}

// RevealSecretByHash updates the secret preimage when discovered via hash lookup.
func (s *Storage) RevealSecretByHash(secretHash string, preimage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	result, err := s.db.Exec(`
		UPDATE secrets SET secret = ?, revealed_at = ?
		WHERE secret_hash = ? AND secret IS NULL
	`, preimage, now, secretHash)

	if err != nil {
		return fmt.Errorf("failed to reveal secret by hash: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Check if it exists
		var existing sql.NullString
		err := s.db.QueryRow("SELECT secret FROM secrets WHERE secret_hash = ?", secretHash).Scan(&existing)
		if err == sql.ErrNoRows {
			return ErrSecretNotFound
		}
		// Already has a secret - idempotent
		return nil
	}

	return nil
}

// ListSecretsByTrade returns all secrets for a trade.
func (s *Storage) ListSecretsByTrade(tradeID string) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trade_id, secret_hash, secret, created_by, created_at, revealed_at
		FROM secrets WHERE trade_id = ?
		ORDER BY created_at
	`, tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var secret Secret
		var secretValue sql.NullString
		var createdAt, revealedAt sql.NullInt64

		err := rows.Scan(
			&secret.ID, &secret.TradeID, &secret.SecretHash, &secretValue,
			&secret.CreatedBy, &createdAt, &revealedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan secret: %w", err)
		}

		if secretValue.Valid {
			secret.Secret = secretValue.String
		}
		secret.CreatedAt = time.Unix(createdAt.Int64, 0)
		if revealedAt.Valid {
			t := time.Unix(revealedAt.Int64, 0)
			secret.RevealedAt = &t
		}

		secrets = append(secrets, &secret)
	}

	return secrets, nil
}

// GetUnrevealedSecrets returns secrets where we know the preimage but haven't revealed yet.
// This is used by us (as secret creator) to track which secrets we've generated.
func (s *Storage) GetUnrevealedSecrets() ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trade_id, secret_hash, secret, created_by, created_at, revealed_at
		FROM secrets
		WHERE created_by = ? AND secret IS NOT NULL AND revealed_at IS NULL
		ORDER BY created_at
	`, SecretCreatorUs)
	if err != nil {
		return nil, fmt.Errorf("failed to get unrevealed secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var secret Secret
		var secretValue sql.NullString
		var createdAt, revealedAt sql.NullInt64

		err := rows.Scan(
			&secret.ID, &secret.TradeID, &secret.SecretHash, &secretValue,
			&secret.CreatedBy, &createdAt, &revealedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan secret: %w", err)
		}

		if secretValue.Valid {
			secret.Secret = secretValue.String
		}
		secret.CreatedAt = time.Unix(createdAt.Int64, 0)
		if revealedAt.Valid {
			t := time.Unix(revealedAt.Int64, 0)
			secret.RevealedAt = &t
		}

		secrets = append(secrets, &secret)
	}

	return secrets, nil
}

// HasSecretPreimage checks if we have the preimage for a secret hash.
func (s *Storage) HasSecretPreimage(secretHash string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var secret sql.NullString
	err := s.db.QueryRow(`
		SELECT secret FROM secrets WHERE secret_hash = ?
	`, secretHash).Scan(&secret)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check secret preimage: %w", err)
	}

	return secret.Valid && secret.String != "", nil
}

// DeleteSecret deletes a secret.
func (s *Storage) DeleteSecret(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM secrets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrSecretNotFound
	}

	return nil
}

// DeleteSecretsByTradeID deletes all secrets for a trade.
func (s *Storage) DeleteSecretsByTradeID(tradeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM secrets WHERE trade_id = ?", tradeID)
	if err != nil {
		return fmt.Errorf("failed to delete secrets: %w", err)
	}

	return nil
}

// isUniqueConstraintError checks if an error is a SQLite unique constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return contains(err.Error(), "UNIQUE constraint failed")
}

// contains checks if a string contains a substring (simple implementation).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
