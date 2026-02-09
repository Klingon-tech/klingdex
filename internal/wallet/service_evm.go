// Package wallet provides EVM-specific wallet service methods.
package wallet

import (
	"context"
	"fmt"
	"math/big"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

// EVMSendResult holds the result of an EVM transaction.
type EVMSendResult struct {
	TxHash   string   `json:"tx_hash"`
	Nonce    uint64   `json:"nonce"`
	GasLimit uint64   `json:"gas_limit"`
	GasPrice *big.Int `json:"gas_price"`
}

// SendEVMTransaction sends a native token (ETH/BNB/MATIC/etc) transaction.
func (s *Service) SendEVMTransaction(ctx context.Context, symbol string, toAddress string, amount *big.Int, account, index uint32) (*EVMSendResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	// Get chain params to verify it's an EVM chain
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if params.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain, use SendTransaction instead", symbol)
	}

	// Get backend
	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	// Type assert to JSONRPCBackend for EVM-specific methods
	evmBackend, ok := b.(*backend.JSONRPCBackend)
	if !ok || !evmBackend.IsEVM() {
		return nil, fmt.Errorf("backend for %s is not an EVM backend", symbol)
	}

	// Get sender address
	fromAddress, err := s.wallet.DeriveAddress(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive address: %w", err)
	}

	// Get private key
	privKey, err := s.wallet.DerivePrivateKey(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive private key: %w", err)
	}

	// Get nonce
	nonce, err := evmBackend.EVMGetNonce(ctx, fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := evmBackend.EVMGetGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}

	// Estimate gas (for simple transfer, it's always 21000)
	gasLimit, err := evmBackend.EVMEstimateGas(ctx, fromAddress, toAddress, amount, nil)
	if err != nil {
		// Fallback to default gas limit for simple transfers
		gasLimit = DefaultGasLimit
	}

	// Build and sign transaction
	txResult, err := BuildAndSignEVMTx(privKey, &EVMTxParams{
		Nonce:    nonce,
		To:       toAddress,
		Value:    amount,
		ChainID:  params.ChainID,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Broadcast
	txHash, err := evmBackend.BroadcastTransaction(ctx, txResult.RawTx)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	return &EVMSendResult{
		TxHash:   txHash,
		Nonce:    nonce,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
	}, nil
}

// SendERC20Transaction sends an ERC-20 token transfer.
func (s *Service) SendERC20Transaction(ctx context.Context, symbol string, tokenContract string, toAddress string, amount *big.Int, account, index uint32) (*EVMSendResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	// Get chain params
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if params.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain", symbol)
	}

	// Validate addresses
	if !ValidateEVMAddress(tokenContract) {
		return nil, fmt.Errorf("invalid token contract address: %s", tokenContract)
	}
	if !ValidateEVMAddress(toAddress) {
		return nil, fmt.Errorf("invalid recipient address: %s", toAddress)
	}

	// Get backend
	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	evmBackend, ok := b.(*backend.JSONRPCBackend)
	if !ok || !evmBackend.IsEVM() {
		return nil, fmt.Errorf("backend for %s is not an EVM backend", symbol)
	}

	// Get sender address
	fromAddress, err := s.wallet.DeriveAddress(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive address: %w", err)
	}

	// Get private key
	privKey, err := s.wallet.DerivePrivateKey(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive private key: %w", err)
	}

	// Encode ERC-20 transfer call data
	callData, err := EncodeERC20Transfer(toAddress, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transfer: %w", err)
	}

	// Get nonce
	nonce, err := evmBackend.EVMGetNonce(ctx, fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := evmBackend.EVMGetGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}

	// Estimate gas for token transfer
	gasLimit, err := evmBackend.EVMEstimateGas(ctx, fromAddress, tokenContract, nil, callData)
	if err != nil {
		// Fallback to default ERC-20 gas limit
		gasLimit = DefaultERC20GasLimit
	}

	// Build and sign transaction
	txResult, err := BuildAndSignEVMTx(privKey, &EVMTxParams{
		Nonce:    nonce,
		To:       tokenContract,
		Value:    big.NewInt(0), // No native value for token transfers
		Data:     callData,
		ChainID:  params.ChainID,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Broadcast
	txHash, err := evmBackend.BroadcastTransaction(ctx, txResult.RawTx)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	return &EVMSendResult{
		TxHash:   txHash,
		Nonce:    nonce,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
	}, nil
}

// GetERC20Balance returns the balance of an ERC-20 token for an address.
func (s *Service) GetERC20Balance(ctx context.Context, symbol string, tokenContract string, account, index uint32) (*big.Int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	// Get chain params
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if params.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain", symbol)
	}

	// Validate token contract
	if !ValidateEVMAddress(tokenContract) {
		return nil, fmt.Errorf("invalid token contract address: %s", tokenContract)
	}

	// Get backend
	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	evmBackend, ok := b.(*backend.JSONRPCBackend)
	if !ok || !evmBackend.IsEVM() {
		return nil, fmt.Errorf("backend for %s is not an EVM backend", symbol)
	}

	// Get holder address
	address, err := s.wallet.DeriveAddress(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive address: %w", err)
	}

	// Encode balanceOf call
	callData, err := EncodeERC20BalanceOf(address)
	if err != nil {
		return nil, fmt.Errorf("failed to encode balanceOf: %w", err)
	}

	// Call contract
	result, err := evmBackend.EVMCall(ctx, tokenContract, callData)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	// Decode result
	balance, err := DecodeERC20BalanceResult(result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode balance: %w", err)
	}

	return balance, nil
}

// GetERC20BalanceForAddress returns the balance of an ERC-20 token for a specific address.
func (s *Service) GetERC20BalanceForAddress(ctx context.Context, symbol string, tokenContract string, address string) (*big.Int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	// Get chain params
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if params.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain", symbol)
	}

	// Validate addresses
	if !ValidateEVMAddress(tokenContract) {
		return nil, fmt.Errorf("invalid token contract address: %s", tokenContract)
	}
	if !ValidateEVMAddress(address) {
		return nil, fmt.Errorf("invalid address: %s", address)
	}

	// Get backend
	b, ok := s.backends.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("no backend for chain: %s", symbol)
	}

	evmBackend, ok := b.(*backend.JSONRPCBackend)
	if !ok || !evmBackend.IsEVM() {
		return nil, fmt.Errorf("backend for %s is not an EVM backend", symbol)
	}

	// Encode balanceOf call
	callData, err := EncodeERC20BalanceOf(address)
	if err != nil {
		return nil, fmt.Errorf("failed to encode balanceOf: %w", err)
	}

	// Call contract
	result, err := evmBackend.EVMCall(ctx, tokenContract, callData)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	// Decode result
	balance, err := DecodeERC20BalanceResult(result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode balance: %w", err)
	}

	return balance, nil
}

// GetEVMBalance returns the native token balance for an EVM chain address.
// Uses wei internally but returns as *big.Int for precision.
func (s *Service) GetEVMBalance(ctx context.Context, symbol string, account, index uint32) (*big.Int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not loaded")
	}

	if s.backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	// Get chain params
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if params.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("chain %s is not an EVM chain", symbol)
	}

	// Get address
	address, err := s.wallet.DeriveAddress(symbol, account, index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive address: %w", err)
	}

	// Get balance (uses eth_getBalance which returns wei)
	balance, err := s.GetBalance(ctx, symbol, address)
	if err != nil {
		return nil, err
	}

	return new(big.Int).SetUint64(balance), nil
}

// IsEVMChain returns true if the given symbol is an EVM chain.
func (s *Service) IsEVMChain(symbol string) bool {
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return false
	}
	return params.Type == chain.ChainTypeEVM
}

// GetChainType returns the chain type for a symbol.
func (s *Service) GetChainType(symbol string) (chain.ChainType, error) {
	params, ok := chain.Get(symbol, s.network)
	if !ok {
		return "", fmt.Errorf("unsupported chain: %s", symbol)
	}
	return params.Type, nil
}
