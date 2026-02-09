package chain

import (
	"testing"
)

func TestAllChainsRegistered(t *testing.T) {
	expectedChains := []string{"BTC", "LTC", "DOGE", "ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "SOL", "XMR"}

	for _, symbol := range expectedChains {
		if !IsSupported(symbol) {
			t.Errorf("expected %s to be registered", symbol)
		}
	}
}

func TestBitcoinMainnet(t *testing.T) {
	params, ok := Get("BTC", Mainnet)
	if !ok {
		t.Fatal("BTC mainnet should be registered")
	}

	if params.Symbol != "BTC" {
		t.Errorf("Symbol = %s, want BTC", params.Symbol)
	}
	if params.Type != ChainTypeBitcoin {
		t.Errorf("Type = %s, want bitcoin", params.Type)
	}
	if params.Decimals != 8 {
		t.Errorf("Decimals = %d, want 8", params.Decimals)
	}
	if params.CoinType != 0 {
		t.Errorf("CoinType = %d, want 0", params.CoinType)
	}
	if params.DefaultPurpose != 84 {
		t.Errorf("DefaultPurpose = %d, want 84 (SegWit)", params.DefaultPurpose)
	}
	if params.Bech32HRP != "bc" {
		t.Errorf("Bech32HRP = %s, want bc", params.Bech32HRP)
	}
	if !params.SupportsSegWit {
		t.Error("BTC should support SegWit")
	}
	if !params.SupportsTaproot {
		t.Error("BTC should support Taproot")
	}
	if params.DefaultAddressType != AddressP2WPKH {
		t.Errorf("DefaultAddressType = %s, want p2wpkh", params.DefaultAddressType)
	}
}

func TestBitcoinTestnet(t *testing.T) {
	params, ok := Get("BTC", Testnet)
	if !ok {
		t.Fatal("BTC testnet should be registered")
	}

	if params.CoinType != 1 {
		t.Errorf("Testnet CoinType = %d, want 1", params.CoinType)
	}
	if params.Bech32HRP != "tb" {
		t.Errorf("Bech32HRP = %s, want tb", params.Bech32HRP)
	}
}

func TestLitecoinMainnet(t *testing.T) {
	params, ok := Get("LTC", Mainnet)
	if !ok {
		t.Fatal("LTC mainnet should be registered")
	}

	if params.CoinType != 2 {
		t.Errorf("CoinType = %d, want 2", params.CoinType)
	}
	if params.Bech32HRP != "ltc" {
		t.Errorf("Bech32HRP = %s, want ltc", params.Bech32HRP)
	}
	if !params.SupportsSegWit {
		t.Error("LTC should support SegWit")
	}
}

func TestDogecoinNoSegWit(t *testing.T) {
	params, ok := Get("DOGE", Mainnet)
	if !ok {
		t.Fatal("DOGE mainnet should be registered")
	}

	if params.CoinType != 3 {
		t.Errorf("CoinType = %d, want 3", params.CoinType)
	}
	if params.SupportsSegWit {
		t.Error("DOGE should NOT support SegWit")
	}
	if params.PubKeyHashAddrID != 0x1E {
		t.Errorf("PubKeyHashAddrID = 0x%X, want 0x1E", params.PubKeyHashAddrID)
	}
}

func TestEthereumMainnet(t *testing.T) {
	params, ok := Get("ETH", Mainnet)
	if !ok {
		t.Fatal("ETH mainnet should be registered")
	}

	if params.Type != ChainTypeEVM {
		t.Errorf("Type = %s, want evm", params.Type)
	}
	if params.CoinType != 60 {
		t.Errorf("CoinType = %d, want 60", params.CoinType)
	}
	if params.ChainID != 1 {
		t.Errorf("ChainID = %d, want 1", params.ChainID)
	}
	if params.Decimals != 18 {
		t.Errorf("Decimals = %d, want 18", params.Decimals)
	}
	if params.DefaultAddressType != AddressEVM {
		t.Errorf("DefaultAddressType = %s, want evm", params.DefaultAddressType)
	}
}

func TestEthereumTestnet(t *testing.T) {
	params, ok := Get("ETH", Testnet)
	if !ok {
		t.Fatal("ETH testnet should be registered")
	}

	if params.ChainID != 11155111 {
		t.Errorf("ChainID = %d, want 11155111 (Sepolia)", params.ChainID)
	}
}

func TestEVMChains(t *testing.T) {
	evmChains := []struct {
		symbol      string
		chainID     uint64
		nativeToken string
	}{
		{"ETH", 1, "ETH"},
		{"BSC", 56, "BNB"},
		{"POLYGON", 137, "POL"},
		{"ARBITRUM", 42161, "ETH"},
		{"OPTIMISM", 10, "ETH"},
		{"BASE", 8453, "ETH"},
		{"AVAX", 43114, "AVAX"},
	}

	for _, tc := range evmChains {
		params, ok := Get(tc.symbol, Mainnet)
		if !ok {
			t.Errorf("%s mainnet should be registered", tc.symbol)
			continue
		}
		if params.ChainID != tc.chainID {
			t.Errorf("%s ChainID = %d, want %d", tc.symbol, params.ChainID, tc.chainID)
		}
		if params.Type != ChainTypeEVM {
			t.Errorf("%s Type = %s, want evm", tc.symbol, params.Type)
		}
		if params.GetNativeToken() != tc.nativeToken {
			t.Errorf("%s NativeToken = %s, want %s", tc.symbol, params.GetNativeToken(), tc.nativeToken)
		}
	}
}

func TestSolanaMainnet(t *testing.T) {
	params, ok := Get("SOL", Mainnet)
	if !ok {
		t.Fatal("SOL mainnet should be registered")
	}

	if params.Type != ChainTypeSolana {
		t.Errorf("Type = %s, want solana", params.Type)
	}
	if params.CoinType != 501 {
		t.Errorf("CoinType = %d, want 501", params.CoinType)
	}
	if params.Decimals != 9 {
		t.Errorf("Decimals = %d, want 9", params.Decimals)
	}
}

func TestMoneroMainnet(t *testing.T) {
	params, ok := Get("XMR", Mainnet)
	if !ok {
		t.Fatal("XMR mainnet should be registered")
	}

	if params.Type != ChainTypeMonero {
		t.Errorf("Type = %s, want monero", params.Type)
	}
	if params.CoinType != 128 {
		t.Errorf("CoinType = %d, want 128", params.CoinType)
	}
	if params.Decimals != 12 {
		t.Errorf("Decimals = %d, want 12", params.Decimals)
	}
}

func TestDerivationPath(t *testing.T) {
	params, _ := Get("BTC", Mainnet)

	// Test m/84'/0'/0'/0/0
	path := params.DerivationPath(0, 0, 0)
	expected := []uint32{
		84 + 0x80000000,  // 84'
		0 + 0x80000000,   // 0'
		0 + 0x80000000,   // 0'
		0,                // 0
		0,                // 0
	}

	if len(path) != len(expected) {
		t.Fatalf("path length = %d, want %d", len(path), len(expected))
	}

	for i, v := range expected {
		if path[i] != v {
			t.Errorf("path[%d] = %d, want %d", i, path[i], v)
		}
	}
}

func TestDerivationPathString(t *testing.T) {
	tests := []struct {
		symbol   string
		network  Network
		account  uint32
		change   uint32
		index    uint32
		expected string
	}{
		{"BTC", Mainnet, 0, 0, 0, "m/84'/0'/0'/0/0"},
		{"BTC", Mainnet, 0, 0, 5, "m/84'/0'/0'/0/5"},
		{"BTC", Mainnet, 1, 0, 0, "m/84'/0'/1'/0/0"},
		{"BTC", Mainnet, 0, 1, 0, "m/84'/0'/0'/1/0"},
		{"BTC", Testnet, 0, 0, 0, "m/84'/1'/0'/0/0"},
		{"ETH", Mainnet, 0, 0, 0, "m/44'/60'/0'/0/0"},
		{"LTC", Mainnet, 0, 0, 0, "m/84'/2'/0'/0/0"},
		{"DOGE", Mainnet, 0, 0, 0, "m/44'/3'/0'/0/0"},
		{"SOL", Mainnet, 0, 0, 0, "m/44'/501'/0'/0/0"},
	}

	for _, tc := range tests {
		params, ok := Get(tc.symbol, tc.network)
		if !ok {
			t.Errorf("%s %s not registered", tc.symbol, tc.network)
			continue
		}

		path := params.DerivationPathString(tc.account, tc.change, tc.index)
		if path != tc.expected {
			t.Errorf("%s %s: path = %s, want %s", tc.symbol, tc.network, path, tc.expected)
		}
	}
}

func TestListChains(t *testing.T) {
	chains := List()
	if len(chains) < 12 {
		t.Errorf("expected at least 12 chains, got %d", len(chains))
	}
}

func TestListByType(t *testing.T) {
	// Bitcoin-type chains
	btcChains := ListByType(ChainTypeBitcoin)
	if len(btcChains) != 3 {
		t.Errorf("expected 3 bitcoin-type chains, got %d: %v", len(btcChains), btcChains)
	}

	// EVM chains
	evmChains := ListByType(ChainTypeEVM)
	if len(evmChains) != 7 {
		t.Errorf("expected 7 evm-type chains, got %d: %v", len(evmChains), evmChains)
	}

	// Monero
	xmrChains := ListByType(ChainTypeMonero)
	if len(xmrChains) != 1 {
		t.Errorf("expected 1 monero-type chain, got %d", len(xmrChains))
	}

	// Solana
	solChains := ListByType(ChainTypeSolana)
	if len(solChains) != 1 {
		t.Errorf("expected 1 solana-type chain, got %d", len(solChains))
	}
}

func TestUnsupportedChain(t *testing.T) {
	if IsSupported("INVALID") {
		t.Error("INVALID should not be supported")
	}

	_, ok := Get("INVALID", Mainnet)
	if ok {
		t.Error("Get(INVALID) should return false")
	}
}

func TestAllTestnetsRegistered(t *testing.T) {
	chains := []string{"BTC", "LTC", "DOGE", "ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "SOL", "XMR"}

	for _, symbol := range chains {
		_, ok := Get(symbol, Testnet)
		if !ok {
			t.Errorf("%s testnet should be registered", symbol)
		}
	}
}

func TestGetByChainID(t *testing.T) {
	tests := []struct {
		chainID uint64
		network Network
		symbol  string
	}{
		{1, Mainnet, "ETH"},
		{10, Mainnet, "OPTIMISM"},
		{56, Mainnet, "BSC"},
		{137, Mainnet, "POLYGON"},
		{8453, Mainnet, "BASE"},
		{42161, Mainnet, "ARBITRUM"},
		{43114, Mainnet, "AVAX"},
		{11155111, Testnet, "ETH"},
	}

	for _, tc := range tests {
		params, ok := GetByChainID(tc.chainID, tc.network)
		if !ok {
			t.Errorf("chainID %d should be registered", tc.chainID)
			continue
		}
		if params.Symbol != tc.symbol {
			t.Errorf("chainID %d symbol = %s, want %s", tc.chainID, params.Symbol, tc.symbol)
		}
	}

	// Test non-existent chain ID
	_, ok := GetByChainID(99999, Mainnet)
	if ok {
		t.Error("chainID 99999 should not exist")
	}
}

func TestListEVMChains(t *testing.T) {
	chains := ListEVMChains(Mainnet)
	if len(chains) != 7 {
		t.Errorf("expected 7 EVM chains, got %d", len(chains))
	}

	expectedChains := map[string]uint64{
		"ETH":      1,
		"BSC":      56,
		"POLYGON":  137,
		"ARBITRUM": 42161,
		"OPTIMISM": 10,
		"BASE":     8453,
		"AVAX":     43114,
	}

	for symbol, chainID := range expectedChains {
		if chains[symbol] != chainID {
			t.Errorf("%s chainID = %d, want %d", symbol, chains[symbol], chainID)
		}
	}
}

func TestTokenRegistry(t *testing.T) {
	// Test USDT on different chains
	usdtTests := []struct {
		chainID  uint64
		address  string
		decimals uint8
	}{
		{1, "0xdAC17F958D2ee523a2206206994597C13D831ec7", 6},
		{42161, "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", 6},
		{10, "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58", 6},
		{56, "0x55d398326f99059fF775485246999027B3197955", 18}, // BSC USDT has 18 decimals
		{137, "0xc2132D05D31c914a87C6611C10748AEb04B58e8F", 6},
	}

	for _, tc := range usdtTests {
		token := GetToken(tc.chainID, "USDT")
		if token == nil {
			t.Errorf("USDT should be registered on chainID %d", tc.chainID)
			continue
		}
		if token.Address != tc.address {
			t.Errorf("USDT address on chainID %d = %s, want %s", tc.chainID, token.Address, tc.address)
		}
		if token.Decimals != tc.decimals {
			t.Errorf("USDT decimals on chainID %d = %d, want %d", tc.chainID, token.Decimals, tc.decimals)
		}
	}

	// Test GetTokenAddress helper
	addr := GetTokenAddress(1, "USDC")
	if addr != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
		t.Errorf("USDC address on Ethereum = %s, want 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", addr)
	}

	// Test non-existent token
	addr = GetTokenAddress(1, "NONEXISTENT")
	if addr != "" {
		t.Errorf("NONEXISTENT token should return empty address, got %s", addr)
	}

	// Test IsTokenSupported
	if !IsTokenSupported(1, "USDT") {
		t.Error("USDT should be supported on Ethereum")
	}
	if IsTokenSupported(1, "NONEXISTENT") {
		t.Error("NONEXISTENT should not be supported")
	}

	// Test ListTokens
	ethTokens := ListTokens(1)
	if len(ethTokens) < 4 {
		t.Errorf("expected at least 4 tokens on Ethereum, got %d", len(ethTokens))
	}
}
