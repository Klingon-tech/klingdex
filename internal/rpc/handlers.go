package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Version of the node
const Version = "0.1.0-dev"

// ========================================
// Node handlers
// ========================================

// NodeInfoResult is the response for node_info.
type NodeInfoResult struct {
	PeerID   string   `json:"peer_id"`
	Addrs    []string `json:"addrs"`
	Peers    int      `json:"peers"`
	Uptime   string   `json:"uptime"`
	Version  string   `json:"version"`
	DataDir  string   `json:"data_dir"`
	MDNSEnabled bool  `json:"mdns_enabled"`
	DHTEnabled  bool  `json:"dht_enabled"`
}

func (s *Server) nodeInfo(ctx context.Context, params json.RawMessage) (interface{}, error) {
	addrs := make([]string, 0)
	for _, addr := range s.node.Addrs() {
		addrs = append(addrs, addr.String()+"/p2p/"+s.node.ID().String())
	}

	cfg := s.node.Config()

	return &NodeInfoResult{
		PeerID:      s.node.ID().String(),
		Addrs:       addrs,
		Peers:       s.node.PeerCount(),
		Uptime:      s.node.Uptime().Round(time.Second).String(),
		Version:     Version,
		DataDir:     cfg.Storage.DataDir,
		MDNSEnabled: cfg.Network.EnableMDNS,
		DHTEnabled:  cfg.Network.EnableDHT,
	}, nil
}

// NodeStatusResult is the response for node_status.
type NodeStatusResult struct {
	Running    bool   `json:"running"`
	PeerCount  int    `json:"peer_count"`
	KnownPeers int    `json:"known_peers"`
	Uptime     string `json:"uptime"`
	WSClients  int    `json:"ws_clients"`
}

func (s *Server) nodeStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	knownPeers := 0
	if s.store != nil {
		count, err := s.store.PeerCount()
		if err == nil {
			knownPeers = count
		}
	}

	wsClients := 0
	if s.wsHub != nil {
		wsClients = s.wsHub.ClientCount()
	}

	return &NodeStatusResult{
		Running:    true,
		PeerCount:  s.node.PeerCount(),
		KnownPeers: knownPeers,
		Uptime:     s.node.Uptime().Round(time.Second).String(),
		WSClients:  wsClients,
	}, nil
}

// ========================================
// Peers handlers
// ========================================

// PeerInfo represents information about a connected peer.
type PeerInfo struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs,omitempty"`
}

// PeersListResult is the response for peers_list.
type PeersListResult struct {
	Peers []PeerInfo `json:"peers"`
	Count int        `json:"count"`
}

func (s *Server) peersList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	peers := s.node.Peers()
	result := make([]PeerInfo, 0, len(peers))

	host := s.node.Host()
	for _, p := range peers {
		addrs := host.Peerstore().Addrs(p)
		addrStrs := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addrStrs = append(addrStrs, addr.String())
		}

		result = append(result, PeerInfo{
			PeerID: p.String(),
			Addrs:  addrStrs,
		})
	}

	return &PeersListResult{
		Peers: result,
		Count: len(result),
	}, nil
}

// PeersCountResult is the response for peers_count.
type PeersCountResult struct {
	Connected int `json:"connected"`
	Known     int `json:"known"`
}

func (s *Server) peersCount(ctx context.Context, params json.RawMessage) (interface{}, error) {
	knownPeers := 0
	if s.store != nil {
		count, err := s.store.PeerCount()
		if err == nil {
			knownPeers = count
		}
	}

	return &PeersCountResult{
		Connected: s.node.PeerCount(),
		Known:     knownPeers,
	}, nil
}

// ConnectParams is the parameters for peers_connect.
type ConnectParams struct {
	Addr string `json:"addr"`
}

func (s *Server) peersConnect(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ConnectParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Addr == "" {
		return nil, fmt.Errorf("addr is required")
	}

	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := s.node.ConnectByAddr(connectCtx, p.Addr); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"addr":    p.Addr,
	}, nil
}

// DisconnectParams is the parameters for peers_disconnect.
type DisconnectParams struct {
	PeerID string `json:"peer_id"`
}

func (s *Server) peersDisconnect(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p DisconnectParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.PeerID == "" {
		return nil, fmt.Errorf("peer_id is required")
	}

	peerID, err := peer.Decode(p.PeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer_id: %w", err)
	}

	// Close connection to peer
	if err := s.node.Host().Network().ClosePeer(peerID); err != nil {
		return nil, fmt.Errorf("failed to disconnect: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"peer_id": p.PeerID,
	}, nil
}

// KnownPeersParams is the parameters for peers_known.
type KnownPeersParams struct {
	Limit int `json:"limit"`
}

// KnownPeerInfo represents a known peer from storage.
type KnownPeerInfo struct {
	PeerID          string   `json:"peer_id"`
	Addrs           []string `json:"addrs"`
	FirstSeen       int64    `json:"first_seen"`
	LastSeen        int64    `json:"last_seen"`
	LastConnected   int64    `json:"last_connected,omitempty"`
	ConnectionCount int      `json:"connection_count"`
	IsBootstrap     bool     `json:"is_bootstrap"`
	IsConnected     bool     `json:"is_connected"`
}

// KnownPeersResult is the response for peers_known.
type KnownPeersResult struct {
	Peers []KnownPeerInfo `json:"peers"`
	Count int             `json:"count"`
}

func (s *Server) peersKnown(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p KnownPeersParams
	if params != nil {
		json.Unmarshal(params, &p)
	}

	if p.Limit == 0 {
		p.Limit = 100
	}

	if s.store == nil {
		return &KnownPeersResult{
			Peers: []KnownPeerInfo{},
			Count: 0,
		}, nil
	}

	records, err := s.store.ListPeers(p.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list peers: %w", err)
	}

	// Get connected peers for status check
	connectedPeers := make(map[string]bool)
	for _, p := range s.node.Peers() {
		connectedPeers[p.String()] = true
	}

	result := make([]KnownPeerInfo, 0, len(records))
	for _, r := range records {
		lastConnected := int64(0)
		if !r.LastConnected.IsZero() {
			lastConnected = r.LastConnected.Unix()
		}

		result = append(result, KnownPeerInfo{
			PeerID:          r.PeerID,
			Addrs:           r.Addresses,
			FirstSeen:       r.FirstSeen.Unix(),
			LastSeen:        r.LastSeen.Unix(),
			LastConnected:   lastConnected,
			ConnectionCount: r.ConnectionCount,
			IsBootstrap:     r.IsBootstrap,
			IsConnected:     connectedPeers[r.PeerID],
		})
	}

	return &KnownPeersResult{
		Peers: result,
		Count: len(result),
	}, nil
}
