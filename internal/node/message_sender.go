// Package node - Message sender with persistence and retry support.
// Implements hybrid delivery: direct streams when connected, encrypted PubSub as fallback.
package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// MessageSenderConfig configures the message sender behavior.
type MessageSenderConfig struct {
	InitialRetryInterval time.Duration // Initial retry interval (default: 10s)
	MaxRetryInterval     time.Duration // Maximum retry interval (default: 10m)
	BackoffMultiplier    float64       // Backoff multiplier (default: 2.0)
	AckTimeout           time.Duration // Time to wait for ACK (default: 30s)
	StopBeforeExpiry     time.Duration // Stop retrying this long before swap expires (default: 1h)
	MaxRetries           int           // Maximum retry attempts before giving up (default: 50)
	DHTLookupTimeout     time.Duration // Timeout for DHT peer lookup (default: 30s)
	ConnectTimeout       time.Duration // Timeout for connecting to peer (default: 15s)
}

// DefaultMessageSenderConfig returns the default configuration.
func DefaultMessageSenderConfig() MessageSenderConfig {
	return MessageSenderConfig{
		InitialRetryInterval: 10 * time.Second,
		MaxRetryInterval:     10 * time.Minute,
		BackoffMultiplier:    2.0,
		AckTimeout:           30 * time.Second,
		StopBeforeExpiry:     1 * time.Hour,
		MaxRetries:           50, // ~8 hours with exponential backoff to 10min
		DHTLookupTimeout:     30 * time.Second,
		ConnectTimeout:       15 * time.Second,
	}
}

// MessageSender handles outbound messages with persistence and retry.
// Uses hybrid delivery: tries direct stream first, falls back to encrypted PubSub.
type MessageSender struct {
	node          *Node
	storage       *storage.Storage
	streamHandler *StreamHandler
	encryptor     *MessageEncryptor
	config        MessageSenderConfig
	log           *logging.Logger
}

// NewMessageSender creates a new message sender.
func NewMessageSender(n *Node, store *storage.Storage, streamHandler *StreamHandler, cfg MessageSenderConfig) *MessageSender {
	// Create message encryptor using node's identity key
	encryptor, err := NewMessageEncryptor(n.Host().Peerstore().PrivKey(n.ID()), n.ID())
	if err != nil {
		logging.GetDefault().Warn("Failed to create message encryptor, encrypted PubSub disabled", "error", err)
	}

	return &MessageSender{
		node:          n,
		storage:       store,
		streamHandler: streamHandler,
		encryptor:     encryptor,
		config:        cfg,
		log:           logging.GetDefault().Component("message-sender"),
	}
}

// SetEncryptor sets the message encryptor (for testing or late initialization).
func (s *MessageSender) SetEncryptor(enc *MessageEncryptor) {
	s.encryptor = enc
}

// SendDirect sends a message directly to a peer with persistence and guaranteed delivery.
// The message is persisted to the outbox first, then immediate delivery is attempted.
// If the peer is offline, the message will be retried automatically.
func (s *MessageSender) SendDirect(ctx context.Context, peerID peer.ID, tradeID string, swapTimeout int64, msg *SwapMessage) error {
	// Ensure message has required fields
	if msg.MessageID == "" {
		msg.MessageID = uuid.New().String()
	}
	if msg.TradeID == "" {
		msg.TradeID = tradeID
	}
	msg.SwapTimeout = swapTimeout
	msg.RequiresAck = true
	msg.FromPeer = s.node.ID().String()
	msg.Timestamp = time.Now().Unix()

	// Get next sequence number for this trade
	seq, err := s.storage.GetNextLocalSequence(tradeID)
	if err != nil {
		return fmt.Errorf("failed to get sequence number: %w", err)
	}
	msg.SequenceNum = seq

	// Serialize payload
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Persist to outbox FIRST (before attempting send)
	outboxMsg := &storage.OutboxMessage{
		MessageID:   msg.MessageID,
		TradeID:     tradeID,
		PeerID:      peerID.String(),
		MessageType: msg.Type,
		Payload:     payload,
		SequenceNum: seq,
		SwapTimeout: swapTimeout,
	}

	if err := s.storage.EnqueueMessage(outboxMsg); err != nil {
		return fmt.Errorf("failed to persist message: %w", err)
	}

	s.log.Debug("Message enqueued",
		"type", msg.Type,
		"trade_id", tradeID,
		"message_id", msg.MessageID,
		"peer", shortPeerID(peerID))

	// Attempt immediate delivery in background
	// Use background context since delivery should outlive the HTTP request
	go s.attemptDelivery(context.Background(), peerID, msg)

	return nil
}

// attemptDelivery tries to deliver a message to a peer using hybrid delivery.
// Strategy:
// 1. If not connected, try to find peer via DHT and connect
// 2. If connected, try direct stream (fastest, most private)
// 3. If direct fails, fallback to encrypted PubSub (guaranteed delivery through gossip)
func (s *MessageSender) attemptDelivery(ctx context.Context, peerID peer.ID, msg *SwapMessage) {
	// Check if swap has expired (minus buffer)
	deadline := time.Unix(msg.SwapTimeout, 0).Add(-s.config.StopBeforeExpiry)
	if time.Now().After(deadline) {
		s.log.Warn("Swap timeout approaching, marking message expired",
			"message_id", msg.MessageID,
			"trade_id", msg.TradeID)
		if err := s.storage.MarkMessageExpired(msg.MessageID); err != nil {
			s.log.Warn("Failed to mark message expired", "error", err)
		}
		return
	}

	// Mark as sent (attempting)
	if err := s.storage.MarkMessageSent(msg.MessageID); err != nil {
		s.log.Warn("Failed to mark message sent", "error", err)
	}

	// Step 1: Try to establish connection if not connected
	if s.node.Host().Network().Connectedness(peerID) != network.Connected {
		s.log.Debug("Peer not connected, attempting DHT lookup",
			"peer", shortPeerID(peerID),
			"message_id", msg.MessageID)

		if s.tryConnectViaDHT(ctx, peerID) {
			s.log.Debug("Connected to peer via DHT", "peer", shortPeerID(peerID))
		}
	}

	// Step 2: Try direct stream if now connected
	if s.node.Host().Network().Connectedness(peerID) == network.Connected {
		deliveryCtx, cancel := context.WithTimeout(ctx, s.config.AckTimeout)
		err := s.streamHandler.SendDirectMessage(deliveryCtx, peerID, msg)
		cancel()

		if err == nil {
			// Success via direct stream
			if err := s.storage.MarkMessageAcked(msg.MessageID); err != nil {
				s.log.Warn("Failed to mark message ACKed", "error", err)
			}
			s.log.Debug("Message delivered via direct stream",
				"type", msg.Type,
				"trade_id", msg.TradeID,
				"message_id", msg.MessageID)
			return
		}

		s.log.Debug("Direct stream failed, trying encrypted PubSub",
			"peer", shortPeerID(peerID),
			"error", err)
	}

	// Step 3: Fallback to encrypted PubSub
	if s.encryptor != nil {
		if s.sendViaEncryptedPubSub(ctx, peerID, msg) {
			// Message sent via PubSub, ACK will come back via PubSub too
			s.log.Debug("Message sent via encrypted PubSub",
				"type", msg.Type,
				"trade_id", msg.TradeID,
				"message_id", msg.MessageID)
			// Don't mark as ACKed yet - wait for ACK via PubSub
			// Schedule a retry in case ACK doesn't come
			s.scheduleRetry(msg.MessageID, 0)
			return
		}
	}

	// All delivery methods failed, schedule retry
	s.log.Debug("All delivery methods failed, scheduling retry",
		"peer", shortPeerID(peerID),
		"message_id", msg.MessageID)

	pending, _ := s.storage.GetPendingForTrade(msg.TradeID)
	retryCount := 0
	for _, p := range pending {
		if p.MessageID == msg.MessageID {
			retryCount = p.RetryCount
			break
		}
	}
	s.scheduleRetry(msg.MessageID, retryCount)
}

// tryConnectViaDHT attempts to find and connect to a peer using the DHT.
func (s *MessageSender) tryConnectViaDHT(ctx context.Context, peerID peer.ID) bool {
	dht := s.node.DHT()
	if dht == nil {
		s.log.Debug("DHT not available for peer lookup")
		return false
	}

	// Look up peer in DHT
	lookupCtx, cancel := context.WithTimeout(ctx, s.config.DHTLookupTimeout)
	defer cancel()

	peerInfo, err := dht.FindPeer(lookupCtx, peerID)
	if err != nil {
		s.log.Debug("DHT peer lookup failed", "peer", shortPeerID(peerID), "error", err)
		return false
	}

	if len(peerInfo.Addrs) == 0 {
		s.log.Debug("DHT found peer but no addresses", "peer", shortPeerID(peerID))
		return false
	}

	// Try to connect
	connectCtx, cancel := context.WithTimeout(ctx, s.config.ConnectTimeout)
	defer cancel()

	if err := s.node.Host().Connect(connectCtx, peerInfo); err != nil {
		s.log.Debug("Failed to connect to peer", "peer", shortPeerID(peerID), "error", err)
		return false
	}

	return true
}

// sendViaEncryptedPubSub sends an encrypted message via PubSub gossip.
// The message is encrypted so only the recipient can read it.
func (s *MessageSender) sendViaEncryptedPubSub(ctx context.Context, peerID peer.ID, msg *SwapMessage) bool {
	if s.encryptor == nil {
		return false
	}

	// Encrypt the message for the recipient
	envelope, err := s.encryptor.Encrypt(peerID, msg)
	if err != nil {
		s.log.Warn("Failed to encrypt message for PubSub", "error", err)
		return false
	}

	// Serialize the envelope
	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		s.log.Warn("Failed to marshal encrypted envelope", "error", err)
		return false
	}

	// Publish to the encrypted swap topic
	topic := s.node.GetTopic(SwapEncryptedTopic)
	if topic == nil {
		s.log.Debug("Encrypted swap topic not available")
		return false
	}

	if err := topic.Publish(ctx, envelopeBytes); err != nil {
		s.log.Warn("Failed to publish encrypted message", "error", err)
		return false
	}

	return true
}

// scheduleRetry schedules a message for retry with exponential backoff.
func (s *MessageSender) scheduleRetry(messageID string, currentRetryCount int) {
	// Calculate backoff
	backoff := s.config.InitialRetryInterval
	for i := 0; i < currentRetryCount; i++ {
		backoff = time.Duration(float64(backoff) * s.config.BackoffMultiplier)
		if backoff > s.config.MaxRetryInterval {
			backoff = s.config.MaxRetryInterval
			break
		}
	}

	nextRetry := time.Now().Add(backoff).Unix()
	if err := s.storage.ScheduleRetry(messageID, nextRetry); err != nil {
		s.log.Warn("Failed to schedule retry", "error", err)
	}

	s.log.Debug("Retry scheduled",
		"message_id", messageID,
		"next_retry", time.Unix(nextRetry, 0).Format(time.RFC3339),
		"backoff", backoff)
}

// RetryMessage retries a pending message from the outbox.
func (s *MessageSender) RetryMessage(ctx context.Context, outboxMsg *storage.OutboxMessage) {
	// Check if max retries exceeded
	if s.config.MaxRetries > 0 && outboxMsg.RetryCount >= s.config.MaxRetries {
		s.log.Warn("Max retries exceeded, marking message failed",
			"message_id", outboxMsg.MessageID,
			"trade_id", outboxMsg.TradeID,
			"retry_count", outboxMsg.RetryCount,
			"max_retries", s.config.MaxRetries)
		if err := s.storage.MarkMessageFailed(outboxMsg.MessageID, "max retries exceeded"); err != nil {
			s.log.Warn("Failed to mark message failed", "error", err)
		}
		return
	}

	// Parse peer ID
	peerID, err := peer.Decode(outboxMsg.PeerID)
	if err != nil {
		s.log.Error("Invalid peer ID in outbox", "peer", outboxMsg.PeerID)
		if err := s.storage.MarkMessageFailed(outboxMsg.MessageID, "invalid peer ID"); err != nil {
			s.log.Warn("Failed to mark message failed", "error", err)
		}
		return
	}

	// Reconstruct SwapMessage from outbox
	var msg SwapMessage
	if err := json.Unmarshal(outboxMsg.Payload, &msg); err != nil {
		s.log.Error("Invalid message payload in outbox", "message_id", outboxMsg.MessageID)
		if err := s.storage.MarkMessageFailed(outboxMsg.MessageID, "invalid payload"); err != nil {
			s.log.Warn("Failed to mark message failed", "error", err)
		}
		return
	}

	// Check if swap has expired
	deadline := time.Unix(outboxMsg.SwapTimeout, 0).Add(-s.config.StopBeforeExpiry)
	if time.Now().After(deadline) {
		s.log.Warn("Swap timeout approaching, marking message expired",
			"message_id", outboxMsg.MessageID,
			"trade_id", outboxMsg.TradeID)
		if err := s.storage.MarkMessageExpired(outboxMsg.MessageID); err != nil {
			s.log.Warn("Failed to mark message expired", "error", err)
		}
		return
	}

	s.attemptDelivery(ctx, peerID, &msg)
}

// FlushPendingForPeer attempts to deliver all pending messages for a peer.
// Called when a peer reconnects.
func (s *MessageSender) FlushPendingForPeer(ctx context.Context, peerID peer.ID) {
	messages, err := s.storage.GetPendingForPeer(peerID.String())
	if err != nil {
		s.log.Warn("Failed to get pending messages for peer", "error", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	s.log.Info("Peer reconnected, flushing pending messages",
		"peer", shortPeerID(peerID),
		"count", len(messages))

	for _, msg := range messages {
		s.RetryMessage(ctx, msg)
	}
}

// GetPendingCount returns the number of pending messages for a trade.
func (s *MessageSender) GetPendingCount(tradeID string) (int, error) {
	messages, err := s.storage.GetPendingForTrade(tradeID)
	if err != nil {
		return 0, err
	}
	return len(messages), nil
}

// CancelPendingForTrade marks all pending messages for a trade as failed.
// Used when a swap is cancelled or times out.
func (s *MessageSender) CancelPendingForTrade(tradeID string, reason string) error {
	messages, err := s.storage.GetPendingForTrade(tradeID)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		if err := s.storage.MarkMessageFailed(msg.MessageID, reason); err != nil {
			s.log.Warn("Failed to mark message failed", "error", err)
		}
	}

	s.log.Info("Cancelled pending messages for trade",
		"trade_id", tradeID,
		"count", len(messages),
		"reason", reason)

	return nil
}
