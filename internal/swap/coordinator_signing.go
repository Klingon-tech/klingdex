// Package swap - Signature operations for the Coordinator.
package swap

import (
	"context"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common"
)

// =============================================================================
// Signing
// =============================================================================

// CreatePartialSignatures creates partial signatures for both chains.
// Returns offer chain sig and request chain sig (each 32 bytes).
func (c *Coordinator) CreatePartialSignatures(ctx context.Context, tradeID string, offerSighash, requestSighash []byte) (offerSig, requestSig []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, nil, ErrSwapNotFound
	}

	if active.Swap.State != StateFunded {
		return nil, nil, ErrNotReadyToSign
	}

	// Check safety margin
	offerHeight, _ := c.getBlockHeight(ctx, active.Swap.Offer.OfferChain)
	requestHeight, _ := c.getBlockHeight(ctx, active.Swap.Offer.RequestChain)
	if err := active.Swap.IsSafeToComplete(offerHeight, requestHeight); err != nil {
		return nil, nil, err
	}

	if len(offerSighash) != 32 || len(requestSighash) != 32 {
		return nil, nil, fmt.Errorf("invalid sighash length: expected 32 bytes each")
	}

	// Sign offer chain
	if err := active.MuSig2.OfferChain.Session.InitSigningSession(); err != nil {
		return nil, nil, fmt.Errorf("failed to init offer chain signing session: %w", err)
	}
	offerMsgHash, _ := chainhash.NewHash(offerSighash)
	offerPartialSig, err := active.MuSig2.OfferChain.Session.Sign(offerMsgHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create offer chain partial signature: %w", err)
	}
	active.MuSig2.OfferChain.PartialSig = offerPartialSig

	// Sign request chain
	if err := active.MuSig2.RequestChain.Session.InitSigningSession(); err != nil {
		return nil, nil, fmt.Errorf("failed to init request chain signing session: %w", err)
	}
	requestMsgHash, _ := chainhash.NewHash(requestSighash)
	requestPartialSig, err := active.MuSig2.RequestChain.Session.Sign(requestMsgHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request chain partial signature: %w", err)
	}
	active.MuSig2.RequestChain.PartialSig = requestPartialSig

	// Persist state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("CreatePartialSignatures: failed to save state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "partial_sigs_created", nil)

	offerSigBytes := offerPartialSig.S.Bytes()
	requestSigBytes := requestPartialSig.S.Bytes()
	return offerSigBytes[:], requestSigBytes[:], nil
}

// CombineSignatures combines partial signatures for a specific chain.
func (c *Coordinator) CombineSignatures(tradeID string, chainSymbol string, remotePartialSig []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	chainData := c.getChainData(active, chainSymbol)
	if chainData == nil {
		return nil, fmt.Errorf("unknown chain: %s", chainSymbol)
	}

	if chainData.PartialSig == nil {
		return nil, errors.New("local partial signature not created for this chain")
	}

	if len(remotePartialSig) != 32 {
		return nil, fmt.Errorf("invalid partial sig length: expected 32, got %d", len(remotePartialSig))
	}

	var sBytes [32]byte
	copy(sBytes[:], remotePartialSig)

	var sScalar secp256k1.ModNScalar
	overflow := sScalar.SetBytes(&sBytes)
	if overflow != 0 {
		return nil, errors.New("partial signature scalar overflow")
	}
	remoteSig := &musig2.PartialSignature{S: &sScalar}

	finalSig, err := chainData.Session.CombineSignatures(chainData.PartialSig, remoteSig)
	if err != nil {
		return nil, fmt.Errorf("failed to combine signatures: %w", err)
	}

	c.emitEvent(tradeID, "signatures_combined", map[string]interface{}{"chain": chainSymbol})
	return finalSig.Serialize(), nil
}

// SetRemotePartialSigs stores the remote partial signatures received via P2P.
func (c *Coordinator) SetRemotePartialSigs(tradeID string, offerSig, requestSig []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	if len(offerSig) != 32 || len(requestSig) != 32 {
		return fmt.Errorf("invalid partial sig length: expected 32 bytes each")
	}

	// Parse offer chain signature
	var offerBytes [32]byte
	copy(offerBytes[:], offerSig)
	var offerScalar secp256k1.ModNScalar
	if offerScalar.SetBytes(&offerBytes) != 0 {
		return errors.New("offer signature scalar overflow")
	}
	active.MuSig2.OfferChain.RemotePartialSig = &musig2.PartialSignature{S: &offerScalar}

	// Parse request chain signature
	var requestBytes [32]byte
	copy(requestBytes[:], requestSig)
	var requestScalar secp256k1.ModNScalar
	if requestScalar.SetBytes(&requestBytes) != 0 {
		return errors.New("request signature scalar overflow")
	}
	active.MuSig2.RequestChain.RemotePartialSig = &musig2.PartialSignature{S: &requestScalar}

	// Persist state
	if err := c.saveSwapState(tradeID); err != nil {
		c.log.Warn("SetRemotePartialSigs: failed to save state", "trade_id", tradeID, "error", err)
	}

	c.emitEvent(tradeID, "remote_partial_sigs_received", nil)
	return nil
}

// HasRemotePartialSigs returns whether both remote partial signatures have been received.
func (c *Coordinator) HasRemotePartialSigs(tradeID string) (hasOffer, hasRequest bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return false, false
	}

	hasOffer = active.MuSig2.OfferChain != nil && active.MuSig2.OfferChain.RemotePartialSig != nil
	hasRequest = active.MuSig2.RequestChain != nil && active.MuSig2.RequestChain.RemotePartialSig != nil
	return
}

// GetRemotePartialSig returns the stored remote partial signature for a chain.
func (c *Coordinator) GetRemotePartialSig(tradeID string, chainSymbol string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	chainData := c.getChainDataRLocked(active, chainSymbol)
	if chainData == nil {
		return nil, fmt.Errorf("unknown chain: %s", chainSymbol)
	}

	if chainData.RemotePartialSig == nil {
		return nil, errors.New("remote partial signature not set for this chain")
	}

	sigBytes := chainData.RemotePartialSig.S.Bytes()
	return sigBytes[:], nil
}

// SetRemoteWalletAddresses sets the counterparty's wallet addresses for both chains.
func (c *Coordinator) SetRemoteWalletAddresses(tradeID string, offerAddr, requestAddr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	active.Swap.RemoteOfferWalletAddr = offerAddr
	active.Swap.RemoteRequestWalletAddr = requestAddr

	// Also update EVM sessions if they exist
	if active.EVMHTLC != nil {
		if active.EVMHTLC.OfferChain != nil && active.EVMHTLC.OfferChain.Session != nil && offerAddr != "" {
			active.EVMHTLC.OfferChain.Session.SetRemoteAddress(common.HexToAddress(offerAddr))
			c.log.Debug("Updated EVM offer session remote address", "trade_id", tradeID, "addr", offerAddr)
		}
		if active.EVMHTLC.RequestChain != nil && active.EVMHTLC.RequestChain.Session != nil && requestAddr != "" {
			active.EVMHTLC.RequestChain.Session.SetRemoteAddress(common.HexToAddress(requestAddr))
			c.log.Debug("Updated EVM request session remote address", "trade_id", tradeID, "addr", requestAddr)
		}
	}

	// Save updated state
	go c.saveSwapState(tradeID)

	return nil
}
