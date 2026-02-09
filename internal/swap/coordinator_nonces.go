// Package swap - Nonce operations for the Coordinator.
package swap

import (
	"fmt"
)

// =============================================================================
// Nonce Exchange
// =============================================================================

// GenerateNonces generates nonces for MuSig2 signing on both chains.
// Returns offer chain nonce and request chain nonce (each 66 bytes).
func (c *Coordinator) GenerateNonces(tradeID string) (offerNonce, requestNonce []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, nil, ErrSwapNotFound
	}

	// Generate nonces for offer chain
	_, err = active.MuSig2.OfferChain.Session.GenerateNonces()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate offer chain nonces: %w", err)
	}
	offerPubNonce, err := active.MuSig2.OfferChain.Session.LocalPubNonce()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get offer chain public nonce: %w", err)
	}
	active.MuSig2.OfferChain.LocalNonce = offerPubNonce[:]

	// Generate nonces for request chain
	_, err = active.MuSig2.RequestChain.Session.GenerateNonces()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate request chain nonces: %w", err)
	}
	requestPubNonce, err := active.MuSig2.RequestChain.Session.LocalPubNonce()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get request chain public nonce: %w", err)
	}
	active.MuSig2.RequestChain.LocalNonce = requestPubNonce[:]

	return offerPubNonce[:], requestPubNonce[:], nil
}

// SetRemoteNonces sets the counterparty's nonces for both chains.
func (c *Coordinator) SetRemoteNonces(tradeID string, offerNonce, requestNonce []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if len(offerNonce) != 66 {
		return fmt.Errorf("invalid offer nonce size: expected 66, got %d", len(offerNonce))
	}
	if len(requestNonce) != 66 {
		return fmt.Errorf("invalid request nonce size: expected 66, got %d", len(requestNonce))
	}

	var offerNonceArr, requestNonceArr [66]byte
	copy(offerNonceArr[:], offerNonce)
	copy(requestNonceArr[:], requestNonce)

	active.MuSig2.OfferChain.Session.SetRemoteNonce(offerNonceArr)
	active.MuSig2.OfferChain.RemoteNonce = offerNonce

	active.MuSig2.RequestChain.Session.SetRemoteNonce(requestNonceArr)
	active.MuSig2.RequestChain.RemoteNonce = requestNonce

	c.emitEvent(tradeID, "nonces_exchanged", nil)
	return nil
}
