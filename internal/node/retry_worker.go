// Package node - Background worker for retrying undelivered messages.
package node

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/Klingon-tech/klingdex/internal/storage"
	"github.com/Klingon-tech/klingdex/pkg/logging"
)

// RetryWorkerConfig configures the retry worker behavior.
type RetryWorkerConfig struct {
	PollInterval    time.Duration // How often to check for messages to retry
	CleanupInterval time.Duration // How often to cleanup old messages
	BatchSize       int           // Max messages to process per poll
	BufferDuration  time.Duration // Stop retrying this long before swap expires
	RetentionPeriod time.Duration // How long to keep completed/failed messages
}

// DefaultRetryWorkerConfig returns the default configuration.
func DefaultRetryWorkerConfig() RetryWorkerConfig {
	return RetryWorkerConfig{
		PollInterval:    5 * time.Second,      // Check every 5 seconds
		CleanupInterval: 1 * time.Hour,        // Cleanup every hour
		BatchSize:       50,                   // Process up to 50 messages per poll
		BufferDuration:  1 * time.Hour,        // Stop 1 hour before expiry
		RetentionPeriod: 7 * 24 * time.Hour,   // Keep messages for 7 days
	}
}

// RetryWorker periodically checks for undelivered messages and retries them.
type RetryWorker struct {
	node    *Node
	storage *storage.Storage
	sender  *MessageSender
	config  RetryWorkerConfig
	log     *logging.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// NewRetryWorker creates a new retry worker.
func NewRetryWorker(n *Node, store *storage.Storage, sender *MessageSender, cfg RetryWorkerConfig) *RetryWorker {
	ctx, cancel := context.WithCancel(context.Background())

	return &RetryWorker{
		node:    n,
		storage: store,
		sender:  sender,
		config:  cfg,
		log:     logging.GetDefault().Component("retry-worker"),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts the retry worker background goroutine.
func (w *RetryWorker) Start() {
	go w.run()
	w.log.Info("Retry worker started", "poll_interval", w.config.PollInterval)
}

// Stop stops the retry worker.
func (w *RetryWorker) Stop() {
	w.cancel()
	w.log.Info("Retry worker stopped")
}

// run is the main loop of the retry worker.
func (w *RetryWorker) run() {
	retryTicker := time.NewTicker(w.config.PollInterval)
	cleanupTicker := time.NewTicker(w.config.CleanupInterval)
	defer retryTicker.Stop()
	defer cleanupTicker.Stop()

	// Run initial cleanup on startup
	w.cleanupOldMessages()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-retryTicker.C:
			w.processRetries()
		case <-cleanupTicker.C:
			w.cleanupOldMessages()
		}
	}
}

// cleanupOldMessages removes old completed/failed/expired messages.
func (w *RetryWorker) cleanupOldMessages() {
	olderThan := time.Now().Add(-w.config.RetentionPeriod).Unix()

	outboxCount, err := w.storage.CleanupOldMessages(olderThan)
	if err != nil {
		w.log.Warn("Failed to cleanup outbox messages", "error", err)
	}

	inboxCount, err := w.storage.CleanupOldInboxMessages(olderThan)
	if err != nil {
		w.log.Warn("Failed to cleanup inbox messages", "error", err)
	}

	if outboxCount > 0 || inboxCount > 0 {
		w.log.Info("Cleaned up old messages", "outbox", outboxCount, "inbox", inboxCount)
	}
}

// processRetries processes all messages that are due for retry.
func (w *RetryWorker) processRetries() {
	now := time.Now().Unix()

	// First, expire messages for swaps that are about to timeout
	bufferSeconds := int64(w.config.BufferDuration.Seconds())
	if err := w.storage.ExpireOldMessages(now, bufferSeconds); err != nil {
		w.log.Warn("Failed to expire old messages", "error", err)
	}

	// Get messages due for retry
	messages, err := w.storage.GetPendingMessages(now)
	if err != nil {
		w.log.Warn("Failed to get pending messages", "error", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	w.log.Debug("Processing pending messages", "count", len(messages))

	for _, msg := range messages {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		// Parse peer ID
		peerID, err := peer.Decode(msg.PeerID)
		if err != nil {
			w.log.Warn("Invalid peer ID", "peer", msg.PeerID, "message_id", msg.MessageID)
			if err := w.storage.MarkMessageFailed(msg.MessageID, "invalid peer ID"); err != nil {
				w.log.Warn("Failed to mark message failed", "error", err)
			}
			continue
		}

		// Check if peer is connected
		connected := w.node.Host().Network().Connectedness(peerID) == network.Connected

		if !connected {
			// Try to find peer in DHT and connect
			if w.node.DHT() != nil {
				ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
				pi, err := w.node.DHT().FindPeer(ctx, peerID)
				cancel()

				if err == nil {
					// Found peer, try to connect
					ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
					err = w.node.Connect(ctx, pi)
					cancel()

					if err == nil {
						connected = true
						w.log.Debug("Reconnected to peer via DHT", "peer", shortPeerID(peerID))
					}
				}
			}
		}

		if !connected {
			// Still not connected - schedule for later retry
			w.log.Debug("Peer not reachable, scheduling retry",
				"peer", shortPeerID(peerID),
				"message_id", msg.MessageID,
				"retry_count", msg.RetryCount)

			// Calculate next retry time with backoff
			nextRetry := w.calculateNextRetry(msg.RetryCount)
			if err := w.storage.ScheduleRetry(msg.MessageID, nextRetry.Unix()); err != nil {
				w.log.Warn("Failed to schedule retry", "error", err)
			}
			continue
		}

		// Peer is connected - attempt delivery
		w.log.Debug("Retrying message",
			"type", msg.MessageType,
			"trade_id", msg.TradeID,
			"message_id", msg.MessageID,
			"retry_count", msg.RetryCount)

		w.sender.RetryMessage(w.ctx, msg)
	}
}

// calculateNextRetry calculates the next retry time using exponential backoff.
func (w *RetryWorker) calculateNextRetry(retryCount int) time.Time {
	// Exponential backoff: 10s → 20s → 40s → 80s → 160s → 320s → 600s (10m max)
	baseInterval := 10 * time.Second
	maxInterval := 10 * time.Minute
	backoffMultiplier := 2.0

	backoff := baseInterval
	for i := 0; i < retryCount; i++ {
		backoff = time.Duration(float64(backoff) * backoffMultiplier)
		if backoff > maxInterval {
			backoff = maxInterval
			break
		}
	}

	return time.Now().Add(backoff)
}

// CleanupOldMessages removes old completed/failed messages from storage.
// Should be called periodically (e.g., once per day).
func (w *RetryWorker) CleanupOldMessages() (int64, error) {
	// Keep messages for 7 days
	olderThan := time.Now().Add(-7 * 24 * time.Hour).Unix()

	count, err := w.storage.CleanupOldMessages(olderThan)
	if err != nil {
		return 0, err
	}

	inboxCount, err := w.storage.CleanupOldInboxMessages(olderThan)
	if err != nil {
		return count, err
	}

	if count > 0 || inboxCount > 0 {
		w.log.Info("Cleaned up old messages", "outbox", count, "inbox", inboxCount)
	}

	return count + inboxCount, nil
}
