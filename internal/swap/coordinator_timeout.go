// Package swap - Timeout monitoring for the Coordinator.
package swap

import (
	"context"
	"fmt"
	"time"
)

// =========================================================================
// Timeout Monitoring
// =========================================================================

// CheckTimeouts checks all pending swaps for timeout conditions.
// If a swap has timed out and we have funds locked, it attempts to broadcast a refund transaction.
func (c *Coordinator) CheckTimeouts(ctx context.Context) ([]TimeoutCheckResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var results []TimeoutCheckResult

	for tradeID, active := range c.swaps {
		// Only check funded swaps that haven't been completed
		if active.Swap.State != StateFunded && active.Swap.State != StateFunding {
			continue
		}

		// Check the offer chain (where we locked funds if we're the initiator)
		if active.Swap.Role == RoleInitiator && active.Swap.OfferChainTimeoutHeight > 0 {
			result := c.checkTimeoutForChainUnlocked(ctx, tradeID, active, active.Swap.Offer.OfferChain, active.Swap.OfferChainTimeoutHeight)
			results = append(results, result)
		}

		// Check the request chain (where we locked funds if we're the responder)
		if active.Swap.Role == RoleResponder && active.Swap.RequestChainTimeoutHeight > 0 {
			result := c.checkTimeoutForChainUnlocked(ctx, tradeID, active, active.Swap.Offer.RequestChain, active.Swap.RequestChainTimeoutHeight)
			results = append(results, result)
		}
	}

	return results, nil
}

// checkTimeoutForChainUnlocked checks timeout for a specific chain (caller must hold lock).
func (c *Coordinator) checkTimeoutForChainUnlocked(ctx context.Context, tradeID string, active *ActiveSwap, chainSymbol string, timeoutHeight uint32) TimeoutCheckResult {
	result := TimeoutCheckResult{
		TradeID:       tradeID,
		Chain:         chainSymbol,
		TimeoutHeight: timeoutHeight,
	}

	// Get backend for chain
	b, ok := c.backends[chainSymbol]
	if !ok {
		result.Error = fmt.Errorf("no backend for chain %s", chainSymbol)
		return result
	}

	// Get current block height
	heightInt64, err := b.GetBlockHeight(ctx)
	if err != nil {
		result.Error = fmt.Errorf("failed to get block height: %w", err)
		return result
	}
	height := uint32(heightInt64)
	result.CurrentHeight = height

	// Calculate blocks remaining
	result.BlocksRemaining = int32(timeoutHeight) - int32(height)

	// Check if timeout has passed
	if height >= timeoutHeight {
		result.CanRefund = true

		// Attempt to build and broadcast refund transaction
		refundTxID, err := c.buildAndBroadcastRefundUnlocked(ctx, tradeID, active, chainSymbol)
		if err != nil {
			result.Error = fmt.Errorf("failed to refund: %w", err)
		} else {
			result.RefundBroadcast = true
			result.RefundTxID = refundTxID
		}
	}

	return result
}

// buildAndBroadcastRefundUnlocked builds and broadcasts a refund transaction (caller must hold lock).
func (c *Coordinator) buildAndBroadcastRefundUnlocked(ctx context.Context, tradeID string, active *ActiveSwap, chainSymbol string) (string, error) {
	// Check if we have MuSig2 data with script tree
	if active.MuSig2 == nil {
		return "", fmt.Errorf("no MuSig2 data for swap")
	}

	// Get the session for the appropriate chain
	var chainData *ChainMuSig2Data
	if chainSymbol == active.Swap.Offer.OfferChain {
		chainData = active.MuSig2.OfferChain
	} else if chainSymbol == active.Swap.Offer.RequestChain {
		chainData = active.MuSig2.RequestChain
	}

	if chainData == nil || chainData.Session == nil {
		return "", fmt.Errorf("no MuSig2 session for chain %s", chainSymbol)
	}

	scriptTree := chainData.Session.GetScriptTree()
	if scriptTree == nil {
		// Script tree not cached - rebuild it for refund
		// We need: aggregated pubkey, refund pubkey (our local pubkey), timeout blocks
		aggPubKey, err := chainData.Session.AggregatedPubKey()
		if err != nil {
			return "", fmt.Errorf("cannot compute aggregated pubkey for refund: %w", err)
		}

		// Refund pubkey is our local pubkey (the one who funded this chain)
		refundPubKey := active.MuSig2.LocalPrivKey.PubKey()

		// Get timeout blocks for this chain
		isMaker := active.Swap.Role == RoleInitiator
		timeoutBlocks := GetTimeoutBlocks(chainSymbol, isMaker)

		// Rebuild script tree
		scriptTree, err = BuildTaprootScriptTree(aggPubKey, refundPubKey, timeoutBlocks)
		if err != nil {
			return "", fmt.Errorf("failed to rebuild script tree for refund: %w", err)
		}

		// Cache it for future use
		chainData.Session.SetScriptTree(scriptTree)
	}

	// Get funding transaction info
	var fundingTxID string
	var fundingVout uint32
	var fundingAmount uint64

	// Determine which leg we're refunding
	// Initiator funds offer chain (their LocalFunding), Responder funds request chain (their LocalFunding)
	if active.Swap.Role == RoleInitiator && chainSymbol == active.Swap.Offer.OfferChain {
		// Initiator refunding their offer chain (BTC in BTC->LTC swap)
		fundingTxID = active.Swap.LocalFundingTxID
		fundingVout = active.Swap.LocalFundingVout
		fundingAmount = active.Swap.Offer.OfferAmount
	} else if active.Swap.Role == RoleResponder && chainSymbol == active.Swap.Offer.RequestChain {
		// Responder refunding their request chain (LTC in BTC->LTC swap)
		fundingTxID = active.Swap.LocalFundingTxID
		fundingVout = active.Swap.LocalFundingVout
		fundingAmount = active.Swap.Offer.RequestAmount
	} else {
		return "", fmt.Errorf("cannot refund: you are %s but trying to refund %s (offer=%s, request=%s)",
			active.Swap.Role, chainSymbol, active.Swap.Offer.OfferChain, active.Swap.Offer.RequestChain)
	}

	if fundingTxID == "" {
		return "", fmt.Errorf("no funding transaction recorded")
	}

	// Get local private key
	localPrivKey := active.MuSig2.LocalPrivKey
	if localPrivKey == nil {
		return "", fmt.Errorf("no local private key for refund signing")
	}

	// Get destination address for refund
	// We use the swap's original wallet addresses if available, otherwise derive a fresh one
	if c.wallet == nil {
		return "", fmt.Errorf("wallet not available for deriving refund address")
	}

	var destAddress string

	// First, try to use the stored local wallet address for this chain
	if chainSymbol == active.Swap.Offer.OfferChain && active.Swap.LocalOfferWalletAddr != "" {
		destAddress = active.Swap.LocalOfferWalletAddr
	} else if chainSymbol == active.Swap.Offer.RequestChain && active.Swap.LocalRequestWalletAddr != "" {
		destAddress = active.Swap.LocalRequestWalletAddr
	}

	// If no stored address, derive one using a deterministic index based on the trade ID
	// This ensures we get a consistent address even if we restart the node
	if destAddress == "" {
		// Use the first 4 bytes of the trade ID as a deterministic index
		// This provides some address variety while remaining deterministic
		tradeIDBytes := []byte(tradeID)
		index := uint32(0)
		if len(tradeIDBytes) >= 4 {
			index = uint32(tradeIDBytes[0])<<24 | uint32(tradeIDBytes[1])<<16 | uint32(tradeIDBytes[2])<<8 | uint32(tradeIDBytes[3])
			index = index % 1000 // Keep index reasonable (0-999)
		}

		var err error
		destAddress, err = c.wallet.DeriveAddress(chainSymbol, 0, index)
		if err != nil {
			return "", fmt.Errorf("failed to derive refund address: %w", err)
		}
	}

	// Get fee rate from backend
	b := c.backends[chainSymbol]
	feeEstimate, err := b.GetFeeEstimates(ctx)
	var feeRate uint64 = 20 // Default fee rate
	if err == nil && feeEstimate != nil {
		feeRate = feeEstimate.HourFee // Use 1-hour fee for refunds
		if feeRate == 0 {
			feeRate = 20
		}
	}

	// Build refund transaction
	refundTx, err := BuildRefundTxFromTree(
		scriptTree,
		chainSymbol,
		c.network,
		fundingTxID,
		fundingVout,
		fundingAmount,
		destAddress,
		feeRate,
		localPrivKey,
	)
	if err != nil {
		return "", fmt.Errorf("failed to build refund transaction: %w", err)
	}

	// Serialize and broadcast
	txHex, err := SerializeTx(refundTx)
	if err != nil {
		return "", fmt.Errorf("failed to serialize refund transaction: %w", err)
	}

	txID, err := b.BroadcastTransaction(ctx, txHex)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast refund transaction: %w", err)
	}

	// Update swap state
	active.Swap.State = StateRefunded
	c.emitEvent(tradeID, "refunded", map[string]string{
		"chain":     chainSymbol,
		"refund_tx": txID,
	})

	// Save state
	if c.store != nil {
		_ = c.saveSwapState(tradeID) // Best effort
	}

	return txID, nil
}

// StartTimeoutMonitor starts a background goroutine that periodically checks for timed-out swaps.
// The check interval should be appropriate for the blockchain block time (e.g., 5-10 minutes for BTC).
func (c *Coordinator) StartTimeoutMonitor(checkInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				results, err := c.CheckTimeouts(c.ctx)
				if err != nil {
					// Log error (in production, use proper logging)
					continue
				}

				// Emit events for any refunds that were broadcast
				for _, result := range results {
					if result.RefundBroadcast {
						c.mu.Lock()
						c.emitEvent(result.TradeID, "timeout_refund", result)
						c.mu.Unlock()
					}
				}
			}
		}
	}()
}

// Stop stops the coordinator and any background processes.
func (c *Coordinator) Stop() {
	c.cancel()
}

// GetSwapTimeoutInfo returns timeout information for a swap.
func (c *Coordinator) GetSwapTimeoutInfo(ctx context.Context, tradeID string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	info := map[string]interface{}{
		"trade_id": tradeID,
		"state":    string(active.Swap.State),
		"role":     string(active.Swap.Role),
	}

	// Add offer chain timeout info
	if active.Swap.OfferChainTimeoutHeight > 0 {
		offerInfo := map[string]interface{}{
			"chain":          active.Swap.Offer.OfferChain,
			"timeout_height": active.Swap.OfferChainTimeoutHeight,
		}

		// Get current height if backend available
		if b, ok := c.backends[active.Swap.Offer.OfferChain]; ok {
			if heightInt64, err := b.GetBlockHeight(ctx); err == nil {
				height := uint32(heightInt64)
				offerInfo["current_height"] = height
				offerInfo["blocks_remaining"] = int32(active.Swap.OfferChainTimeoutHeight) - int32(height)
				offerInfo["can_refund"] = height >= active.Swap.OfferChainTimeoutHeight
			}
		}

		info["offer_chain_timeout"] = offerInfo
	}

	// Add request chain timeout info
	if active.Swap.RequestChainTimeoutHeight > 0 {
		requestInfo := map[string]interface{}{
			"chain":          active.Swap.Offer.RequestChain,
			"timeout_height": active.Swap.RequestChainTimeoutHeight,
		}

		// Get current height if backend available
		if b, ok := c.backends[active.Swap.Offer.RequestChain]; ok {
			if heightInt64, err := b.GetBlockHeight(ctx); err == nil {
				height := uint32(heightInt64)
				requestInfo["current_height"] = height
				requestInfo["blocks_remaining"] = int32(active.Swap.RequestChainTimeoutHeight) - int32(height)
				requestInfo["can_refund"] = height >= active.Swap.RequestChainTimeoutHeight
			}
		}

		info["request_chain_timeout"] = requestInfo
	}

	return info, nil
}

// ForceRefund attempts to refund a swap even if timeout hasn't been reached.
// This will fail on-chain if the CSV timelock hasn't passed.
// Useful for testing or when user wants to try refunding manually.
func (c *Coordinator) ForceRefund(ctx context.Context, tradeID string, chainSymbol string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", ErrSwapNotFound
	}

	return c.buildAndBroadcastRefundUnlocked(ctx, tradeID, active, chainSymbol)
}
