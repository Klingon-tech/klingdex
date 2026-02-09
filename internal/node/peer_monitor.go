// Package node - Monitors peer connection events for message flushing.
package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/Klingon-tech/klingdex/internal/storage"
	"github.com/Klingon-tech/klingdex/pkg/logging"
)

// PeerMonitor watches for peer connection events and triggers message flushing.
type PeerMonitor struct {
	node    *Node
	storage *storage.Storage
	sender  *MessageSender
	log     *logging.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPeerMonitor creates a new peer monitor.
func NewPeerMonitor(n *Node, store *storage.Storage, sender *MessageSender) *PeerMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &PeerMonitor{
		node:    n,
		storage: store,
		sender:  sender,
		log:     logging.GetDefault().Component("peer-monitor"),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts the peer monitor background goroutine.
func (m *PeerMonitor) Start() error {
	// Subscribe to peer connectedness events
	sub, err := m.node.Host().EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		return err
	}

	go m.run(sub)
	m.log.Info("Peer monitor started")
	return nil
}

// Stop stops the peer monitor.
func (m *PeerMonitor) Stop() {
	m.cancel()
	m.log.Info("Peer monitor stopped")
}

// run is the main loop of the peer monitor.
func (m *PeerMonitor) run(sub event.Subscription) {
	defer sub.Close()

	for {
		select {
		case <-m.ctx.Done():
			return
		case ev := <-sub.Out():
			e, ok := ev.(event.EvtPeerConnectednessChanged)
			if !ok {
				continue
			}

			m.handleConnectednessChange(e)
		}
	}
}

// handleConnectednessChange handles a peer connectedness change event.
func (m *PeerMonitor) handleConnectednessChange(e event.EvtPeerConnectednessChanged) {
	switch e.Connectedness {
	case network.Connected:
		m.handlePeerConnected(e.Peer)
	case network.NotConnected:
		m.handlePeerDisconnected(e.Peer)
	}
}

// handlePeerConnected handles when a peer connects.
func (m *PeerMonitor) handlePeerConnected(peerID peer.ID) {
	// Check if we have pending messages for this peer
	messages, err := m.storage.GetPendingForPeer(peerID.String())
	if err != nil {
		m.log.Warn("Failed to get pending messages for peer", "error", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	m.log.Info("Peer connected with pending messages",
		"peer", shortPeerID(peerID),
		"pending_count", len(messages))

	// Flush pending messages in background
	go m.sender.FlushPendingForPeer(m.ctx, peerID)
}

// handlePeerDisconnected handles when a peer disconnects.
func (m *PeerMonitor) handlePeerDisconnected(peerID peer.ID) {
	// Check if we have pending messages for this peer
	messages, err := m.storage.GetPendingForPeer(peerID.String())
	if err != nil {
		return
	}

	if len(messages) > 0 {
		m.log.Debug("Peer disconnected with pending messages",
			"peer", shortPeerID(peerID),
			"pending_count", len(messages))
	}
}
