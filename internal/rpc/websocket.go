package rpc

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/Klingon-tech/klingdex/pkg/logging"
)

// WebSocket configuration
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// EventType represents the type of WebSocket event.
type EventType string

const (
	// Peer events
	EventPeerConnected    EventType = "peer_connected"
	EventPeerDisconnected EventType = "peer_disconnected"

	// System events
	EventNodeStatus EventType = "node_status"
)

// WSEvent is a WebSocket event message.
type WSEvent struct {
	Type      EventType   `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// WSSubscription represents a subscription request.
type WSSubscription struct {
	Action string   `json:"action"` // "subscribe" or "unsubscribe"
	Events []string `json:"events"` // Event types to subscribe to
}

// WSClient represents a connected WebSocket client.
type WSClient struct {
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[EventType]bool
	mu            sync.RWMutex
	hub           *WSHub
}

// WSHub manages all WebSocket connections.
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan *WSEvent
	register   chan *WSClient
	unregister chan *WSClient
	log        *logging.Logger
	mu         sync.RWMutex
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan *WSEvent, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		log:        logging.GetDefault().Component("ws"),
	}
}

// Run starts the hub event loop.
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.log.Debug("WebSocket client connected", "clients", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.log.Debug("WebSocket client disconnected", "clients", len(h.clients))

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				h.log.Error("Failed to marshal event", "error", err)
				continue
			}

			h.mu.RLock()
			for client := range h.clients {
				// Check if client is subscribed to this event
				client.mu.RLock()
				subscribed := client.subscriptions[event.Type] || len(client.subscriptions) == 0
				client.mu.RUnlock()

				if !subscribed {
					continue
				}

				select {
				case client.send <- data:
				default:
					// Client's buffer is full, disconnect
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, client)
					close(client.send)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all subscribed clients.
func (h *WSHub) Broadcast(eventType EventType, data interface{}) {
	event := &WSEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	select {
	case h.broadcast <- event:
	default:
		h.log.Warn("Broadcast channel full, dropping event", "type", eventType)
	}
}

// ClientCount returns the number of connected clients.
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// handleWS handles WebSocket connections.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("WebSocket upgrade failed", "error", err)
		return
	}

	client := &WSClient{
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: make(map[EventType]bool),
		hub:           s.wsHub,
	}

	s.wsHub.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection.
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.log.Debug("WebSocket read error", "error", err)
			}
			break
		}

		// Handle subscription messages
		var sub WSSubscription
		if err := json.Unmarshal(message, &sub); err == nil {
			c.handleSubscription(&sub)
		}
	}
}

// writePump writes messages to the WebSocket connection.
func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleSubscription processes subscription requests.
func (c *WSClient) handleSubscription(sub *WSSubscription) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, eventStr := range sub.Events {
		eventType := EventType(eventStr)
		switch sub.Action {
		case "subscribe":
			c.subscriptions[eventType] = true
		case "unsubscribe":
			delete(c.subscriptions, eventType)
		}
	}
}
