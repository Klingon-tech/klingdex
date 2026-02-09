// Package node - Swap message handler for P2P swap protocol.
package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// PubSub topics for swap messages.
const (
	// SwapTopic is for public swap messages (order announcements).
	SwapTopic = "/klingon/swap/1.0.0"

	// SwapEncryptedTopic is for encrypted private swap messages.
	// Messages are encrypted with recipient's public key, broadcast via gossip,
	// but only the recipient can decrypt them.
	SwapEncryptedTopic = "/klingon/swap/encrypted/1.0.0"

	// Note: SwapDirectProtocol is defined in stream_handler.go
)

// SwapMessage represents a swap protocol message.
type SwapMessage struct {
	Type      string          `json:"type"`       // Message type
	TradeID   string          `json:"trade_id"`   // Trade identifier
	OrderID   string          `json:"order_id"`   // Order identifier (for take messages)
	FromPeer  string          `json:"from_peer"`  // Sender peer ID
	Payload   json.RawMessage `json:"payload"`    // Type-specific payload
	Timestamp int64           `json:"timestamp"`  // Unix timestamp

	// Delivery guarantee fields (for direct P2P messaging)
	MessageID   string `json:"message_id,omitempty"`   // UUID for deduplication
	SequenceNum uint64 `json:"sequence_num,omitempty"` // Per-trade sequence number
	RequiresAck bool   `json:"requires_ack,omitempty"` // Whether sender expects ACK
	SwapTimeout int64  `json:"swap_timeout,omitempty"` // When swap expires (for retry decision)
}

// AckPayload is the acknowledgment message payload.
type AckPayload struct {
	MessageID   string `json:"message_id"`          // Which message we're ACKing
	SequenceNum uint64 `json:"sequence_num"`        // Sequence number ACKed
	Success     bool   `json:"success"`             // Processing successful
	Error       string `json:"error,omitempty"`     // Error if failed
}

// Swap message types
const (
	SwapMsgOrderAnnounce  = "order_announce"
	SwapMsgOrderCancel    = "order_cancel"
	SwapMsgOrderTake      = "order_take"
	SwapMsgOrderTaken     = "order_taken"
	SwapMsgSwapInit       = "swap_init"
	SwapMsgSwapAccept     = "swap_accept"
	SwapMsgPubKeyExchange = "pubkey_exchange"
	SwapMsgNonceExchange  = "nonce_exchange"
	SwapMsgFundingInfo    = "funding_info"
	SwapMsgPartialSig     = "partial_sig"
	SwapMsgComplete       = "complete"
	SwapMsgRefund         = "refund"
	SwapMsgAbort          = "abort"

	// HTLC-specific message types (Bitcoin-family)
	SwapMsgHTLCSecretHash   = "htlc_secret_hash"   // Initiator sends secret hash to responder
	SwapMsgHTLCSecretReveal = "htlc_secret_reveal" // Initiator reveals secret for claiming
	SwapMsgHTLCClaim        = "htlc_claim"         // Notify counterparty of claim tx

	// EVM HTLC-specific message types
	SwapMsgEVMFundingInfo = "evm_funding_info" // EVM HTLC created on-chain (tx hash, swap ID)
	SwapMsgEVMClaimed     = "evm_claimed"      // EVM HTLC claimed (includes secret)
	SwapMsgEVMRefunded    = "evm_refunded"     // EVM HTLC refunded after timeout

	// Acknowledgment message type
	SwapMsgAck = "ack" // Acknowledgment of message receipt
)

// SwapMessageHandler handles incoming swap messages.
type SwapMessageHandler func(ctx context.Context, msg *SwapMessage) error

// SwapHandler manages swap-related PubSub messaging.
type SwapHandler struct {
	node *Node
	log  *logging.Logger

	// Public topic for order announcements
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	// Encrypted topic for private swap messages
	encryptedTopic *pubsub.Topic
	encryptedSub   *pubsub.Subscription
	encryptor      *MessageEncryptor

	handlers map[string]SwapMessageHandler
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewSwapHandler creates a new swap handler.
func NewSwapHandler(n *Node) (*SwapHandler, error) {
	ctx, cancel := context.WithCancel(context.Background())

	h := &SwapHandler{
		node:     n,
		log:      logging.GetDefault().Component("swap-handler"),
		handlers: make(map[string]SwapMessageHandler),
		ctx:      ctx,
		cancel:   cancel,
	}

	return h, nil
}

// Start starts the swap handler and joins the swap topics.
func (h *SwapHandler) Start() error {
	if h.node.pubsub == nil {
		return fmt.Errorf("pubsub not initialized")
	}

	// Join the public swap topic (for order announcements)
	topic, err := h.node.pubsub.Join(SwapTopic)
	if err != nil {
		return fmt.Errorf("failed to join swap topic: %w", err)
	}
	h.topic = topic

	// Subscribe to public messages
	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to swap topic: %w", err)
	}
	h.sub = sub

	// Join the encrypted swap topic (for private swap messages)
	encTopic, err := h.node.pubsub.Join(SwapEncryptedTopic)
	if err != nil {
		return fmt.Errorf("failed to join encrypted swap topic: %w", err)
	}
	h.encryptedTopic = encTopic

	// Subscribe to encrypted messages
	encSub, err := encTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to encrypted swap topic: %w", err)
	}
	h.encryptedSub = encSub

	// Create encryptor for handling encrypted messages
	privKey := h.node.Host().Peerstore().PrivKey(h.node.ID())
	if privKey != nil {
		enc, err := NewMessageEncryptor(privKey, h.node.ID())
		if err != nil {
			h.log.Warn("Failed to create encryptor", "error", err)
		} else {
			h.encryptor = enc
		}
	}

	// Start message processing loops
	go h.processMessages()
	go h.processEncryptedMessages()

	h.log.Info("Swap handler started",
		"public_topic", SwapTopic,
		"encrypted_topic", SwapEncryptedTopic)
	return nil
}

// GetEncryptedTopic returns the encrypted topic for direct publishing.
func (h *SwapHandler) GetEncryptedTopic() *pubsub.Topic {
	return h.encryptedTopic
}

// Stop stops the swap handler.
func (h *SwapHandler) Stop() error {
	h.cancel()

	if h.sub != nil {
		h.sub.Cancel()
	}
	if h.topic != nil {
		h.topic.Close()
	}
	if h.encryptedSub != nil {
		h.encryptedSub.Cancel()
	}
	if h.encryptedTopic != nil {
		h.encryptedTopic.Close()
	}

	h.log.Info("Swap handler stopped")
	return nil
}

// OnMessage registers a handler for a specific message type.
func (h *SwapHandler) OnMessage(msgType string, handler SwapMessageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[msgType] = handler
}

// SendMessage sends a swap message to the network.
func (h *SwapHandler) SendMessage(ctx context.Context, msg *SwapMessage) error {
	if h.topic == nil {
		return fmt.Errorf("not connected to swap topic")
	}

	// Set sender
	msg.FromPeer = h.node.ID().String()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := h.topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	h.log.Debug("Sent swap message", "type", msg.Type, "trade_id", msg.TradeID)
	return nil
}

// processMessages processes incoming swap messages.
func (h *SwapHandler) processMessages() {
	for {
		msg, err := h.sub.Next(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				return // Context cancelled, shutting down
			}
			h.log.Warn("Error receiving message", "error", err)
			continue
		}

		// Don't process our own messages
		if msg.ReceivedFrom == h.node.ID() {
			continue
		}

		// Parse message
		var swapMsg SwapMessage
		if err := json.Unmarshal(msg.Data, &swapMsg); err != nil {
			h.log.Warn("Failed to parse swap message", "error", err)
			continue
		}

		// Get handler
		h.mu.RLock()
		handler, ok := h.handlers[swapMsg.Type]
		h.mu.RUnlock()

		if !ok {
			h.log.Debug("No handler for message type", "type", swapMsg.Type)
			continue
		}

		// Handle message
		h.log.Debug("Received swap message", "type", swapMsg.Type, "from", shortPeerID(msg.ReceivedFrom))

		go func() {
			if err := handler(h.ctx, &swapMsg); err != nil {
				h.log.Warn("Error handling swap message", "type", swapMsg.Type, "error", err)
			}
		}()
	}
}

// processEncryptedMessages processes incoming encrypted swap messages.
// These are messages encrypted with our public key, broadcast via PubSub gossip.
func (h *SwapHandler) processEncryptedMessages() {
	for {
		msg, err := h.encryptedSub.Next(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				return // Context cancelled, shutting down
			}
			h.log.Warn("Error receiving encrypted message", "error", err)
			continue
		}

		// Don't process our own messages
		if msg.ReceivedFrom == h.node.ID() {
			continue
		}

		// Parse envelope
		var envelope EncryptedEnvelope
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			h.log.Debug("Failed to parse encrypted envelope", "error", err)
			continue
		}

		// Check if message is for us
		if h.encryptor == nil || !h.encryptor.IsForUs(&envelope) {
			// Not for us, ignore (this is normal - all peers receive all gossip)
			continue
		}

		// Decrypt the message
		swapMsg, err := h.encryptor.Decrypt(&envelope)
		if err != nil {
			h.log.Warn("Failed to decrypt message", "error", err, "from", envelope.SenderPeerID[:12])
			continue
		}

		h.log.Debug("Received encrypted message",
			"type", swapMsg.Type,
			"trade_id", swapMsg.TradeID,
			"message_id", swapMsg.MessageID,
			"from", envelope.SenderPeerID[:12])

		// Get handler for this message type
		h.mu.RLock()
		handler, ok := h.handlers[swapMsg.Type]
		h.mu.RUnlock()

		if !ok {
			h.log.Debug("No handler for encrypted message type", "type", swapMsg.Type)
			continue
		}

		// Handle message
		go func(env EncryptedEnvelope, sMsg *SwapMessage) {
			if err := handler(h.ctx, sMsg); err != nil {
				h.log.Warn("Error handling encrypted message", "type", sMsg.Type, "error", err)
				// Send NACK if message required ACK
				if sMsg.RequiresAck {
					h.sendEncryptedAck(env.SenderPeerID, sMsg.MessageID, sMsg.SequenceNum, false, err.Error())
				}
				return
			}

			// Send ACK if required
			if sMsg.RequiresAck {
				h.sendEncryptedAck(env.SenderPeerID, sMsg.MessageID, sMsg.SequenceNum, true, "")
			}
		}(envelope, swapMsg)
	}
}

// sendEncryptedAck sends an encrypted ACK back to the sender via PubSub.
func (h *SwapHandler) sendEncryptedAck(senderPeerIDStr string, messageID string, seq uint64, success bool, errMsg string) {
	if h.encryptor == nil || h.encryptedTopic == nil {
		return
	}

	senderPeerID, err := peer.Decode(senderPeerIDStr)
	if err != nil {
		h.log.Warn("Invalid sender peer ID for ACK", "peer", senderPeerIDStr)
		return
	}

	// Create ACK message
	ackPayload := AckPayload{
		MessageID:   messageID,
		SequenceNum: seq,
		Success:     success,
		Error:       errMsg,
	}

	payloadBytes, err := json.Marshal(ackPayload)
	if err != nil {
		h.log.Warn("Failed to marshal ACK payload", "error", err)
		return
	}

	ackMsg := &SwapMessage{
		Type:      SwapMsgAck,
		Payload:   payloadBytes,
		FromPeer:  h.node.ID().String(),
		MessageID: messageID,
	}

	// Encrypt and send ACK
	envelope, err := h.encryptor.Encrypt(senderPeerID, ackMsg)
	if err != nil {
		h.log.Warn("Failed to encrypt ACK", "error", err)
		return
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		h.log.Warn("Failed to marshal ACK envelope", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()

	if err := h.encryptedTopic.Publish(ctx, envelopeBytes); err != nil {
		h.log.Warn("Failed to publish ACK", "error", err)
	}

	h.log.Debug("Sent encrypted ACK", "message_id", messageID, "success", success)
}

func shortPeerID(p peer.ID) string {
	s := p.String()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// Helper functions for creating common messages

// NewOrderAnnounceMessage creates an order announcement message.
func NewOrderAnnounceMessage(orderID string, payload interface{}) (*SwapMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgOrderAnnounce,
		OrderID: orderID,
		Payload: data,
	}, nil
}

// NewOrderTakeMessage creates an order take message.
func NewOrderTakeMessage(orderID, tradeID string, payload interface{}) (*SwapMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgOrderTake,
		OrderID: orderID,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewSwapMessage creates a generic swap message.
func NewSwapMessage(msgType, tradeID string, payload interface{}) (*SwapMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    msgType,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// PubKeyExchangePayload contains public key exchange data.
type PubKeyExchangePayload struct {
	PubKey            string `json:"pubkey"`              // Hex-encoded public key
	OfferWalletAddr   string `json:"offer_wallet_addr"`   // Our address on offer chain for receiving
	RequestWalletAddr string `json:"request_wallet_addr"` // Our address on request chain for receiving
}

// NonceExchangePayload contains nonce exchange data for both chains.
type NonceExchangePayload struct {
	OfferNonce   string `json:"offer_nonce"`   // Hex-encoded public nonce for offer chain (66 bytes)
	RequestNonce string `json:"request_nonce"` // Hex-encoded public nonce for request chain (66 bytes)
}

// FundingInfoPayload contains funding transaction info.
type FundingInfoPayload struct {
	TxID string `json:"txid"`
	Vout uint32 `json:"vout"`
}

// PartialSigPayload contains partial signature data for both chains.
type PartialSigPayload struct {
	OfferPartialSig   string `json:"offer_partial_sig"`   // Hex-encoded partial signature for offer chain
	RequestPartialSig string `json:"request_partial_sig"` // Hex-encoded partial signature for request chain
}

// HTLC-specific payloads

// HTLCSecretHashPayload contains the secret hash for HTLC swaps.
// Sent by the initiator to the responder after swap initialization.
type HTLCSecretHashPayload struct {
	SecretHash        string `json:"secret_hash"`         // Hex-encoded SHA256 hash (32 bytes)
	PubKey            string `json:"pubkey"`              // Initiator's hex-encoded pubkey (for HTLC script)
	OfferWalletAddr   string `json:"offer_wallet_addr"`   // Initiator's address on offer chain for receiving
	RequestWalletAddr string `json:"request_wallet_addr"` // Initiator's address on request chain for receiving
}

// HTLCSecretRevealPayload contains the secret for claiming HTLC outputs.
// Sent by the initiator when ready to complete the swap.
type HTLCSecretRevealPayload struct {
	Secret string `json:"secret"` // Hex-encoded preimage (32 bytes)
}

// HTLCClaimPayload notifies the counterparty of a claim transaction.
// Allows both parties to track swap completion on-chain.
type HTLCClaimPayload struct {
	Chain  string `json:"chain"`  // Chain symbol (BTC, LTC)
	TxID   string `json:"txid"`   // Claim transaction ID
	Secret string `json:"secret"` // Hex-encoded secret used for claim (for counterparty to use)
}

// NewHTLCSecretHashMessage creates a secret hash message for HTLC swaps.
func NewHTLCSecretHashMessage(tradeID, secretHash, pubKey, offerAddr, requestAddr string) (*SwapMessage, error) {
	payload := HTLCSecretHashPayload{
		SecretHash:        secretHash,
		PubKey:            pubKey,
		OfferWalletAddr:   offerAddr,
		RequestWalletAddr: requestAddr,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgHTLCSecretHash,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewHTLCSecretRevealMessage creates a secret reveal message for HTLC swaps.
func NewHTLCSecretRevealMessage(tradeID string, secret string) (*SwapMessage, error) {
	payload := HTLCSecretRevealPayload{
		Secret: secret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgHTLCSecretReveal,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewPubKeyExchangeMessage creates a pubkey exchange message with wallet addresses.
// This is used by both parties to exchange their receiving addresses.
func NewPubKeyExchangeMessage(tradeID, pubKey, offerAddr, requestAddr string) (*SwapMessage, error) {
	payload := PubKeyExchangePayload{
		PubKey:            pubKey,
		OfferWalletAddr:   offerAddr,
		RequestWalletAddr: requestAddr,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgPubKeyExchange,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewHTLCClaimMessage creates a claim notification message for HTLC swaps.
func NewHTLCClaimMessage(tradeID, chain, txid, secret string) (*SwapMessage, error) {
	payload := HTLCClaimPayload{
		Chain:  chain,
		TxID:   txid,
		Secret: secret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgHTLCClaim,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// =============================================================================
// EVM HTLC Payloads
// =============================================================================

// EVMFundingInfoPayload contains EVM HTLC creation info.
// Sent when an EVM HTLC is created on-chain to notify counterparty.
type EVMFundingInfoPayload struct {
	Chain           string `json:"chain"`            // Chain symbol (ETH, BSC, etc.)
	ChainID         uint64 `json:"chain_id"`         // EVM chain ID
	TxHash          string `json:"tx_hash"`          // Creation transaction hash
	SwapID          string `json:"swap_id"`          // On-chain swap ID (keccak256 hash)
	ContractAddress string `json:"contract_address"` // HTLC contract address
	Sender          string `json:"sender"`           // Sender address (initiator)
	Receiver        string `json:"receiver"`         // Receiver address (responder)
	TokenAddress    string `json:"token_address"`    // Token address (0x0 for native)
	Amount          string `json:"amount"`           // Amount in wei (string for big.Int)
	SecretHash      string `json:"secret_hash"`      // Hex-encoded secret hash
	Timelock        int64  `json:"timelock"`         // Unix timestamp when refund becomes available
}

// EVMClaimPayload notifies the counterparty of an EVM HTLC claim.
// Includes the secret so counterparty can claim their side.
type EVMClaimPayload struct {
	Chain   string `json:"chain"`    // Chain symbol (ETH, BSC, etc.)
	ChainID uint64 `json:"chain_id"` // EVM chain ID
	TxHash  string `json:"tx_hash"`  // Claim transaction hash
	SwapID  string `json:"swap_id"`  // On-chain swap ID
	Secret  string `json:"secret"`   // Hex-encoded secret (32 bytes)
}

// EVMRefundPayload notifies the counterparty of an EVM HTLC refund.
// Indicates the swap was not completed and funds were returned.
type EVMRefundPayload struct {
	Chain   string `json:"chain"`    // Chain symbol (ETH, BSC, etc.)
	ChainID uint64 `json:"chain_id"` // EVM chain ID
	TxHash  string `json:"tx_hash"`  // Refund transaction hash
	SwapID  string `json:"swap_id"`  // On-chain swap ID
}

// NewEVMFundingInfoMessage creates an EVM HTLC funding info message.
func NewEVMFundingInfoMessage(tradeID string, payload *EVMFundingInfoPayload) (*SwapMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgEVMFundingInfo,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewEVMClaimMessage creates an EVM HTLC claim notification message.
func NewEVMClaimMessage(tradeID, chain string, chainID uint64, txHash, swapID, secret string) (*SwapMessage, error) {
	payload := EVMClaimPayload{
		Chain:   chain,
		ChainID: chainID,
		TxHash:  txHash,
		SwapID:  swapID,
		Secret:  secret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgEVMClaimed,
		TradeID: tradeID,
		Payload: data,
	}, nil
}

// NewEVMRefundMessage creates an EVM HTLC refund notification message.
func NewEVMRefundMessage(tradeID, chain string, chainID uint64, txHash, swapID string) (*SwapMessage, error) {
	payload := EVMRefundPayload{
		Chain:   chain,
		ChainID: chainID,
		TxHash:  txHash,
		SwapID:  swapID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &SwapMessage{
		Type:    SwapMsgEVMRefunded,
		TradeID: tradeID,
		Payload: data,
	}, nil
}
