// Package rpc - Swap signing handlers.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/config"
	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// swapSign creates and broadcasts partial signatures for both chains.
func (s *Server) swapSign(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapSignParams
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

	// Check that nonces have been exchanged for BOTH chains
	if activeSwap.MuSig2 == nil {
		return nil, fmt.Errorf("MuSig2 not initialized")
	}
	if activeSwap.MuSig2.OfferChain == nil || activeSwap.MuSig2.OfferChain.LocalNonce == nil {
		return nil, fmt.Errorf("offer chain local nonce not generated - call swap_exchangeNonce first")
	}
	if activeSwap.MuSig2.OfferChain.RemoteNonce == nil {
		return nil, fmt.Errorf("offer chain remote nonce not received - wait for counterparty")
	}
	if activeSwap.MuSig2.RequestChain == nil || activeSwap.MuSig2.RequestChain.LocalNonce == nil {
		return nil, fmt.Errorf("request chain local nonce not generated - call swap_exchangeNonce first")
	}
	if activeSwap.MuSig2.RequestChain.RemoteNonce == nil {
		return nil, fmt.Errorf("request chain remote nonce not received - wait for counterparty")
	}

	// Check funding is complete
	if activeSwap.Swap.LocalFundingTxID == "" {
		return nil, fmt.Errorf("local funding not set")
	}
	if activeSwap.Swap.RemoteFundingTxID == "" {
		return nil, fmt.Errorf("remote funding not set - wait for counterparty to fund")
	}

	// Get dynamic fee rates for both chains
	offerFeeRate := s.getFeeRateForChain(ctx, activeSwap.Swap.Offer.OfferChain)
	requestFeeRate := s.getFeeRateForChain(ctx, activeSwap.Swap.Offer.RequestChain)

	// Build sighash for OFFER CHAIN spending transaction
	offerDestAddr, err := s.getDestinationAddressForChain(activeSwap, activeSwap.Swap.Offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer chain dest address: %w", err)
	}

	// Get DAO addresses from config
	exchangeCfg := config.NewExchangeConfig(config.NetworkType(s.coordinator.Network()))
	offerDAOAddr := exchangeCfg.GetDAOAddress(activeSwap.Swap.Offer.OfferChain)
	requestDAOAddr := exchangeCfg.GetDAOAddress(activeSwap.Swap.Offer.RequestChain)

	// Calculate DAO fees
	offerDAOFee := swap.CalculateDAOFee(activeSwap.Swap.Offer.OfferAmount, false)
	requestDAOFee := swap.CalculateDAOFee(activeSwap.Swap.Offer.RequestAmount, true)

	s.log.Info("swap_sign: offer chain dest", "chain", activeSwap.Swap.Offer.OfferChain, "dest", offerDestAddr, "role", activeSwap.Swap.Role, "daoFee", offerDAOFee, "feeRate", offerFeeRate)
	offerSpendParams := &swap.SpendingTxParams{
		Symbol:         activeSwap.Swap.Offer.OfferChain,
		Network:        s.coordinator.Network(),
		FundingTxID:    activeSwap.Swap.LocalFundingTxID,
		FundingVout:    activeSwap.Swap.LocalFundingVout,
		FundingAmount:  activeSwap.Swap.Offer.OfferAmount,
		TaprootAddress: activeSwap.MuSig2.OfferChain.TaprootAddress,
		DestAddress:    offerDestAddr,
		DAOAddress:     offerDAOAddr,
		DAOFee:         offerDAOFee,
		FeeRate:        offerFeeRate,
	}
	if activeSwap.Swap.Role == swap.RoleResponder {
		offerSpendParams.FundingTxID = activeSwap.Swap.RemoteFundingTxID
		offerSpendParams.FundingVout = activeSwap.Swap.RemoteFundingVout
	}
	_, offerSighash, err := swap.BuildSpendingTx(offerSpendParams)
	if err != nil {
		return nil, fmt.Errorf("failed to build offer chain spending tx: %w", err)
	}

	// Build sighash for REQUEST CHAIN spending transaction
	requestDestAddr, err := s.getDestinationAddressForChain(activeSwap, activeSwap.Swap.Offer.RequestChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get request chain dest address: %w", err)
	}
	s.log.Info("swap_sign: request chain dest", "chain", activeSwap.Swap.Offer.RequestChain, "dest", requestDestAddr, "role", activeSwap.Swap.Role, "daoFee", requestDAOFee, "feeRate", requestFeeRate)
	requestSpendParams := &swap.SpendingTxParams{
		Symbol:         activeSwap.Swap.Offer.RequestChain,
		Network:        s.coordinator.Network(),
		FundingTxID:    activeSwap.Swap.RemoteFundingTxID,
		FundingVout:    activeSwap.Swap.RemoteFundingVout,
		FundingAmount:  activeSwap.Swap.Offer.RequestAmount,
		TaprootAddress: activeSwap.MuSig2.RequestChain.TaprootAddress,
		DestAddress:    requestDestAddr,
		DAOAddress:     requestDAOAddr,
		DAOFee:         requestDAOFee,
		FeeRate:        requestFeeRate,
	}
	if activeSwap.Swap.Role == swap.RoleInitiator {
		requestSpendParams.FundingTxID = activeSwap.Swap.RemoteFundingTxID
		requestSpendParams.FundingVout = activeSwap.Swap.RemoteFundingVout
	} else {
		requestSpendParams.FundingTxID = activeSwap.Swap.LocalFundingTxID
		requestSpendParams.FundingVout = activeSwap.Swap.LocalFundingVout
	}
	_, requestSighash, err := swap.BuildSpendingTx(requestSpendParams)
	if err != nil {
		return nil, fmt.Errorf("failed to build request chain spending tx: %w", err)
	}

	// Create partial signatures for BOTH chains
	offerSig, requestSig, err := s.coordinator.CreatePartialSignatures(ctx, p.TradeID, offerSighash[:], requestSighash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create partial signatures: %w", err)
	}

	offerSigHex := hex.EncodeToString(offerSig)
	requestSigHex := hex.EncodeToString(requestSig)

	// Send BOTH partial signatures to counterparty (direct P2P)
	payload := &node.PartialSigPayload{
		OfferPartialSig:   offerSigHex,
		RequestPartialSig: requestSigHex,
	}
	msg, err := node.NewSwapMessage(node.SwapMsgPartialSig, p.TradeID, payload)
	if err == nil {
		if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
			s.log.Warn("Failed to send partial sigs", "trade_id", p.TradeID, "error", err)
		} else {
			s.log.Info("Sent partial signatures for both chains to counterparty", "trade_id", p.TradeID[:8])
		}
	}

	// Update trade state
	if err := s.store.UpdateTradeState(p.TradeID, storage.TradeStateFunded); err != nil {
		s.log.Warn("Failed to update trade state", "error", err)
	}

	// Check if we have remote sigs for both chains
	hasOfferSig, hasRequestSig := s.coordinator.HasRemotePartialSigs(p.TradeID)
	hasRemoteSigs := hasOfferSig && hasRequestSig

	result := &SwapSignResult{
		TradeID:       p.TradeID,
		ReadyToRedeem: hasRemoteSigs,
		State:         string(activeSwap.Swap.State),
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("partial_sigs_created", map[string]interface{}{
			"trade_id":            p.TradeID,
			"offer_partial_sig":   offerSigHex,
			"request_partial_sig": requestSigHex,
		})
	}

	return result, nil
}

// getDestinationAddressForChain returns the destination address for a specific chain.
func (s *Server) getDestinationAddressForChain(activeSwap *swap.ActiveSwap, chainSymbol string) (string, error) {
	isOfferChain := chainSymbol == activeSwap.Swap.Offer.OfferChain

	if activeSwap.Swap.Role == swap.RoleInitiator {
		if isOfferChain {
			if activeSwap.Swap.RemoteOfferWalletAddr == "" {
				return "", fmt.Errorf("remote offer wallet address not set - counterparty hasn't initialized swap yet")
			}
			return activeSwap.Swap.RemoteOfferWalletAddr, nil
		} else {
			if activeSwap.Swap.LocalRequestWalletAddr == "" {
				if s.wallet != nil {
					// Fallback: derive fresh address with proper index management
					addr, _, err := s.getNextWalletAddress(chainSymbol)
					if err != nil {
						return "", fmt.Errorf("failed to derive wallet address: %w", err)
					}
					return addr, nil
				}
				return "", fmt.Errorf("local request wallet address not set")
			}
			return activeSwap.Swap.LocalRequestWalletAddr, nil
		}
	} else {
		if isOfferChain {
			if activeSwap.Swap.LocalOfferWalletAddr == "" {
				if s.wallet != nil {
					// Fallback: derive fresh address with proper index management
					addr, _, err := s.getNextWalletAddress(chainSymbol)
					if err != nil {
						return "", fmt.Errorf("failed to derive wallet address: %w", err)
					}
					return addr, nil
				}
				return "", fmt.Errorf("local offer wallet address not set")
			}
			return activeSwap.Swap.LocalOfferWalletAddr, nil
		} else {
			if activeSwap.Swap.RemoteRequestWalletAddr == "" {
				return "", fmt.Errorf("remote request wallet address not set - counterparty hasn't initialized swap yet")
			}
			return activeSwap.Swap.RemoteRequestWalletAddr, nil
		}
	}
}

// getNextWalletAddress derives a fresh wallet address for a chain using proper index management.
// It tracks used indices in storage to avoid address reuse.
func (s *Server) getNextWalletAddress(chainSymbol string) (string, uint32, error) {
	if s.wallet == nil {
		return "", 0, fmt.Errorf("wallet not available")
	}

	const account = uint32(0)
	const change = uint32(0) // External addresses

	// Get the next available address index from storage
	nextIndex := uint32(0)
	if s.store != nil {
		var err error
		nextIndex, err = s.store.GetNextAddressIndex(chainSymbol, account, change)
		if err != nil {
			s.log.Debug("Failed to get next address index, using 0", "chain", chainSymbol, "error", err)
			nextIndex = 0
		}
	}

	// Derive the address at the next index
	addr, err := s.wallet.GetAddress(chainSymbol, account, nextIndex)
	if err != nil {
		return "", 0, err
	}

	// Save the address to storage for tracking
	if s.store != nil {
		walletAddr := &storage.WalletAddress{
			Address:      addr,
			Chain:        chainSymbol,
			Account:      account,
			Change:       change,
			AddressIndex: nextIndex,
			AddressType:  "p2wpkh", // Default for Bitcoin-like chains
		}
		if err := s.store.SaveWalletAddress(walletAddr); err != nil {
			s.log.Debug("Failed to save wallet address", "address", addr, "error", err)
		} else {
			s.log.Debug("Derived new wallet address", "chain", chainSymbol, "index", nextIndex, "address", addr)
		}
	}

	return addr, nextIndex, nil
}

// getFeeRateForChain fetches dynamic fee rate for a chain, with fallback to default.
// Uses HalfHourFee (30-min confirmation target) for atomic swaps.
func (s *Server) getFeeRateForChain(ctx context.Context, chainSymbol string) uint64 {
	const defaultFeeRate = uint64(10) // Default fallback in sat/vB

	b, ok := s.coordinator.GetBackend(chainSymbol)
	if !ok {
		s.log.Debug("No backend for chain, using default fee rate", "chain", chainSymbol, "feeRate", defaultFeeRate)
		return defaultFeeRate
	}

	feeEstimate, err := b.GetFeeEstimates(ctx)
	if err != nil {
		s.log.Debug("Failed to get fee estimates, using default", "chain", chainSymbol, "error", err, "feeRate", defaultFeeRate)
		return defaultFeeRate
	}

	// Use HalfHourFee for atomic swaps (reasonable confirmation time)
	if feeEstimate.HalfHourFee > 0 {
		s.log.Debug("Using dynamic fee rate", "chain", chainSymbol, "feeRate", feeEstimate.HalfHourFee)
		return feeEstimate.HalfHourFee
	}

	// Fallback to other fee tiers if HalfHourFee not available
	if feeEstimate.HourFee > 0 {
		return feeEstimate.HourFee
	}
	if feeEstimate.FastestFee > 0 {
		return feeEstimate.FastestFee
	}
	if feeEstimate.MinimumFee > 0 {
		return feeEstimate.MinimumFee
	}

	return defaultFeeRate
}
