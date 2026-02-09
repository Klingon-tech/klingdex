// Package rpc - Swap timeout and recovery handlers.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// swapRecover attempts to recover a swap from the database.
func (s *Server) swapRecover(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapRecoverParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	if s.coordinator == nil {
		return nil, fmt.Errorf("coordinator not available")
	}

	if err := s.coordinator.RecoverSwap(ctx, p.TradeID); err != nil {
		return &SwapRecoverResult{
			TradeID: p.TradeID,
			Message: err.Error(),
		}, nil
	}

	// Get the recovered swap state
	activeSwap, _ := s.coordinator.GetSwap(p.TradeID)
	state := ""
	if activeSwap != nil {
		state = string(activeSwap.Swap.State)
	}

	return &SwapRecoverResult{
		TradeID: p.TradeID,
		State:   state,
		Message: "Swap recovered successfully",
	}, nil
}

// swapTimeout returns timeout information for a swap.
func (s *Server) swapTimeout(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapTimeoutParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	if s.coordinator == nil {
		return nil, fmt.Errorf("coordinator not available")
	}

	info, err := s.coordinator.GetSwapTimeoutInfo(ctx, p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeout info: %w", err)
	}

	return info, nil
}

// swapRefund attempts to refund a swap via the script path.
func (s *Server) swapRefund(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapRefundParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	if s.coordinator == nil {
		return nil, fmt.Errorf("coordinator not available")
	}

	// Get swap to determine chain if not specified
	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	chain := p.Chain
	if chain == "" {
		// Default to the chain where we locked funds
		if activeSwap.Swap.Role == swap.RoleInitiator {
			chain = activeSwap.Swap.Offer.OfferChain
		} else {
			chain = activeSwap.Swap.Offer.RequestChain
		}
	}

	txID, err := s.coordinator.ForceRefund(ctx, p.TradeID, chain)
	if err != nil {
		return &SwapRefundResult{
			TradeID: p.TradeID,
			Chain:   chain,
			State:   "failed",
		}, fmt.Errorf("refund failed: %w", err)
	}

	// Emit websocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("swap_refunded", map[string]string{
			"trade_id": p.TradeID,
			"chain":    chain,
			"txid":     txID,
		})
	}

	return &SwapRefundResult{
		TradeID:    p.TradeID,
		RefundTxID: txID,
		Chain:      chain,
		State:      "refunded",
	}, nil
}

// swapCheckTimeouts checks all pending swaps for timeout conditions.
func (s *Server) swapCheckTimeouts(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.coordinator == nil {
		return nil, fmt.Errorf("coordinator not available")
	}

	results, err := s.coordinator.CheckTimeouts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check timeouts: %w", err)
	}

	// Convert to interface{} slice
	resultSlice := make([]interface{}, len(results))
	for i, r := range results {
		resultSlice[i] = r
	}

	return &SwapCheckTimeoutsResult{
		Results: resultSlice,
		Count:   len(results),
	}, nil
}
