// Package swap - Helper functions for the Coordinator.
package swap

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
)

// =============================================================================
// Chain Data Helpers
// =============================================================================

// getChainData returns MuSig2 chain data for the given chain symbol.
// NOTE: Caller must hold c.mu lock.
func (c *Coordinator) getChainData(active *ActiveSwap, chainSymbol string) *ChainMuSig2Data {
	if chainSymbol == active.Swap.Offer.OfferChain {
		return active.MuSig2.OfferChain
	}
	if chainSymbol == active.Swap.Offer.RequestChain {
		return active.MuSig2.RequestChain
	}
	return nil
}

// getChainDataRLocked returns MuSig2 chain data for the given chain symbol.
// NOTE: Caller must hold c.mu read lock.
func (c *Coordinator) getChainDataRLocked(active *ActiveSwap, chainSymbol string) *ChainMuSig2Data {
	if chainSymbol == active.Swap.Offer.OfferChain {
		return active.MuSig2.OfferChain
	}
	if chainSymbol == active.Swap.Offer.RequestChain {
		return active.MuSig2.RequestChain
	}
	return nil
}

// =============================================================================
// Swap Lookup
// =============================================================================

// GetSwap returns an active swap by trade ID.
func (c *Coordinator) GetSwap(tradeID string) (*ActiveSwap, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	return active, nil
}

// =============================================================================
// Backend Helpers
// =============================================================================

// getBlockHeight gets the current block height for a chain.
func (c *Coordinator) getBlockHeight(ctx context.Context, chainSymbol string) (uint32, error) {
	b, ok := c.backends[chainSymbol]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoBackend, chainSymbol)
	}

	height, err := b.GetBlockHeight(ctx)
	if err != nil {
		return 0, err
	}

	return uint32(height), nil
}

// getConfirmations gets the confirmation count for a transaction.
func (c *Coordinator) getConfirmations(ctx context.Context, chainSymbol, txID string) (uint32, error) {
	b, ok := c.backends[chainSymbol]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNoBackend, chainSymbol)
	}

	tx, err := b.GetTransaction(ctx, txID)
	if err != nil {
		return 0, err
	}

	return uint32(tx.Confirmations), nil
}

// getWalletAddress derives a wallet address for a chain using proper index management.
// It tracks used indices in storage to avoid address reuse.
func (c *Coordinator) getWalletAddress(chainSymbol string) (string, error) {
	if c.wallet == nil {
		return "", ErrNoWallet
	}

	const account = uint32(0)
	const change = uint32(0) // External addresses

	// Get the next available address index from storage
	nextIndex := uint32(0)
	if c.store != nil {
		var err error
		nextIndex, err = c.store.GetNextAddressIndex(chainSymbol, account, change)
		if err != nil {
			c.log.Warn("Failed to get next address index, using 0", "chain", chainSymbol, "error", err)
			nextIndex = 0
		}
	}

	// Derive the address at the next index
	addr, err := c.wallet.DeriveAddress(chainSymbol, account, nextIndex)
	if err != nil {
		return "", err
	}

	// Save the address to storage for tracking
	if c.store != nil {
		walletAddr := &storage.WalletAddress{
			Address:      addr,
			Chain:        chainSymbol,
			Account:      account,
			Change:       change,
			AddressIndex: nextIndex,
			AddressType:  "p2wpkh", // Default for Bitcoin-like chains
		}
		if err := c.store.SaveWalletAddress(walletAddr); err != nil {
			c.log.Warn("Failed to save wallet address", "address", addr, "error", err)
		} else {
			c.log.Debug("Derived new wallet address", "chain", chainSymbol, "index", nextIndex, "address", addr)
		}
	}

	return addr, nil
}

// getWalletAddressAtIndex derives a wallet address at a specific index (for deterministic use).
func (c *Coordinator) getWalletAddressAtIndex(chainSymbol string, index uint32) (string, error) {
	if c.wallet == nil {
		return "", ErrNoWallet
	}

	return c.wallet.DeriveAddress(chainSymbol, 0, index)
}

// Network returns the network the coordinator is configured for.
func (c *Coordinator) Network() chain.Network {
	return c.network
}

// GetBackend returns the backend for a chain symbol.
func (c *Coordinator) GetBackend(chainSymbol string) (backend.Backend, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.backends[chainSymbol]
	return b, ok
}

// =============================================================================
// Signature Helpers
// =============================================================================

// parsePartialSig parses a hex-encoded partial signature.
func parsePartialSig(hexSig string) *musig2.PartialSignature {
	sigBytes, err := hex.DecodeString(hexSig)
	if err != nil || len(sigBytes) != 32 {
		return nil
	}
	var sBytes [32]byte
	copy(sBytes[:], sigBytes)
	var sScalar secp256k1.ModNScalar
	sScalar.SetBytes(&sBytes)
	return &musig2.PartialSignature{S: &sScalar}
}
