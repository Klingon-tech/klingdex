// Package swap - Cross-chain swap orchestration for EVM and Bitcoin chains.
// This file handles the coordination of swaps between different chain types.
package swap

import (
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/Klingon-tech/klingdex/internal/chain"
)

// =============================================================================
// Cross-Chain Swap Initialization
// =============================================================================

// InitiateCrossChainSwap starts a cross-chain swap as the initiator.
// Handles EVM ↔ EVM, EVM ↔ Bitcoin, and Bitcoin ↔ Bitcoin swaps.
func (c *Coordinator) InitiateCrossChainSwap(ctx context.Context, tradeID, orderID string, offer Offer) (*ActiveSwap, error) {
	// Determine swap type based on chains
	swapType := GetCrossChainSwapType(offer.OfferChain, offer.RequestChain, c.network)

	c.log.Info("Initiating cross-chain swap",
		"trade_id", tradeID,
		"offer_chain", offer.OfferChain,
		"request_chain", offer.RequestChain,
		"swap_type", swapType.String(),
	)

	switch swapType {
	case CrossChainTypeBitcoinToBitcoin:
		// Use HTLC for Bitcoin-family swaps
		return c.InitiateSwap(ctx, tradeID, orderID, offer, MethodHTLC)

	case CrossChainTypeEVMToEVM:
		return c.initiateEVMToEVMSwap(ctx, tradeID, offer)

	case CrossChainTypeBitcoinToEVM:
		return c.initiateBitcoinToEVMSwap(ctx, tradeID, offer)

	case CrossChainTypeEVMToBitcoin:
		return c.initiateEVMToBitcoinSwap(ctx, tradeID, offer)

	default:
		return nil, fmt.Errorf("unknown swap type for chains %s → %s", offer.OfferChain, offer.RequestChain)
	}
}

// RespondToCrossChainSwap joins a cross-chain swap as the responder.
func (c *Coordinator) RespondToCrossChainSwap(ctx context.Context, tradeID string, offer Offer, remotePubKey []byte, secretHash []byte, remoteEVMAddr string) (*ActiveSwap, error) {
	// Determine swap type based on chains
	swapType := GetCrossChainSwapType(offer.OfferChain, offer.RequestChain, c.network)

	c.log.Info("Responding to cross-chain swap",
		"trade_id", tradeID,
		"offer_chain", offer.OfferChain,
		"request_chain", offer.RequestChain,
		"swap_type", swapType.String(),
	)

	switch swapType {
	case CrossChainTypeBitcoinToBitcoin:
		// Use HTLC for Bitcoin-family swaps
		return c.RespondToSwap(ctx, tradeID, offer, remotePubKey, secretHash, MethodHTLC)

	case CrossChainTypeEVMToEVM:
		return c.respondEVMToEVMSwap(ctx, tradeID, offer, secretHash, remoteEVMAddr)

	case CrossChainTypeBitcoinToEVM:
		return c.respondBitcoinToEVMSwap(ctx, tradeID, offer, remotePubKey, secretHash, remoteEVMAddr)

	case CrossChainTypeEVMToBitcoin:
		return c.respondEVMToBitcoinSwap(ctx, tradeID, offer, remotePubKey, secretHash, remoteEVMAddr)

	default:
		return nil, fmt.Errorf("unknown swap type for chains %s → %s", offer.OfferChain, offer.RequestChain)
	}
}

// =============================================================================
// EVM ↔ EVM Swap
// =============================================================================

// initiateEVMToEVMSwap initiates a swap between two EVM chains.
func (c *Coordinator) initiateEVMToEVMSwap(ctx context.Context, tradeID string, offer Offer) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for EVM swaps (uses HTLC contracts)
	offer.Method = MethodHTLC

	// Create swap with HTLC method (EVM uses HTLC contracts)
	swap, err := NewSwap(c.network, MethodHTLC, RoleInitiator, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID

	// Generate secret (initiator generates)
	if err := swap.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Create EVM sessions for both chains
	offerSession, err := c.createEVMSessionForChain(offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain EVM session: %w", err)
	}
	requestSession, err := c.createEVMSessionForChain(offer.RequestChain)
	if err != nil {
		offerSession.Close()
		return nil, fmt.Errorf("failed to create request chain EVM session: %w", err)
	}

	// Set secret in both sessions
	if err := offerSession.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secretHash := offerSession.GetSecretHash()
	secret := offerSession.GetSecret()
	requestSession.SetSecretHash(secretHash)

	// Store secret in swap
	swap.Secret = secret[:]
	swap.SecretHash = secretHash[:]

	// Store local EVM wallet addresses for P2P exchange
	// Initiator receives on offer chain, sends on request chain
	swap.LocalOfferWalletAddr = offerSession.GetLocalAddress().Hex()
	swap.LocalRequestWalletAddr = requestSession.GetLocalAddress().Hex()

	active := &ActiveSwap{
		Swap: swap,
		EVMHTLC: &EVMHTLCSwapData{
			OfferChain:   &ChainEVMHTLCData{Session: offerSession},
			RequestChain: &ChainEVMHTLCData{Session: requestSession},
		},
	}

	c.swaps[tradeID] = active

	// Save swap state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "evm_swap_initiated", map[string]interface{}{
		"role":          "initiator",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
	})

	return active, nil
}

// respondEVMToEVMSwap responds to a swap between two EVM chains.
func (c *Coordinator) respondEVMToEVMSwap(ctx context.Context, tradeID string, offer Offer, secretHash []byte, remoteEVMAddr string) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for EVM swaps (uses HTLC contracts)
	offer.Method = MethodHTLC

	// Create swap as responder
	swap, err := NewSwap(c.network, MethodHTLC, RoleResponder, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID
	swap.SecretHash = secretHash

	// Create EVM sessions for both chains
	offerSession, err := c.createEVMSessionForChain(offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer chain EVM session: %w", err)
	}
	requestSession, err := c.createEVMSessionForChain(offer.RequestChain)
	if err != nil {
		offerSession.Close()
		return nil, fmt.Errorf("failed to create request chain EVM session: %w", err)
	}

	// Set secret hash in both sessions
	var hash [32]byte
	copy(hash[:], secretHash)
	offerSession.SetSecretHash(hash)
	requestSession.SetSecretHash(hash)

	// Set remote address if provided
	if remoteEVMAddr != "" {
		remoteAddr := common.HexToAddress(remoteEVMAddr)
		offerSession.SetRemoteAddress(remoteAddr)
		requestSession.SetRemoteAddress(remoteAddr)
		// Store remote addresses in swap record
		swap.RemoteOfferWalletAddr = remoteEVMAddr
		swap.RemoteRequestWalletAddr = remoteEVMAddr
	}

	// Store local EVM wallet addresses for P2P exchange
	// Responder receives on request chain, sends on offer chain
	swap.LocalOfferWalletAddr = offerSession.GetLocalAddress().Hex()
	swap.LocalRequestWalletAddr = requestSession.GetLocalAddress().Hex()

	active := &ActiveSwap{
		Swap: swap,
		EVMHTLC: &EVMHTLCSwapData{
			OfferChain:   &ChainEVMHTLCData{Session: offerSession},
			RequestChain: &ChainEVMHTLCData{Session: requestSession},
		},
	}

	c.swaps[tradeID] = active

	// Save swap state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "evm_swap_joined", map[string]interface{}{
		"role":          "responder",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
	})

	return active, nil
}

// =============================================================================
// Bitcoin ↔ EVM Swaps
// =============================================================================

// initiateBitcoinToEVMSwap initiates a swap from Bitcoin to EVM.
// Initiator offers BTC, requests EVM tokens.
func (c *Coordinator) initiateBitcoinToEVMSwap(ctx context.Context, tradeID string, offer Offer) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for cross-chain swaps (uses HTLC)
	offer.Method = MethodHTLC

	// Create swap with HTLC method
	swap, err := NewSwap(c.network, MethodHTLC, RoleInitiator, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID

	// Generate secret
	if err := swap.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Generate ephemeral key for Bitcoin side
	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Create Bitcoin HTLC session for offer chain
	btcSession, err := NewHTLCSessionWithKey(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Bitcoin HTLC session: %w", err)
	}

	// Set secret in Bitcoin session
	_, secretHash, err := btcSession.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	swap.Secret = btcSession.GetSecret()
	swap.SecretHash = secretHash

	// Create EVM session for request chain
	evmSession, err := c.createEVMSessionForChain(offer.RequestChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create EVM session: %w", err)
	}

	// Set secret hash in EVM session
	var hash [32]byte
	copy(hash[:], secretHash)
	evmSession.SetSecretHash(hash)

	// Store local wallet addresses for P2P exchange
	// Offer chain is Bitcoin (need to derive address from wallet)
	if c.wallet != nil {
		btcAddr, err := c.wallet.DeriveAddress(offer.OfferChain, 0, 0)
		if err == nil {
			swap.LocalOfferWalletAddr = btcAddr
		}
	}
	// Request chain is EVM (get from session)
	swap.LocalRequestWalletAddr = evmSession.GetLocalAddress().Hex()

	active := &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainHTLCData{Session: btcSession},
		},
		EVMHTLC: &EVMHTLCSwapData{
			RequestChain: &ChainEVMHTLCData{Session: evmSession},
		},
	}

	c.swaps[tradeID] = active

	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "cross_chain_swap_initiated", map[string]interface{}{
		"role":          "initiator",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
		"type":          "bitcoin_to_evm",
	})

	return active, nil
}

// initiateEVMToBitcoinSwap initiates a swap from EVM to Bitcoin.
// Initiator offers EVM tokens, requests BTC.
func (c *Coordinator) initiateEVMToBitcoinSwap(ctx context.Context, tradeID string, offer Offer) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for cross-chain swaps (uses HTLC)
	offer.Method = MethodHTLC

	// Create swap with HTLC method
	swap, err := NewSwap(c.network, MethodHTLC, RoleInitiator, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID

	// Generate secret
	if err := swap.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Generate ephemeral key for Bitcoin side
	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Create EVM session for offer chain
	evmSession, err := c.createEVMSessionForChain(offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create EVM session: %w", err)
	}

	// Generate secret in EVM session
	if err := evmSession.GenerateSecret(); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secretHash := evmSession.GetSecretHash()
	secret := evmSession.GetSecret()
	swap.Secret = secret[:]
	swap.SecretHash = secretHash[:]

	// Create Bitcoin HTLC session for request chain
	btcSession, err := NewHTLCSessionWithKey(offer.RequestChain, c.network, privKey)
	if err != nil {
		evmSession.Close()
		return nil, fmt.Errorf("failed to create Bitcoin HTLC session: %w", err)
	}

	// Set secret hash in Bitcoin session
	if err := btcSession.SetSecretHash(secretHash[:]); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in Bitcoin session: %w", err)
	}

	// Store local wallet addresses for P2P exchange
	// Offer chain is EVM (get from session)
	swap.LocalOfferWalletAddr = evmSession.GetLocalAddress().Hex()
	// Request chain is Bitcoin (need to derive address from wallet)
	if c.wallet != nil {
		btcAddr, err := c.wallet.DeriveAddress(offer.RequestChain, 0, 0)
		if err == nil {
			swap.LocalRequestWalletAddr = btcAddr
		}
	}

	active := &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			RequestChain: &ChainHTLCData{Session: btcSession},
		},
		EVMHTLC: &EVMHTLCSwapData{
			OfferChain: &ChainEVMHTLCData{Session: evmSession},
		},
	}

	c.swaps[tradeID] = active

	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "cross_chain_swap_initiated", map[string]interface{}{
		"role":          "initiator",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
		"type":          "evm_to_bitcoin",
	})

	return active, nil
}

// respondBitcoinToEVMSwap responds to a Bitcoin → EVM swap.
// Responder receives BTC, sends EVM tokens.
func (c *Coordinator) respondBitcoinToEVMSwap(ctx context.Context, tradeID string, offer Offer, remotePubKey []byte, secretHash []byte, remoteEVMAddr string) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for cross-chain swaps (uses HTLC)
	offer.Method = MethodHTLC

	// Create swap as responder
	swap, err := NewSwap(c.network, MethodHTLC, RoleResponder, offer)
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

	// Generate ephemeral key for Bitcoin side
	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Create Bitcoin HTLC session for offer chain (receiving BTC)
	btcSession, err := NewHTLCSessionWithKey(offer.OfferChain, c.network, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Bitcoin HTLC session: %w", err)
	}
	if err := btcSession.SetSecretHash(secretHash); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in Bitcoin session: %w", err)
	}
	if err := btcSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey: %w", err)
	}

	// Generate HTLC address now that we have both keys
	// For offer chain: maker (initiator) is sender, taker (responder) is receiver
	offerTimeoutBlocks := GetTimeoutBlocks(offer.OfferChain, true) // maker timeout
	htlcAddr, err := btcSession.GenerateSwapAddressWithRoles(remotePub, privKey.PubKey(), offerTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HTLC address: %w", err)
	}

	// Create EVM session for request chain (sending EVM tokens)
	evmSession, err := c.createEVMSessionForChain(offer.RequestChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create EVM session: %w", err)
	}

	// Set secret hash in EVM session
	var hash [32]byte
	copy(hash[:], secretHash)
	evmSession.SetSecretHash(hash)
	if remoteEVMAddr != "" {
		evmSession.SetRemoteAddress(common.HexToAddress(remoteEVMAddr))
		swap.RemoteRequestWalletAddr = remoteEVMAddr
	}

	// Store local wallet addresses for P2P exchange
	// Offer chain is Bitcoin (need to derive address from wallet)
	if c.wallet != nil {
		btcAddr, err := c.wallet.DeriveAddress(offer.OfferChain, 0, 0)
		if err == nil {
			swap.LocalOfferWalletAddr = btcAddr
		}
	}
	// Request chain is EVM (get from session)
	swap.LocalRequestWalletAddr = evmSession.GetLocalAddress().Hex()

	active := &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			OfferChain:   &ChainHTLCData{Session: btcSession, HTLCAddress: htlcAddr},
		},
		EVMHTLC: &EVMHTLCSwapData{
			RequestChain: &ChainEVMHTLCData{Session: evmSession},
		},
	}

	c.swaps[tradeID] = active

	c.log.Info("Generated BTC HTLC address for cross-chain swap",
		"trade_id", tradeID,
		"htlc_address", htlcAddr,
	)

	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "cross_chain_swap_joined", map[string]interface{}{
		"role":          "responder",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
		"type":          "bitcoin_to_evm",
		"htlc_address":  htlcAddr,
	})

	return active, nil
}

// respondEVMToBitcoinSwap responds to an EVM → Bitcoin swap.
// Responder receives EVM tokens, sends BTC.
func (c *Coordinator) respondEVMToBitcoinSwap(ctx context.Context, tradeID string, offer Offer, remotePubKey []byte, secretHash []byte, remoteEVMAddr string) (*ActiveSwap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.swaps[tradeID]; exists {
		return nil, ErrSwapExists
	}

	// Set method for cross-chain swaps (uses HTLC)
	offer.Method = MethodHTLC

	// Create swap as responder
	swap, err := NewSwap(c.network, MethodHTLC, RoleResponder, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = tradeID
	swap.SecretHash = secretHash

	// Parse remote public key (for Bitcoin side)
	remotePub, err := btcec.ParsePubKey(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid remote public key: %w", err)
	}
	if err := swap.SetRemotePubKey(remotePub); err != nil {
		return nil, err
	}

	// Generate ephemeral key for Bitcoin side
	privKey, err := GenerateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	swap.SetLocalPubKey(privKey.PubKey())

	// Create EVM session for offer chain (receiving EVM tokens)
	evmSession, err := c.createEVMSessionForChain(offer.OfferChain)
	if err != nil {
		return nil, fmt.Errorf("failed to create EVM session: %w", err)
	}

	// Set secret hash in EVM session
	var hash [32]byte
	copy(hash[:], secretHash)
	evmSession.SetSecretHash(hash)
	if remoteEVMAddr != "" {
		evmSession.SetRemoteAddress(common.HexToAddress(remoteEVMAddr))
		swap.RemoteOfferWalletAddr = remoteEVMAddr
	}

	// Create Bitcoin HTLC session for request chain (sending BTC)
	btcSession, err := NewHTLCSessionWithKey(offer.RequestChain, c.network, privKey)
	if err != nil {
		evmSession.Close()
		return nil, fmt.Errorf("failed to create Bitcoin HTLC session: %w", err)
	}
	if err := btcSession.SetSecretHash(secretHash); err != nil {
		return nil, fmt.Errorf("failed to set secret hash in Bitcoin session: %w", err)
	}
	if err := btcSession.SetRemotePubKey(remotePub); err != nil {
		return nil, fmt.Errorf("failed to set remote pubkey: %w", err)
	}

	// Generate HTLC address now that we have both keys
	// For request chain: taker (responder) is sender, maker (initiator) is receiver
	requestTimeoutBlocks := GetTimeoutBlocks(offer.RequestChain, false) // taker timeout
	htlcAddr, err := btcSession.GenerateSwapAddressWithRoles(privKey.PubKey(), remotePub, requestTimeoutBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HTLC address: %w", err)
	}

	// Store local wallet addresses for P2P exchange
	// Offer chain is EVM (get from session)
	swap.LocalOfferWalletAddr = evmSession.GetLocalAddress().Hex()
	// Request chain is Bitcoin (need to derive address from wallet)
	if c.wallet != nil {
		btcAddr, err := c.wallet.DeriveAddress(offer.RequestChain, 0, 0)
		if err == nil {
			swap.LocalRequestWalletAddr = btcAddr
		}
	}

	active := &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{
			LocalPrivKey: privKey,
			RequestChain: &ChainHTLCData{Session: btcSession, HTLCAddress: htlcAddr},
		},
		EVMHTLC: &EVMHTLCSwapData{
			OfferChain: &ChainEVMHTLCData{Session: evmSession},
		},
	}

	c.swaps[tradeID] = active

	c.log.Info("Generated BTC HTLC address for cross-chain swap",
		"trade_id", tradeID,
		"htlc_address", htlcAddr,
	)

	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "cross_chain_swap_joined", map[string]interface{}{
		"role":          "responder",
		"offer_chain":   offer.OfferChain,
		"request_chain": offer.RequestChain,
		"type":          "evm_to_bitcoin",
		"htlc_address":  htlcAddr,
	})

	return active, nil
}

// =============================================================================
// Helper Methods
// =============================================================================

// createEVMSessionForChain creates an EVM HTLC session for the given chain.
func (c *Coordinator) createEVMSessionForChain(chainSymbol string) (*EVMHTLCSession, error) {
	rpcURL := c.getEVMRPCURL(chainSymbol)
	if rpcURL == "" {
		return nil, fmt.Errorf("no RPC URL configured for chain %s", chainSymbol)
	}

	session, err := NewEVMHTLCSession(chainSymbol, c.network, rpcURL)
	if err != nil {
		return nil, err
	}

	// Set up private key from wallet
	if c.wallet != nil {
		btcPrivKey, err := c.wallet.DerivePrivateKey(chainSymbol, 0, 0)
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("failed to derive private key: %w", err)
		}
		session.SetLocalKey(btcPrivKey.ToECDSA())
	}

	return session, nil
}

// GetSwapType returns the cross-chain type for a swap.
func (c *Coordinator) GetSwapType(tradeID string) (CrossChainType, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return CrossChainTypeUnknown, ErrSwapNotFound
	}

	return GetCrossChainSwapType(
		active.Swap.Offer.OfferChain,
		active.Swap.Offer.RequestChain,
		c.network,
	), nil
}

// =============================================================================
// Timelock Coordination
// =============================================================================

// GetTimelockForChain returns the appropriate timelock for a chain based on role.
// Initiator's chain (offer) gets longer timeout, responder's chain (request) gets shorter.
func (c *Coordinator) GetTimelockForChain(tradeID, chainSymbol string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return 0, ErrSwapNotFound
	}

	isOfferChain := chainSymbol == active.Swap.Offer.OfferChain
	now := time.Now().Unix()

	// For EVM chains, use absolute timestamps
	// For Bitcoin chains, block heights are handled separately
	if IsEVMChain(chainSymbol, c.network) {
		if isOfferChain {
			// Initiator's chain: longer timeout (24h testnet, 48h mainnet)
			if c.network == chain.Testnet {
				return now + 24*60*60, nil
			}
			return now + 48*60*60, nil
		}
		// Responder's chain: shorter timeout (12h testnet, 24h mainnet)
		if c.network == chain.Testnet {
			return now + 12*60*60, nil
		}
		return now + 24*60*60, nil
	}

	// For Bitcoin chains, return block-based timeout
	return int64(GetTimeoutBlocks(chainSymbol, isOfferChain)), nil
}

// ValidateTimelockSafety ensures the timelock configuration is safe for the swap.
// The initiator's timelock must be > responder's timelock + safety margin.
// Safety margin ensures initiator can claim after seeing responder's claim reveal the secret.
func (c *Coordinator) ValidateTimelockSafety(tradeID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	swapType := GetCrossChainSwapType(
		active.Swap.Offer.OfferChain,
		active.Swap.Offer.RequestChain,
		c.network,
	)

	// For cross-chain swaps, validate timelock ordering
	if swapType.IsCrossChain() {
		offerChain := active.Swap.Offer.OfferChain
		requestChain := active.Swap.Offer.RequestChain

		// Get timelocks for both chains
		offerTimelock, err := c.GetTimelockForChain(tradeID, offerChain)
		if err != nil {
			return fmt.Errorf("failed to get offer chain timelock: %w", err)
		}

		requestTimelock, err := c.GetTimelockForChain(tradeID, requestChain)
		if err != nil {
			return fmt.Errorf("failed to get request chain timelock: %w", err)
		}

		// Safety margin: 2 hours for EVM (block time variations), 6 blocks for Bitcoin
		// For EVM↔EVM or EVM↔Bitcoin, use 2 hours as minimum margin
		safetyMarginSeconds := int64(2 * 60 * 60) // 2 hours

		// For Bitcoin chains, convert blocks to approximate seconds
		// (assuming 10 min blocks for BTC, 2.5 min for LTC)
		isOfferBitcoin := !IsEVMChain(offerChain, c.network)
		isRequestBitcoin := !IsEVMChain(requestChain, c.network)

		// Convert block heights to approximate timestamps for Bitcoin chains
		offerTimestamp := offerTimelock
		requestTimestamp := requestTimelock

		if isOfferBitcoin {
			// Bitcoin block heights - convert to approximate seconds from now
			// For BTC: ~10 min/block, for LTC: ~2.5 min/block
			blockTimeSeconds := int64(600) // default BTC
			if offerChain == "LTC" {
				blockTimeSeconds = 150 // 2.5 min
			}
			offerTimestamp = time.Now().Unix() + offerTimelock*blockTimeSeconds
		}

		if isRequestBitcoin {
			blockTimeSeconds := int64(600)
			if requestChain == "LTC" {
				blockTimeSeconds = 150
			}
			requestTimestamp = time.Now().Unix() + requestTimelock*blockTimeSeconds
		}

		// Offer chain (initiator) must have LONGER timeout than request chain (responder)
		// This is because the initiator must have time to claim after responder reveals secret
		if offerTimestamp <= requestTimestamp+safetyMarginSeconds {
			c.log.Error("Timelock safety violation",
				"trade_id", tradeID,
				"offer_chain", offerChain,
				"offer_timelock", offerTimestamp,
				"request_chain", requestChain,
				"request_timelock", requestTimestamp,
				"safety_margin", safetyMarginSeconds,
			)
			return fmt.Errorf("unsafe timelock configuration: offer chain timelock (%d) must be at least %d seconds greater than request chain timelock (%d)",
				offerTimestamp, safetyMarginSeconds, requestTimestamp)
		}

		c.log.Debug("Timelock safety validated",
			"trade_id", tradeID,
			"swap_type", swapType.String(),
			"offer_timelock", offerTimestamp,
			"request_timelock", requestTimestamp,
			"margin", offerTimestamp-requestTimestamp,
		)
	}

	return nil
}
