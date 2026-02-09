package node

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"

	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)


// Node represents a Klingon P2P node.
type Node struct {
	host   host.Host
	dht    *dht.IpfsDHT
	pubsub *pubsub.PubSub
	config *Config
	log    *logging.Logger

	// Discovery
	mdnsService mdns.Service
	routingDisc *drouting.RoutingDiscovery

	// Peer persistence
	peerStoreAdapter *PeerStoreAdapter

	// Swap handler (PubSub for broadcasts)
	swapHandler *SwapHandler

	// Direct messaging (P2P streams for private messages)
	streamHandler *StreamHandler
	messageSender *MessageSender
	retryWorker   *RetryWorker
	peerMonitor   *PeerMonitor

	// State
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time

	// Callbacks
	onPeerConnected    func(peer.ID)
	onPeerDisconnected func(peer.ID)

	mu sync.RWMutex
}

// New creates a new Klingon P2P node.
func New(ctx context.Context, cfg *Config) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	node := &Node{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		log:    logging.GetDefault().Component("node"),
	}

	// Load or generate identity key
	privKey, err := node.loadOrCreateKey()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load/create key: %w", err)
	}

	// Parse listen addresses
	listenAddrs := make([]multiaddr.Multiaddr, 0, len(cfg.Network.ListenAddrs))
	for _, addr := range cfg.Network.ListenAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid listen address %s: %w", addr, err)
		}
		listenAddrs = append(listenAddrs, ma)
	}

	// Create connection manager
	cm, err := connmgr.NewConnManager(
		cfg.Network.ConnMgr.LowWater,
		cfg.Network.ConnMgr.HighWater,
		connmgr.WithGracePeriod(cfg.Network.ConnMgr.GracePeriod),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create connection manager: %w", err)
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.ConnectionManager(cm),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
	}

	// Add NAT options
	if cfg.Network.EnableNAT {
		opts = append(opts, libp2p.NATPortMap())
	}

	if cfg.Network.EnableRelay {
		opts = append(opts, libp2p.EnableRelay())
	}

	if cfg.Network.EnableHolePunching {
		opts = append(opts, libp2p.EnableHolePunching())
	}

	// Create host
	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}
	node.host = h

	// Set up connection notifications
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			node.mu.RLock()
			cb := node.onPeerConnected
			adapter := node.peerStoreAdapter
			node.mu.RUnlock()

			if cb != nil {
				go cb(conn.RemotePeer())
			}

			// Save peer on connect
			if adapter != nil {
				go node.savePeerOnConnect(conn.RemotePeer())
			}
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			node.mu.RLock()
			cb := node.onPeerDisconnected
			node.mu.RUnlock()
			if cb != nil {
				go cb(conn.RemotePeer())
			}
		},
	})

	// Initialize DHT
	if cfg.Network.EnableDHT {
		if err := node.initDHT(ctx); err != nil {
			h.Close()
			cancel()
			return nil, fmt.Errorf("failed to initialize DHT: %w", err)
		}
	}

	// Initialize PubSub
	if err := node.initPubSub(ctx); err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize pubsub: %w", err)
	}

	// Initialize mDNS discovery
	if cfg.Network.EnableMDNS {
		if err := node.initMDNS(); err != nil {
			// mDNS failure is not fatal
			node.log.Warn("mDNS initialization failed", "error", err)
		}
	}

	return node, nil
}

// loadOrCreateKey loads an existing private key or generates a new one.
func (n *Node) loadOrCreateKey() (crypto.PrivKey, error) {
	keyPath := n.config.Identity.KeyFile
	if !filepath.IsAbs(keyPath) {
		dataDir := expandPath(n.config.Storage.DataDir)
		keyPath = filepath.Join(dataDir, keyPath)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, err
	}

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		return crypto.UnmarshalPrivateKey(data)
	}

	// Generate new key
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save key
	data, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(keyPath, data, 0600); err != nil {
		return nil, err
	}

	n.log.Info("Generated new node identity")
	return privKey, nil
}

// initDHT initializes the Kademlia DHT.
func (n *Node) initDHT(ctx context.Context) error {
	var err error
	n.dht, err = dht.New(ctx, n.host,
		dht.Mode(dht.ModeAutoServer),
		dht.ProtocolPrefix(protocol.ID(n.config.DHTPrefix())),
	)
	if err != nil {
		return err
	}

	// Bootstrap the DHT
	if err := n.dht.Bootstrap(ctx); err != nil {
		return err
	}

	// Create routing discovery
	n.routingDisc = drouting.NewRoutingDiscovery(n.dht)

	return nil
}

// initPubSub initializes GossipSub.
func (n *Node) initPubSub(ctx context.Context) error {
	var err error
	n.pubsub, err = pubsub.NewGossipSub(ctx, n.host,
		pubsub.WithPeerExchange(true),
		pubsub.WithFloodPublish(true),
	)
	return err
}

// initMDNS initializes mDNS discovery for local network peers.
func (n *Node) initMDNS() error {
	n.mdnsService = mdns.NewMdnsService(n.host, n.config.DiscoveryNamespace(), n)
	return n.mdnsService.Start()
}

// HandlePeerFound is called when mDNS discovers a peer.
func (n *Node) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.host.ID() {
		return // Ignore self
	}

	// Add peer addresses to peerstore
	n.host.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)

	// Try to connect
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
		defer cancel()
		if err := n.host.Connect(ctx, pi); err != nil {
			n.log.Debug("Failed to connect to mDNS peer", "peer", shortID(pi.ID), "error", err)
		}
	}()
}

// Start starts the node and connects to bootstrap peers.
func (n *Node) Start() error {
	n.startTime = time.Now()

	// Connect to bootstrap peers
	for _, addrStr := range n.config.Network.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			n.log.Warn("Invalid bootstrap address", "addr", addrStr, "error", err)
			continue
		}

		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			n.log.Warn("Invalid bootstrap peer info", "addr", addrStr, "error", err)
			continue
		}

		go func(pi peer.AddrInfo) {
			ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
			defer cancel()
			if err := n.host.Connect(ctx, pi); err != nil {
				n.log.Warn("Failed to connect to bootstrap peer", "peer", shortID(pi.ID), "error", err)
			} else {
				n.log.Info("Connected to bootstrap peer", "peer", shortID(pi.ID))
			}
		}(*pi)
	}

	// Advertise ourselves for discovery
	if n.routingDisc != nil {
		go func() {
			dutil.Advertise(n.ctx, n.routingDisc, n.config.DiscoveryNamespace())
		}()

		// Start peer discovery loop
		go n.discoverPeers()
	}

	// Initialize swap handler if pubsub is available
	if n.pubsub != nil {
		swapHandler, err := NewSwapHandler(n)
		if err != nil {
			n.log.Warn("Failed to create swap handler", "error", err)
		} else {
			if err := swapHandler.Start(); err != nil {
				n.log.Warn("Failed to start swap handler", "error", err)
			} else {
				n.swapHandler = swapHandler
			}
		}
	}

	return nil
}

// SwapHandler returns the swap message handler.
func (n *Node) SwapHandler() *SwapHandler {
	return n.swapHandler
}

// GetTopic returns a PubSub topic by name.
// Used by message sender to publish encrypted messages.
func (n *Node) GetTopic(topicName string) *pubsub.Topic {
	if n.swapHandler == nil {
		return nil
	}
	switch topicName {
	case SwapEncryptedTopic:
		return n.swapHandler.GetEncryptedTopic()
	default:
		return nil
	}
}

// discoverPeers continuously discovers new peers.
func (n *Node) discoverPeers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			peers, err := dutil.FindPeers(n.ctx, n.routingDisc, n.config.DiscoveryNamespace())
			if err != nil {
				continue
			}

			for _, pi := range peers {
				if pi.ID == n.host.ID() {
					continue
				}

				// Already connected?
				if n.host.Network().Connectedness(pi.ID) == network.Connected {
					continue
				}

				go func(pi peer.AddrInfo) {
					ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
					defer cancel()
					n.host.Connect(ctx, pi)
				}(pi)
			}
		}
	}
}

// Stop stops the node gracefully.
func (n *Node) Stop() error {
	n.cancel()

	// Stop direct messaging components first
	if n.retryWorker != nil {
		n.retryWorker.Stop()
	}

	if n.peerMonitor != nil {
		n.peerMonitor.Stop()
	}

	if n.streamHandler != nil {
		n.streamHandler.Stop()
	}

	// Stop swap handler (PubSub)
	if n.swapHandler != nil {
		n.swapHandler.Stop()
	}

	// Stop discovery
	if n.mdnsService != nil {
		n.mdnsService.Close()
	}

	if n.dht != nil {
		n.dht.Close()
	}

	return n.host.Close()
}

// ID returns the node's peer ID.
func (n *Node) ID() peer.ID {
	return n.host.ID()
}

// Addrs returns the node's listen addresses.
func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.host.Addrs()
}

// Host returns the underlying libp2p host.
func (n *Node) Host() host.Host {
	return n.host
}

// DHT returns the Kademlia DHT.
func (n *Node) DHT() *dht.IpfsDHT {
	return n.dht
}

// PubSub returns the GossipSub instance.
func (n *Node) PubSub() *pubsub.PubSub {
	return n.pubsub
}

// Peers returns the list of connected peers.
func (n *Node) Peers() []peer.ID {
	return n.host.Network().Peers()
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	return len(n.host.Network().Peers())
}

// Connect connects to a peer.
func (n *Node) Connect(ctx context.Context, pi peer.AddrInfo) error {
	return n.host.Connect(ctx, pi)
}

// ConnectByAddr connects to a peer by multiaddr string.
func (n *Node) ConnectByAddr(ctx context.Context, addr string) error {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	pi, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("invalid peer addr info: %w", err)
	}

	return n.host.Connect(ctx, *pi)
}

// OnPeerConnected sets a callback for when a peer connects.
func (n *Node) OnPeerConnected(cb func(peer.ID)) {
	n.mu.Lock()
	n.onPeerConnected = cb
	n.mu.Unlock()
}

// OnPeerDisconnected sets a callback for when a peer disconnects.
func (n *Node) OnPeerDisconnected(cb func(peer.ID)) {
	n.mu.Lock()
	n.onPeerDisconnected = cb
	n.mu.Unlock()
}

// Uptime returns how long the node has been running.
func (n *Node) Uptime() time.Duration {
	return time.Since(n.startTime)
}

// Config returns the node configuration.
func (n *Node) Config() *Config {
	return n.config
}

// shortID returns a truncated peer ID for logging.
func shortID(p peer.ID) string {
	s := p.String()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// =============================================================================
// Direct Messaging Support
// =============================================================================

// SetupDirectMessaging initializes the direct P2P messaging layer.
// This must be called after the node is created and before Start().
func (n *Node) SetupDirectMessaging(store *storage.Storage) error {
	// Create stream handler
	n.streamHandler = NewStreamHandler(n, store)
	if err := n.streamHandler.Start(); err != nil {
		return fmt.Errorf("failed to start stream handler: %w", err)
	}

	// Create message sender with default config
	senderCfg := DefaultMessageSenderConfig()
	n.messageSender = NewMessageSender(n, store, n.streamHandler, senderCfg)

	// Create and start retry worker
	retryCfg := DefaultRetryWorkerConfig()
	n.retryWorker = NewRetryWorker(n, store, n.messageSender, retryCfg)
	n.retryWorker.Start()

	// Create and start peer monitor
	n.peerMonitor = NewPeerMonitor(n, store, n.messageSender)
	if err := n.peerMonitor.Start(); err != nil {
		n.log.Warn("Failed to start peer monitor", "error", err)
		// Not fatal - direct messaging can still work without event-based flushing
	}

	n.log.Info("Direct messaging initialized")
	return nil
}

// StreamHandler returns the direct stream handler.
func (n *Node) StreamHandler() *StreamHandler {
	return n.streamHandler
}

// MessageSender returns the message sender for direct P2P messaging.
func (n *Node) MessageSender() *MessageSender {
	return n.messageSender
}

// SendDirect sends a message directly to a peer with persistence and retry.
// This is the primary method for sending swap protocol messages.
func (n *Node) SendDirect(ctx context.Context, peerID peer.ID, tradeID string, swapTimeout int64, msg *SwapMessage) error {
	if n.messageSender == nil {
		return fmt.Errorf("direct messaging not initialized")
	}
	return n.messageSender.SendDirect(ctx, peerID, tradeID, swapTimeout, msg)
}

// RegisterDirectHandler registers a handler for direct messages of a specific type.
func (n *Node) RegisterDirectHandler(msgType string, handler SwapMessageHandler) {
	if n.streamHandler != nil {
		n.streamHandler.OnMessage(msgType, handler)
	}
}
