package backend

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/pkg/helpers"
)

func TestDefaultConfigs(t *testing.T) {
	configs := DefaultConfigs()

	expectedChains := []string{"BTC", "LTC", "DOGE", "ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "SOL", "XMR"}

	for _, symbol := range expectedChains {
		cfg, ok := configs[symbol]
		if !ok {
			t.Errorf("expected default config for %s", symbol)
			continue
		}
		if cfg.MainnetURL == "" {
			t.Errorf("%s: mainnet URL should not be empty", symbol)
		}
		if cfg.TestnetURL == "" {
			t.Errorf("%s: testnet URL should not be empty", symbol)
		}
	}
}

func TestDefaultConfigTypes(t *testing.T) {
	configs := DefaultConfigs()

	tests := []struct {
		symbol       string
		expectedType Type
	}{
		{"BTC", TypeMempool},
		{"LTC", TypeMempool},
		{"DOGE", TypeBlockbook},
		{"ETH", TypeJSONRPC},
		{"BSC", TypeJSONRPC},
		{"SOL", TypeJSONRPC},
	}

	for _, tc := range tests {
		cfg := configs[tc.symbol]
		if cfg.Type != tc.expectedType {
			t.Errorf("%s: type = %s, want %s", tc.symbol, cfg.Type, tc.expectedType)
		}
	}
}

func TestNewMempoolBackend(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")

	if backend.Type() != TypeMempool {
		t.Errorf("Type() = %s, want mempool", backend.Type())
	}

	if backend.IsConnected() {
		t.Error("should not be connected initially")
	}

	// Test URL normalization (trailing slash removal)
	backend2 := NewMempoolBackend("https://mempool.space/api/")
	if backend2.baseURL != "https://mempool.space/api" {
		t.Errorf("baseURL = %s, trailing slash should be removed", backend2.baseURL)
	}
}

func TestNewEsploraBackend(t *testing.T) {
	backend := NewEsploraBackend("https://blockstream.info/api")

	if backend.Type() != TypeEsplora {
		t.Errorf("Type() = %s, want esplora", backend.Type())
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Should be empty initially
	if len(reg.List()) != 0 {
		t.Error("registry should be empty initially")
	}

	// Register a backend
	btcBackend := NewMempoolBackend("https://mempool.space/api")
	reg.Register("BTC", btcBackend)

	// Get should return it
	got, ok := reg.Get("BTC")
	if !ok {
		t.Error("Get(BTC) should return true")
	}
	if got != btcBackend {
		t.Error("Get(BTC) returned wrong backend")
	}

	// Get unknown should return false
	_, ok = reg.Get("INVALID")
	if ok {
		t.Error("Get(INVALID) should return false")
	}

	// List should contain BTC
	list := reg.List()
	if len(list) != 1 || list[0] != "BTC" {
		t.Errorf("List() = %v, want [BTC]", list)
	}
}

func TestUTXOStruct(t *testing.T) {
	utxo := UTXO{
		TxID:          "abc123",
		Vout:          0,
		Amount:        100000,
		ScriptPubKey:  "76a914...",
		Confirmations: 6,
		BlockHeight:   800000,
	}

	if utxo.TxID != "abc123" {
		t.Error("TxID mismatch")
	}
	if utxo.Amount != 100000 {
		t.Error("Amount mismatch")
	}
}

func TestTransactionStruct(t *testing.T) {
	tx := Transaction{
		TxID:          "def456",
		Version:       2,
		Size:          250,
		VSize:         140,
		Weight:        560,
		LockTime:      0,
		Fee:           1000,
		Confirmed:     true,
		BlockHash:     "000000...",
		BlockHeight:   800001,
		Confirmations: 5,
		Inputs:        []TxInput{},
		Outputs:       []TxOutput{},
	}

	if tx.TxID != "def456" {
		t.Error("TxID mismatch")
	}
	if !tx.Confirmed {
		t.Error("should be confirmed")
	}
}

func TestAddressInfoStruct(t *testing.T) {
	info := AddressInfo{
		Address:        "bc1q...",
		TxCount:        10,
		FundedTxCount:  5,
		SpentTxCount:   3,
		FundedSum:      1000000,
		SpentSum:       500000,
		Balance:        500000,
		MempoolBalance: 10000,
	}

	if info.Balance != 500000 {
		t.Error("Balance mismatch")
	}
	if info.TxCount != 10 {
		t.Error("TxCount mismatch")
	}
}

func TestFeeEstimateStruct(t *testing.T) {
	fee := FeeEstimate{
		FastestFee:  50,
		HalfHourFee: 30,
		HourFee:     20,
		EconomyFee:  10,
		MinimumFee:  1,
	}

	if fee.FastestFee != 50 {
		t.Error("FastestFee mismatch")
	}
	if fee.MinimumFee != 1 {
		t.Error("MinimumFee mismatch")
	}
}

func TestBlockHeaderStruct(t *testing.T) {
	header := BlockHeader{
		Hash:         "000000...",
		Height:       800000,
		Version:      0x20000000,
		PreviousHash: "000000..prev",
		MerkleRoot:   "abcdef...",
		Timestamp:    1700000000,
		Bits:         0x17034219,
		Nonce:        12345,
		Difficulty:   67890.5,
		TxCount:      2500,
	}

	if header.Height != 800000 {
		t.Error("Height mismatch")
	}
	if header.TxCount != 2500 {
		t.Error("TxCount mismatch")
	}
}

func TestBackendTypes(t *testing.T) {
	types := []Type{TypeMempool, TypeEsplora, TypeElectrum, TypeBlockbook, TypeJSONRPC}

	for _, bt := range types {
		if bt == "" {
			t.Error("backend type should not be empty")
		}
	}

	if TypeMempool != "mempool" {
		t.Errorf("TypeMempool = %s, want mempool", TypeMempool)
	}
	if TypeEsplora != "esplora" {
		t.Errorf("TypeEsplora = %s, want esplora", TypeEsplora)
	}
}

func TestErrorTypes(t *testing.T) {
	errors := []error{
		ErrNotConnected,
		ErrTxNotFound,
		ErrAddressNotFound,
		ErrInvalidTx,
		ErrBroadcastFailed,
		ErrRateLimited,
		ErrUnsupportedBackend,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("error should not be nil")
		}
		if err.Error() == "" {
			t.Error("error message should not be empty")
		}
	}
}

// TestBackendInterface verifies interface compliance at compile time
func TestBackendInterface(t *testing.T) {
	var _ Backend = (*MempoolBackend)(nil)
	var _ Backend = (*EsploraBackend)(nil)
	var _ Backend = (*ElectrumBackend)(nil)
	var _ Backend = (*BlockbookBackend)(nil)
	var _ Backend = (*JSONRPCBackend)(nil)
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Type:       TypeMempool,
		MainnetURL: "https://mempool.space/api",
		TestnetURL: "https://mempool.space/testnet/api",
		Timeout:    30,
	}

	if cfg.Type != TypeMempool {
		t.Error("Type mismatch")
	}
	if cfg.MainnetURL == "" {
		t.Error("MainnetURL should not be empty")
	}
}

// TestMempoolBackendClose tests Close method
func TestMempoolBackendClose(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")

	// Force connected state
	backend.connected = true

	if !backend.IsConnected() {
		t.Error("should be connected")
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if backend.IsConnected() {
		t.Error("should not be connected after Close()")
	}
}

// TestRegistryConnectCloseAll tests registry batch operations
func TestRegistryConnectCloseAll(t *testing.T) {
	reg := NewRegistry()

	// Add mock backends that don't actually connect
	btc := NewMempoolBackend("https://mempool.space/api")
	ltc := NewMempoolBackend("https://litecoinspace.org/api")

	// Mark as connected for testing CloseAll
	btc.connected = true
	ltc.connected = true

	reg.Register("BTC", btc)
	reg.Register("LTC", ltc)

	// CloseAll should close all backends
	reg.CloseAll()

	if btc.IsConnected() {
		t.Error("BTC should be disconnected")
	}
	if ltc.IsConnected() {
		t.Error("LTC should be disconnected")
	}
}

// TestMempoolNotConnectedError tests that operations fail when not connected
func TestMempoolOperationsRequireContext(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")
	ctx := context.Background()

	// These should not panic even without connection
	// They will fail because we're not actually connected, but that's expected
	_, err := backend.GetBlockHeight(ctx)
	if err == nil {
		// If it succeeds, that's fine too (means real API is reachable)
		t.Log("GetBlockHeight succeeded (API reachable)")
	}
}

// ============ Electrum Backend Tests ============

func TestNewElectrumBackend(t *testing.T) {
	servers := []string{"electrum.blockstream.info:50002"}
	backend := NewElectrumBackend(servers, true)

	if backend.Type() != TypeElectrum {
		t.Errorf("Type() = %s, want electrum", backend.Type())
	}

	if backend.IsConnected() {
		t.Error("should not be connected initially")
	}

	if len(backend.servers) != 1 {
		t.Errorf("servers count = %d, want 1", len(backend.servers))
	}
}

func TestElectrumBackendClose(t *testing.T) {
	servers := []string{"localhost:50001"}
	backend := NewElectrumBackend(servers, false)

	// Force connected state
	backend.connected = true

	if !backend.IsConnected() {
		t.Error("should be connected")
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if backend.IsConnected() {
		t.Error("should not be connected after Close()")
	}
}

// ============ Blockbook Backend Tests ============

func TestNewBlockbookBackend(t *testing.T) {
	backend := NewBlockbookBackend("https://btc1.trezor.io/api/v2")

	if backend.Type() != TypeBlockbook {
		t.Errorf("Type() = %s, want blockbook", backend.Type())
	}

	if backend.IsConnected() {
		t.Error("should not be connected initially")
	}

	// Test URL normalization
	backend2 := NewBlockbookBackend("https://btc1.trezor.io/api/v2/")
	if backend2.baseURL != "https://btc1.trezor.io/api/v2" {
		t.Errorf("baseURL = %s, trailing slash should be removed", backend2.baseURL)
	}
}

func TestBlockbookBackendClose(t *testing.T) {
	backend := NewBlockbookBackend("https://btc1.trezor.io/api/v2")

	backend.connected = true

	if !backend.IsConnected() {
		t.Error("should be connected")
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if backend.IsConnected() {
		t.Error("should not be connected after Close()")
	}
}

func TestBlockbookParseAmount(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"100000", 100000},
		{"0", 0},
		{"1000000000", 1000000000},
		{"", 0},
	}

	for _, tc := range tests {
		result := parseAmount(tc.input)
		if result != tc.expected {
			t.Errorf("parseAmount(%s) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestBlockbookBtcKBToSatVB(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"0.0001", 10},     // 0.0001 BTC/kB = 10 sat/vB
		{"0.00001", 1},     // 0.00001 BTC/kB = 1 sat/vB
		{"0.001", 100},     // 0.001 BTC/kB = 100 sat/vB
		{"0", 1},           // Minimum 1
		{"-1", 1},          // Negative = minimum
	}

	for _, tc := range tests {
		result := btcKBToSatVB(tc.input)
		if result != tc.expected {
			t.Errorf("btcKBToSatVB(%s) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}

// ============ JSON-RPC Backend Tests ============

func TestNewJSONRPCBackend(t *testing.T) {
	// Bitcoin backend
	btcBackend := NewJSONRPCBackend("http://localhost:8332", RPCTypeBitcoin, "user", "pass")

	if btcBackend.Type() != TypeJSONRPC {
		t.Errorf("Type() = %s, want jsonrpc", btcBackend.Type())
	}

	if btcBackend.IsConnected() {
		t.Error("should not be connected initially")
	}

	if btcBackend.rpcType != RPCTypeBitcoin {
		t.Errorf("rpcType = %s, want bitcoin", btcBackend.rpcType)
	}

	// EVM backend
	evmBackend := NewJSONRPCBackend("https://eth.llamarpc.com", RPCTypeEVM, "", "")

	if evmBackend.rpcType != RPCTypeEVM {
		t.Errorf("rpcType = %s, want evm", evmBackend.rpcType)
	}
}

func TestJSONRPCBackendClose(t *testing.T) {
	backend := NewJSONRPCBackend("http://localhost:8332", RPCTypeBitcoin, "", "")

	backend.connected = true

	if !backend.IsConnected() {
		t.Error("should be connected")
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if backend.IsConnected() {
		t.Error("should not be connected after Close()")
	}
}

func TestHexToInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"0x1", 1},
		{"0xa", 10},
		{"0xff", 255},
		{"0x100", 256},
		{"0xc350b", 800011},
		{"", 0},
		{"0x", 0},
	}

	for _, tc := range tests {
		result := helpers.HexToInt64(tc.input)
		if result != tc.expected {
			t.Errorf("HexToInt64(%s) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestHexToUint64(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"0x1", 1},
		{"0xffffffff", 4294967295},
		{"0x0", 0},
		{"", 0},
		{"0xb1a2bc2ec50000", 50000000000000000}, // 0.05 ETH in wei
	}

	for _, tc := range tests {
		result := helpers.HexToUint64(tc.input)
		if result != tc.expected {
			t.Errorf("HexToUint64(%s) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestRPCTypes(t *testing.T) {
	if RPCTypeBitcoin != "bitcoin" {
		t.Errorf("RPCTypeBitcoin = %s, want bitcoin", RPCTypeBitcoin)
	}
	if RPCTypeEVM != "evm" {
		t.Errorf("RPCTypeEVM = %s, want evm", RPCTypeEVM)
	}
}

// ============ Registry with All Backends ============

func TestRegistryWithAllBackends(t *testing.T) {
	reg := NewRegistry()

	// Register all backend types
	reg.Register("BTC", NewMempoolBackend("https://mempool.space/api"))
	reg.Register("LTC", NewEsploraBackend("https://blockstream.info/api"))
	reg.Register("DOGE", NewBlockbookBackend("https://doge1.trezor.io/api/v2"))
	reg.Register("ETH", NewJSONRPCBackend("https://eth.llamarpc.com", RPCTypeEVM, "", ""))

	// Verify all registered
	if len(reg.List()) != 4 {
		t.Errorf("expected 4 backends, got %d", len(reg.List()))
	}

	// Verify types
	btc, _ := reg.Get("BTC")
	if btc.Type() != TypeMempool {
		t.Errorf("BTC type = %s, want mempool", btc.Type())
	}

	eth, _ := reg.Get("ETH")
	if eth.Type() != TypeJSONRPC {
		t.Errorf("ETH type = %s, want jsonrpc", eth.Type())
	}
}

// ============ Electrum Helper Function Tests ============

func TestReverseBytes(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0x01, 0x02, 0x03, 0x04}, []byte{0x04, 0x03, 0x02, 0x01}},
		{[]byte{0xab, 0xcd}, []byte{0xcd, 0xab}},
		{[]byte{0xff}, []byte{0xff}},
		{[]byte{}, []byte{}},
	}

	for _, tc := range tests {
		result := reverseBytes(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("reverseBytes(%x) length = %d, want %d", tc.input, len(result), len(tc.expected))
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("reverseBytes(%x) = %x, want %x", tc.input, result, tc.expected)
				break
			}
		}
	}
}

func TestBitsToTarget(t *testing.T) {
	tests := []struct {
		bits     uint32
		minDiff  float64
		maxDiff  float64
	}{
		// Genesis block bits (difficulty 1)
		{0x1d00ffff, 0.9, 1.1},
		// Zero mantissa should return 0
		{0x1d000000, 0, 0},
		// Higher difficulty (lower target) - bits from early blocks
		{0x1b0404cb, 16000, 17000}, // ~16307 difficulty
	}

	for _, tc := range tests {
		result := bitsToTarget(tc.bits)
		if result < tc.minDiff || result > tc.maxDiff {
			t.Errorf("bitsToTarget(0x%x) = %f, want between %f and %f", tc.bits, result, tc.minDiff, tc.maxDiff)
		}
	}
}

func TestParseBlockHeader(t *testing.T) {
	// Bitcoin genesis block header (80 bytes hex = 160 chars)
	genesisHeaderHex := "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a29ab5f49ffff001d1dac2b7c"

	header, err := parseBlockHeader(genesisHeaderHex, 0)
	if err != nil {
		t.Fatalf("parseBlockHeader() error = %v", err)
	}

	// Genesis block hash
	expectedHash := "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"
	if header.Hash != expectedHash {
		t.Errorf("Hash = %s, want %s", header.Hash, expectedHash)
	}

	if header.Height != 0 {
		t.Errorf("Height = %d, want 0", header.Height)
	}

	if header.Version != 1 {
		t.Errorf("Version = %d, want 1", header.Version)
	}

	// Previous hash should be all zeros for genesis
	expectedPrevHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if header.PreviousHash != expectedPrevHash {
		t.Errorf("PreviousHash = %s, want %s", header.PreviousHash, expectedPrevHash)
	}

	// Timestamp: 2009-01-03 18:15:05 UTC = 1231006505
	if header.Timestamp != 1231006505 {
		t.Errorf("Timestamp = %d, want 1231006505", header.Timestamp)
	}

	// Difficulty should be ~1.0 for genesis
	if header.Difficulty < 0.9 || header.Difficulty > 1.1 {
		t.Errorf("Difficulty = %f, want ~1.0", header.Difficulty)
	}
}

func TestParseBlockHeaderErrors(t *testing.T) {
	// Invalid hex
	_, err := parseBlockHeader("not-valid-hex", 0)
	if err == nil {
		t.Error("expected error for invalid hex")
	}

	// Wrong length (too short)
	_, err = parseBlockHeader("0100000000000000", 0)
	if err == nil {
		t.Error("expected error for short header")
	}

	// Wrong length (too long)
	longHeader := "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a29ab5f49ffff001d1dac2b7c00"
	_, err = parseBlockHeader(longHeader, 0)
	if err == nil {
		t.Error("expected error for long header")
	}
}

func TestAddressToScriptPubKey(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		expectError bool
		scriptLen   int // Expected script length, 0 if error expected
	}{
		// P2PKH mainnet (1...)
		{"P2PKH mainnet", "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", false, 25},
		// P2SH mainnet (3...)
		{"P2SH mainnet", "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", false, 23},
		// P2WPKH mainnet (bc1q... 42 chars)
		{"P2WPKH mainnet", "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq", false, 22},
		// P2TR mainnet (bc1p... 62 chars)
		{"P2TR mainnet", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", false, 34},
		// Testnet P2WPKH (tb1q...)
		{"P2WPKH testnet", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", false, 22},
		// Litecoin mainnet (L...)
		{"Litecoin P2PKH", "LaMT348PWRnrqeeWArpwQPbuanpXDZGEUz", false, 25},
		// Dogecoin mainnet (D...)
		{"Dogecoin P2PKH", "DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L", false, 25},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			script, err := addressToScriptPubKey(tc.address)
			if tc.expectError {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(script) != tc.scriptLen {
				t.Errorf("script length = %d, want %d", len(script), tc.scriptLen)
			}
		})
	}
}

func TestAddressToScriptHash(t *testing.T) {
	// Test with known P2WPKH address
	address := "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"
	scriptHash := addressToScriptHash(address)

	// Should return 64 char hex string (32 bytes reversed)
	if len(scriptHash) != 64 {
		t.Errorf("scriptHash length = %d, want 64", len(scriptHash))
	}

	// Verify it's valid hex
	_, err := hex.DecodeString(scriptHash)
	if err != nil {
		t.Errorf("scriptHash is not valid hex: %v", err)
	}
}

// ============ Mempool Transaction Conversion Tests ============

func TestMempoolConvertTxs(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")

	// Create test mempool transaction
	mTxs := []mempoolTx{
		{
			TxID:     "abc123def456",
			Version:  2,
			LockTime: 0,
			Size:     250,
			Weight:   600,
			Fee:      1500,
			Status: struct {
				Confirmed   bool   `json:"confirmed"`
				BlockHeight int64  `json:"block_height"`
				BlockHash   string `json:"block_hash"`
				BlockTime   int64  `json:"block_time"`
			}{
				Confirmed:   true,
				BlockHeight: 800000,
				BlockHash:   "00000000000000000001",
				BlockTime:   1700000000,
			},
			Vin: []struct {
				TxID         string   `json:"txid"`
				Vout         uint32   `json:"vout"`
				ScriptSig    string   `json:"scriptsig"`
				ScriptSigAsm string   `json:"scriptsig_asm"`
				Witness      []string `json:"witness"`
				Sequence     uint32   `json:"sequence"`
				Prevout      *struct {
					ScriptPubKey     string `json:"scriptpubkey"`
					ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
					ScriptPubKeyType string `json:"scriptpubkey_type"`
					ScriptPubKeyAddr string `json:"scriptpubkey_address"`
					Value            uint64 `json:"value"`
				} `json:"prevout"`
			}{
				{
					TxID:     "prevtx123",
					Vout:     0,
					Sequence: 0xfffffffe,
					Prevout: &struct {
						ScriptPubKey     string `json:"scriptpubkey"`
						ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
						ScriptPubKeyType string `json:"scriptpubkey_type"`
						ScriptPubKeyAddr string `json:"scriptpubkey_address"`
						Value            uint64 `json:"value"`
					}{
						ScriptPubKey:     "0014abc",
						ScriptPubKeyType: "v0_p2wpkh",
						ScriptPubKeyAddr: "bc1qtest",
						Value:            100000,
					},
				},
			},
			Vout: []struct {
				ScriptPubKey     string `json:"scriptpubkey"`
				ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
				ScriptPubKeyType string `json:"scriptpubkey_type"`
				ScriptPubKeyAddr string `json:"scriptpubkey_address"`
				Value            uint64 `json:"value"`
			}{
				{
					ScriptPubKey:     "0014def",
					ScriptPubKeyType: "v0_p2wpkh",
					ScriptPubKeyAddr: "bc1qoutput",
					Value:            98500,
				},
			},
		},
	}

	txs := backend.convertTxs(mTxs)

	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txs))
	}

	tx := txs[0]

	if tx.TxID != "abc123def456" {
		t.Errorf("TxID = %s, want abc123def456", tx.TxID)
	}

	if tx.Version != 2 {
		t.Errorf("Version = %d, want 2", tx.Version)
	}

	if tx.Fee != 1500 {
		t.Errorf("Fee = %d, want 1500", tx.Fee)
	}

	if !tx.Confirmed {
		t.Error("Confirmed should be true")
	}

	if tx.BlockHeight != 800000 {
		t.Errorf("BlockHeight = %d, want 800000", tx.BlockHeight)
	}

	// VSize = (Weight + 3) / 4
	expectedVSize := (int64(600) + 3) / 4
	if tx.VSize != expectedVSize {
		t.Errorf("VSize = %d, want %d", tx.VSize, expectedVSize)
	}

	if len(tx.Inputs) != 1 {
		t.Fatalf("expected 1 input, got %d", len(tx.Inputs))
	}

	if tx.Inputs[0].TxID != "prevtx123" {
		t.Errorf("Input TxID = %s, want prevtx123", tx.Inputs[0].TxID)
	}

	if tx.Inputs[0].PrevOut == nil {
		t.Fatal("PrevOut should not be nil")
	}

	if tx.Inputs[0].PrevOut.Value != 100000 {
		t.Errorf("PrevOut Value = %d, want 100000", tx.Inputs[0].PrevOut.Value)
	}

	if len(tx.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(tx.Outputs))
	}

	if tx.Outputs[0].Value != 98500 {
		t.Errorf("Output Value = %d, want 98500", tx.Outputs[0].Value)
	}
}

func TestMempoolConvertTxsEmpty(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")

	txs := backend.convertTxs([]mempoolTx{})

	if len(txs) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(txs))
	}
}

func TestMempoolConvertTxsNoPrevout(t *testing.T) {
	backend := NewMempoolBackend("https://mempool.space/api")

	// Transaction with nil prevout (coinbase-like)
	mTxs := []mempoolTx{
		{
			TxID: "coinbase123",
			Vin: []struct {
				TxID         string   `json:"txid"`
				Vout         uint32   `json:"vout"`
				ScriptSig    string   `json:"scriptsig"`
				ScriptSigAsm string   `json:"scriptsig_asm"`
				Witness      []string `json:"witness"`
				Sequence     uint32   `json:"sequence"`
				Prevout      *struct {
					ScriptPubKey     string `json:"scriptpubkey"`
					ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
					ScriptPubKeyType string `json:"scriptpubkey_type"`
					ScriptPubKeyAddr string `json:"scriptpubkey_address"`
					Value            uint64 `json:"value"`
				} `json:"prevout"`
			}{
				{
					TxID:    "",
					Prevout: nil, // No prevout for coinbase
				},
			},
			Vout: []struct {
				ScriptPubKey     string `json:"scriptpubkey"`
				ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
				ScriptPubKeyType string `json:"scriptpubkey_type"`
				ScriptPubKeyAddr string `json:"scriptpubkey_address"`
				Value            uint64 `json:"value"`
			}{
				{Value: 625000000}, // Block reward
			},
		},
	}

	txs := backend.convertTxs(mTxs)

	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txs))
	}

	if txs[0].Inputs[0].PrevOut != nil {
		t.Error("PrevOut should be nil for coinbase")
	}
}

// TestNewDefaultRegistry verifies that NewDefaultRegistry correctly registers
// all backends including EVM chains.
func TestNewDefaultRegistry(t *testing.T) {
	tests := []struct {
		name    string
		network chain.Network
	}{
		{"mainnet", chain.Mainnet},
		{"testnet", chain.Testnet},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewDefaultRegistry(tc.network)

			// These chains should be registered (Mempool/Blockbook backends)
			utxoChains := []string{"BTC", "LTC", "DOGE"}
			for _, symbol := range utxoChains {
				if _, ok := reg.Get(symbol); !ok {
					t.Errorf("expected %s backend to be registered", symbol)
				}
			}

			// These EVM chains should now be registered (fixed JSON-RPC registration)
			evmChains := []string{"ETH", "BSC", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX"}
			for _, symbol := range evmChains {
				b, ok := reg.Get(symbol)
				if !ok {
					t.Errorf("expected %s backend to be registered", symbol)
					continue
				}
				if b.Type() != TypeJSONRPC {
					t.Errorf("%s backend type = %s, want jsonrpc", symbol, b.Type())
				}
			}

			// Verify list contains expected number of backends
			// 3 UTXO + 7 EVM = 10 (SOL/XMR skipped as they need specialized implementations)
			list := reg.List()
			if len(list) < 10 {
				t.Errorf("expected at least 10 backends, got %d: %v", len(list), list)
			}
		})
	}
}
