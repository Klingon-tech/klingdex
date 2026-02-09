package node

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestWriteLengthPrefixed(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty message",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "small message",
			data:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "json message",
			data:    []byte(`{"type":"test","trade_id":"123"}`),
			wantErr: false,
		},
		{
			name:    "binary data",
			data:    []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := writeLengthPrefixed(&buf, tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("writeLengthPrefixed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the length prefix is correct
				result := buf.Bytes()
				if len(result) < 4 {
					t.Fatalf("expected at least 4 bytes, got %d", len(result))
				}

				length := binary.BigEndian.Uint32(result[:4])
				if int(length) != len(tt.data) {
					t.Errorf("length prefix = %d, want %d", length, len(tt.data))
				}

				// Verify the data matches
				if !bytes.Equal(result[4:], tt.data) {
					t.Errorf("data mismatch: got %v, want %v", result[4:], tt.data)
				}
			}
		})
	}
}

func TestWriteLengthPrefixedTooLarge(t *testing.T) {
	// Create a message larger than maxMessageSize
	largeData := make([]byte, maxMessageSize+1)
	var buf bytes.Buffer

	err := writeLengthPrefixed(&buf, largeData)
	if err == nil {
		t.Error("expected error for message exceeding max size")
	}
}

func TestReadLengthPrefixed(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty message",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "small message",
			data:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "json message",
			data:    []byte(`{"type":"test","trade_id":"123"}`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First write the message
			var buf bytes.Buffer
			if err := writeLengthPrefixed(&buf, tt.data); err != nil {
				t.Fatalf("failed to write test data: %v", err)
			}

			// Then read it back
			reader := bytes.NewReader(buf.Bytes())
			result, err := readLengthPrefixed(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("readLengthPrefixed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !bytes.Equal(result, tt.data) {
				t.Errorf("data mismatch: got %v, want %v", result, tt.data)
			}
		})
	}
}

func TestReadLengthPrefixedTooLarge(t *testing.T) {
	// Create a fake length header that exceeds max size
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(maxMessageSize+1))
	buf.Write([]byte("some data"))

	reader := bytes.NewReader(buf.Bytes())
	_, err := readLengthPrefixed(reader)
	if err == nil {
		t.Error("expected error for message exceeding max size")
	}
}

func TestReadLengthPrefixedTruncated(t *testing.T) {
	// Create a header that says 100 bytes but only provide 10
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(100))
	buf.Write([]byte("short"))

	reader := bytes.NewReader(buf.Bytes())
	_, err := readLengthPrefixed(reader)
	if err == nil {
		t.Error("expected error for truncated message")
	}
}

func TestReadLengthPrefixedNoHeader(t *testing.T) {
	// Empty buffer - can't read length
	reader := bytes.NewReader([]byte{})
	_, err := readLengthPrefixed(reader)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestRoundTripSwapMessage(t *testing.T) {
	msg := SwapMessage{
		Type:        SwapMsgPubKeyExchange,
		TradeID:     "trade-123",
		MessageID:   "msg-456",
		FromPeer:    "peer-789",
		Timestamp:   1234567890,
		SequenceNum: 5,
		RequiresAck: true,
		SwapTimeout: 1234567990,
		Payload:     json.RawMessage(`{"pubkey":"02abc..."}`),
	}

	// Marshal to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	// Write with length prefix
	var buf bytes.Buffer
	if err := writeLengthPrefixed(&buf, msgBytes); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// Read back
	reader := bytes.NewReader(buf.Bytes())
	readBytes, err := readLengthPrefixed(reader)
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	// Unmarshal
	var readMsg SwapMessage
	if err := json.Unmarshal(readBytes, &readMsg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	// Verify fields
	if readMsg.Type != msg.Type {
		t.Errorf("Type = %s, want %s", readMsg.Type, msg.Type)
	}
	if readMsg.TradeID != msg.TradeID {
		t.Errorf("TradeID = %s, want %s", readMsg.TradeID, msg.TradeID)
	}
	if readMsg.MessageID != msg.MessageID {
		t.Errorf("MessageID = %s, want %s", readMsg.MessageID, msg.MessageID)
	}
	if readMsg.FromPeer != msg.FromPeer {
		t.Errorf("FromPeer = %s, want %s", readMsg.FromPeer, msg.FromPeer)
	}
	if readMsg.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp = %d, want %d", readMsg.Timestamp, msg.Timestamp)
	}
	if readMsg.SequenceNum != msg.SequenceNum {
		t.Errorf("SequenceNum = %d, want %d", readMsg.SequenceNum, msg.SequenceNum)
	}
	if readMsg.RequiresAck != msg.RequiresAck {
		t.Errorf("RequiresAck = %v, want %v", readMsg.RequiresAck, msg.RequiresAck)
	}
	if readMsg.SwapTimeout != msg.SwapTimeout {
		t.Errorf("SwapTimeout = %d, want %d", readMsg.SwapTimeout, msg.SwapTimeout)
	}
}

func TestRoundTripAckPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload AckPayload
	}{
		{
			name: "success ack",
			payload: AckPayload{
				MessageID:   "msg-123",
				SequenceNum: 5,
				Success:     true,
				Error:       "",
			},
		},
		{
			name: "failure ack",
			payload: AckPayload{
				MessageID:   "msg-456",
				SequenceNum: 10,
				Success:     false,
				Error:       "processing failed: invalid signature",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Unmarshal back
			var result AckPayload
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Verify
			if result.MessageID != tt.payload.MessageID {
				t.Errorf("MessageID = %s, want %s", result.MessageID, tt.payload.MessageID)
			}
			if result.SequenceNum != tt.payload.SequenceNum {
				t.Errorf("SequenceNum = %d, want %d", result.SequenceNum, tt.payload.SequenceNum)
			}
			if result.Success != tt.payload.Success {
				t.Errorf("Success = %v, want %v", result.Success, tt.payload.Success)
			}
			if result.Error != tt.payload.Error {
				t.Errorf("Error = %s, want %s", result.Error, tt.payload.Error)
			}
		})
	}
}

func TestSwapMessageTypes(t *testing.T) {
	// Verify all message type constants are defined
	types := []string{
		SwapMsgOrderAnnounce,
		SwapMsgOrderCancel,
		SwapMsgOrderTake,
		SwapMsgOrderTaken,
		SwapMsgSwapInit,
		SwapMsgSwapAccept,
		SwapMsgPubKeyExchange,
		SwapMsgNonceExchange,
		SwapMsgFundingInfo,
		SwapMsgPartialSig,
		SwapMsgComplete,
		SwapMsgRefund,
		SwapMsgAbort,
		SwapMsgHTLCSecretHash,
		SwapMsgHTLCSecretReveal,
		SwapMsgHTLCClaim,
		SwapMsgAck,
	}

	for _, msgType := range types {
		if msgType == "" {
			t.Error("empty message type found")
		}
	}
}

func TestMaxMessageSizeConstant(t *testing.T) {
	// Verify the constant is reasonable (1MB)
	if maxMessageSize != 1024*1024 {
		t.Errorf("maxMessageSize = %d, want %d", maxMessageSize, 1024*1024)
	}
}

func TestMultipleMessagesInSequence(t *testing.T) {
	messages := [][]byte{
		[]byte(`{"type":"msg1"}`),
		[]byte(`{"type":"msg2"}`),
		[]byte(`{"type":"msg3"}`),
	}

	// Write all messages to a buffer
	var buf bytes.Buffer
	for _, msg := range messages {
		if err := writeLengthPrefixed(&buf, msg); err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}

	// Read them back in sequence
	reader := bytes.NewReader(buf.Bytes())
	for i, expected := range messages {
		result, err := readLengthPrefixed(reader)
		if err != nil {
			t.Fatalf("failed to read message %d: %v", i, err)
		}
		if !bytes.Equal(result, expected) {
			t.Errorf("message %d: got %s, want %s", i, result, expected)
		}
	}
}
