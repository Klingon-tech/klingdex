// Package storage provides persistent storage using SQLite.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage provides persistent storage for the Klingon node.
type Storage struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// Config holds storage configuration.
type Config struct {
	DataDir string
}

// New creates a new Storage instance.
func New(cfg *Config) (*Storage, error) {
	dataDir := expandPath(cfg.DataDir)

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "klingon.db")

	// Open database
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	s := &Storage{
		db:     db,
		dbPath: dbPath,
	}

	// Initialize schema
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection.
func (s *Storage) DB() *sql.DB {
	return s.db
}

// initSchema creates all database tables.
func (s *Storage) initSchema() error {
	schema := `
	-- Known peers table
	CREATE TABLE IF NOT EXISTS peers (
		peer_id TEXT PRIMARY KEY,
		addresses TEXT,
		first_seen INTEGER,
		last_seen INTEGER,
		last_connected INTEGER,
		connection_count INTEGER DEFAULT 0,
		is_bootstrap INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_peers_last_seen ON peers(last_seen);

	-- Settings/config table
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at INTEGER
	);

	-- =========================================================================
	-- Orders, Trades, and Swaps (added for atomic swap support)
	-- =========================================================================

	-- Orders table (method-agnostic)
	-- An order represents an intent to trade, not yet matched
	-- Price is implicit: offer_amount/request_amount ratio
	CREATE TABLE IF NOT EXISTS orders (
		id TEXT PRIMARY KEY,
		peer_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'open',

		-- What is being offered
		offer_chain TEXT NOT NULL,
		offer_amount INTEGER NOT NULL,

		-- What is being requested in return
		request_chain TEXT NOT NULL,
		request_amount INTEGER NOT NULL,

		-- Preferred swap methods (JSON array)
		preferred_methods TEXT NOT NULL,

		-- Timing
		created_at INTEGER NOT NULL,
		expires_at INTEGER,
		updated_at INTEGER,

		-- Whether this is our order or a remote order
		is_local INTEGER NOT NULL DEFAULT 1,

		-- Signature proving ownership (for verification)
		signature TEXT,

		FOREIGN KEY (peer_id) REFERENCES peers(peer_id)
	);

	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	CREATE INDEX IF NOT EXISTS idx_orders_peer ON orders(peer_id);
	CREATE INDEX IF NOT EXISTS idx_orders_pair ON orders(offer_chain, request_chain);
	CREATE INDEX IF NOT EXISTS idx_orders_expires ON orders(expires_at);

	-- Trades table (links order to actual swap execution)
	-- A trade is created when an order is taken
	CREATE TABLE IF NOT EXISTS trades (
		id TEXT PRIMARY KEY,
		order_id TEXT NOT NULL,

		-- Participants
		maker_peer_id TEXT NOT NULL,
		taker_peer_id TEXT NOT NULL,

		-- Public keys for MuSig2 (hex-encoded compressed pubkeys)
		maker_pubkey TEXT,
		taker_pubkey TEXT,

		-- Our role in this trade
		our_role TEXT NOT NULL,

		-- Swap method being used
		method TEXT NOT NULL,

		-- Trade state
		state TEXT NOT NULL DEFAULT 'init',

		-- Actual amounts (may differ from order for partial fills)
		offer_chain TEXT NOT NULL,
		offer_amount INTEGER NOT NULL,
		request_chain TEXT NOT NULL,
		request_amount INTEGER NOT NULL,

		-- Timing
		created_at INTEGER NOT NULL,
		updated_at INTEGER,
		completed_at INTEGER,

		-- Failure tracking
		failure_reason TEXT,

		FOREIGN KEY (order_id) REFERENCES orders(id)
	);

	CREATE INDEX IF NOT EXISTS idx_trades_order ON trades(order_id);
	CREATE INDEX IF NOT EXISTS idx_trades_state ON trades(state);
	CREATE INDEX IF NOT EXISTS idx_trades_maker ON trades(maker_peer_id);
	CREATE INDEX IF NOT EXISTS idx_trades_taker ON trades(taker_peer_id);

	-- Swap legs table (each side of the swap tracked separately)
	-- A trade has two legs: offer chain and request chain
	CREATE TABLE IF NOT EXISTS swap_legs (
		id TEXT PRIMARY KEY,
		trade_id TEXT NOT NULL,

		-- Leg identification
		leg_type TEXT NOT NULL,
		chain TEXT NOT NULL,
		amount INTEGER NOT NULL,

		-- Our role on this leg
		our_role TEXT NOT NULL,

		-- Leg state (can differ between legs)
		state TEXT NOT NULL DEFAULT 'init',

		-- Funding transaction
		funding_txid TEXT,
		funding_vout INTEGER,
		funding_confirms INTEGER DEFAULT 0,
		funding_address TEXT,

		-- Redeem/Refund transactions
		redeem_txid TEXT,
		refund_txid TEXT,

		-- Timeout
		timeout_height INTEGER,
		timeout_timestamp INTEGER,

		-- Method-specific data (JSON blob)
		-- Structure depends on method: musig2, htlc_bitcoin, htlc_evm, etc.
		method_data TEXT,

		-- Timing
		created_at INTEGER NOT NULL,
		updated_at INTEGER,

		FOREIGN KEY (trade_id) REFERENCES trades(id)
	);

	CREATE INDEX IF NOT EXISTS idx_swap_legs_trade ON swap_legs(trade_id);
	CREATE INDEX IF NOT EXISTS idx_swap_legs_state ON swap_legs(state);
	CREATE INDEX IF NOT EXISTS idx_swap_legs_chain ON swap_legs(chain);

	-- Secrets table (separate for security - HTLC secrets)
	CREATE TABLE IF NOT EXISTS secrets (
		id TEXT PRIMARY KEY,
		trade_id TEXT NOT NULL,

		-- The secret hash (always known)
		secret_hash TEXT NOT NULL,

		-- The secret itself (only after reveal)
		secret TEXT,

		-- Who created this secret
		created_by TEXT NOT NULL,

		-- Remote wallet addresses (received with secret hash from counterparty)
		remote_offer_wallet_addr TEXT,
		remote_request_wallet_addr TEXT,

		-- Timing
		created_at INTEGER NOT NULL,
		revealed_at INTEGER,

		FOREIGN KEY (trade_id) REFERENCES trades(id),
		UNIQUE(trade_id, secret_hash)
	);

	CREATE INDEX IF NOT EXISTS idx_secrets_trade ON secrets(trade_id);
	CREATE INDEX IF NOT EXISTS idx_secrets_hash ON secrets(secret_hash);

	-- Message log (for debugging and audit)
	CREATE TABLE IF NOT EXISTS message_log (
		id TEXT PRIMARY KEY,
		message_type TEXT NOT NULL,
		from_peer_id TEXT NOT NULL,
		to_peer_id TEXT,
		trade_id TEXT,
		payload TEXT,
		received_at INTEGER NOT NULL,
		processed INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_messages_type ON message_log(message_type);
	CREATE INDEX IF NOT EXISTS idx_messages_trade ON message_log(trade_id);
	CREATE INDEX IF NOT EXISTS idx_messages_received ON message_log(received_at);

	-- =========================================================================
	-- Active Swaps (runtime swap state for persistence/recovery)
	-- =========================================================================

	-- Active swaps table for persisting swap state
	-- This enables recovery after node restart
	CREATE TABLE IF NOT EXISTS active_swaps (
		trade_id TEXT PRIMARY KEY,
		order_id TEXT NOT NULL,

		-- Participants
		maker_peer_id TEXT NOT NULL,
		taker_peer_id TEXT NOT NULL,

		-- Our role
		our_role TEXT NOT NULL,
		is_maker INTEGER NOT NULL DEFAULT 0,

		-- Swap details
		offer_chain TEXT NOT NULL,
		offer_amount INTEGER NOT NULL,
		request_chain TEXT NOT NULL,
		request_amount INTEGER NOT NULL,

		-- State (init, funding, funded, signing, redeemed, refunded, failed, cancelled)
		state TEXT NOT NULL DEFAULT 'init',

		-- MuSig2 data (JSON blob with keys, nonces, partial sigs, etc.)
		-- SECURITY: This includes encrypted private key data
		method_data TEXT,

		-- Funding transaction info
		local_funding_txid TEXT,
		local_funding_vout INTEGER DEFAULT 0,
		remote_funding_txid TEXT,
		remote_funding_vout INTEGER DEFAULT 0,

		-- Timeout tracking for refunds (separate for each chain)
		timeout_height INTEGER DEFAULT 0,
		request_timeout_height INTEGER DEFAULT 0,
		timeout_timestamp INTEGER DEFAULT 0,

		-- Result
		redeem_txid TEXT,
		refund_txid TEXT,
		failure_reason TEXT,

		-- Timing
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		completed_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_active_swaps_state ON active_swaps(state);
	CREATE INDEX IF NOT EXISTS idx_active_swaps_timeout ON active_swaps(timeout_height);
	CREATE INDEX IF NOT EXISTS idx_active_swaps_updated ON active_swaps(updated_at);

	-- =========================================================================
	-- Wallet UTXO Tracking (for multi-address spending)
	-- =========================================================================

	-- Wallet addresses table (tracks all derived addresses)
	CREATE TABLE IF NOT EXISTS wallet_addresses (
		address TEXT PRIMARY KEY,
		chain TEXT NOT NULL,

		-- Derivation path components (BIP44: m/purpose'/coin'/account'/change/index)
		account INTEGER NOT NULL DEFAULT 0,
		change INTEGER NOT NULL DEFAULT 0,
		address_index INTEGER NOT NULL,

		-- Address type (p2wpkh, p2tr, p2pkh)
		address_type TEXT NOT NULL DEFAULT 'p2wpkh',

		-- Usage tracking
		tx_count INTEGER DEFAULT 0,
		total_received INTEGER DEFAULT 0,
		total_sent INTEGER DEFAULT 0,

		-- Timestamps
		created_at INTEGER NOT NULL,
		first_seen_at INTEGER,
		last_seen_at INTEGER,

		UNIQUE(chain, account, change, address_index)
	);

	CREATE INDEX IF NOT EXISTS idx_wallet_addresses_chain ON wallet_addresses(chain);
	CREATE INDEX IF NOT EXISTS idx_wallet_addresses_path ON wallet_addresses(account, change, address_index);

	-- UTXOs table (all unspent outputs across all addresses)
	CREATE TABLE IF NOT EXISTS wallet_utxos (
		txid TEXT NOT NULL,
		vout INTEGER NOT NULL,

		-- Amount in smallest units (satoshis, litoshis, etc.)
		amount INTEGER NOT NULL,

		-- Which address owns this UTXO
		address TEXT NOT NULL,
		chain TEXT NOT NULL,

		-- Derivation path (for key derivation during signing)
		account INTEGER NOT NULL DEFAULT 0,
		change INTEGER NOT NULL DEFAULT 0,
		address_index INTEGER NOT NULL,

		-- Script info
		script_pubkey TEXT,
		address_type TEXT NOT NULL DEFAULT 'p2wpkh',

		-- Status: 'unconfirmed', 'confirmed', 'pending_spend', 'spent'
		status TEXT NOT NULL DEFAULT 'unconfirmed',

		-- Confirmation tracking
		block_height INTEGER,
		block_hash TEXT,
		confirmations INTEGER DEFAULT 0,

		-- Spending info (if spent)
		spent_txid TEXT,
		spent_at INTEGER,

		-- Timestamps
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,

		PRIMARY KEY (txid, vout),
		FOREIGN KEY (address) REFERENCES wallet_addresses(address)
	);

	CREATE INDEX IF NOT EXISTS idx_wallet_utxos_address ON wallet_utxos(address);
	CREATE INDEX IF NOT EXISTS idx_wallet_utxos_chain ON wallet_utxos(chain);
	CREATE INDEX IF NOT EXISTS idx_wallet_utxos_status ON wallet_utxos(status);
	CREATE INDEX IF NOT EXISTS idx_wallet_utxos_chain_status ON wallet_utxos(chain, status);

	-- Wallet sync state (tracks sync progress per chain)
	CREATE TABLE IF NOT EXISTS wallet_sync_state (
		chain TEXT PRIMARY KEY,

		-- Last scanned indices (gap limit tracking)
		last_external_index INTEGER DEFAULT 0,
		last_change_index INTEGER DEFAULT 0,

		-- Gap limit used
		gap_limit INTEGER DEFAULT 20,

		-- Sync status
		last_sync_at INTEGER,
		last_block_height INTEGER,
		sync_status TEXT DEFAULT 'pending'
	);

	-- =========================================================================
	-- P2P Message Queue (for reliable direct messaging)
	-- =========================================================================

	-- Outbound message queue (pending delivery with retry)
	CREATE TABLE IF NOT EXISTS message_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT UNIQUE NOT NULL,      -- UUID for deduplication
		trade_id TEXT NOT NULL,               -- Associated swap trade
		peer_id TEXT NOT NULL,                -- Target peer
		message_type TEXT NOT NULL,           -- pubkey_exchange, nonce_exchange, etc.
		payload BLOB NOT NULL,                -- Full message JSON
		sequence_num INTEGER NOT NULL,        -- Per-trade sequence number

		-- Swap timeout (for retry decision)
		swap_timeout INTEGER NOT NULL,        -- Unix timestamp when swap expires

		-- Retry tracking
		created_at INTEGER NOT NULL,          -- When message was queued
		retry_count INTEGER DEFAULT 0,        -- Number of send attempts
		last_attempt_at INTEGER,              -- Last send attempt timestamp
		next_retry_at INTEGER NOT NULL,       -- When to retry next

		-- Delivery status
		acked_at INTEGER,                     -- When ACK received (NULL until ACKed)
		status TEXT DEFAULT 'pending',        -- pending, sent, acked, failed, expired
		error_message TEXT                    -- Error if failed
	);

	CREATE INDEX IF NOT EXISTS idx_outbox_pending ON message_outbox(status, next_retry_at)
		WHERE status = 'pending' OR status = 'sent';
	CREATE INDEX IF NOT EXISTS idx_outbox_trade ON message_outbox(trade_id);
	CREATE INDEX IF NOT EXISTS idx_outbox_peer ON message_outbox(peer_id, status);
	CREATE INDEX IF NOT EXISTS idx_outbox_message ON message_outbox(message_id);

	-- Inbound message log (for deduplication/idempotency)
	CREATE TABLE IF NOT EXISTS message_inbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT UNIQUE NOT NULL,      -- UUID from sender (for dedup)
		trade_id TEXT NOT NULL,               -- Associated swap trade
		peer_id TEXT NOT NULL,                -- Sender peer ID
		message_type TEXT NOT NULL,           -- Message type
		sequence_num INTEGER NOT NULL,        -- Sequence number from sender

		-- Processing status
		received_at INTEGER NOT NULL,         -- When received
		processed_at INTEGER,                 -- When handler completed (NULL until done)
		ack_sent INTEGER DEFAULT 0            -- Whether ACK was sent
	);

	CREATE INDEX IF NOT EXISTS idx_inbox_message ON message_inbox(message_id);
	CREATE INDEX IF NOT EXISTS idx_inbox_trade ON message_inbox(trade_id, sequence_num);
	CREATE INDEX IF NOT EXISTS idx_inbox_peer ON message_inbox(peer_id);

	-- Sequence number tracking per trade (for ordering)
	CREATE TABLE IF NOT EXISTS message_sequences (
		trade_id TEXT PRIMARY KEY,
		local_seq INTEGER DEFAULT 0,          -- Our next outbound sequence number
		remote_seq INTEGER DEFAULT 0,         -- Last received inbound sequence number
		updated_at INTEGER NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Run migrations for existing databases
	return s.runMigrations()
}

// runMigrations runs schema migrations for existing databases.
// These are ALTER TABLE statements that add columns to existing tables.
// Errors are ignored since columns may already exist.
func (s *Storage) runMigrations() error {
	// Migration: Add remote wallet address columns to secrets table
	migrations := []string{
		"ALTER TABLE secrets ADD COLUMN remote_offer_wallet_addr TEXT",
		"ALTER TABLE secrets ADD COLUMN remote_request_wallet_addr TEXT",
	}

	for _, migration := range migrations {
		// Ignore errors - column may already exist
		_, _ = s.db.Exec(migration)
	}

	return nil
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
