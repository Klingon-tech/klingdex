// Package rpc - EVM HTLC swap handlers.
package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/klingon-exchange/klingon-v2/internal/config"
	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
)

// =============================================================================
// Cross-Chain Swap Init
// =============================================================================

// swapInitCrossChain initializes a cross-chain swap (EVM ↔ EVM or EVM ↔ Bitcoin).
func (s *Server) swapInitCrossChain(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapInitCrossChainParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Role == "" {
		return nil, fmt.Errorf("role is required (initiator or responder)")
	}
	if p.Role != "initiator" && p.Role != "responder" {
		return nil, fmt.Errorf("role must be 'initiator' or 'responder'")
	}

	// Get trade from storage
	trade, err := s.store.GetTrade(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("trade not found: %w", err)
	}

	// Get order for additional details
	order, err := s.store.GetOrder(trade.OrderID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	// Build offer from trade/order
	offer := swap.Offer{
		OfferChain:    order.OfferChain,
		OfferAmount:   order.OfferAmount,
		RequestChain:  order.RequestChain,
		RequestAmount: order.RequestAmount,
	}

	var activeSwap *swap.ActiveSwap
	var secretHash []byte

	if p.Role == "initiator" {
		// Initiator creates the swap and generates the secret
		activeSwap, err = s.coordinator.InitiateCrossChainSwap(ctx, p.TradeID, trade.OrderID, offer)
		if err != nil {
			return nil, fmt.Errorf("failed to initiate cross-chain swap: %w", err)
		}
		secretHash = activeSwap.Swap.SecretHash

		// Send secret hash + wallet addresses to responder via P2P
		if err := s.sendHTLCSecretHashToCounterparty(ctx, p.TradeID, activeSwap); err != nil {
			s.log.Warn("Failed to send secret hash to counterparty", "error", err)
			// Don't fail - they may receive it via other means or can request it
		} else {
			s.log.Info("Sent HTLC secret hash to counterparty",
				"trade_id", p.TradeID,
				"secret_hash", hex.EncodeToString(secretHash)[:16],
			)
		}
	} else {
		// Responder needs secret hash from initiator
		// Check if we have a stored secret for this trade (received via P2P)
		storedSecret, err := s.store.GetSecretByTradeID(p.TradeID)
		if err != nil || storedSecret.SecretHash == "" {
			return nil, fmt.Errorf("responder needs secret_hash from initiator - not yet received via P2P")
		}

		secretHashBytes, err := hex.DecodeString(storedSecret.SecretHash)
		if err != nil {
			return nil, fmt.Errorf("invalid stored secret hash: %w", err)
		}

		// Get remote EVM addresses from stored secret (set by handleHTLCSecretHash)
		// These are the initiator's addresses where they want to receive funds
		remoteOfferAddr := storedSecret.RemoteOfferWalletAddr
		remoteRequestAddr := storedSecret.RemoteRequestWalletAddr

		s.log.Info("Retrieved remote wallet addresses from stored secret",
			"trade_id", p.TradeID,
			"remote_offer_addr", remoteOfferAddr,
			"remote_request_addr", remoteRequestAddr,
		)

		// Get maker's pubkey from trade record (set by handleHTLCSecretHash)
		// This is needed for cross-chain swaps involving Bitcoin
		var remotePubKey []byte
		if trade.MakerPubKey != "" {
			remotePubKey, err = hex.DecodeString(trade.MakerPubKey)
			if err != nil {
				return nil, fmt.Errorf("invalid maker pubkey: %w", err)
			}
			s.log.Info("Retrieved maker pubkey from trade",
				"trade_id", p.TradeID,
				"pubkey", trade.MakerPubKey[:16]+"...",
			)
		}

		activeSwap, err = s.coordinator.RespondToCrossChainSwap(ctx, p.TradeID, offer, remotePubKey, secretHashBytes, remoteOfferAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to respond to cross-chain swap: %w", err)
		}

		// Set remote addresses in the coordinator for both chains
		if remoteOfferAddr != "" || remoteRequestAddr != "" {
			if err := s.coordinator.SetRemoteWalletAddresses(p.TradeID, remoteOfferAddr, remoteRequestAddr); err != nil {
				s.log.Warn("Failed to set remote wallet addresses", "error", err)
			}
		}

		// Send our wallet addresses back to the initiator
		if err := s.sendWalletAddressesToCounterparty(ctx, p.TradeID, activeSwap); err != nil {
			s.log.Warn("Failed to send wallet addresses to initiator", "error", err)
			// Don't fail - they may receive it via other means
		} else {
			s.log.Info("Sent wallet addresses to initiator",
				"trade_id", p.TradeID,
				"local_offer_addr", activeSwap.Swap.LocalOfferWalletAddr,
				"local_request_addr", activeSwap.Swap.LocalRequestWalletAddr,
			)
		}

		secretHash = secretHashBytes
	}

	// Get swap type
	swapType, _ := s.coordinator.GetSwapType(p.TradeID)
	swapTypeStr := "unknown"
	switch swapType {
	case swap.CrossChainTypeEVMToEVM:
		swapTypeStr = "evm_to_evm"
	case swap.CrossChainTypeBitcoinToEVM:
		swapTypeStr = "bitcoin_to_evm"
	case swap.CrossChainTypeEVMToBitcoin:
		swapTypeStr = "evm_to_bitcoin"
	}

	// Get local EVM address from the active swap
	localEVMAddr := ""
	if activeSwap != nil && activeSwap.Swap != nil {
		// Use the appropriate wallet address based on role
		if p.Role == "initiator" {
			localEVMAddr = activeSwap.Swap.LocalOfferWalletAddr
		} else {
			localEVMAddr = activeSwap.Swap.LocalRequestWalletAddr
		}
	}

	s.log.Info("Cross-chain swap initialized",
		"trade_id", p.TradeID,
		"role", p.Role,
		"swap_type", swapTypeStr,
		"offer_chain", offer.OfferChain,
		"request_chain", offer.RequestChain,
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("cross_chain_swap_initialized", map[string]string{
			"trade_id":  p.TradeID,
			"role":      p.Role,
			"swap_type": swapTypeStr,
		})
	}

	return &SwapInitCrossChainResult{
		TradeID:      p.TradeID,
		Role:         p.Role,
		SwapType:     swapTypeStr,
		OfferChain:   offer.OfferChain,
		RequestChain: offer.RequestChain,
		SecretHash:   hex.EncodeToString(secretHash),
		LocalEVMAddr: localEVMAddr,
		State:        "initialized",
		Message:      fmt.Sprintf("Cross-chain %s swap initialized as %s", swapTypeStr, p.Role),
	}, nil
}

// swapGetSwapType returns the swap type for a given trade.
func (s *Server) swapGetSwapType(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapGetSwapTypeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}

	swapType, err := s.coordinator.GetSwapType(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap type: %w", err)
	}

	// Get the active swap for additional details
	activeSwap, err := s.coordinator.GetSwap(p.TradeID)
	if err != nil {
		return nil, fmt.Errorf("swap not found: %w", err)
	}

	swapTypeStr := "unknown"
	switch swapType {
	case swap.CrossChainTypeEVMToEVM:
		swapTypeStr = "evm_to_evm"
	case swap.CrossChainTypeBitcoinToEVM:
		swapTypeStr = "bitcoin_to_evm"
	case swap.CrossChainTypeEVMToBitcoin:
		swapTypeStr = "evm_to_bitcoin"
	case swap.CrossChainTypeBitcoinToBitcoin:
		swapTypeStr = "bitcoin_to_bitcoin"
	}

	methodStr := "htlc"
	if activeSwap.IsMuSig2() {
		methodStr = "musig2"
	}

	return &SwapGetSwapTypeResult{
		TradeID:      p.TradeID,
		SwapType:     swapTypeStr,
		OfferChain:   activeSwap.Swap.Offer.OfferChain,
		RequestChain: activeSwap.Swap.Offer.RequestChain,
		Method:       methodStr,
	}, nil
}

// =============================================================================
// EVM HTLC Create
// =============================================================================

// swapEVMCreate creates an EVM HTLC swap on the specified chain.
func (s *Server) swapEVMCreate(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	txHash, err := s.coordinator.CreateEVMHTLC(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to create EVM HTLC: %w", err)
	}

	s.log.Info("EVM HTLC created",
		"trade_id", p.TradeID,
		"chain", p.Chain,
		"tx_hash", txHash.Hex(),
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("evm_htlc_created", map[string]string{
			"trade_id": p.TradeID,
			"chain":    p.Chain,
			"tx_hash":  txHash.Hex(),
		})
	}

	return &SwapEVMCreateResult{
		TradeID:  p.TradeID,
		Chain:    p.Chain,
		TxHash:   txHash.Hex(),
		State:    "created",
		Message:  "EVM HTLC created successfully",
	}, nil
}

// =============================================================================
// EVM HTLC Claim
// =============================================================================

// swapEVMClaim claims an EVM HTLC using the secret.
func (s *Server) swapEVMClaim(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMClaimParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	txHash, err := s.coordinator.ClaimEVMHTLC(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to claim EVM HTLC: %w", err)
	}

	s.log.Info("EVM HTLC claimed",
		"trade_id", p.TradeID,
		"chain", p.Chain,
		"tx_hash", txHash.Hex(),
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("evm_htlc_claimed", map[string]string{
			"trade_id": p.TradeID,
			"chain":    p.Chain,
			"tx_hash":  txHash.Hex(),
		})
	}

	return &SwapEVMClaimResult{
		TradeID:     p.TradeID,
		Chain:       p.Chain,
		ClaimTxHash: txHash.Hex(),
		State:       "claimed",
	}, nil
}

// =============================================================================
// EVM HTLC Refund
// =============================================================================

// swapEVMRefund refunds an EVM HTLC after the timelock expires.
func (s *Server) swapEVMRefund(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMRefundParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	txHash, err := s.coordinator.RefundEVMHTLC(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to refund EVM HTLC: %w", err)
	}

	s.log.Info("EVM HTLC refunded",
		"trade_id", p.TradeID,
		"chain", p.Chain,
		"tx_hash", txHash.Hex(),
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("evm_htlc_refunded", map[string]string{
			"trade_id": p.TradeID,
			"chain":    p.Chain,
			"tx_hash":  txHash.Hex(),
		})
	}

	return &SwapEVMRefundResult{
		TradeID:      p.TradeID,
		Chain:        p.Chain,
		RefundTxHash: txHash.Hex(),
		State:        "refunded",
	}, nil
}

// =============================================================================
// EVM HTLC Status
// =============================================================================

// swapEVMStatus gets the status of an EVM HTLC on the specified chain.
func (s *Server) swapEVMStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	swap, err := s.coordinator.GetEVMHTLCStatus(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM HTLC status: %w", err)
	}

	// Convert state to string
	stateStr := "unknown"
	switch swap.State {
	case 0:
		stateStr = "invalid"
	case 1:
		stateStr = "active"
	case 2:
		stateStr = "claimed"
	case 3:
		stateStr = "refunded"
	}

	return &SwapEVMStatusResult{
		TradeID:      p.TradeID,
		Chain:        p.Chain,
		State:        stateStr,
		Initiator:    swap.Sender.Hex(),
		Receiver:     swap.Receiver.Hex(),
		TokenAddress: swap.Token.Hex(),
		Amount:       swap.Amount.String(),
		SecretHash:   hex.EncodeToString(swap.SecretHash[:]),
		Timelock:     swap.Timelock.Int64(),
		IsNative:     swap.IsNativeToken(),
	}, nil
}

// =============================================================================
// EVM Wait for Secret
// =============================================================================

// swapEVMWaitSecret waits for the secret to be revealed on an EVM chain.
func (s *Server) swapEVMWaitSecret(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMWaitSecretParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("chain is required")
	}

	secret, err := s.coordinator.WaitForEVMSecret(ctx, p.TradeID, p.Chain)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for secret: %w", err)
	}

	s.log.Info("Secret revealed on EVM chain",
		"trade_id", p.TradeID,
		"chain", p.Chain,
	)

	return &SwapEVMWaitSecretResult{
		TradeID: p.TradeID,
		Chain:   p.Chain,
		Secret:  hex.EncodeToString(secret[:]),
		Message: "Secret revealed",
	}, nil
}

// =============================================================================
// EVM Set Secret
// =============================================================================

// swapEVMSetSecret sets the secret for an EVM HTLC swap.
// This is used when the counterparty reveals the secret on another chain
// and we need to use it to claim on our chain.
func (s *Server) swapEVMSetSecret(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMSetSecretParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.TradeID == "" {
		return nil, fmt.Errorf("trade_id is required")
	}
	if p.Secret == "" {
		return nil, fmt.Errorf("secret is required")
	}

	// Decode hex secret
	secretBytes, err := hex.DecodeString(p.Secret)
	if err != nil {
		return nil, fmt.Errorf("invalid secret hex: %w", err)
	}
	if len(secretBytes) != 32 {
		return nil, fmt.Errorf("secret must be 32 bytes")
	}

	var secret [32]byte
	copy(secret[:], secretBytes)

	// Set the secret on the swap
	if err := s.coordinator.SetEVMSecret(p.TradeID, secret); err != nil {
		return nil, fmt.Errorf("failed to set secret: %w", err)
	}

	s.log.Info("Set secret for EVM swap",
		"trade_id", p.TradeID,
	)

	return &SwapEVMSetSecretResult{
		TradeID: p.TradeID,
		Message: "Secret set successfully",
	}, nil
}

// =============================================================================
// EVM Contract Info
// =============================================================================

// swapEVMGetContracts returns the deployed HTLC contract addresses.
func (s *Server) swapEVMGetContracts(ctx context.Context, params json.RawMessage) (interface{}, error) {
	deployedChains := config.ListDeployedHTLCChains()

	contracts := make([]EVMContractInfo, 0, len(deployedChains))
	for _, chainID := range deployedChains {
		addr := config.GetHTLCContract(chainID)
		contracts = append(contracts, EVMContractInfo{
			ChainID:         chainID,
			ContractAddress: addr.Hex(),
		})
	}

	return &SwapEVMGetContractsResult{
		Contracts: contracts,
		Count:     len(contracts),
	}, nil
}

// swapEVMGetContract returns the HTLC contract address for a specific chain.
func (s *Server) swapEVMGetContract(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMGetContractParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.ChainID == 0 {
		return nil, fmt.Errorf("chain_id is required")
	}

	if !config.IsHTLCDeployed(p.ChainID) {
		return nil, fmt.Errorf("HTLC contract not deployed on chain %d", p.ChainID)
	}

	addr := config.GetHTLCContract(p.ChainID)

	return &SwapEVMGetContractResult{
		ChainID:         p.ChainID,
		ContractAddress: addr.Hex(),
		Deployed:        true,
	}, nil
}

// =============================================================================
// EVM Compute Swap ID
// =============================================================================

// swapEVMComputeSwapID computes the swap ID for an EVM HTLC.
func (s *Server) swapEVMComputeSwapID(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p SwapEVMComputeSwapIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Initiator == "" {
		return nil, fmt.Errorf("initiator is required")
	}
	if p.Receiver == "" {
		return nil, fmt.Errorf("receiver is required")
	}
	if p.SecretHash == "" {
		return nil, fmt.Errorf("secret_hash is required")
	}
	if p.Timelock == 0 {
		return nil, fmt.Errorf("timelock is required")
	}

	initiator := common.HexToAddress(p.Initiator)
	receiver := common.HexToAddress(p.Receiver)
	token := common.HexToAddress(p.TokenAddress)

	secretHashBytes, err := hex.DecodeString(p.SecretHash)
	if err != nil {
		return nil, fmt.Errorf("invalid secret_hash: %w", err)
	}
	if len(secretHashBytes) != 32 {
		return nil, fmt.Errorf("secret_hash must be 32 bytes")
	}
	var secretHash [32]byte
	copy(secretHash[:], secretHashBytes)

	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount")
	}

	timelock := big.NewInt(p.Timelock)

	// Compute swap ID using Keccak256
	// swapId = keccak256(abi.encodePacked(initiator, receiver, tokenAddress, amount, secretHash, timelock))
	data := make([]byte, 0, 20+20+20+32+32+32)
	data = append(data, initiator.Bytes()...)
	data = append(data, receiver.Bytes()...)
	data = append(data, token.Bytes()...)
	data = append(data, common.LeftPadBytes(amount.Bytes(), 32)...)
	data = append(data, secretHash[:]...)
	data = append(data, common.LeftPadBytes(timelock.Bytes(), 32)...)

	swapID := common.BytesToHash(common.FromHex(common.Bytes2Hex(data)))

	return &SwapEVMComputeSwapIDResult{
		SwapID: swapID.Hex(),
	}, nil
}

// =============================================================================
// P2P Messaging Helpers for Cross-Chain Swaps
// =============================================================================

// sendHTLCSecretHashToCounterparty sends the secret hash and wallet addresses to the counterparty.
// This is called by the initiator after creating the swap.
func (s *Server) sendHTLCSecretHashToCounterparty(ctx context.Context, tradeID string, activeSwap *swap.ActiveSwap) error {
	if activeSwap == nil || activeSwap.Swap == nil {
		return fmt.Errorf("no active swap")
	}

	// Get local public key (for Bitcoin-family cross-chain swaps)
	// LocalPubKey is stored as []byte (compressed format)
	pubKeyHex := ""
	if len(activeSwap.Swap.LocalPubKey) > 0 {
		pubKeyHex = hex.EncodeToString(activeSwap.Swap.LocalPubKey)
	}

	// Create the message
	msg, err := node.NewHTLCSecretHashMessage(
		tradeID,
		hex.EncodeToString(activeSwap.Swap.SecretHash),
		pubKeyHex,
		activeSwap.Swap.LocalOfferWalletAddr,
		activeSwap.Swap.LocalRequestWalletAddr,
	)
	if err != nil {
		return fmt.Errorf("failed to create secret hash message: %w", err)
	}

	// Send via direct P2P messaging to counterparty
	return s.sendDirectToCounterparty(ctx, tradeID, msg)
}

// sendWalletAddressesToCounterparty sends wallet addresses to the counterparty.
// This is called by the responder after initializing their swap to send their addresses back.
func (s *Server) sendWalletAddressesToCounterparty(ctx context.Context, tradeID string, activeSwap *swap.ActiveSwap) error {
	if activeSwap == nil || activeSwap.Swap == nil {
		return fmt.Errorf("no active swap")
	}

	// Get local public key (for Bitcoin-family cross-chain swaps)
	pubKeyHex := ""
	if len(activeSwap.Swap.LocalPubKey) > 0 {
		pubKeyHex = hex.EncodeToString(activeSwap.Swap.LocalPubKey)
	}

	// Create the pubkey exchange message with wallet addresses
	msg, err := node.NewPubKeyExchangeMessage(
		tradeID,
		pubKeyHex,
		activeSwap.Swap.LocalOfferWalletAddr,
		activeSwap.Swap.LocalRequestWalletAddr,
	)
	if err != nil {
		return fmt.Errorf("failed to create pubkey exchange message: %w", err)
	}

	// Send via direct P2P messaging to counterparty
	return s.sendDirectToCounterparty(ctx, tradeID, msg)
}
