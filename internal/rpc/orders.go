package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/Klingon-tech/klingdex/internal/node"
	"github.com/Klingon-tech/klingdex/internal/storage"
)

// ========================================
// Order handlers
// ========================================

// OrderCreateParams is the parameters for orders_create.
type OrderCreateParams struct {
	OfferChain       string   `json:"offer_chain"`       // e.g., "BTC"
	OfferAmount      uint64   `json:"offer_amount"`      // In smallest unit (satoshis)
	RequestChain     string   `json:"request_chain"`     // e.g., "LTC"
	RequestAmount    uint64   `json:"request_amount"`    // In smallest unit
	PreferredMethods []string `json:"preferred_methods"` // e.g., ["musig2", "htlc"]
	ExpiresInHours   int      `json:"expires_in_hours"`  // Optional, default 24
}

// OrderInfo represents order information in RPC responses.
type OrderInfo struct {
	ID               string   `json:"id"`
	PeerID           string   `json:"peer_id"`
	Status           string   `json:"status"`
	IsLocal          bool     `json:"is_local"`
	OfferChain       string   `json:"offer_chain"`
	OfferAmount      uint64   `json:"offer_amount"`
	RequestChain     string   `json:"request_chain"`
	RequestAmount    uint64   `json:"request_amount"`
	PreferredMethods []string `json:"preferred_methods"`
	CreatedAt        int64    `json:"created_at"`
	ExpiresAt        *int64   `json:"expires_at,omitempty"`
}

func orderToInfo(o *storage.Order) OrderInfo {
	info := OrderInfo{
		ID:               o.ID,
		PeerID:           o.PeerID,
		Status:           string(o.Status),
		IsLocal:          o.IsLocal,
		OfferChain:       o.OfferChain,
		OfferAmount:      o.OfferAmount,
		RequestChain:     o.RequestChain,
		RequestAmount:    o.RequestAmount,
		PreferredMethods: o.PreferredMethods,
		CreatedAt:        o.CreatedAt.Unix(),
	}
	if o.ExpiresAt != nil {
		ts := o.ExpiresAt.Unix()
		info.ExpiresAt = &ts
	}
	return info
}

func (s *Server) ordersCreate(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p OrderCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Validate required fields
	if p.OfferChain == "" || p.RequestChain == "" {
		return nil, fmt.Errorf("offer_chain and request_chain are required")
	}
	if p.OfferAmount == 0 || p.RequestAmount == 0 {
		return nil, fmt.Errorf("offer_amount and request_amount must be positive")
	}
	if len(p.PreferredMethods) == 0 {
		p.PreferredMethods = []string{"musig2"} // Default to MuSig2
	}
	if p.ExpiresInHours == 0 {
		p.ExpiresInHours = 24 // Default 24 hours
	}

	// Generate order ID
	orderID := uuid.New().String()

	// Calculate expiry
	now := time.Now()
	expiresAt := now.Add(time.Duration(p.ExpiresInHours) * time.Hour)

	// Create order
	order := &storage.Order{
		ID:               orderID,
		PeerID:           s.node.ID().String(),
		Status:           storage.OrderStatusOpen,
		IsLocal:          true,
		OfferChain:       p.OfferChain,
		OfferAmount:      p.OfferAmount,
		RequestChain:     p.RequestChain,
		RequestAmount:    p.RequestAmount,
		PreferredMethods: p.PreferredMethods,
		CreatedAt:        now,
		ExpiresAt:        &expiresAt,
	}

	if err := s.store.CreateOrder(order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	// Broadcast order to network via PubSub (public announcement)
	msg, err := node.NewOrderAnnounceMessage(orderID, orderToInfo(order))
	if err == nil {
		if err := s.broadcastToAll(ctx, msg); err != nil {
			s.log.Warn("Failed to broadcast order", "id", orderID, "error", err)
		}
	}

	s.log.Info("Order created",
		"id", orderID,
		"offer", fmt.Sprintf("%d %s", p.OfferAmount, p.OfferChain),
		"request", fmt.Sprintf("%d %s", p.RequestAmount, p.RequestChain),
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("order_created", orderToInfo(order))
	}

	return orderToInfo(order), nil
}

// OrdersListParams is the parameters for orders_list.
type OrdersListParams struct {
	Status       string `json:"status,omitempty"`        // Filter by status
	OfferChain   string `json:"offer_chain,omitempty"`   // Filter by offer chain
	RequestChain string `json:"request_chain,omitempty"` // Filter by request chain
	LocalOnly    bool   `json:"local_only,omitempty"`    // Only show our orders
	Limit        int    `json:"limit,omitempty"`         // Max results
}

// OrdersListResult is the response for orders_list.
type OrdersListResult struct {
	Orders []OrderInfo `json:"orders"`
	Count  int         `json:"count"`
}

func (s *Server) ordersList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p OrdersListParams
	if params != nil {
		json.Unmarshal(params, &p)
	}

	if p.Limit == 0 {
		p.Limit = 100
	}

	filter := storage.OrderFilter{
		OfferChain:   p.OfferChain,
		RequestChain: p.RequestChain,
		Limit:        p.Limit,
	}

	if p.Status != "" {
		status := storage.OrderStatus(p.Status)
		filter.Status = &status
	}

	if p.LocalOnly {
		isLocal := true
		filter.IsLocal = &isLocal
	}

	orders, err := s.store.ListOrders(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list orders: %w", err)
	}

	result := make([]OrderInfo, 0, len(orders))
	for _, o := range orders {
		result = append(result, orderToInfo(o))
	}

	return &OrdersListResult{
		Orders: result,
		Count:  len(result),
	}, nil
}

// OrdersGetParams is the parameters for orders_get.
type OrdersGetParams struct {
	ID string `json:"id"`
}

func (s *Server) ordersGet(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p OrdersGetParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	order, err := s.store.GetOrder(p.ID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	return orderToInfo(order), nil
}

// OrdersCancelParams is the parameters for orders_cancel.
type OrdersCancelParams struct {
	ID string `json:"id"`
}

func (s *Server) ordersCancel(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p OrdersCancelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	// Get order to verify ownership
	order, err := s.store.GetOrder(p.ID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	if !order.IsLocal {
		return nil, fmt.Errorf("cannot cancel order created by another peer")
	}

	if order.Status != storage.OrderStatusOpen {
		return nil, fmt.Errorf("can only cancel open orders, current status: %s", order.Status)
	}

	if err := s.store.UpdateOrderStatus(p.ID, storage.OrderStatusCancelled); err != nil {
		return nil, fmt.Errorf("failed to cancel order: %w", err)
	}

	// Broadcast cancellation to network via PubSub
	cancelPayload := map[string]interface{}{
		"order_id":  p.ID,
		"cancelled": true,
	}
	cancelMsg, err := node.NewSwapMessage(node.SwapMsgOrderCancel, "", cancelPayload)
	if err == nil {
		cancelMsg.OrderID = p.ID
		if err := s.broadcastToAll(ctx, cancelMsg); err != nil {
			s.log.Warn("Failed to broadcast order cancellation", "id", p.ID, "error", err)
		}
	}

	s.log.Info("Order cancelled", "id", p.ID)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("order_cancelled", map[string]string{"id": p.ID})
	}

	return map[string]interface{}{
		"success": true,
		"id":      p.ID,
	}, nil
}

// OrdersTakeParams is the parameters for orders_take.
type OrdersTakeParams struct {
	OrderID         string `json:"order_id"`
	PreferredMethod string `json:"preferred_method,omitempty"` // Override method if supported
}

// OrdersTakeResult is the response for orders_take.
type OrdersTakeResult struct {
	TradeID    string `json:"trade_id"`
	OrderID    string `json:"order_id"`
	Method     string `json:"method"`
	Status     string `json:"status"`
	NextAction string `json:"next_action"`
}

func (s *Server) ordersTake(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p OrdersTakeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.OrderID == "" {
		return nil, fmt.Errorf("order_id is required")
	}

	// Check wallet is unlocked
	if s.wallet == nil || !s.wallet.IsUnlocked() {
		return nil, fmt.Errorf("wallet must be unlocked to take orders")
	}

	// Get order
	order, err := s.store.GetOrder(p.OrderID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	if order.Status != storage.OrderStatusOpen {
		return nil, fmt.Errorf("order is not open, status: %s", order.Status)
	}

	if order.IsLocal {
		return nil, fmt.Errorf("cannot take your own order")
	}

	// Determine method to use
	method := p.PreferredMethod
	if method == "" && len(order.PreferredMethods) > 0 {
		method = order.PreferredMethods[0]
	}
	if method == "" {
		method = "musig2"
	}

	// Generate trade ID
	tradeID := uuid.New().String()

	// Create trade record
	trade := &storage.Trade{
		ID:          tradeID,
		OrderID:     order.ID,
		MakerPeerID: order.PeerID,
		TakerPeerID: s.node.ID().String(),
		Method:      method,
		State:       storage.TradeStateInit,
		OfferAmount:   order.OfferAmount,
		RequestAmount: order.RequestAmount,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateTrade(trade); err != nil {
		return nil, fmt.Errorf("failed to create trade: %w", err)
	}

	// Update order status
	if err := s.store.UpdateOrderStatus(order.ID, storage.OrderStatusMatched); err != nil {
		return nil, fmt.Errorf("failed to update order status: %w", err)
	}

	// Broadcast order take message to network via PubSub (public announcement)
	// This notifies all peers that the order has been taken
	takePayload := map[string]interface{}{
		"trade_id":       tradeID,
		"order_id":       order.ID,
		"taker_peer_id":  s.node.ID().String(),
		"method":         method,
		"offer_amount":   order.OfferAmount,
		"request_amount": order.RequestAmount,
	}
	takeMsg, err := node.NewOrderTakeMessage(order.ID, tradeID, takePayload)
	if err == nil {
		if err := s.broadcastToAll(ctx, takeMsg); err != nil {
			s.log.Warn("Failed to broadcast order take", "trade_id", tradeID, "error", err)
		}
	}

	s.log.Info("Order taken",
		"trade_id", tradeID,
		"order_id", order.ID,
		"method", method,
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("trade_started", map[string]string{
			"trade_id": tradeID,
			"order_id": order.ID,
			"method":   method,
		})
	}

	return &OrdersTakeResult{
		TradeID:    tradeID,
		OrderID:    order.ID,
		Method:     method,
		Status:     string(storage.TradeStateInit),
		NextAction: "waiting_for_maker_response",
	}, nil
}
