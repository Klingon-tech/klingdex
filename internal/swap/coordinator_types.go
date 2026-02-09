// Package swap - Type definitions for the Coordinator.
package swap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/Klingon-tech/klingdex/internal/backend"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/internal/storage"
	"github.com/Klingon-tech/klingdex/internal/wallet"
	"github.com/Klingon-tech/klingdex/pkg/logging"
)

// Coordinator errors
var (
	ErrSwapNotFound     = errors.New("swap not found")
	ErrSwapExists       = errors.New("swap already exists")
	ErrNoWallet         = errors.New("wallet not available")
	ErrNoBackend        = errors.New("backend not available for chain")
	ErrAlreadyFunded    = errors.New("already funded")
	ErrNotReadyToSign   = errors.New("not ready to sign")
	ErrNotReadyToRedeem = errors.New("not ready to redeem")
)

// SwapEvent represents an event that occurred during a swap.
type SwapEvent struct {
	TradeID   string
	EventType string
	Data      interface{}
	Timestamp time.Time
}

// EventHandler is called when swap events occur.
type EventHandler func(event SwapEvent)

// ChainMuSig2Data holds MuSig2 data for a single chain in the swap.
type ChainMuSig2Data struct {
	Session          *MuSig2Session
	TaprootAddress   string
	LocalNonce       []byte
	RemoteNonce      []byte
	PartialSig       *musig2.PartialSignature
	RemotePartialSig *musig2.PartialSignature
}

// MuSig2SwapData holds MuSig2-specific data for a swap.
// Each chain has its own session because MuSig2 nonces can only be used once.
type MuSig2SwapData struct {
	LocalPrivKey *btcec.PrivateKey
	OfferChain   *ChainMuSig2Data
	RequestChain *ChainMuSig2Data
}

// ChainHTLCData holds HTLC data for a single chain in the swap.
type ChainHTLCData struct {
	Session     *HTLCSession
	HTLCAddress string // P2WSH address
	ClaimTxID   string // Claim transaction ID (after claiming)
	RefundTxID  string // Refund transaction ID (if refunded)
}

// HTLCSwapData holds HTLC-specific data for a swap.
type HTLCSwapData struct {
	LocalPrivKey *btcec.PrivateKey
	OfferChain   *ChainHTLCData
	RequestChain *ChainHTLCData
}

// ActiveSwap holds runtime data for an active swap.
type ActiveSwap struct {
	Swap       *Swap
	Trade      *storage.Trade
	OfferLeg   *storage.SwapLeg
	RequestLeg *storage.SwapLeg
	MuSig2     *MuSig2SwapData  // Populated for MuSig2 swaps
	HTLC       *HTLCSwapData    // Populated for Bitcoin HTLC swaps
	EVMHTLC    *EVMHTLCSwapData // Populated for EVM HTLC swaps
}

// IsHTLC returns true if this swap uses HTLC method (Bitcoin-family).
func (a *ActiveSwap) IsHTLC() bool {
	return a.Swap.Method == MethodHTLC && a.HTLC != nil
}

// IsMuSig2 returns true if this swap uses MuSig2 method.
func (a *ActiveSwap) IsMuSig2() bool {
	return a.Swap.Method == MethodMuSig2
}

// IsEVMHTLC returns true if this swap involves EVM chains.
func (a *ActiveSwap) IsEVMHTLC() bool {
	return a.EVMHTLC != nil
}

// IsCrossChain returns true if this is a cross-chain type swap (Bitcoin <-> EVM).
func (a *ActiveSwap) IsCrossChain() bool {
	return a.HTLC != nil && a.EVMHTLC != nil
}

// Coordinator manages active swaps.
type Coordinator struct {
	mu sync.RWMutex

	// Dependencies
	store         *storage.Storage
	wallet        *wallet.Wallet
	walletService *wallet.Service // For transaction building/signing
	backends      map[string]backend.Backend // chain symbol -> backend

	// Network
	network chain.Network

	// Active swaps (tradeID -> ActiveSwap)
	swaps map[string]*ActiveSwap

	// Event handlers
	eventHandlers []EventHandler

	// Logger
	log *logging.Logger

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc
}

// CoordinatorConfig holds configuration for the Coordinator.
type CoordinatorConfig struct {
	Store         *storage.Storage
	Wallet        *wallet.Wallet
	WalletService *wallet.Service // For transaction building/signing
	Backends      map[string]backend.Backend
	Network       chain.Network
}

// =============================================================================
// Storage Types
// =============================================================================

// ChainStorageData holds storage data for a single chain.
type ChainStorageData struct {
	Chain            string   `json:"chain"`
	TaprootAddress   string   `json:"taproot_address"`
	AggregatedPubKey string   `json:"aggregated_pubkey,omitempty"`
	LocalNonce       string   `json:"local_nonce,omitempty"`
	RemoteNonce      string   `json:"remote_nonce,omitempty"`
	PartialSig       string   `json:"partial_sig,omitempty"`
	RemotePartialSig string   `json:"remote_partial_sig,omitempty"`
	UsedNonces       []string `json:"used_nonces,omitempty"`
	NonceUsed        bool     `json:"nonce_used"`
	SessionInvalid   bool     `json:"session_invalid"`
	RefundScript     string   `json:"refund_script,omitempty"`
	TimeoutBlocks    uint32   `json:"timeout_blocks,omitempty"`
	ControlBlock     string   `json:"control_block,omitempty"`
}

// MuSig2StorageData is the JSON structure stored for swap recovery.
type MuSig2StorageData struct {
	LocalPubKey  string `json:"local_pubkey"`
	RemotePubKey string `json:"remote_pubkey"`
	LocalPrivKey string `json:"local_privkey,omitempty"`

	// Wallet addresses for redemption
	LocalOfferWalletAddr    string `json:"local_offer_wallet_addr,omitempty"`
	LocalRequestWalletAddr  string `json:"local_request_wallet_addr,omitempty"`
	RemoteOfferWalletAddr   string `json:"remote_offer_wallet_addr,omitempty"`
	RemoteRequestWalletAddr string `json:"remote_request_wallet_addr,omitempty"`

	OfferChain   *ChainStorageData `json:"offer_chain"`
	RequestChain *ChainStorageData `json:"request_chain"`
}

// CoordinatorHTLCStorageData is the storage format for HTLC swap state (coordinator level).
type CoordinatorHTLCStorageData struct {
	LocalPubKey  string `json:"local_pubkey"`
	RemotePubKey string `json:"remote_pubkey"`

	// Wallet addresses for redemption
	LocalOfferWalletAddr    string `json:"local_offer_wallet_addr,omitempty"`
	LocalRequestWalletAddr  string `json:"local_request_wallet_addr,omitempty"`
	RemoteOfferWalletAddr   string `json:"remote_offer_wallet_addr,omitempty"`
	RemoteRequestWalletAddr string `json:"remote_request_wallet_addr,omitempty"`

	// HTLC-specific fields
	Secret     string `json:"secret,omitempty"`      // Hex, only for initiator
	SecretHash string `json:"secret_hash,omitempty"` // Hex

	OfferChain   *HTLCChainStorageData `json:"offer_chain,omitempty"`
	RequestChain *HTLCChainStorageData `json:"request_chain,omitempty"`
}

// HTLCChainStorageData stores per-chain HTLC data.
type HTLCChainStorageData struct {
	Symbol      string `json:"symbol"`
	HTLCAddress string `json:"htlc_address"`
	SessionData string `json:"session_data,omitempty"` // JSON of HTLCSession
}

// =============================================================================
// EVM HTLC Storage Types
// =============================================================================

// CoordinatorEVMHTLCStorageData is the storage format for EVM HTLC swap state.
type CoordinatorEVMHTLCStorageData struct {
	// Wallet addresses for redemption
	LocalOfferWalletAddr    string `json:"local_offer_wallet_addr,omitempty"`
	LocalRequestWalletAddr  string `json:"local_request_wallet_addr,omitempty"`
	RemoteOfferWalletAddr   string `json:"remote_offer_wallet_addr,omitempty"`
	RemoteRequestWalletAddr string `json:"remote_request_wallet_addr,omitempty"`

	// HTLC-specific fields
	Secret     string `json:"secret,omitempty"`      // Hex, only for initiator
	SecretHash string `json:"secret_hash,omitempty"` // Hex

	OfferChain   *EVMHTLCChainStorageData `json:"offer_chain,omitempty"`
	RequestChain *EVMHTLCChainStorageData `json:"request_chain,omitempty"`
}

// EVMHTLCChainStorageData stores per-chain EVM HTLC data.
type EVMHTLCChainStorageData struct {
	Symbol          string `json:"symbol"`
	ChainID         uint64 `json:"chain_id"`
	ContractAddress string `json:"contract_address"`
	SwapID          string `json:"swap_id,omitempty"`     // Hex
	CreateTxHash    string `json:"create_tx_hash,omitempty"`
	ClaimTxHash     string `json:"claim_tx_hash,omitempty"`
	RefundTxHash    string `json:"refund_tx_hash,omitempty"`
	SessionData     string `json:"session_data,omitempty"` // JSON of EVMHTLCSession
}

// =============================================================================
// Timeout Types
// =============================================================================

// TimeoutCheckResult holds the result of checking a swap for timeout.
type TimeoutCheckResult struct {
	TradeID          string
	Chain            string
	CurrentHeight    uint32
	TimeoutHeight    uint32
	BlocksRemaining  int32 // Negative means timeout passed
	CanRefund        bool
	RefundBroadcast  bool
	RefundTxID       string
	Error            error
}
