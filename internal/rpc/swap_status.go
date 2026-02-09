// Package rpc - Swap status and list handlers.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// swapStatus returns detailed status of a swap.
func (s *Server) swapStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapStatusParams
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

	result := &SwapStatusResult{
		TradeID:     p.TradeID,
		State:       string(activeSwap.Swap.State),
		Role:        string(activeSwap.Swap.Role),
		LocalPubKey: hex.EncodeToString(activeSwap.Swap.LocalPubKey),
	}

	// Set swap type
	swapType, _ := s.coordinator.GetSwapType(p.TradeID)
	switch swapType {
	case swap.CrossChainTypeEVMToEVM:
		result.SwapType = "evm_to_evm"
	case swap.CrossChainTypeBitcoinToEVM:
		result.SwapType = "bitcoin_to_evm"
	case swap.CrossChainTypeEVMToBitcoin:
		result.SwapType = "evm_to_bitcoin"
	case swap.CrossChainTypeBitcoinToBitcoin:
		result.SwapType = "bitcoin_to_bitcoin"
	}

	// Set method
	if activeSwap.IsMuSig2() {
		result.Method = "musig2"
	} else {
		result.Method = "htlc"
	}

	// Set addresses and method-specific status based on swap method
	if activeSwap.IsMuSig2() && activeSwap.MuSig2 != nil {
		offerChain := activeSwap.MuSig2.OfferChain
		requestChain := activeSwap.MuSig2.RequestChain
		if offerChain != nil {
			result.OfferTaprootAddress = offerChain.TaprootAddress
			result.HasOfferNonces = offerChain.LocalNonce != nil && offerChain.RemoteNonce != nil
			result.HasOfferSigs = offerChain.PartialSig != nil && offerChain.RemotePartialSig != nil
		}
		if requestChain != nil {
			result.RequestTaprootAddress = requestChain.TaprootAddress
			result.HasRequestNonces = requestChain.LocalNonce != nil && requestChain.RemoteNonce != nil
			result.HasRequestSigs = requestChain.PartialSig != nil && requestChain.RemotePartialSig != nil
		}
	} else if activeSwap.IsHTLC() {
		// Bitcoin HTLC addresses
		if activeSwap.HTLC != nil {
			if activeSwap.HTLC.OfferChain != nil && activeSwap.HTLC.OfferChain.HTLCAddress != "" {
				result.OfferHTLCAddress = activeSwap.HTLC.OfferChain.HTLCAddress
				// Also set OfferTaprootAddress for backward compatibility with BTC-BTC swaps
				result.OfferTaprootAddress = activeSwap.HTLC.OfferChain.HTLCAddress
			}
			if activeSwap.HTLC.RequestChain != nil && activeSwap.HTLC.RequestChain.HTLCAddress != "" {
				result.RequestHTLCAddress = activeSwap.HTLC.RequestChain.HTLCAddress
				// Also set RequestTaprootAddress for backward compatibility
				result.RequestTaprootAddress = activeSwap.HTLC.RequestChain.HTLCAddress
			}
		}
	}

	// EVM wallet addresses (from swap state)
	if activeSwap.Swap.LocalOfferWalletAddr != "" {
		result.OfferEVMAddress = activeSwap.Swap.LocalOfferWalletAddr
	}
	if activeSwap.Swap.LocalRequestWalletAddr != "" {
		result.RequestEVMAddress = activeSwap.Swap.LocalRequestWalletAddr
	}

	if len(activeSwap.Swap.RemotePubKey) > 0 {
		result.RemotePubKey = hex.EncodeToString(activeSwap.Swap.RemotePubKey)
	}

	// Local funding status
	if activeSwap.Swap.LocalFundingTxID != "" {
		var amount uint64
		if activeSwap.Swap.Role == swap.RoleInitiator {
			amount = activeSwap.Swap.Offer.OfferAmount
		} else {
			amount = activeSwap.Swap.Offer.RequestAmount
		}
		result.LocalFunding = &FundingStatus{
			TxID:          activeSwap.Swap.LocalFundingTxID,
			Vout:          activeSwap.Swap.LocalFundingVout,
			Amount:        amount,
			Confirmations: activeSwap.Swap.LocalFundingConfirms,
			Confirmed:     activeSwap.Swap.LocalFundingConfirms >= 1,
		}
	}

	// Remote funding status
	if activeSwap.Swap.RemoteFundingTxID != "" {
		var amount uint64
		if activeSwap.Swap.Role == swap.RoleInitiator {
			amount = activeSwap.Swap.Offer.RequestAmount
		} else {
			amount = activeSwap.Swap.Offer.OfferAmount
		}
		result.RemoteFunding = &FundingStatus{
			TxID:          activeSwap.Swap.RemoteFundingTxID,
			Vout:          activeSwap.Swap.RemoteFundingVout,
			Amount:        amount,
			Confirmations: activeSwap.Swap.RemoteFundingConfirms,
			Confirmed:     activeSwap.Swap.RemoteFundingConfirms >= 1,
		}
	}

	// Ready to redeem if we have all signatures for both chains
	result.ReadyToRedeem = result.HasOfferSigs && result.HasRequestSigs

	return result, nil
}

// swapList returns all active and historical swaps.
func (s *Server) swapList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapListParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	records, err := s.coordinator.ListSwaps(p.IncludeCompleted)
	if err != nil {
		return nil, fmt.Errorf("failed to list swaps: %w", err)
	}

	items := make([]SwapListItem, 0, len(records))
	for _, rec := range records {
		item := SwapListItem{
			TradeID:       rec.TradeID,
			State:         string(rec.State),
			Role:          rec.OurRole,
			OfferChain:    rec.OfferChain,
			OfferAmount:   rec.OfferAmount,
			RequestChain:  rec.RequestChain,
			RequestAmount: rec.RequestAmount,
			CreatedAt:     rec.CreatedAt.Unix(),
		}
		if !rec.UpdatedAt.IsZero() {
			item.UpdatedAt = rec.UpdatedAt.Unix()
		}
		items = append(items, item)
	}

	return &SwapListResult{
		Swaps: items,
		Count: len(items),
	}, nil
}
