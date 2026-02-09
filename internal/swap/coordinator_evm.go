// Package swap - EVM HTLC operations for the Coordinator.
// This file contains methods for creating, claiming, and refunding EVM HTLC swaps.
package swap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/Klingon-tech/klingdex/internal/backend"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/internal/config"
	"github.com/Klingon-tech/klingdex/internal/contracts/htlc"
)

// =============================================================================
// EVM HTLC Creation
// =============================================================================

// CreateEVMHTLC creates an HTLC on an EVM chain.
// This is called after the swap has been initialized and parameters are set.
func (c *Coordinator) CreateEVMHTLC(ctx context.Context, tradeID string, chainSymbol string) (common.Hash, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return common.Hash{}, ErrSwapNotFound
	}

	// Validate chain is EVM
	if !IsEVMChain(chainSymbol, c.network) {
		return common.Hash{}, fmt.Errorf("chain %s is not an EVM chain", chainSymbol)
	}

	// Get the EVM session for this chain
	evmSession, err := c.getOrCreateEVMSession(active, chainSymbol)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get EVM session: %w", err)
	}

	// Validate receiver is set (requires P2P address exchange to have completed)
	receiver := evmSession.GetRemoteAddress()
	if receiver == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("counterparty EVM address not set - ensure P2P address exchange is complete before creating HTLC")
	}

	// Determine if this is a native token or ERC20 swap
	// For now, we assume native token. Token address can be added to swap params later.
	isNativeToken := evmSession.tokenAddress == (common.Address{})

	var txHash common.Hash
	if isNativeToken {
		txHash, err = evmSession.CreateSwapNative(ctx)
	} else {
		txHash, err = evmSession.CreateSwapERC20(ctx)
	}

	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create EVM HTLC: %w", err)
	}

	swapID := evmSession.GetSwapID()
	c.log.Info("Created EVM HTLC",
		"trade_id", tradeID,
		"chain", chainSymbol,
		"tx_hash", txHash.Hex(),
		"swap_id", common.Bytes2Hex(swapID[:]),
	)

	// Emit event
	c.emitEvent(tradeID, "evm_htlc_created", map[string]interface{}{
		"chain":   chainSymbol,
		"tx_hash": txHash.Hex(),
		"swap_id": common.Bytes2Hex(swapID[:]),
	})

	return txHash, nil
}

// =============================================================================
// EVM HTLC Claim
// =============================================================================

// ClaimEVMHTLC claims an EVM HTLC using the secret.
func (c *Coordinator) ClaimEVMHTLC(ctx context.Context, tradeID string, chainSymbol string) (common.Hash, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return common.Hash{}, ErrSwapNotFound
	}

	// Validate chain is EVM
	if !IsEVMChain(chainSymbol, c.network) {
		return common.Hash{}, fmt.Errorf("chain %s is not an EVM chain", chainSymbol)
	}

	// Get the EVM session for this chain
	evmSession, err := c.getEVMSession(active, chainSymbol)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get EVM session: %w", err)
	}

	// Ensure we have the secret
	if !evmSession.HasSecret() {
		// Try to get secret from the other chain's session
		secret, err := c.getSecretFromSwap(active)
		if err != nil {
			return common.Hash{}, fmt.Errorf("secret not available for claim: %w", err)
		}
		if err := evmSession.SetSecret(secret); err != nil {
			return common.Hash{}, fmt.Errorf("failed to set secret: %w", err)
		}
	}

	// Claim the HTLC
	txHash, err := evmSession.Claim(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to claim EVM HTLC: %w", err)
	}

	c.log.Info("Claimed EVM HTLC",
		"trade_id", tradeID,
		"chain", chainSymbol,
		"tx_hash", txHash.Hex(),
	)

	// Emit event
	c.emitEvent(tradeID, "evm_htlc_claimed", map[string]interface{}{
		"chain":   chainSymbol,
		"tx_hash": txHash.Hex(),
	})

	return txHash, nil
}

// =============================================================================
// EVM HTLC Refund
// =============================================================================

// RefundEVMHTLC refunds an EVM HTLC after timeout.
func (c *Coordinator) RefundEVMHTLC(ctx context.Context, tradeID string, chainSymbol string) (common.Hash, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return common.Hash{}, ErrSwapNotFound
	}

	// Validate chain is EVM
	if !IsEVMChain(chainSymbol, c.network) {
		return common.Hash{}, fmt.Errorf("chain %s is not an EVM chain", chainSymbol)
	}

	// Get the EVM session for this chain
	evmSession, err := c.getEVMSession(active, chainSymbol)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get EVM session: %w", err)
	}

	// Check if refund is possible
	canRefund, err := evmSession.CanRefund(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to check refund status: %w", err)
	}
	if !canRefund {
		remaining, _ := evmSession.TimeUntilRefund(ctx)
		return common.Hash{}, fmt.Errorf("cannot refund yet, %s seconds remaining", remaining)
	}

	// Refund the HTLC
	txHash, err := evmSession.Refund(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to refund EVM HTLC: %w", err)
	}

	c.log.Info("Refunded EVM HTLC",
		"trade_id", tradeID,
		"chain", chainSymbol,
		"tx_hash", txHash.Hex(),
	)

	// Emit event
	c.emitEvent(tradeID, "evm_htlc_refunded", map[string]interface{}{
		"chain":   chainSymbol,
		"tx_hash": txHash.Hex(),
	})

	return txHash, nil
}

// =============================================================================
// EVM HTLC Status
// =============================================================================

// GetEVMHTLCStatus returns the on-chain status of an EVM HTLC.
func (c *Coordinator) GetEVMHTLCStatus(ctx context.Context, tradeID string, chainSymbol string) (*htlc.Swap, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	// Get the EVM session for this chain
	evmSession, err := c.getEVMSession(active, chainSymbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM session: %w", err)
	}

	return evmSession.GetSwapFromChain(ctx)
}

// =============================================================================
// EVM Secret Monitoring
// =============================================================================

// WaitForEVMSecret waits for the counterparty to claim and reveal the secret.
func (c *Coordinator) WaitForEVMSecret(ctx context.Context, tradeID string, chainSymbol string) ([32]byte, error) {
	c.mu.RLock()
	active, ok := c.swaps[tradeID]
	c.mu.RUnlock()

	if !ok {
		return [32]byte{}, ErrSwapNotFound
	}

	// Get the EVM session for this chain
	evmSession, err := c.getEVMSession(active, chainSymbol)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to get EVM session: %w", err)
	}

	secret, err := evmSession.WaitForSecret(ctx)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed waiting for secret: %w", err)
	}

	c.log.Info("Received secret from EVM claim",
		"trade_id", tradeID,
		"chain", chainSymbol,
	)

	// Store secret in swap and propagate to other chains
	c.mu.Lock()
	active.Swap.Secret = secret[:]
	c.mu.Unlock()

	// Emit event
	c.emitEvent(tradeID, "secret_revealed", map[string]interface{}{
		"chain":  chainSymbol,
		"source": "evm_claim",
	})

	return secret, nil
}

// =============================================================================
// Set Secret (for responders)
// =============================================================================

// SetEVMSecret sets the secret for an EVM HTLC swap.
// This is used when the counterparty reveals the secret on another chain
// and we need to use it to claim on our chain.
func (c *Coordinator) SetEVMSecret(tradeID string, secret [32]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	// Store secret in the swap record
	active.Swap.Secret = secret[:]

	// Set secret in any existing EVM sessions
	if active.EVMHTLC != nil {
		if active.EVMHTLC.OfferChain != nil && active.EVMHTLC.OfferChain.Session != nil {
			if err := active.EVMHTLC.OfferChain.Session.SetSecret(secret); err != nil {
				c.log.Warn("Failed to set secret on offer chain session", "error", err)
			}
		}
		if active.EVMHTLC.RequestChain != nil && active.EVMHTLC.RequestChain.Session != nil {
			if err := active.EVMHTLC.RequestChain.Session.SetSecret(secret); err != nil {
				c.log.Warn("Failed to set secret on request chain session", "error", err)
			}
		}
	}

	c.log.Info("Set secret for EVM swap",
		"trade_id", tradeID,
	)

	// Save state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("Failed to save swap state after setting secret", "error", err)
	}

	return nil
}

// =============================================================================
// Helper Methods
// =============================================================================

// getOrCreateEVMSession gets or creates an EVM session for the given chain.
func (c *Coordinator) getOrCreateEVMSession(active *ActiveSwap, chainSymbol string) (*EVMHTLCSession, error) {
	// Check if we already have an EVM HTLC data structure
	if active.EVMHTLC == nil {
		active.EVMHTLC = &EVMHTLCSwapData{}
	}

	// Determine which chain this is (offer or request)
	isOfferChain := chainSymbol == active.Swap.Offer.OfferChain
	isRequestChain := chainSymbol == active.Swap.Offer.RequestChain

	if !isOfferChain && !isRequestChain {
		return nil, fmt.Errorf("chain %s is not part of this swap", chainSymbol)
	}

	// Get existing session or create new one
	var evmData *ChainEVMHTLCData
	if isOfferChain {
		evmData = active.EVMHTLC.OfferChain
	} else {
		evmData = active.EVMHTLC.RequestChain
	}

	if evmData != nil && evmData.Session != nil {
		// Existing session - ensure swap params are set (they may not have been set during initial creation)
		session := evmData.Session
		c.ensureEVMSwapParamsSet(active, chainSymbol, session)
		return session, nil
	}

	// Create new session
	rpcURL := c.getEVMRPCURL(chainSymbol)
	if rpcURL == "" {
		return nil, fmt.Errorf("no RPC URL configured for chain %s", chainSymbol)
	}

	session, err := NewEVMHTLCSession(chainSymbol, c.network, rpcURL)
	if err != nil {
		return nil, err
	}

	// Set up the session with keys and parameters
	privKey, err := c.getEVMPrivateKey(active, chainSymbol)
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}
	session.SetLocalKey(privKey)

	// Set secret/hash based on role
	if active.Swap.Role == RoleInitiator {
		// Initiator generates secret
		if len(active.Swap.SecretHash) == 0 {
			if err := session.GenerateSecret(); err != nil {
				session.Close()
				return nil, err
			}
			// Store secret in swap
			secret := session.GetSecret()
			hash := session.GetSecretHash()
			active.Swap.Secret = secret[:]
			active.Swap.SecretHash = hash[:]
		} else {
			// Restore from existing
			var hash [32]byte
			copy(hash[:], active.Swap.SecretHash)
			session.SetSecretHash(hash)
			if len(active.Swap.Secret) == 32 {
				var secret [32]byte
				copy(secret[:], active.Swap.Secret)
				session.SetSecret(secret)
			}
		}
	} else {
		// Responder uses initiator's hash
		if len(active.Swap.SecretHash) == 32 {
			var hash [32]byte
			copy(hash[:], active.Swap.SecretHash)
			session.SetSecretHash(hash)
		}
	}

	// Set swap parameters
	swapID, receiver, amount, timelock := c.computeEVMSwapParams(active, chainSymbol, session)
	session.SetSwapParams(swapID, receiver, common.Address{}, amount, timelock)

	// Store session
	chainParams, _ := chain.Get(chainSymbol, c.network)
	contractAddr := config.GetHTLCContract(chainParams.ChainID)

	newData := &ChainEVMHTLCData{
		Session:         session,
		ContractAddress: contractAddr,
		SwapID:          swapID,
	}

	if isOfferChain {
		active.EVMHTLC.OfferChain = newData
	} else {
		active.EVMHTLC.RequestChain = newData
	}

	return session, nil
}

// getEVMSession gets an EVM session, creating one lazily if needed.
// It ensures swap params are set on the session so it can query/interact with on-chain HTLCs.
func (c *Coordinator) getEVMSession(active *ActiveSwap, chainSymbol string) (*EVMHTLCSession, error) {
	// Initialize EVMHTLC data if nil (e.g., recovered from storage)
	if active.EVMHTLC == nil {
		active.EVMHTLC = &EVMHTLCSwapData{}
	}

	isOfferChain := chainSymbol == active.Swap.Offer.OfferChain
	isRequestChain := chainSymbol == active.Swap.Offer.RequestChain

	if !isOfferChain && !isRequestChain {
		return nil, fmt.Errorf("chain %s is not part of this swap", chainSymbol)
	}

	var evmData *ChainEVMHTLCData
	if isOfferChain {
		evmData = active.EVMHTLC.OfferChain
	} else {
		evmData = active.EVMHTLC.RequestChain
	}

	// Create session lazily if needed (for recovered swaps)
	if evmData == nil || evmData.Session == nil {
		c.log.Debug("Creating EVM session lazily", "chain", chainSymbol, "trade_id", active.Swap.ID)
		rpcURL := c.getEVMRPCURL(chainSymbol)
		if rpcURL == "" {
			return nil, fmt.Errorf("no RPC URL configured for chain %s", chainSymbol)
		}

		session, err := NewEVMHTLCSession(chainSymbol, c.network, rpcURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create EVM session: %w", err)
		}

		// Set local key
		privKey, err := c.getEVMPrivateKey(active, chainSymbol)
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("failed to get private key: %w", err)
		}
		session.SetLocalKey(privKey)

		// Set secret hash
		if len(active.Swap.SecretHash) == 32 {
			var hash [32]byte
			copy(hash[:], active.Swap.SecretHash)
			session.SetSecretHash(hash)
		}

		// Set secret if we have it
		if len(active.Swap.Secret) == 32 {
			var secret [32]byte
			copy(secret[:], active.Swap.Secret)
			_ = session.SetSecret(secret)
		}

		// Get contract address
		chainParams, _ := chain.Get(chainSymbol, c.network)
		contractAddr := config.GetHTLCContract(chainParams.ChainID)

		evmData = &ChainEVMHTLCData{
			Session:         session,
			ContractAddress: contractAddr,
		}

		// Store the session
		if isOfferChain {
			active.EVMHTLC.OfferChain = evmData
		} else {
			active.EVMHTLC.RequestChain = evmData
		}
	}

	// Ensure swap params are set - this is critical for nodes that didn't create the HTLC
	// on this chain but need to query status or claim it
	c.ensureEVMSwapParamsSet(active, chainSymbol, evmData.Session)

	return evmData.Session, nil
}

// getEVMRPCURL gets the RPC URL for an EVM chain from the backend config.
func (c *Coordinator) getEVMRPCURL(chainSymbol string) string {
	// Get from backend if available
	if b, ok := c.backends[chainSymbol]; ok {
		// Try to get URL from backend via interface
		if urlGetter, ok := b.(interface{ GetURL() string }); ok {
			if url := urlGetter.GetURL(); url != "" {
				return url
			}
		}
	}

	// Use backend.DefaultConfigs() for consistent RPC URLs across the codebase
	// This ensures we use the same RPC endpoints defined in backend/backend.go
	backendConfigs := backend.DefaultConfigs()
	if cfg, ok := backendConfigs[chainSymbol]; ok {
		if c.network == chain.Testnet {
			return cfg.TestnetURL
		}
		return cfg.MainnetURL
	}

	// Chain not found in configs
	c.log.Warn("No RPC URL configured for EVM chain", "chain", chainSymbol)
	return ""
}

// getEVMPrivateKey derives or retrieves the EVM private key for the swap.
func (c *Coordinator) getEVMPrivateKey(active *ActiveSwap, chainSymbol string) (*ecdsa.PrivateKey, error) {
	if c.wallet == nil {
		return nil, ErrNoWallet
	}

	// Derive key from wallet
	// Use account 0, index based on trade to ensure uniqueness
	// For now, use index 0 (same as wallet's default address)
	btcPrivKey, err := c.wallet.DerivePrivateKey(chainSymbol, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to derive private key: %w", err)
	}

	// Convert btcec private key to ecdsa private key
	// btcec uses the same curve (secp256k1) as Ethereum
	return btcPrivKey.ToECDSA(), nil
}

// computeEVMSwapParams computes the swap parameters for an EVM HTLC.
func (c *Coordinator) computeEVMSwapParams(active *ActiveSwap, chainSymbol string, session *EVMHTLCSession) (swapID [32]byte, receiver common.Address, amount *big.Int, timelock *big.Int) {
	isOfferChain := chainSymbol == active.Swap.Offer.OfferChain

	// Determine amount based on chain
	if isOfferChain {
		amount = big.NewInt(int64(active.Swap.Offer.OfferAmount))
	} else {
		amount = big.NewInt(int64(active.Swap.Offer.RequestAmount))
	}

	// Determine receiver (counterparty's address)
	// For offer chain: receiver is counterparty's offer chain address
	// For request chain: receiver is counterparty's request chain address
	if isOfferChain {
		if active.Swap.RemoteOfferWalletAddr != "" {
			receiver = common.HexToAddress(active.Swap.RemoteOfferWalletAddr)
		} else {
			// Fallback: check if session has remote address
			receiver = session.GetRemoteAddress()
		}
	} else {
		if active.Swap.RemoteRequestWalletAddr != "" {
			receiver = common.HexToAddress(active.Swap.RemoteRequestWalletAddr)
		} else {
			// Fallback: check if session has remote address
			receiver = session.GetRemoteAddress()
		}
	}

	// Validate we have a receiver - this is required for HTLC creation
	if receiver == (common.Address{}) {
		c.log.Error("No counterparty EVM address set for swap - cannot create HTLC",
			"chain", chainSymbol,
			"is_offer_chain", isOfferChain,
			"remote_offer_addr", active.Swap.RemoteOfferWalletAddr,
			"remote_request_addr", active.Swap.RemoteRequestWalletAddr,
		)
	}

	// Timelock: initiator's chain has longer timeout
	// Offer chain = initiator funds first, so longer timeout
	// Request chain = responder funds, shorter timeout
	now := time.Now().Unix()
	if isOfferChain {
		// Initiator's chain: 24 hours for testnet, 48 hours for mainnet
		if c.network == chain.Testnet {
			timelock = big.NewInt(now + 24*60*60)
		} else {
			timelock = big.NewInt(now + 48*60*60)
		}
	} else {
		// Responder's chain: 12 hours for testnet, 24 hours for mainnet
		if c.network == chain.Testnet {
			timelock = big.NewInt(now + 12*60*60)
		} else {
			timelock = big.NewInt(now + 24*60*60)
		}
	}

	// Compute swap ID from parameters
	// Use a deterministic ID based on trade parameters
	secretHash := session.GetSecretHash()
	// Use Swap.ID (always set) instead of Trade.ID (may be nil)
	tradeID := active.Swap.ID
	swapIDData := append([]byte(tradeID), secretHash[:]...)
	swapIDData = append(swapIDData, []byte(chainSymbol)...)
	swapID = crypto.Keccak256Hash(swapIDData)

	return swapID, receiver, amount, timelock
}

// getSecretFromSwap retrieves the secret from the swap, checking all sources.
func (c *Coordinator) getSecretFromSwap(active *ActiveSwap) ([32]byte, error) {
	// Check swap record
	if len(active.Swap.Secret) == 32 {
		var secret [32]byte
		copy(secret[:], active.Swap.Secret)
		return secret, nil
	}

	// Check EVM sessions
	if active.EVMHTLC != nil {
		if active.EVMHTLC.OfferChain != nil && active.EVMHTLC.OfferChain.Session != nil {
			if active.EVMHTLC.OfferChain.Session.HasSecret() {
				return active.EVMHTLC.OfferChain.Session.GetSecret(), nil
			}
		}
		if active.EVMHTLC.RequestChain != nil && active.EVMHTLC.RequestChain.Session != nil {
			if active.EVMHTLC.RequestChain.Session.HasSecret() {
				return active.EVMHTLC.RequestChain.Session.GetSecret(), nil
			}
		}
	}

	// Check Bitcoin HTLC sessions
	if active.HTLC != nil {
		if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
			secret := active.HTLC.OfferChain.Session.GetSecret()
			if len(secret) == 32 {
				var s [32]byte
				copy(s[:], secret)
				return s, nil
			}
		}
		if active.HTLC.RequestChain != nil && active.HTLC.RequestChain.Session != nil {
			secret := active.HTLC.RequestChain.Session.GetSecret()
			if len(secret) == 32 {
				var s [32]byte
				copy(s[:], secret)
				return s, nil
			}
		}
	}

	return [32]byte{}, fmt.Errorf("secret not found in any session")
}

// ensureEVMSwapParamsSet ensures swap parameters are set on an existing session.
// This is needed because sessions may be created during swap init but params
// are only computed when the HTLC is actually being created.
func (c *Coordinator) ensureEVMSwapParamsSet(active *ActiveSwap, chainSymbol string, session *EVMHTLCSession) {
	// Check if swap params are already set (swapID is non-zero)
	swapID := session.GetSwapID()
	if swapID != ([32]byte{}) {
		return // Already set
	}

	// Ensure secret hash is set in the session
	if len(active.Swap.SecretHash) == 32 {
		var hash [32]byte
		copy(hash[:], active.Swap.SecretHash)
		session.SetSecretHash(hash)
	}

	// Ensure secret is set if we have it (initiator)
	if len(active.Swap.Secret) == 32 {
		var secret [32]byte
		copy(secret[:], active.Swap.Secret)
		_ = session.SetSecret(secret) // Ignore error if hash mismatch
	}

	// Set swap parameters
	newSwapID, receiver, amount, timelock := c.computeEVMSwapParams(active, chainSymbol, session)
	session.SetSwapParams(newSwapID, receiver, common.Address{}, amount, timelock)

	c.log.Debug("Set EVM swap params on existing session",
		"chain", chainSymbol,
		"swap_id", common.Bytes2Hex(newSwapID[:])[:16],
		"receiver", receiver.Hex(),
		"amount", amount,
	)
}
