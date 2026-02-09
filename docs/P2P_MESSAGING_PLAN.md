# P2P Direct Messaging & Message Persistence Plan

## Current State (Problems)

| Issue | Impact |
|-------|--------|
| All swap messages broadcast via PubSub | Privacy leak - all peers see all swap data |
| No message persistence | If peer offline, message lost forever |
| No retry mechanism | Swaps stuck if any message fails |
| No acknowledgments | Sender doesn't know if received |
| No idempotency | Duplicate messages can corrupt state |

## Proposed Architecture

### 1. Message Categories

**Public (PubSub broadcast):**
- `order_announce` - Needs to reach all peers
- `order_cancel` - Needs to reach all peers

**Private (Direct P2P stream):**
- `pubkey_exchange`
- `nonce_exchange`
- `funding_info`
- `partial_sig`
- `htlc_secret_hash`
- `htlc_secret_reveal`
- `htlc_claim`
- `complete`
- `abort`
- `ack` (NEW)

### 2. Message Envelope (Enhanced)

```go
// SwapMessage with delivery guarantees
type SwapMessage struct {
    // Existing fields
    Type      string          `json:"type"`
    TradeID   string          `json:"trade_id"`
    OrderID   string          `json:"order_id,omitempty"`
    FromPeer  string          `json:"from_peer"`
    Payload   json.RawMessage `json:"payload"`
    Timestamp int64           `json:"timestamp"`

    // NEW: Delivery guarantee fields
    MessageID   string `json:"message_id"`    // UUID for deduplication
    SequenceNum uint64 `json:"sequence_num"`  // Per-trade sequence number
    RequiresAck bool   `json:"requires_ack"`  // Whether sender expects ACK
}

// Acknowledgment message
type AckPayload struct {
    MessageID   string `json:"message_id"`   // Which message we're ACKing
    SequenceNum uint64 `json:"sequence_num"` // Sequence number ACKed
    Success     bool   `json:"success"`      // Processing successful
    Error       string `json:"error,omitempty"` // Error if failed
}
```

### 3. Storage Schema

```sql
-- Outbound message queue (pending delivery)
CREATE TABLE message_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT UNIQUE NOT NULL,      -- UUID
    trade_id TEXT NOT NULL,
    peer_id TEXT NOT NULL,                -- Target peer
    message_type TEXT NOT NULL,
    payload BLOB NOT NULL,                -- Full message JSON
    sequence_num INTEGER NOT NULL,
    created_at INTEGER NOT NULL,          -- Unix timestamp
    retry_count INTEGER DEFAULT 0,
    next_retry_at INTEGER NOT NULL,       -- When to retry
    acked_at INTEGER,                     -- NULL until ACKed
    status TEXT DEFAULT 'pending'         -- pending, sent, acked, failed
);

CREATE INDEX idx_outbox_pending ON message_outbox(status, next_retry_at)
    WHERE status = 'pending' OR status = 'sent';
CREATE INDEX idx_outbox_trade ON message_outbox(trade_id);

-- Inbound message log (for deduplication)
CREATE TABLE message_inbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT UNIQUE NOT NULL,      -- UUID (for dedup)
    trade_id TEXT NOT NULL,
    peer_id TEXT NOT NULL,                -- Sender peer
    message_type TEXT NOT NULL,
    sequence_num INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    processed_at INTEGER                  -- NULL until processed
);

CREATE INDEX idx_inbox_message ON message_inbox(message_id);
CREATE INDEX idx_inbox_trade ON message_inbox(trade_id, sequence_num);

-- Sequence number tracking per trade
CREATE TABLE message_sequences (
    trade_id TEXT PRIMARY KEY,
    local_seq INTEGER DEFAULT 0,          -- Our next sequence number
    remote_seq INTEGER DEFAULT 0          -- Last received sequence number
);
```

### 4. Direct P2P Stream Protocol

Using libp2p streams for direct messaging:

```go
// Protocol ID for swap messages
const SwapProtocolID = "/klingon/swap/direct/1.0.0"

// SwapStreamHandler handles incoming direct messages
type SwapStreamHandler struct {
    node     *Node
    handlers map[string]MessageHandler
    inbox    *MessageInbox
    log      *logging.Logger
}

// Handle incoming stream
func (h *SwapStreamHandler) HandleStream(s network.Stream) {
    defer s.Close()

    // Read message
    reader := bufio.NewReader(s)
    msgBytes, err := readLengthPrefixed(reader)
    if err != nil {
        h.log.Warn("Failed to read message", "error", err)
        return
    }

    var msg SwapMessage
    if err := json.Unmarshal(msgBytes, &msg); err != nil {
        h.log.Warn("Failed to parse message", "error", err)
        return
    }

    // Check for duplicate (idempotency)
    if h.inbox.HasMessage(msg.MessageID) {
        // Already processed - send ACK again
        h.sendAck(s, msg.MessageID, msg.SequenceNum, true, "")
        return
    }

    // Process message
    handler, ok := h.handlers[msg.Type]
    if !ok {
        h.log.Warn("Unknown message type", "type", msg.Type)
        h.sendAck(s, msg.MessageID, msg.SequenceNum, false, "unknown type")
        return
    }

    // Record in inbox BEFORE processing (for dedup)
    h.inbox.RecordMessage(msg)

    // Process (may fail)
    err = handler(context.Background(), &msg)

    // Send ACK
    if msg.RequiresAck {
        if err != nil {
            h.sendAck(s, msg.MessageID, msg.SequenceNum, false, err.Error())
        } else {
            h.sendAck(s, msg.MessageID, msg.SequenceNum, true, "")
        }
    }

    // Mark processed
    h.inbox.MarkProcessed(msg.MessageID)
}

// sendAck writes acknowledgment to stream
func (h *SwapStreamHandler) sendAck(s network.Stream, msgID string, seq uint64, success bool, errMsg string) {
    ack := SwapMessage{
        Type:        "ack",
        MessageID:   uuid.New().String(),
        SequenceNum: seq,
        Payload:     mustMarshal(AckPayload{
            MessageID:   msgID,
            SequenceNum: seq,
            Success:     success,
            Error:       errMsg,
        }),
    }

    data, _ := json.Marshal(ack)
    writeLengthPrefixed(s, data)
}
```

### 5. Message Sender with Retry

```go
// MessageSender handles outbound messages with persistence
type MessageSender struct {
    node    *Node
    store   *storage.Storage
    outbox  *MessageOutbox
    log     *logging.Logger

    retryInterval time.Duration // Base retry interval
    maxRetries    int           // Max retry attempts
}

// SendDirect sends a message directly to a peer with persistence
func (s *MessageSender) SendDirect(ctx context.Context, peerID peer.ID, msg *SwapMessage) error {
    // Generate message ID if not set
    if msg.MessageID == "" {
        msg.MessageID = uuid.New().String()
    }

    // Get next sequence number for this trade
    msg.SequenceNum = s.outbox.NextSequence(msg.TradeID)
    msg.RequiresAck = true
    msg.FromPeer = s.node.ID().String()
    msg.Timestamp = time.Now().Unix()

    // Persist to outbox FIRST (before attempting send)
    if err := s.outbox.Enqueue(peerID.String(), msg); err != nil {
        return fmt.Errorf("failed to persist message: %w", err)
    }

    // Attempt immediate delivery
    go s.attemptDelivery(ctx, peerID, msg)

    return nil
}

// attemptDelivery tries to deliver a message to a peer
func (s *MessageSender) attemptDelivery(ctx context.Context, peerID peer.ID, msg *SwapMessage) {
    // Open stream to peer
    stream, err := s.node.Host().NewStream(ctx, peerID, SwapProtocolID)
    if err != nil {
        s.log.Debug("Peer unreachable, will retry", "peer", peerID, "error", err)
        s.outbox.ScheduleRetry(msg.MessageID, s.retryInterval)
        return
    }
    defer stream.Close()

    // Send message
    data, _ := json.Marshal(msg)
    if err := writeLengthPrefixed(stream, data); err != nil {
        s.log.Debug("Send failed, will retry", "error", err)
        s.outbox.ScheduleRetry(msg.MessageID, s.retryInterval)
        return
    }

    // Wait for ACK (with timeout)
    stream.SetReadDeadline(time.Now().Add(30 * time.Second))
    ackBytes, err := readLengthPrefixed(bufio.NewReader(stream))
    if err != nil {
        s.log.Debug("ACK timeout, will retry", "error", err)
        s.outbox.ScheduleRetry(msg.MessageID, s.retryInterval)
        return
    }

    var ackMsg SwapMessage
    if err := json.Unmarshal(ackBytes, &ackMsg); err != nil {
        s.log.Warn("Invalid ACK", "error", err)
        s.outbox.ScheduleRetry(msg.MessageID, s.retryInterval)
        return
    }

    var ack AckPayload
    json.Unmarshal(ackMsg.Payload, &ack)

    if ack.Success {
        // Message delivered successfully
        s.outbox.MarkAcked(msg.MessageID)
        s.log.Debug("Message delivered", "type", msg.Type, "trade", msg.TradeID)
    } else {
        // Peer rejected message - log error but don't retry
        s.log.Warn("Message rejected by peer", "error", ack.Error)
        s.outbox.MarkFailed(msg.MessageID, ack.Error)
    }
}
```

### 6. Retry Worker (Background)

```go
// RetryWorker periodically retries undelivered messages
type RetryWorker struct {
    sender   *MessageSender
    outbox   *MessageOutbox
    node     *Node
    interval time.Duration
    log      *logging.Logger
}

func (w *RetryWorker) Start(ctx context.Context) {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.processRetries(ctx)
        }
    }
}

func (w *RetryWorker) processRetries(ctx context.Context) {
    // Get messages due for retry
    messages, err := w.outbox.GetPendingRetries(time.Now().Unix())
    if err != nil {
        w.log.Error("Failed to get pending retries", "error", err)
        return
    }

    for _, msg := range messages {
        // Check retry count
        if msg.RetryCount >= w.sender.maxRetries {
            w.log.Warn("Max retries exceeded",
                "message_id", msg.MessageID,
                "type", msg.Type,
                "trade", msg.TradeID)
            w.outbox.MarkFailed(msg.MessageID, "max retries exceeded")
            continue
        }

        // Parse peer ID
        peerID, err := peer.Decode(msg.PeerID)
        if err != nil {
            w.log.Error("Invalid peer ID", "peer", msg.PeerID)
            continue
        }

        // Check if peer is connected
        if w.node.Host().Network().Connectedness(peerID) != network.Connected {
            // Peer not connected - try to connect first
            if err := w.node.Connect(ctx, peerID); err != nil {
                // Schedule for later retry with backoff
                backoff := w.calculateBackoff(msg.RetryCount)
                w.outbox.ScheduleRetry(msg.MessageID, backoff)
                continue
            }
        }

        // Attempt delivery
        w.outbox.IncrementRetry(msg.MessageID)
        go w.sender.attemptDelivery(ctx, peerID, msg.ToSwapMessage())
    }
}

func (w *RetryWorker) calculateBackoff(retryCount int) time.Duration {
    // Exponential backoff: 5s, 10s, 20s, 40s, 80s, 160s (max)
    base := 5 * time.Second
    backoff := base * time.Duration(1<<retryCount)
    if backoff > 160*time.Second {
        backoff = 160 * time.Second
    }
    return backoff
}
```

### 7. Peer Online Detection & Reconnection

```go
// PeerMonitor watches for peer connection events
type PeerMonitor struct {
    node   *Node
    outbox *MessageOutbox
    sender *MessageSender
    log    *logging.Logger
}

func (m *PeerMonitor) Start(ctx context.Context) {
    // Subscribe to peer connection events
    sub, err := m.node.Host().EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
    if err != nil {
        m.log.Error("Failed to subscribe to peer events", "error", err)
        return
    }
    defer sub.Close()

    for {
        select {
        case <-ctx.Done():
            return
        case ev := <-sub.Out():
            e := ev.(event.EvtPeerConnectednessChanged)
            if e.Connectedness == network.Connected {
                // Peer came online - check for pending messages
                m.flushPendingMessages(ctx, e.Peer)
            }
        }
    }
}

func (m *PeerMonitor) flushPendingMessages(ctx context.Context, peerID peer.ID) {
    // Get pending messages for this peer
    messages, err := m.outbox.GetPendingForPeer(peerID.String())
    if err != nil {
        return
    }

    if len(messages) > 0 {
        m.log.Info("Peer reconnected, flushing pending messages",
            "peer", peerID,
            "count", len(messages))

        for _, msg := range messages {
            go m.sender.attemptDelivery(ctx, peerID, msg.ToSwapMessage())
        }
    }
}
```

### 8. Swap State Synchronization

When a peer reconnects after missing messages:

```go
// SyncHandler handles state synchronization requests
type SyncHandler struct {
    coordinator *swap.Coordinator
    store       *storage.Storage
    log         *logging.Logger
}

// HandleSyncRequest responds to sync requests from a peer
func (h *SyncHandler) HandleSyncRequest(ctx context.Context, msg *SwapMessage) error {
    var req SyncRequestPayload
    json.Unmarshal(msg.Payload, &req)

    // Get our current state for this trade
    swap, err := h.coordinator.GetSwap(req.TradeID)
    if err != nil {
        return err
    }

    // Build state summary
    state := SwapStateSummary{
        TradeID:           req.TradeID,
        State:             swap.State,
        HasRemotePubKey:   swap.HasRemotePubKey(),
        HasRemoteNonces:   swap.HasRemoteNonces(),
        HasFunding:        swap.HasFunding(),
        HasRemotePartials: swap.HasRemotePartialSigs(),
        LocalSequence:     h.store.GetLocalSequence(req.TradeID),
    }

    // Send state summary back
    // Peer can then request specific missing data
    return h.sendStateSummary(ctx, msg.FromPeer, state)
}
```

### 9. Implementation Order

#### Phase 1: Storage & Infrastructure
1. Add `message_outbox` table to storage
2. Add `message_inbox` table to storage
3. Add `message_sequences` table to storage
4. Implement `MessageOutbox` and `MessageInbox` structs
5. Add message ID and sequence fields to `SwapMessage`

#### Phase 2: Direct Streaming
1. Create `SwapStreamHandler` for incoming streams
2. Register stream handler with libp2p host
3. Implement `SendDirect()` in `MessageSender`
4. Add ACK message type and handling

#### Phase 3: Retry & Persistence
1. Implement `RetryWorker` background goroutine
2. Add exponential backoff logic
3. Implement `PeerMonitor` for reconnection detection
4. Add flush-on-connect logic

#### Phase 4: Integration
1. Update RPC handlers to use `SendDirect()` instead of `SendMessage()`
2. Keep `SendMessage()` for public broadcasts (orders)
3. Update handler registration
4. Add sync request/response protocol

#### Phase 5: Testing & Hardening
1. Unit tests for message persistence
2. Integration tests for offline peer scenarios
3. Stress tests for message ordering
4. Idempotency tests

### 10. Migration Path

**Backward Compatibility:**
- Keep PubSub for order announcements
- New protocol for direct swap messages
- Old nodes will timeout if they can't receive direct messages
- Version negotiation during swap init

**Gradual Rollout:**
1. Deploy new code with both PubSub and direct messaging
2. Use feature flag to enable direct messaging
3. Monitor for issues
4. Remove PubSub for swap messages after validation

### 11. Timeouts & Limits

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| ACK timeout | 30 seconds | Allow for slow connections |
| Initial retry interval | 10 seconds | Quick first retry |
| Max retry interval | 10 minutes | Balance between responsiveness and not hammering |
| Backoff multiplier | 2.0 | 10s → 20s → 40s → 80s → 160s → 320s → 600s (10m max) |
| Stop before expiry | 1 hour | Leave time for refund if swap fails |
| Max retries | None (timeout-based) | Retry until swap expires minus buffer |
| Outbox cleanup | 7 days | Keep history for debugging |

**Retry Strategy - Tied to Swap Timeout:**

```go
type RetryConfig struct {
    InitialInterval   time.Duration // 10 seconds
    MaxInterval       time.Duration // 10 minutes
    BackoffMultiplier float64       // 2.0
    StopBeforeExpiry  time.Duration // 1 hour buffer for refund
}

// Retry until 1 hour before swap timeout
func (w *RetryWorker) shouldRetry(msg *OutboxMessage, swapTimeout time.Time) bool {
    deadline := swapTimeout.Add(-w.config.StopBeforeExpiry)
    return time.Now().Before(deadline)
}
```

**Coverage by Swap Timeout:**

| Swap Timeout | Retry Until | Approx Retries | Coverage |
|--------------|-------------|----------------|----------|
| 12 hours | 11 hours | ~70 at 10m max | Full |
| 24 hours | 23 hours | ~140 at 10m max | Full |
| 48 hours | 47 hours | ~280 at 10m max | Full |

### 12. Security Considerations

- **Message Authentication:** All messages signed by sender's peer key
- **Replay Protection:** Message IDs + sequence numbers prevent replay
- **Peer Verification:** Verify sender matches expected counterparty for trade
- **DoS Protection:** Rate limit incoming messages per peer
- **Privacy:** Direct streams encrypted by libp2p noise protocol

### 13. Monitoring & Observability

Add metrics for:
- Messages sent/received per type
- ACK success/failure rate
- Retry counts
- Delivery latency
- Peer online/offline events
- Outbox queue depth

### 14. Files to Create/Modify

**New Files:**
- `internal/node/stream_handler.go` - Direct stream protocol
- `internal/node/message_sender.go` - Outbound message handling
- `internal/node/retry_worker.go` - Background retry logic
- `internal/node/peer_monitor.go` - Connection event handling
- `internal/storage/message_queue.go` - Outbox/inbox persistence

**Modified Files:**
- `internal/node/swap_handler.go` - Add message ID, sequence fields
- `internal/node/node.go` - Register stream handler
- `internal/rpc/swap_*.go` - Use SendDirect instead of SendMessage
- `internal/storage/storage.go` - Add message queue tables
