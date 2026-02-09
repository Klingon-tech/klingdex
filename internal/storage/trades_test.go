package storage

import (
	"os"
	"testing"
	"time"
)

func TestTradeCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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
	trade := &Trade{
		ID:            "trade-123",
		OrderID:       "order-456",
		MakerPeerID:   "12D3KooWMaker",
		TakerPeerID:   "12D3KooWTaker",
		OurRole:       TradeRoleMaker,
		Method:        "musig2",
		State:         TradeStateInit,
		OfferChain:    "BTC",
		OfferAmount:   100000000,
		RequestChain:  "LTC",
		RequestAmount: 5000000000,
		CreatedAt:     now,
	}

	// Create
	if err := store.CreateTrade(trade); err != nil {
		t.Fatalf("CreateTrade() error = %v", err)
	}

	// Get
	got, err := store.GetTrade(trade.ID)
	if err != nil {
		t.Fatalf("GetTrade() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetTrade() returned nil")
	}

	if got.ID != trade.ID {
		t.Errorf("ID = %s, want %s", got.ID, trade.ID)
	}
	if got.State != TradeStateInit {
		t.Errorf("State = %s, want %s", got.State, TradeStateInit)
	}
	if got.OurRole != TradeRoleMaker {
		t.Errorf("OurRole = %s, want %s", got.OurRole, TradeRoleMaker)
	}
	if got.Method != "musig2" {
		t.Errorf("Method = %s, want musig2", got.Method)
	}

	// Get by order ID
	byOrder, err := store.GetTradeByOrderID("order-456")
	if err != nil {
		t.Fatalf("GetTradeByOrderID() error = %v", err)
	}
	if byOrder.ID != trade.ID {
		t.Errorf("GetTradeByOrderID() ID = %s, want %s", byOrder.ID, trade.ID)
	}

	// Update state
	if err := store.UpdateTradeState(trade.ID, TradeStateFunded); err != nil {
		t.Fatalf("UpdateTradeState() error = %v", err)
	}

	got, _ = store.GetTrade(trade.ID)
	if got.State != TradeStateFunded {
		t.Errorf("State after update = %s, want %s", got.State, TradeStateFunded)
	}

	// Delete
	if err := store.DeleteTrade(trade.ID); err != nil {
		t.Fatalf("DeleteTrade() error = %v", err)
	}

	got, err = store.GetTrade(trade.ID)
	if err != ErrTradeNotFound {
		t.Errorf("GetTrade after delete should return ErrTradeNotFound, got %v", err)
	}
}

func TestTradeNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	_, err = store.GetTrade("nonexistent")
	if err != ErrTradeNotFound {
		t.Errorf("GetTrade(nonexistent) should return ErrTradeNotFound, got %v", err)
	}

	err = store.UpdateTradeState("nonexistent", TradeStateFailed)
	if err != ErrTradeNotFound {
		t.Errorf("UpdateTradeState(nonexistent) should return ErrTradeNotFound, got %v", err)
	}

	err = store.DeleteTrade("nonexistent")
	if err != ErrTradeNotFound {
		t.Errorf("DeleteTrade(nonexistent) should return ErrTradeNotFound, got %v", err)
	}
}

func TestTradeFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	trade := &Trade{
		ID:            "trade-fail",
		OrderID:       "order-456",
		MakerPeerID:   "12D3KooWMaker",
		TakerPeerID:   "12D3KooWTaker",
		OurRole:       TradeRoleTaker,
		Method:        "htlc_bitcoin",
		State:         TradeStateFunding,
		OfferChain:    "BTC",
		OfferAmount:   100000000,
		RequestChain:  "LTC",
		RequestAmount: 5000000000,
		CreatedAt:     time.Now(),
	}

	if err := store.CreateTrade(trade); err != nil {
		t.Fatalf("CreateTrade() error = %v", err)
	}

	// Mark as failed
	if err := store.UpdateTradeFailure(trade.ID, "funding timeout"); err != nil {
		t.Fatalf("UpdateTradeFailure() error = %v", err)
	}

	got, _ := store.GetTrade(trade.ID)
	if got.State != TradeStateFailed {
		t.Errorf("State after failure = %s, want %s", got.State, TradeStateFailed)
	}
	if got.FailureReason != "funding timeout" {
		t.Errorf("FailureReason = %s, want 'funding timeout'", got.FailureReason)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set after failure")
	}
}

func TestListTrades(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	// Create trades with different attributes
	trades := []*Trade{
		{
			ID: "trade-1", OrderID: "order-1", MakerPeerID: "maker1", TakerPeerID: "taker1",
			OurRole: TradeRoleMaker, Method: "musig2", State: TradeStateInit,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			CreatedAt: now,
		},
		{
			ID: "trade-2", OrderID: "order-2", MakerPeerID: "maker1", TakerPeerID: "taker2",
			OurRole: TradeRoleTaker, Method: "htlc_bitcoin", State: TradeStateFunded,
			OfferChain: "LTC", OfferAmount: 5000000000, RequestChain: "BTC", RequestAmount: 100000000,
			CreatedAt: now.Add(time.Second),
		},
		{
			ID: "trade-3", OrderID: "order-3", MakerPeerID: "maker2", TakerPeerID: "taker1",
			OurRole: TradeRoleMaker, Method: "musig2", State: TradeStateRedeemed,
			OfferChain: "BTC", OfferAmount: 50000000, RequestChain: "ETH", RequestAmount: 1000000000000000000,
			CreatedAt: now.Add(2 * time.Second),
		},
	}

	for _, tr := range trades {
		if err := store.CreateTrade(tr); err != nil {
			t.Fatalf("CreateTrade() error = %v", err)
		}
	}

	// List all
	all, err := store.ListTrades(TradeFilter{})
	if err != nil {
		t.Fatalf("ListTrades() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListTrades() returned %d trades, want 3", len(all))
	}

	// Filter by state
	state := TradeStateInit
	initTrades, err := store.ListTrades(TradeFilter{State: &state})
	if err != nil {
		t.Fatalf("ListTrades(state=init) error = %v", err)
	}
	if len(initTrades) != 1 {
		t.Errorf("ListTrades(state=init) returned %d trades, want 1", len(initTrades))
	}

	// Filter by role
	role := TradeRoleMaker
	makerTrades, err := store.ListTrades(TradeFilter{OurRole: &role})
	if err != nil {
		t.Fatalf("ListTrades(role=maker) error = %v", err)
	}
	if len(makerTrades) != 2 {
		t.Errorf("ListTrades(role=maker) returned %d trades, want 2", len(makerTrades))
	}

	// Filter by method
	musigTrades, err := store.ListTrades(TradeFilter{Method: "musig2"})
	if err != nil {
		t.Fatalf("ListTrades(method=musig2) error = %v", err)
	}
	if len(musigTrades) != 2 {
		t.Errorf("ListTrades(method=musig2) returned %d trades, want 2", len(musigTrades))
	}
}

func TestGetActiveTrades(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	// Create active trades
	activeTrades := []*Trade{
		{
			ID: "active-1", OrderID: "order-1", MakerPeerID: "maker1", TakerPeerID: "taker1",
			OurRole: TradeRoleMaker, Method: "musig2", State: TradeStateInit,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			CreatedAt: now,
		},
		{
			ID: "active-2", OrderID: "order-2", MakerPeerID: "maker1", TakerPeerID: "taker2",
			OurRole: TradeRoleTaker, Method: "htlc_bitcoin", State: TradeStateFunding,
			OfferChain: "LTC", OfferAmount: 5000000000, RequestChain: "BTC", RequestAmount: 100000000,
			CreatedAt: now.Add(time.Second),
		},
	}

	// Create completed trade
	completedTrade := &Trade{
		ID: "completed-1", OrderID: "order-3", MakerPeerID: "maker2", TakerPeerID: "taker1",
		OurRole: TradeRoleMaker, Method: "musig2", State: TradeStateRedeemed,
		OfferChain: "BTC", OfferAmount: 50000000, RequestChain: "ETH", RequestAmount: 1000000000000000000,
		CreatedAt: now.Add(2 * time.Second),
	}

	for _, tr := range activeTrades {
		if err := store.CreateTrade(tr); err != nil {
			t.Fatalf("CreateTrade() error = %v", err)
		}
	}
	if err := store.CreateTrade(completedTrade); err != nil {
		t.Fatalf("CreateTrade() error = %v", err)
	}

	active, err := store.GetActiveTrades()
	if err != nil {
		t.Fatalf("GetActiveTrades() error = %v", err)
	}
	if len(active) != 2 {
		t.Errorf("GetActiveTrades() returned %d trades, want 2", len(active))
	}
}

func TestCountTrades(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	// Create trades
	states := []TradeState{TradeStateInit, TradeStateFunded, TradeStateRedeemed}
	for i, state := range states {
		trade := &Trade{
			ID: "trade-" + string(rune('A'+i)), OrderID: "order-" + string(rune('A'+i)),
			MakerPeerID: "maker1", TakerPeerID: "taker1",
			OurRole: TradeRoleMaker, Method: "musig2", State: state,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			CreatedAt: now,
		}
		if err := store.CreateTrade(trade); err != nil {
			t.Fatalf("CreateTrade() error = %v", err)
		}
	}

	// Count all
	total, err := store.CountTrades(nil)
	if err != nil {
		t.Fatalf("CountTrades(nil) error = %v", err)
	}
	if total != 3 {
		t.Errorf("CountTrades(nil) = %d, want 3", total)
	}

	// Count redeemed
	redeemed := TradeStateRedeemed
	redeemedCount, err := store.CountTrades(&redeemed)
	if err != nil {
		t.Fatalf("CountTrades(redeemed) error = %v", err)
	}
	if redeemedCount != 1 {
		t.Errorf("CountTrades(redeemed) = %d, want 1", redeemedCount)
	}
}

func TestTradeTerminalStatesSetCompletedAt(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-trades-test-*")
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

	trade := &Trade{
		ID: "trade-terminal", OrderID: "order-1",
		MakerPeerID: "maker1", TakerPeerID: "taker1",
		OurRole: TradeRoleMaker, Method: "musig2", State: TradeStateFunded,
		OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
		CreatedAt: time.Now(),
	}

	if err := store.CreateTrade(trade); err != nil {
		t.Fatalf("CreateTrade() error = %v", err)
	}

	// Update to terminal state
	if err := store.UpdateTradeState(trade.ID, TradeStateRedeemed); err != nil {
		t.Fatalf("UpdateTradeState() error = %v", err)
	}

	got, _ := store.GetTrade(trade.ID)
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set for terminal state")
	}
}
