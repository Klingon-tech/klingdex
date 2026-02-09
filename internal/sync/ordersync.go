// Package sync provides order and trade synchronization between peers.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	gosync "sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// Protocol IDs for syncing
const (
	OrderSyncProtocol = "/klingon/ordersync/1.0.0"
	TradeSyncProtocol = "/klingon/tradesync/1.0.0"
)

// Sync configuration
const (
	SyncCooldown   = 5 * time.Minute  // Don't re-sync with same peer within this period
	SyncTimeout    = 30 * time.Second // Timeout for sync operations
	MaxOrdersPerSync = 100            // Maximum orders to sync at once
)

// SyncRequest is sent to request orders from a peer.
type SyncRequest struct {
	Since int64 `json:"since"` // Get orders updated after this timestamp (Unix)
	Limit int   `json:"limit"` // Maximum orders to return
}

// SyncResponse contains orders from a peer.
type SyncResponse struct {
	Orders    []*storage.Order `json:"orders"`
	HasMore   bool             `json:"has_more"`
	Timestamp int64            `json:"timestamp"` // Server timestamp
}

// TradeSyncRequest is sent to request trades from a peer.
type TradeSyncRequest struct {
	Since int64 `json:"since"` // Get trades updated after this timestamp
	Limit int   `json:"limit"`
}

// TradeSyncResponse contains trades from a peer.
type TradeSyncResponse struct {
	Trades    []*storage.Trade `json:"trades"`
	HasMore   bool             `json:"has_more"`
	Timestamp int64            `json:"timestamp"`
}

// OrderValidator validates incoming orders.
type OrderValidator func(order *storage.Order) error

// OrderSync handles synchronization of orders between peers.
type OrderSync struct {
	host      host.Host
	store     *storage.Storage
	validator OrderValidator
	log       *logging.Logger

	// Track synced peers to avoid excessive syncing
	syncedPeers map[peer.ID]time.Time
	mu          gosync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewOrderSync creates a new order sync handler.
func NewOrderSync(h host.Host, store *storage.Storage, validator OrderValidator) *OrderSync {
	ctx, cancel := context.WithCancel(context.Background())

	return &OrderSync{
		host:        h,
		store:       store,
		validator:   validator,
		log:         logging.GetDefault().Component("ordersync"),
		syncedPeers: make(map[peer.ID]time.Time),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the order sync service.
func (os *OrderSync) Start() error {
	// Register protocol handler for incoming sync requests
	os.host.SetStreamHandler(protocol.ID(OrderSyncProtocol), os.handleSyncStream)

	// Watch for peer connections
	go os.watchConnections()

	os.log.Info("Order sync started", "protocol", OrderSyncProtocol)
	return nil
}

// Stop stops the order sync service.
func (os *OrderSync) Stop() error {
	os.cancel()
	os.host.RemoveStreamHandler(protocol.ID(OrderSyncProtocol))
	os.log.Info("Order sync stopped")
	return nil
}

// watchConnections watches for peer connections and triggers sync.
func (os *OrderSync) watchConnections() {
	// Get notification channel for peer connections
	sub, err := os.host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		os.log.Error("Failed to subscribe to peer events", "error", err)
		return
	}
	defer sub.Close()

	for {
		select {
		case <-os.ctx.Done():
			return
		case ev := <-sub.Out():
			e, ok := ev.(event.EvtPeerConnectednessChanged)
			if !ok {
				continue
			}

			if e.Connectedness == network.Connected {
				go os.onPeerConnected(e.Peer)
			}
		}
	}
}

// onPeerConnected is called when a new peer connects.
func (os *OrderSync) onPeerConnected(p peer.ID) {
	// Check if we've recently synced with this peer
	os.mu.RLock()
	lastSync, synced := os.syncedPeers[p]
	os.mu.RUnlock()

	if synced && time.Since(lastSync) < SyncCooldown {
		os.log.Debug("Recently synced with peer, skipping", "peer", shortPeerID(p))
		return
	}

	// Small delay to let connection stabilize
	time.Sleep(500 * time.Millisecond)

	// Sync with the new peer
	if err := os.SyncWithPeer(p); err != nil {
		os.log.Debug("Failed to sync with peer", "peer", shortPeerID(p), "error", err)
	}
}

// SyncWithPeer synchronizes orders with a specific peer.
func (os *OrderSync) SyncWithPeer(p peer.ID) error {
	os.log.Debug("Syncing orders with peer", "peer", shortPeerID(p))

	ctx, cancel := context.WithTimeout(os.ctx, SyncTimeout)
	defer cancel()

	// Open stream to peer
	stream, err := os.host.NewStream(ctx, p, protocol.ID(OrderSyncProtocol))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Send sync request
	req := SyncRequest{
		Since: 0, // Get all active orders
		Limit: MaxOrdersPerSync,
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(&req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(stream)
	var resp SyncResponse
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Process received orders
	newOrders := 0
	for _, order := range resp.Orders {
		// Validate order if validator is set
		if os.validator != nil {
			if err := os.validator(order); err != nil {
				os.log.Debug("Order validation failed", "id", order.ID, "error", err)
				continue
			}
		}

		// Check if we already have this order
		existing, _ := os.store.GetOrder(order.ID)
		if existing != nil {
			// Update if newer
			if order.CreatedAt.After(existing.CreatedAt) {
				// Preserve our is_local flag
				order.IsLocal = existing.IsLocal
				if err := os.store.SaveOrder(order); err != nil {
					os.log.Debug("Failed to update order", "id", order.ID, "error", err)
				}
			}
			continue
		}

		// New order - mark as not local
		order.IsLocal = false
		if err := os.store.CreateOrder(order); err != nil {
			os.log.Debug("Failed to save order", "id", order.ID, "error", err)
			continue
		}
		newOrders++
	}

	// Mark peer as synced
	os.mu.Lock()
	os.syncedPeers[p] = time.Now()
	os.mu.Unlock()

	os.log.Info("Order sync completed",
		"peer", shortPeerID(p),
		"received", len(resp.Orders),
		"new", newOrders,
	)

	return nil
}

// handleSyncStream handles incoming sync requests from peers.
func (os *OrderSync) handleSyncStream(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer()
	os.log.Debug("Handling order sync request", "from", shortPeerID(remotePeer))

	// Read request
	decoder := json.NewDecoder(stream)
	var req SyncRequest
	if err := decoder.Decode(&req); err != nil {
		if err != io.EOF {
			os.log.Debug("Failed to read sync request", "error", err)
		}
		return
	}

	if req.Limit == 0 || req.Limit > MaxOrdersPerSync {
		req.Limit = MaxOrdersPerSync
	}

	// Get our active orders
	filter := storage.OrderFilter{
		Limit: req.Limit,
	}
	status := storage.OrderStatusOpen
	filter.Status = &status

	orders, err := os.store.ListOrders(filter)
	if err != nil {
		os.log.Debug("Failed to list orders", "error", err)
		return
	}

	// Filter by timestamp if requested
	var filteredOrders []*storage.Order
	since := time.Unix(req.Since, 0)
	for _, o := range orders {
		if req.Since == 0 || o.CreatedAt.After(since) {
			filteredOrders = append(filteredOrders, o)
		}
	}

	// Send response
	resp := SyncResponse{
		Orders:    filteredOrders,
		HasMore:   len(filteredOrders) == req.Limit,
		Timestamp: time.Now().Unix(),
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(&resp); err != nil {
		os.log.Debug("Failed to send sync response", "error", err)
		return
	}

	os.log.Debug("Sent order sync response",
		"to", shortPeerID(remotePeer),
		"orders", len(filteredOrders),
	)
}

// BroadcastOrder broadcasts a new order to all connected peers via sync protocol.
// Note: This is used in addition to PubSub broadcasting for immediate sync.
func (os *OrderSync) BroadcastOrder(order *storage.Order) {
	peers := os.host.Network().Peers()
	for _, p := range peers {
		go func(peerID peer.ID) {
			ctx, cancel := context.WithTimeout(os.ctx, 5*time.Second)
			defer cancel()

			stream, err := os.host.NewStream(ctx, peerID, protocol.ID(OrderSyncProtocol))
			if err != nil {
				return
			}
			defer stream.Close()

			// Send a sync request that will trigger a response with our order
			// (The peer will already have received the order via PubSub)
			req := SyncRequest{Since: order.CreatedAt.Unix() - 1, Limit: 1}
			json.NewEncoder(stream).Encode(&req)
		}(p)
	}
}

// GetSyncedPeersCount returns the number of peers we've synced with.
func (os *OrderSync) GetSyncedPeersCount() int {
	os.mu.RLock()
	defer os.mu.RUnlock()
	return len(os.syncedPeers)
}

// shortPeerID returns a shortened peer ID for logging.
func shortPeerID(p peer.ID) string {
	s := p.String()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// TradeSync handles synchronization of trades between peers.
type TradeSync struct {
	host  host.Host
	store *storage.Storage
	log   *logging.Logger

	syncedPeers map[peer.ID]time.Time
	mu          gosync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewTradeSync creates a new trade sync handler.
func NewTradeSync(h host.Host, store *storage.Storage) *TradeSync {
	ctx, cancel := context.WithCancel(context.Background())

	return &TradeSync{
		host:        h,
		store:       store,
		log:         logging.GetDefault().Component("tradesync"),
		syncedPeers: make(map[peer.ID]time.Time),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the trade sync service.
func (ts *TradeSync) Start() error {
	ts.host.SetStreamHandler(protocol.ID(TradeSyncProtocol), ts.handleSyncStream)
	go ts.watchConnections()
	ts.log.Info("Trade sync started", "protocol", TradeSyncProtocol)
	return nil
}

// Stop stops the trade sync service.
func (ts *TradeSync) Stop() error {
	ts.cancel()
	ts.host.RemoveStreamHandler(protocol.ID(TradeSyncProtocol))
	ts.log.Info("Trade sync stopped")
	return nil
}

// watchConnections watches for peer connections and triggers sync.
func (ts *TradeSync) watchConnections() {
	sub, err := ts.host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		ts.log.Error("Failed to subscribe to peer events", "error", err)
		return
	}
	defer sub.Close()

	for {
		select {
		case <-ts.ctx.Done():
			return
		case ev := <-sub.Out():
			e, ok := ev.(event.EvtPeerConnectednessChanged)
			if !ok {
				continue
			}

			if e.Connectedness == network.Connected {
				go ts.onPeerConnected(e.Peer)
			}
		}
	}
}

// onPeerConnected is called when a new peer connects.
func (ts *TradeSync) onPeerConnected(p peer.ID) {
	ts.mu.RLock()
	lastSync, synced := ts.syncedPeers[p]
	ts.mu.RUnlock()

	if synced && time.Since(lastSync) < SyncCooldown {
		return
	}

	time.Sleep(500 * time.Millisecond)

	if err := ts.SyncWithPeer(p); err != nil {
		ts.log.Debug("Failed to sync trades with peer", "peer", shortPeerID(p), "error", err)
	}
}

// SyncWithPeer synchronizes trades with a specific peer.
func (ts *TradeSync) SyncWithPeer(p peer.ID) error {
	ts.log.Debug("Syncing trades with peer", "peer", shortPeerID(p))

	ctx, cancel := context.WithTimeout(ts.ctx, SyncTimeout)
	defer cancel()

	stream, err := ts.host.NewStream(ctx, p, protocol.ID(TradeSyncProtocol))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	req := TradeSyncRequest{
		Since: 0,
		Limit: MaxOrdersPerSync,
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(&req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	decoder := json.NewDecoder(stream)
	var resp TradeSyncResponse
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Process received trades - only store if we're a participant
	newTrades := 0
	myPeerID := ts.host.ID().String()

	for _, trade := range resp.Trades {
		// Only sync trades where we're a participant
		if trade.MakerPeerID != myPeerID && trade.TakerPeerID != myPeerID {
			continue
		}

		existing, _ := ts.store.GetTrade(trade.ID)
		if existing != nil {
			// Update state if it's more advanced
			if shouldUpdateTradeState(existing.State, trade.State) {
				if err := ts.store.UpdateTradeState(trade.ID, trade.State); err != nil {
					ts.log.Debug("Failed to update trade state", "id", trade.ID, "error", err)
				}
			}
			continue
		}

		// New trade
		if err := ts.store.CreateTrade(trade); err != nil {
			ts.log.Debug("Failed to save trade", "id", trade.ID, "error", err)
			continue
		}
		newTrades++
	}

	ts.mu.Lock()
	ts.syncedPeers[p] = time.Now()
	ts.mu.Unlock()

	ts.log.Info("Trade sync completed",
		"peer", shortPeerID(p),
		"received", len(resp.Trades),
		"new", newTrades,
	)

	return nil
}

// handleSyncStream handles incoming trade sync requests.
func (ts *TradeSync) handleSyncStream(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer()
	ts.log.Debug("Handling trade sync request", "from", shortPeerID(remotePeer))

	decoder := json.NewDecoder(stream)
	var req TradeSyncRequest
	if err := decoder.Decode(&req); err != nil {
		if err != io.EOF {
			ts.log.Debug("Failed to read sync request", "error", err)
		}
		return
	}

	if req.Limit == 0 || req.Limit > MaxOrdersPerSync {
		req.Limit = MaxOrdersPerSync
	}

	// Get trades where the requesting peer is a participant
	filter := storage.TradeFilter{
		Limit: req.Limit,
	}

	trades, err := ts.store.ListTrades(filter)
	if err != nil {
		ts.log.Debug("Failed to list trades", "error", err)
		return
	}

	// Filter to only include trades where remote peer is a participant
	remotePeerStr := remotePeer.String()
	var filteredTrades []*storage.Trade
	for _, t := range trades {
		if t.MakerPeerID == remotePeerStr || t.TakerPeerID == remotePeerStr {
			filteredTrades = append(filteredTrades, t)
		}
	}

	resp := TradeSyncResponse{
		Trades:    filteredTrades,
		HasMore:   len(filteredTrades) == req.Limit,
		Timestamp: time.Now().Unix(),
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(&resp); err != nil {
		ts.log.Debug("Failed to send sync response", "error", err)
		return
	}

	ts.log.Debug("Sent trade sync response",
		"to", shortPeerID(remotePeer),
		"trades", len(filteredTrades),
	)
}

// shouldUpdateTradeState determines if we should update to a new state.
// States progress: init -> accepted -> funding -> funded -> redeemed/refunded/failed
func shouldUpdateTradeState(current, incoming storage.TradeState) bool {
	stateOrder := map[storage.TradeState]int{
		storage.TradeStateInit:     1,
		storage.TradeStateAccepted: 2,
		storage.TradeStateFunding:  3,
		storage.TradeStateFunded:   4,
		storage.TradeStateRedeemed: 5,
		storage.TradeStateRefunded: 5,
		storage.TradeStateFailed:   5,
		storage.TradeStateAborted:  5,
	}

	return stateOrder[incoming] > stateOrder[current]
}
