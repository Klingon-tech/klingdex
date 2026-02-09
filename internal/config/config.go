// Package config provides centralized configuration for the Klingon exchange.
// ALL exchange parameters (coins, addresses, fees, timeouts) MUST be defined here.
// No hardcoded values should exist elsewhere in the codebase.
package config

import "time"

// =============================================================================
// Network Types
// =============================================================================

// NetworkType represents mainnet or testnet.
type NetworkType string

const (
	Mainnet NetworkType = "mainnet"
	Testnet NetworkType = "testnet"
)

// =============================================================================
// Coin Definitions
// =============================================================================

// CoinType represents the type/family of a coin.
type CoinType string

const (
	CoinTypeBitcoin CoinType = "bitcoin" // BTC and forks (LTC, DOGE)
	CoinTypeMonero  CoinType = "monero"  // XMR
	CoinTypeEVM     CoinType = "evm"     // ETH, BSC, POLYGON, ARBITRUM, etc.
	CoinTypeSolana  CoinType = "solana"  // SOL
)

// SwapMethod represents the atomic swap method supported by a coin.
type SwapMethod string

const (
	SwapMethodMuSig2   SwapMethod = "musig2"   // Taproot MuSig2 (preferred)
	SwapMethodHTLC     SwapMethod = "htlc"     // Hash Time-Locked Contract
	SwapMethodAdaptor  SwapMethod = "adaptor"  // Adaptor signatures (Monero)
	SwapMethodContract SwapMethod = "contract" // Smart contract (EVM)
	SwapMethodProgram  SwapMethod = "program"  // Solana program
)

// Coin represents a supported cryptocurrency.
type Coin struct {
	Symbol       string       // e.g., "BTC", "ETH"
	Name         string       // e.g., "Bitcoin", "Ethereum"
	Type         CoinType     // Coin family
	Decimals     uint8        // Decimal places (8 for BTC, 18 for ETH)
	SwapMethods  []SwapMethod // Supported swap methods in priority order
	MinAmount    uint64       // Minimum trade amount in smallest unit
	MaxAmount    uint64       // Maximum trade amount in smallest unit (0 = no limit)
}

// SupportedCoins defines all supported cryptocurrencies.
var SupportedCoins = map[string]Coin{
	// Bitcoin and forks
	"BTC": {
		Symbol:      "BTC",
		Name:        "Bitcoin",
		Type:        CoinTypeBitcoin,
		Decimals:    8,
		SwapMethods: []SwapMethod{SwapMethodMuSig2, SwapMethodHTLC}, // MuSig2 priority, HTLC fallback
		MinAmount:   10000,    // 0.0001 BTC
		MaxAmount:   100000000000, // 1000 BTC
	},
	"LTC": {
		Symbol:      "LTC",
		Name:        "Litecoin",
		Type:        CoinTypeBitcoin,
		Decimals:    8,
		SwapMethods: []SwapMethod{SwapMethodMuSig2, SwapMethodHTLC},
		MinAmount:   100000,   // 0.001 LTC
		MaxAmount:   0,        // No limit
	},
	"DOGE": {
		Symbol:      "DOGE",
		Name:        "Dogecoin",
		Type:        CoinTypeBitcoin,
		Decimals:    8,
		SwapMethods: []SwapMethod{SwapMethodHTLC},
		MinAmount:   100000000, // 1 DOGE
		MaxAmount:   0,
	},

	// Monero
	"XMR": {
		Symbol:      "XMR",
		Name:        "Monero",
		Type:        CoinTypeMonero,
		Decimals:    12,
		SwapMethods: []SwapMethod{SwapMethodAdaptor},
		MinAmount:   1000000000, // 0.001 XMR
		MaxAmount:   0,
	},

	// EVM chains
	"ETH": {
		Symbol:      "ETH",
		Name:        "Ethereum",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000, // 0.001 ETH
		MaxAmount:   0,
	},
	"BSC": {
		Symbol:      "BNB",
		Name:        "BNB Smart Chain",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000,
		MaxAmount:   0,
	},
	"POLYGON": {
		Symbol:      "POL",
		Name:        "Polygon",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000000, // 1 POL
		MaxAmount:   0,
	},
	"ARBITRUM": {
		Symbol:      "ETH",
		Name:        "Arbitrum One",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000,
		MaxAmount:   0,
	},
	"OPTIMISM": {
		Symbol:      "ETH",
		Name:        "Optimism",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000,
		MaxAmount:   0,
	},
	"BASE": {
		Symbol:      "ETH",
		Name:        "Base",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000,
		MaxAmount:   0,
	},
	"AVAX": {
		Symbol:      "AVAX",
		Name:        "Avalanche C-Chain",
		Type:        CoinTypeEVM,
		Decimals:    18,
		SwapMethods: []SwapMethod{SwapMethodContract},
		MinAmount:   1000000000000000,
		MaxAmount:   0,
	},

	// Solana
	"SOL": {
		Symbol:      "SOL",
		Name:        "Solana",
		Type:        CoinTypeSolana,
		Decimals:    9,
		SwapMethods: []SwapMethod{SwapMethodProgram},
		MinAmount:   10000000, // 0.01 SOL
		MaxAmount:   0,
	},
}

// =============================================================================
// Chain Parameters (Mainnet)
// =============================================================================

// ChainParams holds network-specific parameters for a coin.
type ChainParams struct {
	ChainID       uint64 // EVM chain ID (0 for non-EVM)
	RPCEndpoint   string // Default RPC endpoint
	ExplorerURL   string // Block explorer URL
	Confirmations uint32 // Required confirmations for finality
}

// MainnetChainParams contains mainnet parameters for each coin.
var MainnetChainParams = map[string]ChainParams{
	"BTC": {
		ChainID:       0,
		RPCEndpoint:   "", // User must configure
		ExplorerURL:   "https://blockstream.info",
		Confirmations: 3,
	},
	"LTC": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://blockchair.com/litecoin",
		Confirmations: 6,
	},
	"DOGE": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://blockchair.com/dogecoin",
		Confirmations: 6,
	},
	"XMR": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://xmrchain.net",
		Confirmations: 10,
	},
	"ETH": {
		ChainID:       1,
		RPCEndpoint:   "https://eth.llamarpc.com",
		ExplorerURL:   "https://etherscan.io",
		Confirmations: 12,
	},
	"BSC": {
		ChainID:       56,
		RPCEndpoint:   "https://bsc-dataseed.binance.org",
		ExplorerURL:   "https://bscscan.com",
		Confirmations: 15,
	},
	"POLYGON": {
		ChainID:       137,
		RPCEndpoint:   "https://polygon-rpc.com",
		ExplorerURL:   "https://polygonscan.com",
		Confirmations: 128,
	},
	"ARBITRUM": {
		ChainID:       42161,
		RPCEndpoint:   "https://arb1.arbitrum.io/rpc",
		ExplorerURL:   "https://arbiscan.io",
		Confirmations: 12,
	},
	"OPTIMISM": {
		ChainID:       10,
		RPCEndpoint:   "https://mainnet.optimism.io",
		ExplorerURL:   "https://optimistic.etherscan.io",
		Confirmations: 12,
	},
	"BASE": {
		ChainID:       8453,
		RPCEndpoint:   "https://mainnet.base.org",
		ExplorerURL:   "https://basescan.org",
		Confirmations: 12,
	},
	"AVAX": {
		ChainID:       43114,
		RPCEndpoint:   "https://api.avax.network/ext/bc/C/rpc",
		ExplorerURL:   "https://snowtrace.io",
		Confirmations: 12,
	},
	"SOL": {
		ChainID:       0,
		RPCEndpoint:   "https://api.mainnet-beta.solana.com",
		ExplorerURL:   "https://solscan.io",
		Confirmations: 32,
	},
}

// =============================================================================
// Chain Parameters (Testnet)
// =============================================================================

// TestnetChainParams contains testnet parameters for each coin.
var TestnetChainParams = map[string]ChainParams{
	"BTC": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://blockstream.info/testnet",
		Confirmations: 1,
	},
	"LTC": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://blockchair.com/litecoin/testnet",
		Confirmations: 1,
	},
	"DOGE": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://blockchair.com/dogecoin",
		Confirmations: 1,
	},
	"XMR": {
		ChainID:       0,
		RPCEndpoint:   "",
		ExplorerURL:   "https://stagenet.xmrchain.net",
		Confirmations: 1,
	},
	"ETH": {
		ChainID:       11155111, // Sepolia
		RPCEndpoint:   "https://rpc.sepolia.org",
		ExplorerURL:   "https://sepolia.etherscan.io",
		Confirmations: 2,
	},
	"BSC": {
		ChainID:       97, // BSC Testnet
		RPCEndpoint:   "https://data-seed-prebsc-1-s1.binance.org:8545",
		ExplorerURL:   "https://testnet.bscscan.com",
		Confirmations: 3,
	},
	"POLYGON": {
		ChainID:       80002, // Polygon Amoy
		RPCEndpoint:   "https://rpc-amoy.polygon.technology",
		ExplorerURL:   "https://amoy.polygonscan.com",
		Confirmations: 5,
	},
	"ARBITRUM": {
		ChainID:       421614, // Arbitrum Sepolia
		RPCEndpoint:   "https://sepolia-rollup.arbitrum.io/rpc",
		ExplorerURL:   "https://sepolia.arbiscan.io",
		Confirmations: 2,
	},
	"OPTIMISM": {
		ChainID:       11155420, // Optimism Sepolia
		RPCEndpoint:   "https://sepolia.optimism.io",
		ExplorerURL:   "https://sepolia-optimism.etherscan.io",
		Confirmations: 2,
	},
	"BASE": {
		ChainID:       84532, // Base Sepolia
		RPCEndpoint:   "https://sepolia.base.org",
		ExplorerURL:   "https://sepolia.basescan.org",
		Confirmations: 2,
	},
	"AVAX": {
		ChainID:       43113, // Avalanche Fuji
		RPCEndpoint:   "https://api.avax-test.network/ext/bc/C/rpc",
		ExplorerURL:   "https://testnet.snowtrace.io",
		Confirmations: 2,
	},
	"SOL": {
		ChainID:       0,
		RPCEndpoint:   "https://api.devnet.solana.com",
		ExplorerURL:   "https://solscan.io/?cluster=devnet",
		Confirmations: 1,
	},
}

// =============================================================================
// DAO Fee Addresses
// =============================================================================

// DAOAddresses holds fee collection addresses for each chain.
type DAOAddresses struct {
	BTC  string // Bitcoin address
	LTC  string // Litecoin address
	DOGE string // Dogecoin address
	XMR  string // Monero address
	EVM  string // EVM address (same for all EVM chains)
	SOL  string // Solana address
}

// MainnetDAOAddresses contains mainnet DAO fee addresses.
// TODO: Replace with actual mainnet addresses before production
var MainnetDAOAddresses = DAOAddresses{
	BTC:  "", // TODO: Set mainnet BTC DAO address
	LTC:  "", // TODO: Set mainnet LTC DAO address
	DOGE: "", // TODO: Set mainnet DOGE DAO address
	XMR:  "", // TODO: Set mainnet XMR DAO address
	EVM:  "", // TODO: Set mainnet EVM DAO address
	SOL:  "", // TODO: Set mainnet SOL DAO address
}

// TestnetDAOAddresses contains testnet DAO fee addresses.
// These are actual funded testnet addresses used for fee collection testing.
var TestnetDAOAddresses = DAOAddresses{
	BTC:  "tb1qtgy7uafh0eurh04eztdzjfn2ujn9ds6ls0zsk6",                                                       // Node 1 testnet BTC address
	LTC:  "tltc1qgmwwayvwdrj2kqt6gk7drwzh9ssmrysw8t90us",                                                     // Node 2 testnet LTC address
	DOGE: "noBHyVeJcEB7JMrV2CDKP4UKuQMGCmZspT",                                                               // Testnet placeholder
	XMR:  "9ujeXrjzf7bfeK3KZdCqnYaMwZVFuXemPU8Ubw335rj2FN1CdMiWNyFSe1bqSKbAWme1H6bFW8WMeaSzhoSv7YCK72E2sbb", // Stagenet placeholder
	EVM:  "0x37e565Bab0c11756806480102E09871f33403D8d",                                                       // Provided testnet address
	SOL:  "GsbwXfJraMomNxBcjYLcG3mxkBUiyWXAB32fGbSMQRdW",                                                     // Devnet placeholder
}

// =============================================================================
// Fee Configuration
// =============================================================================

// FeeConfig holds fee-related configuration.
type FeeConfig struct {
	// MakerFeeBPS is the maker fee in basis points (100 = 1%).
	MakerFeeBPS uint16

	// TakerFeeBPS is the taker fee in basis points (100 = 1%).
	TakerFeeBPS uint16

	// DAOShareBPS is the DAO's share of fees in basis points (5000 = 50%).
	DAOShareBPS uint16

	// NodeOperatorShareBPS is node operators' share of fees in basis points (5000 = 50%).
	NodeOperatorShareBPS uint16
}

// DefaultFeeConfig returns the default fee configuration.
// Maker: 0.2%, Taker: 0.2%, Split: 50% DAO / 50% Node Operators
func DefaultFeeConfig() FeeConfig {
	return FeeConfig{
		MakerFeeBPS:          20,   // 0.2%
		TakerFeeBPS:          20,   // 0.2%
		DAOShareBPS:          5000, // 50%
		NodeOperatorShareBPS: 5000, // 50%
	}
}

// TotalFeeBPS returns the total fee in basis points.
func (f FeeConfig) TotalFeeBPS() uint16 {
	return f.MakerFeeBPS + f.TakerFeeBPS
}

// CalculateFee calculates the fee amount for a given trade amount.
func (f FeeConfig) CalculateFee(amount uint64, isMaker bool) uint64 {
	var feeBPS uint16
	if isMaker {
		feeBPS = f.MakerFeeBPS
	} else {
		feeBPS = f.TakerFeeBPS
	}
	return (amount * uint64(feeBPS)) / 10000
}

// CalculateDAOShare calculates the DAO's share of a fee amount.
func (f FeeConfig) CalculateDAOShare(feeAmount uint64) uint64 {
	return (feeAmount * uint64(f.DAOShareBPS)) / 10000
}

// CalculateNodeOperatorShare calculates the node operators' share of a fee amount.
func (f FeeConfig) CalculateNodeOperatorShare(feeAmount uint64) uint64 {
	return (feeAmount * uint64(f.NodeOperatorShareBPS)) / 10000
}

// =============================================================================
// Atomic Swap Configuration
// =============================================================================

// SwapConfig holds atomic swap timing and security parameters.
type SwapConfig struct {
	// InitiatorLockTime is how long the initiator's funds are locked.
	// Must be longer than ResponderLockTime to ensure safety.
	InitiatorLockTime time.Duration

	// ResponderLockTime is how long the responder's funds are locked.
	ResponderLockTime time.Duration

	// MinLockTimeDelta is the minimum difference between lock times.
	MinLockTimeDelta time.Duration

	// SecretSize is the size of the swap secret in bytes.
	SecretSize int

	// MaxSwapDuration is the maximum time a swap can be active.
	MaxSwapDuration time.Duration
}

// DefaultSwapConfig returns the default swap configuration.
func DefaultSwapConfig() SwapConfig {
	return SwapConfig{
		InitiatorLockTime: 48 * time.Hour, // 48 hours
		ResponderLockTime: 24 * time.Hour, // 24 hours
		MinLockTimeDelta:  12 * time.Hour, // 12 hours minimum difference
		SecretSize:        32,              // 32 bytes (256 bits)
		MaxSwapDuration:   72 * time.Hour, // 72 hours max
	}
}

// =============================================================================
// Chain Timeout Configuration (for Atomic Swaps)
// =============================================================================

// ChainTimeoutConfig holds chain-specific timeout parameters for atomic swaps.
// These are specified in blocks, not time, for precision.
type ChainTimeoutConfig struct {
	// MakerBlocks is the timeout for maker (initiator) in blocks.
	// The maker's funds are locked longer to ensure the taker can claim.
	MakerBlocks uint32

	// TakerBlocks is the timeout for taker (responder) in blocks.
	// Must be shorter than MakerBlocks.
	TakerBlocks uint32

	// SafetyMarginBlocks is the number of blocks before timeout to stop accepting new operations.
	// This prevents timeout race conditions where both claim and refund could be valid.
	SafetyMarginBlocks uint32

	// MinConfirmations is the minimum confirmations required before proceeding.
	// This protects against reorg attacks.
	MinConfirmations uint32

	// AvgBlockTimeSeconds is the average block time for this chain.
	// Used for time-based estimations.
	AvgBlockTimeSeconds uint32
}

// ChainTimeouts defines chain-specific timeout configurations.
// SECURITY: These values are critical for atomic swap safety.
var ChainTimeouts = map[string]ChainTimeoutConfig{
	"BTC": {
		MakerBlocks:         144,  // ~24 hours at 10 min/block
		TakerBlocks:         72,   // ~12 hours
		SafetyMarginBlocks:  6,    // ~1 hour before timeout (stop accepting)
		MinConfirmations:    3,    // Mainnet: 3 confirmations
		AvgBlockTimeSeconds: 600,  // 10 minutes
	},
	"LTC": {
		MakerBlocks:         576,  // ~24 hours at 2.5 min/block
		TakerBlocks:         288,  // ~12 hours
		SafetyMarginBlocks:  24,   // ~1 hour before timeout
		MinConfirmations:    6,    // Mainnet: 6 confirmations
		AvgBlockTimeSeconds: 150,  // 2.5 minutes
	},
	"DOGE": {
		MakerBlocks:         1440, // ~24 hours at 1 min/block
		TakerBlocks:         720,  // ~12 hours
		SafetyMarginBlocks:  60,   // ~1 hour before timeout
		MinConfirmations:    6,    // Mainnet: 6 confirmations
		AvgBlockTimeSeconds: 60,   // 1 minute
	},
}

// TestnetChainTimeouts defines chain-specific timeout configurations for testnet.
// SECURITY: Testnet uses lower values for faster testing but must still allow
// enough time for the complete swap flow (fund -> confirm -> sign -> redeem).
var TestnetChainTimeouts = map[string]ChainTimeoutConfig{
	"BTC": {
		MakerBlocks:         72,   // ~12 hours at 10 min/block (testnet can vary)
		TakerBlocks:         36,   // ~6 hours
		SafetyMarginBlocks:  6,    // ~1 hour before timeout
		MinConfirmations:    0,    // Testnet: 0 confirmations for faster testing
		AvgBlockTimeSeconds: 600,
	},
	"LTC": {
		MakerBlocks:         288,  // ~12 hours at 2.5 min/block
		TakerBlocks:         144,  // ~6 hours
		SafetyMarginBlocks:  24,   // ~1 hour before timeout
		MinConfirmations:    0,    // Testnet: 0 confirmations for faster testing
		AvgBlockTimeSeconds: 150,
	},
	"DOGE": {
		MakerBlocks:         720,  // ~12 hours at 1 min/block
		TakerBlocks:         360,  // ~6 hours
		SafetyMarginBlocks:  60,   // ~1 hour before timeout
		MinConfirmations:    0,    // Testnet: 0 confirmations for faster testing
		AvgBlockTimeSeconds: 60,
	},
}

// GetChainTimeout returns the timeout configuration for a chain.
func GetChainTimeout(symbol string, isTestnet bool) (ChainTimeoutConfig, bool) {
	if isTestnet {
		cfg, ok := TestnetChainTimeouts[symbol]
		return cfg, ok
	}
	cfg, ok := ChainTimeouts[symbol]
	return cfg, ok
}

// IsSafeToComplete checks if it's safe to complete an operation given the current height.
// Returns true if there's enough time (blocks) before the timeout.
//
// SECURITY: This prevents timeout race conditions where both claim and refund
// could potentially be valid if executed near the timeout boundary.
func IsSafeToComplete(currentHeight, timeoutHeight uint32, safetyMargin uint32) bool {
	if currentHeight >= timeoutHeight {
		return false // Already past timeout
	}
	return currentHeight+safetyMargin < timeoutHeight
}

// BlocksUntilTimeout returns the number of blocks until timeout.
// Returns 0 if already past timeout.
func BlocksUntilTimeout(currentHeight, timeoutHeight uint32) uint32 {
	if currentHeight >= timeoutHeight {
		return 0
	}
	return timeoutHeight - currentHeight
}

// EstimateTimeUntilTimeout estimates the time until timeout based on block time.
func EstimateTimeUntilTimeout(currentHeight, timeoutHeight uint32, avgBlockTimeSeconds uint32) time.Duration {
	blocks := BlocksUntilTimeout(currentHeight, timeoutHeight)
	return time.Duration(blocks*avgBlockTimeSeconds) * time.Second
}

// =============================================================================
// Exchange Configuration
// =============================================================================

// ExchangeConfig holds all exchange configuration.
type ExchangeConfig struct {
	Network     NetworkType
	Fees        FeeConfig
	Swap        SwapConfig
	DAOAddrs    DAOAddresses
	ChainParams map[string]ChainParams
}

// NewExchangeConfig creates a new exchange configuration for the given network.
func NewExchangeConfig(network NetworkType) *ExchangeConfig {
	cfg := &ExchangeConfig{
		Network: network,
		Fees:    DefaultFeeConfig(),
		Swap:    DefaultSwapConfig(),
	}

	if network == Testnet {
		cfg.DAOAddrs = TestnetDAOAddresses
		cfg.ChainParams = TestnetChainParams
	} else {
		cfg.DAOAddrs = MainnetDAOAddresses
		cfg.ChainParams = MainnetChainParams
	}

	return cfg
}

// GetCoin returns the coin configuration for a given symbol.
func GetCoin(symbol string) (Coin, bool) {
	coin, ok := SupportedCoins[symbol]
	return coin, ok
}

// GetDAOAddress returns the DAO address for a given coin symbol.
func (c *ExchangeConfig) GetDAOAddress(symbol string) string {
	coin, ok := SupportedCoins[symbol]
	if !ok {
		return ""
	}

	switch coin.Type {
	case CoinTypeBitcoin:
		switch symbol {
		case "BTC":
			return c.DAOAddrs.BTC
		case "LTC":
			return c.DAOAddrs.LTC
		case "DOGE":
			return c.DAOAddrs.DOGE
		}
	case CoinTypeMonero:
		return c.DAOAddrs.XMR
	case CoinTypeEVM:
		return c.DAOAddrs.EVM
	case CoinTypeSolana:
		return c.DAOAddrs.SOL
	}

	return ""
}

// GetChainParams returns the chain parameters for a given coin symbol.
func (c *ExchangeConfig) GetChainParams(symbol string) (ChainParams, bool) {
	params, ok := c.ChainParams[symbol]
	return params, ok
}

// IsCoinSupported returns true if the coin is supported.
func IsCoinSupported(symbol string) bool {
	_, ok := SupportedCoins[symbol]
	return ok
}

// GetPreferredSwapMethod returns the preferred swap method for a coin.
func GetPreferredSwapMethod(symbol string) SwapMethod {
	coin, ok := SupportedCoins[symbol]
	if !ok || len(coin.SwapMethods) == 0 {
		return ""
	}
	return coin.SwapMethods[0]
}

// SupportsSwapMethod returns true if the coin supports the given swap method.
func SupportsSwapMethod(symbol string, method SwapMethod) bool {
	coin, ok := SupportedCoins[symbol]
	if !ok {
		return false
	}
	for _, m := range coin.SwapMethods {
		if m == method {
			return true
		}
	}
	return false
}

// ListSupportedCoins returns a list of all supported coin symbols.
func ListSupportedCoins() []string {
	coins := make([]string, 0, len(SupportedCoins))
	for symbol := range SupportedCoins {
		coins = append(coins, symbol)
	}
	return coins
}

// ListCoinsByType returns a list of coins of a specific type.
func ListCoinsByType(coinType CoinType) []string {
	var coins []string
	for symbol, coin := range SupportedCoins {
		if coin.Type == coinType {
			coins = append(coins, symbol)
		}
	}
	return coins
}
