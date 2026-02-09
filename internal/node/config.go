// Package node provides the libp2p node implementation for the Klingon P2P network.
package node

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"gopkg.in/yaml.v3"
)

// NetworkType represents the network (mainnet or testnet).
type NetworkType string

const (
	NetworkMainnet NetworkType = "mainnet"
	NetworkTestnet NetworkType = "testnet"
)

// Network-specific constants for peer separation.
const (
	// Mainnet
	MainnetDHTPrefix         = "/klingon"
	MainnetDiscoveryNS       = "klingon-mainnet"

	// Testnet
	TestnetDHTPrefix         = "/klingon-testnet"
	TestnetDiscoveryNS       = "klingon-testnet"
)

// Config holds all configuration for the P2P node.
type Config struct {
	// NetworkType is the network type (mainnet or testnet).
	NetworkType NetworkType `yaml:"network_type"`

	// Identity
	Identity IdentityConfig `yaml:"identity"`

	// Network settings
	Network NetworkConfig `yaml:"network"`

	// Storage
	Storage StorageConfig `yaml:"storage"`

	// Logging
	Logging LoggingConfig `yaml:"logging"`

	// Backends holds blockchain API configurations per chain symbol.
	// If not specified, defaults to public APIs (mempool.space, etc.)
	Backends map[string]*backend.Config `yaml:"backends,omitempty"`
}

// DHTPrefix returns the DHT protocol prefix for the configured network.
func (c *Config) DHTPrefix() string {
	if c.NetworkType == NetworkTestnet {
		return TestnetDHTPrefix
	}
	return MainnetDHTPrefix
}

// DiscoveryNamespace returns the discovery namespace for the configured network.
func (c *Config) DiscoveryNamespace() string {
	if c.NetworkType == NetworkTestnet {
		return TestnetDiscoveryNS
	}
	return MainnetDiscoveryNS
}

// IsTestnet returns true if running on testnet.
func (c *Config) IsTestnet() bool {
	return c.NetworkType == NetworkTestnet
}

// GetBackendConfig returns the backend config for a chain symbol.
// Returns default config if not explicitly configured.
func (c *Config) GetBackendConfig(symbol string) *backend.Config {
	if c.Backends != nil {
		if cfg, ok := c.Backends[symbol]; ok {
			return cfg
		}
	}
	// Return default config
	defaults := backend.DefaultConfigs()
	if cfg, ok := defaults[symbol]; ok {
		return cfg
	}
	return nil
}

// GetBackendURL returns the appropriate backend URL for the chain and network.
func (c *Config) GetBackendURL(symbol string) string {
	cfg := c.GetBackendConfig(symbol)
	if cfg == nil {
		return ""
	}
	if c.IsTestnet() {
		return cfg.TestnetURL
	}
	return cfg.MainnetURL
}

// IdentityConfig holds identity-related settings.
type IdentityConfig struct {
	// KeyFile is the path to the node's private key file.
	KeyFile string `yaml:"key_file"`
}

// NetworkConfig holds P2P network settings.
type NetworkConfig struct {
	// ListenAddrs are the multiaddrs to listen on.
	ListenAddrs []string `yaml:"listen_addrs"`

	// BootstrapPeers are the initial peers to connect to.
	BootstrapPeers []string `yaml:"bootstrap_peers"`

	// EnableMDNS enables local peer discovery via mDNS.
	EnableMDNS bool `yaml:"enable_mdns"`

	// EnableDHT enables the Kademlia DHT for peer discovery.
	EnableDHT bool `yaml:"enable_dht"`

	// EnableRelay enables circuit relay for NAT traversal.
	EnableRelay bool `yaml:"enable_relay"`

	// EnableNAT enables NAT port mapping (UPnP/NAT-PMP).
	EnableNAT bool `yaml:"enable_nat"`

	// EnableHolePunching enables direct connection establishment through NAT.
	EnableHolePunching bool `yaml:"enable_hole_punching"`

	// ConnectionManager settings
	ConnMgr ConnMgrConfig `yaml:"conn_mgr"`
}

// ConnMgrConfig holds connection manager settings.
type ConnMgrConfig struct {
	// LowWater is the minimum number of connections to maintain.
	LowWater int `yaml:"low_water"`

	// HighWater is the maximum number of connections before pruning.
	HighWater int `yaml:"high_water"`

	// GracePeriod is how long to wait before closing new connections.
	GracePeriod time.Duration `yaml:"grace_period"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	// DataDir is the directory for all data files.
	DataDir string `yaml:"data_dir"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	// Level is the log level (debug, info, warn, error).
	Level string `yaml:"level"`

	// File is the log file path (empty for stdout).
	File string `yaml:"file"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		NetworkType: NetworkMainnet,
		Identity: IdentityConfig{
			KeyFile: "node.key",
		},
		Network: NetworkConfig{
			ListenAddrs: []string{
				"/ip4/0.0.0.0/tcp/4001",
				"/ip4/0.0.0.0/udp/4001/quic-v1",
				"/ip6/::/tcp/4001",
				"/ip6/::/udp/4001/quic-v1",
			},
			BootstrapPeers:     []string{},
			EnableMDNS:         true,
			EnableDHT:          true,
			EnableRelay:        true,
			EnableNAT:          true,
			EnableHolePunching: true,
			ConnMgr: ConnMgrConfig{
				LowWater:    100,
				HighWater:   400,
				GracePeriod: time.Minute,
			},
		},
		Storage: StorageConfig{
			DataDir: "~/.klingon",
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  "",
		},
	}
}

// ConfigFileName is the default config file name.
const ConfigFileName = "config.yaml"

// LoadConfig loads configuration from a YAML file.
// If the file doesn't exist, it creates one with default values.
func LoadConfig(dataDir string) (*Config, error) {
	expandedDir := expandPath(dataDir)
	configPath := filepath.Join(expandedDir, ConfigFileName)

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		cfg.Storage.DataDir = dataDir

		// Save default config
		if err := cfg.Save(configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}

		return cfg, nil
	}

	// Load existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to a YAML file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := []byte("# Klingon P2P Node Configuration\n# Generated automatically on first run\n\n")
	data = append(header, data...)

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ConfigPath returns the full path to the config file for the given data directory.
func ConfigPath(dataDir string) string {
	return filepath.Join(expandPath(dataDir), ConfigFileName)
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
