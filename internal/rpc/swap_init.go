// Package rpc - Swap initialization handler.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// swapInit initializes a swap for a trade.
// This generates an ephemeral key pair and prepares for MuSig2 signing.
func (s *Server) swapInit(ctx context.Context, params json.RawMessage) (interface{}, error) {
	s.log.Info("swap_init called")
	var p SwapInitParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	s.log.Info("swap_init params parsed", "trade_id", p.TradeID)

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	// Get trade to determine our role
	trade, err := s.store.GetTrade(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("trade not found: %w", err)
	}

	// Get the order for offer details
	order, err := s.store.GetOrder(trade.OrderID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	// Check if swap already exists in coordinator
	if activeSwap, _ := s.coordinator.GetSwap(p.TradeID); activeSwap != nil {
		// Return existing pubkey
		pubKey, _ := s.coordinator.GetLocalPubKey(p.TradeID)
		// Use method-agnostic GetSwapAddresses (P2TR for MuSig2, P2WSH for HTLC)
		offerAddr, requestAddr, _ := s.coordinator.GetSwapAddresses(p.TradeID)
		// Return the address for the chain we're funding based on role
		var addr string
		if activeSwap.Swap.Role == swap.RoleInitiator {
			addr = offerAddr
		} else {
			addr = requestAddr
		}
		return &SwapInitResult{
			TradeID:        p.TradeID,
			LocalPubKey:    hex.EncodeToString(pubKey),
			TaprootAddress: addr,
			State:          string(activeSwap.Swap.State),
		}, nil
	}

	// Create offer struct for coordinator
	offer := swap.Offer{
		OfferChain:    order.OfferChain,
		OfferAmount:   order.OfferAmount,
		RequestChain:  order.RequestChain,
		RequestAmount: order.RequestAmount,
		Method:        swap.Method(trade.Method),
	}

	// Determine if we're maker or taker
	isMaker := trade.MakerPeerID == s.node.ID().String()
	s.log.Info("swap_init calling coordinator", "isMaker", isMaker, "trade_id", p.TradeID)

	// Determine swap method (default to MuSig2 if not specified)
	method := offer.Method
	if method == "" {
		method = swap.MethodMuSig2
	}

	var activeSwap *swap.ActiveSwap
	if isMaker {
		// Maker initiates
		s.log.Info("swap_init: calling InitiateSwap", "method", method)
		activeSwap, err = s.coordinator.InitiateSwap(ctx, p.TradeID, trade.OrderID, offer, method)
	} else {
		// Taker responds - check if we have maker's pubkey from P2P
		var remotePubKey []byte
		if trade.MakerPubKey != "" {
			remotePubKey, err = hex.DecodeString(trade.MakerPubKey)
			if err != nil {
				return nil, fmt.Errorf("invalid maker pubkey in trade: %w", err)
			}
			s.log.Info("swap_init: using maker pubkey from trade", "pubkey", trade.MakerPubKey[:16])
		} else {
			// Wait for maker's pubkey - return error asking to retry
			return nil, fmt.Errorf("maker pubkey not yet received - the maker needs to call swap_init first, then retry")
		}

		// For HTLC, try to get the secret hash from storage
		var secretHash []byte
		if method == swap.MethodHTLC {
			secrets, err := s.store.ListSecretsByTrade(p.TradeID)
			if err == nil && len(secrets) > 0 {
				for _, secret := range secrets {
					if secret.CreatedBy == storage.SecretCreatorThem && secret.SecretHash != "" {
						secretHash, _ = hex.DecodeString(secret.SecretHash)
						s.log.Info("swap_init: using secret hash from storage", "hash", secret.SecretHash[:16])
						break
					}
				}
			}
		}

		s.log.Info("swap_init: calling RespondToSwap", "method", method)
		activeSwap, err = s.coordinator.RespondToSwap(ctx, p.TradeID, offer, remotePubKey, secretHash, method)
	}

	s.log.Info("swap_init coordinator returned", "err", err)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize swap: %w", err)
	}

	// Associate trade with the active swap for persistence
	activeSwap.Trade = trade

	// Get our public key
	pubKey, err := s.coordinator.GetLocalPubKey(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get local pubkey: %w", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)

	// Store our pubkey in the trade record
	if err := s.store.UpdateTradePubKey(p.TradeID, isMaker, pubKeyHex); err != nil {
		s.log.Warn("Failed to store local pubkey in trade", "error", err)
	}

	// Get our wallet addresses for both chains (using proper index management)
	var offerWalletAddr, requestWalletAddr string
	if s.wallet != nil {
		var err error
		offerWalletAddr, _, err = s.getNextWalletAddress(activeSwap.Swap.Offer.OfferChain)
		if err != nil {
			s.log.Warn("Failed to get offer wallet address", "error", err)
		}
		requestWalletAddr, _, err = s.getNextWalletAddress(activeSwap.Swap.Offer.RequestChain)
		if err != nil {
			s.log.Warn("Failed to get request wallet address", "error", err)
		}
		// Store our own wallet addresses in the swap
		activeSwap.Swap.LocalOfferWalletAddr = offerWalletAddr
		activeSwap.Swap.LocalRequestWalletAddr = requestWalletAddr
	}

	// Send our public key and wallet addresses to the counterparty (direct P2P)
	// For MuSig2: send pubkey exchange
	if activeSwap.IsMuSig2() {
		payload := &node.PubKeyExchangePayload{
			PubKey:            pubKeyHex,
			OfferWalletAddr:   offerWalletAddr,
			RequestWalletAddr: requestWalletAddr,
		}
		msg, err := node.NewSwapMessage(node.SwapMsgPubKeyExchange, p.TradeID, payload)
		if err == nil {
			if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
				s.log.Warn("Failed to send pubkey", "trade_id", p.TradeID, "error", err)
			} else {
				s.log.Info("Sent pubkey to counterparty", "trade_id", p.TradeID[:8])
			}
		}
	}

	// For HTLC: initiator sends secret hash + pubkey, responder sends pubkey
	if activeSwap.IsHTLC() {
		if activeSwap.Swap.Role == swap.RoleInitiator && len(activeSwap.Swap.SecretHash) > 0 {
			// Initiator sends secret hash and pubkey
			secretHashHex := hex.EncodeToString(activeSwap.Swap.SecretHash)
			msg, err := node.NewHTLCSecretHashMessage(p.TradeID, secretHashHex, pubKeyHex, offerWalletAddr, requestWalletAddr)
			if err == nil {
				if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
					s.log.Warn("Failed to send secret hash", "trade_id", p.TradeID, "error", err)
				} else {
					s.log.Info("Sent HTLC secret hash to counterparty", "trade_id", p.TradeID[:8])
				}
			}
		} else {
			// Responder sends pubkey
			payload := &node.PubKeyExchangePayload{
				PubKey:            pubKeyHex,
				OfferWalletAddr:   offerWalletAddr,
				RequestWalletAddr: requestWalletAddr,
			}
			msg, err := node.NewSwapMessage(node.SwapMsgPubKeyExchange, p.TradeID, payload)
			if err == nil {
				if err := s.sendDirectToCounterparty(ctx, p.TradeID, msg); err != nil {
					s.log.Warn("Failed to send pubkey", "trade_id", p.TradeID, "error", err)
				} else {
					s.log.Info("Sent pubkey to counterparty", "trade_id", p.TradeID[:8])
				}
			}
		}
	}

	// Update trade state to accepted
	if err := s.store.UpdateTradeState(p.TradeID, storage.TradeStateAccepted); err != nil {
		s.log.Warn("Failed to update trade state", "error", err)
	}

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("swap_initialized", map[string]string{
			"trade_id":     p.TradeID,
			"local_pubkey": pubKeyHex,
		})
	}

	return &SwapInitResult{
		TradeID:     p.TradeID,
		LocalPubKey: pubKeyHex,
		State:       string(activeSwap.Swap.State),
	}, nil
}
