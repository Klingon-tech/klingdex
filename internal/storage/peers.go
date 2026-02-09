package storage

import (
	"database/sql"
	"encoding/json"
	"time"
)

// PeerRecord represents a known peer in the database.
type PeerRecord struct {
	PeerID          string    `json:"peer_id"`
	Addresses       []string  `json:"addresses"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	LastConnected   time.Time `json:"last_connected"`
	ConnectionCount int       `json:"connection_count"`
	IsBootstrap     bool      `json:"is_bootstrap"`
}

// SavePeer saves or updates a peer record.
func (s *Storage) SavePeer(peer *PeerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addrsJSON, err := json.Marshal(peer.Addresses)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO peers (peer_id, addresses, first_seen, last_seen, last_connected, connection_count, is_bootstrap)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			addresses = excluded.addresses,
			last_seen = excluded.last_seen,
			last_connected = CASE WHEN excluded.last_connected > 0 THEN excluded.last_connected ELSE peers.last_connected END,
			connection_count = peers.connection_count + 1,
			is_bootstrap = CASE WHEN excluded.is_bootstrap THEN 1 ELSE peers.is_bootstrap END
	`

	_, err = s.db.Exec(query,
		peer.PeerID,
		string(addrsJSON),
		peer.FirstSeen.Unix(),
		peer.LastSeen.Unix(),
		timeToUnixOrZero(peer.LastConnected),
		peer.ConnectionCount,
		boolToInt(peer.IsBootstrap),
	)
	return err
}

// GetPeer retrieves a peer record by ID.
func (s *Storage) GetPeer(peerID string) (*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT peer_id, addresses, first_seen, last_seen, last_connected, connection_count, is_bootstrap
		FROM peers WHERE peer_id = ?
	`

	row := s.db.QueryRow(query, peerID)
	return scanPeerRecord(row)
}

// ListPeers returns peers ordered by last seen (most recent first).
func (s *Storage) ListPeers(limit int) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT peer_id, addresses, first_seen, last_seen, last_connected, connection_count, is_bootstrap
		FROM peers
		ORDER BY last_seen DESC
	`

	if limit > 0 {
		query += " LIMIT ?"
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(query, limit)
	} else {
		rows, err = s.db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*PeerRecord
	for rows.Next() {
		peer, err := scanPeerRecordRows(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, rows.Err()
}

// ListRecentPeers returns peers seen within the given duration.
func (s *Storage) ListRecentPeers(since time.Duration, limit int) ([]*PeerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since).Unix()

	query := `
		SELECT peer_id, addresses, first_seen, last_seen, last_connected, connection_count, is_bootstrap
		FROM peers
		WHERE last_seen > ?
		ORDER BY connection_count DESC, last_seen DESC
	`

	if limit > 0 {
		query += " LIMIT ?"
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(query, cutoff, limit)
	} else {
		rows, err = s.db.Query(query, cutoff)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*PeerRecord
	for rows.Next() {
		peer, err := scanPeerRecordRows(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, rows.Err()
}

// UpdatePeerConnected updates the last_connected time and increments connection count.
func (s *Storage) UpdatePeerConnected(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		"UPDATE peers SET last_connected = ?, last_seen = ?, connection_count = connection_count + 1 WHERE peer_id = ?",
		now, now, peerID,
	)
	return err
}

// UpdatePeerSeen updates the last_seen time.
func (s *Storage) UpdatePeerSeen(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		"UPDATE peers SET last_seen = ? WHERE peer_id = ?",
		time.Now().Unix(), peerID,
	)
	return err
}

// DeletePeer removes a peer from the database.
func (s *Storage) DeletePeer(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM peers WHERE peer_id = ?", peerID)
	return err
}

// PeerCount returns the total number of known peers.
func (s *Storage) PeerCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM peers").Scan(&count)
	return count, err
}

// Helper functions

func scanPeerRecord(row *sql.Row) (*PeerRecord, error) {
	var peer PeerRecord
	var addrsJSON string
	var firstSeen, lastSeen, lastConnected int64
	var isBootstrap int

	err := row.Scan(
		&peer.PeerID,
		&addrsJSON,
		&firstSeen,
		&lastSeen,
		&lastConnected,
		&peer.ConnectionCount,
		&isBootstrap,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if addrsJSON != "" {
		json.Unmarshal([]byte(addrsJSON), &peer.Addresses)
	}

	peer.FirstSeen = time.Unix(firstSeen, 0)
	peer.LastSeen = time.Unix(lastSeen, 0)
	if lastConnected > 0 {
		peer.LastConnected = time.Unix(lastConnected, 0)
	}
	peer.IsBootstrap = isBootstrap == 1

	return &peer, nil
}

func scanPeerRecordRows(rows *sql.Rows) (*PeerRecord, error) {
	var peer PeerRecord
	var addrsJSON string
	var firstSeen, lastSeen, lastConnected int64
	var isBootstrap int

	err := rows.Scan(
		&peer.PeerID,
		&addrsJSON,
		&firstSeen,
		&lastSeen,
		&lastConnected,
		&peer.ConnectionCount,
		&isBootstrap,
	)
	if err != nil {
		return nil, err
	}

	if addrsJSON != "" {
		json.Unmarshal([]byte(addrsJSON), &peer.Addresses)
	}

	peer.FirstSeen = time.Unix(firstSeen, 0)
	peer.LastSeen = time.Unix(lastSeen, 0)
	if lastConnected > 0 {
		peer.LastConnected = time.Unix(lastConnected, 0)
	}
	peer.IsBootstrap = isBootstrap == 1

	return &peer, nil
}

func timeToUnixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
