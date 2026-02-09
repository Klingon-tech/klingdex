package storage

import (
	"os"
	"testing"
	"time"
)

// setupTestStorage creates a temporary storage for testing.
func setupTestStorage(t *testing.T) (*Storage, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "klingon-mq-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestEnqueueMessage(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	msg := &OutboxMessage{
		MessageID:   "msg-123",
		TradeID:     "trade-456",
		PeerID:      "peer-789",
		MessageType: "pubkey_exchange",
		Payload:     []byte(`{"test":"data"}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	// Verify message was stored
	pending, err := store.GetPendingMessages(time.Now().Unix())
	if err != nil {
		t.Fatalf("GetPendingMessages() error = %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(pending))
	}

	if pending[0].MessageID != "msg-123" {
		t.Errorf("expected message_id 'msg-123', got '%s'", pending[0].MessageID)
	}
	if pending[0].Status != OutboxStatusPending {
		t.Errorf("expected status 'pending', got '%s'", pending[0].Status)
	}
}

func TestMessageStatusTransitions(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	msg := &OutboxMessage{
		MessageID:   "msg-status-test",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	// Test MarkMessageSent
	if err := store.MarkMessageSent(msg.MessageID); err != nil {
		t.Fatalf("MarkMessageSent() error = %v", err)
	}

	// Verify status is 'sent'
	outMsg, err := store.GetOutboxMessage(msg.MessageID)
	if err != nil {
		t.Fatalf("GetOutboxMessage() error = %v", err)
	}
	if outMsg.Status != OutboxStatusSent {
		t.Errorf("expected status 'sent', got '%s'", outMsg.Status)
	}

	// Test MarkMessageAcked
	if err := store.MarkMessageAcked(msg.MessageID); err != nil {
		t.Fatalf("MarkMessageAcked() error = %v", err)
	}

	outMsg, _ = store.GetOutboxMessage(msg.MessageID)
	if outMsg.Status != OutboxStatusAcked {
		t.Errorf("expected status 'acked', got '%s'", outMsg.Status)
	}
	if outMsg.AckedAt == nil {
		t.Error("expected AckedAt to be set")
	}
}

func TestMarkMessageFailed(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	msg := &OutboxMessage{
		MessageID:   "msg-fail-test",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	errMsg := "max retries exceeded"
	if err := store.MarkMessageFailed(msg.MessageID, errMsg); err != nil {
		t.Fatalf("MarkMessageFailed() error = %v", err)
	}

	outMsg, _ := store.GetOutboxMessage(msg.MessageID)
	if outMsg.Status != OutboxStatusFailed {
		t.Errorf("expected status 'failed', got '%s'", outMsg.Status)
	}
	if outMsg.ErrorMessage != errMsg {
		t.Errorf("expected error message '%s', got '%s'", errMsg, outMsg.ErrorMessage)
	}
}

func TestMarkMessageExpired(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	msg := &OutboxMessage{
		MessageID:   "msg-expire-test",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	if err := store.MarkMessageExpired(msg.MessageID); err != nil {
		t.Fatalf("MarkMessageExpired() error = %v", err)
	}

	outMsg, _ := store.GetOutboxMessage(msg.MessageID)
	if outMsg.Status != OutboxStatusExpired {
		t.Errorf("expected status 'expired', got '%s'", outMsg.Status)
	}
}

func TestScheduleRetry(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	msg := &OutboxMessage{
		MessageID:   "msg-retry-test",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	// Simulate first delivery attempt (this increments retry_count)
	if err := store.MarkMessageSent(msg.MessageID); err != nil {
		t.Fatalf("MarkMessageSent() error = %v", err)
	}

	// Schedule retry for 1 minute in the future
	nextRetry := time.Now().Add(1 * time.Minute).Unix()
	if err := store.ScheduleRetry(msg.MessageID, nextRetry); err != nil {
		t.Fatalf("ScheduleRetry() error = %v", err)
	}

	outMsg, _ := store.GetOutboxMessage(msg.MessageID)
	if outMsg.RetryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", outMsg.RetryCount)
	}
	if outMsg.NextRetryAt != nextRetry {
		t.Errorf("expected next_retry_at %d, got %d", nextRetry, outMsg.NextRetryAt)
	}
	if outMsg.Status != OutboxStatusPending {
		t.Errorf("expected status 'pending' after ScheduleRetry, got '%s'", outMsg.Status)
	}

	// Should not appear in pending messages yet (retry time in future)
	pending, _ := store.GetPendingMessages(time.Now().Unix())
	for _, p := range pending {
		if p.MessageID == msg.MessageID {
			t.Error("message should not appear in pending list before next_retry_at")
		}
	}

	// Should appear after next_retry_at
	pending, _ = store.GetPendingMessages(nextRetry + 1)
	found := false
	for _, p := range pending {
		if p.MessageID == msg.MessageID {
			found = true
			break
		}
	}
	if !found {
		t.Error("message should appear in pending list after next_retry_at")
	}
}

func TestGetPendingForPeer(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create messages for two different peers
	for i, peerID := range []string{"peer-A", "peer-B", "peer-A"} {
		msg := &OutboxMessage{
			MessageID:   "msg-" + string(rune('1'+i)),
			TradeID:     "trade-1",
			PeerID:      peerID,
			MessageType: "test",
			Payload:     []byte(`{}`),
			SequenceNum: uint64(i + 1),
			SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
		}
		if err := store.EnqueueMessage(msg); err != nil {
			t.Fatalf("EnqueueMessage() error = %v", err)
		}
	}

	// Get messages for peer-A
	msgs, err := store.GetPendingForPeer("peer-A")
	if err != nil {
		t.Fatalf("GetPendingForPeer() error = %v", err)
	}

	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for peer-A, got %d", len(msgs))
	}
}

func TestGetPendingForTrade(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create messages for two different trades
	for i, tradeID := range []string{"trade-X", "trade-Y", "trade-X"} {
		msg := &OutboxMessage{
			MessageID:   "msg-t" + string(rune('1'+i)),
			TradeID:     tradeID,
			PeerID:      "peer-1",
			MessageType: "test",
			Payload:     []byte(`{}`),
			SequenceNum: uint64(i + 1),
			SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
		}
		if err := store.EnqueueMessage(msg); err != nil {
			t.Fatalf("EnqueueMessage() error = %v", err)
		}
	}

	// Get messages for trade-X
	msgs, err := store.GetPendingForTrade("trade-X")
	if err != nil {
		t.Fatalf("GetPendingForTrade() error = %v", err)
	}

	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for trade-X, got %d", len(msgs))
	}
}

func TestInboxOperations(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	inMsg := &InboxMessage{
		MessageID:   "inbox-msg-1",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "pubkey_exchange",
		SequenceNum: 1,
	}

	// Record received message
	if err := store.RecordReceivedMessage(inMsg); err != nil {
		t.Fatalf("RecordReceivedMessage() error = %v", err)
	}

	// Check for duplicate
	isDup, err := store.HasReceivedMessage(inMsg.MessageID)
	if err != nil {
		t.Fatalf("HasReceivedMessage() error = %v", err)
	}
	if !isDup {
		t.Error("expected message to be recognized as duplicate")
	}

	// Check non-existent message
	isDup, _ = store.HasReceivedMessage("non-existent")
	if isDup {
		t.Error("non-existent message should not be a duplicate")
	}

	// Mark as processed
	if err := store.MarkMessageProcessed(inMsg.MessageID); err != nil {
		t.Fatalf("MarkMessageProcessed() error = %v", err)
	}

	// Mark ACK sent
	if err := store.MarkAckSent(inMsg.MessageID); err != nil {
		t.Fatalf("MarkAckSent() error = %v", err)
	}
}

func TestSequenceNumbers(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	tradeID := "trade-seq-test"

	// Get first sequence number
	seq1, err := store.GetNextLocalSequence(tradeID)
	if err != nil {
		t.Fatalf("GetNextLocalSequence() error = %v", err)
	}
	if seq1 != 1 {
		t.Errorf("expected first sequence to be 1, got %d", seq1)
	}

	// Get next sequence number
	seq2, err := store.GetNextLocalSequence(tradeID)
	if err != nil {
		t.Fatalf("GetNextLocalSequence() error = %v", err)
	}
	if seq2 != 2 {
		t.Errorf("expected second sequence to be 2, got %d", seq2)
	}

	// Update remote sequence
	if err := store.UpdateRemoteSequence(tradeID, 5); err != nil {
		t.Fatalf("UpdateRemoteSequence() error = %v", err)
	}

	// Verify sequence state
	seqState, err := store.GetSequences(tradeID)
	if err != nil {
		t.Fatalf("GetSequences() error = %v", err)
	}
	if seqState.LocalSeq != 2 {
		t.Errorf("expected local_seq 2, got %d", seqState.LocalSeq)
	}
	if seqState.RemoteSeq != 5 {
		t.Errorf("expected remote_seq 5, got %d", seqState.RemoteSeq)
	}
}

func TestExpireOldMessages(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create a message with swap timeout in the past
	msg := &OutboxMessage{
		MessageID:   "msg-old",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(-1 * time.Hour).Unix(), // Already expired
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	// Expire old messages (with 30 min buffer)
	now := time.Now().Unix()
	bufferSeconds := int64(30 * 60) // 30 minutes
	if err := store.ExpireOldMessages(now, bufferSeconds); err != nil {
		t.Fatalf("ExpireOldMessages() error = %v", err)
	}

	// Check message is expired
	outMsg, _ := store.GetOutboxMessage(msg.MessageID)
	if outMsg.Status != OutboxStatusExpired {
		t.Errorf("expected status 'expired', got '%s'", outMsg.Status)
	}
}

func TestCleanupOldMessages(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create an old acked message
	msg := &OutboxMessage{
		MessageID:   "msg-cleanup",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		Payload:     []byte(`{}`),
		SequenceNum: 1,
		SwapTimeout: time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := store.EnqueueMessage(msg); err != nil {
		t.Fatalf("EnqueueMessage() error = %v", err)
	}

	// Mark as acked
	if err := store.MarkMessageAcked(msg.MessageID); err != nil {
		t.Fatalf("MarkMessageAcked() error = %v", err)
	}

	// Cleanup messages older than now (should include our message since created_at < now+1)
	count, err := store.CleanupOldMessages(time.Now().Add(1 * time.Second).Unix())
	if err != nil {
		t.Fatalf("CleanupOldMessages() error = %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 message cleaned up, got %d", count)
	}

	// Verify message is gone
	_, err = store.GetOutboxMessage(msg.MessageID)
	if err == nil {
		t.Error("expected message to be deleted")
	}
}

func TestCleanupOldInboxMessages(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	inMsg := &InboxMessage{
		MessageID:   "inbox-cleanup",
		TradeID:     "trade-1",
		PeerID:      "peer-1",
		MessageType: "test",
		SequenceNum: 1,
	}

	if err := store.RecordReceivedMessage(inMsg); err != nil {
		t.Fatalf("RecordReceivedMessage() error = %v", err)
	}

	// Cleanup messages older than now+1s
	count, err := store.CleanupOldInboxMessages(time.Now().Add(1 * time.Second).Unix())
	if err != nil {
		t.Fatalf("CleanupOldInboxMessages() error = %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 inbox message cleaned up, got %d", count)
	}

	// Verify message is gone (not recognized as duplicate anymore)
	isDup, _ := store.HasReceivedMessage(inMsg.MessageID)
	if isDup {
		t.Error("expected inbox message to be deleted")
	}
}
