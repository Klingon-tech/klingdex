package node

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.NetworkType != NetworkMainnet {
		t.Errorf("expected NetworkMainnet, got %s", cfg.NetworkType)
	}

	if cfg.Identity.KeyFile != "node.key" {
		t.Errorf("expected node.key, got %s", cfg.Identity.KeyFile)
	}

	if len(cfg.Network.ListenAddrs) != 4 {
		t.Errorf("expected 4 listen addresses, got %d", len(cfg.Network.ListenAddrs))
	}

	if !cfg.Network.EnableMDNS {
		t.Error("expected EnableMDNS to be true")
	}

	if !cfg.Network.EnableDHT {
		t.Error("expected EnableDHT to be true")
	}

	if cfg.Network.ConnMgr.LowWater != 100 {
		t.Errorf("expected LowWater 100, got %d", cfg.Network.ConnMgr.LowWater)
	}

	if cfg.Network.ConnMgr.HighWater != 400 {
		t.Errorf("expected HighWater 400, got %d", cfg.Network.ConnMgr.HighWater)
	}

	if cfg.Network.ConnMgr.GracePeriod != time.Minute {
		t.Errorf("expected GracePeriod 1m, got %v", cfg.Network.ConnMgr.GracePeriod)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Logging.Level)
	}
}

func TestConfigDHTPrefix(t *testing.T) {
	tests := []struct {
		networkType NetworkType
		expected    string
	}{
		{NetworkMainnet, MainnetDHTPrefix},
		{NetworkTestnet, TestnetDHTPrefix},
	}

	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.NetworkType = tt.networkType

		if got := cfg.DHTPrefix(); got != tt.expected {
			t.Errorf("DHTPrefix() for %s = %s, want %s", tt.networkType, got, tt.expected)
		}
	}
}

func TestConfigDiscoveryNamespace(t *testing.T) {
	tests := []struct {
		networkType NetworkType
		expected    string
	}{
		{NetworkMainnet, MainnetDiscoveryNS},
		{NetworkTestnet, TestnetDiscoveryNS},
	}

	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.NetworkType = tt.networkType

		if got := cfg.DiscoveryNamespace(); got != tt.expected {
			t.Errorf("DiscoveryNamespace() for %s = %s, want %s", tt.networkType, got, tt.expected)
		}
	}
}

func TestConfigIsTestnet(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.IsTestnet() {
		t.Error("expected IsTestnet() to be false for mainnet")
	}

	cfg.NetworkType = NetworkTestnet
	if !cfg.IsTestnet() {
		t.Error("expected IsTestnet() to be true for testnet")
	}
}

func TestLoadConfigCreatesDefault(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "klingon-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Load config (should create default)
	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify config was created
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify default values
	if cfg.NetworkType != NetworkMainnet {
		t.Errorf("expected NetworkMainnet, got %s", cfg.NetworkType)
	}

	if cfg.Storage.DataDir != tmpDir {
		t.Errorf("expected DataDir %s, got %s", tmpDir, cfg.Storage.DataDir)
	}
}

func TestLoadConfigReadsExisting(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "klingon-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create custom config
	customConfig := `network_type: testnet
identity:
  key_file: custom.key
network:
  listen_addrs:
    - /ip4/0.0.0.0/tcp/5001
  enable_mdns: false
  enable_dht: true
logging:
  level: debug
`
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(customConfig), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config
	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify custom values
	if cfg.NetworkType != NetworkTestnet {
		t.Errorf("expected NetworkTestnet, got %s", cfg.NetworkType)
	}

	if cfg.Identity.KeyFile != "custom.key" {
		t.Errorf("expected custom.key, got %s", cfg.Identity.KeyFile)
	}

	if len(cfg.Network.ListenAddrs) != 1 || cfg.Network.ListenAddrs[0] != "/ip4/0.0.0.0/tcp/5001" {
		t.Errorf("unexpected listen addrs: %v", cfg.Network.ListenAddrs)
	}

	if cfg.Network.EnableMDNS {
		t.Error("expected EnableMDNS to be false")
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected debug log level, got %s", cfg.Logging.Level)
	}
}

func TestConfigSave(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "klingon-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and save config
	cfg := DefaultConfig()
	cfg.NetworkType = NetworkTestnet
	cfg.Logging.Level = "debug"

	configPath := filepath.Join(tmpDir, "test-config.yaml")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Read and verify content contains header
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	content := string(data)
	if !contains(content, "# Klingon P2P Node Configuration") {
		t.Error("config file missing header comment")
	}

	if !contains(content, "network_type: testnet") {
		t.Error("config file missing network_type")
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.klingon", filepath.Join(home, ".klingon")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		got := expandPath(tt.input)
		if got != tt.expected {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConfigPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		dataDir  string
		expected string
	}{
		{"~/.klingon", filepath.Join(home, ".klingon", ConfigFileName)},
		{"/tmp/test", filepath.Join("/tmp/test", ConfigFileName)},
	}

	for _, tt := range tests {
		got := ConfigPath(tt.dataDir)
		if got != tt.expected {
			t.Errorf("ConfigPath(%q) = %q, want %q", tt.dataDir, got, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
