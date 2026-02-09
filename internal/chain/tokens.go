package chain

// TokenInfo contains information about an ERC-20 token on a specific chain.
type TokenInfo struct {
	Symbol   string // Token symbol (USDT, USDC, etc.)
	Name     string // Full name
	Decimals uint8  // Token decimals
	Address  string // Contract address on this chain
	ChainID  uint64 // EVM chain ID
}

// tokenRegistry maps chainID -> symbol -> TokenInfo
var tokenRegistry = make(map[uint64]map[string]*TokenInfo)

func init() {
	// ==========================================================================
	// Ethereum Mainnet (chainID 1)
	// ==========================================================================
	registerToken(1, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Address:  "0xdAC17F958D2ee523a2206206994597C13D831ec7",
		ChainID:  1,
	})
	registerToken(1, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		ChainID:  1,
	})
	registerToken(1, &TokenInfo{
		Symbol:   "WETH",
		Name:     "Wrapped Ether",
		Decimals: 18,
		Address:  "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		ChainID:  1,
	})
	registerToken(1, &TokenInfo{
		Symbol:   "WBTC",
		Name:     "Wrapped Bitcoin",
		Decimals: 8,
		Address:  "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599",
		ChainID:  1,
	})
	registerToken(1, &TokenInfo{
		Symbol:   "DAI",
		Name:     "Dai Stablecoin",
		Decimals: 18,
		Address:  "0x6B175474E89094C44Da98b954EescdeCB5BE3830",
		ChainID:  1,
	})

	// ==========================================================================
	// Arbitrum One (chainID 42161)
	// ==========================================================================
	registerToken(42161, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Address:  "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9",
		ChainID:  42161,
	})
	registerToken(42161, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		ChainID:  42161,
	})
	registerToken(42161, &TokenInfo{
		Symbol:   "WETH",
		Name:     "Wrapped Ether",
		Decimals: 18,
		Address:  "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1",
		ChainID:  42161,
	})
	registerToken(42161, &TokenInfo{
		Symbol:   "WBTC",
		Name:     "Wrapped Bitcoin",
		Decimals: 8,
		Address:  "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f",
		ChainID:  42161,
	})

	// ==========================================================================
	// Optimism (chainID 10)
	// ==========================================================================
	registerToken(10, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Address:  "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58",
		ChainID:  10,
	})
	registerToken(10, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85",
		ChainID:  10,
	})
	registerToken(10, &TokenInfo{
		Symbol:   "WETH",
		Name:     "Wrapped Ether",
		Decimals: 18,
		Address:  "0x4200000000000000000000000000000000000006",
		ChainID:  10,
	})

	// ==========================================================================
	// Base (chainID 8453)
	// ==========================================================================
	registerToken(8453, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		ChainID:  8453,
	})
	registerToken(8453, &TokenInfo{
		Symbol:   "WETH",
		Name:     "Wrapped Ether",
		Decimals: 18,
		Address:  "0x4200000000000000000000000000000000000006",
		ChainID:  8453,
	})

	// ==========================================================================
	// BNB Smart Chain (chainID 56)
	// ==========================================================================
	registerToken(56, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 18, // BSC USDT has 18 decimals
		Address:  "0x55d398326f99059fF775485246999027B3197955",
		ChainID:  56,
	})
	registerToken(56, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 18,
		Address:  "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d",
		ChainID:  56,
	})
	registerToken(56, &TokenInfo{
		Symbol:   "WBNB",
		Name:     "Wrapped BNB",
		Decimals: 18,
		Address:  "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		ChainID:  56,
	})

	// ==========================================================================
	// Polygon (chainID 137)
	// ==========================================================================
	registerToken(137, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Address:  "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
		ChainID:  137,
	})
	registerToken(137, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
		ChainID:  137,
	})
	registerToken(137, &TokenInfo{
		Symbol:   "WETH",
		Name:     "Wrapped Ether",
		Decimals: 18,
		Address:  "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619",
		ChainID:  137,
	})
	registerToken(137, &TokenInfo{
		Symbol:   "WPOL",
		Name:     "Wrapped POL",
		Decimals: 18,
		Address:  "0x0d500B1d8E8eF31E21C99d1Db9A6444d3ADf1270",
		ChainID:  137,
	})

	// ==========================================================================
	// Avalanche C-Chain (chainID 43114)
	// ==========================================================================
	registerToken(43114, &TokenInfo{
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Address:  "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7",
		ChainID:  43114,
	})
	registerToken(43114, &TokenInfo{
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Address:  "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E",
		ChainID:  43114,
	})
	registerToken(43114, &TokenInfo{
		Symbol:   "WAVAX",
		Name:     "Wrapped AVAX",
		Decimals: 18,
		Address:  "0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7",
		ChainID:  43114,
	})

	// ==========================================================================
	// TESTNETS
	// ==========================================================================

	// Sepolia Testnet (chainID 11155111)
	registerToken(11155111, &TokenInfo{
		Symbol:   "KGX",
		Name:     "Klingex",
		Decimals: 6,
		Address:  "0x805a55da2ff4c72ecabd4e496dd8ce4ea923f25d",
		ChainID:  11155111,
	})
}

func registerToken(chainID uint64, token *TokenInfo) {
	if tokenRegistry[chainID] == nil {
		tokenRegistry[chainID] = make(map[string]*TokenInfo)
	}
	tokenRegistry[chainID][token.Symbol] = token
}

// GetToken returns token info for a symbol on a specific chain.
// Returns nil if the token is not registered on that chain.
func GetToken(chainID uint64, symbol string) *TokenInfo {
	if tokens, ok := tokenRegistry[chainID]; ok {
		return tokens[symbol]
	}
	return nil
}

// GetTokenAddress returns the contract address for a token on a specific chain.
// Returns empty string if not found.
func GetTokenAddress(chainID uint64, symbol string) string {
	if token := GetToken(chainID, symbol); token != nil {
		return token.Address
	}
	return ""
}

// ListTokens returns all registered tokens for a specific chain.
func ListTokens(chainID uint64) []*TokenInfo {
	tokens, ok := tokenRegistry[chainID]
	if !ok {
		return nil
	}
	result := make([]*TokenInfo, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, token)
	}
	return result
}

// ListAllTokens returns all registered tokens across all chains.
func ListAllTokens() []*TokenInfo {
	var result []*TokenInfo
	for _, tokens := range tokenRegistry {
		for _, token := range tokens {
			result = append(result, token)
		}
	}
	return result
}

// IsTokenSupported checks if a token is supported on a specific chain.
func IsTokenSupported(chainID uint64, symbol string) bool {
	return GetToken(chainID, symbol) != nil
}

// GetTokenDecimals returns the decimals for a token on a specific chain.
// Returns 0 if not found.
func GetTokenDecimals(chainID uint64, symbol string) uint8 {
	if token := GetToken(chainID, symbol); token != nil {
		return token.Decimals
	}
	return 0
}
