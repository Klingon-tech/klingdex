// Package swap - EVM HTLC types for atomic swaps with EVM chains.
package swap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/config"
	"github.com/klingon-exchange/klingon-v2/internal/contracts/htlc"
)

// EVMSwapState represents the state of an EVM HTLC swap
type EVMSwapState uint8

const (
	EVMSwapStateEmpty    EVMSwapState = 0
	EVMSwapStateActive   EVMSwapState = 1
	EVMSwapStateClaimed  EVMSwapState = 2
	EVMSwapStateRefunded EVMSwapState = 3
)

func (s EVMSwapState) String() string {
	switch s {
	case EVMSwapStateEmpty:
		return "empty"
	case EVMSwapStateActive:
		return "active"
	case EVMSwapStateClaimed:
		return "claimed"
	case EVMSwapStateRefunded:
		return "refunded"
	default:
		return "unknown"
	}
}

// EVMHTLCSession manages the EVM HTLC protocol for one chain in an atomic swap.
type EVMHTLCSession struct {
	mu sync.RWMutex

	// Chain identity
	symbol  string
	chainID uint64
	network chain.Network

	// Client for contract interaction
	client *htlc.Client

	// Keys
	localPrivKey *ecdsa.PrivateKey
	localAddress common.Address

	// Secret (initiator has both, responder only has hash until revealed)
	secret     [32]byte
	secretHash [32]byte
	hasSecret  bool

	// Swap parameters
	swapID       [32]byte
	receiver     common.Address
	tokenAddress common.Address // Zero address for native token
	amount       *big.Int
	timelock     *big.Int

	// Transaction hashes
	createTxHash common.Hash
	claimTxHash  common.Hash
	refundTxHash common.Hash

	// State
	state       EVMSwapState
	isInitiator bool
}

// EVMHTLCStorageData is the serializable form for persistence.
type EVMHTLCStorageData struct {
	Symbol       string `json:"symbol"`
	ChainID      uint64 `json:"chain_id"`
	Network      string `json:"network"`
	LocalPrivKey string `json:"local_priv_key,omitempty"` // Hex-encoded
	LocalAddress string `json:"local_address"`

	// Secret
	Secret     string `json:"secret,omitempty"`      // Hex, only for initiator
	SecretHash string `json:"secret_hash,omitempty"` // Hex
	HasSecret  bool   `json:"has_secret"`

	// Swap parameters
	SwapID       string `json:"swap_id"`       // Hex
	Receiver     string `json:"receiver"`      // Address
	TokenAddress string `json:"token_address"` // Zero for native
	Amount       string `json:"amount"`        // Wei as string
	Timelock     string `json:"timelock"`      // Unix timestamp

	// Transaction hashes
	CreateTxHash string `json:"create_tx_hash,omitempty"`
	ClaimTxHash  string `json:"claim_tx_hash,omitempty"`
	RefundTxHash string `json:"refund_tx_hash,omitempty"`

	// State
	State       uint8 `json:"state"`
	IsInitiator bool  `json:"is_initiator"`
}

// ChainEVMHTLCData holds EVM HTLC data for a single chain in the swap.
type ChainEVMHTLCData struct {
	Session         *EVMHTLCSession
	ContractAddress common.Address
	SwapID          [32]byte
	CreateTxHash    common.Hash
	ClaimTxHash     common.Hash
	RefundTxHash    common.Hash
}

// EVMHTLCSwapData holds EVM HTLC-specific data for a swap.
type EVMHTLCSwapData struct {
	LocalPrivKey *ecdsa.PrivateKey
	OfferChain   *ChainEVMHTLCData  // Only set if offer chain is EVM
	RequestChain *ChainEVMHTLCData  // Only set if request chain is EVM
}

// =============================================================================
// EVMHTLCSession Constructor
// =============================================================================

// NewEVMHTLCSession creates a new EVM HTLC session.
func NewEVMHTLCSession(symbol string, network chain.Network, rpcURL string) (*EVMHTLCSession, error) {
	// Validate chain is EVM
	chainParams, ok := chain.Get(symbol, network)
	if !ok {
		return nil, fmt.Errorf("unsupported chain: %s", symbol)
	}
	if chainParams.Type != chain.ChainTypeEVM {
		return nil, fmt.Errorf("EVM HTLC only supported for EVM chains, got %s", chainParams.Type)
	}

	// Get contract address
	contractAddr := config.GetHTLCContract(chainParams.ChainID)
	if contractAddr == (common.Address{}) {
		return nil, fmt.Errorf("HTLC contract not deployed on %s (chainID %d)", symbol, chainParams.ChainID)
	}

	// Create client
	client, err := htlc.NewClient(rpcURL, contractAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTLC client: %w", err)
	}

	return &EVMHTLCSession{
		symbol:  symbol,
		chainID: chainParams.ChainID,
		network: network,
		client:  client,
		state:   EVMSwapStateEmpty,
	}, nil
}

// =============================================================================
// Key Management
// =============================================================================

// SetLocalKey sets the local private key for signing transactions.
func (s *EVMHTLCSession) SetLocalKey(privKey *ecdsa.PrivateKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localPrivKey = privKey
	s.localAddress = htlc.AddressFromPrivateKey(privKey)
}

// GetLocalAddress returns the local address.
func (s *EVMHTLCSession) GetLocalAddress() common.Address {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localAddress
}

// =============================================================================
// Secret Management
// =============================================================================

// GenerateSecret generates a new secret (initiator only).
func (s *EVMHTLCSession) GenerateSecret() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	secret, hash, err := htlc.GenerateSecret()
	if err != nil {
		return fmt.Errorf("failed to generate secret: %w", err)
	}

	s.secret = secret
	s.secretHash = hash
	s.hasSecret = true
	s.isInitiator = true
	return nil
}

// SetSecretHash sets the secret hash (responder only).
func (s *EVMHTLCSession) SetSecretHash(hash [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secretHash = hash
	s.isInitiator = false
}

// SetSecret sets the secret (responder after reveal).
func (s *EVMHTLCSession) SetSecret(secret [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify secret matches hash
	if !htlc.VerifySecret(secret, s.secretHash) {
		return fmt.Errorf("secret does not match hash")
	}

	s.secret = secret
	s.hasSecret = true
	return nil
}

// GetSecret returns the secret if available.
func (s *EVMHTLCSession) GetSecret() [32]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secret
}

// GetSecretHash returns the secret hash.
func (s *EVMHTLCSession) GetSecretHash() [32]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secretHash
}

// HasSecret returns true if the secret is known.
func (s *EVMHTLCSession) HasSecret() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasSecret
}

// IsInitiator returns true if this session is the initiator.
func (s *EVMHTLCSession) IsInitiator() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isInitiator
}

// =============================================================================
// Remote Address
// =============================================================================

// SetRemoteAddress sets the counterparty's EVM address.
func (s *EVMHTLCSession) SetRemoteAddress(addr common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receiver = addr
}

// GetRemoteAddress returns the counterparty's EVM address.
func (s *EVMHTLCSession) GetRemoteAddress() common.Address {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.receiver
}

// =============================================================================
// Swap Operations
// =============================================================================

// SetSwapParams sets the swap parameters before creating.
func (s *EVMHTLCSession) SetSwapParams(swapID [32]byte, receiver common.Address, token common.Address, amount *big.Int, timelock *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swapID = swapID
	s.receiver = receiver
	s.tokenAddress = token
	s.amount = amount
	s.timelock = timelock
}

// GetSwapID returns the swap ID.
func (s *EVMHTLCSession) GetSwapID() [32]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.swapID
}

// CreateSwapNative creates an HTLC with native token (ETH/BNB/etc).
func (s *EVMHTLCSession) CreateSwapNative(ctx context.Context) (common.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPrivKey == nil {
		return common.Hash{}, fmt.Errorf("local private key not set")
	}

	tx, err := s.client.CreateSwapNative(ctx, s.localPrivKey, s.swapID, s.receiver, s.secretHash, s.timelock, s.amount)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create native swap: %w", err)
	}

	s.createTxHash = tx.Hash()
	s.state = EVMSwapStateActive
	return tx.Hash(), nil
}

// CreateSwapERC20 creates an HTLC with ERC20 token.
func (s *EVMHTLCSession) CreateSwapERC20(ctx context.Context) (common.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPrivKey == nil {
		return common.Hash{}, fmt.Errorf("local private key not set")
	}
	if s.tokenAddress == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("token address not set for ERC20 swap")
	}

	tx, err := s.client.CreateSwapERC20(ctx, s.localPrivKey, s.swapID, s.receiver, s.tokenAddress, s.amount, s.secretHash, s.timelock)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create ERC20 swap: %w", err)
	}

	s.createTxHash = tx.Hash()
	s.state = EVMSwapStateActive
	return tx.Hash(), nil
}

// Claim claims the HTLC using the secret.
func (s *EVMHTLCSession) Claim(ctx context.Context) (common.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPrivKey == nil {
		return common.Hash{}, fmt.Errorf("local private key not set")
	}
	if !s.hasSecret {
		return common.Hash{}, fmt.Errorf("secret not available")
	}

	tx, err := s.client.Claim(ctx, s.localPrivKey, s.swapID, s.secret)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to claim: %w", err)
	}

	s.claimTxHash = tx.Hash()
	s.state = EVMSwapStateClaimed
	return tx.Hash(), nil
}

// Refund refunds the HTLC after timeout.
func (s *EVMHTLCSession) Refund(ctx context.Context) (common.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPrivKey == nil {
		return common.Hash{}, fmt.Errorf("local private key not set")
	}

	tx, err := s.client.Refund(ctx, s.localPrivKey, s.swapID)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to refund: %w", err)
	}

	s.refundTxHash = tx.Hash()
	s.state = EVMSwapStateRefunded
	return tx.Hash(), nil
}

// =============================================================================
// Status Queries
// =============================================================================

// GetState returns the current swap state.
func (s *EVMHTLCSession) GetState() EVMSwapState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// GetSwapFromChain queries the on-chain state.
func (s *EVMHTLCSession) GetSwapFromChain(ctx context.Context) (*htlc.Swap, error) {
	return s.client.GetSwap(ctx, s.swapID)
}

// CanClaim returns true if the swap can be claimed.
func (s *EVMHTLCSession) CanClaim(ctx context.Context) (bool, error) {
	return s.client.CanClaim(ctx, s.swapID)
}

// CanRefund returns true if the swap can be refunded.
func (s *EVMHTLCSession) CanRefund(ctx context.Context) (bool, error) {
	return s.client.CanRefund(ctx, s.swapID)
}

// TimeUntilRefund returns seconds until refund is possible.
func (s *EVMHTLCSession) TimeUntilRefund(ctx context.Context) (*big.Int, error) {
	return s.client.TimeUntilRefund(ctx, s.swapID)
}

// =============================================================================
// Secret Monitoring
// =============================================================================

// WaitForSecret waits for the swap to be claimed and returns the secret.
func (s *EVMHTLCSession) WaitForSecret(ctx context.Context) ([32]byte, error) {
	secret, err := s.client.WaitForSecret(ctx, s.swapID)
	if err != nil {
		return [32]byte{}, err
	}

	// Store the secret
	s.mu.Lock()
	s.secret = secret
	s.hasSecret = true
	s.mu.Unlock()

	return secret, nil
}

// GetSecretFromClaimTx extracts the secret from a claim transaction.
func (s *EVMHTLCSession) GetSecretFromClaimTx(ctx context.Context, txHash common.Hash) ([32]byte, error) {
	return s.client.GetSecretFromClaim(ctx, txHash)
}

// =============================================================================
// Serialization
// =============================================================================

// ToStorageData serializes the session for persistence.
func (s *EVMHTLCSession) ToStorageData() *EVMHTLCStorageData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := &EVMHTLCStorageData{
		Symbol:       s.symbol,
		ChainID:      s.chainID,
		Network:      string(s.network),
		LocalAddress: s.localAddress.Hex(),
		SecretHash:   common.Bytes2Hex(s.secretHash[:]),
		HasSecret:    s.hasSecret,
		SwapID:       common.Bytes2Hex(s.swapID[:]),
		Receiver:     s.receiver.Hex(),
		TokenAddress: s.tokenAddress.Hex(),
		State:        uint8(s.state),
		IsInitiator:  s.isInitiator,
	}

	if s.hasSecret {
		data.Secret = common.Bytes2Hex(s.secret[:])
	}
	if s.amount != nil {
		data.Amount = s.amount.String()
	}
	if s.timelock != nil {
		data.Timelock = s.timelock.String()
	}
	if s.createTxHash != (common.Hash{}) {
		data.CreateTxHash = s.createTxHash.Hex()
	}
	if s.claimTxHash != (common.Hash{}) {
		data.ClaimTxHash = s.claimTxHash.Hex()
	}
	if s.refundTxHash != (common.Hash{}) {
		data.RefundTxHash = s.refundTxHash.Hex()
	}

	return data
}

// Close closes the session and releases resources.
func (s *EVMHTLCSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// IsEVMChain returns true if the chain is an EVM chain.
func IsEVMChain(symbol string, network chain.Network) bool {
	params, ok := chain.Get(symbol, network)
	if !ok {
		return false
	}
	return params.Type == chain.ChainTypeEVM
}

// IsBitcoinChain returns true if the chain is a Bitcoin-family chain.
func IsBitcoinChain(symbol string, network chain.Network) bool {
	params, ok := chain.Get(symbol, network)
	if !ok {
		return false
	}
	return params.Type == chain.ChainTypeBitcoin
}

// GetCrossChainType determines the swap type based on offer and request chains.
type CrossChainType int

const (
	CrossChainTypeBitcoinToBitcoin CrossChainType = iota // BTC <-> LTC
	CrossChainTypeEVMToEVM                               // ETH <-> BSC
	CrossChainTypeBitcoinToEVM                           // BTC <-> ETH
	CrossChainTypeEVMToBitcoin                           // ETH <-> BTC
	CrossChainTypeUnknown
)

func (t CrossChainType) String() string {
	switch t {
	case CrossChainTypeBitcoinToBitcoin:
		return "bitcoin_to_bitcoin"
	case CrossChainTypeEVMToEVM:
		return "evm_to_evm"
	case CrossChainTypeBitcoinToEVM:
		return "bitcoin_to_evm"
	case CrossChainTypeEVMToBitcoin:
		return "evm_to_bitcoin"
	default:
		return "unknown"
	}
}

// IsCrossChain returns true if this is a cross-chain swap (Bitcoin <-> EVM).
func (t CrossChainType) IsCrossChain() bool {
	return t == CrossChainTypeBitcoinToEVM || t == CrossChainTypeEVMToBitcoin
}

// InvolvesEVM returns true if this swap type involves an EVM chain.
func (t CrossChainType) InvolvesEVM() bool {
	return t == CrossChainTypeEVMToEVM || t == CrossChainTypeBitcoinToEVM || t == CrossChainTypeEVMToBitcoin
}

// InvolvesBitcoin returns true if this swap type involves a Bitcoin-family chain.
func (t CrossChainType) InvolvesBitcoin() bool {
	return t == CrossChainTypeBitcoinToBitcoin || t == CrossChainTypeBitcoinToEVM || t == CrossChainTypeEVMToBitcoin
}

// GetCrossChainSwapType determines the swap type.
func GetCrossChainSwapType(offerChain, requestChain string, network chain.Network) CrossChainType {
	offerIsEVM := IsEVMChain(offerChain, network)
	requestIsEVM := IsEVMChain(requestChain, network)
	offerIsBTC := IsBitcoinChain(offerChain, network)
	requestIsBTC := IsBitcoinChain(requestChain, network)

	if offerIsBTC && requestIsBTC {
		return CrossChainTypeBitcoinToBitcoin
	}
	if offerIsEVM && requestIsEVM {
		return CrossChainTypeEVMToEVM
	}
	if offerIsBTC && requestIsEVM {
		return CrossChainTypeBitcoinToEVM
	}
	if offerIsEVM && requestIsBTC {
		return CrossChainTypeEVMToBitcoin
	}
	return CrossChainTypeUnknown
}
