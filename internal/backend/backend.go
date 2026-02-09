// Package backend provides blockchain API interfaces for fetching data and broadcasting transactions.
// This package is read-only for private keys - all signing happens in the wallet package.
package backend

import (
	"context"
	"errors"

	"github.com/Klingon-tech/klingdex/internal/chain"
)

// Common errors
var (
	ErrNotConnected       = errors.New("backend not connected")
	ErrTxNotFound         = errors.New("transaction not found")
	ErrAddressNotFound    = errors.New("address not found")
	ErrInvalidTx          = errors.New("invalid transaction")
	ErrBroadcastFailed    = errors.New("broadcast failed")
	ErrRateLimited        = errors.New("rate limited")
	ErrUnsupportedBackend = errors.New("unsupported backend type")
)

// Type represents the backend type.
type Type string

const (
	TypeMempool   Type = "mempool"   // mempool.space API
	TypeEsplora   Type = "esplora"   // blockstream.info API
	TypeElectrum  Type = "electrum"  // Electrum protocol
	TypeBlockbook Type = "blockbook" // Trezor Blockbook
	TypeJSONRPC   Type = "jsonrpc"   // Direct node RPC
)

// UTXO represents an unspent transaction output.
type UTXO struct {
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Amount        uint64 `json:"value"`        // in smallest unit (satoshis)
	ScriptPubKey  string `json:"scriptpubkey"` // hex encoded
	Confirmations int64  `json:"confirmations"`
	BlockHeight   int64  `json:"block_height,omitempty"`
}

// Transaction represents a transaction.
type Transaction struct {
	TxID          string     `json:"txid"`
	Version       int32      `json:"version"`
	Size          int64      `json:"size"`
	VSize         int64      `json:"vsize"` // Virtual size (for SegWit)
	Weight        int64      `json:"weight"`
	LockTime      uint32     `json:"locktime"`
	Fee           uint64     `json:"fee"`
	Confirmed     bool       `json:"confirmed"`
	BlockHash     string     `json:"block_hash,omitempty"`
	BlockHeight   int64      `json:"block_height,omitempty"`
	BlockTime     int64      `json:"block_time,omitempty"`
	Confirmations int64      `json:"confirmations"`
	Inputs        []TxInput  `json:"vin"`
	Outputs       []TxOutput `json:"vout"`
	Hex           string     `json:"hex,omitempty"`
}

// TxInput represents a transaction input.
type TxInput struct {
	TxID         string    `json:"txid"`
	Vout         uint32    `json:"vout"`
	ScriptSig    string    `json:"scriptsig,omitempty"`
	ScriptSigAsm string    `json:"scriptsig_asm,omitempty"`
	Witness      []string  `json:"witness,omitempty"`
	Sequence     uint32    `json:"sequence"`
	PrevOut      *TxOutput `json:"prevout,omitempty"` // Previous output being spent
}

// TxOutput represents a transaction output.
type TxOutput struct {
	ScriptPubKey     string `json:"scriptpubkey"`
	ScriptPubKeyAsm  string `json:"scriptpubkey_asm,omitempty"`
	ScriptPubKeyType string `json:"scriptpubkey_type,omitempty"`
	ScriptPubKeyAddr string `json:"scriptpubkey_address,omitempty"`
	Value            uint64 `json:"value"`
}

// AddressInfo contains address balance and transaction info.
type AddressInfo struct {
	Address        string `json:"address"`
	TxCount        int64  `json:"tx_count"`
	FundedTxCount  int64  `json:"funded_txo_count"`
	SpentTxCount   int64  `json:"spent_txo_count"`
	FundedSum      uint64 `json:"funded_txo_sum"`
	SpentSum       uint64 `json:"spent_txo_sum"`
	Balance        uint64 `json:"balance"`         // confirmed
	MempoolBalance int64  `json:"mempool_balance"` // unconfirmed delta
}

// BlockHeader contains block header info.
type BlockHeader struct {
	Hash         string  `json:"hash"`
	Height       int64   `json:"height"`
	Version      int32   `json:"version"`
	PreviousHash string  `json:"previousblockhash"`
	MerkleRoot   string  `json:"merkle_root"`
	Timestamp    int64   `json:"timestamp"`
	Bits         uint32  `json:"bits"`
	Nonce        uint32  `json:"nonce"`
	Difficulty   float64 `json:"difficulty"`
	TxCount      int64   `json:"tx_count"`
}

// FeeEstimate contains fee estimation for different confirmation targets.
type FeeEstimate struct {
	FastestFee  uint64 `json:"fastest_fee"`   // sat/vB for next block
	HalfHourFee uint64 `json:"half_hour_fee"` // sat/vB for ~30 min
	HourFee     uint64 `json:"hour_fee"`      // sat/vB for ~1 hour
	EconomyFee  uint64 `json:"economy_fee"`   // sat/vB for low priority
	MinimumFee  uint64 `json:"minimum_fee"`   // sat/vB minimum relay fee
}

// Backend defines the interface for blockchain data providers.
// All methods are read-only - no private keys are handled here.
type Backend interface {
	// Type returns the backend type (mempool, esplora, etc.)
	Type() Type

	// Connect establishes connection to the backend.
	Connect(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// IsConnected returns true if connected.
	IsConnected() bool

	// Address operations
	GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error)
	GetAddressUTXOs(ctx context.Context, address string) ([]UTXO, error)
	GetAddressTxs(ctx context.Context, address string, lastSeenTxID string) ([]Transaction, error)

	// Transaction operations
	GetTransaction(ctx context.Context, txID string) (*Transaction, error)
	GetRawTransaction(ctx context.Context, txID string) ([]byte, error)
	BroadcastTransaction(ctx context.Context, rawTxHex string) (string, error)

	// Block operations
	GetBlockHeight(ctx context.Context) (int64, error)
	GetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error)

	// Fee estimation
	GetFeeEstimates(ctx context.Context) (*FeeEstimate, error)
}

// Config contains backend configuration.
type Config struct {
	Type       Type    `yaml:"type"`
	MainnetURL string  `yaml:"mainnet"`
	TestnetURL string  `yaml:"testnet"`
	RPCType    RPCType `yaml:"rpc_type,omitempty"` // For JSON-RPC: "bitcoin" or "evm"

	// For Electrum
	Servers []string `yaml:"servers,omitempty"`

	// For JSON-RPC (direct node)
	RPCUser string `yaml:"rpc_user,omitempty"`
	RPCPass string `yaml:"rpc_pass,omitempty"`

	// Optional settings
	Timeout int `yaml:"timeout,omitempty"` // seconds, default 30
}

// DefaultConfigs returns default backend configurations for all supported chains.
func DefaultConfigs() map[string]*Config {
	return map[string]*Config{
		"BTC": {
			Type:       TypeMempool,
			MainnetURL: "https://mempool.space/api",
			TestnetURL: "https://mempool.space/testnet4/api",
		},
		"LTC": {
			Type:       TypeMempool,
			MainnetURL: "https://litecoinspace.org/api",
			TestnetURL: "https://litecoinspace.org/testnet/api",
		},
		"DOGE": {
			Type:       TypeBlockbook,
			MainnetURL: "https://doge1.trezor.io/api/v2",
			TestnetURL: "https://doge1.trezor.io/api/v2", // No public testnet
		},
		"ETH": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://eth.llamarpc.com",
			TestnetURL: "https://ethereum-sepolia-rpc.publicnode.com",
		},
		"BSC": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://bsc-dataseed.binance.org",
			TestnetURL: "https://data-seed-prebsc-1-s1.binance.org:8545",
		},
		"POLYGON": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://polygon-rpc.com",
			TestnetURL: "https://rpc-amoy.polygon.technology",
		},
		"ARBITRUM": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://arb1.arbitrum.io/rpc",
			TestnetURL: "https://sepolia-rollup.arbitrum.io/rpc",
		},
		"OPTIMISM": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://mainnet.optimism.io",
			TestnetURL: "https://sepolia.optimism.io",
		},
		"BASE": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://mainnet.base.org",
			TestnetURL: "https://sepolia.base.org",
		},
		"AVAX": {
			Type:       TypeJSONRPC,
			RPCType:    RPCTypeEVM,
			MainnetURL: "https://api.avax.network/ext/bc/C/rpc",
			TestnetURL: "https://api.avax-test.network/ext/bc/C/rpc",
		},
		"SOL": {
			Type:       TypeJSONRPC,
			RPCType:    "", // Solana has its own RPC format
			MainnetURL: "https://api.mainnet-beta.solana.com",
			TestnetURL: "https://api.devnet.solana.com",
		},
		"XMR": {
			Type:       TypeJSONRPC,
			RPCType:    "", // Monero has its own RPC format
			MainnetURL: "https://node.moneroworld.com:18089",
			TestnetURL: "https://stagenet.xmr.ditatompel.com",
		},
	}
}

// Registry holds backend instances by chain symbol.
type Registry struct {
	backends map[string]Backend
}

// NewRegistry creates a new backend registry.
func NewRegistry() *Registry {
	return &Registry{
		backends: make(map[string]Backend),
	}
}

// NewDefaultRegistry creates a registry with default backends for the given network.
func NewDefaultRegistry(network chain.Network) *Registry {
	r := NewRegistry()
	configs := DefaultConfigs()

	for symbol, cfg := range configs {
		var url string
		if network == chain.Testnet {
			url = cfg.TestnetURL
		} else {
			url = cfg.MainnetURL
		}

		if url == "" {
			continue
		}

		switch cfg.Type {
		case TypeMempool:
			r.Register(symbol, NewMempoolBackend(url))
		case TypeEsplora:
			r.Register(symbol, NewEsploraBackend(url))
		case TypeBlockbook:
			r.Register(symbol, NewBlockbookBackend(url))
		case TypeJSONRPC:
			// Only register if RPCType is specified (EVM chains)
			if cfg.RPCType != "" {
				r.Register(symbol, NewJSONRPCBackend(url, cfg.RPCType, cfg.RPCUser, cfg.RPCPass))
			}
			// Skip SOL/XMR for now - they need specialized implementations
		}
	}

	return r
}

// Register adds a backend to the registry.
func (r *Registry) Register(symbol string, backend Backend) {
	r.backends[symbol] = backend
}

// Get returns a backend by symbol.
func (r *Registry) Get(symbol string) (Backend, bool) {
	b, ok := r.backends[symbol]
	return b, ok
}

// List returns all registered symbols.
func (r *Registry) List() []string {
	symbols := make([]string, 0, len(r.backends))
	for s := range r.backends {
		symbols = append(symbols, s)
	}
	return symbols
}

// ConnectAll connects all registered backends.
func (r *Registry) ConnectAll(ctx context.Context) error {
	for _, b := range r.backends {
		if err := b.Connect(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CloseAll closes all registered backends.
func (r *Registry) CloseAll() {
	for _, b := range r.backends {
		b.Close()
	}
}

// All returns all backends as a map.
func (r *Registry) All() map[string]Backend {
	return r.backends
}
