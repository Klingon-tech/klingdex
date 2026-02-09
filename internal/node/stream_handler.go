// Package node - Direct P2P stream handler for private swap messages.
package node

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// SwapDirectProtocol is the protocol ID for direct swap messages.
const SwapDirectProtocol protocol.ID = "/klingon/swap/direct/1.0.0"

// StreamHandler handles incoming direct P2P streams for swap messages.
type StreamHandler struct {
	node    *Node
	storage *storage.Storage
	log     *logging.Logger

	handlers map[string]SwapMessageHandler
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewStreamHandler creates a new direct stream handler.
func NewStreamHandler(n *Node, store *storage.Storage) *StreamHandler {
	ctx, cancel := context.WithCancel(context.Background())

	return &StreamHandler{
		node:     n,
		storage:  store,
		log:      logging.GetDefault().Component("stream-handler"),
		handlers: make(map[string]SwapMessageHandler),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start registers the stream handler with the libp2p host.
func (h *StreamHandler) Start() error {
	h.node.Host().SetStreamHandler(SwapDirectProtocol, h.handleStream)
	h.log.Info("Direct stream handler started", "protocol", SwapDirectProtocol)
	return nil
}

// Stop stops the stream handler.
func (h *StreamHandler) Stop() {
	h.cancel()
	h.node.Host().RemoveStreamHandler(SwapDirectProtocol)
	h.log.Info("Direct stream handler stopped")
}

// OnMessage registers a handler for a specific message type.
func (h *StreamHandler) OnMessage(msgType string, handler SwapMessageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[msgType] = handler
}

// handleStream handles an incoming direct stream.
func (h *StreamHandler) handleStream(s network.Stream) {
	defer s.Close()

	remotePeer := s.Conn().RemotePeer()
	h.log.Debug("Incoming direct stream", "peer", shortPeerID(remotePeer))

	// Set read deadline
	s.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Read message
	reader := bufio.NewReader(s)
	msgBytes, err := readLengthPrefixed(reader)
	if err != nil {
		h.log.Warn("Failed to read message", "peer", shortPeerID(remotePeer), "error", err)
		return
	}

	// Parse message
	var msg SwapMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		h.log.Warn("Failed to parse message", "peer", shortPeerID(remotePeer), "error", err)
		return
	}

	h.log.Debug("Received direct message",
		"type", msg.Type,
		"trade_id", msg.TradeID,
		"message_id", msg.MessageID,
		"from", shortPeerID(remotePeer))

	// Check for duplicate (idempotency)
	if msg.MessageID != "" && h.storage != nil {
		isDuplicate, err := h.storage.HasReceivedMessage(msg.MessageID)
		if err != nil {
			h.log.Warn("Failed to check for duplicate", "error", err)
		} else if isDuplicate {
			h.log.Debug("Duplicate message, re-sending ACK", "message_id", msg.MessageID)
			h.sendAck(s, msg.MessageID, msg.SequenceNum, true, "")
			return
		}

		// Record in inbox BEFORE processing (for dedup)
		inboxMsg := &storage.InboxMessage{
			MessageID:   msg.MessageID,
			TradeID:     msg.TradeID,
			PeerID:      remotePeer.String(),
			MessageType: msg.Type,
			SequenceNum: msg.SequenceNum,
		}
		if err := h.storage.RecordReceivedMessage(inboxMsg); err != nil {
			h.log.Warn("Failed to record message", "error", err)
		}

		// Update remote sequence
		if msg.SequenceNum > 0 {
			if err := h.storage.UpdateRemoteSequence(msg.TradeID, msg.SequenceNum); err != nil {
				h.log.Warn("Failed to update remote sequence", "error", err)
			}
		}
	}

	// Get handler
	h.mu.RLock()
	handler, ok := h.handlers[msg.Type]
	h.mu.RUnlock()

	if !ok {
		h.log.Warn("No handler for message type", "type", msg.Type)
		if msg.RequiresAck {
			h.sendAck(s, msg.MessageID, msg.SequenceNum, false, "unknown message type")
		}
		return
	}

	// Process message
	err = handler(h.ctx, &msg)

	// Send ACK if required
	if msg.RequiresAck {
		if err != nil {
			h.log.Debug("Message processing failed", "type", msg.Type, "error", err)
			h.sendAck(s, msg.MessageID, msg.SequenceNum, false, err.Error())
		} else {
			h.sendAck(s, msg.MessageID, msg.SequenceNum, true, "")
		}
	}

	// Mark as processed
	if msg.MessageID != "" && h.storage != nil {
		if err := h.storage.MarkMessageProcessed(msg.MessageID); err != nil {
			h.log.Warn("Failed to mark message processed", "error", err)
		}
		if msg.RequiresAck {
			if err := h.storage.MarkAckSent(msg.MessageID); err != nil {
				h.log.Warn("Failed to mark ACK sent", "error", err)
			}
		}
	}
}

// sendAck sends an acknowledgment message back through the stream.
func (h *StreamHandler) sendAck(s network.Stream, msgID string, seq uint64, success bool, errMsg string) {
	ackPayload := AckPayload{
		MessageID:   msgID,
		SequenceNum: seq,
		Success:     success,
		Error:       errMsg,
	}

	payloadBytes, err := json.Marshal(ackPayload)
	if err != nil {
		h.log.Warn("Failed to marshal ACK payload", "error", err)
		return
	}

	ack := SwapMessage{
		Type:        SwapMsgAck,
		MessageID:   uuid.New().String(),
		SequenceNum: seq,
		Timestamp:   time.Now().Unix(),
		FromPeer:    h.node.ID().String(),
		Payload:     payloadBytes,
	}

	ackBytes, err := json.Marshal(ack)
	if err != nil {
		h.log.Warn("Failed to marshal ACK", "error", err)
		return
	}

	s.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := writeLengthPrefixed(s, ackBytes); err != nil {
		h.log.Warn("Failed to send ACK", "error", err)
	}
}

// =============================================================================
// Length-prefixed message framing utilities
// =============================================================================

const maxMessageSize = 1024 * 1024 // 1MB max message size

// readLengthPrefixed reads a length-prefixed message from the reader.
func readLengthPrefixed(r io.Reader) ([]byte, error) {
	// Read 4-byte length prefix (big endian)
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	if length > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d > %d", length, maxMessageSize)
	}

	// Read message body
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	return data, nil
}

// writeLengthPrefixed writes a length-prefixed message to the writer.
func writeLengthPrefixed(w io.Writer, data []byte) error {
	if len(data) > maxMessageSize {
		return fmt.Errorf("message too large: %d > %d", len(data), maxMessageSize)
	}

	// Write 4-byte length prefix (big endian)
	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write message body
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// =============================================================================
// Direct message sending
// =============================================================================

// SendDirectMessage sends a message directly to a peer and waits for ACK.
// This is a blocking call that returns when ACK is received or timeout occurs.
func (h *StreamHandler) SendDirectMessage(ctx context.Context, peerID peer.ID, msg *SwapMessage) error {
	// Open stream to peer
	stream, err := h.node.Host().NewStream(ctx, peerID, SwapDirectProtocol)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Set write deadline
	stream.SetWriteDeadline(time.Now().Add(30 * time.Second))

	// Ensure message has required fields
	if msg.MessageID == "" {
		msg.MessageID = uuid.New().String()
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}
	msg.FromPeer = h.node.ID().String()

	// Marshal and send message
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := writeLengthPrefixed(stream, msgBytes); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// If no ACK required, we're done
	if !msg.RequiresAck {
		return nil
	}

	// Wait for ACK
	stream.SetReadDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(stream)
	ackBytes, err := readLengthPrefixed(reader)
	if err != nil {
		return fmt.Errorf("failed to read ACK: %w", err)
	}

	// Parse ACK
	var ackMsg SwapMessage
	if err := json.Unmarshal(ackBytes, &ackMsg); err != nil {
		return fmt.Errorf("failed to parse ACK: %w", err)
	}

	if ackMsg.Type != SwapMsgAck {
		return fmt.Errorf("unexpected response type: %s", ackMsg.Type)
	}

	var ack AckPayload
	if err := json.Unmarshal(ackMsg.Payload, &ack); err != nil {
		return fmt.Errorf("failed to parse ACK payload: %w", err)
	}

	if !ack.Success {
		return fmt.Errorf("message rejected by peer: %s", ack.Error)
	}

	h.log.Debug("Message delivered successfully",
		"type", msg.Type,
		"trade_id", msg.TradeID,
		"message_id", msg.MessageID)

	return nil
}
