package config

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSupportedCoins(t *testing.T) {
	expectedCoins := []string{"BTC", "LTC", "DOGE", "XMR", "ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "SOL"}

	for _, symbol := range expectedCoins {
		if !IsCoinSupported(symbol) {
			t.Errorf("expected %s to be supported", symbol)
		}
	}

	// Test unsupported coin
	if IsCoinSupported("INVALID") {
		t.Error("INVALID should not be supported")
	}
}

func TestGetCoin(t *testing.T) {
	// Test BTC
	btc, ok := GetCoin("BTC")
	if !ok {
		t.Fatal("BTC should exist")
	}
	if btc.Symbol != "BTC" {
		t.Errorf("expected BTC, got %s", btc.Symbol)
	}
	if btc.Decimals != 8 {
		t.Errorf("expected 8 decimals, got %d", btc.Decimals)
	}
	if btc.Type != CoinTypeBitcoin {
		t.Errorf("expected bitcoin type, got %s", btc.Type)
	}

	// Test ETH
	eth, ok := GetCoin("ETH")
	if !ok {
		t.Fatal("ETH should exist")
	}
	if eth.Decimals != 18 {
		t.Errorf("expected 18 decimals, got %d", eth.Decimals)
	}
	if eth.Type != CoinTypeEVM {
		t.Errorf("expected evm type, got %s", eth.Type)
	}

	// Test XMR
	xmr, ok := GetCoin("XMR")
	if !ok {
		t.Fatal("XMR should exist")
	}
	if xmr.Decimals != 12 {
		t.Errorf("expected 12 decimals, got %d", xmr.Decimals)
	}
	if xmr.Type != CoinTypeMonero {
		t.Errorf("expected monero type, got %s", xmr.Type)
	}

	// Test non-existent
	_, ok = GetCoin("INVALID")
	if ok {
		t.Error("INVALID should not exist")
	}
}

func TestSwapMethods(t *testing.T) {
	// BTC should support MuSig2 and HTLC
	if !SupportsSwapMethod("BTC", SwapMethodMuSig2) {
		t.Error("BTC should support MuSig2")
	}
	if !SupportsSwapMethod("BTC", SwapMethodHTLC) {
		t.Error("BTC should support HTLC")
	}

	// ETH should support contract
	if !SupportsSwapMethod("ETH", SwapMethodContract) {
		t.Error("ETH should support contract")
	}

	// XMR should support adaptor
	if !SupportsSwapMethod("XMR", SwapMethodAdaptor) {
		t.Error("XMR should support adaptor")
	}

	// SOL should support program
	if !SupportsSwapMethod("SOL", SwapMethodProgram) {
		t.Error("SOL should support program")
	}
}

func TestGetPreferredSwapMethod(t *testing.T) {
	// BTC should prefer MuSig2
	if method := GetPreferredSwapMethod("BTC"); method != SwapMethodMuSig2 {
		t.Errorf("BTC preferred method should be musig2, got %s", method)
	}

	// ETH should prefer contract
	if method := GetPreferredSwapMethod("ETH"); method != SwapMethodContract {
		t.Errorf("ETH preferred method should be contract, got %s", method)
	}

	// Invalid coin
	if method := GetPreferredSwapMethod("INVALID"); method != "" {
		t.Errorf("invalid coin should return empty method, got %s", method)
	}
}

func TestFeeConfig(t *testing.T) {
	fees := DefaultFeeConfig()

	// Check default values
	if fees.MakerFeeBPS != 20 {
		t.Errorf("expected maker fee 20 bps, got %d", fees.MakerFeeBPS)
	}
	if fees.TakerFeeBPS != 20 {
		t.Errorf("expected taker fee 20 bps, got %d", fees.TakerFeeBPS)
	}
	if fees.DAOShareBPS != 5000 {
		t.Errorf("expected DAO share 5000 bps, got %d", fees.DAOShareBPS)
	}
	if fees.NodeOperatorShareBPS != 5000 {
		t.Errorf("expected node operator share 5000 bps, got %d", fees.NodeOperatorShareBPS)
	}

	// Check total fee
	if fees.TotalFeeBPS() != 40 {
		t.Errorf("expected total fee 40 bps, got %d", fees.TotalFeeBPS())
	}
}

func TestCalculateFee(t *testing.T) {
	fees := DefaultFeeConfig()

	// Test maker fee (0.2% of 1 BTC = 0.002 BTC = 200000 satoshis)
	amount := uint64(100000000) // 1 BTC in satoshis
	makerFee := fees.CalculateFee(amount, true)
	expectedMakerFee := uint64(200000) // 0.2%
	if makerFee != expectedMakerFee {
		t.Errorf("maker fee: expected %d, got %d", expectedMakerFee, makerFee)
	}

	// Test taker fee
	takerFee := fees.CalculateFee(amount, false)
	if takerFee != expectedMakerFee {
		t.Errorf("taker fee: expected %d, got %d", expectedMakerFee, takerFee)
	}
}

func TestCalculateFeeShares(t *testing.T) {
	fees := DefaultFeeConfig()

	feeAmount := uint64(200000) // Example fee

	daoShare := fees.CalculateDAOShare(feeAmount)
	nodeShare := fees.CalculateNodeOperatorShare(feeAmount)

	expectedShare := uint64(100000) // 50% each

	if daoShare != expectedShare {
		t.Errorf("DAO share: expected %d, got %d", expectedShare, daoShare)
	}
	if nodeShare != expectedShare {
		t.Errorf("node share: expected %d, got %d", expectedShare, nodeShare)
	}

	// Shares should equal total
	if daoShare+nodeShare != feeAmount {
		t.Errorf("shares should equal total fee: %d + %d != %d", daoShare, nodeShare, feeAmount)
	}
}

func TestExchangeConfigMainnet(t *testing.T) {
	cfg := NewExchangeConfig(Mainnet)

	if cfg.Network != Mainnet {
		t.Errorf("expected mainnet, got %s", cfg.Network)
	}

	// Check ETH chain ID
	ethParams, ok := cfg.GetChainParams("ETH")
	if !ok {
		t.Fatal("ETH chain params should exist")
	}
	if ethParams.ChainID != 1 {
		t.Errorf("ETH mainnet chain ID should be 1, got %d", ethParams.ChainID)
	}

	// Check BSC chain ID
	bscParams, ok := cfg.GetChainParams("BSC")
	if !ok {
		t.Fatal("BSC chain params should exist")
	}
	if bscParams.ChainID != 56 {
		t.Errorf("BSC mainnet chain ID should be 56, got %d", bscParams.ChainID)
	}
}

func TestExchangeConfigTestnet(t *testing.T) {
	cfg := NewExchangeConfig(Testnet)

	if cfg.Network != Testnet {
		t.Errorf("expected testnet, got %s", cfg.Network)
	}

	// Check ETH Sepolia chain ID
	ethParams, ok := cfg.GetChainParams("ETH")
	if !ok {
		t.Fatal("ETH chain params should exist")
	}
	if ethParams.ChainID != 11155111 {
		t.Errorf("ETH testnet chain ID should be 11155111 (Sepolia), got %d", ethParams.ChainID)
	}

	// Check testnet DAO address
	evmAddr := cfg.GetDAOAddress("ETH")
	expectedAddr := "0x37e565Bab0c11756806480102E09871f33403D8d"
	if evmAddr != expectedAddr {
		t.Errorf("EVM testnet DAO address: expected %s, got %s", expectedAddr, evmAddr)
	}
}

func TestGetDAOAddress(t *testing.T) {
	cfg := NewExchangeConfig(Testnet)

	// All EVM chains should use the same address
	ethAddr := cfg.GetDAOAddress("ETH")
	bscAddr := cfg.GetDAOAddress("BSC")
	polygonAddr := cfg.GetDAOAddress("POLYGON")
	arbAddr := cfg.GetDAOAddress("ARBITRUM")
	opAddr := cfg.GetDAOAddress("OPTIMISM")
	baseAddr := cfg.GetDAOAddress("BASE")

	if ethAddr != bscAddr || bscAddr != polygonAddr || polygonAddr != arbAddr || arbAddr != opAddr || opAddr != baseAddr {
		t.Error("all EVM chains should use the same DAO address")
	}

	// BTC should have its own address
	btcAddr := cfg.GetDAOAddress("BTC")
	if btcAddr == ethAddr {
		t.Error("BTC should have different DAO address than EVM")
	}

	// Invalid coin
	invalidAddr := cfg.GetDAOAddress("INVALID")
	if invalidAddr != "" {
		t.Error("invalid coin should return empty address")
	}
}

func TestSwapConfig(t *testing.T) {
	swap := DefaultSwapConfig()

	// Initiator lock time should be longer than responder
	if swap.InitiatorLockTime <= swap.ResponderLockTime {
		t.Error("initiator lock time should be longer than responder lock time")
	}

	// Check minimum delta
	delta := swap.InitiatorLockTime - swap.ResponderLockTime
	if delta < swap.MinLockTimeDelta {
		t.Error("lock time delta should be at least MinLockTimeDelta")
	}

	// Check secret size
	if swap.SecretSize != 32 {
		t.Errorf("secret size should be 32 bytes, got %d", swap.SecretSize)
	}
}

func TestListSupportedCoins(t *testing.T) {
	coins := ListSupportedCoins()

	if len(coins) != len(SupportedCoins) {
		t.Errorf("expected %d coins, got %d", len(SupportedCoins), len(coins))
	}

	// Check that all returned coins are valid
	for _, symbol := range coins {
		if !IsCoinSupported(symbol) {
			t.Errorf("coin %s should be supported", symbol)
		}
	}
}

func TestListCoinsByType(t *testing.T) {
	// Bitcoin type coins
	btcCoins := ListCoinsByType(CoinTypeBitcoin)
	expectedBTC := []string{"BTC", "LTC", "DOGE"}
	if len(btcCoins) != len(expectedBTC) {
		t.Errorf("expected %d bitcoin type coins, got %d", len(expectedBTC), len(btcCoins))
	}

	// EVM type coins
	evmCoins := ListCoinsByType(CoinTypeEVM)
	expectedEVM := []string{"ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX"}
	if len(evmCoins) != len(expectedEVM) {
		t.Errorf("expected %d evm type coins, got %d: %v", len(expectedEVM), len(evmCoins), evmCoins)
	}

	// Monero type
	xmrCoins := ListCoinsByType(CoinTypeMonero)
	if len(xmrCoins) != 1 || xmrCoins[0] != "XMR" {
		t.Error("should have exactly one monero type coin: XMR")
	}

	// Solana type
	solCoins := ListCoinsByType(CoinTypeSolana)
	if len(solCoins) != 1 || solCoins[0] != "SOL" {
		t.Error("should have exactly one solana type coin: SOL")
	}
}

func TestCoinMinMaxAmounts(t *testing.T) {
	btc, _ := GetCoin("BTC")

	// BTC min should be 0.0001 BTC (10000 satoshis)
	if btc.MinAmount != 10000 {
		t.Errorf("BTC min amount should be 10000 satoshis, got %d", btc.MinAmount)
	}

	// BTC max should be 1000 BTC
	expectedMax := uint64(100000000000)
	if btc.MaxAmount != expectedMax {
		t.Errorf("BTC max amount should be %d, got %d", expectedMax, btc.MaxAmount)
	}

	// LTC max should be 0 (no limit)
	ltc, _ := GetCoin("LTC")
	if ltc.MaxAmount != 0 {
		t.Errorf("LTC max amount should be 0 (no limit), got %d", ltc.MaxAmount)
	}
}

func TestChainConfirmations(t *testing.T) {
	mainnetCfg := NewExchangeConfig(Mainnet)
	testnetCfg := NewExchangeConfig(Testnet)

	// Mainnet should require more confirmations than testnet
	btcMainnet, _ := mainnetCfg.GetChainParams("BTC")
	btcTestnet, _ := testnetCfg.GetChainParams("BTC")

	if btcMainnet.Confirmations <= btcTestnet.Confirmations {
		t.Error("mainnet should require more confirmations than testnet")
	}
}

// =============================================================================
// EVM Contract Tests
// =============================================================================

func TestGetHTLCContract(t *testing.T) {
	// Sepolia should have HTLC deployed
	sepoliaHTLC := GetHTLCContract(11155111)
	expectedAddr := common.HexToAddress("0x628c677e7b8889e64564d3f381565a9e6656aade")
	if sepoliaHTLC != expectedAddr {
		t.Errorf("Sepolia HTLC = %s, want %s", sepoliaHTLC.Hex(), expectedAddr.Hex())
	}

	// Mainnet should NOT have HTLC deployed (pending audit)
	mainnetHTLC := GetHTLCContract(1)
	if mainnetHTLC.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Errorf("Mainnet HTLC should be zero address (not deployed), got %s", mainnetHTLC.Hex())
	}

	// Unknown chain should return zero address
	unknownHTLC := GetHTLCContract(999999)
	if unknownHTLC.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Errorf("Unknown chain HTLC should be zero address, got %s", unknownHTLC.Hex())
	}
}

func TestIsHTLCDeployed(t *testing.T) {
	// Sepolia should be deployed
	if !IsHTLCDeployed(11155111) {
		t.Error("HTLC should be deployed on Sepolia")
	}

	// Mainnet should NOT be deployed
	if IsHTLCDeployed(1) {
		t.Error("HTLC should NOT be deployed on mainnet yet")
	}

	// Unknown chain should NOT be deployed
	if IsHTLCDeployed(999999) {
		t.Error("HTLC should NOT be deployed on unknown chain")
	}
}

func TestListDeployedHTLCChains(t *testing.T) {
	chains := ListDeployedHTLCChains()

	// Should have at least Sepolia
	found := false
	for _, chainID := range chains {
		if chainID == 11155111 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Sepolia (11155111) should be in deployed chains list")
	}

	// Should NOT include mainnet (not deployed)
	for _, chainID := range chains {
		if chainID == 1 {
			t.Error("Mainnet (1) should NOT be in deployed chains list")
		}
	}
}

func TestGetEVMContracts(t *testing.T) {
	// Sepolia should return contracts
	sepolia := GetEVMContracts(11155111)
	if sepolia == nil {
		t.Fatal("GetEVMContracts(11155111) should not return nil")
	}
	expectedAddr := common.HexToAddress("0x628c677e7b8889e64564d3f381565a9e6656aade")
	if sepolia.HTLCContract != expectedAddr {
		t.Errorf("Sepolia HTLC = %s, want %s", sepolia.HTLCContract.Hex(), expectedAddr.Hex())
	}

	// Unknown chain should return nil
	unknown := GetEVMContracts(999999)
	if unknown != nil {
		t.Error("GetEVMContracts(999999) should return nil")
	}
}
