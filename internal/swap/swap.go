// Package swap implements atomic swap protocol logic.
// This package contains ONLY protocol-specific logic (key aggregation, state machine, messages).
// It uses existing packages directly:
//   - chain.Get() for chain parameters
//   - backend.Backend for blockchain operations
//   - wallet.Wallet for key operations
//   - config for fees and DAO addresses
package swap

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/config"
)

// Common errors
var (
	ErrUnsupportedChain         = errors.New("unsupported chain")
	ErrTaprootNotSupported      = errors.New("taproot not supported on this chain")
	ErrInvalidState             = errors.New("invalid swap state")
	ErrInvalidPubKey            = errors.New("invalid public key")
	ErrKeyAggregationFailed     = errors.New("key aggregation failed")
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrSwapExpired              = errors.New("swap expired")
	ErrSecretMismatch           = errors.New("secret does not match hash")
	ErrTimeoutRace              = errors.New("too close to timeout - safety margin not met")
	ErrInsufficientConfirmations = errors.New("insufficient confirmations")
)

// Role represents the role in a swap (initiator or responder).
type Role string

const (
	RoleInitiator Role = "initiator"
	RoleResponder Role = "responder"
)

// State represents the current state of a swap.
type State string

const (
	StateNone       State = ""
	StateInit       State = "init"       // Initial state, negotiating terms
	StateFunding    State = "funding"    // Waiting for funding transactions
	StateFunded     State = "funded"     // Both parties have funded
	StateRedeemed   State = "redeemed"   // Swap completed successfully
	StateRefunded   State = "refunded"   // Swap refunded (timeout)
	StateFailed     State = "failed"     // Swap failed
	StateCancelled  State = "cancelled"  // Swap cancelled before funding
)

// Method represents the swap method.
type Method string

const (
	MethodMuSig2   Method = "musig2"   // MuSig2 (Taproot)
	MethodHTLC     Method = "htlc"     // Hash Time-Locked Contract
	MethodAdaptor  Method = "adaptor"  // Adaptor signatures (for Monero)
	MethodContract Method = "contract" // Smart contract (EVM)
)

// ChainConfig holds chain-specific configuration for a swap.
// This is constructed from chain.Params, not hardcoded.
type ChainConfig struct {
	Symbol          string
	Network         chain.Network
	SupportsTaproot bool
	Decimals        uint8
}

// NewChainConfig creates a ChainConfig from the chain registry.
func NewChainConfig(symbol string, network chain.Network) (*ChainConfig, error) {
	params, ok := chain.Get(symbol, network)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChain, symbol)
	}

	return &ChainConfig{
		Symbol:          symbol,
		Network:         network,
		SupportsTaproot: params.SupportsTaproot,
		Decimals:        params.Decimals,
	}, nil
}

// SupportsMethod checks if the chain supports the given swap method.
func (c *ChainConfig) SupportsMethod(method Method) bool {
	switch method {
	case MethodMuSig2:
		return c.SupportsTaproot
	case MethodHTLC:
		// Bitcoin-family chains support HTLC via P2WSH scripts
		// EVM chains support HTLC via smart contracts
		params, ok := chain.Get(c.Symbol, c.Network)
		if !ok {
			return false
		}
		return params.Type == chain.ChainTypeBitcoin || params.Type == chain.ChainTypeEVM
	default:
		return false
	}
}

// Offer represents a swap offer from one party.
type Offer struct {
	// Chain being offered
	OfferChain string
	// Amount being offered (in smallest unit)
	OfferAmount uint64
	// Chain being requested
	RequestChain string
	// Amount being requested (in smallest unit)
	RequestAmount uint64
	// Preferred swap method
	Method Method
	// Offer expiry
	ExpiresAt time.Time
}

// Validate checks if the offer is valid.
func (o *Offer) Validate(network chain.Network) error {
	// Check both chains are supported
	offerCfg, err := NewChainConfig(o.OfferChain, network)
	if err != nil {
		return fmt.Errorf("offer chain: %w", err)
	}

	requestCfg, err := NewChainConfig(o.RequestChain, network)
	if err != nil {
		return fmt.Errorf("request chain: %w", err)
	}

	// Check method is supported on both chains
	if !offerCfg.SupportsMethod(o.Method) {
		return fmt.Errorf("%s does not support %s", o.OfferChain, o.Method)
	}
	if !requestCfg.SupportsMethod(o.Method) {
		return fmt.Errorf("%s does not support %s", o.RequestChain, o.Method)
	}

	// Check amounts are within limits
	offerCoin, _ := config.GetCoin(o.OfferChain)
	if o.OfferAmount < offerCoin.MinAmount {
		return fmt.Errorf("offer amount below minimum: %d < %d", o.OfferAmount, offerCoin.MinAmount)
	}
	if offerCoin.MaxAmount > 0 && o.OfferAmount > offerCoin.MaxAmount {
		return fmt.Errorf("offer amount above maximum: %d > %d", o.OfferAmount, offerCoin.MaxAmount)
	}

	requestCoin, _ := config.GetCoin(o.RequestChain)
	if o.RequestAmount < requestCoin.MinAmount {
		return fmt.Errorf("request amount below minimum: %d < %d", o.RequestAmount, requestCoin.MinAmount)
	}
	if requestCoin.MaxAmount > 0 && o.RequestAmount > requestCoin.MaxAmount {
		return fmt.Errorf("request amount above maximum: %d > %d", o.RequestAmount, requestCoin.MaxAmount)
	}

	return nil
}

// Swap represents an atomic swap between two parties.
type Swap struct {
	// Unique identifier
	ID string

	// Network (mainnet/testnet)
	Network chain.Network

	// Method being used
	Method Method

	// Our role in the swap
	Role Role

	// Current state
	State State

	// Offer details
	Offer Offer

	// Timing (time-based)
	CreatedAt      time.Time
	InitiatorLock  time.Duration // Lock time for initiator's funds
	ResponderLock  time.Duration // Lock time for responder's funds

	// Block-based timeout tracking (SECURITY)
	// These are more precise than time-based for blockchain operations
	OfferChainStartHeight   uint32 // Block height when swap started on offer chain
	RequestChainStartHeight uint32 // Block height when swap started on request chain
	OfferChainTimeoutHeight uint32 // Block height when offer chain timeout expires
	RequestChainTimeoutHeight uint32 // Block height when request chain timeout expires

	// Public keys (compressed, 33 bytes)
	LocalPubKey  []byte // Our public key for the swap
	RemotePubKey []byte // Counterparty's public key

	// MuSig2-specific fields
	AggregatedPubKey []byte // Aggregated public key (for Taproot address)

	// Funding transaction info with confirmation tracking
	LocalFundingTxID     string
	LocalFundingVout     uint32
	LocalFundingConfirms uint32 // Current confirmation count
	RemoteFundingTxID    string
	RemoteFundingVout    uint32
	RemoteFundingConfirms uint32 // Current confirmation count

	// Wallet addresses for redemption
	// Each party provides their addresses on both chains
	// Offer chain: initiator funded → responder redeems → needs responder's address
	// Request chain: responder funded → initiator redeems → needs initiator's address
	LocalOfferWalletAddr   string // Our address on offer chain (where we receive if we're responder)
	LocalRequestWalletAddr string // Our address on request chain (where we receive if we're initiator)
	RemoteOfferWalletAddr  string // Counterparty's address on offer chain
	RemoteRequestWalletAddr string // Counterparty's address on request chain

	// Secret (only initiator has this initially)
	Secret     []byte // 32-byte secret
	SecretHash []byte // SHA256(secret)
}

// NewSwap creates a new swap with a generated ID.
func NewSwap(network chain.Network, method Method, role Role, offer Offer) (*Swap, error) {
	// Validate offer
	if err := offer.Validate(network); err != nil {
		return nil, fmt.Errorf("invalid offer: %w", err)
	}

	// Get swap config for timeouts
	swapCfg := config.DefaultSwapConfig()

	// Generate unique ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("failed to generate ID: %w", err)
	}

	return &Swap{
		ID:            fmt.Sprintf("%x", idBytes),
		Network:       network,
		Method:        method,
		Role:          role,
		State:         StateInit,
		Offer:         offer,
		CreatedAt:     time.Now(),
		InitiatorLock: swapCfg.InitiatorLockTime,
		ResponderLock: swapCfg.ResponderLockTime,
	}, nil
}

// SetLocalPubKey sets our public key for the swap.
func (s *Swap) SetLocalPubKey(pubKey *btcec.PublicKey) {
	s.LocalPubKey = pubKey.SerializeCompressed()
}

// SetRemotePubKey sets the counterparty's public key.
func (s *Swap) SetRemotePubKey(pubKey *btcec.PublicKey) error {
	if pubKey == nil {
		return ErrInvalidPubKey
	}
	s.RemotePubKey = pubKey.SerializeCompressed()
	return nil
}

// GetLocalPubKey returns our public key as a btcec.PublicKey.
func (s *Swap) GetLocalPubKey() (*btcec.PublicKey, error) {
	if len(s.LocalPubKey) == 0 {
		return nil, ErrInvalidPubKey
	}
	return btcec.ParsePubKey(s.LocalPubKey)
}

// GetRemotePubKey returns the counterparty's public key as a btcec.PublicKey.
func (s *Swap) GetRemotePubKey() (*btcec.PublicKey, error) {
	if len(s.RemotePubKey) == 0 {
		return nil, ErrInvalidPubKey
	}
	return btcec.ParsePubKey(s.RemotePubKey)
}

// GenerateSecret generates a random 32-byte secret and its hash.
// Only the initiator calls this.
func (s *Swap) GenerateSecret() error {
	if s.Role != RoleInitiator {
		return errors.New("only initiator can generate secret")
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("failed to generate secret: %w", err)
	}

	s.Secret = secret
	s.SecretHash = HashSecret(secret)
	return nil
}

// HashSecret computes SHA256 hash of a secret.
func HashSecret(secret []byte) []byte {
	// Simple SHA256 hash of the secret
	hash := sha256.Sum256(secret)
	return hash[:]
}

// VerifySecret checks if a secret matches the stored hash.
func (s *Swap) VerifySecret(secret []byte) bool {
	if len(s.SecretHash) == 0 {
		return false
	}
	hash := HashSecret(secret)
	if len(hash) != len(s.SecretHash) {
		return false
	}
	for i := range hash {
		if hash[i] != s.SecretHash[i] {
			return false
		}
	}
	return true
}

// TransitionTo attempts to transition the swap to a new state.
func (s *Swap) TransitionTo(newState State) error {
	// Define valid state transitions
	valid := map[State][]State{
		StateInit:      {StateFunding, StateCancelled},
		StateFunding:   {StateFunded, StateRefunded, StateFailed},
		StateFunded:    {StateRedeemed, StateRefunded},
		StateRedeemed:  {}, // Terminal state
		StateRefunded:  {}, // Terminal state
		StateFailed:    {}, // Terminal state
		StateCancelled: {}, // Terminal state
	}

	validTransitions, ok := valid[s.State]
	if !ok {
		return fmt.Errorf("%w: unknown current state %s", ErrInvalidState, s.State)
	}

	for _, validState := range validTransitions {
		if validState == newState {
			s.State = newState
			return nil
		}
	}

	return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidState, s.State, newState)
}

// IsTerminal returns true if the swap is in a terminal state.
func (s *Swap) IsTerminal() bool {
	switch s.State {
	case StateRedeemed, StateRefunded, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// InitiatorLockTime returns the absolute lock time for the initiator.
func (s *Swap) InitiatorLockTime() time.Time {
	return s.CreatedAt.Add(s.InitiatorLock)
}

// ResponderLockTime returns the absolute lock time for the responder.
func (s *Swap) ResponderLockTime() time.Time {
	return s.CreatedAt.Add(s.ResponderLock)
}

// CanRefund checks if we can refund based on the timelock.
func (s *Swap) CanRefund() bool {
	now := time.Now()
	if s.Role == RoleInitiator {
		return now.After(s.InitiatorLockTime())
	}
	return now.After(s.ResponderLockTime())
}

// =============================================================================
// Block-based Safety Margin Enforcement
// =============================================================================

// SetBlockHeights sets the starting block heights for both chains.
// This should be called when the swap is created.
func (s *Swap) SetBlockHeights(offerChainHeight, requestChainHeight uint32) {
	isTestnet := s.Network == chain.Testnet

	// Get timeout config for offer chain
	offerTimeout, _ := config.GetChainTimeout(s.Offer.OfferChain, isTestnet)
	requestTimeout, _ := config.GetChainTimeout(s.Offer.RequestChain, isTestnet)

	// Set starting heights
	s.OfferChainStartHeight = offerChainHeight
	s.RequestChainStartHeight = requestChainHeight

	// Calculate timeout heights based on role
	if s.Role == RoleInitiator {
		// Initiator (maker) has longer timeout
		s.OfferChainTimeoutHeight = offerChainHeight + offerTimeout.MakerBlocks
		s.RequestChainTimeoutHeight = requestChainHeight + requestTimeout.TakerBlocks
	} else {
		// Responder (taker) has shorter timeout
		s.OfferChainTimeoutHeight = offerChainHeight + offerTimeout.TakerBlocks
		s.RequestChainTimeoutHeight = requestChainHeight + requestTimeout.MakerBlocks
	}
}

// IsSafeToComplete checks if it's safe to complete the swap given current block heights.
// Returns nil if safe, or an error explaining why it's not safe.
//
// SECURITY: This prevents timeout race conditions where both claim and refund
// could potentially be valid if executed near the timeout boundary.
func (s *Swap) IsSafeToComplete(offerChainCurrentHeight, requestChainCurrentHeight uint32) error {
	isTestnet := s.Network == chain.Testnet

	// Check offer chain
	offerTimeout, ok := config.GetChainTimeout(s.Offer.OfferChain, isTestnet)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedChain, s.Offer.OfferChain)
	}
	if !config.IsSafeToComplete(offerChainCurrentHeight, s.OfferChainTimeoutHeight, offerTimeout.SafetyMarginBlocks) {
		blocksLeft := config.BlocksUntilTimeout(offerChainCurrentHeight, s.OfferChainTimeoutHeight)
		return fmt.Errorf("%w: %s chain has only %d blocks until timeout (need %d margin)",
			ErrTimeoutRace, s.Offer.OfferChain, blocksLeft, offerTimeout.SafetyMarginBlocks)
	}

	// Check request chain
	requestTimeout, ok := config.GetChainTimeout(s.Offer.RequestChain, isTestnet)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedChain, s.Offer.RequestChain)
	}
	if !config.IsSafeToComplete(requestChainCurrentHeight, s.RequestChainTimeoutHeight, requestTimeout.SafetyMarginBlocks) {
		blocksLeft := config.BlocksUntilTimeout(requestChainCurrentHeight, s.RequestChainTimeoutHeight)
		return fmt.Errorf("%w: %s chain has only %d blocks until timeout (need %d margin)",
			ErrTimeoutRace, s.Offer.RequestChain, blocksLeft, requestTimeout.SafetyMarginBlocks)
	}

	return nil
}

// CanRefundByBlock checks if we can refund based on block height (more precise than time).
func (s *Swap) CanRefundByBlock(offerChainCurrentHeight, requestChainCurrentHeight uint32) bool {
	if s.Role == RoleInitiator {
		// Initiator checks offer chain timeout
		return offerChainCurrentHeight >= s.OfferChainTimeoutHeight
	}
	// Responder checks request chain timeout
	return requestChainCurrentHeight >= s.RequestChainTimeoutHeight
}

// BlocksUntilRefund returns the number of blocks until we can refund.
// Returns 0 if we can already refund.
func (s *Swap) BlocksUntilRefund(currentHeight uint32) uint32 {
	var timeoutHeight uint32
	if s.Role == RoleInitiator {
		timeoutHeight = s.OfferChainTimeoutHeight
	} else {
		timeoutHeight = s.RequestChainTimeoutHeight
	}
	return config.BlocksUntilTimeout(currentHeight, timeoutHeight)
}

// =============================================================================
// Confirmation Tracking
// =============================================================================

// FundingStatus represents the confirmation status of a funding transaction.
type FundingStatus struct {
	TxID          string
	Confirmations uint32
	Required      uint32
	IsFinal       bool // True if confirmations >= required
}

// GetLocalFundingStatus returns the status of our funding transaction.
func (s *Swap) GetLocalFundingStatus() *FundingStatus {
	if s.LocalFundingTxID == "" {
		return nil
	}

	// Local chain depends on role: initiator funds offer chain, responder funds request chain
	var localChain string
	if s.Role == RoleInitiator {
		localChain = s.Offer.OfferChain
	} else {
		localChain = s.Offer.RequestChain
	}

	isTestnet := s.Network == chain.Testnet
	chainCfg, _ := config.GetChainTimeout(localChain, isTestnet)

	return &FundingStatus{
		TxID:          s.LocalFundingTxID,
		Confirmations: s.LocalFundingConfirms,
		Required:      chainCfg.MinConfirmations,
		IsFinal:       s.LocalFundingConfirms >= chainCfg.MinConfirmations,
	}
}

// GetRemoteFundingStatus returns the status of the counterparty's funding transaction.
func (s *Swap) GetRemoteFundingStatus() *FundingStatus {
	if s.RemoteFundingTxID == "" {
		return nil
	}

	// Remote chain depends on role: initiator's remote is request chain, responder's remote is offer chain
	var remoteChain string
	if s.Role == RoleInitiator {
		remoteChain = s.Offer.RequestChain
	} else {
		remoteChain = s.Offer.OfferChain
	}

	isTestnet := s.Network == chain.Testnet
	chainCfg, _ := config.GetChainTimeout(remoteChain, isTestnet)

	return &FundingStatus{
		TxID:          s.RemoteFundingTxID,
		Confirmations: s.RemoteFundingConfirms,
		Required:      chainCfg.MinConfirmations,
		IsFinal:       s.RemoteFundingConfirms >= chainCfg.MinConfirmations,
	}
}

// UpdateLocalConfirmations updates the confirmation count for our funding tx.
func (s *Swap) UpdateLocalConfirmations(confirmations uint32) {
	s.LocalFundingConfirms = confirmations
}

// UpdateRemoteConfirmations updates the confirmation count for counterparty's funding tx.
func (s *Swap) UpdateRemoteConfirmations(confirmations uint32) {
	s.RemoteFundingConfirms = confirmations
}

// IsFundingConfirmed returns true if both funding transactions have sufficient confirmations.
// SECURITY: This protects against reorg attacks by ensuring transactions are deep enough.
func (s *Swap) IsFundingConfirmed() bool {
	localStatus := s.GetLocalFundingStatus()
	remoteStatus := s.GetRemoteFundingStatus()

	if localStatus == nil || remoteStatus == nil {
		return false
	}

	return localStatus.IsFinal && remoteStatus.IsFinal
}

// CheckConfirmations validates that both funding transactions have sufficient confirmations.
// Returns nil if OK, or an error with details about insufficient confirmations.
func (s *Swap) CheckConfirmations() error {
	localStatus := s.GetLocalFundingStatus()
	if localStatus == nil {
		return fmt.Errorf("%w: local funding transaction not set", ErrInsufficientConfirmations)
	}
	if !localStatus.IsFinal {
		return fmt.Errorf("%w: local funding has %d/%d confirmations",
			ErrInsufficientConfirmations, localStatus.Confirmations, localStatus.Required)
	}

	remoteStatus := s.GetRemoteFundingStatus()
	if remoteStatus == nil {
		return fmt.Errorf("%w: remote funding transaction not set", ErrInsufficientConfirmations)
	}
	if !remoteStatus.IsFinal {
		return fmt.Errorf("%w: remote funding has %d/%d confirmations",
			ErrInsufficientConfirmations, remoteStatus.Confirmations, remoteStatus.Required)
	}

	return nil
}
