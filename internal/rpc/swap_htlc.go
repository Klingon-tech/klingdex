// Package rpc - HTLC-specific swap handlers.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/node"
)

// swapHTLCRevealSecret reveals the secret for an HTLC swap (initiator only).
// This broadcasts the secret to the counterparty so they can claim.
func (s *Server) swapHTLCRevealSecret(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapHTLCRevealSecretParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	// Get the secret from coordinator
	secret, err := s.coordinator.RevealSecret(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to reveal secret: %w", err)
	}

	secretHex := hex.EncodeToString(secret)

	// Get the secret hash for reference
	secretHash, _ := s.coordinator.GetSecretHash(p.TradeID)
	secretHashHex := hex.EncodeToString(secretHash)

	// Send secret to counterparty (direct P2P - CRITICAL for swap completion)
	msg, err := node.NewHTLCSecretRevealMessage(p.TradeID, secretHex)
	if err == nil {
		if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
			s.log.Warn("Failed to send secret", "trade_id", p.TradeID, "error", err)
		} else {
			s.log.Info("Sent HTLC secret to counterparty", "trade_id", p.TradeID[:8])
		}
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("htlc_secret_revealed", map[string]string{
			"trade_id":    p.TradeID,
			"secret":      secretHex,
			"secret_hash": secretHashHex,
		})
	}

	return &SwapHTLCRevealSecretResult{
		TradeID:    p.TradeID,
		Secret:     secretHex,
		SecretHash: secretHashHex,
		Message:    "Secret revealed and broadcast",
	}, nil
}

// swapHTLCGetSecret returns the secret for an HTLC swap (if available).
func (s *Server) swapHTLCGetSecret(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapHTLCGetSecretParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	result := &SwapHTLCGetSecretResult{
		TradeID: p.TradeID,
	}

	if len(activeSwap.Swap.Secret) > 0 {
		result.SecretRevealed = true
		result.Secret = hex.EncodeToString(activeSwap.Swap.Secret)
	}

	if len(activeSwap.Swap.SecretHash) > 0 {
		result.SecretHash = hex.EncodeToString(activeSwap.Swap.SecretHash)
	}

	return result, nil
}

// swapHTLCClaim claims an HTLC output using the secret.
func (s *Server) swapHTLCClaim(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapHTLCClaimParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	txID, err := s.coordinator.ClaimHTLC(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to claim HTLC: %w", err)
	}

	s.log.Info("HTLC claimed", "trade_id", p.TradeID, "chain", p.Chain, "txid", txID)

	return &SwapHTLCClaimResult{
		TradeID:   p.TradeID,
		ClaimTxID: txID,
		Chain:     p.Chain,
		State:     "claimed",
	}, nil
}

// swapHTLCRefund refunds an HTLC output after the CSV timeout.
func (s *Server) swapHTLCRefund(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapHTLCRefundParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	txID, err := s.coordinator.RefundHTLC(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to refund HTLC: %w", err)
	}

	s.log.Info("HTLC refunded", "trade_id", p.TradeID, "chain", p.Chain, "txid", txID)

	return &SwapHTLCRefundResult{
		TradeID:    p.TradeID,
		RefundTxID: txID,
		Chain:      p.Chain,
		State:      "refunded",
	}, nil
}

// swapHTLCExtractSecret extracts the secret from an HTLC claim transaction.
func (s *Server) swapHTLCExtractSecret(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapHTLCExtractSecretParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.TxID == "" {
		return nil, fmt.Errorf("txid is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	secret, err := s.coordinator.ExtractSecretFromTx(ctx, p.TradeID, p.TxID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to extract secret: %w", err)
	}

	s.log.Info("HTLC secret extracted", "trade_id", p.TradeID, "chain", p.Chain)

	return &SwapHTLCExtractSecretResult{
		TradeID: p.TradeID,
		Secret:  hex.EncodeToString(secret),
		Message: "Secret extracted successfully",
	}, nil
}
