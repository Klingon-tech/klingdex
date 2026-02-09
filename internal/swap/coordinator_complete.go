// Package swap - Completion and refund operations for the Coordinator.
package swap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
)

// =============================================================================
// Completion
// =============================================================================

// CompleteSwap marks the swap as completed.
func (c *Coordinator) CompleteSwap(tradeID string, redeemTxID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if err := active.Swap.TransitionTo(StateRedeemed); err != nil {
		return err
	}

	// Save swap state to database
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("CompleteSwap: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "swap_completed", map[string]interface{}{
		"redeem_txid": redeemTxID,
	})

	return nil
}

// RefundSwap initiates a refund after timeout.
func (c *Coordinator) RefundSwap(ctx context.Context, tradeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	// Check if we can refund
	offerHeight, _ := c.getBlockHeight(ctx, active.Swap.Offer.OfferChain)
	requestHeight, _ := c.getBlockHeight(ctx, active.Swap.Offer.RequestChain)

	if !active.Swap.CanRefundByBlock(offerHeight, requestHeight) {
		blocksLeft := active.Swap.BlocksUntilRefund(offerHeight)
		return fmt.Errorf("cannot refund yet - %d blocks remaining", blocksLeft)
	}

	if err := active.Swap.TransitionTo(StateRefunded); err != nil {
		return err
	}

	// Save swap state to database
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("RefundSwap: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "swap_refunded", nil)
	return nil
}

// =============================================================================
// Secret/Hash Getters (for HTLC swaps)
// =============================================================================

// GetSecretHash returns the secret hash for a swap (for HTLC or verification).
func (c *Coordinator) GetSecretHash(tradeID string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	return active.Swap.SecretHash, nil
}

// RevealSecret reveals the secret after redemption (initiator only).
func (c *Coordinator) RevealSecret(tradeID string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	if active.Swap.Role != RoleInitiator {
		return nil, errors.New("only initiator has the secret")
	}

	return active.Swap.Secret, nil
}

// SetRemoteSecretHash sets the secret hash received from the initiator (for HTLC).
// Called by responder when they receive the secret hash from initiator.
func (c *Coordinator) SetRemoteSecretHash(tradeID string, secretHash []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if len(secretHash) != 32 {
		return fmt.Errorf("invalid secret hash length: expected 32, got %d", len(secretHash))
	}

	active.Swap.SecretHash = secretHash

	// Update HTLC sessions if present
	if active.HTLC != nil {
		if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
			if err := active.HTLC.OfferChain.Session.SetSecretHash(secretHash); err != nil {
				return fmt.Errorf("failed to set secret hash for offer chain: %w", err)
			}
		}
		if active.HTLC.RequestChain != nil && active.HTLC.RequestChain.Session != nil {
			if err := active.HTLC.RequestChain.Session.SetSecretHash(secretHash); err != nil {
				return fmt.Errorf("failed to set secret hash for request chain: %w", err)
			}
		}
	}

	c.emitEvent(tradeID, "secret_hash_received", nil)
	return nil
}

// SetRevealedSecret sets the secret when it's revealed by the initiator (for HTLC claiming).
func (c *Coordinator) SetRevealedSecret(tradeID string, secret []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if len(secret) != 32 {
		return fmt.Errorf("invalid secret length: expected 32, got %d", len(secret))
	}

	// Verify secret matches the hash
	if len(active.Swap.SecretHash) > 0 {
		hash := sha256.Sum256(secret)
		if !bytes.Equal(hash[:], active.Swap.SecretHash) {
			return errors.New("secret does not match hash")
		}
	}

	active.Swap.Secret = secret

	// Update HTLC sessions if present
	if active.HTLC != nil {
		if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
			if err := active.HTLC.OfferChain.Session.SetSecret(secret); err != nil {
				c.log.Warn("Failed to set secret for offer chain session", "error", err)
			}
		}
		if active.HTLC.RequestChain != nil && active.HTLC.RequestChain.Session != nil {
			if err := active.HTLC.RequestChain.Session.SetSecret(secret); err != nil {
				c.log.Warn("Failed to set secret for request chain session", "error", err)
			}
		}
	}

	c.emitEvent(tradeID, "secret_revealed", nil)
	return nil
}
