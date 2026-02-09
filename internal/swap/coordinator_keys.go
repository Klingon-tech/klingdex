// Package swap - Key and address management functions for the Coordinator.
package swap

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// =============================================================================
// Key Exchange
// =============================================================================

// SetRemotePubKey sets the counterparty's public key after receiving it.
func (c *Coordinator) SetRemotePubKey(tradeID string, remotePubKey []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	remotePub, err := btcec.ParsePubKey(remotePubKey)
	if err != nil {
		return fmt.Errorf("invalid remote public key: %w", err)
	}

	if err := active.Swap.SetRemotePubKey(remotePub); err != nil {
		return err
	}

	// For HTLC, set the remote pubkey and generate addresses if not already done
	if active.IsHTLC() {
		// Set remote pubkey in HTLC sessions and generate addresses
		if active.HTLC != nil {
			// LocalPrivKey may be nil for swaps recovered from the database
			// (private keys aren't persisted for security reasons)
			if active.HTLC.LocalPrivKey == nil {
				c.log.Warn("SetRemotePubKey (HTLC): LocalPrivKey is nil, cannot generate HTLC addresses", "trade_id", tradeID)
				return errors.New("swap recovered from database without ephemeral key - cannot continue")
			}
			localPubKey := active.HTLC.LocalPrivKey.PubKey()
			isMaker := active.Swap.Role == RoleInitiator

			// Determine sender/receiver for each chain based on role
			// Offer chain: Maker=sender, Taker=receiver
			// Request chain: Taker=sender, Maker=receiver
			var offerSender, offerReceiver, requestSender, requestReceiver *btcec.PublicKey
			if isMaker {
				offerSender, offerReceiver = localPubKey, remotePub     // Maker sends, Taker receives
				requestSender, requestReceiver = remotePub, localPubKey // Taker sends, Maker receives
			} else {
				offerSender, offerReceiver = remotePub, localPubKey     // Maker sends, Taker receives
				requestSender, requestReceiver = localPubKey, remotePub // Taker sends, Maker receives
			}

			if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
				if err := active.HTLC.OfferChain.Session.SetRemotePubKey(remotePub); err != nil {
					c.log.Warn("Failed to set remote pubkey in HTLC offer session", "error", err)
				}
				// Generate HTLC address now that we have the remote pubkey
				if active.HTLC.OfferChain.HTLCAddress == "" {
					offerTimeoutBlocks := GetTimeoutBlocks(active.Swap.Offer.OfferChain, true) // maker timeout
					addr, err := active.HTLC.OfferChain.Session.GenerateSwapAddressWithRoles(offerSender, offerReceiver, offerTimeoutBlocks)
					if err != nil {
						c.log.Warn("Failed to generate offer chain HTLC address", "error", err)
					} else {
						active.HTLC.OfferChain.HTLCAddress = addr
						c.log.Debug("Generated HTLC offer chain address", "trade_id", tradeID, "address", addr)
					}
				}
			}
			if active.HTLC.RequestChain != nil && active.HTLC.RequestChain.Session != nil {
				if err := active.HTLC.RequestChain.Session.SetRemotePubKey(remotePub); err != nil {
					c.log.Warn("Failed to set remote pubkey in HTLC request session", "error", err)
				}
				// Generate HTLC address now that we have the remote pubkey
				if active.HTLC.RequestChain.HTLCAddress == "" {
					requestTimeoutBlocks := GetTimeoutBlocks(active.Swap.Offer.RequestChain, false) // taker timeout
					addr, err := active.HTLC.RequestChain.Session.GenerateSwapAddressWithRoles(requestSender, requestReceiver, requestTimeoutBlocks)
					if err != nil {
						c.log.Warn("Failed to generate request chain HTLC address", "error", err)
					} else {
						active.HTLC.RequestChain.HTLCAddress = addr
						c.log.Debug("Generated HTLC request chain address", "trade_id", tradeID, "address", addr)
					}
				}
			}
		}
		// Save state
		if err := c.saveSwapState(tradeID); err != nil {
			c.log.Warn("SetRemotePubKey (HTLC): failed to save swap state", "trade_id", tradeID, "error", err)
		}
		return nil
	}

	// MuSig2: Set remote pubkey in both chain sessions
	if err := active.MuSig2.OfferChain.Session.SetRemotePubKey(remotePub); err != nil {
		return fmt.Errorf("failed to set remote pubkey in offer session: %w", err)
	}
	if err := active.MuSig2.RequestChain.Session.SetRemotePubKey(remotePub); err != nil {
		return fmt.Errorf("failed to set remote pubkey in request session: %w", err)
	}

	// Generate Taproot addresses for both chains
	// CRITICAL: Both parties must use the SAME refund pubkey for each chain:
	// - Offer chain escrow (maker funds) → maker's pubkey for refund
	// - Request chain escrow (taker funds) → taker's pubkey for refund
	isMaker := active.Swap.Role == RoleInitiator
	localPubKey := active.MuSig2.LocalPrivKey.PubKey()

	var makerPubKey, takerPubKey *btcec.PublicKey
	if isMaker {
		makerPubKey = localPubKey
		takerPubKey = remotePub
	} else {
		makerPubKey = remotePub
		takerPubKey = localPubKey
	}

	// Offer chain address - maker funds, so maker's pubkey for refund
	offerTimeoutBlocks := GetTimeoutBlocks(active.Swap.Offer.OfferChain, true) // maker timeout
	offerAddr, err := active.MuSig2.OfferChain.Session.TaprootAddressWithRefund(makerPubKey, offerTimeoutBlocks)
	if err != nil {
		return fmt.Errorf("failed to generate offer chain taproot address: %w", err)
	}
	active.MuSig2.OfferChain.TaprootAddress = offerAddr

	// Request chain address - taker funds, so taker's pubkey for refund
	requestTimeoutBlocks := GetTimeoutBlocks(active.Swap.Offer.RequestChain, false) // taker timeout
	requestAddr, err := active.MuSig2.RequestChain.Session.TaprootAddressWithRefund(takerPubKey, requestTimeoutBlocks)
	if err != nil {
		return fmt.Errorf("failed to generate request chain taproot address: %w", err)
	}
	active.MuSig2.RequestChain.TaprootAddress = requestAddr

	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("SetRemotePubKey: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "pubkey_exchanged", map[string]interface{}{
		"offer_taproot_address":   offerAddr,
		"request_taproot_address": requestAddr,
	})

	return nil
}

// GetLocalPubKey returns our public key for the swap.
func (c *Coordinator) GetLocalPubKey(tradeID string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	return active.Swap.LocalPubKey, nil
}

// =============================================================================
// Address Getters
// =============================================================================

// GetTaprootAddress returns the Taproot address for the specified chain.
func (c *Coordinator) GetTaprootAddress(tradeID string, chainSymbol string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", ErrSwapNotFound
	}

	chainData := c.getChainData(active, chainSymbol)
	if chainData == nil {
		return "", fmt.Errorf("unknown chain: %s", chainSymbol)
	}

	if chainData.TaprootAddress == "" {
		return "", errors.New("taproot address not yet generated - exchange pubkeys first")
	}

	return chainData.TaprootAddress, nil
}

// GetTaprootAddresses returns both Taproot addresses for the swap.
func (c *Coordinator) GetTaprootAddresses(tradeID string) (offerAddr, requestAddr string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", "", ErrSwapNotFound
	}

	return active.MuSig2.OfferChain.TaprootAddress, active.MuSig2.RequestChain.TaprootAddress, nil
}

// GetHTLCAddresses returns both HTLC addresses for the swap.
func (c *Coordinator) GetHTLCAddresses(tradeID string) (offerAddr, requestAddr string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", "", ErrSwapNotFound
	}

	if !active.IsHTLC() || active.HTLC == nil {
		return "", "", errors.New("not an HTLC swap")
	}

	if active.HTLC.OfferChain != nil {
		offerAddr = active.HTLC.OfferChain.HTLCAddress
	}
	if active.HTLC.RequestChain != nil {
		requestAddr = active.HTLC.RequestChain.HTLCAddress
	}

	return offerAddr, requestAddr, nil
}

// GetSwapAddresses returns the escrow addresses for the swap (works for both MuSig2 and HTLC).
func (c *Coordinator) GetSwapAddresses(tradeID string) (offerAddr, requestAddr string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", "", ErrSwapNotFound
	}

	if active.IsMuSig2() && active.MuSig2 != nil {
		if active.MuSig2.OfferChain != nil {
			offerAddr = active.MuSig2.OfferChain.TaprootAddress
		}
		if active.MuSig2.RequestChain != nil {
			requestAddr = active.MuSig2.RequestChain.TaprootAddress
		}
	} else if active.IsHTLC() && active.HTLC != nil {
		if active.HTLC.OfferChain != nil {
			offerAddr = active.HTLC.OfferChain.HTLCAddress
		}
		if active.HTLC.RequestChain != nil {
			requestAddr = active.HTLC.RequestChain.HTLCAddress
		}
	}

	return offerAddr, requestAddr, nil
}
