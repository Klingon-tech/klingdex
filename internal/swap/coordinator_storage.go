// Package swap - Storage and persistence functions for the Coordinator.
package swap

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
)

// =============================================================================
// Serialization for storage
// =============================================================================

// getChainStorageData extracts storage data from a ChainMuSig2Data.
func getChainStorageData(chainData *ChainMuSig2Data, chainSymbol string) *ChainStorageData {
	if chainData == nil {
		return nil
	}

	data := &ChainStorageData{
		Chain:          chainSymbol,
		TaprootAddress: chainData.TaprootAddress,
	}

	if chainData.LocalNonce != nil {
		data.LocalNonce = hex.EncodeToString(chainData.LocalNonce)
	}
	if chainData.RemoteNonce != nil {
		data.RemoteNonce = hex.EncodeToString(chainData.RemoteNonce)
	}
	if chainData.PartialSig != nil {
		sigBytes := chainData.PartialSig.S.Bytes()
		data.PartialSig = hex.EncodeToString(sigBytes[:])
	}
	if chainData.RemotePartialSig != nil {
		sigBytes := chainData.RemotePartialSig.S.Bytes()
		data.RemotePartialSig = hex.EncodeToString(sigBytes[:])
	}
	if chainData.Session != nil {
		if aggKey, err := chainData.Session.AggregatedPubKey(); err == nil && aggKey != nil {
			data.AggregatedPubKey = hex.EncodeToString(aggKey.SerializeCompressed())
		}
		data.NonceUsed = chainData.Session.IsNonceUsed()
		data.SessionInvalid = chainData.Session.IsInvalidated()
		data.UsedNonces = chainData.Session.GetUsedNoncesHex()
	}

	return data
}

// GetMuSig2StorageData returns the MuSig2 data formatted for storage.
func (c *Coordinator) GetMuSig2StorageData(tradeID string) (json.RawMessage, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active, ok := c.swaps[tradeID]
	if !ok {
		return nil, ErrSwapNotFound
	}

	data := MuSig2StorageData{
		LocalPubKey:  hex.EncodeToString(active.Swap.LocalPubKey),
		RemotePubKey: hex.EncodeToString(active.Swap.RemotePubKey),
		OfferChain:   getChainStorageData(active.MuSig2.OfferChain, active.Swap.Offer.OfferChain),
		RequestChain: getChainStorageData(active.MuSig2.RequestChain, active.Swap.Offer.RequestChain),
	}

	if active.MuSig2.LocalPrivKey != nil {
		data.LocalPrivKey = hex.EncodeToString(active.MuSig2.LocalPrivKey.Serialize())
	}

	return json.Marshal(data)
}

// =============================================================================
// Persistence Methods
// =============================================================================

// saveSwapState persists the current swap state to the database.
// This should be called after any state change to enable recovery after restart.
// NOTE: Caller must hold c.mu lock.
func (c *Coordinator) saveSwapState(tradeID string) error {
	if c.store == nil {
		return nil // No storage configured, skip persistence
	}

	active, ok := c.swaps[tradeID]
	if !ok {
		return ErrSwapNotFound
	}

	// Get method-specific storage data
	var methodData json.RawMessage
	var err error

	// Determine storage type based on what data is present
	if active.IsCrossChain() {
		// Cross-chain swap (Bitcoin <-> EVM)
		methodData, err = c.getCrossChainStorageDataUnlocked(active)
	} else if active.IsEVMHTLC() {
		// EVM-only swap
		methodData, err = c.getEVMHTLCStorageDataUnlocked(active)
	} else if active.IsMuSig2() {
		// MuSig2 Bitcoin swap
		methodData, err = c.getMuSig2StorageDataUnlocked(active)
	} else if active.IsHTLC() {
		// HTLC Bitcoin swap
		methodData, err = c.getHTLCStorageDataUnlocked(active)
	} else {
		return fmt.Errorf("unknown swap method: %s", active.Swap.Offer.Method)
	}
	if err != nil {
		return fmt.Errorf("failed to get method data: %w", err)
	}

	// Build swap record
	record := &storage.SwapRecord{
		TradeID:     tradeID,
		OrderID:     "",
		MakerPeerID: "",
		TakerPeerID: "",
		OurRole:     string(active.Swap.Role),
		IsMaker:     active.Swap.Role == RoleInitiator,

		OfferChain:    active.Swap.Offer.OfferChain,
		OfferAmount:   active.Swap.Offer.OfferAmount,
		RequestChain:  active.Swap.Offer.RequestChain,
		RequestAmount: active.Swap.Offer.RequestAmount,

		State:      swapStateToStorage(active.Swap.State),
		MethodData: methodData,

		LocalFundingTxID:  active.Swap.LocalFundingTxID,
		LocalFundingVout:  active.Swap.LocalFundingVout,
		RemoteFundingTxID: active.Swap.RemoteFundingTxID,
		RemoteFundingVout: active.Swap.RemoteFundingVout,

		TimeoutHeight:        active.Swap.OfferChainTimeoutHeight,
		RequestTimeoutHeight: active.Swap.RequestChainTimeoutHeight,
	}

	// Track order and peer IDs from trade if available
	if active.Trade != nil {
		record.OrderID = active.Trade.OrderID
		record.MakerPeerID = active.Trade.MakerPeerID
		record.TakerPeerID = active.Trade.TakerPeerID
	}

	return c.store.SaveSwap(record)
}

// getMuSig2StorageDataUnlocked gets MuSig2 storage data without locking.
func (c *Coordinator) getMuSig2StorageDataUnlocked(active *ActiveSwap) (json.RawMessage, error) {
	data := MuSig2StorageData{
		LocalPubKey:             hex.EncodeToString(active.Swap.LocalPubKey),
		RemotePubKey:            hex.EncodeToString(active.Swap.RemotePubKey),
		LocalOfferWalletAddr:    active.Swap.LocalOfferWalletAddr,
		LocalRequestWalletAddr:  active.Swap.LocalRequestWalletAddr,
		RemoteOfferWalletAddr:   active.Swap.RemoteOfferWalletAddr,
		RemoteRequestWalletAddr: active.Swap.RemoteRequestWalletAddr,
		OfferChain:              getChainStorageData(active.MuSig2.OfferChain, active.Swap.Offer.OfferChain),
		RequestChain:            getChainStorageData(active.MuSig2.RequestChain, active.Swap.Offer.RequestChain),
	}

	if active.MuSig2.LocalPrivKey != nil {
		data.LocalPrivKey = hex.EncodeToString(active.MuSig2.LocalPrivKey.Serialize())
	}

	return json.Marshal(data)
}

// getHTLCStorageDataUnlocked gets HTLC storage data without locking.
func (c *Coordinator) getHTLCStorageDataUnlocked(active *ActiveSwap) (json.RawMessage, error) {
	data := CoordinatorHTLCStorageData{
		LocalPubKey:             hex.EncodeToString(active.Swap.LocalPubKey),
		RemotePubKey:            hex.EncodeToString(active.Swap.RemotePubKey),
		LocalOfferWalletAddr:    active.Swap.LocalOfferWalletAddr,
		LocalRequestWalletAddr:  active.Swap.LocalRequestWalletAddr,
		RemoteOfferWalletAddr:   active.Swap.RemoteOfferWalletAddr,
		RemoteRequestWalletAddr: active.Swap.RemoteRequestWalletAddr,
	}

	// Add secret/secret hash
	if len(active.Swap.Secret) > 0 {
		data.Secret = hex.EncodeToString(active.Swap.Secret)
	}
	if len(active.Swap.SecretHash) > 0 {
		data.SecretHash = hex.EncodeToString(active.Swap.SecretHash)
	}

	// Add HTLC chain data
	if active.HTLC != nil {
		if active.HTLC.OfferChain != nil {
			data.OfferChain = &HTLCChainStorageData{
				Symbol:      active.Swap.Offer.OfferChain,
				HTLCAddress: active.HTLC.OfferChain.HTLCAddress,
			}
			if active.HTLC.OfferChain.Session != nil {
				sessionData, _ := active.HTLC.OfferChain.Session.MarshalStorageData()
				data.OfferChain.SessionData = string(sessionData)
			}
		}
		if active.HTLC.RequestChain != nil {
			data.RequestChain = &HTLCChainStorageData{
				Symbol:      active.Swap.Offer.RequestChain,
				HTLCAddress: active.HTLC.RequestChain.HTLCAddress,
			}
			if active.HTLC.RequestChain.Session != nil {
				sessionData, _ := active.HTLC.RequestChain.Session.MarshalStorageData()
				data.RequestChain.SessionData = string(sessionData)
			}
		}
	}

	return json.Marshal(data)
}

// getEVMHTLCStorageDataUnlocked gets EVM HTLC storage data without locking.
func (c *Coordinator) getEVMHTLCStorageDataUnlocked(active *ActiveSwap) (json.RawMessage, error) {
	data := CoordinatorEVMHTLCStorageData{
		LocalOfferWalletAddr:    active.Swap.LocalOfferWalletAddr,
		LocalRequestWalletAddr:  active.Swap.LocalRequestWalletAddr,
		RemoteOfferWalletAddr:   active.Swap.RemoteOfferWalletAddr,
		RemoteRequestWalletAddr: active.Swap.RemoteRequestWalletAddr,
	}

	// Add secret/secret hash
	if len(active.Swap.Secret) > 0 {
		data.Secret = hex.EncodeToString(active.Swap.Secret)
	}
	if len(active.Swap.SecretHash) > 0 {
		data.SecretHash = hex.EncodeToString(active.Swap.SecretHash)
	}

	// Add EVM HTLC chain data
	if active.EVMHTLC != nil {
		if active.EVMHTLC.OfferChain != nil {
			data.OfferChain = &EVMHTLCChainStorageData{
				Symbol:          active.Swap.Offer.OfferChain,
				ContractAddress: active.EVMHTLC.OfferChain.ContractAddress.Hex(),
				SwapID:          hex.EncodeToString(active.EVMHTLC.OfferChain.SwapID[:]),
				CreateTxHash:    active.EVMHTLC.OfferChain.CreateTxHash.Hex(),
				ClaimTxHash:     active.EVMHTLC.OfferChain.ClaimTxHash.Hex(),
				RefundTxHash:    active.EVMHTLC.OfferChain.RefundTxHash.Hex(),
			}
			if active.EVMHTLC.OfferChain.Session != nil {
				sessionData := active.EVMHTLC.OfferChain.Session.ToStorageData()
				data.OfferChain.ChainID = sessionData.ChainID
				jsonData, _ := json.Marshal(sessionData)
				data.OfferChain.SessionData = string(jsonData)
			}
		}
		if active.EVMHTLC.RequestChain != nil {
			data.RequestChain = &EVMHTLCChainStorageData{
				Symbol:          active.Swap.Offer.RequestChain,
				ContractAddress: active.EVMHTLC.RequestChain.ContractAddress.Hex(),
				SwapID:          hex.EncodeToString(active.EVMHTLC.RequestChain.SwapID[:]),
				CreateTxHash:    active.EVMHTLC.RequestChain.CreateTxHash.Hex(),
				ClaimTxHash:     active.EVMHTLC.RequestChain.ClaimTxHash.Hex(),
				RefundTxHash:    active.EVMHTLC.RequestChain.RefundTxHash.Hex(),
			}
			if active.EVMHTLC.RequestChain.Session != nil {
				sessionData := active.EVMHTLC.RequestChain.Session.ToStorageData()
				data.RequestChain.ChainID = sessionData.ChainID
				jsonData, _ := json.Marshal(sessionData)
				data.RequestChain.SessionData = string(jsonData)
			}
		}
	}

	return json.Marshal(data)
}

// getCrossChainStorageDataUnlocked gets combined HTLC + EVM HTLC storage data for cross-chain swaps.
func (c *Coordinator) getCrossChainStorageDataUnlocked(active *ActiveSwap) (json.RawMessage, error) {
	// For cross-chain swaps, we combine both HTLC and EVM HTLC data
	type CrossChainStorageData struct {
		// Wallet addresses
		LocalOfferWalletAddr    string `json:"local_offer_wallet_addr,omitempty"`
		LocalRequestWalletAddr  string `json:"local_request_wallet_addr,omitempty"`
		RemoteOfferWalletAddr   string `json:"remote_offer_wallet_addr,omitempty"`
		RemoteRequestWalletAddr string `json:"remote_request_wallet_addr,omitempty"`

		// Secret
		Secret     string `json:"secret,omitempty"`
		SecretHash string `json:"secret_hash,omitempty"`

		// Bitcoin HTLC data (if any)
		BitcoinHTLC *CoordinatorHTLCStorageData `json:"bitcoin_htlc,omitempty"`

		// EVM HTLC data (if any)
		EVMHTLC *CoordinatorEVMHTLCStorageData `json:"evm_htlc,omitempty"`
	}

	data := CrossChainStorageData{
		LocalOfferWalletAddr:    active.Swap.LocalOfferWalletAddr,
		LocalRequestWalletAddr:  active.Swap.LocalRequestWalletAddr,
		RemoteOfferWalletAddr:   active.Swap.RemoteOfferWalletAddr,
		RemoteRequestWalletAddr: active.Swap.RemoteRequestWalletAddr,
	}

	// Add secret/secret hash
	if len(active.Swap.Secret) > 0 {
		data.Secret = hex.EncodeToString(active.Swap.Secret)
	}
	if len(active.Swap.SecretHash) > 0 {
		data.SecretHash = hex.EncodeToString(active.Swap.SecretHash)
	}

	// Get Bitcoin HTLC data if present
	if active.HTLC != nil {
		htlcData, err := c.getHTLCStorageDataUnlocked(active)
		if err == nil {
			var htlc CoordinatorHTLCStorageData
			if json.Unmarshal(htlcData, &htlc) == nil {
				data.BitcoinHTLC = &htlc
			}
		}
	}

	// Get EVM HTLC data if present
	if active.EVMHTLC != nil {
		evmData, err := c.getEVMHTLCStorageDataUnlocked(active)
		if err == nil {
			var evm CoordinatorEVMHTLCStorageData
			if json.Unmarshal(evmData, &evm) == nil {
				data.EVMHTLC = &evm
			}
		}
	}

	return json.Marshal(data)
}

// LoadPendingSwaps loads all pending swaps from the database on startup.
// This enables recovery after a node restart.
func (c *Coordinator) LoadPendingSwaps(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.store == nil {
		return nil // No storage configured
	}

	records, err := c.store.GetPendingSwaps()
	if err != nil {
		return fmt.Errorf("failed to get pending swaps: %w", err)
	}

	var recoveryErrors []error
	for _, record := range records {
		if err := c.recoverSwapFromRecord(ctx, record); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("swap %s: %w", record.TradeID, err))
		}
	}

	if len(recoveryErrors) > 0 {
		return fmt.Errorf("failed to recover %d swaps: %v", len(recoveryErrors), recoveryErrors)
	}

	return nil
}

// recoverSwapFromRecord reconstructs an ActiveSwap from a database record.
// NOTE: Caller must hold c.mu lock.
func (c *Coordinator) recoverSwapFromRecord(ctx context.Context, record *storage.SwapRecord) error {
	// Check if swap already exists in memory
	if _, exists := c.swaps[record.TradeID]; exists {
		return nil // Already loaded
	}

	// Detect swap type by attempting to parse different storage formats
	swapType := c.detectSwapTypeFromMethodData(record.MethodData, record.OfferChain, record.RequestChain)

	switch swapType {
	case "evm_htlc":
		return c.recoverEVMHTLCSwap(ctx, record)
	case "bitcoin_htlc":
		return c.recoverBitcoinHTLCSwap(ctx, record)
	case "cross_chain":
		return c.recoverCrossChainSwap(ctx, record)
	default:
		// Default to MuSig2 for backward compatibility
		return c.recoverMuSig2Swap(ctx, record)
	}
}

// detectSwapTypeFromMethodData determines the swap type from stored method data.
func (c *Coordinator) detectSwapTypeFromMethodData(methodData json.RawMessage, offerChain, requestChain string) string {
	// Check if chains are EVM
	offerIsEVM := IsEVMChain(offerChain, c.network)
	requestIsEVM := IsEVMChain(requestChain, c.network)

	// Try to detect based on field presence in the JSON
	var probe struct {
		// EVM HTLC specific
		OfferChainEVM   *EVMHTLCChainStorageData `json:"offer_chain,omitempty"`
		RequestChainEVM *EVMHTLCChainStorageData `json:"request_chain,omitempty"`
		// Bitcoin HTLC specific
		OfferChainBTC   *HTLCChainStorageData `json:"offer_chain,omitempty"`
		RequestChainBTC *HTLCChainStorageData `json:"request_chain,omitempty"`
		// Cross-chain specific
		BitcoinHTLC *CoordinatorHTLCStorageData    `json:"bitcoin_htlc,omitempty"`
		EVMHTLC     *CoordinatorEVMHTLCStorageData `json:"evm_htlc,omitempty"`
		// HTLC specific
		SecretHash string `json:"secret_hash,omitempty"`
		// MuSig2 specific
		LocalPrivKey string `json:"local_priv_key,omitempty"`
	}
	_ = json.Unmarshal(methodData, &probe)

	// Cross-chain swap (has both Bitcoin and EVM data)
	if probe.BitcoinHTLC != nil || probe.EVMHTLC != nil {
		return "cross_chain"
	}

	// If both chains are EVM and we have secret hash, it's EVM HTLC
	if offerIsEVM && requestIsEVM && probe.SecretHash != "" {
		return "evm_htlc"
	}

	// If neither chain is EVM and we have secret hash, it's Bitcoin HTLC
	if !offerIsEVM && !requestIsEVM && probe.SecretHash != "" {
		return "bitcoin_htlc"
	}

	// Default to MuSig2
	return "musig2"
}

// recoverMuSig2Swap recovers a MuSig2 swap from storage.
func (c *Coordinator) recoverMuSig2Swap(ctx context.Context, record *storage.SwapRecord) error {
	// Parse MuSig2 storage data
	var methodData MuSig2StorageData
	if err := json.Unmarshal(record.MethodData, &methodData); err != nil {
		return fmt.Errorf("failed to unmarshal method data: %w", err)
	}

	// Reconstruct offer
	offer := Offer{
		OfferChain:    record.OfferChain,
		OfferAmount:   record.OfferAmount,
		RequestChain:  record.RequestChain,
		RequestAmount: record.RequestAmount,
		Method:        MethodMuSig2,
	}

	// Determine role
	role := Role(record.OurRole)
	if role != RoleInitiator && role != RoleResponder {
		if record.IsMaker {
			role = RoleInitiator
		} else {
			role = RoleResponder
		}
	}

	// Reconstruct swap
	swap, err := NewSwap(c.network, MethodMuSig2, role, offer)
	if err != nil {
		return fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = record.TradeID
	swap.State = storageStateToSwap(record.State)
	swap.LocalFundingTxID = record.LocalFundingTxID
	swap.LocalFundingVout = record.LocalFundingVout
	swap.RemoteFundingTxID = record.RemoteFundingTxID
	swap.RemoteFundingVout = record.RemoteFundingVout
	swap.OfferChainTimeoutHeight = record.TimeoutHeight
	swap.RequestChainTimeoutHeight = record.RequestTimeoutHeight

	// Parse public keys
	if methodData.LocalPubKey != "" {
		localPubBytes, err := hex.DecodeString(methodData.LocalPubKey)
		if err == nil {
			swap.LocalPubKey = localPubBytes
		}
	}

	// Parse remote public key
	var remotePub *btcec.PublicKey
	if methodData.RemotePubKey != "" {
		remotePubBytes, err := hex.DecodeString(methodData.RemotePubKey)
		if err == nil {
			remotePub, err = btcec.ParsePubKey(remotePubBytes)
			if err == nil {
				_ = swap.SetRemotePubKey(remotePub)
			}
		}
	}

	// Restore wallet addresses
	swap.LocalOfferWalletAddr = methodData.LocalOfferWalletAddr
	swap.LocalRequestWalletAddr = methodData.LocalRequestWalletAddr
	swap.RemoteOfferWalletAddr = methodData.RemoteOfferWalletAddr
	swap.RemoteRequestWalletAddr = methodData.RemoteRequestWalletAddr

	// Reconstruct private key
	var privKey *btcec.PrivateKey
	if methodData.LocalPrivKey != "" {
		privKeyBytes, err := hex.DecodeString(methodData.LocalPrivKey)
		if err == nil && len(privKeyBytes) == 32 {
			privKey, _ = btcec.PrivKeyFromBytes(privKeyBytes)
		}
	}

	if privKey == nil {
		return fmt.Errorf("cannot recover swap: private key not available")
	}

	// Create sessions for BOTH chains
	offerSession, err := NewMuSig2Session(record.OfferChain, c.network, privKey)
	if err != nil {
		return fmt.Errorf("failed to create offer chain session: %w", err)
	}
	requestSession, err := NewMuSig2Session(record.RequestChain, c.network, privKey)
	if err != nil {
		return fmt.Errorf("failed to create request chain session: %w", err)
	}

	// Restore remote public key to both sessions
	if remotePub != nil {
		_ = offerSession.SetRemotePubKey(remotePub)
		_ = requestSession.SetRemotePubKey(remotePub)
	}

	// Restore offer chain data
	offerChainData := &ChainMuSig2Data{Session: offerSession}
	if methodData.OfferChain != nil {
		offerChainData.TaprootAddress = methodData.OfferChain.TaprootAddress
		if methodData.OfferChain.LocalNonce != "" {
			offerChainData.LocalNonce, _ = hex.DecodeString(methodData.OfferChain.LocalNonce)
		}
		if methodData.OfferChain.RemoteNonce != "" {
			offerChainData.RemoteNonce, _ = hex.DecodeString(methodData.OfferChain.RemoteNonce)
		}
		if methodData.OfferChain.PartialSig != "" {
			offerChainData.PartialSig = parsePartialSig(methodData.OfferChain.PartialSig)
		}
		if methodData.OfferChain.RemotePartialSig != "" {
			offerChainData.RemotePartialSig = parsePartialSig(methodData.OfferChain.RemotePartialSig)
		}
		// Restore session state
		if methodData.OfferChain.SessionInvalid {
			offerSession.SetInvalidated(true)
		}
		if methodData.OfferChain.NonceUsed {
			offerSession.SetNonceUsed(true)
		}
		if len(methodData.OfferChain.UsedNonces) > 0 {
			_ = offerSession.SetUsedNonces(methodData.OfferChain.UsedNonces)
		}
	}

	// Restore request chain data
	requestChainData := &ChainMuSig2Data{Session: requestSession}
	if methodData.RequestChain != nil {
		requestChainData.TaprootAddress = methodData.RequestChain.TaprootAddress
		if methodData.RequestChain.LocalNonce != "" {
			requestChainData.LocalNonce, _ = hex.DecodeString(methodData.RequestChain.LocalNonce)
		}
		if methodData.RequestChain.RemoteNonce != "" {
			requestChainData.RemoteNonce, _ = hex.DecodeString(methodData.RequestChain.RemoteNonce)
		}
		if methodData.RequestChain.PartialSig != "" {
			requestChainData.PartialSig = parsePartialSig(methodData.RequestChain.PartialSig)
		}
		if methodData.RequestChain.RemotePartialSig != "" {
			requestChainData.RemotePartialSig = parsePartialSig(methodData.RequestChain.RemotePartialSig)
		}
		// Restore session state
		if methodData.RequestChain.SessionInvalid {
			requestSession.SetInvalidated(true)
		}
		if methodData.RequestChain.NonceUsed {
			requestSession.SetNonceUsed(true)
		}
		if len(methodData.RequestChain.UsedNonces) > 0 {
			_ = requestSession.SetUsedNonces(methodData.RequestChain.UsedNonces)
		}
	}

	// Create active swap with per-chain data
	active := &ActiveSwap{
		Swap: swap,
		MuSig2: &MuSig2SwapData{
			LocalPrivKey: privKey,
			OfferChain:   offerChainData,
			RequestChain: requestChainData,
		},
	}

	c.swaps[record.TradeID] = active

	c.emitEvent(record.TradeID, "swap_recovered", map[string]interface{}{
		"state":                  string(record.State),
		"offer_timeout_height":   record.TimeoutHeight,
		"request_timeout_height": record.RequestTimeoutHeight,
	})

	return nil
}

// recoverEVMHTLCSwap recovers an EVM-to-EVM HTLC swap from storage.
func (c *Coordinator) recoverEVMHTLCSwap(ctx context.Context, record *storage.SwapRecord) error {
	var methodData CoordinatorEVMHTLCStorageData
	if err := json.Unmarshal(record.MethodData, &methodData); err != nil {
		return fmt.Errorf("failed to unmarshal EVM HTLC data: %w", err)
	}

	// Reconstruct offer
	offer := Offer{
		OfferChain:    record.OfferChain,
		OfferAmount:   record.OfferAmount,
		RequestChain:  record.RequestChain,
		RequestAmount: record.RequestAmount,
		Method:        MethodHTLC,
	}

	// Determine role
	role := Role(record.OurRole)
	if role != RoleInitiator && role != RoleResponder {
		if record.IsMaker {
			role = RoleInitiator
		} else {
			role = RoleResponder
		}
	}

	// Create swap
	swap, err := NewSwap(c.network, MethodHTLC, role, offer)
	if err != nil {
		return fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = record.TradeID
	swap.State = storageStateToSwap(record.State)

	// Restore wallet addresses
	swap.LocalOfferWalletAddr = methodData.LocalOfferWalletAddr
	swap.LocalRequestWalletAddr = methodData.LocalRequestWalletAddr
	swap.RemoteOfferWalletAddr = methodData.RemoteOfferWalletAddr
	swap.RemoteRequestWalletAddr = methodData.RemoteRequestWalletAddr

	// Restore secret/hash
	if methodData.Secret != "" {
		swap.Secret, _ = hex.DecodeString(methodData.Secret)
	}
	if methodData.SecretHash != "" {
		swap.SecretHash, _ = hex.DecodeString(methodData.SecretHash)
	}

	// Create active swap - sessions will be recreated when needed via getOrCreateEVMSession
	active := &ActiveSwap{
		Swap:    swap,
		EVMHTLC: &EVMHTLCSwapData{},
	}

	c.swaps[record.TradeID] = active

	c.log.Info("Recovered EVM HTLC swap", "trade_id", record.TradeID, "state", record.State)
	c.emitEvent(record.TradeID, "swap_recovered", map[string]interface{}{
		"type":  "evm_htlc",
		"state": string(record.State),
	})

	return nil
}

// recoverBitcoinHTLCSwap recovers a Bitcoin-family HTLC swap from storage.
func (c *Coordinator) recoverBitcoinHTLCSwap(ctx context.Context, record *storage.SwapRecord) error {
	var methodData CoordinatorHTLCStorageData
	if err := json.Unmarshal(record.MethodData, &methodData); err != nil {
		return fmt.Errorf("failed to unmarshal HTLC data: %w", err)
	}

	// Reconstruct offer
	offer := Offer{
		OfferChain:    record.OfferChain,
		OfferAmount:   record.OfferAmount,
		RequestChain:  record.RequestChain,
		RequestAmount: record.RequestAmount,
		Method:        MethodHTLC,
	}

	// Determine role
	role := Role(record.OurRole)
	if role != RoleInitiator && role != RoleResponder {
		if record.IsMaker {
			role = RoleInitiator
		} else {
			role = RoleResponder
		}
	}

	// Create swap
	swap, err := NewSwap(c.network, MethodHTLC, role, offer)
	if err != nil {
		return fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = record.TradeID
	swap.State = storageStateToSwap(record.State)
	swap.LocalFundingTxID = record.LocalFundingTxID
	swap.LocalFundingVout = record.LocalFundingVout
	swap.RemoteFundingTxID = record.RemoteFundingTxID
	swap.RemoteFundingVout = record.RemoteFundingVout
	swap.OfferChainTimeoutHeight = record.TimeoutHeight
	swap.RequestChainTimeoutHeight = record.RequestTimeoutHeight

	// Restore wallet addresses
	swap.LocalOfferWalletAddr = methodData.LocalOfferWalletAddr
	swap.LocalRequestWalletAddr = methodData.LocalRequestWalletAddr
	swap.RemoteOfferWalletAddr = methodData.RemoteOfferWalletAddr
	swap.RemoteRequestWalletAddr = methodData.RemoteRequestWalletAddr

	// Restore public keys
	if methodData.LocalPubKey != "" {
		swap.LocalPubKey, _ = hex.DecodeString(methodData.LocalPubKey)
	}
	if methodData.RemotePubKey != "" {
		remotePubBytes, err := hex.DecodeString(methodData.RemotePubKey)
		if err == nil {
			remotePub, err := btcec.ParsePubKey(remotePubBytes)
			if err == nil {
				_ = swap.SetRemotePubKey(remotePub)
			}
		}
	}

	// Restore secret/hash
	if methodData.Secret != "" {
		swap.Secret, _ = hex.DecodeString(methodData.Secret)
	}
	if methodData.SecretHash != "" {
		swap.SecretHash, _ = hex.DecodeString(methodData.SecretHash)
	}

	// Create active swap - sessions will be recreated when needed
	active := &ActiveSwap{
		Swap: swap,
		HTLC: &HTLCSwapData{},
	}

	c.swaps[record.TradeID] = active

	c.log.Info("Recovered Bitcoin HTLC swap", "trade_id", record.TradeID, "state", record.State)
	c.emitEvent(record.TradeID, "swap_recovered", map[string]interface{}{
		"type":  "bitcoin_htlc",
		"state": string(record.State),
	})

	return nil
}

// recoverCrossChainSwap recovers a cross-chain (Bitcoin <-> EVM) swap from storage.
func (c *Coordinator) recoverCrossChainSwap(ctx context.Context, record *storage.SwapRecord) error {
	// Parse cross-chain storage data
	type CrossChainStorageData struct {
		LocalOfferWalletAddr    string                         `json:"local_offer_wallet_addr,omitempty"`
		LocalRequestWalletAddr  string                         `json:"local_request_wallet_addr,omitempty"`
		RemoteOfferWalletAddr   string                         `json:"remote_offer_wallet_addr,omitempty"`
		RemoteRequestWalletAddr string                         `json:"remote_request_wallet_addr,omitempty"`
		Secret                  string                         `json:"secret,omitempty"`
		SecretHash              string                         `json:"secret_hash,omitempty"`
		BitcoinHTLC             *CoordinatorHTLCStorageData    `json:"bitcoin_htlc,omitempty"`
		EVMHTLC                 *CoordinatorEVMHTLCStorageData `json:"evm_htlc,omitempty"`
	}

	var methodData CrossChainStorageData
	if err := json.Unmarshal(record.MethodData, &methodData); err != nil {
		return fmt.Errorf("failed to unmarshal cross-chain data: %w", err)
	}

	// Reconstruct offer
	offer := Offer{
		OfferChain:    record.OfferChain,
		OfferAmount:   record.OfferAmount,
		RequestChain:  record.RequestChain,
		RequestAmount: record.RequestAmount,
		Method:        MethodHTLC,
	}

	// Determine role
	role := Role(record.OurRole)
	if role != RoleInitiator && role != RoleResponder {
		if record.IsMaker {
			role = RoleInitiator
		} else {
			role = RoleResponder
		}
	}

	// Create swap
	swap, err := NewSwap(c.network, MethodHTLC, role, offer)
	if err != nil {
		return fmt.Errorf("failed to create swap: %w", err)
	}
	swap.ID = record.TradeID
	swap.State = storageStateToSwap(record.State)
	swap.LocalFundingTxID = record.LocalFundingTxID
	swap.LocalFundingVout = record.LocalFundingVout
	swap.RemoteFundingTxID = record.RemoteFundingTxID
	swap.RemoteFundingVout = record.RemoteFundingVout
	swap.OfferChainTimeoutHeight = record.TimeoutHeight
	swap.RequestChainTimeoutHeight = record.RequestTimeoutHeight

	// Restore wallet addresses
	swap.LocalOfferWalletAddr = methodData.LocalOfferWalletAddr
	swap.LocalRequestWalletAddr = methodData.LocalRequestWalletAddr
	swap.RemoteOfferWalletAddr = methodData.RemoteOfferWalletAddr
	swap.RemoteRequestWalletAddr = methodData.RemoteRequestWalletAddr

	// Restore secret/hash
	if methodData.Secret != "" {
		swap.Secret, _ = hex.DecodeString(methodData.Secret)
	}
	if methodData.SecretHash != "" {
		swap.SecretHash, _ = hex.DecodeString(methodData.SecretHash)
	}

	// Create active swap - sessions will be recreated when needed
	active := &ActiveSwap{
		Swap:    swap,
		HTLC:    &HTLCSwapData{},
		EVMHTLC: &EVMHTLCSwapData{},
	}

	c.swaps[record.TradeID] = active

	c.log.Info("Recovered cross-chain swap", "trade_id", record.TradeID, "state", record.State)
	c.emitEvent(record.TradeID, "swap_recovered", map[string]interface{}{
		"type":  "cross_chain",
		"state": string(record.State),
	})

	return nil
}

// RecoverSwap loads and recovers a single swap from the database.
func (c *Coordinator) RecoverSwap(ctx context.Context, tradeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.store == nil {
		return errors.New("no storage configured")
	}

	record, err := c.store.GetSwap(tradeID)
	if err != nil {
		return fmt.Errorf("failed to get swap: %w", err)
	}

	return c.recoverSwapFromRecord(ctx, record)
}

// ListSwaps returns info about all swaps (both memory and database).
func (c *Coordinator) ListSwaps(includeCompleted bool) ([]*storage.SwapRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.store == nil {
		// Return in-memory swaps as records
		var records []*storage.SwapRecord
		for tradeID, active := range c.swaps {
			record := &storage.SwapRecord{
				TradeID:       tradeID,
				OurRole:       string(active.Swap.Role),
				IsMaker:       active.Swap.Role == RoleInitiator,
				OfferChain:    active.Swap.Offer.OfferChain,
				OfferAmount:   active.Swap.Offer.OfferAmount,
				RequestChain:  active.Swap.Offer.RequestChain,
				RequestAmount: active.Swap.Offer.RequestAmount,
				State:         swapStateToStorage(active.Swap.State),
			}
			records = append(records, record)
		}
		return records, nil
	}

	return c.store.ListSwaps(100, includeCompleted)
}

// =============================================================================
// State Conversion Helpers
// =============================================================================

func swapStateToStorage(s State) storage.SwapState {
	switch s {
	case StateInit:
		return storage.SwapStateInit
	case StateFunding:
		return storage.SwapStateFunding
	case StateFunded:
		return storage.SwapStateFunded
	case StateRedeemed:
		return storage.SwapStateRedeemed
	case StateRefunded:
		return storage.SwapStateRefunded
	case StateFailed:
		return storage.SwapStateFailed
	case StateCancelled:
		return storage.SwapStateCancelled
	default:
		return storage.SwapStateInit
	}
}

func storageStateToSwap(s storage.SwapState) State {
	switch s {
	case storage.SwapStateInit:
		return StateInit
	case storage.SwapStateFunding:
		return StateFunding
	case storage.SwapStateFunded:
		return StateFunded
	case storage.SwapStateSigning:
		// Map signing to funded (closest state since signing doesn't exist in swap)
		return StateFunded
	case storage.SwapStateRedeemed:
		return StateRedeemed
	case storage.SwapStateRefunded:
		return StateRefunded
	case storage.SwapStateFailed:
		return StateFailed
	case storage.SwapStateCancelled:
		return StateCancelled
	default:
		return StateInit
	}
}
