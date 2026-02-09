package node

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"

	"github.com/Klingon-tech/klingdex/internal/storage"
)

// PeerStoreAdapter provides persistent storage for peer information.
type PeerStoreAdapter struct {
	store *storage.Storage
}

// NewPeerStoreAdapter creates a new peer store adapter.
func NewPeerStoreAdapter(store *storage.Storage) *PeerStoreAdapter {
	return &PeerStoreAdapter{store: store}
}

// SavePeer saves a peer's information to persistent storage.
func (a *PeerStoreAdapter) SavePeer(peerID peer.ID, addrs []multiaddr.Multiaddr, isBootstrap bool) error {
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	now := time.Now()
	record := &storage.PeerRecord{
		PeerID:      peerID.String(),
		Addresses:   addrStrs,
		FirstSeen:   now,
		LastSeen:    now,
		IsBootstrap: isBootstrap,
	}

	return a.store.SavePeer(record)
}

// UpdatePeerConnected updates the peer's connection timestamp.
func (a *PeerStoreAdapter) UpdatePeerConnected(peerID peer.ID) error {
	return a.store.UpdatePeerConnected(peerID.String())
}

// UpdatePeerSeen updates the peer's last seen timestamp.
func (a *PeerStoreAdapter) UpdatePeerSeen(peerID peer.ID) error {
	return a.store.UpdatePeerSeen(peerID.String())
}

// LoadPeers loads all known peers from storage.
func (a *PeerStoreAdapter) LoadPeers(limit int) ([]*storage.PeerRecord, error) {
	return a.store.ListPeers(limit)
}

// LoadRecentPeers loads peers seen within the given duration.
func (a *PeerStoreAdapter) LoadRecentPeers(since time.Duration, limit int) ([]*storage.PeerRecord, error) {
	return a.store.ListRecentPeers(since, limit)
}

// PeerCount returns the number of known peers.
func (a *PeerStoreAdapter) PeerCount() (int, error) {
	return a.store.PeerCount()
}

// SetPeerStoreAdapter sets the peer store adapter for the node.
func (n *Node) SetPeerStoreAdapter(adapter *PeerStoreAdapter) {
	n.mu.Lock()
	n.peerStoreAdapter = adapter
	n.mu.Unlock()
}

// LoadPersistedPeers loads known peers from storage into the peerstore.
func (n *Node) LoadPersistedPeers() error {
	n.mu.RLock()
	adapter := n.peerStoreAdapter
	n.mu.RUnlock()

	if adapter == nil {
		return nil // No adapter set
	}

	// Load peers seen in the last 7 days
	records, err := adapter.LoadRecentPeers(7*24*time.Hour, 100)
	if err != nil {
		return err
	}

	loaded := 0
	for _, record := range records {
		peerID, err := peer.Decode(record.PeerID)
		if err != nil {
			n.log.Debug("Invalid peer ID in storage", "peer", record.PeerID, "error", err)
			continue
		}

		if peerID == n.host.ID() {
			continue // Skip self
		}

		// Convert addresses
		addrs := make([]multiaddr.Multiaddr, 0, len(record.Addresses))
		for _, addrStr := range record.Addresses {
			addr, err := multiaddr.NewMultiaddr(addrStr)
			if err != nil {
				continue
			}
			addrs = append(addrs, addr)
		}

		if len(addrs) == 0 {
			continue
		}

		// Add to peerstore with temporary TTL
		n.host.Peerstore().AddAddrs(peerID, addrs, peerstore.TempAddrTTL)
		loaded++
	}

	if loaded > 0 {
		n.log.Info("Loaded persisted peers", "count", loaded)
	}

	return nil
}

// SavePeerCache saves the current peerstore to persistent storage.
func (n *Node) SavePeerCache() error {
	n.mu.RLock()
	adapter := n.peerStoreAdapter
	n.mu.RUnlock()

	if adapter == nil {
		return nil
	}

	// Get all peers from peerstore
	peers := n.host.Peerstore().Peers()
	saved := 0

	for _, peerID := range peers {
		if peerID == n.host.ID() {
			continue
		}

		addrs := n.host.Peerstore().Addrs(peerID)
		if len(addrs) == 0 {
			continue
		}

		if err := adapter.SavePeer(peerID, addrs, false); err != nil {
			n.log.Debug("Failed to save peer", "peer", shortID(peerID), "error", err)
			continue
		}
		saved++
	}

	if saved > 0 {
		n.log.Info("Saved peer cache", "count", saved)
	}

	return nil
}

// savePeerOnConnect saves peer info when they connect.
func (n *Node) savePeerOnConnect(peerID peer.ID) {
	n.mu.RLock()
	adapter := n.peerStoreAdapter
	n.mu.RUnlock()

	if adapter == nil {
		return
	}

	addrs := n.host.Peerstore().Addrs(peerID)
	if len(addrs) == 0 {
		return
	}

	// Save peer asynchronously
	if err := adapter.SavePeer(peerID, addrs, false); err != nil {
		n.log.Debug("Failed to save connected peer", "error", err)
	}
	adapter.UpdatePeerConnected(peerID)
}
