package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/storage"
)

// ========================================
// Trade handlers
// ========================================

// TradeInfo represents trade information in RPC responses.
type TradeInfo struct {
	ID            string        `json:"id"`
	OrderID       string        `json:"order_id"`
	MakerPeerID   string        `json:"maker_peer_id"`
	TakerPeerID   string        `json:"taker_peer_id"`
	Method        string        `json:"method"`
	State         string        `json:"state"`
	OfferAmount   uint64        `json:"offer_amount"`
	RequestAmount uint64        `json:"request_amount"`
	CreatedAt     int64         `json:"created_at"`
	CompletedAt   *int64        `json:"completed_at,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
	Legs          []SwapLegInfo `json:"legs,omitempty"`
}

// SwapLegInfo represents swap leg information.
type SwapLegInfo struct {
	ID              string `json:"id"`
	LegType         string `json:"leg_type"` // "offer" or "request"
	Chain           string `json:"chain"`
	Amount          uint64 `json:"amount"`
	OurRole         string `json:"our_role"` // "sender" or "receiver"
	State           string `json:"state"`
	FundingTxID     string `json:"funding_txid,omitempty"`
	FundingVout     uint32 `json:"funding_vout,omitempty"`
	FundingConfirms uint32 `json:"funding_confirms"`
	RedeemTxID      string `json:"redeem_txid,omitempty"`
	RefundTxID      string `json:"refund_txid,omitempty"`
	TimeoutHeight   uint32 `json:"timeout_height,omitempty"`
}

func tradeToInfo(t *storage.Trade, legs []*storage.SwapLeg) TradeInfo {
	info := TradeInfo{
		ID:            t.ID,
		OrderID:       t.OrderID,
		MakerPeerID:   t.MakerPeerID,
		TakerPeerID:   t.TakerPeerID,
		Method:        t.Method,
		State:         string(t.State),
		OfferAmount:   t.OfferAmount,
		RequestAmount: t.RequestAmount,
		CreatedAt:     t.CreatedAt.Unix(),
		FailureReason: t.FailureReason,
	}
	if t.CompletedAt != nil {
		ts := t.CompletedAt.Unix()
		info.CompletedAt = &ts
	}

	if legs != nil {
		info.Legs = make([]SwapLegInfo, 0, len(legs))
		for _, leg := range legs {
			info.Legs = append(info.Legs, SwapLegInfo{
				ID:              leg.ID,
				LegType:         string(leg.LegType),
				Chain:           leg.Chain,
				Amount:          leg.Amount,
				OurRole:         string(leg.OurRole),
				State:           string(leg.State),
				FundingTxID:     leg.FundingTxID,
				FundingVout:     leg.FundingVout,
				FundingConfirms: leg.FundingConfirms,
				RedeemTxID:      leg.RedeemTxID,
				RefundTxID:      leg.RefundTxID,
				TimeoutHeight:   leg.TimeoutHeight,
			})
		}
	}

	return info
}

// TradesListParams is the parameters for trades_list.
type TradesListParams struct {
	State string `json:"state,omitempty"` // Filter by state
	Limit int    `json:"limit,omitempty"` // Max results
}

// TradesListResult is the response for trades_list.
type TradesListResult struct {
	Trades []TradeInfo `json:"trades"`
	Count  int         `json:"count"`
}

func (s *Server) tradesList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p TradesListParams
	if params != nil {
		json.Unmarshal(params, &p)
	}

	if p.Limit == 0 {
		p.Limit = 100
	}

	filter := storage.TradeFilter{
		Limit: p.Limit,
	}

	if p.State != "" {
		state := storage.TradeState(p.State)
		filter.State = &state
	}

	trades, err := s.store.ListTrades(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list trades: %w", err)
	}

	result := make([]TradeInfo, 0, len(trades))
	for _, t := range trades {
		result = append(result, tradeToInfo(t, nil))
	}

	return &TradesListResult{
		Trades: result,
		Count:  len(result),
	}, nil
}

// TradesGetParams is the parameters for trades_get.
type TradesGetParams struct {
	ID string `json:"id"`
}

func (s *Server) tradesGet(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p TradesGetParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	trade, err := s.store.GetTrade(p.ID)
	if err != nil {
		return nil, fmt.Errorf("trade not found: %w", err)
	}

	// Get swap legs
	legs, err := s.store.GetSwapLegsByTradeID(p.ID)
	if err != nil {
		s.log.Warn("Failed to get swap legs", "trade_id", p.ID, "error", err)
	}

	return tradeToInfo(trade, legs), nil
}

// TradesStatusParams is the parameters for trades_status.
type TradesStatusParams struct {
	ID string `json:"id"`
}

// TradesStatusResult is the detailed status for a trade.
type TradesStatusResult struct {
	Trade         TradeInfo              `json:"trade"`
	SwapState     string                 `json:"swap_state,omitempty"`
	LocalPubKey   string                 `json:"local_pubkey,omitempty"`
	RemotePubKey  string                 `json:"remote_pubkey,omitempty"`
	TaprootAddr   string                 `json:"taproot_address,omitempty"`
	NoncesReady   bool                   `json:"nonces_ready"`
	FundingReady  bool                   `json:"funding_ready"`
	SigningReady  bool                   `json:"signing_ready"`
	NextAction    string                 `json:"next_action"`
	MethodData    map[string]interface{} `json:"method_data,omitempty"`
}

func (s *Server) tradesStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p TradesStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	trade, err := s.store.GetTrade(p.ID)
	if err != nil {
		return nil, fmt.Errorf("trade not found: %w", err)
	}

	// Get swap legs
	legs, err := s.store.GetSwapLegsByTradeID(p.ID)
	if err != nil {
		s.log.Warn("Failed to get swap legs", "trade_id", p.ID, "error", err)
	}

	result := TradesStatusResult{
		Trade: tradeToInfo(trade, legs),
	}

	// Get active swap state from coordinator if available
	if s.coordinator != nil {
		activeSwap, err := s.coordinator.GetSwap(p.ID)
		if err == nil && activeSwap != nil {
			result.SwapState = string(activeSwap.Swap.State)

			// Get pubkeys
			if activeSwap.Swap.LocalPubKey != nil {
				result.LocalPubKey = fmt.Sprintf("%x", activeSwap.Swap.LocalPubKey)
			}
			if activeSwap.Swap.RemotePubKey != nil {
				result.RemotePubKey = fmt.Sprintf("%x", activeSwap.Swap.RemotePubKey)
			}

			// Get taproot address (use offer chain as primary for display)
			if activeSwap.MuSig2 != nil && activeSwap.MuSig2.OfferChain != nil {
				result.TaprootAddr = activeSwap.MuSig2.OfferChain.TaprootAddress
			}

			// Check nonces (both chains must have nonces)
			if activeSwap.MuSig2 != nil {
				offerNoncesReady := activeSwap.MuSig2.OfferChain != nil &&
					activeSwap.MuSig2.OfferChain.LocalNonce != nil &&
					activeSwap.MuSig2.OfferChain.RemoteNonce != nil
				requestNoncesReady := activeSwap.MuSig2.RequestChain != nil &&
					activeSwap.MuSig2.RequestChain.LocalNonce != nil &&
					activeSwap.MuSig2.RequestChain.RemoteNonce != nil
				result.NoncesReady = offerNoncesReady && requestNoncesReady
			}

			// Check funding
			result.FundingReady = activeSwap.Swap.IsFundingConfirmed()

			// Check signing (both chains must have partial sigs)
			if activeSwap.MuSig2 != nil {
				offerSigReady := activeSwap.MuSig2.OfferChain != nil &&
					activeSwap.MuSig2.OfferChain.PartialSig != nil
				requestSigReady := activeSwap.MuSig2.RequestChain != nil &&
					activeSwap.MuSig2.RequestChain.PartialSig != nil
				result.SigningReady = offerSigReady && requestSigReady
			}
		}
	}

	// Determine next action based on state
	result.NextAction = determineNextAction(trade, result)

	return result, nil
}

func determineNextAction(trade *storage.Trade, status TradesStatusResult) string {
	switch trade.State {
	case storage.TradeStateInit:
		if status.LocalPubKey == "" {
			return "generate_keys"
		}
		if status.RemotePubKey == "" {
			return "waiting_for_counterparty_pubkey"
		}
		return "exchange_nonces"

	case storage.TradeStateAccepted:
		if !status.NoncesReady {
			return "exchange_nonces"
		}
		return "create_funding_tx"

	case storage.TradeStateFunding:
		if !status.FundingReady {
			return "waiting_for_confirmations"
		}
		return "ready_to_sign"

	case storage.TradeStateFunded:
		if !status.SigningReady {
			return "create_partial_signature"
		}
		return "exchange_signatures_and_broadcast"

	case storage.TradeStateRedeemed:
		return "completed"

	case storage.TradeStateRefunded:
		return "refunded"

	case storage.TradeStateFailed:
		return "check_refund_eligibility"

	case storage.TradeStateAborted:
		return "aborted"

	default:
		return "unknown"
	}
}
