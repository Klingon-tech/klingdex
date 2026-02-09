// Package swap - Confirmation monitor for tracking funding transactions.
package swap

import (
	"context"
	"sync"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

// Monitor watches for transaction confirmations and updates swap state.
type Monitor struct {
	coordinator *Coordinator
	backends    map[string]backend.Backend
	log         *logging.Logger

	// Polling interval
	interval time.Duration

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
}

// MonitorConfig holds configuration for the Monitor.
type MonitorConfig struct {
	Coordinator *Coordinator
	Backends    map[string]backend.Backend
	Interval    time.Duration // Polling interval, default 30s
}

// NewMonitor creates a new confirmation monitor.
func NewMonitor(cfg *MonitorConfig) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())

	interval := cfg.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	return &Monitor{
		coordinator: cfg.Coordinator,
		backends:    cfg.Backends,
		log:         logging.GetDefault().Component("swap-monitor"),
		interval:    interval,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetBackend sets or updates a backend for a chain.
func (m *Monitor) SetBackend(chainSymbol string, b backend.Backend) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.backends == nil {
		m.backends = make(map[string]backend.Backend)
	}
	m.backends[chainSymbol] = b
}

// Start starts the confirmation monitor.
func (m *Monitor) Start() {
	go m.run()
	m.log.Info("Confirmation monitor started", "interval", m.interval)
}

// Stop stops the confirmation monitor.
func (m *Monitor) Stop() {
	m.cancel()
	m.log.Info("Confirmation monitor stopped")
}

// run is the main monitoring loop.
func (m *Monitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllSwaps()
		}
	}
}

// checkAllSwaps checks confirmations for all active swaps.
func (m *Monitor) checkAllSwaps() {
	if m.coordinator == nil {
		return
	}

	m.coordinator.mu.RLock()
	swapIDs := make([]string, 0, len(m.coordinator.swaps))
	for id := range m.coordinator.swaps {
		swapIDs = append(swapIDs, id)
	}
	m.coordinator.mu.RUnlock()

	for _, tradeID := range swapIDs {
		if err := m.checkSwapConfirmations(tradeID); err != nil {
			m.log.Debug("Error checking confirmations", "trade_id", tradeID, "error", err)
		}
	}
}

// checkSwapConfirmations checks confirmations for a specific swap.
func (m *Monitor) checkSwapConfirmations(tradeID string) error {
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	return m.coordinator.UpdateConfirmations(ctx, tradeID)
}

// CheckNow triggers an immediate check for a specific swap.
func (m *Monitor) CheckNow(tradeID string) error {
	return m.checkSwapConfirmations(tradeID)
}

// WaitForConfirmations waits for a transaction to reach the required confirmations.
// Returns when confirmed or context is cancelled.
func (m *Monitor) WaitForConfirmations(ctx context.Context, chainSymbol, txID string, required uint32) error {
	m.mu.RLock()
	b, ok := m.backends[chainSymbol]
	m.mu.RUnlock()

	if !ok {
		return ErrNoBackend
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			tx, err := b.GetTransaction(ctx, txID)
			if err != nil {
				m.log.Debug("Error getting transaction", "txid", txID, "error", err)
				continue
			}

			if tx.Confirmations >= int64(required) {
				m.log.Info("Transaction confirmed",
					"txid", txID,
					"confirmations", tx.Confirmations,
					"required", required,
				)
				return nil
			}

			m.log.Debug("Waiting for confirmations",
				"txid", txID,
				"current", tx.Confirmations,
				"required", required,
			)
		}
	}
}

// GetConfirmations returns the current confirmation count for a transaction.
func (m *Monitor) GetConfirmations(ctx context.Context, chainSymbol, txID string) (int64, error) {
	m.mu.RLock()
	b, ok := m.backends[chainSymbol]
	m.mu.RUnlock()

	if !ok {
		return 0, ErrNoBackend
	}

	tx, err := b.GetTransaction(ctx, txID)
	if err != nil {
		return 0, err
	}

	return tx.Confirmations, nil
}
