// Package swap - Unified secret monitoring for cross-chain atomic swaps.
// This file provides a unified interface for extracting secrets from both
// EVM claim events and Bitcoin witness data.
package swap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/Klingon-tech/klingdex/internal/backend"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/pkg/logging"
)

// SecretSource indicates where the secret was extracted from.
type SecretSource string

const (
	SecretSourceEVMClaim     SecretSource = "evm_claim"
	SecretSourceBitcoinWitness SecretSource = "bitcoin_witness"
	SecretSourceManual       SecretSource = "manual"
)

// SecretRevealEvent is emitted when a secret is discovered.
type SecretRevealEvent struct {
	TradeID    string
	Secret     [32]byte
	SecretHash [32]byte
	Source     SecretSource
	Chain      string
	TxHash     string
	Timestamp  time.Time
}

// SecretMonitor watches for secret reveals across multiple chains.
type SecretMonitor struct {
	mu sync.RWMutex

	// Dependencies
	coordinator *Coordinator
	backends    map[string]backend.Backend
	network     chain.Network
	log         *logging.Logger

	// Active monitors
	monitors map[string]context.CancelFunc // tradeID -> cancel func

	// Event channel
	events chan SecretRevealEvent

	// Context
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSecretMonitor creates a new secret monitor.
func NewSecretMonitor(coordinator *Coordinator, backends map[string]backend.Backend, network chain.Network) *SecretMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &SecretMonitor{
		coordinator: coordinator,
		backends:    backends,
		network:     network,
		log:         logging.Default().Component("secret-monitor"),
		monitors:    make(map[string]context.CancelFunc),
		events:      make(chan SecretRevealEvent, 100),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Events returns the channel for secret reveal events.
func (m *SecretMonitor) Events() <-chan SecretRevealEvent {
	return m.events
}

// StartMonitoring starts monitoring for secret reveals for a swap.
func (m *SecretMonitor) StartMonitoring(tradeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already monitoring
	if _, exists := m.monitors[tradeID]; exists {
		return nil // Already monitoring
	}

	// Get the active swap
	active, err := m.coordinator.GetSwap(tradeID)
	if err != nil {
		return fmt.Errorf("swap not found: %w", err)
	}

	// Create context for this monitor
	ctx, cancel := context.WithCancel(m.ctx)
	m.monitors[tradeID] = cancel

	// Start monitoring goroutines based on chain types
	swapType := GetCrossChainSwapType(
		active.Swap.Offer.OfferChain,
		active.Swap.Offer.RequestChain,
		m.network,
	)

	switch swapType {
	case CrossChainTypeEVMToEVM:
		// Monitor both EVM chains
		go m.monitorEVMChain(ctx, tradeID, active.Swap.Offer.OfferChain)
		go m.monitorEVMChain(ctx, tradeID, active.Swap.Offer.RequestChain)

	case CrossChainTypeBitcoinToBitcoin:
		// Monitor both Bitcoin chains
		go m.monitorBitcoinChain(ctx, tradeID, active.Swap.Offer.OfferChain)
		go m.monitorBitcoinChain(ctx, tradeID, active.Swap.Offer.RequestChain)

	case CrossChainTypeBitcoinToEVM:
		// Bitcoin offer, EVM request
		go m.monitorBitcoinChain(ctx, tradeID, active.Swap.Offer.OfferChain)
		go m.monitorEVMChain(ctx, tradeID, active.Swap.Offer.RequestChain)

	case CrossChainTypeEVMToBitcoin:
		// EVM offer, Bitcoin request
		go m.monitorEVMChain(ctx, tradeID, active.Swap.Offer.OfferChain)
		go m.monitorBitcoinChain(ctx, tradeID, active.Swap.Offer.RequestChain)

	default:
		cancel()
		delete(m.monitors, tradeID)
		return fmt.Errorf("unsupported cross-chain type: %s", swapType)
	}

	m.log.Info("Started secret monitoring",
		"trade_id", tradeID,
		"swap_type", swapType.String(),
	)

	return nil
}

// StopMonitoring stops monitoring for a specific swap.
func (m *SecretMonitor) StopMonitoring(tradeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.monitors[tradeID]; exists {
		cancel()
		delete(m.monitors, tradeID)
		m.log.Debug("Stopped secret monitoring", "trade_id", tradeID)
	}
}

// Stop stops all monitoring.
func (m *SecretMonitor) Stop() {
	m.cancel()
	m.mu.Lock()
	for tradeID, cancel := range m.monitors {
		cancel()
		delete(m.monitors, tradeID)
	}
	m.mu.Unlock()
	close(m.events)
}

// =============================================================================
// EVM Monitoring
// =============================================================================

func (m *SecretMonitor) monitorEVMChain(ctx context.Context, tradeID, chainSymbol string) {
	m.log.Debug("Starting EVM chain monitor", "trade_id", tradeID, "chain", chainSymbol)

	// Get the active swap and EVM session
	active, err := m.coordinator.GetSwap(tradeID)
	if err != nil {
		m.log.Error("Failed to get swap for monitoring", "error", err)
		return
	}

	if active.EVMHTLC == nil {
		m.log.Debug("No EVM HTLC data, skipping EVM monitor", "trade_id", tradeID)
		return
	}

	// Get the appropriate session
	var session *EVMHTLCSession
	if chainSymbol == active.Swap.Offer.OfferChain && active.EVMHTLC.OfferChain != nil {
		session = active.EVMHTLC.OfferChain.Session
	} else if chainSymbol == active.Swap.Offer.RequestChain && active.EVMHTLC.RequestChain != nil {
		session = active.EVMHTLC.RequestChain.Session
	}

	if session == nil {
		m.log.Debug("No EVM session for chain", "trade_id", tradeID, "chain", chainSymbol)
		return
	}

	// Wait for secret
	secret, err := session.WaitForSecret(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return // Context cancelled, normal shutdown
		}
		m.log.Error("Error waiting for EVM secret", "error", err)
		return
	}

	// Emit event
	secretHash := session.GetSecretHash()
	event := SecretRevealEvent{
		TradeID:    tradeID,
		Secret:     secret,
		SecretHash: secretHash,
		Source:     SecretSourceEVMClaim,
		Chain:      chainSymbol,
		Timestamp:  time.Now(),
	}

	select {
	case m.events <- event:
		m.log.Info("Secret revealed from EVM claim",
			"trade_id", tradeID,
			"chain", chainSymbol,
		)
	case <-ctx.Done():
		return
	}

	// Propagate secret to coordinator
	m.propagateSecret(tradeID, secret)
}

// =============================================================================
// Bitcoin Monitoring
// =============================================================================

func (m *SecretMonitor) monitorBitcoinChain(ctx context.Context, tradeID, chainSymbol string) {
	m.log.Debug("Starting Bitcoin chain monitor", "trade_id", tradeID, "chain", chainSymbol)

	// Get the active swap
	active, err := m.coordinator.GetSwap(tradeID)
	if err != nil {
		m.log.Error("Failed to get swap for monitoring", "error", err)
		return
	}

	if active.HTLC == nil {
		m.log.Debug("No HTLC data, skipping Bitcoin monitor", "trade_id", tradeID)
		return
	}

	// Get the appropriate session and address
	var htlcAddress string
	var secretHash []byte

	if chainSymbol == active.Swap.Offer.OfferChain && active.HTLC.OfferChain != nil {
		htlcAddress = active.HTLC.OfferChain.HTLCAddress
		if active.HTLC.OfferChain.Session != nil {
			secretHash = active.HTLC.OfferChain.Session.GetSecretHash()
		}
	} else if chainSymbol == active.Swap.Offer.RequestChain && active.HTLC.RequestChain != nil {
		htlcAddress = active.HTLC.RequestChain.HTLCAddress
		if active.HTLC.RequestChain.Session != nil {
			secretHash = active.HTLC.RequestChain.Session.GetSecretHash()
		}
	}

	if htlcAddress == "" {
		m.log.Debug("No HTLC address for chain", "trade_id", tradeID, "chain", chainSymbol)
		return
	}

	// Get backend
	b, ok := m.backends[chainSymbol]
	if !ok {
		m.log.Error("No backend for chain", "chain", chainSymbol)
		return
	}

	// Poll for claim transaction
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			secret, txHash, err := m.checkBitcoinClaim(ctx, b, htlcAddress, secretHash)
			if err != nil {
				m.log.Debug("No claim found yet", "chain", chainSymbol, "error", err)
				continue
			}

			if len(secret) == 32 {
				var secretArr [32]byte
				copy(secretArr[:], secret)
				var hashArr [32]byte
				copy(hashArr[:], secretHash)

				event := SecretRevealEvent{
					TradeID:    tradeID,
					Secret:     secretArr,
					SecretHash: hashArr,
					Source:     SecretSourceBitcoinWitness,
					Chain:      chainSymbol,
					TxHash:     txHash,
					Timestamp:  time.Now(),
				}

				select {
				case m.events <- event:
					m.log.Info("Secret revealed from Bitcoin witness",
						"trade_id", tradeID,
						"chain", chainSymbol,
						"tx_hash", txHash,
					)
				case <-ctx.Done():
					return
				}

				// Propagate secret
				m.propagateSecret(tradeID, secretArr)
				return
			}
		}
	}
}

// checkBitcoinClaim checks if the HTLC has been claimed and extracts the secret.
func (m *SecretMonitor) checkBitcoinClaim(ctx context.Context, b backend.Backend, htlcAddress string, expectedHash []byte) (secret []byte, txHash string, err error) {
	// Get transaction history for the HTLC address
	txs, err := b.GetAddressTxs(ctx, htlcAddress, "")
	if err != nil {
		return nil, "", fmt.Errorf("failed to get address transactions: %w", err)
	}

	// Look for spending transaction
	for _, tx := range txs {
		// Skip if this is the funding tx (output to HTLC address)
		isSpending := false
		for _, input := range tx.Inputs {
			if input.PrevOut != nil && input.PrevOut.ScriptPubKeyAddr == htlcAddress {
				isSpending = true
				break
			}
		}
		if !isSpending {
			continue
		}

		// Extract secret from witness data
		for _, input := range tx.Inputs {
			if input.PrevOut == nil || input.PrevOut.ScriptPubKeyAddr != htlcAddress {
				continue
			}

			// Check witness for secret (second-to-last item in claim path)
			if len(input.Witness) >= 2 {
				// In HTLC claim, witness is: [signature, secret, htlc_script]
				// Secret is typically the second item
				for i, witnessItem := range input.Witness {
					witnessBytes, err := hex.DecodeString(witnessItem)
					if err != nil {
						continue
					}

					// Secret should be 32 bytes
					if len(witnessBytes) == 32 {
						// Verify it matches the expected hash
						if len(expectedHash) == 32 {
							hash := HashSecretBytes(witnessBytes)
							if common.Bytes2Hex(hash) == common.Bytes2Hex(expectedHash) {
								return witnessBytes, tx.TxID, nil
							}
						} else {
							// If no hash to verify, just return the 32-byte value
							// This might be the secret
							m.log.Debug("Found potential secret in witness",
								"tx", tx.TxID,
								"witness_index", i,
							)
							return witnessBytes, tx.TxID, nil
						}
					}
				}
			}
		}
	}

	return nil, "", fmt.Errorf("no claim transaction found")
}

// =============================================================================
// Helper Methods
// =============================================================================

// propagateSecret stores the secret in the swap and propagates to other sessions.
func (m *SecretMonitor) propagateSecret(tradeID string, secret [32]byte) {
	active, err := m.coordinator.GetSwap(tradeID)
	if err != nil {
		m.log.Error("Failed to get swap for secret propagation", "error", err)
		return
	}

	// Store in swap
	active.Swap.Secret = secret[:]

	// Propagate to EVM sessions
	if active.EVMHTLC != nil {
		if active.EVMHTLC.OfferChain != nil && active.EVMHTLC.OfferChain.Session != nil {
			if err := active.EVMHTLC.OfferChain.Session.SetSecret(secret); err != nil {
				m.log.Debug("Failed to set secret in offer EVM session", "error", err)
			}
		}
		if active.EVMHTLC.RequestChain != nil && active.EVMHTLC.RequestChain.Session != nil {
			if err := active.EVMHTLC.RequestChain.Session.SetSecret(secret); err != nil {
				m.log.Debug("Failed to set secret in request EVM session", "error", err)
			}
		}
	}

	// Propagate to Bitcoin HTLC sessions
	if active.HTLC != nil {
		if active.HTLC.OfferChain != nil && active.HTLC.OfferChain.Session != nil {
			active.HTLC.OfferChain.Session.SetSecret(secret[:])
		}
		if active.HTLC.RequestChain != nil && active.HTLC.RequestChain.Session != nil {
			active.HTLC.RequestChain.Session.SetSecret(secret[:])
		}
	}

	m.log.Info("Secret propagated to all sessions", "trade_id", tradeID)
}

// HashSecretBytes computes SHA256 of secret bytes.
func HashSecretBytes(secret []byte) []byte {
	hash := sha256.Sum256(secret)
	return hash[:]
}
