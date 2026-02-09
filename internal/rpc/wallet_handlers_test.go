package rpc

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWalletStatusResult(t *testing.T) {
	result := WalletStatusResult{
		HasWallet: true,
		Unlocked:  true,
		Network:   "testnet",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletStatusResult: %v", err)
	}

	var parsed WalletStatusResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletStatusResult: %v", err)
	}

	if !parsed.HasWallet {
		t.Error("HasWallet should be true")
	}
	if !parsed.Unlocked {
		t.Error("Unlocked should be true")
	}
	if parsed.Network != "testnet" {
		t.Errorf("Network = %s, want testnet", parsed.Network)
	}
}

func TestWalletGenerateResult(t *testing.T) {
	result := WalletGenerateResult{
		Mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletGenerateResult: %v", err)
	}

	var parsed WalletGenerateResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGenerateResult: %v", err)
	}

	if parsed.Mnemonic == "" {
		t.Error("Mnemonic should not be empty")
	}
}

func TestWalletCreateParams(t *testing.T) {
	params := WalletCreateParams{
		Mnemonic:   "test mnemonic",
		Passphrase: "optional passphrase",
		Password:   "TestPassword123!",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletCreateParams: %v", err)
	}

	var parsed WalletCreateParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletCreateParams: %v", err)
	}

	if parsed.Mnemonic != params.Mnemonic {
		t.Errorf("Mnemonic = %s, want %s", parsed.Mnemonic, params.Mnemonic)
	}
	if parsed.Password != params.Password {
		t.Errorf("Password = %s, want %s", parsed.Password, params.Password)
	}
}

func TestWalletUnlockParams(t *testing.T) {
	params := WalletUnlockParams{
		Password:   "TestPassword123!",
		Passphrase: "optional",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletUnlockParams: %v", err)
	}

	var parsed WalletUnlockParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletUnlockParams: %v", err)
	}

	if parsed.Password != params.Password {
		t.Errorf("Password = %s, want %s", parsed.Password, params.Password)
	}
}

func TestWalletGetAddressParams(t *testing.T) {
	params := WalletGetAddressParams{
		Symbol:  "BTC",
		Account: 0,
		Index:   5,
		Type:    "p2wpkh",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetAddressParams: %v", err)
	}

	var parsed WalletGetAddressParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetAddressParams: %v", err)
	}

	if parsed.Symbol != "BTC" {
		t.Errorf("Symbol = %s, want BTC", parsed.Symbol)
	}
	if parsed.Index != 5 {
		t.Errorf("Index = %d, want 5", parsed.Index)
	}
	if parsed.Type != "p2wpkh" {
		t.Errorf("Type = %s, want p2wpkh", parsed.Type)
	}
}

func TestWalletGetAddressResult(t *testing.T) {
	result := WalletGetAddressResult{
		Address: "bc1qtest",
		Path:    "m/84'/0'/0'/0/0",
		Symbol:  "BTC",
		Type:    "p2wpkh",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetAddressResult: %v", err)
	}

	var parsed WalletGetAddressResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetAddressResult: %v", err)
	}

	if parsed.Address != result.Address {
		t.Errorf("Address = %s, want %s", parsed.Address, result.Address)
	}
	if parsed.Path != result.Path {
		t.Errorf("Path = %s, want %s", parsed.Path, result.Path)
	}
}

func TestWalletGetAllAddressesParams(t *testing.T) {
	params := WalletGetAllAddressesParams{
		Symbol:  "BTC",
		Account: 1,
		Index:   2,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetAllAddressesParams: %v", err)
	}

	var parsed WalletGetAllAddressesParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetAllAddressesParams: %v", err)
	}

	if parsed.Account != 1 {
		t.Errorf("Account = %d, want 1", parsed.Account)
	}
}

func TestWalletGetAllAddressesResult(t *testing.T) {
	result := WalletGetAllAddressesResult{
		Addresses: map[string]string{
			"p2pkh":  "1Test",
			"p2wpkh": "bc1qtest",
		},
		Path:   "m/84'/0'/0'/0/0",
		Symbol: "BTC",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetAllAddressesResult: %v", err)
	}

	var parsed WalletGetAllAddressesResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetAllAddressesResult: %v", err)
	}

	if len(parsed.Addresses) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(parsed.Addresses))
	}
}

func TestWalletGetPublicKeyParams(t *testing.T) {
	params := WalletGetPublicKeyParams{
		Symbol:  "ETH",
		Account: 0,
		Index:   0,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetPublicKeyParams: %v", err)
	}

	var parsed WalletGetPublicKeyParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetPublicKeyParams: %v", err)
	}

	if parsed.Symbol != "ETH" {
		t.Errorf("Symbol = %s, want ETH", parsed.Symbol)
	}
}

func TestWalletGetPublicKeyResult(t *testing.T) {
	result := WalletGetPublicKeyResult{
		PublicKey:    "02abcdef",
		Uncompressed: "04abcdef",
		Path:         "m/44'/60'/0'/0/0",
		Symbol:       "ETH",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetPublicKeyResult: %v", err)
	}

	var parsed WalletGetPublicKeyResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetPublicKeyResult: %v", err)
	}

	if parsed.PublicKey != result.PublicKey {
		t.Errorf("PublicKey = %s, want %s", parsed.PublicKey, result.PublicKey)
	}
}

func TestWalletSupportedChainsResult(t *testing.T) {
	result := WalletSupportedChainsResult{
		Chains: []ChainInfo{
			{Symbol: "BTC", Name: "Bitcoin", Type: "bitcoin", Decimals: 8},
			{Symbol: "ETH", Name: "Ethereum", Type: "evm", Decimals: 18},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletSupportedChainsResult: %v", err)
	}

	var parsed WalletSupportedChainsResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletSupportedChainsResult: %v", err)
	}

	if len(parsed.Chains) != 2 {
		t.Errorf("expected 2 chains, got %d", len(parsed.Chains))
	}
	if parsed.Chains[0].Decimals != 8 {
		t.Errorf("BTC decimals = %d, want 8", parsed.Chains[0].Decimals)
	}
}

func TestChainInfo(t *testing.T) {
	info := ChainInfo{
		Symbol:   "LTC",
		Name:     "Litecoin",
		Type:     "bitcoin",
		Decimals: 8,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal ChainInfo: %v", err)
	}

	var parsed ChainInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ChainInfo: %v", err)
	}

	if parsed.Symbol != "LTC" {
		t.Errorf("Symbol = %s, want LTC", parsed.Symbol)
	}
}

func TestWalletValidateMnemonicParams(t *testing.T) {
	params := WalletValidateMnemonicParams{
		Mnemonic: "test words here",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletValidateMnemonicParams: %v", err)
	}

	var parsed WalletValidateMnemonicParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletValidateMnemonicParams: %v", err)
	}

	if parsed.Mnemonic != params.Mnemonic {
		t.Errorf("Mnemonic = %s, want %s", parsed.Mnemonic, params.Mnemonic)
	}
}

func TestWalletGetBalanceParams(t *testing.T) {
	params := WalletGetBalanceParams{
		Symbol:  "BTC",
		Address: "bc1qtest",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetBalanceParams: %v", err)
	}

	var parsed WalletGetBalanceParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetBalanceParams: %v", err)
	}

	if parsed.Symbol != "BTC" {
		t.Errorf("Symbol = %s, want BTC", parsed.Symbol)
	}
	if parsed.Address != "bc1qtest" {
		t.Errorf("Address = %s, want bc1qtest", parsed.Address)
	}
}

func TestWalletGetBalanceResult(t *testing.T) {
	result := WalletGetBalanceResult{
		Balance: 100000000, // 1 BTC in satoshis
		Symbol:  "BTC",
		Address: "bc1qtest",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetBalanceResult: %v", err)
	}

	var parsed WalletGetBalanceResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetBalanceResult: %v", err)
	}

	if parsed.Balance != 100000000 {
		t.Errorf("Balance = %d, want 100000000", parsed.Balance)
	}
}

func TestWalletGetFeeEstimatesParams(t *testing.T) {
	params := WalletGetFeeEstimatesParams{
		Symbol: "BTC",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletGetFeeEstimatesParams: %v", err)
	}

	var parsed WalletGetFeeEstimatesParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletGetFeeEstimatesParams: %v", err)
	}

	if parsed.Symbol != "BTC" {
		t.Errorf("Symbol = %s, want BTC", parsed.Symbol)
	}
}

func TestWalletParamsOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when zero
	params := WalletGetAddressParams{
		Symbol: "BTC",
		// Account and Index are 0 (omitempty should work)
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse back
	var parsed WalletGetAddressParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Account != 0 {
		t.Errorf("Account should be 0, got %d", parsed.Account)
	}
	if parsed.Index != 0 {
		t.Errorf("Index should be 0, got %d", parsed.Index)
	}
}

// ============ Handler Error Tests ============

func TestWalletStatusHandlerNoWallet(t *testing.T) {
	// Server without wallet
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletStatus(context.Background(), nil)
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGenerateHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGenerate(context.Background(), nil)
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletCreateHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletCreate(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletCreateHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletCreate(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletUnlockHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletUnlock(context.Background(), json.RawMessage(`{"password":"test"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletUnlockHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletUnlock(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletLockHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletLock(context.Background(), nil)
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetAddressHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGetAddress(context.Background(), json.RawMessage(`{"symbol":"BTC"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetAddressHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletGetAddress(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletGetAllAddressesHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGetAllAddresses(context.Background(), json.RawMessage(`{"symbol":"BTC"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetAllAddressesHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletGetAllAddresses(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletGetPublicKeyHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGetPublicKey(context.Background(), json.RawMessage(`{"symbol":"BTC"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetPublicKeyHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletGetPublicKey(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletSupportedChainsHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletSupportedChains(context.Background(), nil)
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletValidateMnemonicHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletValidateMnemonic(context.Background(), json.RawMessage(`{"mnemonic":"test"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletValidateMnemonicHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletValidateMnemonic(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletGetBalanceHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGetBalance(context.Background(), json.RawMessage(`{"symbol":"BTC","address":"test"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetBalanceHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletGetBalance(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWalletGetFeeEstimatesHandlerNoWallet(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	_, err := s.walletGetFeeEstimates(context.Background(), json.RawMessage(`{"symbol":"BTC"}`))
	if err == nil {
		t.Error("expected error when wallet is nil")
	}
}

func TestWalletGetFeeEstimatesHandlerInvalidParams(t *testing.T) {
	s := &Server{
		wallet: nil,
	}

	// Invalid JSON
	_, err := s.walletGetFeeEstimates(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
