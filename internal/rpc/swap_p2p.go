// Package rpc - P2P message handlers for swap protocol.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/Klingon-tech/klingdex/internal/node"
	"github.com/Klingon-tech/klingdex/internal/storage"
)

// =============================================================================
// Direct P2P Messaging Helpers
// =============================================================================

// sendDirectToCounterparty sends a swap message directly to the counterparty peer.
// This uses persistent messaging with retry support for offline peers.
// Falls back to PubSub broadcast if direct messaging is not available.
func (s *Server) sendDirectToCounterparty(ctx context.Context, tradeID string, msg *node.SwapMessage) error {
	// Get the trade to find the counterparty peer ID
	trade, err := s.store.GetTrade(tradeID)
	if err != nil {
		return fmt.Errorf("trade not found: %w", err)
	}

	// Determine counterparty peer ID
	var counterpartyID string
	if trade.MakerPeerID == s.node.ID().String() {
		counterpartyID = trade.TakerPeerID
	} else {
		counterpartyID = trade.MakerPeerID
	}

	// Parse peer ID
	peerID, err := peer.Decode(counterpartyID)
	if err != nil {
		return fmt.Errorf("invalid counterparty peer ID: %w", err)
	}

	// Calculate swap timeout
	// Default to 24 hours from now for swap messages
	// This gives ample time for message delivery while still expiring eventually
	swapTimeout := time.Now().Add(24 * time.Hour).Unix()

	// If swap exists with lock times, use the shorter one as basis
	if activeSwap, err := s.coordinator.GetSwap(tradeID); err == nil && activeSwap != nil {
		// Use the swap's responder lock time as the messaging timeout
		// (responder must complete within this time)
		if activeSwap.Swap.ResponderLock > 0 {
			swapTimeout = activeSwap.Swap.CreatedAt.Add(activeSwap.Swap.ResponderLock).Unix()
		}
	}

	// Try direct messaging first (preferred - private and persistent)
	if s.node.MessageSender() != nil {
		s.log.Debug("Sending direct message",
			"type", msg.Type,
			"trade_id", tradeID[:8],
			"peer", counterpartyID[:12])

		if err := s.node.SendDirect(ctx, peerID, tradeID, swapTimeout, msg); err != nil {
			s.log.Warn("Direct send failed, falling back to PubSub",
				"error", err,
				"type", msg.Type)
			// Fall through to PubSub fallback
		} else {
			return nil // Success via direct messaging
		}
	}

	// Fallback to PubSub broadcast (for backward compatibility)
	if swapHandler := s.node.SwapHandler(); swapHandler != nil {
		s.log.Debug("Using PubSub broadcast fallback", "type", msg.Type, "trade_id", tradeID[:8])
		return swapHandler.SendMessage(ctx, msg)
	}

	return fmt.Errorf("no messaging handler available")
}

// broadcastToAll sends a message via PubSub broadcast (for public messages like orders).
func (s *Server) broadcastToAll(ctx context.Context, msg *node.SwapMessage) error {
	if swapHandler := s.node.SwapHandler(); swapHandler != nil {
		return swapHandler.SendMessage(ctx, msg)
	}
	return fmt.Errorf("no pubsub handler available")
}

// ========================================
// P2P Message Handlers for Swap Protocol
// ========================================

// handlePubKeyExchange processes incoming pubkey exchange messages.
func (s *Server) handlePubKeyExchange(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("PubKey exchange missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.PubKeyExchangePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse pubkey exchange payload", "error", err)
		return nil
	}

	// Decode pubkey (may be empty for EVM-only swaps)
	var pubKeyBytes []byte
	if payload.PubKey != "" {
		var err error
		pubKeyBytes, err = hex.DecodeString(payload.PubKey)
		if err != nil {
			s.log.Warn("Invalid pubkey hex", "error", err)
			// Don't return - wallet addresses are still important
		}
	}

	// Check if we have this trade
	trade, err := s.store.GetTrade(msg.TradeID)
	if err != nil {
		s.log.Debug("Trade not found for pubkey exchange", "trade_id", msg.TradeID)
		return nil
	}

	// Verify the message is from the expected peer and determine if it's from maker or taker
	var fromMaker bool
	expectedPeer := trade.TakerPeerID
	if trade.TakerPeerID == s.node.ID().String() {
		expectedPeer = trade.MakerPeerID
		fromMaker = true
	}
	if msg.FromPeer != expectedPeer {
		s.log.Warn("PubKey from unexpected peer", "expected", expectedPeer[:12], "got", msg.FromPeer[:12])
		return nil
	}

	// Store the remote pubkey in the trade record (if provided)
	if payload.PubKey != "" {
		if err := s.store.UpdateTradePubKey(msg.TradeID, fromMaker, payload.PubKey); err != nil {
			s.log.Warn("Failed to store remote pubkey in trade", "error", err)
		}
	}

	// Set the remote pubkey in coordinator (if provided and swap exists)
	if len(pubKeyBytes) > 0 {
		if err := s.coordinator.SetRemotePubKey(msg.TradeID, pubKeyBytes); err != nil {
			// Swap might not be initialized yet - that's OK, addresses are more important for EVM swaps
			s.log.Debug("Failed to set remote pubkey, swap may not be initialized", "error", err)
		}
	}

	// Set the remote wallet addresses (critical for EVM swaps)
	if payload.OfferWalletAddr != "" || payload.RequestWalletAddr != "" {
		if err := s.coordinator.SetRemoteWalletAddresses(msg.TradeID, payload.OfferWalletAddr, payload.RequestWalletAddr); err != nil {
			s.log.Warn("Failed to set remote wallet addresses", "error", err)
		} else {
			s.log.Info("Stored remote wallet addresses",
				"trade_id", msg.TradeID[:8],
				"offer_addr", payload.OfferWalletAddr,
				"request_addr", payload.RequestWalletAddr,
			)
		}
	}

	// Get the swap addresses for both chains (works for both MuSig2 and HTLC)
	offerAddr, requestAddr, err := s.coordinator.GetSwapAddresses(msg.TradeID)
	if err != nil {
		s.log.Warn("Failed to get swap addresses after pubkey exchange", "error", err)
		return nil
	}

	s.log.Info("Received counterparty pubkey",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
		"offer_addr", offerAddr,
		"request_addr", requestAddr,
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("pubkey_received", map[string]string{
			"trade_id":     msg.TradeID,
			"from_peer":    msg.FromPeer,
			"offer_addr":   offerAddr,
			"request_addr": requestAddr,
		})
	}

	return nil
}

// handleNonceExchange processes incoming nonce exchange messages for both chains.
func (s *Server) handleNonceExchange(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("Nonce exchange missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.NonceExchangePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse nonce exchange payload", "error", err)
		return nil
	}

	// Decode offer nonce
	offerNonceBytes, err := hex.DecodeString(payload.OfferNonce)
	if err != nil {
		s.log.Warn("Invalid offer nonce hex", "error", err)
		return nil
	}
	if len(offerNonceBytes) != 66 {
		s.log.Warn("Invalid offer nonce size", "expected", 66, "got", len(offerNonceBytes))
		return nil
	}

	// Decode request nonce
	requestNonceBytes, err := hex.DecodeString(payload.RequestNonce)
	if err != nil {
		s.log.Warn("Invalid request nonce hex", "error", err)
		return nil
	}
	if len(requestNonceBytes) != 66 {
		s.log.Warn("Invalid request nonce size", "expected", 66, "got", len(requestNonceBytes))
		return nil
	}

	// Set both remote nonces in coordinator
	if err := s.coordinator.SetRemoteNonces(msg.TradeID, offerNonceBytes, requestNonceBytes); err != nil {
		s.log.Warn("Failed to set remote nonces", "error", err)
		return nil
	}

	s.log.Info("Received counterparty nonces for both chains",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
	)

	// Check if we have both nonces for both chains and can proceed to funding
	activeSwap, _ := s.coordinator.GetSwap(msg.TradeID)
	if activeSwap != nil && activeSwap.MuSig2 != nil {
		offerReady := activeSwap.MuSig2.OfferChain != nil &&
			activeSwap.MuSig2.OfferChain.LocalNonce != nil &&
			activeSwap.MuSig2.OfferChain.RemoteNonce != nil
		requestReady := activeSwap.MuSig2.RequestChain != nil &&
			activeSwap.MuSig2.RequestChain.LocalNonce != nil &&
			activeSwap.MuSig2.RequestChain.RemoteNonce != nil

		if offerReady && requestReady {
			if err := s.store.UpdateTradeState(msg.TradeID, storage.TradeStateFunding); err != nil {
				s.log.Warn("Failed to update trade state", "error", err)
			}
		}
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("nonces_received", map[string]string{
			"trade_id":  msg.TradeID,
			"from_peer": msg.FromPeer,
		})
	}

	return nil
}

// handleFundingInfo processes incoming funding transaction info.
func (s *Server) handleFundingInfo(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("Funding info missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.FundingInfoPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse funding info payload", "error", err)
		return nil
	}

	// Set remote funding info in coordinator
	if err := s.coordinator.SetFundingTx(msg.TradeID, payload.TxID, payload.Vout, false); err != nil {
		s.log.Warn("Failed to set remote funding tx", "error", err)
		return nil
	}

	s.log.Info("Received counterparty funding info",
		"trade_id", msg.TradeID[:8],
		"txid", payload.TxID[:16],
		"vout", payload.Vout,
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("funding_received", map[string]interface{}{
			"trade_id":  msg.TradeID,
			"txid":      payload.TxID,
			"vout":      payload.Vout,
			"from_peer": msg.FromPeer,
		})
	}

	return nil
}

// handlePartialSig processes incoming partial signatures for BOTH chains.
func (s *Server) handlePartialSig(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("Partial sig missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.PartialSigPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse partial sig payload", "error", err)
		return nil
	}

	// Decode offer chain partial signature
	offerSigBytes, err := hex.DecodeString(payload.OfferPartialSig)
	if err != nil {
		s.log.Warn("Invalid offer partial sig hex", "error", err)
		return nil
	}

	// Decode request chain partial signature
	requestSigBytes, err := hex.DecodeString(payload.RequestPartialSig)
	if err != nil {
		s.log.Warn("Invalid request partial sig hex", "error", err)
		return nil
	}

	s.log.Info("Received counterparty partial signatures for both chains",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
	)

	// Store both remote partial signatures
	if err := s.coordinator.SetRemotePartialSigs(msg.TradeID, offerSigBytes, requestSigBytes); err != nil {
		s.log.Warn("Failed to store remote partial sigs", "error", err)
		return nil
	}

	s.log.Info("Stored remote partial signatures for both chains",
		"trade_id", msg.TradeID[:8],
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("remote_partial_sigs_received", map[string]interface{}{
			"trade_id":            msg.TradeID,
			"offer_partial_sig":   payload.OfferPartialSig,
			"request_partial_sig": payload.RequestPartialSig,
		})
	}

	return nil
}

// ========================================
// HTLC P2P Message Handlers
// ========================================

// handleHTLCSecretHash processes incoming secret hash messages (from initiator).
func (s *Server) handleHTLCSecretHash(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("HTLC secret hash missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.HTLCSecretHashPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse HTLC secret hash payload", "error", err)
		return nil
	}

	// Decode secret hash
	secretHash, err := hex.DecodeString(payload.SecretHash)
	if err != nil {
		s.log.Warn("Invalid secret hash hex", "error", err)
		return nil
	}

	if len(secretHash) != 32 {
		s.log.Warn("Invalid secret hash size", "expected", 32, "got", len(secretHash))
		return nil
	}

	// Store the secret hash and wallet addresses in the secrets table
	// (for use when swap_initCrossChain is called later by the responder)
	secretRecord := &storage.Secret{
		ID:                      fmt.Sprintf("%s-hash", msg.TradeID),
		TradeID:                 msg.TradeID,
		SecretHash:              payload.SecretHash,
		CreatedBy:               storage.SecretCreatorThem,
		RemoteOfferWalletAddr:   payload.OfferWalletAddr,
		RemoteRequestWalletAddr: payload.RequestWalletAddr,
		CreatedAt:               time.Now(),
	}
	if err := s.store.CreateSecret(secretRecord); err != nil {
		// May already exist from previous message, ignore
		s.log.Debug("Secret hash storage", "error", err)
	}

	// Try to set in coordinator (may fail if swap not initialized yet - that's OK)
	if err := s.coordinator.SetRemoteSecretHash(msg.TradeID, secretHash); err != nil {
		s.log.Debug("Coordinator secret hash not set yet (swap not initialized)", "error", err)
	}

	// Set the remote wallet addresses in coordinator if swap exists
	if payload.OfferWalletAddr != "" || payload.RequestWalletAddr != "" {
		if err := s.coordinator.SetRemoteWalletAddresses(msg.TradeID, payload.OfferWalletAddr, payload.RequestWalletAddr); err != nil {
			s.log.Debug("Coordinator wallet addresses not set yet", "error", err)
		}
	}

	// Store the maker's pubkey in the trade record so swap_init can proceed
	if payload.PubKey != "" {
		if err := s.store.UpdateTradePubKey(msg.TradeID, true, payload.PubKey); err != nil {
			s.log.Warn("Failed to store maker pubkey from secret hash", "error", err)
		} else {
			s.log.Info("Stored maker pubkey from HTLC secret hash", "pubkey", payload.PubKey[:16])
		}
	}

	s.log.Info("Received HTLC secret hash from initiator",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("htlc_secret_hash_received", map[string]string{
			"trade_id":    msg.TradeID,
			"from_peer":   msg.FromPeer,
			"secret_hash": payload.SecretHash,
		})
	}

	return nil
}

// handleHTLCSecretReveal processes incoming secret reveal messages (from initiator).
func (s *Server) handleHTLCSecretReveal(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("HTLC secret reveal missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.HTLCSecretRevealPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse HTLC secret reveal payload", "error", err)
		return nil
	}

	// Decode secret
	secret, err := hex.DecodeString(payload.Secret)
	if err != nil {
		s.log.Warn("Invalid secret hex", "error", err)
		return nil
	}

	if len(secret) != 32 {
		s.log.Warn("Invalid secret size", "expected", 32, "got", len(secret))
		return nil
	}

	// Set the revealed secret in coordinator
	if err := s.coordinator.SetRevealedSecret(msg.TradeID, secret); err != nil {
		s.log.Warn("Failed to set revealed secret", "error", err)
		return nil
	}

	s.log.Info("Received HTLC secret from initiator",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("htlc_secret_revealed", map[string]string{
			"trade_id":  msg.TradeID,
			"from_peer": msg.FromPeer,
			"secret":    payload.Secret,
		})
	}

	return nil
}

// handleHTLCClaim processes incoming claim notification messages.
func (s *Server) handleHTLCClaim(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	if msg.TradeID == "" {
		s.log.Warn("HTLC claim missing trade_id")
		return nil
	}

	// Parse payload
	var payload node.HTLCClaimPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse HTLC claim payload", "error", err)
		return nil
	}

	s.log.Info("Received HTLC claim notification",
		"trade_id", msg.TradeID[:8],
		"from", msg.FromPeer[:12],
		"chain", payload.Chain,
		"txid", payload.TxID,
	)

	// If the claim includes the secret, set it in coordinator
	if payload.Secret != "" {
		secret, err := hex.DecodeString(payload.Secret)
		if err == nil && len(secret) == 32 {
			if err := s.coordinator.SetRevealedSecret(msg.TradeID, secret); err != nil {
				s.log.Warn("Failed to set revealed secret from claim", "error", err)
			}
		}
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("htlc_claim_received", map[string]string{
			"trade_id":  msg.TradeID,
			"from_peer": msg.FromPeer,
			"chain":     payload.Chain,
			"txid":      payload.TxID,
		})
	}

	return nil
}
