// Package rpc - Nonce exchange handler.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Klingon-tech/klingdex/internal/node"
)

// swapExchangeNonce generates and exchanges nonces for both chains.
func (s *Server) swapExchangeNonce(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapExchangeNonceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	// Check if we need to re-broadcast pubkey (in case counterparty missed our initial broadcast)
	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	// Always re-send our pubkey+wallet addresses when exchanging nonces.
	// This ensures counterparty receives our addresses even if they missed the initial message.
	if activeSwap.MuSig2 != nil && activeSwap.MuSig2.LocalPrivKey != nil {
		pubKeyHex := hex.EncodeToString(activeSwap.MuSig2.LocalPrivKey.PubKey().SerializeCompressed())
		payload := &node.PubKeyExchangePayload{
			PubKey:            pubKeyHex,
			OfferWalletAddr:   activeSwap.Swap.LocalOfferWalletAddr,
			RequestWalletAddr: activeSwap.Swap.LocalRequestWalletAddr,
		}
		msg, msgErr := node.NewSwapMessage(node.SwapMsgPubKeyExchange, p.TradeID, payload)
		if msgErr == nil {
			if sendErr := s.sendDirectToCounterparty(ctx, p.TradeID, msg); sendErr != nil {
				s.log.Warn("Failed to re-send pubkey", "error", sendErr)
			} else {
				s.log.Debug("Re-sent pubkey with wallet addresses", "trade_id", p.TradeID[:8])
			}
		}
	}

	// Generate nonces for BOTH chains
	offerNonce, requestNonce, err := s.coordinator.GenerateNonces(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonces: %w", err)
	}

	// Send both nonces to counterparty (direct P2P)
	payload := &node.NonceExchangePayload{
		OfferNonce:   hex.EncodeToString(offerNonce),
		RequestNonce: hex.EncodeToString(requestNonce),
	}
	msg, err := node.NewSwapMessage(node.SwapMsgNonceExchange, p.TradeID, payload)
	if err == nil {
		if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
			s.log.Warn("Failed to send nonces", "trade_id", p.TradeID, "error", err)
		} else {
			s.log.Info("Sent nonces for both chains to counterparty", "trade_id", p.TradeID[:8])
		}
	}

	// Check if we have the remote nonces for both chains (re-fetch to get latest state)
	activeSwap, _ = s.coordinator.GetSwap(p.TradeID)
	hasRemoteNonces := activeSwap != nil && activeSwap.MuSig2 != nil &&
		activeSwap.MuSig2.OfferChain != nil && activeSwap.MuSig2.OfferChain.RemoteNonce != nil &&
		activeSwap.MuSig2.RequestChain != nil && activeSwap.MuSig2.RequestChain.RemoteNonce != nil

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("nonces_generated", map[string]interface{}{
			"trade_id":          p.TradeID,
			"has_remote_nonces": hasRemoteNonces,
		})
	}

	return &SwapExchangeNonceResult{
		TradeID:         p.TradeID,
		OfferNonce:      hex.EncodeToString(offerNonce),
		RequestNonce:    hex.EncodeToString(requestNonce),
		HasRemoteNonces: hasRemoteNonces,
	}, nil
}
