// Package rpc provides a JSON-RPC 2.0 server for the Klingon daemon.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
	"github.com/klingon-exchange/klingon-v2/internal/wallet"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// Server is a JSON-RPC 2.0 server.
type Server struct {
	node        *node.Node
	store       *storage.Storage
	wallet      *wallet.Service
	coordinator *swap.Coordinator
	log         *logging.Logger
	wsHub       *WSHub

	server   *http.Server
	listener net.Listener

	handlers map[string]Handler
	mu       sync.RWMutex
}

// Handler is a JSON-RPC method handler.
type Handler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error represents a JSON-RPC 2.0 error.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard error codes.
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// NewServer creates a new JSON-RPC server.
func NewServer(n *node.Node, store *storage.Storage, w *wallet.Service, coord *swap.Coordinator) *Server {
	s := &Server{
		node:        n,
		store:       store,
		wallet:      w,
		coordinator: coord,
		log:         logging.GetDefault().Component("rpc"),
		handlers:    make(map[string]Handler),
	}

	// Register handlers
	s.registerHandlers()

	return s
}

// registerHandlers registers all JSON-RPC method handlers.
func (s *Server) registerHandlers() {
	// Node methods
	s.handlers["node_info"] = s.nodeInfo
	s.handlers["node_status"] = s.nodeStatus

	// Peer methods
	s.handlers["peers_list"] = s.peersList
	s.handlers["peers_count"] = s.peersCount
	s.handlers["peers_connect"] = s.peersConnect
	s.handlers["peers_disconnect"] = s.peersDisconnect
	s.handlers["peers_known"] = s.peersKnown

	// Wallet methods
	s.handlers["wallet_status"] = s.walletStatus
	s.handlers["wallet_generate"] = s.walletGenerate
	s.handlers["wallet_create"] = s.walletCreate
	s.handlers["wallet_unlock"] = s.walletUnlock
	s.handlers["wallet_lock"] = s.walletLock
	s.handlers["wallet_getAddress"] = s.walletGetAddress
	s.handlers["wallet_getAllAddresses"] = s.walletGetAllAddresses
	s.handlers["wallet_getPublicKey"] = s.walletGetPublicKey
	s.handlers["wallet_supportedChains"] = s.walletSupportedChains
	s.handlers["wallet_validateMnemonic"] = s.walletValidateMnemonic
	s.handlers["wallet_getBalance"] = s.walletGetBalance
	s.handlers["wallet_getFeeEstimates"] = s.walletGetFeeEstimates
	s.handlers["wallet_send"] = s.walletSend
	s.handlers["wallet_getUTXOs"] = s.walletGetUTXOs
	s.handlers["wallet_scanBalance"] = s.walletScanBalance
	s.handlers["wallet_getAddressWithChange"] = s.walletGetAddressWithChange

	// Multi-address wallet methods (aggregates UTXOs from all addresses)
	s.handlers["wallet_sendAll"] = s.walletSendAll
	s.handlers["wallet_sendMax"] = s.walletSendMax
	s.handlers["wallet_getAggregatedBalance"] = s.walletGetAggregatedBalance
	s.handlers["wallet_listAllUTXOs"] = s.walletListAllUTXOs
	s.handlers["wallet_syncUTXOs"] = s.walletSyncUTXOs

	// EVM wallet methods
	s.handlers["wallet_sendEVM"] = s.walletSendEVM
	s.handlers["wallet_sendERC20"] = s.walletSendERC20
	s.handlers["wallet_getERC20Balance"] = s.walletGetERC20Balance
	s.handlers["wallet_getChainType"] = s.walletGetChainType
	s.handlers["wallet_listTokens"] = s.walletListTokens

	// Order methods
	s.handlers["orders_create"] = s.ordersCreate
	s.handlers["orders_list"] = s.ordersList
	s.handlers["orders_get"] = s.ordersGet
	s.handlers["orders_cancel"] = s.ordersCancel
	s.handlers["orders_take"] = s.ordersTake

	// Trade methods
	s.handlers["trades_list"] = s.tradesList
	s.handlers["trades_get"] = s.tradesGet
	s.handlers["trades_status"] = s.tradesStatus

	// Swap methods (MuSig2 key exchange and signing)
	s.handlers["swap_init"] = s.swapInit
	s.handlers["swap_exchangeNonce"] = s.swapExchangeNonce
	s.handlers["swap_getAddress"] = s.swapGetAddress
	s.handlers["swap_setFunding"] = s.swapSetFunding
	s.handlers["swap_checkFunding"] = s.swapCheckFunding
	s.handlers["swap_fund"] = s.swapFund // Auto-fund: scan wallet, sign, broadcast, set funding
	s.handlers["swap_sign"] = s.swapSign
	s.handlers["swap_redeem"] = s.swapRedeem
	s.handlers["swap_status"] = s.swapStatus

	// Swap recovery and timeout methods
	s.handlers["swap_list"] = s.swapList
	s.handlers["swap_recover"] = s.swapRecover
	s.handlers["swap_timeout"] = s.swapTimeout
	s.handlers["swap_refund"] = s.swapRefund
	s.handlers["swap_checkTimeouts"] = s.swapCheckTimeouts

	// HTLC-specific methods (Bitcoin-family)
	s.handlers["swap_htlcRevealSecret"] = s.swapHTLCRevealSecret
	s.handlers["swap_htlcGetSecret"] = s.swapHTLCGetSecret
	s.handlers["swap_htlcClaim"] = s.swapHTLCClaim
	s.handlers["swap_htlcRefund"] = s.swapHTLCRefund
	s.handlers["swap_htlcExtractSecret"] = s.swapHTLCExtractSecret

	// EVM HTLC methods
	s.handlers["swap_evmCreate"] = s.swapEVMCreate
	s.handlers["swap_evmClaim"] = s.swapEVMClaim
	s.handlers["swap_evmRefund"] = s.swapEVMRefund
	s.handlers["swap_evmStatus"] = s.swapEVMStatus
	s.handlers["swap_evmWaitSecret"] = s.swapEVMWaitSecret
	s.handlers["swap_evmSetSecret"] = s.swapEVMSetSecret
	s.handlers["swap_evmGetContracts"] = s.swapEVMGetContracts
	s.handlers["swap_evmGetContract"] = s.swapEVMGetContract
	s.handlers["swap_evmComputeSwapID"] = s.swapEVMComputeSwapID

	// Cross-chain swap methods
	s.handlers["swap_initCrossChain"] = s.swapInitCrossChain
	s.handlers["swap_getSwapType"] = s.swapGetSwapType
}

// Start starts the RPC server.
func (s *Server) Start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Initialize WebSocket hub
	s.wsHub = NewWSHub()
	go s.wsHub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /", s.handleRPC)
	mux.HandleFunc("POST /{$}", s.handleRPC)
	mux.HandleFunc("OPTIONS /", s.handleCORS)
	mux.HandleFunc("OPTIONS /{$}", s.handleCORS)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /ws/", s.handleWS)

	s.server = &http.Server{
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.log.Error("RPC server error", "error", err)
		}
	}()

	s.log.Info("RPC server started", "addr", addr, "ws", "ws://"+addr+"/ws")
	return nil
}

// Stop stops the RPC server.
func (s *Server) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// SetupSwapHandlers registers handlers for swap protocol messages.
// This should be called after the node's swap handler is started.
func (s *Server) SetupSwapHandlers() {
	// Register handlers on PubSub (for public broadcasts like orders)
	swapHandler := s.node.SwapHandler()
	if swapHandler != nil {
		// Handle incoming order announcements (public, via PubSub)
		swapHandler.OnMessage(node.SwapMsgOrderAnnounce, s.handleOrderAnnounce)
		swapHandler.OnMessage(node.SwapMsgOrderCancel, s.handleOrderCancel)
		swapHandler.OnMessage(node.SwapMsgOrderTake, s.handleOrderTake)
	}

	// Register handlers on direct stream handler (for private swap messages)
	s.node.RegisterDirectHandler(node.SwapMsgPubKeyExchange, s.handlePubKeyExchange)
	s.node.RegisterDirectHandler(node.SwapMsgNonceExchange, s.handleNonceExchange)
	s.node.RegisterDirectHandler(node.SwapMsgFundingInfo, s.handleFundingInfo)
	s.node.RegisterDirectHandler(node.SwapMsgPartialSig, s.handlePartialSig)
	s.node.RegisterDirectHandler(node.SwapMsgHTLCSecretHash, s.handleHTLCSecretHash)
	s.node.RegisterDirectHandler(node.SwapMsgHTLCSecretReveal, s.handleHTLCSecretReveal)
	s.node.RegisterDirectHandler(node.SwapMsgHTLCClaim, s.handleHTLCClaim)

	s.log.Info("Swap message handlers registered")
}

// handleOrderAnnounce processes incoming order announcements from other peers.
func (s *Server) handleOrderAnnounce(ctx context.Context, msg *node.SwapMessage) error {
	// Skip if this is our own message
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	// Parse order info from payload
	var orderInfo OrderInfo
	if err := json.Unmarshal(msg.Payload, &orderInfo); err != nil {
		s.log.Warn("Failed to parse order announcement", "error", err)
		return nil // Don't return error - just log and continue
	}

	// Check if we already have this order
	if existing, _ := s.store.GetOrder(orderInfo.ID); existing != nil {
		return nil // Already have it
	}

	// Create order record (as non-local)
	var expiresAt *time.Time
	if orderInfo.ExpiresAt != nil {
		t := time.Unix(*orderInfo.ExpiresAt, 0)
		expiresAt = &t
	}

	order := &storage.Order{
		ID:               orderInfo.ID,
		PeerID:           orderInfo.PeerID,
		Status:           storage.OrderStatus(orderInfo.Status),
		IsLocal:          false, // This is from another peer
		OfferChain:       orderInfo.OfferChain,
		OfferAmount:      orderInfo.OfferAmount,
		RequestChain:     orderInfo.RequestChain,
		RequestAmount:    orderInfo.RequestAmount,
		PreferredMethods: orderInfo.PreferredMethods,
		CreatedAt:        time.Unix(orderInfo.CreatedAt, 0),
		ExpiresAt:        expiresAt,
	}

	if err := s.store.CreateOrder(order); err != nil {
		s.log.Debug("Failed to store order", "id", order.ID, "error", err)
		return nil
	}

	s.log.Info("Received order announcement",
		"id", order.ID,
		"from", order.PeerID[:12],
		"offer", fmt.Sprintf("%d %s", order.OfferAmount, order.OfferChain),
		"request", fmt.Sprintf("%d %s", order.RequestAmount, order.RequestChain),
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("order_received", orderToInfo(order))
	}

	return nil
}

// handleOrderCancel processes incoming order cancellation messages.
func (s *Server) handleOrderCancel(ctx context.Context, msg *node.SwapMessage) error {
	if msg.OrderID == "" {
		return nil
	}

	order, err := s.store.GetOrder(msg.OrderID)
	if err != nil || order == nil {
		return nil // Don't have this order
	}

	// Only process if it's from the order creator
	if order.PeerID != msg.FromPeer {
		return nil // Not authorized
	}

	if err := s.store.UpdateOrderStatus(msg.OrderID, storage.OrderStatusCancelled); err != nil {
		s.log.Debug("Failed to cancel order", "id", msg.OrderID, "error", err)
	}

	s.log.Info("Order cancelled by peer", "id", msg.OrderID)

	if s.wsHub != nil {
		s.wsHub.Broadcast("order_cancelled", map[string]string{"id": msg.OrderID})
	}

	return nil
}

// OrderTakePayload represents the payload for an order take message.
type OrderTakePayload struct {
	TradeID       string `json:"trade_id"`
	OrderID       string `json:"order_id"`
	TakerPeerID   string `json:"taker_peer_id"`
	Method        string `json:"method"`
	OfferAmount   uint64 `json:"offer_amount"`
	RequestAmount uint64 `json:"request_amount"`
}

// handleOrderTake processes incoming order take messages (for makers).
func (s *Server) handleOrderTake(ctx context.Context, msg *node.SwapMessage) error {
	// Skip our own messages
	if msg.FromPeer == s.node.ID().String() {
		return nil
	}

	// Parse the take payload
	var payload OrderTakePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		s.log.Warn("Failed to parse order take message", "error", err)
		return nil
	}

	// Verify we own this order
	order, err := s.store.GetOrder(payload.OrderID)
	if err != nil || order == nil {
		return nil // Not our order
	}

	if !order.IsLocal {
		return nil // We didn't create this order
	}

	if order.Status != storage.OrderStatusOpen {
		s.log.Debug("Order not open, ignoring take", "id", payload.OrderID, "status", order.Status)
		return nil
	}

	// Check if we already have a trade for this order
	if existing, _ := s.store.GetTrade(payload.TradeID); existing != nil {
		return nil // Already have this trade
	}

	// Create trade on the maker side
	trade := &storage.Trade{
		ID:            payload.TradeID,
		OrderID:       payload.OrderID,
		MakerPeerID:   s.node.ID().String(),
		TakerPeerID:   payload.TakerPeerID,
		Method:        payload.Method,
		State:         storage.TradeStateInit,
		OfferAmount:   payload.OfferAmount,
		RequestAmount: payload.RequestAmount,
		CreatedAt:     time.Now(),
	}

	if err := s.store.CreateTrade(trade); err != nil {
		s.log.Warn("Failed to create trade from take message", "error", err)
		return nil
	}

	// Update order status
	if err := s.store.UpdateOrderStatus(payload.OrderID, storage.OrderStatusMatched); err != nil {
		s.log.Warn("Failed to update order status", "error", err)
	}

	s.log.Info("Order taken by peer",
		"trade_id", payload.TradeID,
		"order_id", payload.OrderID,
		"taker", payload.TakerPeerID[:12],
		"method", payload.Method,
	)

	// Emit WebSocket event
	if s.wsHub != nil {
		s.wsHub.Broadcast("trade_started", map[string]string{
			"trade_id": payload.TradeID,
			"order_id": payload.OrderID,
			"taker":    payload.TakerPeerID,
			"method":   payload.Method,
		})
	}

	return nil
}

// handleRPC handles incoming JSON-RPC requests.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, nil, ParseError, "Parse error", nil)
		return
	}

	if req.JSONRPC != "2.0" {
		s.writeError(w, req.ID, InvalidRequest, "Invalid Request", nil)
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	if !ok {
		s.writeError(w, req.ID, MethodNotFound, "Method not found", req.Method)
		return
	}

	result, err := handler(r.Context(), req.Params)
	if err != nil {
		s.writeError(w, req.ID, InternalError, err.Error(), nil)
		return
	}

	s.writeResult(w, req.ID, result)
}

// writeResult writes a successful response.
func (s *Server) writeResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeError writes an error response.
func (s *Server) writeError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// WSHub returns the WebSocket hub.
func (s *Server) WSHub() *WSHub {
	return s.wsHub
}

// handleCORS handles CORS preflight requests.
func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// corsMiddleware adds CORS headers to all responses.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests from any origin (for Electron apps and web clients)
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
