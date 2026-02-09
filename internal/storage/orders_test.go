package storage

import (
	"os"
	"testing"
	"time"
)

func TestOrderCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	// Create order
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	order := &Order{
		ID:               "order-123",
		PeerID:           "12D3KooWTestPeer",
		Status:           OrderStatusOpen,
		IsLocal:          true,
		OfferChain:       "BTC",
		OfferAmount:      100000000, // 1 BTC in satoshis
		RequestChain:     "LTC",
		RequestAmount:    5000000000, // 50 LTC
		PreferredMethods: []string{"musig2", "htlc_bitcoin"},
		CreatedAt:        now,
		ExpiresAt:        &expires,
		Signature:        "sig123",
	}

	// Create
	if err := store.CreateOrder(order); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	// Get
	got, err := store.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetOrder() returned nil")
	}

	if got.ID != order.ID {
		t.Errorf("ID = %s, want %s", got.ID, order.ID)
	}
	if got.Status != OrderStatusOpen {
		t.Errorf("Status = %s, want %s", got.Status, OrderStatusOpen)
	}
	if got.OfferChain != "BTC" {
		t.Errorf("OfferChain = %s, want BTC", got.OfferChain)
	}
	if got.OfferAmount != 100000000 {
		t.Errorf("OfferAmount = %d, want 100000000", got.OfferAmount)
	}
	if len(got.PreferredMethods) != 2 {
		t.Errorf("PreferredMethods length = %d, want 2", len(got.PreferredMethods))
	}
	if !got.IsLocal {
		t.Error("IsLocal should be true")
	}

	// Update status
	if err := store.UpdateOrderStatus(order.ID, OrderStatusMatched); err != nil {
		t.Fatalf("UpdateOrderStatus() error = %v", err)
	}

	got, _ = store.GetOrder(order.ID)
	if got.Status != OrderStatusMatched {
		t.Errorf("Status after update = %s, want %s", got.Status, OrderStatusMatched)
	}

	// Delete
	if err := store.DeleteOrder(order.ID); err != nil {
		t.Fatalf("DeleteOrder() error = %v", err)
	}

	got, err = store.GetOrder(order.ID)
	if err != ErrOrderNotFound {
		t.Errorf("GetOrder after delete should return ErrOrderNotFound, got %v", err)
	}
}

func TestOrderNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	_, err = store.GetOrder("nonexistent")
	if err != ErrOrderNotFound {
		t.Errorf("GetOrder(nonexistent) should return ErrOrderNotFound, got %v", err)
	}

	err = store.UpdateOrderStatus("nonexistent", OrderStatusCancelled)
	if err != ErrOrderNotFound {
		t.Errorf("UpdateOrderStatus(nonexistent) should return ErrOrderNotFound, got %v", err)
	}

	err = store.DeleteOrder("nonexistent")
	if err != ErrOrderNotFound {
		t.Errorf("DeleteOrder(nonexistent) should return ErrOrderNotFound, got %v", err)
	}
}

func TestListOrders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	// Create multiple orders
	orders := []*Order{
		{
			ID: "order-1", PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			PreferredMethods: []string{"musig2"}, CreatedAt: now,
		},
		{
			ID: "order-2", PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
			OfferChain: "BTC", OfferAmount: 200000000, RequestChain: "ETH", RequestAmount: 1000000000000000000,
			PreferredMethods: []string{"htlc_evm"}, CreatedAt: now.Add(time.Second),
		},
		{
			ID: "order-3", PeerID: "peer2", Status: OrderStatusCompleted, IsLocal: false,
			OfferChain: "LTC", OfferAmount: 1000000000, RequestChain: "BTC", RequestAmount: 20000000,
			PreferredMethods: []string{"htlc_bitcoin"}, CreatedAt: now.Add(2 * time.Second),
		},
	}

	for _, o := range orders {
		if err := store.CreateOrder(o); err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
	}

	// List all
	all, err := store.ListOrders(OrderFilter{})
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListOrders() returned %d orders, want 3", len(all))
	}

	// Filter by status
	status := OrderStatusOpen
	open, err := store.ListOrders(OrderFilter{Status: &status})
	if err != nil {
		t.Fatalf("ListOrders(status=open) error = %v", err)
	}
	if len(open) != 2 {
		t.Errorf("ListOrders(status=open) returned %d orders, want 2", len(open))
	}

	// Filter by offer chain
	btcOrders, err := store.ListOrders(OrderFilter{OfferChain: "BTC"})
	if err != nil {
		t.Fatalf("ListOrders(offerChain=BTC) error = %v", err)
	}
	if len(btcOrders) != 2 {
		t.Errorf("ListOrders(offerChain=BTC) returned %d orders, want 2", len(btcOrders))
	}

	// Filter by local
	isLocal := true
	local, err := store.ListOrders(OrderFilter{IsLocal: &isLocal})
	if err != nil {
		t.Fatalf("ListOrders(isLocal=true) error = %v", err)
	}
	if len(local) != 2 {
		t.Errorf("ListOrders(isLocal=true) returned %d orders, want 2", len(local))
	}

	// Test limit
	limited, err := store.ListOrders(OrderFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListOrders(limit=1) error = %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("ListOrders(limit=1) returned %d orders, want 1", len(limited))
	}
}

func TestGetOpenOrders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	// Create BTC -> LTC orders
	for i := 0; i < 3; i++ {
		order := &Order{
			ID: "btc-ltc-" + string(rune('A'+i)), PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			PreferredMethods: []string{"musig2"}, CreatedAt: now,
		}
		if err := store.CreateOrder(order); err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
	}

	// Create ETH -> BTC order
	order := &Order{
		ID: "eth-btc-1", PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
		OfferChain: "ETH", OfferAmount: 1000000000000000000, RequestChain: "BTC", RequestAmount: 50000000,
		PreferredMethods: []string{"htlc_evm"}, CreatedAt: now,
	}
	if err := store.CreateOrder(order); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	// Get open BTC -> LTC orders
	orders, err := store.GetOpenOrders("BTC", "LTC")
	if err != nil {
		t.Fatalf("GetOpenOrders() error = %v", err)
	}
	if len(orders) != 3 {
		t.Errorf("GetOpenOrders(BTC, LTC) returned %d orders, want 3", len(orders))
	}
}

func TestGetMyOrders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	// Create local orders
	for i := 0; i < 2; i++ {
		order := &Order{
			ID: "local-" + string(rune('A'+i)), PeerID: "mypeer", Status: OrderStatusOpen, IsLocal: true,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			PreferredMethods: []string{"musig2"}, CreatedAt: now,
		}
		if err := store.CreateOrder(order); err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
	}

	// Create remote order
	order := &Order{
		ID: "remote-1", PeerID: "otherpeer", Status: OrderStatusOpen, IsLocal: false,
		OfferChain: "ETH", OfferAmount: 1000000000000000000, RequestChain: "BTC", RequestAmount: 50000000,
		PreferredMethods: []string{"htlc_evm"}, CreatedAt: now,
	}
	if err := store.CreateOrder(order); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	myOrders, err := store.GetMyOrders()
	if err != nil {
		t.Fatalf("GetMyOrders() error = %v", err)
	}
	if len(myOrders) != 2 {
		t.Errorf("GetMyOrders() returned %d orders, want 2", len(myOrders))
	}
}

func TestExpireOldOrders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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
	pastExpiry := now.Add(-1 * time.Hour)
	futureExpiry := now.Add(1 * time.Hour)

	// Create expired order
	expiredOrder := &Order{
		ID: "expired-1", PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
		OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
		PreferredMethods: []string{"musig2"}, CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: &pastExpiry,
	}
	if err := store.CreateOrder(expiredOrder); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	// Create non-expired order
	validOrder := &Order{
		ID: "valid-1", PeerID: "peer1", Status: OrderStatusOpen, IsLocal: true,
		OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
		PreferredMethods: []string{"musig2"}, CreatedAt: now,
		ExpiresAt: &futureExpiry,
	}
	if err := store.CreateOrder(validOrder); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	// Expire old orders
	count, err := store.ExpireOldOrders()
	if err != nil {
		t.Fatalf("ExpireOldOrders() error = %v", err)
	}
	if count != 1 {
		t.Errorf("ExpireOldOrders() expired %d orders, want 1", count)
	}

	// Verify expired order status
	expired, _ := store.GetOrder("expired-1")
	if expired.Status != OrderStatusExpired {
		t.Errorf("expired order status = %s, want %s", expired.Status, OrderStatusExpired)
	}

	// Verify valid order unchanged
	valid, _ := store.GetOrder("valid-1")
	if valid.Status != OrderStatusOpen {
		t.Errorf("valid order status = %s, want %s", valid.Status, OrderStatusOpen)
	}
}

func TestCountOrders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-orders-test-*")
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

	// Create orders with different statuses
	statuses := []OrderStatus{OrderStatusOpen, OrderStatusOpen, OrderStatusCompleted}
	for i, status := range statuses {
		order := &Order{
			ID: "order-" + string(rune('A'+i)), PeerID: "peer1", Status: status, IsLocal: true,
			OfferChain: "BTC", OfferAmount: 100000000, RequestChain: "LTC", RequestAmount: 5000000000,
			PreferredMethods: []string{"musig2"}, CreatedAt: now,
		}
		if err := store.CreateOrder(order); err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
	}

	// Count all
	total, err := store.CountOrders(nil)
	if err != nil {
		t.Fatalf("CountOrders(nil) error = %v", err)
	}
	if total != 3 {
		t.Errorf("CountOrders(nil) = %d, want 3", total)
	}

	// Count open
	open := OrderStatusOpen
	openCount, err := store.CountOrders(&open)
	if err != nil {
		t.Fatalf("CountOrders(open) error = %v", err)
	}
	if openCount != 2 {
		t.Errorf("CountOrders(open) = %d, want 2", openCount)
	}
}
