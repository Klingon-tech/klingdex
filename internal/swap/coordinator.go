// Package swap - Coordinator manages active swaps and orchestrates the swap flow.
package swap

import (
	"context"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/wallet"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// NewCoordinator creates a new swap coordinator.
func NewCoordinator(cfg *CoordinatorConfig) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Coordinator{
		store:         cfg.Store,
		wallet:        cfg.Wallet,
		walletService: cfg.WalletService,
		backends:      cfg.Backends,
		network:       cfg.Network,
		swaps:         make(map[string]*ActiveSwap),
		eventHandlers: make([]EventHandler, 0),
		log:           logging.GetDefault().Component("swap"),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetWallet sets or updates the wallet.
func (c *Coordinator) SetWallet(w *wallet.Wallet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wallet = w
}

// SetWalletService sets or updates the wallet service.
func (c *Coordinator) SetWalletService(ws *wallet.Service) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.walletService = ws
}

// SetBackend sets or updates a backend for a chain.
func (c *Coordinator) SetBackend(chainSymbol string, b backend.Backend) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backends == nil {
		c.backends = make(map[string]backend.Backend)
	}
	c.backends[chainSymbol] = b
}

// OnEvent registers an event handler.
func (c *Coordinator) OnEvent(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandlers = append(c.eventHandlers, handler)
}

// emitEvent emits an event to all handlers.
// NOTE: Caller must hold c.mu (read or write lock).
func (c *Coordinator) emitEvent(tradeID, eventType string, data interface{}) {
	event := SwapEvent{
		TradeID:   tradeID,
		EventType: eventType,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Copy handlers while we already hold the lock (caller holds c.mu)
	handlers := make([]EventHandler, len(c.eventHandlers))
	copy(handlers, c.eventHandlers)

	for _, handler := range handlers {
		go handler(event)
	}
}

// Close shuts down the coordinator.
func (c *Coordinator) Close() error {
	c.cancel()
	return nil
}

