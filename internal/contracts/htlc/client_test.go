// Package htlc provides tests for the KlingonHTLC client wrapper.
//
// Integration tests require a local Anvil node running with the contract deployed:
//
//	cd contracts && anvil &
//	forge script script/Deploy.s.sol --rpc-url http://localhost:8545 --broadcast
//
// Then run tests with:
//
//	go test -v ./internal/contracts/htlc/... -run TestIntegration
package htlc

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// =============================================================================
// Unit Tests (no network required)
// =============================================================================

func TestGenerateSecret(t *testing.T) {
	secret1, hash1, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	// Secret should not be all zeros
	allZero := true
	for _, b := range secret1 {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Secret is all zeros")
	}

	// Hash should not be all zeros
	allZero = true
	for _, b := range hash1 {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Hash is all zeros")
	}

	// Generate another secret - should be different
	secret2, hash2, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	if secret1 == secret2 {
		t.Error("Two generated secrets are identical")
	}
	if hash1 == hash2 {
		t.Error("Two generated hashes are identical")
	}
}

func TestHashSecret(t *testing.T) {
	secret := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	hash := HashSecret(secret)

	// Hash should be deterministic
	hash2 := HashSecret(secret)
	if hash != hash2 {
		t.Error("HashSecret is not deterministic")
	}

	// Different secret should produce different hash
	secret2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	hash3 := HashSecret(secret2)
	if hash == hash3 {
		t.Error("Different secrets produced same hash")
	}
}

func TestVerifySecret(t *testing.T) {
	secret, hash, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	// Correct secret should verify
	if !VerifySecret(secret, hash) {
		t.Error("VerifySecret returned false for correct secret")
	}

	// Wrong secret should not verify
	wrongSecret := [32]byte{}
	if VerifySecret(wrongSecret, hash) {
		t.Error("VerifySecret returned true for wrong secret")
	}

	// Wrong hash should not verify
	wrongHash := [32]byte{}
	if VerifySecret(secret, wrongHash) {
		t.Error("VerifySecret returned true for wrong hash")
	}
}

func TestSwapState(t *testing.T) {
	tests := []struct {
		state    SwapState
		expected string
	}{
		{SwapStateEmpty, "empty"},
		{SwapStateActive, "active"},
		{SwapStateClaimed, "claimed"},
		{SwapStateRefunded, "refunded"},
		{SwapState(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.state.String() != tc.expected {
			t.Errorf("SwapState(%d).String() = %s, want %s", tc.state, tc.state.String(), tc.expected)
		}
	}
}

func TestSwapIsNativeToken(t *testing.T) {
	swap := &Swap{Token: common.Address{}}
	if !swap.IsNativeToken() {
		t.Error("IsNativeToken should return true for zero address")
	}

	swap.Token = common.HexToAddress("0x1234567890123456789012345678901234567890")
	if swap.IsNativeToken() {
		t.Error("IsNativeToken should return false for non-zero address")
	}
}

func TestSwapIsActive(t *testing.T) {
	swap := &Swap{State: SwapStateActive}
	if !swap.IsActive() {
		t.Error("IsActive should return true for active state")
	}

	swap.State = SwapStateEmpty
	if swap.IsActive() {
		t.Error("IsActive should return false for empty state")
	}

	swap.State = SwapStateClaimed
	if swap.IsActive() {
		t.Error("IsActive should return false for claimed state")
	}
}

func TestAddressFromPrivateKey(t *testing.T) {
	// Use a known private key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	addr := AddressFromPrivateKey(privateKey)
	expected := crypto.PubkeyToAddress(privateKey.PublicKey)

	if addr != expected {
		t.Errorf("AddressFromPrivateKey = %s, want %s", addr.Hex(), expected.Hex())
	}
}

func TestParsePrivateKey(t *testing.T) {
	// Generate a key and get its hex
	originalKey, _ := crypto.GenerateKey()
	hexKey := common.Bytes2Hex(crypto.FromECDSA(originalKey))

	// Parse it back
	parsedKey, err := ParsePrivateKey(hexKey)
	if err != nil {
		t.Fatalf("ParsePrivateKey failed: %v", err)
	}

	// Addresses should match
	originalAddr := crypto.PubkeyToAddress(originalKey.PublicKey)
	parsedAddr := crypto.PubkeyToAddress(parsedKey.PublicKey)
	if originalAddr != parsedAddr {
		t.Errorf("Parsed key address = %s, want %s", parsedAddr.Hex(), originalAddr.Hex())
	}

	// Invalid hex should fail
	_, err = ParsePrivateKey("invalid")
	if err == nil {
		t.Error("ParsePrivateKey should fail for invalid hex")
	}
}

// =============================================================================
// Integration Tests (require Anvil node)
// =============================================================================

// testConfig holds test configuration
type testConfig struct {
	rpcURL          string
	contractAddress common.Address
	deployerKey     *ecdsa.PrivateKey
	userKey         *ecdsa.PrivateKey
	daoAddress      common.Address
}

// getTestConfig returns test configuration from environment or defaults
func getTestConfig(t *testing.T) *testConfig {
	t.Helper()

	rpcURL := os.Getenv("TEST_RPC_URL")
	if rpcURL == "" {
		rpcURL = "http://localhost:8545"
	}

	contractAddr := os.Getenv("TEST_CONTRACT_ADDRESS")
	if contractAddr == "" {
		t.Skip("TEST_CONTRACT_ADDRESS not set, skipping integration test")
	}

	// Anvil default private keys
	// Account 0: 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
	// Account 1: 0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
	deployerKeyHex := os.Getenv("TEST_DEPLOYER_KEY")
	if deployerKeyHex == "" {
		deployerKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	}
	deployerKey, err := crypto.HexToECDSA(deployerKeyHex)
	if err != nil {
		t.Fatalf("Invalid deployer key: %v", err)
	}

	userKeyHex := os.Getenv("TEST_USER_KEY")
	if userKeyHex == "" {
		userKeyHex = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	}
	userKey, err := crypto.HexToECDSA(userKeyHex)
	if err != nil {
		t.Fatalf("Invalid user key: %v", err)
	}

	daoAddr := os.Getenv("TEST_DAO_ADDRESS")
	if daoAddr == "" {
		daoAddr = crypto.PubkeyToAddress(deployerKey.PublicKey).Hex()
	}

	return &testConfig{
		rpcURL:          rpcURL,
		contractAddress: common.HexToAddress(contractAddr),
		deployerKey:     deployerKey,
		userKey:         userKey,
		daoAddress:      common.HexToAddress(daoAddr),
	}
}

func TestIntegrationNewClient(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	if client.ChainID() == nil {
		t.Error("ChainID is nil")
	}

	if client.ContractAddress() != cfg.contractAddress {
		t.Errorf("ContractAddress = %s, want %s", client.ContractAddress().Hex(), cfg.contractAddress.Hex())
	}
}

func TestIntegrationComputeSwapID(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)
	token := common.Address{} // Native token
	amount := big.NewInt(1e18)
	_, secretHash, _ := GenerateSecret()
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	nonce := big.NewInt(1)

	swapID1, err := client.ComputeSwapID(ctx, sender, receiver, token, amount, secretHash, timelock, nonce)
	if err != nil {
		t.Fatalf("ComputeSwapID failed: %v", err)
	}

	// Same params should produce same ID
	swapID2, err := client.ComputeSwapID(ctx, sender, receiver, token, amount, secretHash, timelock, nonce)
	if err != nil {
		t.Fatalf("ComputeSwapID failed: %v", err)
	}
	if swapID1 != swapID2 {
		t.Error("ComputeSwapID is not deterministic")
	}

	// Different nonce should produce different ID
	swapID3, err := client.ComputeSwapID(ctx, sender, receiver, token, amount, secretHash, timelock, big.NewInt(2))
	if err != nil {
		t.Fatalf("ComputeSwapID failed: %v", err)
	}
	if swapID1 == swapID3 {
		t.Error("Different nonce produced same swap ID")
	}
}

func TestIntegrationCreateAndClaimNativeSwap(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)

	// Generate secret
	secret, secretHash, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	// Compute swap ID
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	nonce := big.NewInt(time.Now().UnixNano())
	amount := big.NewInt(1e16) // 0.01 ETH

	swapID, err := client.ComputeSwapID(ctx, sender, receiver, common.Address{}, amount, secretHash, timelock, nonce)
	if err != nil {
		t.Fatalf("ComputeSwapID failed: %v", err)
	}

	// Create swap
	tx, err := client.CreateSwapNative(ctx, cfg.deployerKey, swapID, receiver, secretHash, timelock, amount)
	if err != nil {
		t.Fatalf("CreateSwapNative failed: %v", err)
	}

	// Wait for tx
	receipt, err := client.WaitForTx(ctx, tx)
	if err != nil {
		t.Fatalf("WaitForTx failed: %v", err)
	}
	if receipt.Status != 1 {
		t.Fatal("CreateSwapNative transaction failed")
	}

	t.Logf("Created swap %x in tx %s", swapID, tx.Hash().Hex())

	// Verify swap exists
	swap, err := client.GetSwap(ctx, swapID)
	if err != nil {
		t.Fatalf("GetSwap failed: %v", err)
	}
	if swap.State != SwapStateActive {
		t.Errorf("Swap state = %s, want active", swap.State)
	}
	if swap.Sender != sender {
		t.Errorf("Swap sender = %s, want %s", swap.Sender.Hex(), sender.Hex())
	}
	if swap.Receiver != receiver {
		t.Errorf("Swap receiver = %s, want %s", swap.Receiver.Hex(), receiver.Hex())
	}
	if !swap.IsNativeToken() {
		t.Error("Swap should be native token")
	}

	// Check can claim
	canClaim, err := client.CanClaim(ctx, swapID)
	if err != nil {
		t.Fatalf("CanClaim failed: %v", err)
	}
	if !canClaim {
		t.Error("CanClaim should return true")
	}

	// Claim the swap (receiver claims)
	claimTx, err := client.Claim(ctx, cfg.userKey, swapID, secret)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	claimReceipt, err := client.WaitForTx(ctx, claimTx)
	if err != nil {
		t.Fatalf("WaitForTx for claim failed: %v", err)
	}
	if claimReceipt.Status != 1 {
		t.Fatal("Claim transaction failed")
	}

	t.Logf("Claimed swap in tx %s", claimTx.Hash().Hex())

	// Verify swap is claimed
	swap, err = client.GetSwap(ctx, swapID)
	if err != nil {
		t.Fatalf("GetSwap after claim failed: %v", err)
	}
	if swap.State != SwapStateClaimed {
		t.Errorf("Swap state = %s, want claimed", swap.State)
	}

	// Extract secret from claim tx
	extractedSecret, err := client.GetSecretFromClaim(ctx, claimTx.Hash())
	if err != nil {
		t.Fatalf("GetSecretFromClaim failed: %v", err)
	}
	if extractedSecret != secret {
		t.Error("Extracted secret does not match original")
	}
}

func TestIntegrationCreateAndRefundNativeSwap(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)

	// Generate secret
	_, secretHash, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	// Set timelock to past (for immediate refund)
	timelock := big.NewInt(time.Now().Add(-1 * time.Second).Unix())
	nonce := big.NewInt(time.Now().UnixNano())
	amount := big.NewInt(1e16) // 0.01 ETH

	swapID, err := client.ComputeSwapID(ctx, sender, receiver, common.Address{}, amount, secretHash, timelock, nonce)
	if err != nil {
		t.Fatalf("ComputeSwapID failed: %v", err)
	}

	// Create swap with past timelock
	tx, err := client.CreateSwapNative(ctx, cfg.deployerKey, swapID, receiver, secretHash, timelock, amount)
	if err != nil {
		t.Fatalf("CreateSwapNative failed: %v", err)
	}

	receipt, err := client.WaitForTx(ctx, tx)
	if err != nil {
		t.Fatalf("WaitForTx failed: %v", err)
	}
	if receipt.Status != 1 {
		t.Fatal("CreateSwapNative transaction failed")
	}

	t.Logf("Created swap %x with expired timelock", swapID)

	// Check can refund
	canRefund, err := client.CanRefund(ctx, swapID)
	if err != nil {
		t.Fatalf("CanRefund failed: %v", err)
	}
	if !canRefund {
		t.Error("CanRefund should return true for expired swap")
	}

	// Refund the swap (sender refunds)
	refundTx, err := client.Refund(ctx, cfg.deployerKey, swapID)
	if err != nil {
		t.Fatalf("Refund failed: %v", err)
	}

	refundReceipt, err := client.WaitForTx(ctx, refundTx)
	if err != nil {
		t.Fatalf("WaitForTx for refund failed: %v", err)
	}
	if refundReceipt.Status != 1 {
		t.Fatal("Refund transaction failed")
	}

	t.Logf("Refunded swap in tx %s", refundTx.Hash().Hex())

	// Verify swap is refunded
	swap, err := client.GetSwap(ctx, swapID)
	if err != nil {
		t.Fatalf("GetSwap after refund failed: %v", err)
	}
	if swap.State != SwapStateRefunded {
		t.Errorf("Swap state = %s, want refunded", swap.State)
	}
}

func TestIntegrationGetSwapEvents(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Get current block
	ethClient, _ := ethclient.Dial(cfg.rpcURL)
	defer ethClient.Close()
	blockNum, err := ethClient.BlockNumber(ctx)
	if err != nil {
		t.Fatalf("BlockNumber failed: %v", err)
	}

	// Create a swap to generate an event
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)
	secret, secretHash, _ := GenerateSecret()
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	nonce := big.NewInt(time.Now().UnixNano())
	amount := big.NewInt(1e16)

	swapID, _ := client.ComputeSwapID(ctx, sender, receiver, common.Address{}, amount, secretHash, timelock, nonce)

	tx, err := client.CreateSwapNative(ctx, cfg.deployerKey, swapID, receiver, secretHash, timelock, amount)
	if err != nil {
		t.Fatalf("CreateSwapNative failed: %v", err)
	}
	client.WaitForTx(ctx, tx)

	// Query events
	events, err := client.GetSwapCreatedEvents(ctx, blockNum, blockNum+10, [][32]byte{swapID})
	if err != nil {
		t.Fatalf("GetSwapCreatedEvents failed: %v", err)
	}

	if len(events) == 0 {
		t.Error("Expected at least one SwapCreated event")
	} else {
		event := events[0]
		if event.SwapID != swapID {
			t.Errorf("Event swapID = %x, want %x", event.SwapID, swapID)
		}
		if event.Sender != sender {
			t.Errorf("Event sender = %s, want %s", event.Sender.Hex(), sender.Hex())
		}
		t.Logf("Found SwapCreated event in block %d, tx %s", event.BlockNum, event.TxHash.Hex())
	}

	// Claim the swap
	claimTx, _ := client.Claim(ctx, cfg.userKey, swapID, secret)
	client.WaitForTx(ctx, claimTx)

	// Query claim events
	claimEvents, err := client.GetSwapClaimedEvents(ctx, blockNum, blockNum+20, [][32]byte{swapID})
	if err != nil {
		t.Fatalf("GetSwapClaimedEvents failed: %v", err)
	}

	if len(claimEvents) == 0 {
		t.Error("Expected at least one SwapClaimed event")
	} else {
		event := claimEvents[0]
		if event.SwapID != swapID {
			t.Errorf("Event swapID = %x, want %x", event.SwapID, swapID)
		}
		if event.Secret != secret {
			t.Error("Event secret does not match")
		}
		t.Logf("Found SwapClaimed event with secret in block %d", event.BlockNum)
	}
}

func TestIntegrationContractViewFunctions(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test GetDaoAddress
	daoAddr, err := client.GetDaoAddress(ctx)
	if err != nil {
		t.Fatalf("GetDaoAddress failed: %v", err)
	}
	t.Logf("DAO address: %s", daoAddr.Hex())

	// Test GetFeeBps
	feeBps, err := client.GetFeeBps(ctx)
	if err != nil {
		t.Fatalf("GetFeeBps failed: %v", err)
	}
	t.Logf("Fee: %d bps", feeBps.Int64())
	if feeBps.Cmp(big.NewInt(0)) < 0 || feeBps.Cmp(big.NewInt(10000)) > 0 {
		t.Errorf("Fee %d is out of valid range (0-10000)", feeBps.Int64())
	}

	// Test IsPaused
	paused, err := client.IsPaused(ctx)
	if err != nil {
		t.Fatalf("IsPaused failed: %v", err)
	}
	t.Logf("Contract paused: %v", paused)
}

func TestIntegrationEstimateGas(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)
	_, secretHash, _ := GenerateSecret()
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	nonce := big.NewInt(time.Now().UnixNano())
	amount := big.NewInt(1e16)

	swapID, _ := client.ComputeSwapID(ctx, sender, receiver, common.Address{}, amount, secretHash, timelock, nonce)

	gas, err := client.EstimateGasCreateSwapNative(ctx, sender, swapID, receiver, secretHash, timelock, amount)
	if err != nil {
		t.Fatalf("EstimateGasCreateSwapNative failed: %v", err)
	}

	t.Logf("Estimated gas for CreateSwapNative: %d", gas)

	// Gas should be reasonable (between 50k and 200k)
	if gas < 50000 || gas > 200000 {
		t.Errorf("Gas estimate %d seems unreasonable", gas)
	}
}

func TestIntegrationTimeUntilRefund(t *testing.T) {
	cfg := getTestConfig(t)

	client, err := NewClient(cfg.rpcURL, cfg.contractAddress)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	sender := crypto.PubkeyToAddress(cfg.deployerKey.PublicKey)
	receiver := crypto.PubkeyToAddress(cfg.userKey.PublicKey)
	_, secretHash, _ := GenerateSecret()

	// Create swap with 1 hour timelock
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	nonce := big.NewInt(time.Now().UnixNano())
	amount := big.NewInt(1e16)

	swapID, _ := client.ComputeSwapID(ctx, sender, receiver, common.Address{}, amount, secretHash, timelock, nonce)

	tx, err := client.CreateSwapNative(ctx, cfg.deployerKey, swapID, receiver, secretHash, timelock, amount)
	if err != nil {
		t.Fatalf("CreateSwapNative failed: %v", err)
	}
	client.WaitForTx(ctx, tx)

	// Check time until refund
	remaining, err := client.TimeUntilRefund(ctx, swapID)
	if err != nil {
		t.Fatalf("TimeUntilRefund failed: %v", err)
	}

	t.Logf("Time until refund: %d seconds", remaining.Int64())

	// Should be roughly 1 hour (3600 seconds, give or take a few)
	if remaining.Cmp(big.NewInt(3500)) < 0 || remaining.Cmp(big.NewInt(3700)) > 0 {
		t.Errorf("TimeUntilRefund %d is not close to 3600", remaining.Int64())
	}

	// Clean up - refund won't work since timelock hasn't expired
	// This swap will remain on chain (test cleanup)
}
