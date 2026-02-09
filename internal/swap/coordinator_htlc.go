// Package swap - HTLC claim and refund operations for the Coordinator.
package swap

import (
	"context"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/config"
)

// =============================================================================
// HTLC Claim and Refund Methods
// =============================================================================

// ClaimHTLC claims the HTLC output on the specified chain using the secret.
// For initiator: claims responder's chain (request chain) after funding
// For responder: claims initiator's chain (offer chain) after secret is revealed
func (c *Coordinator) ClaimHTLC(ctx context.Context, tradeID string, chainSymbol string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", ErrSwapNotFound
	}

	if active.Swap.Offer.Method != MethodHTLC {
		return "", fmt.Errorf("swap method is %s, not HTLC", active.Swap.Offer.Method)
	}

	if active.HTLC == nil {
		return "", fmt.Errorf("no HTLC data for swap")
	}

	// Get the appropriate HTLC session based on the chain
	var htlcSession *HTLCSession
	var fundingTxID string
	var fundingVout uint32
	var fundingAmount uint64

	// Determine claim scenario:
	// - Initiator claims on request chain (responder's chain)
	// - Responder claims on offer chain (initiator's chain)
	if chainSymbol == active.Swap.Offer.RequestChain {
		// Initiator claiming responder's funds
		htlcSession = active.HTLC.RequestChain.Session
		fundingTxID = active.Swap.RemoteFundingTxID
		fundingVout = active.Swap.RemoteFundingVout
		fundingAmount = active.Swap.Offer.RequestAmount
	} else if chainSymbol == active.Swap.Offer.OfferChain {
		// Responder claiming initiator's funds
		htlcSession = active.HTLC.OfferChain.Session
		fundingTxID = active.Swap.RemoteFundingTxID
		fundingVout = active.Swap.RemoteFundingVout
		fundingAmount = active.Swap.Offer.OfferAmount
	} else {
		return "", fmt.Errorf("invalid chain for claim: %s", chainSymbol)
	}

	if htlcSession == nil {
		return "", fmt.Errorf("no HTLC session for chain %s", chainSymbol)
	}

	if fundingTxID == "" {
		return "", fmt.Errorf("no funding transaction recorded for claim")
	}

	// Get the secret - try session first, then fall back to swap record
	secret := htlcSession.GetSecret()
	if len(secret) != 32 {
		// Try the offer chain session (initiator stores secret there)
		if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
			secret = active.HTLC.OfferChain.Session.GetSecret()
		}
	}
	if len(secret) != 32 {
		// Fall back to swap record
		secret = active.Swap.Secret
	}
	if len(secret) != 32 {
		return "", fmt.Errorf("secret not available for claim")
	}

	// Get HTLC script
	htlcScript := htlcSession.GetHTLCScript()
	if len(htlcScript) == 0 {
		return "", fmt.Errorf("HTLC script not available")
	}

	// Get destination address
	if c.wallet == nil {
		return "", fmt.Errorf("wallet not available for deriving claim address")
	}
	destAddress, err := c.wallet.DeriveAddress(chainSymbol, 0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to derive claim address: %w", err)
	}

	// Get fee rate
	b := c.backends[chainSymbol]
	feeEstimate, err := b.GetFeeEstimates(ctx)
	var feeRate uint64 = 20
	if err == nil && feeEstimate != nil {
		feeRate = feeEstimate.HalfHourFee
		if feeRate == 0 {
			feeRate = 20
		}
	}

	// Get the private key for signing
	// For claiming, we use the remote pubkey's corresponding private key
	// which is actually our local key on the counterparty's chain
	privKey := htlcSession.GetLocalPrivKey()
	if privKey == nil {
		return "", fmt.Errorf("private key not available for claim")
	}

	// Calculate DAO fee - claimer pays the fee
	// Initiator claims on request chain, Responder claims on offer chain
	isMaker := active.Swap.Role == RoleInitiator
	daoFee := CalculateDAOFee(fundingAmount, isMaker)

	// Get DAO address from config
	exchangeCfg := config.NewExchangeConfig(config.NetworkType(c.network))
	daoAddress := exchangeCfg.GetDAOAddress(chainSymbol)

	c.log.Debug("Building HTLC claim tx",
		"chain", chainSymbol,
		"funding_amount", fundingAmount,
		"dao_fee", daoFee,
		"dao_address", daoAddress,
	)

	// Build claim transaction
	claimTx, err := BuildHTLCClaimTx(&HTLCClaimTxParams{
		Symbol:        chainSymbol,
		Network:       c.network,
		FundingTxID:   fundingTxID,
		FundingVout:   fundingVout,
		FundingAmount: fundingAmount,
		HTLCScript:    htlcScript,
		Secret:        secret,
		DestAddress:   destAddress,
		DAOAddress:    daoAddress,
		DAOFee:        daoFee,
		FeeRate:       feeRate,
		PrivKey:       privKey,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build claim transaction: %w", err)
	}

	// Serialize and broadcast
	txHex, err := SerializeTx(claimTx)
	if err != nil {
		return "", fmt.Errorf("failed to serialize claim transaction: %w", err)
	}

	txID, err := b.BroadcastTransaction(ctx, txHex)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast claim transaction: %w", err)
	}

	// Update swap state
	active.Swap.State = StateRedeemed
	c.emitEvent(tradeID, "htlc_claimed", map[string]string{
		"chain":    chainSymbol,
		"claim_tx": txID,
	})

	// Save state
	if c.store != nil {
		_ = c.saveSwapState(tradeID)
	}

	return txID, nil
}

// RefundHTLC refunds the HTLC output on the specified chain after the CSV timeout.
// Only the original sender can refund their own chain's output.
func (c *Coordinator) RefundHTLC(ctx context.Context, tradeID string, chainSymbol string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return "", ErrSwapNotFound
	}

	if active.Swap.Offer.Method != MethodHTLC {
		return "", fmt.Errorf("swap method is %s, not HTLC", active.Swap.Offer.Method)
	}

	if active.HTLC == nil {
		return "", fmt.Errorf("no HTLC data for swap")
	}

	// Get the appropriate HTLC session and funding info
	var htlcSession *HTLCSession
	var fundingTxID string
	var fundingVout uint32
	var fundingAmount uint64
	var timeoutBlocks uint32

	// Determine refund scenario:
	// - Initiator refunds their own offer chain output
	// - Responder refunds their own request chain output
	isMaker := active.Swap.Role == RoleInitiator

	if active.Swap.Role == RoleInitiator && chainSymbol == active.Swap.Offer.OfferChain {
		// Initiator refunding their offer
		htlcSession = active.HTLC.OfferChain.Session
		fundingTxID = active.Swap.LocalFundingTxID
		fundingVout = active.Swap.LocalFundingVout
		fundingAmount = active.Swap.Offer.OfferAmount
		timeoutBlocks = GetTimeoutBlocks(chainSymbol, isMaker)
	} else if active.Swap.Role == RoleResponder && chainSymbol == active.Swap.Offer.RequestChain {
		// Responder refunding their request
		htlcSession = active.HTLC.RequestChain.Session
		fundingTxID = active.Swap.LocalFundingTxID
		fundingVout = active.Swap.LocalFundingVout
		fundingAmount = active.Swap.Offer.RequestAmount
		timeoutBlocks = GetTimeoutBlocks(chainSymbol, !isMaker)
	} else {
		return "", fmt.Errorf("cannot refund: you are %s but trying to refund %s", active.Swap.Role, chainSymbol)
	}

	if htlcSession == nil {
		return "", fmt.Errorf("no HTLC session for chain %s", chainSymbol)
	}

	if fundingTxID == "" {
		return "", fmt.Errorf("no funding transaction recorded for refund")
	}

	// Get HTLC script
	htlcScript := htlcSession.GetHTLCScript()
	if len(htlcScript) == 0 {
		return "", fmt.Errorf("HTLC script not available")
	}

	// Get destination address
	if c.wallet == nil {
		return "", fmt.Errorf("wallet not available for deriving refund address")
	}
	destAddress, err := c.wallet.DeriveAddress(chainSymbol, 0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to derive refund address: %w", err)
	}

	// Get fee rate
	b := c.backends[chainSymbol]
	feeEstimate, err := b.GetFeeEstimates(ctx)
	var feeRate uint64 = 20
	if err == nil && feeEstimate != nil {
		feeRate = feeEstimate.HourFee // Use slower fee for refunds
		if feeRate == 0 {
			feeRate = 20
		}
	}

	// Get the private key (sender's key for refund)
	privKey := htlcSession.GetLocalPrivKey()
	if privKey == nil {
		return "", fmt.Errorf("private key not available for refund")
	}

	// Build refund transaction
	refundTx, err := BuildHTLCRefundTx(&HTLCRefundTxParams{
		Symbol:        chainSymbol,
		Network:       c.network,
		FundingTxID:   fundingTxID,
		FundingVout:   fundingVout,
		FundingAmount: fundingAmount,
		HTLCScript:    htlcScript,
		TimeoutBlocks: timeoutBlocks,
		DestAddress:   destAddress,
		FeeRate:       feeRate,
		PrivKey:       privKey,
	})
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
	c.emitEvent(tradeID, "htlc_refunded", map[string]string{
		"chain":     chainSymbol,
		"refund_tx": txID,
	})

	// Save state
	if c.store != nil {
		_ = c.saveSwapState(tradeID)
	}

	return txID, nil
}

// ExtractSecretFromTx extracts the secret from an HTLC claim transaction.
// This is used by the counterparty to learn the secret after the initiator claims.
func (c *Coordinator) ExtractSecretFromTx(ctx context.Context, tradeID string, txID string, chainSymbol string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	if active.HTLC == nil {
		return nil, fmt.Errorf("no HTLC data for swap")
	}

	// Get the transaction from the blockchain
	b, ok := c.backends[chainSymbol]
	if !ok {
		return nil, fmt.Errorf("no backend for chain %s", chainSymbol)
	}

	tx, err := b.GetTransaction(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Parse the transaction to extract witness data
	msgTx, err := DeserializeTx(tx.Hex)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}

	// The secret is in the witness of the input spending the HTLC
	// Witness structure for claim: [signature, secret, 0x01, script]
	for _, txIn := range msgTx.TxIn {
		if len(txIn.Witness) >= 2 {
			// The second element should be the secret (32 bytes)
			potentialSecret := txIn.Witness[1]
			if len(potentialSecret) == 32 {
				// Verify it matches the expected hash
				var htlcSession *HTLCSession
				if chainSymbol == active.Swap.Offer.OfferChain && active.HTLC.OfferChain != nil {
					htlcSession = active.HTLC.OfferChain.Session
				} else if chainSymbol == active.Swap.Offer.RequestChain && active.HTLC.RequestChain != nil {
					htlcSession = active.HTLC.RequestChain.Session
				}

				if htlcSession != nil && VerifySecret(potentialSecret, htlcSession.GetSecretHash()) {
					// Store the secret
					if err := htlcSession.SetSecret(potentialSecret); err == nil {
						// Also set in the other chain's session
						if chainSymbol == active.Swap.Offer.OfferChain && active.HTLC.RequestChain != nil {
							_ = active.HTLC.RequestChain.Session.SetSecret(potentialSecret)
						} else if chainSymbol == active.Swap.Offer.RequestChain && active.HTLC.OfferChain != nil {
							_ = active.HTLC.OfferChain.Session.SetSecret(potentialSecret)
						}

						// Save state
						if c.store != nil {
							_ = c.saveSwapState(tradeID)
						}

						return potentialSecret, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("secret not found in transaction witness")
}
