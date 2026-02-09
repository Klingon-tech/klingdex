// Package rpc - Swap funding handlers.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// swapGetAddress returns the funding address for the swap.
// Each party funds their own chain:
// - Initiator funds offer chain
// - Responder funds request chain
// Returns Taproot (P2TR) address for MuSig2, P2WSH address for HTLC.
func (s *Server) swapGetAddress(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapGetAddressParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	// Get swap details for chain/amount info
	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	// Determine which chain and amount we're funding based on our role
	var chainSymbol string
	var amount uint64
	var address string
	var method string

	if activeSwap.Swap.Role == swap.RoleInitiator {
		// Initiator funds the offer chain
		chainSymbol = activeSwap.Swap.Offer.OfferChain
		amount = activeSwap.Swap.Offer.OfferAmount
	} else {
		// Responder funds the request chain
		chainSymbol = activeSwap.Swap.Offer.RequestChain
		amount = activeSwap.Swap.Offer.RequestAmount
	}

	result := &SwapGetAddressResult{
		TradeID: p.TradeID,
		Chain:   chainSymbol,
		Amount:  amount,
	}

	// Get the address based on swap method
	if activeSwap.IsMuSig2() {
		if activeSwap.MuSig2 == nil {
			return nil, fmt.Errorf("MuSig2 not initialized - call swap_init first")
		}

		var chainData *swap.ChainMuSig2Data
		if activeSwap.Swap.Role == swap.RoleInitiator {
			chainData = activeSwap.MuSig2.OfferChain
		} else {
			chainData = activeSwap.MuSig2.RequestChain
		}

		if chainData == nil || chainData.TaprootAddress == "" {
			return nil, fmt.Errorf("taproot address not ready - exchange pubkeys first")
		}

		address = chainData.TaprootAddress
		method = "musig2"
		result.TaprootAddress = address
	} else if activeSwap.IsHTLC() {
		if activeSwap.HTLC == nil {
			return nil, fmt.Errorf("HTLC not initialized - call swap_init first")
		}

		var chainData *swap.ChainHTLCData
		if activeSwap.Swap.Role == swap.RoleInitiator {
			chainData = activeSwap.HTLC.OfferChain
		} else {
			chainData = activeSwap.HTLC.RequestChain
		}

		if chainData == nil || chainData.HTLCAddress == "" {
			return nil, fmt.Errorf("HTLC address not ready - exchange secret hash first")
		}

		address = chainData.HTLCAddress
		method = "htlc"
		result.HTLCAddress = address
	} else {
		return nil, fmt.Errorf("unknown swap method")
	}

	result.Address = address
	result.Method = method

	return result, nil
}

// swapSetFunding sets the funding transaction info for a swap.
func (s *Server) swapSetFunding(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapSetFundingParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.TxID == "" {
		return nil, fmt.Errorf("txid is required")
	}

	// Set local funding info in coordinator
	if err := s.coordinator.SetFundingTx(p.TradeID, p.TxID, p.Vout, true); err != nil {
		return nil, fmt.Errorf("failed to set funding tx: %w", err)
	}

	// Send funding info to counterparty (direct P2P)
	payload := &node.FundingInfoPayload{
		TxID: p.TxID,
		Vout: p.Vout,
	}
	msg, err := node.NewSwapMessage(node.SwapMsgFundingInfo, p.TradeID, payload)
	if err == nil {
		if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
			s.log.Warn("Failed to send funding info", "trade_id", p.TradeID, "error", err)
		} else {
			s.log.Info("Sent funding info to counterparty", "trade_id", p.TradeID[:8], "txid", p.TxID[:16])
		}
	}

	// Update trade state
	if err := s.store.UpdateTradeState(p.TradeID, storage.TradeStateFunding); err != nil {
		s.log.Warn("Failed to update trade state", "error", err)
	}

	// Get current swap state
	activeSwap, _ := s.coordinator.GetSwap(p.TradeID)
	state := "funding"
	if activeSwap != nil {
		state = string(activeSwap.Swap.State)
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("funding_set", map[string]interface{}{
			"trade_id": p.TradeID,
			"txid":     p.TxID,
			"vout":     p.Vout,
		})
	}

	return &SwapSetFundingResult{
		TradeID: p.TradeID,
		State:   state,
		Message: "Funding set successfully",
	}, nil
}

// swapCheckFunding checks the funding status for a swap.
func (s *Server) swapCheckFunding(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapCheckFundingParams
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

	// Update confirmations
	_ = s.coordinator.UpdateConfirmations(ctx, p.TradeID)

	result := &SwapCheckFundingResult{
		TradeID:             p.TradeID,
		LocalFunded:         activeSwap.Swap.LocalFundingTxID != "",
		LocalConfirmations:  activeSwap.Swap.LocalFundingConfirms,
		RemoteFunded:        activeSwap.Swap.RemoteFundingTxID != "",
		RemoteConfirmations: activeSwap.Swap.RemoteFundingConfirms,
		BothFunded:          activeSwap.Swap.LocalFundingTxID != "" && activeSwap.Swap.RemoteFundingTxID != "",
		State:               string(activeSwap.Swap.State),
	}

	// Ready for nonce exchange if both sides are funded
	result.ReadyForNonceExchange = result.BothFunded

	return result, nil
}

// swapFund automatically funds the swap escrow address.
// This builds, signs, and broadcasts the funding transaction, then sets the funding info.
func (s *Server) swapFund(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapFundParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	// Call the coordinator's FundSwap method
	fundResult, err := s.coordinator.FundSwap(ctx, p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fund swap: %w", err)
	}

	// Send funding info to counterparty (direct P2P)
	fundPayload := &node.FundingInfoPayload{
		TxID: fundResult.TxID,
		Vout: fundResult.EscrowVout,
	}
	fundMsg, err := node.NewSwapMessage(node.SwapMsgFundingInfo, p.TradeID, fundPayload)
	if err == nil {
		if err := s.sendDirectToCounterparty(ctx, p.TradeID, fundMsg); err != nil {
			s.log.Warn("Failed to send funding info", "trade_id", p.TradeID, "error", err)
		} else {
			s.log.Info("Sent funding info to counterparty", "trade_id", p.TradeID[:8], "txid", fundResult.TxID[:16])
		}
	}

	// Update trade state
	if err := s.store.UpdateTradeState(p.TradeID, storage.TradeStateFunding); err != nil {
		s.log.Warn("Failed to update trade state", "error", err)
	}

	// Get current swap state
	activeSwap, _ := s.coordinator.GetSwap(p.TradeID)
	state := "funding"
	if activeSwap != nil {
		state = string(activeSwap.Swap.State)
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("funding_broadcast", map[string]interface{}{
			"trade_id":    p.TradeID,
			"txid":        fundResult.TxID,
			"chain":       fundResult.Chain,
			"amount":      fundResult.Amount,
			"escrow_vout": fundResult.EscrowVout,
		})
	}

	return &SwapFundResult{
		TradeID:    p.TradeID,
		TxID:       fundResult.TxID,
		Chain:      fundResult.Chain,
		Amount:     fundResult.Amount,
		Fee:        fundResult.Fee,
		EscrowVout: fundResult.EscrowVout,
		EscrowAddr: fundResult.EscrowAddr,
		InputCount: fundResult.InputCount,
		TotalInput: fundResult.TotalInput,
		Change:     fundResult.Change,
		State:      state,
	}, nil
}
