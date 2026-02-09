package storage

import (
	"encoding/json"
	"os"
	"testing"
)

// createTestSwapRecord creates a test swap record with sensible defaults.
func createTestSwapRecord(tradeID string) *SwapRecord {
	return &SwapRecord{
		TradeID:       tradeID,
		OrderID:       "order-" + tradeID,
		MakerPeerID:   "12D3KooWMaker123",
		TakerPeerID:   "12D3KooWTaker456",
		OurRole:       "maker",
		IsMaker:       true,
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 10000000,
		State:         SwapStateInit,
		MethodData:    json.RawMessage(`{"test": "data"}`),
	}
}

func TestSwapCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swap
	swap := createTestSwapRecord("trade-001")

	// Save swap
	if err := store.SaveSwap(swap); err != nil {
		t.Fatalf("SaveSwap() error = %v", err)
	}

	// Get swap
	got, err := store.GetSwap("trade-001")
	if err != nil {
		t.Fatalf("GetSwap() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetSwap() returned nil")
	}

	// Verify fields
	if got.TradeID != swap.TradeID {
		t.Errorf("TradeID = %s, want %s", got.TradeID, swap.TradeID)
	}
	if got.OrderID != swap.OrderID {
		t.Errorf("OrderID = %s, want %s", got.OrderID, swap.OrderID)
	}
	if got.State != SwapStateInit {
		t.Errorf("State = %s, want %s", got.State, SwapStateInit)
	}
	if got.IsMaker != true {
		t.Error("IsMaker should be true")
	}
	if got.OfferChain != "BTC" {
		t.Errorf("OfferChain = %s, want BTC", got.OfferChain)
	}
	if got.OfferAmount != 100000 {
		t.Errorf("OfferAmount = %d, want 100000", got.OfferAmount)
	}

	// Update swap (save again should update)
	swap.State = SwapStateFunding
	swap.LocalFundingTxID = "abc123def456"
	swap.LocalFundingVout = 0
	if err := store.SaveSwap(swap); err != nil {
		t.Fatalf("SaveSwap() update error = %v", err)
	}

	got, err = store.GetSwap("trade-001")
	if err != nil {
		t.Fatalf("GetSwap() after update error = %v", err)
	}

	if got.State != SwapStateFunding {
		t.Errorf("State = %s, want %s", got.State, SwapStateFunding)
	}
	if got.LocalFundingTxID != "abc123def456" {
		t.Errorf("LocalFundingTxID = %s, want abc123def456", got.LocalFundingTxID)
	}

	// Delete swap
	if err := store.DeleteSwap("trade-001"); err != nil {
		t.Fatalf("DeleteSwap() error = %v", err)
	}

	got, err = store.GetSwap("trade-001")
	if err != nil && err != ErrSwapNotFound {
		t.Fatalf("GetSwap() after delete error = %v", err)
	}
	if got != nil {
		t.Error("swap should be nil after delete")
	}
}

func TestGetPendingSwaps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swaps with different states
	pending1 := createTestSwapRecord("pending-001")
	pending1.State = SwapStateInit
	store.SaveSwap(pending1)

	pending2 := createTestSwapRecord("pending-002")
	pending2.State = SwapStateFunding
	store.SaveSwap(pending2)

	pending3 := createTestSwapRecord("pending-003")
	pending3.State = SwapStateFunded
	store.SaveSwap(pending3)

	// Terminal states (should not be returned)
	completed := createTestSwapRecord("completed-001")
	completed.State = SwapStateRedeemed
	store.SaveSwap(completed)

	refunded := createTestSwapRecord("refunded-001")
	refunded.State = SwapStateRefunded
	store.SaveSwap(refunded)

	failed := createTestSwapRecord("failed-001")
	failed.State = SwapStateFailed
	store.SaveSwap(failed)

	// Get pending swaps
	pending, err := store.GetPendingSwaps()
	if err != nil {
		t.Fatalf("GetPendingSwaps() error = %v", err)
	}

	// Should return only the 3 non-terminal swaps
	if len(pending) != 3 {
		t.Errorf("GetPendingSwaps() returned %d swaps, want 3", len(pending))
	}

	// Verify none of them are terminal
	for _, s := range pending {
		switch s.State {
		case SwapStateRedeemed, SwapStateRefunded, SwapStateFailed, SwapStateCancelled:
			t.Errorf("GetPendingSwaps() returned terminal state: %s", s.State)
		}
	}
}

func TestGetSwapsNearingTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	currentHeight := uint32(1000)
	safetyMargin := uint32(10)

	// Swap nearing timeout (within safety margin)
	nearTimeout := createTestSwapRecord("near-timeout")
	nearTimeout.State = SwapStateFunded
	nearTimeout.TimeoutHeight = 1005 // Within current + safety margin
	store.SaveSwap(nearTimeout)

	// Swap far from timeout
	farFromTimeout := createTestSwapRecord("far-timeout")
	farFromTimeout.State = SwapStateFunded
	farFromTimeout.TimeoutHeight = 2000 // Way beyond threshold
	store.SaveSwap(farFromTimeout)

	// Swap in wrong state (should not be included)
	wrongState := createTestSwapRecord("wrong-state")
	wrongState.State = SwapStateInit
	wrongState.TimeoutHeight = 1005
	store.SaveSwap(wrongState)

	// Get swaps nearing timeout
	swaps, err := store.GetSwapsNearingTimeout(currentHeight, safetyMargin)
	if err != nil {
		t.Fatalf("GetSwapsNearingTimeout() error = %v", err)
	}

	// Should return only the one nearing timeout in funded/signing state
	if len(swaps) != 1 {
		t.Errorf("GetSwapsNearingTimeout() returned %d swaps, want 1", len(swaps))
	}

	if len(swaps) > 0 && swaps[0].TradeID != "near-timeout" {
		t.Errorf("Expected near-timeout swap, got %s", swaps[0].TradeID)
	}
}

func TestGetSwapsPastTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	currentHeight := uint32(1000)

	// Swap past timeout
	pastTimeout := createTestSwapRecord("past-timeout")
	pastTimeout.State = SwapStateFunded
	pastTimeout.TimeoutHeight = 900 // Past current height
	store.SaveSwap(pastTimeout)

	// Swap not past timeout
	notPastTimeout := createTestSwapRecord("not-past-timeout")
	notPastTimeout.State = SwapStateFunded
	notPastTimeout.TimeoutHeight = 1100 // Future
	store.SaveSwap(notPastTimeout)

	// Swap past timeout but already completed
	completedPast := createTestSwapRecord("completed-past")
	completedPast.State = SwapStateRedeemed
	completedPast.TimeoutHeight = 800
	store.SaveSwap(completedPast)

	// Get swaps past timeout
	swaps, err := store.GetSwapsPastTimeout(currentHeight)
	if err != nil {
		t.Fatalf("GetSwapsPastTimeout() error = %v", err)
	}

	// Should return only the one past timeout in active state
	if len(swaps) != 1 {
		t.Errorf("GetSwapsPastTimeout() returned %d swaps, want 1", len(swaps))
	}

	if len(swaps) > 0 && swaps[0].TradeID != "past-timeout" {
		t.Errorf("Expected past-timeout swap, got %s", swaps[0].TradeID)
	}
}

func TestUpdateSwapState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swap
	swap := createTestSwapRecord("state-test")
	store.SaveSwap(swap)

	// Update to funding state
	if err := store.UpdateSwapState("state-test", SwapStateFunding); err != nil {
		t.Fatalf("UpdateSwapState(funding) error = %v", err)
	}

	got, _ := store.GetSwap("state-test")
	if got.State != SwapStateFunding {
		t.Errorf("State = %s, want %s", got.State, SwapStateFunding)
	}
	if !got.CompletedAt.IsZero() {
		t.Error("CompletedAt should be zero for non-terminal state")
	}

	// Update to terminal state
	if err := store.UpdateSwapState("state-test", SwapStateRedeemed); err != nil {
		t.Fatalf("UpdateSwapState(redeemed) error = %v", err)
	}

	got, _ = store.GetSwap("state-test")
	if got.State != SwapStateRedeemed {
		t.Errorf("State = %s, want %s", got.State, SwapStateRedeemed)
	}
	if got.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set for terminal state")
	}

	// Update non-existent swap
	err = store.UpdateSwapState("non-existent", SwapStateFunding)
	if err != ErrSwapNotFound {
		t.Errorf("UpdateSwapState(non-existent) error = %v, want ErrSwapNotFound", err)
	}
}

func TestUpdateSwapMethodData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swap
	swap := createTestSwapRecord("method-data-test")
	store.SaveSwap(swap)

	// Update method data
	newData := json.RawMessage(`{"pubkey": "abc123", "nonce": "def456"}`)
	if err := store.UpdateSwapMethodData("method-data-test", newData); err != nil {
		t.Fatalf("UpdateSwapMethodData() error = %v", err)
	}

	got, _ := store.GetSwap("method-data-test")
	if string(got.MethodData) != string(newData) {
		t.Errorf("MethodData = %s, want %s", got.MethodData, newData)
	}

	// Update non-existent swap
	err = store.UpdateSwapMethodData("non-existent", newData)
	if err != ErrSwapNotFound {
		t.Errorf("UpdateSwapMethodData(non-existent) error = %v, want ErrSwapNotFound", err)
	}
}

func TestUpdateSwapFunding(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swap
	swap := createTestSwapRecord("funding-test")
	store.SaveSwap(swap)

	// Update local funding
	if err := store.UpdateSwapFunding("funding-test", true, "local-tx-123", 0); err != nil {
		t.Fatalf("UpdateSwapFunding(local) error = %v", err)
	}

	got, _ := store.GetSwap("funding-test")
	if got.LocalFundingTxID != "local-tx-123" {
		t.Errorf("LocalFundingTxID = %s, want local-tx-123", got.LocalFundingTxID)
	}
	if got.LocalFundingVout != 0 {
		t.Errorf("LocalFundingVout = %d, want 0", got.LocalFundingVout)
	}

	// Update remote funding
	if err := store.UpdateSwapFunding("funding-test", false, "remote-tx-456", 1); err != nil {
		t.Fatalf("UpdateSwapFunding(remote) error = %v", err)
	}

	got, _ = store.GetSwap("funding-test")
	if got.RemoteFundingTxID != "remote-tx-456" {
		t.Errorf("RemoteFundingTxID = %s, want remote-tx-456", got.RemoteFundingTxID)
	}
	if got.RemoteFundingVout != 1 {
		t.Errorf("RemoteFundingVout = %d, want 1", got.RemoteFundingVout)
	}

	// Update non-existent swap
	err = store.UpdateSwapFunding("non-existent", true, "tx", 0)
	if err != ErrSwapNotFound {
		t.Errorf("UpdateSwapFunding(non-existent) error = %v, want ErrSwapNotFound", err)
	}
}

func TestListSwaps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swaps
	for i := 0; i < 5; i++ {
		swap := createTestSwapRecord("list-" + string(rune('A'+i)))
		if i < 3 {
			swap.State = SwapStateFunding
		} else {
			swap.State = SwapStateRedeemed
		}
		store.SaveSwap(swap)
	}

	// List without completed
	swaps, err := store.ListSwaps(100, false)
	if err != nil {
		t.Fatalf("ListSwaps(false) error = %v", err)
	}
	if len(swaps) != 3 {
		t.Errorf("ListSwaps(false) returned %d swaps, want 3", len(swaps))
	}

	// List with completed
	swaps, err = store.ListSwaps(100, true)
	if err != nil {
		t.Fatalf("ListSwaps(true) error = %v", err)
	}
	if len(swaps) != 5 {
		t.Errorf("ListSwaps(true) returned %d swaps, want 5", len(swaps))
	}

	// List with limit
	swaps, err = store.ListSwaps(2, true)
	if err != nil {
		t.Fatalf("ListSwaps(limit=2) error = %v", err)
	}
	if len(swaps) != 2 {
		t.Errorf("ListSwaps(limit=2) returned %d swaps, want 2", len(swaps))
	}
}

func TestSwapStates(t *testing.T) {
	// Verify state constants
	states := []SwapState{
		SwapStateInit,
		SwapStateFunding,
		SwapStateFunded,
		SwapStateSigning,
		SwapStateRedeemed,
		SwapStateRefunded,
		SwapStateFailed,
		SwapStateCancelled,
	}

	for _, s := range states {
		if s == "" {
			t.Error("SwapState constant should not be empty")
		}
	}

	// Verify terminal states
	terminalStates := []SwapState{SwapStateRedeemed, SwapStateRefunded, SwapStateFailed, SwapStateCancelled}
	for _, s := range terminalStates {
		if !isTerminalState(s) {
			t.Errorf("%s should be a terminal state", s)
		}
	}

	// Verify non-terminal states
	nonTerminalStates := []SwapState{SwapStateInit, SwapStateFunding, SwapStateFunded, SwapStateSigning}
	for _, s := range nonTerminalStates {
		if isTerminalState(s) {
			t.Errorf("%s should not be a terminal state", s)
		}
	}
}

func TestSwapTimestamps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create swap without timestamps (should be auto-set)
	swap := createTestSwapRecord("timestamp-test")
	store.SaveSwap(swap)

	got, _ := store.GetSwap("timestamp-test")

	// CreatedAt should be set automatically
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set automatically")
	}

	// UpdatedAt should be set automatically
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set automatically")
	}

	// CreatedAt should equal UpdatedAt on initial save
	if got.CreatedAt.Unix() != got.UpdatedAt.Unix() {
		t.Logf("Note: CreatedAt (%d) and UpdatedAt (%d) differ slightly",
			got.CreatedAt.Unix(), got.UpdatedAt.Unix())
	}

	// Update state and verify UpdatedAt changes
	initialUpdatedAt := got.UpdatedAt.Unix()
	store.UpdateSwapState("timestamp-test", SwapStateFunding)

	got, _ = store.GetSwap("timestamp-test")

	// UpdatedAt should be >= initial
	if got.UpdatedAt.Unix() < initialUpdatedAt {
		t.Errorf("UpdatedAt should not decrease: was %d, now %d",
			initialUpdatedAt, got.UpdatedAt.Unix())
	}
}

func TestSwapNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Get non-existent swap
	got, err := store.GetSwap("non-existent")
	if err != nil && err != ErrSwapNotFound {
		t.Fatalf("GetSwap(non-existent) unexpected error = %v", err)
	}
	if got != nil {
		t.Error("GetSwap(non-existent) should return nil")
	}
}
