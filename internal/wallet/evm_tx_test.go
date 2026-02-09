package wallet

import (
	"math/big"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
)

func TestRLPEncodeUint(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected []byte
	}{
		{"zero", 0, []byte{0x80}},
		{"single byte", 127, []byte{0x7f}},
		{"0x80", 128, []byte{0x81, 0x80}},
		{"0x400", 1024, []byte{0x82, 0x04, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rlpEncodeUint(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestRLPEncodeBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"empty", nil, []byte{0x80}},
		{"single byte < 0x80", []byte{0x00}, []byte{0x00}},
		{"single byte < 0x80 (2)", []byte{0x7f}, []byte{0x7f}},
		{"single byte >= 0x80", []byte{0x80}, []byte{0x81, 0x80}},
		{"short string", []byte("dog"), []byte{0x83, 'd', 'o', 'g'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rlpEncodeBytes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestRLPEncodeList(t *testing.T) {
	// Test empty list
	result := rlpEncodeList([]interface{}{})
	expected := []byte{0xc0}
	if len(result) != len(expected) {
		t.Errorf("empty list: length mismatch: got %d, want %d", len(result), len(expected))
	}
	if result[0] != expected[0] {
		t.Errorf("empty list: got 0x%02x, want 0x%02x", result[0], expected[0])
	}

	// Test list with items
	result = rlpEncodeList([]interface{}{uint64(1), uint64(2), uint64(3)})
	if result[0] != 0xc3 { // 0xc0 + 3 bytes
		t.Errorf("list of 3 uints: got prefix 0x%02x, want 0xc3", result[0])
	}
}

func TestBuildAndSignEVMTx(t *testing.T) {
	// Generate a test private key
	privKeyBytes := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)

	tests := []struct {
		name    string
		params  *EVMTxParams
		wantErr bool
	}{
		{
			name: "simple ETH transfer",
			params: &EVMTxParams{
				Nonce:    0,
				To:       "0x742d35Cc6634C0532925a3b844Bc9e7595f43092",
				Value:    big.NewInt(1000000000000000000), // 1 ETH
				ChainID:  1,
				GasLimit: 21000,
				GasPrice: big.NewInt(20000000000), // 20 gwei
			},
			wantErr: false,
		},
		{
			name: "sepolia testnet transfer",
			params: &EVMTxParams{
				Nonce:    5,
				To:       "0x742d35Cc6634C0532925a3b844Bc9e7595f43092",
				Value:    big.NewInt(100000000000000000), // 0.1 ETH
				ChainID:  11155111,                       // Sepolia
				GasLimit: 21000,
				GasPrice: big.NewInt(10000000000), // 10 gwei
			},
			wantErr: false,
		},
		{
			name: "invalid destination address",
			params: &EVMTxParams{
				Nonce:    0,
				To:       "invalid",
				Value:    big.NewInt(1000000000000000000),
				ChainID:  1,
				GasLimit: 21000,
				GasPrice: big.NewInt(20000000000),
			},
			wantErr: true,
		},
		{
			name: "missing gas price",
			params: &EVMTxParams{
				Nonce:    0,
				To:       "0x742d35Cc6634C0532925a3b844Bc9e7595f43092",
				Value:    big.NewInt(1000000000000000000),
				ChainID:  1,
				GasLimit: 21000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildAndSignEVMTx(privKey, tt.params)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify result
			if result.TxHash == "" {
				t.Error("empty tx hash")
			}
			if !strings.HasPrefix(result.TxHash, "0x") {
				t.Error("tx hash should have 0x prefix")
			}
			if len(result.TxHash) != 66 { // 0x + 64 hex chars
				t.Errorf("tx hash length: got %d, want 66", len(result.TxHash))
			}

			if result.RawTx == "" {
				t.Error("empty raw tx")
			}
			if !strings.HasPrefix(result.RawTx, "0x") {
				t.Error("raw tx should have 0x prefix")
			}

			if result.Nonce != tt.params.Nonce {
				t.Errorf("nonce mismatch: got %d, want %d", result.Nonce, tt.params.Nonce)
			}
		})
	}
}

func TestBuildAndSignEVMTxEIP1559(t *testing.T) {
	// Generate a test private key
	privKeyBytes := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)

	result, err := BuildAndSignEVMTx(privKey, &EVMTxParams{
		Nonce:                0,
		To:                   "0x742d35Cc6634C0532925a3b844Bc9e7595f43092",
		Value:                big.NewInt(1000000000000000000),
		ChainID:              1,
		GasLimit:             21000,
		MaxFeePerGas:         big.NewInt(30000000000), // 30 gwei
		MaxPriorityFeePerGas: big.NewInt(2000000000),  // 2 gwei
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// EIP-1559 transactions start with 0x02
	if !strings.HasPrefix(result.RawTx, "0x02") {
		t.Error("EIP-1559 tx should start with 0x02")
	}

	if result.TxHash == "" {
		t.Error("empty tx hash")
	}
}

func TestEncodeERC20Transfer(t *testing.T) {
	to := "0x742d35Cc6634C0532925a3b844Bc9e7595f43092"
	amount := big.NewInt(1000000) // 1 USDT (6 decimals)

	data, err := EncodeERC20Transfer(to, amount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check function selector (transfer)
	expectedSelector := []byte{0xa9, 0x05, 0x9c, 0xbb}
	if data[0] != expectedSelector[0] || data[1] != expectedSelector[1] ||
		data[2] != expectedSelector[2] || data[3] != expectedSelector[3] {
		t.Errorf("wrong function selector: got %x, want %x", data[:4], expectedSelector)
	}

	// Check length: 4 (selector) + 32 (address) + 32 (amount) = 68
	if len(data) != 68 {
		t.Errorf("wrong data length: got %d, want 68", len(data))
	}
}

func TestEncodeERC20BalanceOf(t *testing.T) {
	address := "0x742d35Cc6634C0532925a3b844Bc9e7595f43092"

	data, err := EncodeERC20BalanceOf(address)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check function selector (balanceOf)
	expectedSelector := []byte{0x70, 0xa0, 0x82, 0x31}
	if data[0] != expectedSelector[0] || data[1] != expectedSelector[1] ||
		data[2] != expectedSelector[2] || data[3] != expectedSelector[3] {
		t.Errorf("wrong function selector: got %x, want %x", data[:4], expectedSelector)
	}

	// Check length: 4 (selector) + 32 (address) = 36
	if len(data) != 36 {
		t.Errorf("wrong data length: got %d, want 36", len(data))
	}
}

func TestDecodeERC20BalanceResult(t *testing.T) {
	// Simulate a balance of 1000000 (6 decimals = 1.0 USDT)
	expected := big.NewInt(1000000)
	data := make([]byte, 32)
	copy(data[32-len(expected.Bytes()):], expected.Bytes())

	balance, err := DecodeERC20BalanceResult(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if balance.Cmp(expected) != 0 {
		t.Errorf("balance mismatch: got %s, want %s", balance.String(), expected.String())
	}
}

func TestAddressToBytes(t *testing.T) {
	// Test with 0x prefix
	addr := "0x742d35Cc6634C0532925a3b844Bc9e7595f43092"
	bytes := addressToBytes(addr)
	if len(bytes) != 20 {
		t.Errorf("expected 20 bytes, got %d", len(bytes))
	}

	// Test without 0x prefix
	addr2 := "742d35Cc6634C0532925a3b844Bc9e7595f43092"
	bytes2 := addressToBytes(addr2)
	if len(bytes2) != 20 {
		t.Errorf("expected 20 bytes, got %d", len(bytes2))
	}

	// Should be equal
	for i := range bytes {
		if bytes[i] != bytes2[i] {
			t.Errorf("byte %d mismatch", i)
		}
	}

	// Test empty
	empty := addressToBytes("")
	if len(empty) != 0 {
		t.Errorf("empty address should return empty bytes")
	}
}
