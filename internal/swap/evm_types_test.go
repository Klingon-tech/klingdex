// Package swap - Tests for EVM HTLC types.
package swap

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

func TestGetCrossChainSwapType(t *testing.T) {
	tests := []struct {
		name         string
		offerChain   string
		requestChain string
		network      chain.Network
		expected     CrossChainType
	}{
		{
			name:         "BTC to LTC (Bitcoin to Bitcoin)",
			offerChain:   "BTC",
			requestChain: "LTC",
			network:      chain.Testnet,
			expected:     CrossChainTypeBitcoinToBitcoin,
		},
		{
			name:         "ETH to BSC (EVM to EVM)",
			offerChain:   "ETH",
			requestChain: "BSC",
			network:      chain.Mainnet,
			expected:     CrossChainTypeEVMToEVM,
		},
		{
			name:         "BTC to ETH (Bitcoin to EVM)",
			offerChain:   "BTC",
			requestChain: "ETH",
			network:      chain.Testnet,
			expected:     CrossChainTypeBitcoinToEVM,
		},
		{
			name:         "ETH to BTC (EVM to Bitcoin)",
			offerChain:   "ETH",
			requestChain: "BTC",
			network:      chain.Testnet,
			expected:     CrossChainTypeEVMToBitcoin,
		},
		{
			name:         "LTC to DOGE (Bitcoin to Bitcoin)",
			offerChain:   "LTC",
			requestChain: "DOGE",
			network:      chain.Testnet,
			expected:     CrossChainTypeBitcoinToBitcoin,
		},
		{
			name:         "POLYGON to ARBITRUM (EVM to EVM)",
			offerChain:   "POLYGON",
			requestChain: "ARBITRUM",
			network:      chain.Mainnet,
			expected:     CrossChainTypeEVMToEVM,
		},
		{
			name:         "Unknown chains",
			offerChain:   "UNKNOWN1",
			requestChain: "UNKNOWN2",
			network:      chain.Testnet,
			expected:     CrossChainTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCrossChainSwapType(tt.offerChain, tt.requestChain, tt.network)
			if result != tt.expected {
				t.Errorf("GetCrossChainSwapType(%s, %s, %s) = %s, want %s",
					tt.offerChain, tt.requestChain, tt.network, result, tt.expected)
			}
		})
	}
}

func TestCrossChainTypeString(t *testing.T) {
	tests := []struct {
		swapType CrossChainType
		expected string
	}{
		{CrossChainTypeBitcoinToBitcoin, "bitcoin_to_bitcoin"},
		{CrossChainTypeEVMToEVM, "evm_to_evm"},
		{CrossChainTypeBitcoinToEVM, "bitcoin_to_evm"},
		{CrossChainTypeEVMToBitcoin, "evm_to_bitcoin"},
		{CrossChainTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.swapType.String() != tt.expected {
				t.Errorf("CrossChainType.String() = %s, want %s", tt.swapType.String(), tt.expected)
			}
		})
	}
}

func TestCrossChainTypeIsCrossChain(t *testing.T) {
	tests := []struct {
		swapType    CrossChainType
		isCrossChain bool
	}{
		{CrossChainTypeBitcoinToBitcoin, false},
		{CrossChainTypeEVMToEVM, false},
		{CrossChainTypeBitcoinToEVM, true},
		{CrossChainTypeEVMToBitcoin, true},
		{CrossChainTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.swapType.String(), func(t *testing.T) {
			if tt.swapType.IsCrossChain() != tt.isCrossChain {
				t.Errorf("CrossChainType.IsCrossChain() = %v, want %v",
					tt.swapType.IsCrossChain(), tt.isCrossChain)
			}
		})
	}
}

func TestCrossChainTypeInvolvesEVM(t *testing.T) {
	tests := []struct {
		swapType   CrossChainType
		involvesEVM bool
	}{
		{CrossChainTypeBitcoinToBitcoin, false},
		{CrossChainTypeEVMToEVM, true},
		{CrossChainTypeBitcoinToEVM, true},
		{CrossChainTypeEVMToBitcoin, true},
		{CrossChainTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.swapType.String(), func(t *testing.T) {
			if tt.swapType.InvolvesEVM() != tt.involvesEVM {
				t.Errorf("CrossChainType.InvolvesEVM() = %v, want %v",
					tt.swapType.InvolvesEVM(), tt.involvesEVM)
			}
		})
	}
}

func TestCrossChainTypeInvolvesBitcoin(t *testing.T) {
	tests := []struct {
		swapType       CrossChainType
		involvesBitcoin bool
	}{
		{CrossChainTypeBitcoinToBitcoin, true},
		{CrossChainTypeEVMToEVM, false},
		{CrossChainTypeBitcoinToEVM, true},
		{CrossChainTypeEVMToBitcoin, true},
		{CrossChainTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.swapType.String(), func(t *testing.T) {
			if tt.swapType.InvolvesBitcoin() != tt.involvesBitcoin {
				t.Errorf("CrossChainType.InvolvesBitcoin() = %v, want %v",
					tt.swapType.InvolvesBitcoin(), tt.involvesBitcoin)
			}
		})
	}
}

func TestEVMHTLCSessionGenerateSecret(t *testing.T) {
	session := &EVMHTLCSession{
		symbol:    "ETH",
		chainID:   1,
		network:   chain.Mainnet,
		hasSecret: false,
	}

	// Generate secret
	err := session.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	// Verify secret was set
	if !session.HasSecret() {
		t.Error("HasSecret() = false after GenerateSecret()")
	}

	// Verify secret is not zero
	secret := session.GetSecret()
	allZero := true
	for _, b := range secret {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Generated secret is all zeros")
	}

	// Verify secret hash is correct
	hash := session.GetSecretHash()
	expectedHash := HashSecretBytes(secret[:])
	for i := 0; i < 32; i++ {
		if hash[i] != expectedHash[i] {
			t.Errorf("Secret hash mismatch at byte %d: got %x, want %x", i, hash[i], expectedHash[i])
		}
	}
}

func TestEVMHTLCSessionSetSecret(t *testing.T) {
	// Set a known secret
	var testSecret [32]byte
	copy(testSecret[:], []byte("test-secret-32-bytes-exactly!!!!"))

	// Compute the expected hash
	expectedHash := HashSecretBytes(testSecret[:])
	var secretHash [32]byte
	copy(secretHash[:], expectedHash)

	session := &EVMHTLCSession{
		symbol:     "ETH",
		chainID:    1,
		network:    chain.Mainnet,
		hasSecret:  false,
		secretHash: secretHash, // Set the expected hash first
	}

	err := session.SetSecret(testSecret)
	if err != nil {
		t.Fatalf("SetSecret() error = %v", err)
	}

	// Verify secret was set
	if !session.HasSecret() {
		t.Error("HasSecret() = false after SetSecret()")
	}

	// Verify secret matches
	retrievedSecret := session.GetSecret()
	if retrievedSecret != testSecret {
		t.Error("Retrieved secret does not match set secret")
	}
}

func TestEVMHTLCSessionSetLocalKey(t *testing.T) {
	session := &EVMHTLCSession{
		symbol:  "ETH",
		chainID: 1,
		network: chain.Mainnet,
	}

	// Generate a test private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Set local key
	session.SetLocalKey(privKey)

	// Verify address was derived
	addr := session.GetLocalAddress()
	if addr == (common.Address{}) {
		t.Error("GetLocalAddress() returned zero address")
	}
}

func TestEVMHTLCSessionToStorageData(t *testing.T) {
	session := &EVMHTLCSession{
		symbol:    "ETH",
		chainID:   1,
		network:   chain.Mainnet,
		hasSecret: false,
	}

	// Generate secret
	_ = session.GenerateSecret()

	// Convert to storage data
	storageData := session.ToStorageData()

	// Verify fields
	if storageData.Symbol != "ETH" {
		t.Errorf("StorageData.Symbol = %s, want ETH", storageData.Symbol)
	}
	if storageData.ChainID != 1 {
		t.Errorf("StorageData.ChainID = %d, want 1", storageData.ChainID)
	}
	if !storageData.HasSecret {
		t.Error("StorageData.HasSecret = false, want true")
	}
	if storageData.SecretHash == "" {
		t.Error("StorageData.SecretHash is empty")
	}
}

func TestActiveSwapIsCrossChain(t *testing.T) {
	tests := []struct {
		name        string
		htlc        *HTLCSwapData
		evmHTLC     *EVMHTLCSwapData
		isCrossChain bool
	}{
		{
			name:        "Neither HTLC nor EVMHTLC",
			htlc:        nil,
			evmHTLC:     nil,
			isCrossChain: false,
		},
		{
			name:        "Only HTLC",
			htlc:        &HTLCSwapData{},
			evmHTLC:     nil,
			isCrossChain: false,
		},
		{
			name:        "Only EVMHTLC",
			htlc:        nil,
			evmHTLC:     &EVMHTLCSwapData{},
			isCrossChain: false,
		},
		{
			name:        "Both HTLC and EVMHTLC (cross-chain)",
			htlc:        &HTLCSwapData{},
			evmHTLC:     &EVMHTLCSwapData{},
			isCrossChain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveSwap{
				HTLC:    tt.htlc,
				EVMHTLC: tt.evmHTLC,
			}
			if active.IsCrossChain() != tt.isCrossChain {
				t.Errorf("IsCrossChain() = %v, want %v", active.IsCrossChain(), tt.isCrossChain)
			}
		})
	}
}

func TestActiveSwapIsEVMHTLC(t *testing.T) {
	tests := []struct {
		name      string
		evmHTLC   *EVMHTLCSwapData
		isEVMHTLC bool
	}{
		{
			name:      "No EVMHTLC",
			evmHTLC:   nil,
			isEVMHTLC: false,
		},
		{
			name:      "Has EVMHTLC",
			evmHTLC:   &EVMHTLCSwapData{},
			isEVMHTLC: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveSwap{
				EVMHTLC: tt.evmHTLC,
			}
			if active.IsEVMHTLC() != tt.isEVMHTLC {
				t.Errorf("IsEVMHTLC() = %v, want %v", active.IsEVMHTLC(), tt.isEVMHTLC)
			}
		})
	}
}
