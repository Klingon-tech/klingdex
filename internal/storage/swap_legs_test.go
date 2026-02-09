package storage

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSwapLegCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	methodData := json.RawMessage(`{"pubkey":"02abc123","pub_nonce":"03def456"}`)
	now := time.Now()
	leg := &SwapLeg{
		ID:               "leg-123",
		TradeID:          "trade-456",
		LegType:          SwapLegTypeOffer,
		Chain:            "BTC",
		Amount:           100000000,
		OurRole:          SwapLegRoleSender,
		State:            SwapLegStateInit,
		FundingAddress:   "bc1qtest...",
		TimeoutHeight:    800000,
		TimeoutTimestamp: now.Add(24 * time.Hour).Unix(),
		MethodData:       methodData,
		CreatedAt:        now,
	}

	// Create
	if err := store.CreateSwapLeg(leg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Get
	got, err := store.GetSwapLeg(leg.ID)
	if err != nil {
		t.Fatalf("GetSwapLeg() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetSwapLeg() returned nil")
	}

	if got.ID != leg.ID {
		t.Errorf("ID = %s, want %s", got.ID, leg.ID)
	}
	if got.LegType != SwapLegTypeOffer {
		t.Errorf("LegType = %s, want %s", got.LegType, SwapLegTypeOffer)
	}
	if got.Chain != "BTC" {
		t.Errorf("Chain = %s, want BTC", got.Chain)
	}
	if got.OurRole != SwapLegRoleSender {
		t.Errorf("OurRole = %s, want %s", got.OurRole, SwapLegRoleSender)
	}
	if got.TimeoutHeight != 800000 {
		t.Errorf("TimeoutHeight = %d, want 800000", got.TimeoutHeight)
	}
	if string(got.MethodData) != string(methodData) {
		t.Errorf("MethodData = %s, want %s", string(got.MethodData), string(methodData))
	}

	// Delete
	if err := store.DeleteSwapLeg(leg.ID); err != nil {
		t.Fatalf("DeleteSwapLeg() error = %v", err)
	}

	got, err = store.GetSwapLeg(leg.ID)
	if err != ErrSwapLegNotFound {
		t.Errorf("GetSwapLeg after delete should return ErrSwapLegNotFound, got %v", err)
	}
}

func TestSwapLegNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	_, err = store.GetSwapLeg("nonexistent")
	if err != ErrSwapLegNotFound {
		t.Errorf("GetSwapLeg(nonexistent) should return ErrSwapLegNotFound, got %v", err)
	}

	err = store.UpdateSwapLegState("nonexistent", SwapLegStateFunded)
	if err != ErrSwapLegNotFound {
		t.Errorf("UpdateSwapLegState(nonexistent) should return ErrSwapLegNotFound, got %v", err)
	}

	err = store.DeleteSwapLeg("nonexistent")
	if err != ErrSwapLegNotFound {
		t.Errorf("DeleteSwapLeg(nonexistent) should return ErrSwapLegNotFound, got %v", err)
	}
}

func TestSwapLegUpdates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	leg := &SwapLeg{
		ID:        "leg-update",
		TradeID:   "trade-456",
		LegType:   SwapLegTypeOffer,
		Chain:     "BTC",
		Amount:    100000000,
		OurRole:   SwapLegRoleSender,
		State:     SwapLegStateInit,
		CreatedAt: time.Now(),
	}

	if err := store.CreateSwapLeg(leg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Update state
	if err := store.UpdateSwapLegState(leg.ID, SwapLegStatePending); err != nil {
		t.Fatalf("UpdateSwapLegState() error = %v", err)
	}

	got, _ := store.GetSwapLeg(leg.ID)
	if got.State != SwapLegStatePending {
		t.Errorf("State = %s, want %s", got.State, SwapLegStatePending)
	}

	// Update funding
	if err := store.UpdateSwapLegFunding(leg.ID, "abc123txid", 0, "bc1qfunding..."); err != nil {
		t.Fatalf("UpdateSwapLegFunding() error = %v", err)
	}

	got, _ = store.GetSwapLeg(leg.ID)
	if got.FundingTxID != "abc123txid" {
		t.Errorf("FundingTxID = %s, want abc123txid", got.FundingTxID)
	}
	if got.State != SwapLegStateFunding {
		t.Errorf("State after funding = %s, want %s", got.State, SwapLegStateFunding)
	}

	// Update confirmations
	if err := store.UpdateSwapLegConfirmations(leg.ID, 3); err != nil {
		t.Fatalf("UpdateSwapLegConfirmations() error = %v", err)
	}

	got, _ = store.GetSwapLeg(leg.ID)
	if got.FundingConfirms != 3 {
		t.Errorf("FundingConfirms = %d, want 3", got.FundingConfirms)
	}

	// Update redeemed
	if err := store.UpdateSwapLegRedeemed(leg.ID, "redeem123txid"); err != nil {
		t.Fatalf("UpdateSwapLegRedeemed() error = %v", err)
	}

	got, _ = store.GetSwapLeg(leg.ID)
	if got.State != SwapLegStateRedeemed {
		t.Errorf("State = %s, want %s", got.State, SwapLegStateRedeemed)
	}
	if got.RedeemTxID != "redeem123txid" {
		t.Errorf("RedeemTxID = %s, want redeem123txid", got.RedeemTxID)
	}
}

func TestSwapLegRefunded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	leg := &SwapLeg{
		ID:        "leg-refund",
		TradeID:   "trade-456",
		LegType:   SwapLegTypeOffer,
		Chain:     "BTC",
		Amount:    100000000,
		OurRole:   SwapLegRoleSender,
		State:     SwapLegStateFunded,
		CreatedAt: time.Now(),
	}

	if err := store.CreateSwapLeg(leg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Update refunded
	if err := store.UpdateSwapLegRefunded(leg.ID, "refund123txid"); err != nil {
		t.Fatalf("UpdateSwapLegRefunded() error = %v", err)
	}

	got, _ := store.GetSwapLeg(leg.ID)
	if got.State != SwapLegStateRefunded {
		t.Errorf("State = %s, want %s", got.State, SwapLegStateRefunded)
	}
	if got.RefundTxID != "refund123txid" {
		t.Errorf("RefundTxID = %s, want refund123txid", got.RefundTxID)
	}
}

func TestSwapLegMethodData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	leg := &SwapLeg{
		ID:        "leg-method-data",
		TradeID:   "trade-456",
		LegType:   SwapLegTypeOffer,
		Chain:     "BTC",
		Amount:    100000000,
		OurRole:   SwapLegRoleSender,
		State:     SwapLegStateInit,
		CreatedAt: time.Now(),
	}

	if err := store.CreateSwapLeg(leg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Update method data
	newMethodData := json.RawMessage(`{"htlc_script":"76a914...","htlc_address":"3ABC..."}`)
	if err := store.UpdateSwapLegMethodData(leg.ID, newMethodData); err != nil {
		t.Fatalf("UpdateSwapLegMethodData() error = %v", err)
	}

	got, _ := store.GetSwapLeg(leg.ID)
	if string(got.MethodData) != string(newMethodData) {
		t.Errorf("MethodData = %s, want %s", string(got.MethodData), string(newMethodData))
	}
}

func TestGetSwapLegsByTradeID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	now := time.Now()

	// Create offer leg
	offerLeg := &SwapLeg{
		ID:        "leg-offer",
		TradeID:   "trade-123",
		LegType:   SwapLegTypeOffer,
		Chain:     "BTC",
		Amount:    100000000,
		OurRole:   SwapLegRoleSender,
		State:     SwapLegStateInit,
		CreatedAt: now,
	}
	if err := store.CreateSwapLeg(offerLeg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Create request leg
	requestLeg := &SwapLeg{
		ID:        "leg-request",
		TradeID:   "trade-123",
		LegType:   SwapLegTypeRequest,
		Chain:     "LTC",
		Amount:    5000000000,
		OurRole:   SwapLegRoleReceiver,
		State:     SwapLegStateInit,
		CreatedAt: now,
	}
	if err := store.CreateSwapLeg(requestLeg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	// Get legs by trade ID
	legs, err := store.GetSwapLegsByTradeID("trade-123")
	if err != nil {
		t.Fatalf("GetSwapLegsByTradeID() error = %v", err)
	}
	if len(legs) != 2 {
		t.Errorf("GetSwapLegsByTradeID() returned %d legs, want 2", len(legs))
	}

	// Get by trade and type
	offer, err := store.GetSwapLegByTradeAndType("trade-123", SwapLegTypeOffer)
	if err != nil {
		t.Fatalf("GetSwapLegByTradeAndType() error = %v", err)
	}
	if offer.Chain != "BTC" {
		t.Errorf("Offer leg chain = %s, want BTC", offer.Chain)
	}
}

func TestListSwapLegs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	now := time.Now()

	// Create legs for different trades
	legs := []*SwapLeg{
		{ID: "leg-1", TradeID: "trade-1", LegType: SwapLegTypeOffer, Chain: "BTC", Amount: 100000000, OurRole: SwapLegRoleSender, State: SwapLegStateInit, CreatedAt: now},
		{ID: "leg-2", TradeID: "trade-1", LegType: SwapLegTypeRequest, Chain: "LTC", Amount: 5000000000, OurRole: SwapLegRoleReceiver, State: SwapLegStateFunded, CreatedAt: now.Add(time.Second)},
		{ID: "leg-3", TradeID: "trade-2", LegType: SwapLegTypeOffer, Chain: "BTC", Amount: 50000000, OurRole: SwapLegRoleSender, State: SwapLegStateRedeemed, CreatedAt: now.Add(2 * time.Second)},
	}

	for _, leg := range legs {
		if err := store.CreateSwapLeg(leg); err != nil {
			t.Fatalf("CreateSwapLeg() error = %v", err)
		}
	}

	// List all
	all, err := store.ListSwapLegs(SwapLegFilter{})
	if err != nil {
		t.Fatalf("ListSwapLegs() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListSwapLegs() returned %d legs, want 3", len(all))
	}

	// Filter by chain
	btcLegs, err := store.ListSwapLegs(SwapLegFilter{Chain: "BTC"})
	if err != nil {
		t.Fatalf("ListSwapLegs(chain=BTC) error = %v", err)
	}
	if len(btcLegs) != 2 {
		t.Errorf("ListSwapLegs(chain=BTC) returned %d legs, want 2", len(btcLegs))
	}

	// Filter by state
	state := SwapLegStateFunded
	fundedLegs, err := store.ListSwapLegs(SwapLegFilter{State: &state})
	if err != nil {
		t.Fatalf("ListSwapLegs(state=funded) error = %v", err)
	}
	if len(fundedLegs) != 1 {
		t.Errorf("ListSwapLegs(state=funded) returned %d legs, want 1", len(fundedLegs))
	}
}

func TestGetPendingSwapLegs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	now := time.Now()

	// Create pending legs
	pendingLegs := []*SwapLeg{
		{ID: "leg-pending-1", TradeID: "trade-1", LegType: SwapLegTypeOffer, Chain: "BTC", Amount: 100000000, OurRole: SwapLegRoleSender, State: SwapLegStateInit, CreatedAt: now},
		{ID: "leg-pending-2", TradeID: "trade-1", LegType: SwapLegTypeRequest, Chain: "LTC", Amount: 5000000000, OurRole: SwapLegRoleReceiver, State: SwapLegStateFunding, CreatedAt: now.Add(time.Second)},
	}

	// Create completed leg
	completedLeg := &SwapLeg{
		ID: "leg-completed", TradeID: "trade-2", LegType: SwapLegTypeOffer, Chain: "BTC",
		Amount: 50000000, OurRole: SwapLegRoleSender, State: SwapLegStateRedeemed, CreatedAt: now.Add(2 * time.Second),
	}

	for _, leg := range pendingLegs {
		if err := store.CreateSwapLeg(leg); err != nil {
			t.Fatalf("CreateSwapLeg() error = %v", err)
		}
	}
	if err := store.CreateSwapLeg(completedLeg); err != nil {
		t.Fatalf("CreateSwapLeg() error = %v", err)
	}

	pending, err := store.GetPendingSwapLegs()
	if err != nil {
		t.Fatalf("GetPendingSwapLegs() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("GetPendingSwapLegs() returned %d legs, want 2", len(pending))
	}
}

func TestDeleteSwapLegsByTradeID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-swap-legs-test-*")
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

	now := time.Now()

	// Create legs for trade-1
	for i := 0; i < 2; i++ {
		leg := &SwapLeg{
			ID: "leg-" + string(rune('A'+i)), TradeID: "trade-1", LegType: SwapLegTypeOffer,
			Chain: "BTC", Amount: 100000000, OurRole: SwapLegRoleSender, State: SwapLegStateInit, CreatedAt: now,
		}
		if err := store.CreateSwapLeg(leg); err != nil {
			t.Fatalf("CreateSwapLeg() error = %v", err)
		}
	}

	// Delete by trade ID
	if err := store.DeleteSwapLegsByTradeID("trade-1"); err != nil {
		t.Fatalf("DeleteSwapLegsByTradeID() error = %v", err)
	}

	legs, err := store.GetSwapLegsByTradeID("trade-1")
	if err != nil {
		t.Fatalf("GetSwapLegsByTradeID() error = %v", err)
	}
	if len(legs) != 0 {
		t.Errorf("GetSwapLegsByTradeID() after delete returned %d legs, want 0", len(legs))
	}
}
