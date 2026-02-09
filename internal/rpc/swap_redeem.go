// Package rpc - Swap redemption handlers.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/Klingon-tech/klingdex/internal/config"
	"github.com/Klingon-tech/klingdex/internal/storage"
	"github.com/Klingon-tech/klingdex/internal/swap"
)

// swapRedeem redeems funds from the counterparty's chain.
func (s *Server) swapRedeem(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapRedeemParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	// Determine which chain we're redeeming from based on our role
	var redeemChain string
	var redeemChainData *swap.ChainMuSig2Data
	var redeemAmount uint64
	var fundingTxID string
	var fundingVout uint32

	if activeSwap.Swap.Role == swap.RoleInitiator {
		// Initiator redeems from request chain (responder's funds)
		redeemChain = activeSwap.Swap.Offer.RequestChain
		redeemChainData = activeSwap.MuSig2.RequestChain
		redeemAmount = activeSwap.Swap.Offer.RequestAmount
		fundingTxID = activeSwap.Swap.RemoteFundingTxID
		fundingVout = activeSwap.Swap.RemoteFundingVout
	} else {
		// Responder redeems from offer chain (initiator's funds)
		redeemChain = activeSwap.Swap.Offer.OfferChain
		redeemChainData = activeSwap.MuSig2.OfferChain
		redeemAmount = activeSwap.Swap.Offer.OfferAmount
		fundingTxID = activeSwap.Swap.RemoteFundingTxID
		fundingVout = activeSwap.Swap.RemoteFundingVout
	}

	// Check we have signatures for the redemption chain
	if redeemChainData == nil {
		return nil, fmt.Errorf("MuSig2 data for %s chain not initialized", redeemChain)
	}
	if redeemChainData.PartialSig == nil {
		return nil, fmt.Errorf("local partial signature for %s not created - call swap_sign first", redeemChain)
	}
	if redeemChainData.RemotePartialSig == nil {
		return nil, fmt.Errorf("remote partial signature for %s not received - wait for counterparty to sign", redeemChain)
	}
	if fundingTxID == "" {
		return nil, fmt.Errorf("remote funding transaction not set")
	}

	// Get remote partial signature bytes
	sigArr := redeemChainData.RemotePartialSig.S.Bytes()
	remoteSigBytes := sigArr[:]

	// Combine signatures for the specific chain
	finalSig, err := s.coordinator.CombineSignatures(p.TradeID, redeemChain, remoteSigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to combine signatures: %w", err)
	}

	// Build the complete spending transaction
	destAddr, err := s.getDestinationAddressForChain(activeSwap, redeemChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination address: %w", err)
	}

	// Calculate DAO fee
	var daoFee uint64
	if redeemChain == activeSwap.Swap.Offer.OfferChain {
		daoFee = swap.CalculateDAOFee(redeemAmount, false) // taker
	} else {
		daoFee = swap.CalculateDAOFee(redeemAmount, true) // maker
	}

	// Get DAO address from config
	exchangeCfg := config.NewExchangeConfig(config.NetworkType(s.coordinator.Network()))
	daoAddress := exchangeCfg.GetDAOAddress(redeemChain)

	// Get dynamic fee rate for redemption chain
	feeRate := s.getFeeRateForChain(ctx, redeemChain)

	s.log.Info("swap_redeem: dest", "chain", redeemChain, "dest", destAddr, "role", activeSwap.Swap.Role, "fundingTxID", fundingTxID, "daoFee", daoFee, "daoAddress", daoAddress, "feeRate", feeRate)

	spendParams := &swap.SpendingTxParams{
		Symbol:         redeemChain,
		Network:        s.coordinator.Network(),
		FundingTxID:    fundingTxID,
		FundingVout:    fundingVout,
		FundingAmount:  redeemAmount,
		TaprootAddress: redeemChainData.TaprootAddress,
		DestAddress:    destAddr,
		DAOAddress:     daoAddress,
		DAOFee:         daoFee,
		FeeRate:        feeRate,
	}

	redeemTx, _, err := swap.BuildSpendingTx(spendParams)
	if err != nil {
		return nil, fmt.Errorf("failed to build redeem tx: %w", err)
	}

	// Parse the final signature and add witness
	finalSchnorrSig, err := parseSchnorrSignature(finalSig)
	if err != nil {
		return nil, fmt.Errorf("invalid final signature: %w", err)
	}
	swap.AddWitness(redeemTx, 0, finalSchnorrSig)

	// Serialize the transaction
	redeemTxHex, err := swap.SerializeTx(redeemTx)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize redeem tx: %w", err)
	}

	// Broadcast the transaction
	b, ok := s.coordinator.GetBackend(redeemChain)
	if !ok {
		return nil, fmt.Errorf("backend not available for chain: %s", redeemChain)
	}

	redeemTxID, err := b.BroadcastTransaction(ctx, redeemTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast redeem tx: %w", err)
	}

	// Mark swap as complete
	if err := s.coordinator.CompleteSwap(p.TradeID, redeemTxID); err != nil {
		s.log.Warn("Failed to complete swap", "error", err)
	}

	// Update trade state
	if err := s.store.UpdateTradeState(p.TradeID, storage.TradeStateRedeemed); err != nil {
		s.log.Warn("Failed to update trade state", "error", err)
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("swap_redeemed", map[string]string{
			"trade_id":     p.TradeID,
			"redeem_txid":  redeemTxID,
			"redeem_chain": redeemChain,
		})
	}

	s.log.Info("Swap redeemed successfully",
		"trade_id", p.TradeID[:8],
		"redeem_txid", redeemTxID,
		"chain", redeemChain,
	)

	return &SwapRedeemResult{
		TradeID:     p.TradeID,
		RedeemTxID:  redeemTxID,
		RedeemChain: redeemChain,
		State:       "redeemed",
		Message:     "Swap redeemed successfully",
	}, nil
}

// parseSchnorrSignature parses a 64-byte Schnorr signature.
func parseSchnorrSignature(sigBytes []byte) (*schnorr.Signature, error) {
	if len(sigBytes) != 64 {
		return nil, fmt.Errorf("invalid signature length: expected 64, got %d", len(sigBytes))
	}
	return schnorr.ParseSignature(sigBytes)
}
