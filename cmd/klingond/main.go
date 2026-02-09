// Package main provides the klingond daemon - a minimal P2P node.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/node"
	"github.com/klingon-exchange/klingon-v2/internal/rpc"
	"github.com/klingon-exchange/klingon-v2/internal/storage"
	"github.com/klingon-exchange/klingon-v2/internal/swap"
	"github.com/klingon-exchange/klingon-v2/internal/sync"
	"github.com/klingon-exchange/klingon-v2/internal/wallet"
	"github.com/klingon-exchange/klingon-v2/pkg/logging"
)

var (
	version = "0.1.0-dev"
	commit  = "unknown"
)

func main() {
	// Parse flags
	var (
		dataDir        = flag.String("data-dir", "~/.klingon", "Data directory")
		configFile     = flag.String("config", "", "Config file path (default: <data-dir>/config.yaml)")
		listenAddr     = flag.String("listen", "", "Listen address (multiaddr), overrides config")
		apiAddr        = flag.String("api", "127.0.0.1:8080", "JSON-RPC API address")
		enableMDNS     = flag.Bool("mdns", true, "Enable mDNS discovery")
		enableDHT      = flag.Bool("dht", true, "Enable DHT discovery")
		testnet        = flag.Bool("testnet", false, "Run on testnet (separate network and data)")
		bootstrapPeers = flag.String("bootstrap", "", "Bootstrap peers (comma-separated multiaddrs)")
		logLevel       = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		showVersion    = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	// Set up logging (initial, may be overridden by config)
	log := logging.New(&logging.Config{
		Level:      *logLevel,
		TimeFormat: time.TimeOnly,
	})
	logging.SetDefault(log)

	if *showVersion {
		log.Infof("klingond %s (commit: %s)", version, commit)
		os.Exit(0)
	}

	// Determine data directory (testnet uses subdirectory)
	effectiveDataDir := *dataDir
	if *testnet {
		effectiveDataDir = filepath.Join(*dataDir, "testnet")
	}

	// Load or create config file
	var cfg *node.Config
	var err error

	if *configFile != "" {
		// Use specified config file
		cfg, err = node.LoadConfig(filepath.Dir(*configFile))
	} else {
		// Use default config location in data directory
		cfg, err = node.LoadConfig(effectiveDataDir)
	}
	if err != nil {
		log.Fatal("Failed to load config", "error", err)
	}

	// Apply CLI overrides (CLI flags take precedence over config file)
	if *listenAddr != "" {
		cfg.Network.ListenAddrs = []string{*listenAddr}
	}
	cfg.Network.EnableMDNS = *enableMDNS
	cfg.Network.EnableDHT = *enableDHT
	cfg.Logging.Level = *logLevel
	cfg.Storage.DataDir = effectiveDataDir

	// Set network type
	if *testnet {
		cfg.NetworkType = node.NetworkTestnet
	} else {
		cfg.NetworkType = node.NetworkMainnet
	}

	if *bootstrapPeers != "" {
		cfg.Network.BootstrapPeers = parseBootstrapPeers(*bootstrapPeers)
	}

	// Update logging with config level
	log = logging.New(&logging.Config{
		Level:      cfg.Logging.Level,
		TimeFormat: time.TimeOnly,
	})
	logging.SetDefault(log)

	log.Info("Config loaded", "path", node.ConfigPath(effectiveDataDir))

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize storage
	dataPath := expandPath(cfg.Storage.DataDir)
	storeCfg := &storage.Config{
		DataDir: dataPath,
	}
	store, err := storage.New(storeCfg)
	if err != nil {
		log.Fatal("Failed to initialize storage", "error", err)
	}
	defer store.Close()
	log.Info("Storage initialized", "path", dataPath)

	// Initialize wallet service
	walletNetwork := chain.Mainnet
	if *testnet {
		walletNetwork = chain.Testnet
	}

	// Initialize backend registry for blockchain access
	backendRegistry := backend.NewDefaultRegistry(walletNetwork)
	log.Info("Backend registry initialized", "network", walletNetwork, "backends", backendRegistry.List())

	walletService := wallet.NewService(&wallet.ServiceConfig{
		DataDir:  dataPath,
		Network:  walletNetwork,
		Backends: backendRegistry,
	})
	log.Info("Wallet service initialized", "network", walletNetwork)

	// Initialize swap coordinator with backends and wallet service
	coordinator := swap.NewCoordinator(&swap.CoordinatorConfig{
		Store:         store,
		Network:       walletNetwork,
		Backends:      backendRegistry.All(),
		WalletService: walletService,
	})
	defer coordinator.Close()
	log.Info("Swap coordinator initialized")

	// Load pending swaps from database on startup
	if err := coordinator.LoadPendingSwaps(ctx); err != nil {
		log.Warn("Failed to load pending swaps", "error", err)
	} else {
		log.Info("Pending swaps loaded from database")
	}

	// Create node
	log.Info("Starting Klingon P2P Node...")
	n, err := node.New(ctx, cfg)
	if err != nil {
		log.Fatal("Failed to create node", "error", err)
	}

	// Set up peer store persistence
	peerStoreAdapter := node.NewPeerStoreAdapter(store)
	n.SetPeerStoreAdapter(peerStoreAdapter)

	// Load persisted peers before starting
	if err := n.LoadPersistedPeers(); err != nil {
		log.Warn("Failed to load persisted peers", "error", err)
	}

	// Initialize direct P2P messaging (for private swap messages with persistence)
	if err := n.SetupDirectMessaging(store); err != nil {
		log.Warn("Failed to setup direct messaging", "error", err)
	} else {
		log.Info("Direct P2P messaging initialized")
	}

	// Start node
	if err := n.Start(); err != nil {
		log.Fatal("Failed to start node", "error", err)
	}

	// Start RPC server
	rpcServer := rpc.NewServer(n, store, walletService, coordinator)
	if err := rpcServer.Start(*apiAddr); err != nil {
		log.Fatal("Failed to start RPC server", "error", err)
	}

	// Set up swap message handlers (for order broadcasting, etc.)
	rpcServer.SetupSwapHandlers()

	// Initialize order and trade sync services
	orderSync := sync.NewOrderSync(n.Host(), store, nil)
	if err := orderSync.Start(); err != nil {
		log.Warn("Failed to start order sync", "error", err)
	}

	tradeSync := sync.NewTradeSync(n.Host(), store)
	if err := tradeSync.Start(); err != nil {
		log.Warn("Failed to start trade sync", "error", err)
	}

	log.Info("Order/trade sync initialized")

	// Print node info
	printBanner(log, n, cfg, *apiAddr)

	// Set up peer connection logging and WebSocket broadcasting
	nodeLog := log.Component("p2p")
	n.OnPeerConnected(func(p peer.ID) {
		nodeLog.Info("Peer connected", "peer", shortID(p), "total", n.PeerCount())
		// Broadcast to WebSocket clients
		if hub := rpcServer.WSHub(); hub != nil {
			hub.Broadcast(rpc.EventPeerConnected, map[string]interface{}{
				"peer_id":     p.String(),
				"total_peers": n.PeerCount(),
			})
		}
	})

	n.OnPeerDisconnected(func(p peer.ID) {
		nodeLog.Info("Peer disconnected", "peer", shortID(p), "total", n.PeerCount())
		// Broadcast to WebSocket clients
		if hub := rpcServer.WSHub(); hub != nil {
			hub.Broadcast(rpc.EventPeerDisconnected, map[string]interface{}{
				"peer_id":     p.String(),
				"total_peers": n.PeerCount(),
			})
		}
	})

	// Start status ticker
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Info("Status", "peers", n.PeerCount(), "uptime", n.Uptime().Round(time.Second))
			}
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	log.Info("Shutting down...")

	// Save peer cache before shutdown
	if err := n.SavePeerCache(); err != nil {
		log.Error("Error saving peer cache", "error", err)
	}

	// Graceful shutdown
	cancel()

	// Stop sync services
	if orderSync != nil {
		orderSync.Stop()
	}
	if tradeSync != nil {
		tradeSync.Stop()
	}

	if err := rpcServer.Stop(); err != nil {
		log.Error("Error stopping RPC server", "error", err)
	}
	if err := n.Stop(); err != nil {
		log.Error("Error during shutdown", "error", err)
	}

	log.Info("Goodbye!")
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func printBanner(log *logging.Logger, n *node.Node, cfg *node.Config, apiAddr string) {
	networkLabel := "mainnet"
	if cfg.IsTestnet() {
		networkLabel = "TESTNET"
	}

	log.Info("")
	log.Info("=================================================")
	log.Infof("  Klingon P2P Node (%s)", networkLabel)
	log.Infof("  Version: %s", version)
	log.Info("=================================================")
	log.Info("")
	log.Infof("  Peer ID: %s", n.ID().String())
	log.Info("")
	log.Info("  Listening on:")
	for _, addr := range n.Addrs() {
		log.Infof("    %s/p2p/%s", addr.String(), n.ID().String())
	}
	log.Info("")
	log.Infof("  API: http://%s", apiAddr)
	log.Infof("  WS:  ws://%s/ws", apiAddr)
	log.Info("")
	log.Infof("  Network: %s | mDNS: %v | DHT: %v", networkLabel, cfg.Network.EnableMDNS, cfg.Network.EnableDHT)
	log.Infof("  Data dir: %s", expandPath(cfg.Storage.DataDir))
	log.Info("")
	log.Info("=================================================")
	log.Info("")
}

func parseBootstrapPeers(s string) []string {
	if s == "" {
		return nil
	}
	var peers []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			peers = append(peers, p)
		}
	}
	return peers
}

func shortID(p peer.ID) string {
	s := p.String()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
