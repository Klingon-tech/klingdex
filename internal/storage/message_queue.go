// Package storage provides persistent storage using SQLite.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// Message Status Constants
// =============================================================================

// OutboxStatus represents the status of an outbound message.
type OutboxStatus string

const (
	OutboxStatusPending OutboxStatus = "pending" // Awaiting delivery
	OutboxStatusSent    OutboxStatus = "sent"    // Sent, awaiting ACK
	OutboxStatusAcked   OutboxStatus = "acked"   // Successfully delivered
	OutboxStatusFailed  OutboxStatus = "failed"  // Permanently failed
	OutboxStatusExpired OutboxStatus = "expired" // Swap expired before delivery
)

// =============================================================================
// Outbox Message Types
// =============================================================================

// OutboxMessage represents a message in the outbound queue.
type OutboxMessage struct {
	ID           int64        `json:"id"`
	MessageID    string       `json:"message_id"`
	TradeID      string       `json:"trade_id"`
	PeerID       string       `json:"peer_id"`
	MessageType  string       `json:"message_type"`
	Payload      []byte       `json:"payload"`
	SequenceNum  uint64       `json:"sequence_num"`
	SwapTimeout  int64        `json:"swap_timeout"`
	CreatedAt    int64        `json:"created_at"`
	RetryCount   int          `json:"retry_count"`
	LastAttempt  int64        `json:"last_attempt_at"`
	NextRetryAt  int64        `json:"next_retry_at"`
	AckedAt      *int64       `json:"acked_at"`
	Status       OutboxStatus `json:"status"`
	ErrorMessage string       `json:"error_message"`
}

// InboxMessage represents a received message for deduplication.
type InboxMessage struct {
	ID          int64  `json:"id"`
	MessageID   string `json:"message_id"`
	TradeID     string `json:"trade_id"`
	PeerID      string `json:"peer_id"`
	MessageType string `json:"message_type"`
	SequenceNum uint64 `json:"sequence_num"`
	ReceivedAt  int64  `json:"received_at"`
	ProcessedAt *int64 `json:"processed_at"`
	AckSent     bool   `json:"ack_sent"`
}

// MessageSequence tracks sequence numbers for a trade.
type MessageSequence struct {
	TradeID   string `json:"trade_id"`
	LocalSeq  uint64 `json:"local_seq"`
	RemoteSeq uint64 `json:"remote_seq"`
	UpdatedAt int64  `json:"updated_at"`
}

// =============================================================================
// Outbox Operations
// =============================================================================

// EnqueueMessage adds a message to the outbox for delivery.
func (s *Storage) EnqueueMessage(msg *OutboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		INSERT INTO message_outbox (
			message_id, trade_id, peer_id, message_type, payload, sequence_num,
			swap_timeout, created_at, retry_count, next_retry_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 'pending')
	`,
		msg.MessageID, msg.TradeID, msg.PeerID, msg.MessageType, msg.Payload,
		msg.SequenceNum, msg.SwapTimeout, now, now,
	)

	if err != nil {
		return fmt.Errorf("failed to enqueue message: %w", err)
	}

	return nil
}

// GetPendingMessages returns messages due for retry.
func (s *Storage) GetPendingMessages(now int64) ([]*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, message_id, trade_id, peer_id, message_type, payload, sequence_num,
		       swap_timeout, created_at, retry_count, last_attempt_at, next_retry_at,
		       acked_at, status, error_message
		FROM message_outbox
		WHERE (status = 'pending' OR status = 'sent')
		  AND next_retry_at <= ?
		ORDER BY next_retry_at ASC
		LIMIT 100
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending messages: %w", err)
	}
	defer rows.Close()

	return scanOutboxMessages(rows)
}

// GetPendingForPeer returns pending messages for a specific peer.
func (s *Storage) GetPendingForPeer(peerID string) ([]*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, message_id, trade_id, peer_id, message_type, payload, sequence_num,
		       swap_timeout, created_at, retry_count, last_attempt_at, next_retry_at,
		       acked_at, status, error_message
		FROM message_outbox
		WHERE peer_id = ?
		  AND (status = 'pending' OR status = 'sent')
		ORDER BY sequence_num ASC
	`, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages for peer: %w", err)
	}
	defer rows.Close()

	return scanOutboxMessages(rows)
}

// GetPendingForTrade returns pending messages for a specific trade.
func (s *Storage) GetPendingForTrade(tradeID string) ([]*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, message_id, trade_id, peer_id, message_type, payload, sequence_num,
		       swap_timeout, created_at, retry_count, last_attempt_at, next_retry_at,
		       acked_at, status, error_message
		FROM message_outbox
		WHERE trade_id = ?
		  AND (status = 'pending' OR status = 'sent')
		ORDER BY sequence_num ASC
	`, tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages for trade: %w", err)
	}
	defer rows.Close()

	return scanOutboxMessages(rows)
}

// MarkMessageSent marks a message as sent (awaiting ACK).
func (s *Storage) MarkMessageSent(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'sent', last_attempt_at = ?, retry_count = retry_count + 1
		WHERE message_id = ?
	`, now, messageID)

	return err
}

// MarkMessageAcked marks a message as successfully delivered.
func (s *Storage) MarkMessageAcked(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'acked', acked_at = ?
		WHERE message_id = ?
	`, now, messageID)

	return err
}

// MarkMessageFailed marks a message as permanently failed.
func (s *Storage) MarkMessageFailed(messageID string, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'failed', error_message = ?
		WHERE message_id = ?
	`, errorMsg, messageID)

	return err
}

// MarkMessageExpired marks a message as expired (swap timed out).
func (s *Storage) MarkMessageExpired(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'expired', error_message = 'swap timeout expired'
		WHERE message_id = ?
	`, messageID)

	return err
}

// ScheduleRetry schedules a message for retry at the given time.
func (s *Storage) ScheduleRetry(messageID string, nextRetryAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'pending', next_retry_at = ?
		WHERE message_id = ?
	`, nextRetryAt, messageID)

	return err
}

// ExpireOldMessages marks messages for expired swaps.
func (s *Storage) ExpireOldMessages(now int64, bufferSeconds int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Expire messages where swap_timeout - buffer has passed
	deadline := now + bufferSeconds

	_, err := s.db.Exec(`
		UPDATE message_outbox
		SET status = 'expired', error_message = 'swap timeout approaching'
		WHERE (status = 'pending' OR status = 'sent')
		  AND swap_timeout <= ?
	`, deadline)

	return err
}

// CleanupOldMessages removes old completed/failed messages.
func (s *Storage) CleanupOldMessages(olderThan int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM message_outbox
		WHERE status IN ('acked', 'failed', 'expired')
		  AND created_at < ?
	`, olderThan)

	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// GetOutboxStats returns statistics about the outbox.
func (s *Storage) GetOutboxStats() (map[OutboxStatus]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT status, COUNT(*) as count
		FROM message_outbox
		GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[OutboxStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[OutboxStatus(status)] = count
	}

	return stats, nil
}

// GetOutboxMessage retrieves a single outbox message by message ID.
func (s *Storage) GetOutboxMessage(messageID string) (*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var msg OutboxMessage
	var lastAttempt, ackedAt sql.NullInt64
	var errorMsg sql.NullString

	err := s.db.QueryRow(`
		SELECT id, message_id, trade_id, peer_id, message_type, payload, sequence_num,
			   swap_timeout, created_at, retry_count, last_attempt_at, next_retry_at,
			   acked_at, status, error_message
		FROM message_outbox
		WHERE message_id = ?
	`, messageID).Scan(
		&msg.ID, &msg.MessageID, &msg.TradeID, &msg.PeerID, &msg.MessageType,
		&msg.Payload, &msg.SequenceNum, &msg.SwapTimeout, &msg.CreatedAt,
		&msg.RetryCount, &lastAttempt, &msg.NextRetryAt, &ackedAt,
		&msg.Status, &errorMsg,
	)

	if err != nil {
		return nil, err
	}

	if lastAttempt.Valid {
		msg.LastAttempt = lastAttempt.Int64
	}
	if ackedAt.Valid {
		msg.AckedAt = &ackedAt.Int64
	}
	if errorMsg.Valid {
		msg.ErrorMessage = errorMsg.String
	}

	return &msg, nil
}

// =============================================================================
// Inbox Operations (for deduplication)
// =============================================================================

// HasReceivedMessage checks if a message was already received.
func (s *Storage) HasReceivedMessage(messageID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM message_inbox WHERE message_id = ?
	`, messageID).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// RecordReceivedMessage records a received message for deduplication.
func (s *Storage) RecordReceivedMessage(msg *InboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO message_inbox (
			message_id, trade_id, peer_id, message_type, sequence_num, received_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		msg.MessageID, msg.TradeID, msg.PeerID, msg.MessageType,
		msg.SequenceNum, now,
	)

	return err
}

// MarkMessageProcessed marks an inbox message as processed.
func (s *Storage) MarkMessageProcessed(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		UPDATE message_inbox
		SET processed_at = ?
		WHERE message_id = ?
	`, now, messageID)

	return err
}

// MarkAckSent marks that an ACK was sent for this message.
func (s *Storage) MarkAckSent(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE message_inbox
		SET ack_sent = 1
		WHERE message_id = ?
	`, messageID)

	return err
}

// GetInboxMessage retrieves an inbox message by ID.
func (s *Storage) GetInboxMessage(messageID string) (*InboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var msg InboxMessage
	var processedAt sql.NullInt64
	var ackSent int

	err := s.db.QueryRow(`
		SELECT id, message_id, trade_id, peer_id, message_type, sequence_num,
		       received_at, processed_at, ack_sent
		FROM message_inbox
		WHERE message_id = ?
	`, messageID).Scan(
		&msg.ID, &msg.MessageID, &msg.TradeID, &msg.PeerID, &msg.MessageType,
		&msg.SequenceNum, &msg.ReceivedAt, &processedAt, &ackSent,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if processedAt.Valid {
		msg.ProcessedAt = &processedAt.Int64
	}
	msg.AckSent = ackSent == 1

	return &msg, nil
}

// CleanupOldInboxMessages removes old inbox entries.
func (s *Storage) CleanupOldInboxMessages(olderThan int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM message_inbox
		WHERE received_at < ?
	`, olderThan)

	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// =============================================================================
// Sequence Number Operations
// =============================================================================

// GetNextLocalSequence gets and increments the local sequence for a trade.
func (s *Storage) GetNextLocalSequence(tradeID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// Try to increment existing sequence
	result, err := s.db.Exec(`
		UPDATE message_sequences
		SET local_seq = local_seq + 1, updated_at = ?
		WHERE trade_id = ?
	`, now, tradeID)
	if err != nil {
		return 0, err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		// Create new sequence entry
		_, err = s.db.Exec(`
			INSERT INTO message_sequences (trade_id, local_seq, remote_seq, updated_at)
			VALUES (?, 1, 0, ?)
		`, tradeID, now)
		if err != nil {
			return 0, err
		}
		return 1, nil
	}

	// Get the new value
	var seq uint64
	err = s.db.QueryRow(`
		SELECT local_seq FROM message_sequences WHERE trade_id = ?
	`, tradeID).Scan(&seq)

	return seq, err
}

// UpdateRemoteSequence updates the last received sequence number.
func (s *Storage) UpdateRemoteSequence(tradeID string, seq uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// Upsert the sequence
	_, err := s.db.Exec(`
		INSERT INTO message_sequences (trade_id, local_seq, remote_seq, updated_at)
		VALUES (?, 0, ?, ?)
		ON CONFLICT(trade_id) DO UPDATE SET
			remote_seq = MAX(remote_seq, excluded.remote_seq),
			updated_at = excluded.updated_at
	`, tradeID, seq, now)

	return err
}

// GetSequences returns sequence numbers for a trade.
func (s *Storage) GetSequences(tradeID string) (*MessageSequence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var seq MessageSequence
	err := s.db.QueryRow(`
		SELECT trade_id, local_seq, remote_seq, updated_at
		FROM message_sequences
		WHERE trade_id = ?
	`, tradeID).Scan(&seq.TradeID, &seq.LocalSeq, &seq.RemoteSeq, &seq.UpdatedAt)

	if err == sql.ErrNoRows {
		return &MessageSequence{TradeID: tradeID}, nil
	}
	if err != nil {
		return nil, err
	}

	return &seq, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func scanOutboxMessages(rows *sql.Rows) ([]*OutboxMessage, error) {
	var messages []*OutboxMessage

	for rows.Next() {
		var msg OutboxMessage
		var lastAttempt, ackedAt sql.NullInt64
		var errorMsg sql.NullString

		err := rows.Scan(
			&msg.ID, &msg.MessageID, &msg.TradeID, &msg.PeerID, &msg.MessageType,
			&msg.Payload, &msg.SequenceNum, &msg.SwapTimeout, &msg.CreatedAt,
			&msg.RetryCount, &lastAttempt, &msg.NextRetryAt, &ackedAt,
			&msg.Status, &errorMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox message: %w", err)
		}

		if lastAttempt.Valid {
			msg.LastAttempt = lastAttempt.Int64
		}
		if ackedAt.Valid {
			msg.AckedAt = &ackedAt.Int64
		}
		if errorMsg.Valid {
			msg.ErrorMessage = errorMsg.String
		}

		messages = append(messages, &msg)
	}

	return messages, rows.Err()
}

// ToJSON converts an OutboxMessage payload to the original message type.
func (m *OutboxMessage) ToJSON(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}
