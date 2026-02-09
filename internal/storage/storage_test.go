package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Verify database file was created
	dbPath := filepath.Join(tmpDir, "klingon.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	// Verify DB is accessible
	if store.DB() == nil {
		t.Error("DB() returned nil")
	}
}

func TestNewWithTildeExpansion(t *testing.T) {
	// This test verifies tilde expansion works
	// We can't actually test ~ without potentially creating files in user's home
	// So we just verify the expandPath function works
	home, _ := os.UserHomeDir()
	expanded := expandPath("~/.test")
	expected := filepath.Join(home, ".test")

	if expanded != expected {
		t.Errorf("expandPath(~/.test) = %s, want %s", expanded, expected)
	}
}

func TestStorageSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Verify peers table exists
	var tableName string
	err = store.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='peers'").Scan(&tableName)
	if err != nil {
		t.Errorf("peers table not found: %v", err)
	}

	// Verify settings table exists
	err = store.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='settings'").Scan(&tableName)
	if err != nil {
		t.Errorf("settings table not found: %v", err)
	}
}

func TestPeerCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Create peer
	now := time.Now()
	peer := &PeerRecord{
		PeerID:          "12D3KooWTestPeer1234567890",
		Addresses:       []string{"/ip4/127.0.0.1/tcp/4001", "/ip4/192.168.1.1/tcp/4001"},
		FirstSeen:       now,
		LastSeen:        now,
		LastConnected:   now,
		ConnectionCount: 1,
		IsBootstrap:     false,
	}

	// Save peer
	if err := store.SavePeer(peer); err != nil {
		t.Fatalf("SavePeer() error = %v", err)
	}

	// Get peer
	got, err := store.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetPeer() returned nil")
	}

	if got.PeerID != peer.PeerID {
		t.Errorf("PeerID = %s, want %s", got.PeerID, peer.PeerID)
	}

	if len(got.Addresses) != 2 {
		t.Errorf("len(Addresses) = %d, want 2", len(got.Addresses))
	}

	// Update peer (save again should increment connection count)
	peer.LastSeen = time.Now()
	if err := store.SavePeer(peer); err != nil {
		t.Fatalf("SavePeer() update error = %v", err)
	}

	got, err = store.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer() after update error = %v", err)
	}

	if got.ConnectionCount != 2 {
		t.Errorf("ConnectionCount = %d, want 2", got.ConnectionCount)
	}

	// Delete peer
	if err := store.DeletePeer(peer.PeerID); err != nil {
		t.Fatalf("DeletePeer() error = %v", err)
	}

	got, err = store.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer() after delete error = %v", err)
	}
	if got != nil {
		t.Error("peer should be nil after delete")
	}
}

func TestListPeers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Add multiple peers
	now := time.Now()
	for i := 0; i < 5; i++ {
		peer := &PeerRecord{
			PeerID:    "12D3KooWTestPeer" + string(rune('A'+i)),
			Addresses: []string{"/ip4/127.0.0.1/tcp/4001"},
			FirstSeen: now.Add(time.Duration(i) * time.Minute),
			LastSeen:  now.Add(time.Duration(i) * time.Minute),
		}
		if err := store.SavePeer(peer); err != nil {
			t.Fatalf("SavePeer() error = %v", err)
		}
	}

	// List all peers
	peers, err := store.ListPeers(0)
	if err != nil {
		t.Fatalf("ListPeers() error = %v", err)
	}
	if len(peers) != 5 {
		t.Errorf("ListPeers(0) returned %d peers, want 5", len(peers))
	}

	// List with limit
	peers, err = store.ListPeers(3)
	if err != nil {
		t.Fatalf("ListPeers(3) error = %v", err)
	}
	if len(peers) != 3 {
		t.Errorf("ListPeers(3) returned %d peers, want 3", len(peers))
	}
}

func TestListRecentPeers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now()

	// Add recent peer
	recentPeer := &PeerRecord{
		PeerID:    "12D3KooWRecentPeer",
		Addresses: []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen: now,
		LastSeen:  now,
	}
	if err := store.SavePeer(recentPeer); err != nil {
		t.Fatalf("SavePeer() error = %v", err)
	}

	// Add old peer (manually set last_seen to old time)
	oldPeer := &PeerRecord{
		PeerID:    "12D3KooWOldPeer",
		Addresses: []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen: now.Add(-30 * 24 * time.Hour), // 30 days ago
		LastSeen:  now.Add(-30 * 24 * time.Hour),
	}
	if err := store.SavePeer(oldPeer); err != nil {
		t.Fatalf("SavePeer() error = %v", err)
	}

	// List peers seen in last 7 days
	peers, err := store.ListRecentPeers(7*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("ListRecentPeers() error = %v", err)
	}

	// Should only get the recent peer
	if len(peers) != 1 {
		t.Errorf("ListRecentPeers() returned %d peers, want 1", len(peers))
	}

	if len(peers) > 0 && peers[0].PeerID != "12D3KooWRecentPeer" {
		t.Errorf("expected recent peer, got %s", peers[0].PeerID)
	}
}

func TestUpdatePeerConnected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Add peer
	peer := &PeerRecord{
		PeerID:          "12D3KooWTestPeer",
		Addresses:       []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen:       time.Now(),
		LastSeen:        time.Now(),
		ConnectionCount: 5,
	}
	if err := store.SavePeer(peer); err != nil {
		t.Fatalf("SavePeer() error = %v", err)
	}

	// Update connected
	if err := store.UpdatePeerConnected(peer.PeerID); err != nil {
		t.Fatalf("UpdatePeerConnected() error = %v", err)
	}

	// Verify connection count increased
	got, err := store.GetPeer(peer.PeerID)
	if err != nil {
		t.Fatalf("GetPeer() error = %v", err)
	}

	// SavePeer inserted connection_count=5, UpdatePeerConnected adds 1 more = 6
	if got.ConnectionCount != 6 {
		t.Errorf("ConnectionCount = %d, want 6", got.ConnectionCount)
	}
}

func TestPeerCount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "klingon-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Initially empty
	count, err := store.PeerCount()
	if err != nil {
		t.Fatalf("PeerCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("PeerCount() = %d, want 0", count)
	}

	// Add peers
	for i := 0; i < 3; i++ {
		peer := &PeerRecord{
			PeerID:    "12D3KooWTestPeer" + string(rune('A'+i)),
			Addresses: []string{"/ip4/127.0.0.1/tcp/4001"},
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		if err := store.SavePeer(peer); err != nil {
			t.Fatalf("SavePeer() error = %v", err)
		}
	}

	count, err = store.PeerCount()
	if err != nil {
		t.Fatalf("PeerCount() error = %v", err)
	}
	if count != 3 {
		t.Errorf("PeerCount() = %d, want 3", count)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should return 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should return 0")
	}
}

func TestTimeToUnixOrZero(t *testing.T) {
	if timeToUnixOrZero(time.Time{}) != 0 {
		t.Error("timeToUnixOrZero(zero time) should return 0")
	}

	now := time.Now()
	if timeToUnixOrZero(now) != now.Unix() {
		t.Error("timeToUnixOrZero should return Unix timestamp")
	}
}
