package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/Klingon-tech/klingdex/internal/chain"
)

// Note: wallet_scanBalance and wallet_getAddressWithChange handlers are registered in server.go

// ========================================
// Wallet handlers
// ========================================

// WalletStatusResult is the response for wallet_status.
type WalletStatusResult struct {
	HasWallet bool   `json:"has_wallet"`
	Unlocked  bool   `json:"unlocked"`
	Network   string `json:"network"`
}

func (s *Server) walletStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	return &WalletStatusResult{
		HasWallet: s.wallet.HasWallet(),
		Unlocked:  s.wallet.IsUnlocked(),
		Network:   string(s.wallet.Network()),
	}, nil
}

// WalletGenerateResult is the response for wallet_generate.
type WalletGenerateResult struct {
	Mnemonic string `json:"mnemonic"`
}

func (s *Server) walletGenerate(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	mnemonic, err := s.wallet.GenerateMnemonic()
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return &WalletGenerateResult{
		Mnemonic: mnemonic,
	}, nil
}

// WalletCreateParams is the parameters for wallet_create.
type WalletCreateParams struct {
	Mnemonic   string `json:"mnemonic"`
	Passphrase string `json:"passphrase"` // BIP39 passphrase (optional)
	Password   string `json:"password"`   // Encryption password (required)
}

func (s *Server) walletCreate(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Mnemonic == "" {
		return nil, fmt.Errorf("mnemonic is required")
	}
	if p.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	if err := s.wallet.CreateWallet(p.Mnemonic, p.Passphrase, p.Password); err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Set wallet on coordinator for swap operations (refunds, etc.)
	if s.coordinator != nil {
		if w := s.wallet.GetWallet(); w != nil {
			s.coordinator.SetWallet(w)
		}
	}

	return map[string]interface{}{
		"success": true,
		"message": "Wallet created successfully",
	}, nil
}

// WalletUnlockParams is the parameters for wallet_unlock.
type WalletUnlockParams struct {
	Password   string `json:"password"`
	Passphrase string `json:"passphrase"` // BIP39 passphrase (optional)
}

func (s *Server) walletUnlock(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletUnlockParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	if err := s.wallet.LoadWallet(p.Password, p.Passphrase); err != nil {
		return nil, fmt.Errorf("failed to unlock wallet: %w", err)
	}

	// Set wallet on coordinator for swap operations (refunds, etc.)
	if s.coordinator != nil {
		if w := s.wallet.GetWallet(); w != nil {
			s.coordinator.SetWallet(w)
		}
	}

	return map[string]interface{}{
		"success": true,
		"message": "Wallet unlocked successfully",
	}, nil
}

func (s *Server) walletLock(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	s.wallet.Lock()

	// Clear wallet from coordinator when locked
	if s.coordinator != nil {
		s.coordinator.SetWallet(nil)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Wallet locked successfully",
	}, nil
}

// WalletGetAddressParams is the parameters for wallet_getAddress.
type WalletGetAddressParams struct {
	Symbol  string `json:"symbol"`            // Chain symbol (BTC, ETH, LTC, etc.)
	Account uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	Index   uint32 `json:"index,omitempty"`   // Address index (default 0)
	Type    string `json:"type,omitempty"`    // Address type: p2pkh, p2wpkh, p2tr (default: chain default)
}

// WalletGetAddressResult is the response for wallet_getAddress.
type WalletGetAddressResult struct {
	Address string `json:"address"`
	Path    string `json:"path"`
	Symbol  string `json:"symbol"`
	Type    string `json:"type,omitempty"`
}

func (s *Server) walletGetAddress(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletGetAddressParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	var address string
	var err error

	if p.Type != "" {
		address, err = s.wallet.GetAddressWithType(p.Symbol, p.Account, p.Index, chain.AddressType(p.Type))
	} else {
		address, err = s.wallet.GetAddress(p.Symbol, p.Account, p.Index)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	path, _ := s.wallet.GetDerivationPath(p.Symbol, p.Account, p.Index)

	return &WalletGetAddressResult{
		Address: address,
		Path:    path,
		Symbol:  p.Symbol,
		Type:    p.Type,
	}, nil
}

// WalletGetAllAddressesParams is the parameters for wallet_getAllAddresses.
type WalletGetAllAddressesParams struct {
	Symbol  string `json:"symbol"`
	Account uint32 `json:"account,omitempty"`
	Index   uint32 `json:"index,omitempty"`
}

// WalletGetAllAddressesResult is the response for wallet_getAllAddresses.
type WalletGetAllAddressesResult struct {
	Addresses map[string]string `json:"addresses"`
	Path      string            `json:"path"`
	Symbol    string            `json:"symbol"`
}

func (s *Server) walletGetAllAddresses(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletGetAllAddressesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	addresses, err := s.wallet.GetAllAddresses(p.Symbol, p.Account, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses: %w", err)
	}

	// Convert map keys to strings
	addrMap := make(map[string]string)
	for k, v := range addresses {
		addrMap[string(k)] = v
	}

	path, _ := s.wallet.GetDerivationPath(p.Symbol, p.Account, p.Index)

	return &WalletGetAllAddressesResult{
		Addresses: addrMap,
		Path:      path,
		Symbol:    p.Symbol,
	}, nil
}

// WalletGetPublicKeyParams is the parameters for wallet_getPublicKey.
type WalletGetPublicKeyParams struct {
	Symbol  string `json:"symbol"`
	Account uint32 `json:"account,omitempty"`
	Index   uint32 `json:"index,omitempty"`
}

// WalletGetPublicKeyResult is the response for wallet_getPublicKey.
type WalletGetPublicKeyResult struct {
	PublicKey   string `json:"public_key"`    // Compressed public key (hex)
	Uncompressed string `json:"uncompressed"` // Uncompressed public key (hex)
	Path        string `json:"path"`
	Symbol      string `json:"symbol"`
}

func (s *Server) walletGetPublicKey(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletGetPublicKeyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	pubKey, err := s.wallet.GetPublicKey(p.Symbol, p.Account, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	path, _ := s.wallet.GetDerivationPath(p.Symbol, p.Account, p.Index)

	return &WalletGetPublicKeyResult{
		PublicKey:    hex.EncodeToString(pubKey.SerializeCompressed()),
		Uncompressed: hex.EncodeToString(pubKey.SerializeUncompressed()),
		Path:         path,
		Symbol:       p.Symbol,
	}, nil
}

// WalletSupportedChainsResult is the response for wallet_supportedChains.
type WalletSupportedChainsResult struct {
	Chains []ChainInfo `json:"chains"`
}

// ChainInfo represents information about a supported chain.
type ChainInfo struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Decimals uint8  `json:"decimals"`
}

func (s *Server) walletSupportedChains(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	symbols := s.wallet.SupportedChains()
	chains := make([]ChainInfo, 0, len(symbols))

	for _, symbol := range symbols {
		chainParams, ok := chain.Get(symbol, s.wallet.Network())
		if !ok {
			continue
		}

		chains = append(chains, ChainInfo{
			Symbol:   chainParams.Symbol,
			Name:     chainParams.Name,
			Type:     string(chainParams.Type),
			Decimals: chainParams.Decimals,
		})
	}

	return &WalletSupportedChainsResult{
		Chains: chains,
	}, nil
}

// WalletValidateMnemonicParams is the parameters for wallet_validateMnemonic.
type WalletValidateMnemonicParams struct {
	Mnemonic string `json:"mnemonic"`
}

func (s *Server) walletValidateMnemonic(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletValidateMnemonicParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	valid := s.wallet.ValidateMnemonic(p.Mnemonic)

	return map[string]interface{}{
		"valid": valid,
	}, nil
}

// WalletGetBalanceParams is the parameters for wallet_getBalance.
type WalletGetBalanceParams struct {
	Symbol  string `json:"symbol"`
	Address string `json:"address"`
}

// WalletGetBalanceResult is the response for wallet_getBalance.
type WalletGetBalanceResult struct {
	Balance uint64 `json:"balance"`
	Symbol  string `json:"symbol"`
	Address string `json:"address"`
}

func (s *Server) walletGetBalance(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletGetBalanceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	balance, err := s.wallet.GetBalance(ctx, p.Symbol, p.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &WalletGetBalanceResult{
		Balance: balance,
		Symbol:  p.Symbol,
		Address: p.Address,
	}, nil
}

// WalletGetFeeEstimatesParams is the parameters for wallet_getFeeEstimates.
type WalletGetFeeEstimatesParams struct {
	Symbol string `json:"symbol"`
}

func (s *Server) walletGetFeeEstimates(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletGetFeeEstimatesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	estimates, err := s.wallet.GetFeeEstimates(ctx, p.Symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee estimates: %w", err)
	}

	return estimates, nil
}

// WalletSendParams is the parameters for wallet_send.
type WalletSendParams struct {
	Symbol  string `json:"symbol"`            // Chain symbol (BTC, LTC, etc.)
	To      string `json:"to"`                // Destination address
	Amount  uint64 `json:"amount"`            // Amount in smallest units (satoshis, etc.)
	Account uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	Change  uint32 `json:"change,omitempty"`  // 0=external, 1=change (default 0)
	Index   uint32 `json:"index,omitempty"`   // Address index (default 0)
}

// WalletSendResult is the response for wallet_send.
type WalletSendResult struct {
	TxID   string `json:"txid"`
	Symbol string `json:"symbol"`
	To     string `json:"to"`
	Amount uint64 `json:"amount"`
}

func (s *Server) walletSend(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.To == "" {
		return nil, fmt.Errorf("to address is required")
	}
	if p.Amount == 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}

	// Use SendTransactionFromPath to support change addresses (change=0 or change=1)
	txid, err := s.wallet.SendTransactionFromPath(ctx, p.Symbol, p.To, p.Amount, p.Account, p.Change, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	return &WalletSendResult{
		TxID:   txid,
		Symbol: p.Symbol,
		To:     p.To,
		Amount: p.Amount,
	}, nil
}

// WalletGetUTXOsParams is the parameters for wallet_getUTXOs.
type WalletGetUTXOsParams struct {
	Symbol  string `json:"symbol"`
	Address string `json:"address"`
}

func (s *Server) walletGetUTXOs(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletGetUTXOsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	utxos, err := s.wallet.GetUTXOs(ctx, p.Symbol, p.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	return map[string]interface{}{
		"utxos":   utxos,
		"count":   len(utxos),
		"symbol":  p.Symbol,
		"address": p.Address,
	}, nil
}

// WalletScanBalanceParams is the parameters for wallet_scanBalance.
type WalletScanBalanceParams struct {
	Symbol   string `json:"symbol"`            // Chain symbol (BTC, LTC, etc.)
	Account  uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	GapLimit uint32 `json:"gap_limit,omitempty"` // Gap limit for scanning (default 20)
}

func (s *Server) walletScanBalance(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletScanBalanceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	result, err := s.wallet.ScanBalance(ctx, p.Symbol, p.Account, p.GapLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to scan balance: %w", err)
	}

	return result, nil
}

// WalletGetAddressWithChangeParams is the parameters for wallet_getAddressWithChange.
type WalletGetAddressWithChangeParams struct {
	Symbol  string `json:"symbol"`            // Chain symbol (BTC, LTC, etc.)
	Account uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	Change  uint32 `json:"change"`            // 0=external, 1=change
	Index   uint32 `json:"index,omitempty"`   // Address index (default 0)
}

// WalletGetAddressWithChangeResult is the response for wallet_getAddressWithChange.
type WalletGetAddressWithChangeResult struct {
	Address  string `json:"address"`
	Path     string `json:"path"`
	Symbol   string `json:"symbol"`
	IsChange bool   `json:"is_change"`
}

func (s *Server) walletGetAddressWithChange(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletGetAddressWithChangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	address, err := s.wallet.GetAddressWithChange(p.Symbol, p.Account, p.Change, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	chainParams, ok := chain.Get(p.Symbol, s.wallet.Network())
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", p.Symbol)
	}

	path := fmt.Sprintf("m/%d'/%d'/%d'/%d/%d", chainParams.DefaultPurpose, chainParams.CoinType, p.Account, p.Change, p.Index)

	return &WalletGetAddressWithChangeResult{
		Address:  address,
		Path:     path,
		Symbol:   p.Symbol,
		IsChange: p.Change == 1,
	}, nil
}

// =============================================================================
// Multi-Address Wallet Methods
// =============================================================================

// WalletSendAllParams is the parameters for wallet_sendAll.
type WalletSendAllParams struct {
	Symbol string `json:"symbol"` // Chain symbol (BTC, LTC, etc.)
	To     string `json:"to"`     // Destination address
	Amount uint64 `json:"amount"` // Amount in smallest units (satoshis, etc.)
}

// WalletSendAllResult is the response for wallet_sendAll.
type WalletSendAllResult struct {
	TxID        string   `json:"txid"`
	Symbol      string   `json:"symbol"`
	To          string   `json:"to"`
	Amount      uint64   `json:"amount"`
	Fee         uint64   `json:"fee"`
	TotalInput  uint64   `json:"total_input"`
	Change      uint64   `json:"change"`
	InputCount  int      `json:"input_count"`
	OutputCount int      `json:"output_count"`
	UsedUTXOs   []string `json:"used_utxos"`
}

func (s *Server) walletSendAll(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var p WalletSendAllParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.To == "" {
		return nil, fmt.Errorf("to address is required")
	}
	if p.Amount == 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}

	result, err := s.wallet.SendFromAllAddresses(ctx, p.Symbol, p.To, p.Amount, s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to send: %w", err)
	}

	return &WalletSendAllResult{
		TxID:        result.TxID,
		Symbol:      p.Symbol,
		To:          p.To,
		Amount:      p.Amount,
		Fee:         result.Fee,
		TotalInput:  result.TotalInput,
		Change:      result.Change,
		InputCount:  result.InputCount,
		OutputCount: result.OutputCount,
		UsedUTXOs:   result.UsedUTXOs,
	}, nil
}

// WalletSendMaxParams is the parameters for wallet_sendMax.
type WalletSendMaxParams struct {
	Symbol string `json:"symbol"` // Chain symbol (BTC, LTC, etc.)
	To     string `json:"to"`     // Destination address
}

func (s *Server) walletSendMax(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var p WalletSendMaxParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.To == "" {
		return nil, fmt.Errorf("to address is required")
	}

	result, err := s.wallet.SendMaxFromAllAddresses(ctx, p.Symbol, p.To, s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to send max: %w", err)
	}

	return &WalletSendAllResult{
		TxID:        result.TxID,
		Symbol:      p.Symbol,
		To:          p.To,
		Amount:      result.TotalOutput,
		Fee:         result.Fee,
		TotalInput:  result.TotalInput,
		Change:      0, // No change for send max
		InputCount:  result.InputCount,
		OutputCount: result.OutputCount,
		UsedUTXOs:   result.UsedUTXOs,
	}, nil
}

// WalletAggregatedBalanceParams is the parameters for wallet_getAggregatedBalance.
type WalletAggregatedBalanceParams struct {
	Symbol string `json:"symbol"` // Chain symbol
}

// WalletAggregatedBalanceResult is the response for wallet_getAggregatedBalance.
type WalletAggregatedBalanceResult struct {
	Symbol      string `json:"symbol"`
	Confirmed   uint64 `json:"confirmed"`
	Unconfirmed uint64 `json:"unconfirmed"`
	Total       uint64 `json:"total"`
}

func (s *Server) walletGetAggregatedBalance(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var p WalletAggregatedBalanceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	confirmed, unconfirmed, err := s.wallet.GetAggregatedBalance(ctx, p.Symbol, s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregated balance: %w", err)
	}

	return &WalletAggregatedBalanceResult{
		Symbol:      p.Symbol,
		Confirmed:   confirmed,
		Unconfirmed: unconfirmed,
		Total:       confirmed + unconfirmed,
	}, nil
}

// WalletListAllUTXOsParams is the parameters for wallet_listAllUTXOs.
type WalletListAllUTXOsParams struct {
	Symbol string `json:"symbol"` // Chain symbol
}

// WalletListAllUTXOsResult is the response for wallet_listAllUTXOs.
type WalletListAllUTXOsResult struct {
	Symbol string         `json:"symbol"`
	UTXOs  []UTXOWithPath `json:"utxos"`
	Count  int            `json:"count"`
	Total  uint64         `json:"total"`
}

// UTXOWithPath represents a UTXO with its derivation path.
type UTXOWithPath struct {
	TxID         string `json:"txid"`
	Vout         uint32 `json:"vout"`
	Amount       uint64 `json:"amount"`
	Address      string `json:"address"`
	Account      uint32 `json:"account"`
	Change       uint32 `json:"change"`
	AddressIndex uint32 `json:"address_index"`
	AddressType  string `json:"address_type"`
	Path         string `json:"path"`
}

func (s *Server) walletListAllUTXOs(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var p WalletListAllUTXOsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	utxos, err := s.wallet.ListAllUTXOs(ctx, p.Symbol, s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to list UTXOs: %w", err)
	}

	chainParams, ok := chain.Get(p.Symbol, s.wallet.Network())
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", p.Symbol)
	}

	result := make([]UTXOWithPath, len(utxos))
	var total uint64
	for i, u := range utxos {
		path := fmt.Sprintf("m/%d'/%d'/%d'/%d/%d",
			chainParams.DefaultPurpose, chainParams.CoinType, u.Account, u.Change, u.AddressIndex)

		result[i] = UTXOWithPath{
			TxID:         u.TxID,
			Vout:         u.Vout,
			Amount:       u.Amount,
			Address:      u.Address,
			Account:      u.Account,
			Change:       u.Change,
			AddressIndex: u.AddressIndex,
			AddressType:  u.AddressType,
			Path:         path,
		}
		total += u.Amount
	}

	return &WalletListAllUTXOsResult{
		Symbol: p.Symbol,
		UTXOs:  result,
		Count:  len(result),
		Total:  total,
	}, nil
}

// WalletSyncUTXOsParams is the parameters for wallet_syncUTXOs.
type WalletSyncUTXOsParams struct {
	Symbol string `json:"symbol"` // Chain symbol
}

func (s *Server) walletSyncUTXOs(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}
	if s.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var p WalletSyncUTXOsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	err := s.wallet.ScanAndPersistUTXOs(ctx, p.Symbol, s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to sync UTXOs: %w", err)
	}

	// Return the current state after sync
	utxos, _ := s.wallet.ListAllUTXOs(ctx, p.Symbol, s.store)
	var total uint64
	for _, u := range utxos {
		total += u.Amount
	}

	return map[string]interface{}{
		"symbol":     p.Symbol,
		"utxo_count": len(utxos),
		"total":      total,
		"synced":     true,
	}, nil
}

// =============================================================================
// EVM Wallet Methods
// =============================================================================

// WalletSendEVMParams is the parameters for wallet_sendEVM.
type WalletSendEVMParams struct {
	Symbol  string `json:"symbol"`            // EVM chain symbol (ETH, BSC, MATIC, ARB)
	To      string `json:"to"`                // Destination address
	Amount  string `json:"amount"`            // Amount in wei (as string to handle big numbers)
	Account uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	Index   uint32 `json:"index,omitempty"`   // Address index (default 0)
}

// WalletSendEVMResult is the response for wallet_sendEVM.
type WalletSendEVMResult struct {
	TxHash   string `json:"tx_hash"`
	Symbol   string `json:"symbol"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	Nonce    uint64 `json:"nonce"`
	GasLimit uint64 `json:"gas_limit"`
	GasPrice string `json:"gas_price"`
}

func (s *Server) walletSendEVM(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletSendEVMParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.To == "" {
		return nil, fmt.Errorf("to address is required")
	}
	if p.Amount == "" {
		return nil, fmt.Errorf("amount is required")
	}

	// Check if it's an EVM chain
	if !s.wallet.IsEVMChain(p.Symbol) {
		return nil, fmt.Errorf("chain %s is not an EVM chain, use wallet_send instead", p.Symbol)
	}

	// Parse amount (wei as string to handle big numbers)
	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", p.Amount)
	}

	result, err := s.wallet.SendEVMTransaction(ctx, p.Symbol, p.To, amount, p.Account, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to send EVM transaction: %w", err)
	}

	return &WalletSendEVMResult{
		TxHash:   result.TxHash,
		Symbol:   p.Symbol,
		To:       p.To,
		Amount:   p.Amount,
		Nonce:    result.Nonce,
		GasLimit: result.GasLimit,
		GasPrice: result.GasPrice.String(),
	}, nil
}

// WalletSendERC20Params is the parameters for wallet_sendERC20.
type WalletSendERC20Params struct {
	Symbol   string `json:"symbol"`            // EVM chain symbol (ETH, BSC, MATIC, ARB)
	Token    string `json:"token"`             // ERC-20 token contract address
	To       string `json:"to"`                // Recipient address
	Amount   string `json:"amount"`            // Amount in token's smallest unit (as string)
	Account  uint32 `json:"account,omitempty"` // BIP44 account (default 0)
	Index    uint32 `json:"index,omitempty"`   // Address index (default 0)
}

// WalletSendERC20Result is the response for wallet_sendERC20.
type WalletSendERC20Result struct {
	TxHash   string `json:"tx_hash"`
	Symbol   string `json:"symbol"`
	Token    string `json:"token"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	Nonce    uint64 `json:"nonce"`
	GasLimit uint64 `json:"gas_limit"`
	GasPrice string `json:"gas_price"`
}

func (s *Server) walletSendERC20(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}
	if !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet is locked")
	}

	var p WalletSendERC20Params
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.Token == "" {
		return nil, fmt.Errorf("token contract address is required")
	}
	if p.To == "" {
		return nil, fmt.Errorf("to address is required")
	}
	if p.Amount == "" {
		return nil, fmt.Errorf("amount is required")
	}

	// Check if it's an EVM chain
	if !s.wallet.IsEVMChain(p.Symbol) {
		return nil, fmt.Errorf("chain %s is not an EVM chain", p.Symbol)
	}

	// Parse amount
	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount: %s", p.Amount)
	}

	result, err := s.wallet.SendERC20Transaction(ctx, p.Symbol, p.Token, p.To, amount, p.Account, p.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to send ERC-20 transaction: %w", err)
	}

	return &WalletSendERC20Result{
		TxHash:   result.TxHash,
		Symbol:   p.Symbol,
		Token:    p.Token,
		To:       p.To,
		Amount:   p.Amount,
		Nonce:    result.Nonce,
		GasLimit: result.GasLimit,
		GasPrice: result.GasPrice.String(),
	}, nil
}

// WalletGetERC20BalanceParams is the parameters for wallet_getERC20Balance.
type WalletGetERC20BalanceParams struct {
	Symbol  string `json:"symbol"`            // EVM chain symbol
	Token   string `json:"token"`             // ERC-20 token contract address
	Address string `json:"address,omitempty"` // Optional: specific address (otherwise uses wallet)
	Account uint32 `json:"account,omitempty"` // BIP44 account (if no address specified)
	Index   uint32 `json:"index,omitempty"`   // Address index (if no address specified)
}

// WalletGetERC20BalanceResult is the response for wallet_getERC20Balance.
type WalletGetERC20BalanceResult struct {
	Symbol  string `json:"symbol"`
	Token   string `json:"token"`
	Address string `json:"address"`
	Balance string `json:"balance"` // Balance in smallest unit (as string for big numbers)
}

func (s *Server) walletGetERC20Balance(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletGetERC20BalanceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if p.Token == "" {
		return nil, fmt.Errorf("token contract address is required")
	}

	// Check if it's an EVM chain
	if !s.wallet.IsEVMChain(p.Symbol) {
		return nil, fmt.Errorf("chain %s is not an EVM chain", p.Symbol)
	}

	var balance *big.Int
	var address string
	var err error

	if p.Address != "" {
		// Query specific address
		address = p.Address
		balance, err = s.wallet.GetERC20BalanceForAddress(ctx, p.Symbol, p.Token, p.Address)
	} else {
		// Query wallet address
		if !s.wallet.IsUnlocked() {
			return nil, fmt.Errorf("wallet is locked (provide address or unlock wallet)")
		}
		address, err = s.wallet.GetAddress(p.Symbol, p.Account, p.Index)
		if err != nil {
			return nil, fmt.Errorf("failed to get wallet address: %w", err)
		}
		balance, err = s.wallet.GetERC20Balance(ctx, p.Symbol, p.Token, p.Account, p.Index)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get ERC-20 balance: %w", err)
	}

	return &WalletGetERC20BalanceResult{
		Symbol:  p.Symbol,
		Token:   p.Token,
		Address: address,
		Balance: balance.String(),
	}, nil
}

// WalletListTokensParams is the parameters for wallet_listTokens.
type WalletListTokensParams struct {
	Symbol string `json:"symbol"` // EVM chain symbol (e.g., "ETH", "BSC", "POLYGON")
}

// WalletListTokensResult is the response for wallet_listTokens.
type WalletListTokensResult struct {
	Symbol string               `json:"symbol"`
	Tokens []WalletListTokensItem `json:"tokens"`
	Count  int                  `json:"count"`
}

// WalletListTokensItem represents a single ERC-20 token in the list.
type WalletListTokensItem struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals uint8  `json:"decimals"`
	Address  string `json:"address"`
	ChainID  uint64 `json:"chain_id"`
}

func (s *Server) walletListTokens(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p WalletListTokensParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	// Get chain params to find the chainID
	network := chain.Mainnet
	if s.wallet != nil {
		network = s.wallet.Network()
	}

	chainParams, ok := chain.Get(p.Symbol, network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", p.Symbol)
	}

	if chainParams.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain (ERC-20 tokens only exist on EVM chains)", p.Symbol)
	}

	tokens := chain.ListTokens(chainParams.ChainID)

	items := make([]WalletListTokensItem, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, WalletListTokensItem{
			Symbol:   t.Symbol,
			Name:     t.Name,
			Decimals: t.Decimals,
			Address:  t.Address,
			ChainID:  t.ChainID,
		})
	}

	return &WalletListTokensResult{
		Symbol: p.Symbol,
		Tokens: items,
		Count:  len(items),
	}, nil
}

// WalletGetChainTypeParams is the parameters for wallet_getChainType.
type WalletGetChainTypeParams struct {
	Symbol string `json:"symbol"`
}

func (s *Server) walletGetChainType(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet service not initialized")
	}

	var p WalletGetChainTypeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	chainType, err := s.wallet.GetChainType(p.Symbol)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"symbol":     p.Symbol,
		"chain_type": string(chainType),
		"is_evm":     chainType == chain.ChainTypeEVM,
		"is_bitcoin": chainType == chain.ChainTypeBitcoin,
	}, nil
}
