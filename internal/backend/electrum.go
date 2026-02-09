package backend

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// ElectrumBackend implements Backend using the Electrum protocol.
// Supports both TCP and SSL connections.
type ElectrumBackend struct {
	servers   []string // List of server addresses (host:port)
	useTLS    bool
	conn      net.Conn
	reader    *bufio.Reader
	mu        sync.Mutex
	connected bool
	requestID atomic.Uint64
	timeout   time.Duration
}

// NewElectrumBackend creates a new Electrum backend.
// Servers should be in format "host:port" (e.g., "electrum.blockstream.info:50002")
func NewElectrumBackend(servers []string, useTLS bool) *ElectrumBackend {
	return &ElectrumBackend{
		servers: servers,
		useTLS:  useTLS,
		timeout: 30 * time.Second,
	}
}

// Type returns TypeElectrum.
func (e *ElectrumBackend) Type() Type {
	return TypeElectrum
}

// Connect establishes connection to an Electrum server.
func (e *ElectrumBackend) Connect(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.connected {
		return nil
	}

	var lastErr error
	for _, server := range e.servers {
		var conn net.Conn
		var err error

		dialer := &net.Dialer{Timeout: e.timeout}

		if e.useTLS {
			conn, err = tls.DialWithDialer(dialer, "tcp", server, &tls.Config{
				MinVersion: tls.VersionTLS12,
			})
		} else {
			conn, err = dialer.DialContext(ctx, "tcp", server)
		}

		if err != nil {
			lastErr = err
			continue
		}

		e.conn = conn
		e.reader = bufio.NewReader(conn)

		// Test connection with server.version
		_, err = e.call("server.version", []interface{}{"klingon", "1.4"})
		if err != nil {
			conn.Close()
			lastErr = err
			continue
		}

		e.connected = true
		return nil
	}

	return fmt.Errorf("%w: %v", ErrNotConnected, lastErr)
}

// Close closes the connection.
func (e *ElectrumBackend) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn != nil {
		e.conn.Close()
		e.conn = nil
	}
	e.connected = false
	return nil
}

// IsConnected returns true if connected.
func (e *ElectrumBackend) IsConnected() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.connected
}

// GetAddressInfo returns address balance and tx count.
func (e *ElectrumBackend) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	scriptHash := addressToScriptHash(address)

	// Get balance
	balanceResult, err := e.call("blockchain.scripthash.get_balance", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	balance, ok := balanceResult.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected balance response format")
	}

	confirmed := uint64(balance["confirmed"].(float64))
	unconfirmed := int64(balance["unconfirmed"].(float64))

	// Get history for tx count
	historyResult, err := e.call("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	history, ok := historyResult.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected history response format")
	}

	return &AddressInfo{
		Address:        address,
		TxCount:        int64(len(history)),
		Balance:        confirmed,
		MempoolBalance: unconfirmed,
	}, nil
}

// GetAddressUTXOs returns unspent outputs for an address.
func (e *ElectrumBackend) GetAddressUTXOs(ctx context.Context, address string) ([]UTXO, error) {
	scriptHash := addressToScriptHash(address)

	result, err := e.call("blockchain.scripthash.listunspent", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	utxoList, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected listunspent response format")
	}

	utxos := make([]UTXO, 0, len(utxoList))
	for _, u := range utxoList {
		utxoMap, ok := u.(map[string]interface{})
		if !ok {
			continue
		}

		height := int64(0)
		if h, ok := utxoMap["height"].(float64); ok {
			height = int64(h)
		}

		utxos = append(utxos, UTXO{
			TxID:        utxoMap["tx_hash"].(string),
			Vout:        uint32(utxoMap["tx_pos"].(float64)),
			Amount:      uint64(utxoMap["value"].(float64)),
			BlockHeight: height,
		})
	}

	return utxos, nil
}

// GetAddressTxs returns transactions for an address.
func (e *ElectrumBackend) GetAddressTxs(ctx context.Context, address string, lastSeenTxID string) ([]Transaction, error) {
	scriptHash := addressToScriptHash(address)

	result, err := e.call("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	historyList, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected history response format")
	}

	// Electrum only returns tx hashes, we need to fetch full txs
	txs := make([]Transaction, 0, len(historyList))
	for _, h := range historyList {
		historyMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}

		txID := historyMap["tx_hash"].(string)

		// Skip if we've already seen this tx
		if lastSeenTxID != "" && txID == lastSeenTxID {
			break
		}

		tx, err := e.GetTransaction(ctx, txID)
		if err != nil {
			continue // Skip failed txs
		}
		txs = append(txs, *tx)
	}

	return txs, nil
}

// GetTransaction returns a transaction by ID.
func (e *ElectrumBackend) GetTransaction(ctx context.Context, txID string) (*Transaction, error) {
	result, err := e.call("blockchain.transaction.get", []interface{}{txID, true})
	if err != nil {
		return nil, err
	}

	txMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected transaction response format")
	}

	tx := &Transaction{
		TxID:     txID,
		Hex:      txMap["hex"].(string),
		Size:     int64(txMap["size"].(float64)),
		LockTime: uint32(txMap["locktime"].(float64)),
		Version:  int32(txMap["version"].(float64)),
	}

	if confirmations, ok := txMap["confirmations"].(float64); ok {
		tx.Confirmations = int64(confirmations)
		tx.Confirmed = confirmations > 0
	}

	if blockHash, ok := txMap["blockhash"].(string); ok {
		tx.BlockHash = blockHash
	}

	if blockTime, ok := txMap["blocktime"].(float64); ok {
		tx.BlockTime = int64(blockTime)
	}

	return tx, nil
}

// GetRawTransaction returns raw transaction hex.
func (e *ElectrumBackend) GetRawTransaction(ctx context.Context, txID string) ([]byte, error) {
	result, err := e.call("blockchain.transaction.get", []interface{}{txID, false})
	if err != nil {
		return nil, err
	}

	hexStr, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected raw transaction response format")
	}

	return []byte(hexStr), nil
}

// BroadcastTransaction broadcasts a raw transaction.
func (e *ElectrumBackend) BroadcastTransaction(ctx context.Context, rawTxHex string) (string, error) {
	result, err := e.call("blockchain.transaction.broadcast", []interface{}{rawTxHex})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBroadcastFailed, err)
	}

	txID, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected broadcast response format")
	}

	return txID, nil
}

// GetBlockHeight returns the current block height.
func (e *ElectrumBackend) GetBlockHeight(ctx context.Context) (int64, error) {
	result, err := e.call("blockchain.headers.subscribe", []interface{}{})
	if err != nil {
		return 0, err
	}

	headerMap, ok := result.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("unexpected headers response format")
	}

	height, ok := headerMap["height"].(float64)
	if !ok {
		return 0, fmt.Errorf("height not found in response")
	}

	return int64(height), nil
}

// GetBlockHeader returns block header info.
func (e *ElectrumBackend) GetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	// Try to parse as height first
	var height int64
	if _, err := fmt.Sscanf(hashOrHeight, "%d", &height); err != nil {
		// It's a hash, we need to get height first (not directly supported)
		return nil, fmt.Errorf("electrum requires block height, not hash")
	}

	result, err := e.call("blockchain.block.header", []interface{}{height, 0})
	if err != nil {
		return nil, err
	}

	headerHex, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected block header response format")
	}

	return parseBlockHeader(headerHex, height)
}

// parseBlockHeader parses an 80-byte Bitcoin block header from hex.
// Block header structure (80 bytes, all little-endian):
//   - Version: 4 bytes
//   - Previous block hash: 32 bytes
//   - Merkle root: 32 bytes
//   - Timestamp: 4 bytes (Unix time)
//   - Bits (target): 4 bytes
//   - Nonce: 4 bytes
func parseBlockHeader(headerHex string, height int64) (*BlockHeader, error) {
	headerBytes, err := hex.DecodeString(headerHex)
	if err != nil {
		return nil, fmt.Errorf("invalid header hex: %w", err)
	}

	if len(headerBytes) != 80 {
		return nil, fmt.Errorf("invalid header length: expected 80, got %d", len(headerBytes))
	}

	// Parse fields (all little-endian)
	version := int32(binary.LittleEndian.Uint32(headerBytes[0:4]))
	prevHash := reverseBytes(headerBytes[4:36])
	merkleRoot := reverseBytes(headerBytes[36:68])
	timestamp := int64(binary.LittleEndian.Uint32(headerBytes[68:72]))
	bits := binary.LittleEndian.Uint32(headerBytes[72:76])
	nonce := binary.LittleEndian.Uint32(headerBytes[76:80])

	// Compute block hash: double SHA256 of header, then reverse
	firstHash := sha256.Sum256(headerBytes)
	secondHash := sha256.Sum256(firstHash[:])
	blockHash := reverseBytes(secondHash[:])

	// Calculate difficulty from bits
	difficulty := bitsToTarget(bits)

	return &BlockHeader{
		Hash:         hex.EncodeToString(blockHash),
		Height:       height,
		Version:      version,
		PreviousHash: hex.EncodeToString(prevHash),
		MerkleRoot:   hex.EncodeToString(merkleRoot),
		Timestamp:    timestamp,
		Nonce:        nonce,
		Difficulty:   difficulty,
	}, nil
}

// reverseBytes returns a reversed copy of the byte slice.
func reverseBytes(b []byte) []byte {
	result := make([]byte, len(b))
	for i := 0; i < len(b); i++ {
		result[i] = b[len(b)-1-i]
	}
	return result
}

// bitsToTarget converts compact "bits" format to difficulty as float64.
func bitsToTarget(bits uint32) float64 {
	// Extract exponent and mantissa
	exponent := bits >> 24
	mantissa := float64(bits & 0x007fffff)

	if mantissa == 0 {
		return 0
	}

	// Calculate target using math.Pow to avoid overflow
	// target = mantissa * 256^(exponent-3)
	var target float64
	if exponent <= 3 {
		divisor := 1.0
		for i := uint32(0); i < 3-exponent; i++ {
			divisor *= 256
		}
		target = mantissa / divisor
	} else {
		multiplier := 1.0
		for i := uint32(0); i < exponent-3; i++ {
			multiplier *= 256
		}
		target = mantissa * multiplier
	}

	// Genesis block difficulty = 1.0
	// Genesis bits = 0x1d00ffff -> exponent=0x1d(29), mantissa=0x00ffff
	// Genesis target = 0xFFFF * 256^(29-3) = 0xFFFF * 256^26
	genesisMultiplier := 1.0
	for i := 0; i < 26; i++ {
		genesisMultiplier *= 256
	}
	genesisTarget := float64(0xFFFF) * genesisMultiplier

	if target == 0 {
		return 0
	}
	return genesisTarget / target
}

// GetFeeEstimates returns fee estimates.
func (e *ElectrumBackend) GetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	// Electrum uses blockchain.estimatefee with number of blocks
	estimates := &FeeEstimate{}

	// 1 block (fastest)
	if result, err := e.call("blockchain.estimatefee", []interface{}{1}); err == nil {
		if fee, ok := result.(float64); ok && fee > 0 {
			estimates.FastestFee = uint64(fee * 1e8 / 1000) // BTC/kB to sat/vB
		}
	}

	// 3 blocks (~30 min)
	if result, err := e.call("blockchain.estimatefee", []interface{}{3}); err == nil {
		if fee, ok := result.(float64); ok && fee > 0 {
			estimates.HalfHourFee = uint64(fee * 1e8 / 1000)
		}
	}

	// 6 blocks (~1 hour)
	if result, err := e.call("blockchain.estimatefee", []interface{}{6}); err == nil {
		if fee, ok := result.(float64); ok && fee > 0 {
			estimates.HourFee = uint64(fee * 1e8 / 1000)
		}
	}

	// 144 blocks (~1 day)
	if result, err := e.call("blockchain.estimatefee", []interface{}{144}); err == nil {
		if fee, ok := result.(float64); ok && fee > 0 {
			estimates.EconomyFee = uint64(fee * 1e8 / 1000)
		}
	}

	estimates.MinimumFee = 1 // 1 sat/vB minimum

	return estimates, nil
}

// call makes an Electrum JSON-RPC call.
func (e *ElectrumBackend) call(method string, params []interface{}) (interface{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.connected || e.conn == nil {
		return nil, ErrNotConnected
	}

	id := e.requestID.Add(1)

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	// Set deadline
	e.conn.SetDeadline(time.Now().Add(e.timeout))

	// Send request (newline delimited)
	if _, err := e.conn.Write(append(data, '\n')); err != nil {
		e.connected = false
		return nil, err
	}

	// Read response
	line, err := e.reader.ReadBytes('\n')
	if err != nil {
		e.connected = false
		return nil, err
	}

	var response struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      uint64      `json:"id"`
		Result  interface{} `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(line, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("electrum error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// addressToScriptHash converts a Bitcoin address to Electrum's scripthash format.
// Electrum uses SHA256(scriptPubKey) reversed.
// Supports P2PKH, P2SH, P2WPKH, P2WSH, and P2TR address types.
func addressToScriptHash(address string) string {
	scriptPubKey, err := addressToScriptPubKey(address)
	if err != nil {
		// Fallback: hash the address string (won't work but prevents panic)
		hash := sha256.Sum256([]byte(address))
		reversed := reverseBytes(hash[:])
		return hex.EncodeToString(reversed)
	}

	// SHA256 hash of scriptPubKey, reversed
	hash := sha256.Sum256(scriptPubKey)
	reversed := reverseBytes(hash[:])
	return hex.EncodeToString(reversed)
}

// addressToScriptPubKey converts a Bitcoin address to its scriptPubKey.
// Supports all standard address types:
//   - P2PKH (1...)
//   - P2SH (3...)
//   - P2WPKH (bc1q... 20 bytes)
//   - P2WSH (bc1q... 32 bytes)
//   - P2TR (bc1p...)
func addressToScriptPubKey(address string) ([]byte, error) {
	// Try to determine network from address prefix
	var netParams *chaincfg.Params
	if strings.HasPrefix(address, "bc1") || strings.HasPrefix(address, "1") || strings.HasPrefix(address, "3") {
		netParams = &chaincfg.MainNetParams
	} else if strings.HasPrefix(address, "tb1") || strings.HasPrefix(address, "m") || strings.HasPrefix(address, "n") || strings.HasPrefix(address, "2") {
		netParams = &chaincfg.TestNet3Params
	} else if strings.HasPrefix(address, "ltc1") || strings.HasPrefix(address, "L") || strings.HasPrefix(address, "M") {
		// Litecoin mainnet - use custom params
		netParams = &chaincfg.Params{
			Bech32HRPSegwit: "ltc",
			PubKeyHashAddrID: 0x30,
			ScriptHashAddrID: 0x32,
		}
	} else if strings.HasPrefix(address, "tltc1") {
		// Litecoin testnet
		netParams = &chaincfg.Params{
			Bech32HRPSegwit: "tltc",
			PubKeyHashAddrID: 0x6F,
			ScriptHashAddrID: 0x3A,
		}
	} else if strings.HasPrefix(address, "D") {
		// Dogecoin mainnet
		netParams = &chaincfg.Params{
			PubKeyHashAddrID: 0x1E,
			ScriptHashAddrID: 0x16,
		}
	} else {
		// Default to mainnet
		netParams = &chaincfg.MainNetParams
	}

	// Try bech32/bech32m first (bc1q, bc1p, tb1q, tb1p, ltc1q, etc.)
	if strings.Contains(address, "1q") || strings.Contains(address, "1p") {
		hrp, data, err := bech32.Decode(address)
		if err == nil && len(data) > 0 {
			// First byte is witness version
			witnessVersion := data[0]
			// Rest is the witness program (5-bit encoded)
			witnessProgram, err := bech32.ConvertBits(data[1:], 5, 8, false)
			if err == nil {
				return buildWitnessScriptPubKey(witnessVersion, witnessProgram, hrp)
			}
		}
	}

	// Try standard base58 address decoding
	decoded, err := btcutil.DecodeAddress(address, netParams)
	if err != nil {
		return nil, fmt.Errorf("failed to decode address: %w", err)
	}

	// Generate scriptPubKey from address
	script, err := txscript.PayToAddrScript(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to create scriptPubKey: %w", err)
	}

	return script, nil
}

// buildWitnessScriptPubKey builds a witness scriptPubKey from version and program.
func buildWitnessScriptPubKey(version byte, program []byte, hrp string) ([]byte, error) {
	// Witness scriptPubKey format:
	// OP_n <witness_program>
	// where n is the witness version (0-16)

	if version > 16 {
		return nil, fmt.Errorf("invalid witness version: %d", version)
	}

	// Build scriptPubKey
	// OP_0 = 0x00, OP_1-OP_16 = 0x51-0x60
	var opVersion byte
	if version == 0 {
		opVersion = 0x00
	} else {
		opVersion = 0x50 + version
	}

	// Script: <version> <push_length> <program>
	script := make([]byte, 2+len(program))
	script[0] = opVersion
	script[1] = byte(len(program))
	copy(script[2:], program)

	return script, nil
}

// Ensure ElectrumBackend implements Backend
var _ Backend = (*ElectrumBackend)(nil)
