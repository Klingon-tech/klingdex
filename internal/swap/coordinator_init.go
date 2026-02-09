// Package swap - Swap initiation and response functions for the Coordinator.
package swap

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// =============================================================================
// Swap Initiation (Maker side)
// =============================================================================

// InitiateSwap starts a new swap as the maker (initiator).
// Called when someone takes our order.
// The method parameter specifies MuSig2 or HTLC.
func (c *Coordinator) InitiateSwap(ctx context.Context, tradeID, orderID string, offer Offer, method Method) (*ActiveSwap, error) {
	c.log.Debug("InitiateSwap: acquiring lock", "trade_id", tradeID, "method", method)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log.Debug("InitiateSwap: lock acquired", "trade_id", tradeID)

	// Check if swap already exists
	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	c.log.Debug("InitiateSwap: creating swap", "trade_id", tradeID)
	// Create swap
	swap, err := NewSwap(c.network, method, RoleInitiator, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID

	// Generate secret (initiator generates)
	if err := swap.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	c.log.Debug("InitiateSwap: generating ephemeral key", "trade_id", tradeID)
	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Get current block heights
	c.log.Debug("InitiateSwap: getting block heights", "trade_id", tradeID, "offer_chain", offer.OfferChain, "backends", len(c.backends))
	offerHeight, err := c.getBlockHeight(ctx, offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer chain height: %w", err)
	}
	c.log.Debug("InitiateSwap: got offer height", "trade_id", tradeID, "offer_height", offerHeight, "request_chain", offer.RequestChain)
	requestHeight, err := c.getBlockHeight(ctx, offer.RequestChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get request chain height: %w", err)
	}
	c.log.Debug("InitiateSwap: got request height", "trade_id", tradeID, "request_height", requestHeight)
	swap.SetBlockHeights(offerHeight, requestHeight)

	var active *ActiveSwap

	switch method {
	case MethodMuSig2:
		active, err = c.initiateMuSig2Swap(tradeID, swap, offer, privKey)
	case MethodHTLC:
		active, err = c.initiateHTLCSwap(tradeID, swap, offer, privKey)
	default:
		return nil, fmt.Errorf("unsupported swap method: %s", method)
	}
	if err != nil {
		return nil, err
	}

	c.log.Debug("InitiateSwap: storing in map", "trade_id", tradeID)
	c.swaps[tradeID] = active

	// Save swap state to database
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("InitiateSwap: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.log.Debug("InitiateSwap: emitting event", "trade_id", tradeID)
	c.emitEvent(tradeID, "swap_initiated", map[string]interface{}{
		"role":         "initiator",
		"method":       string(method),
		"offer_chain":  offer.OfferChain,
		"offer_amount": offer.OfferAmount,
	})

	c.log.Debug("InitiateSwap: completed", "trade_id", tradeID)
	return active, nil
}

// initiateMuSig2Swap creates MuSig2 sessions for the swap.
func (c *Coordinator) initiateMuSig2Swap(tradeID string, swap *Swap, offer Offer, privKey *btcec.PrivateKey) (*ActiveSwap, error) {
	c.log.Debug("initiateMuSig2Swap: creating MuSig2 sessions", "trade_id", tradeID, "offer_chain", offer.OfferChain, "request_chain", offer.RequestChain)

	offerSession, err := NewMuSig2Session(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain MuSig2 session: %w", err)
	}
	requestSession, err := NewMuSig2Session(offer.RequestChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create request chain MuSig2 session: %w", err)
	}
	c.log.Debug("initiateMuSig2Swap: MuSig2 sessions created", "trade_id", tradeID)

	return &ActiveSwap{
		Swap: swap,
		MuSig2: &MuSig2SwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainMuSig2Data{Session: offerSession},
			RequestChain: &ChainMuSig2Data{Session: requestSession},
		},
	}, nil
}

// initiateHTLCSwap creates HTLC sessions for the swap.
func (c *Coordinator) initiateHTLCSwap(tradeID string, swap *Swap, offer Offer, privKey *btcec.PrivateKey) (*ActiveSwap, error) {
	c.log.Debug("initiateHTLCSwap: creating HTLC sessions", "trade_id", tradeID, "offer_chain", offer.OfferChain, "request_chain", offer.RequestChain)

	// Create HTLC sessions with the ephemeral private key
	offerSession, err := NewHTLCSessionWithKey(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain HTLC session: %w", err)
	}
	requestSession, err := NewHTLCSessionWithKey(offer.RequestChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create request chain HTLC session: %w", err)
	}

	// Initiator generates secret for HTLC
	_, secretHash, err := offerSession.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate HTLC secret: %w", err)
	}
	// Set the same secret hash in request session
	if err := requestSession.SetSecretHash(secretHash); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in request session: %w", err)
	}
	// Also store the secret in swap
	swap.Secret = offerSession.GetSecret()
	swap.SecretHash = secretHash

	c.log.Debug("initiateHTLCSwap: HTLC sessions created", "trade_id", tradeID)

	return &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainHTLCData{Session: offerSession},
			RequestChain: &ChainHTLCData{Session: requestSession},
		},
	}, nil
}

// =============================================================================
// Swap Response (Taker side)
// =============================================================================

// RespondToSwap joins a swap as the taker (responder).
// Called when we take someone's order.
// The method parameter specifies MuSig2 or HTLC.
func (c *Coordinator) RespondToSwap(ctx context.Context, tradeID string, offer Offer, remotePubKey []byte, secretHash []byte, method Method) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if swap already exists
	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Create swap as responder
	swap, err := NewSwap(c.network, method, RoleResponder, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID
	swap.SecretHash = secretHash

	// Parse remote public key
	remotePub, err := btcec.ParsePubKey(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid remote public key: %w", err)
	}
	if err := swap.SetRemotePubKey(remotePub); err != nil {
		return nil, err
	}

	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Get current block heights
	offerHeight, err := c.getBlockHeight(ctx, offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer chain height: %w", err)
	}
	requestHeight, err := c.getBlockHeight(ctx, offer.RequestChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get request chain height: %w", err)
	}
	swap.SetBlockHeights(offerHeight, requestHeight)

	var active *ActiveSwap

	switch method {
	case MethodMuSig2:
		active, err = c.respondMuSig2Swap(tradeID, swap, offer, privKey, remotePub)
	case MethodHTLC:
		active, err = c.respondHTLCSwap(tradeID, swap, offer, privKey, remotePub, secretHash)
	default:
		return nil, fmt.Errorf("unsupported swap method: %s", method)
	}
	if err != nil {
		return nil, err
	}

	c.swaps[tradeID] = active

	// Save swap state to database
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("RespondToSwap: failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "swap_joined", map[string]interface{}{
		"role":           "responder",
		"method":         string(method),
		"request_chain":  offer.RequestChain,
		"request_amount": offer.RequestAmount,
	})

	return active, nil
}

// respondMuSig2Swap creates MuSig2 sessions for the responder.
func (c *Coordinator) respondMuSig2Swap(tradeID string, swap *Swap, offer Offer, privKey *btcec.PrivateKey, remotePub *btcec.PublicKey) (*ActiveSwap, error) {
	// Create MuSig2 sessions for both chains
	offerSession, err := NewMuSig2Session(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain MuSig2 session: %w", err)
	}
	requestSession, err := NewMuSig2Session(offer.RequestChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create request chain MuSig2 session: %w", err)
	}

	// Set remote pubkey in both sessions
	if err := offerSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey in offer session: %w", err)
	}
	if err := requestSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey in request session: %w", err)
	}

	// Generate Taproot addresses for both chains
	// CRITICAL: Both parties must use the SAME refund pubkey for each chain:
	// - Offer chain escrow (maker funds) → maker's pubkey (remotePub) for refund
	// - Request chain escrow (taker funds) → taker's pubkey (localPubKey) for refund
	localPubKey := privKey.PubKey()
	makerPubKey := remotePub   // Remote is maker
	takerPubKey := localPubKey // We are taker

	// Offer chain address - maker funds, so maker's pubkey for refund
	offerTimeoutBlocks := GetTimeoutBlocks(offer.OfferChain, true) // maker timeout
	offerTaprootAddr, err := offerSession.TaprootAddressWithRefund(makerPubKey, offerTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate offer chain taproot address: %w", err)
	}

	// Request chain address - taker funds, so taker's pubkey for refund
	requestTimeoutBlocks := GetTimeoutBlocks(offer.RequestChain, false) // taker timeout
	requestTaprootAddr, err := requestSession.TaprootAddressWithRefund(takerPubKey, requestTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate request chain taproot address: %w", err)
	}

	return &ActiveSwap{
		Swap: swap,
		MuSig2: &MuSig2SwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainMuSig2Data{Session: offerSession, TaprootAddress: offerTaprootAddr},
			RequestChain: &ChainMuSig2Data{Session: requestSession, TaprootAddress: requestTaprootAddr},
		},
	}, nil
}

// respondHTLCSwap creates HTLC sessions for the responder.
func (c *Coordinator) respondHTLCSwap(tradeID string, swap *Swap, offer Offer, privKey *btcec.PrivateKey, remotePub *btcec.PublicKey, secretHash []byte) (*ActiveSwap, error) {
	// Create HTLC sessions with the ephemeral private key
	offerSession, err := NewHTLCSessionWithKey(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain HTLC session: %w", err)
	}
	requestSession, err := NewHTLCSessionWithKey(offer.RequestChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create request chain HTLC session: %w", err)
	}

	// Responder receives the secret hash from initiator
	if err := offerSession.SetSecretHash(secretHash); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in offer session: %w", err)
	}
	if err := requestSession.SetSecretHash(secretHash); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in request session: %w", err)
	}

	// Set remote public key in both sessions
	if err := offerSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey in offer HTLC session: %w", err)
	}
	if err := requestSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey in request HTLC session: %w", err)
	}

	// Generate HTLC addresses for both chains
	// For HTLC, the roles are:
	// - Offer chain: Maker (remote) is sender, Taker (local) is receiver
	// - Request chain: Taker (local) is sender, Maker (remote) is receiver
	localPubKey := privKey.PubKey()

	offerTimeoutBlocks := GetTimeoutBlocks(offer.OfferChain, true) // maker timeout
	// Offer chain: Maker=sender, Taker=receiver
	offerHTLCAddr, err := offerSession.GenerateSwapAddressWithRoles(remotePub, localPubKey, offerTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate offer chain HTLC address: %w", err)
	}

	requestTimeoutBlocks := GetTimeoutBlocks(offer.RequestChain, false) // taker timeout
	// Request chain: Taker=sender, Maker=receiver
	requestHTLCAddr, err := requestSession.GenerateSwapAddressWithRoles(localPubKey, remotePub, requestTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate request chain HTLC address: %w", err)
	}

	return &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainHTLCData{Session: offerSession, HTLCAddress: offerHTLCAddr},
			RequestChain: &ChainHTLCData{Session: requestSession, HTLCAddress: requestHTLCAddr},
		},
	}, nil
}
