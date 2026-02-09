package backend

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Klingon-tech/klingdex/pkg/helpers"
)

// RPCType identifies the RPC protocol type.
type RPCType string

const (
	RPCTypeBitcoin RPCType = "bitcoin" // Bitcoin Core style RPC
	RPCTypeEVM     RPCType = "evm"     // Ethereum/EVM style RPC
)

// JSONRPCBackend implements Backend using direct JSON-RPC to nodes.
// Supports both Bitcoin Core and EVM (Ethereum) RPC protocols.
type JSONRPCBackend struct {
	rpcURL     string
	rpcType    RPCType
	rpcUser    string
	rpcPass    string
	httpClient *http.Client
	mu         sync.RWMutex
	connected  bool
	requestID  atomic.Uint64
}

// NewJSONRPCBackend creates a new JSON-RPC backend.
// rpcType should be "bitcoin" for Bitcoin Core or "evm" for Ethereum.
func NewJSONRPCBackend(rpcURL string, rpcType RPCType, user, pass string) *JSONRPCBackend {
	return &JSONRPCBackend{
		rpcURL:  rpcURL,
		rpcType: rpcType,
		rpcUser: user,
		rpcPass: pass,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns TypeJSONRPC.
func (j *JSONRPCBackend) Type() Type {
	return TypeJSONRPC
}

// Connect tests the connection to the node.
func (j *JSONRPCBackend) Connect(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var err error
	if j.rpcType == RPCTypeEVM {
		// Test with eth_blockNumber
		_, err = j.evmCall(ctx, "eth_blockNumber", []interface{}{})
	} else {
		// Test with getblockchaininfo
		_, err = j.bitcoinCall(ctx, "getblockchaininfo", []interface{}{})
	}

	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConnected, err)
	}

	j.connected = true
	return nil
}

// Close closes the connection.
func (j *JSONRPCBackend) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.connected = false
	return nil
}

// IsConnected returns true if connected.
func (j *JSONRPCBackend) IsConnected() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.connected
}

// GetAddressInfo returns address balance.
// For Bitcoin, uses scantxoutset to scan the UTXO set.
// For EVM, uses eth_getBalance.
func (j *JSONRPCBackend) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmGetAddressInfo(ctx, address)
	}
	return j.bitcoinGetAddressInfo(ctx, address)
}

// bitcoinGetAddressInfo uses scantxoutset to get address balance from UTXO set.
// Note: scantxoutset scans the entire UTXO set, which can be slow on first run.
// The scan is cached by Bitcoin Core for subsequent calls.
func (j *JSONRPCBackend) bitcoinGetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	result, err := j.bitcoinCall(ctx, "scantxoutset", []interface{}{
		"start",
		[]string{"addr(" + address + ")"},
	})
	if err != nil {
		return nil, fmt.Errorf("scantxoutset failed: %w", err)
	}

	var scan struct {
		Success     bool    `json:"success"`
		TxOuts      int64   `json:"txouts"`
		Height      int64   `json:"height"`
		TotalAmount float64 `json:"total_amount"`
		Unspent     []struct {
			TxID   string  `json:"txid"`
			Vout   uint32  `json:"vout"`
			Amount float64 `json:"amount"`
			Height int64   `json:"height"`
		} `json:"unspents"`
	}

	if err := json.Unmarshal(result, &scan); err != nil {
		return nil, fmt.Errorf("failed to parse scantxoutset result: %w", err)
	}

	if !scan.Success {
		return nil, fmt.Errorf("scantxoutset scan failed")
	}

	// Convert BTC to satoshis
	balance := uint64(scan.TotalAmount * 1e8)

	return &AddressInfo{
		Address:       address,
		Balance:       balance,
		FundedTxCount: int64(len(scan.Unspent)), // Number of unspent outputs
		FundedSum:     balance,                  // Total funded amount
	}, nil
}

// GetAddressUTXOs returns UTXOs (Bitcoin only).
func (j *JSONRPCBackend) GetAddressUTXOs(ctx context.Context, address string) ([]UTXO, error) {
	if j.rpcType == RPCTypeBitcoin {
		return j.bitcoinGetUTXOs(ctx, address)
	}
	// EVM doesn't have UTXOs
	return nil, fmt.Errorf("UTXOs not applicable for EVM chains")
}

// GetAddressTxs is not directly supported by node RPCs.
func (j *JSONRPCBackend) GetAddressTxs(ctx context.Context, address string, lastSeenTxID string) ([]Transaction, error) {
	return nil, fmt.Errorf("address transaction history not supported by node RPC (use indexer)")
}

// GetTransaction returns a transaction.
func (j *JSONRPCBackend) GetTransaction(ctx context.Context, txID string) (*Transaction, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmGetTransaction(ctx, txID)
	}
	return j.bitcoinGetTransaction(ctx, txID)
}

// GetRawTransaction returns raw transaction.
func (j *JSONRPCBackend) GetRawTransaction(ctx context.Context, txID string) ([]byte, error) {
	if j.rpcType == RPCTypeEVM {
		tx, err := j.evmGetTransaction(ctx, txID)
		if err != nil {
			return nil, err
		}
		return []byte(tx.Hex), nil
	}
	return j.bitcoinGetRawTransaction(ctx, txID)
}

// BroadcastTransaction broadcasts a transaction.
func (j *JSONRPCBackend) BroadcastTransaction(ctx context.Context, rawTxHex string) (string, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmBroadcast(ctx, rawTxHex)
	}
	return j.bitcoinBroadcast(ctx, rawTxHex)
}

// GetBlockHeight returns current block height.
func (j *JSONRPCBackend) GetBlockHeight(ctx context.Context) (int64, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmGetBlockHeight(ctx)
	}
	return j.bitcoinGetBlockHeight(ctx)
}

// GetBlockHeader returns block header.
func (j *JSONRPCBackend) GetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmGetBlockHeader(ctx, hashOrHeight)
	}
	return j.bitcoinGetBlockHeader(ctx, hashOrHeight)
}

// GetFeeEstimates returns fee estimates.
func (j *JSONRPCBackend) GetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	if j.rpcType == RPCTypeEVM {
		return j.evmGetFeeEstimates(ctx)
	}
	return j.bitcoinGetFeeEstimates(ctx)
}

// ============ Bitcoin RPC Methods ============

func (j *JSONRPCBackend) bitcoinCall(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	return j.call(ctx, method, params, true)
}

func (j *JSONRPCBackend) bitcoinGetBlockHeight(ctx context.Context) (int64, error) {
	result, err := j.bitcoinCall(ctx, "getblockcount", []interface{}{})
	if err != nil {
		return 0, err
	}

	var height int64
	if err := json.Unmarshal(result, &height); err != nil {
		return 0, err
	}

	return height, nil
}

func (j *JSONRPCBackend) bitcoinGetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	// If it's a height, get the hash first
	hash := hashOrHeight
	var height int64

	if _, err := fmt.Sscanf(hashOrHeight, "%d", &height); err == nil {
		// It's a height, get hash
		result, err := j.bitcoinCall(ctx, "getblockhash", []interface{}{height})
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(result, &hash); err != nil {
			return nil, err
		}
	}

	result, err := j.bitcoinCall(ctx, "getblockheader", []interface{}{hash, true})
	if err != nil {
		return nil, err
	}

	var header struct {
		Hash          string  `json:"hash"`
		Height        int64   `json:"height"`
		Version       int32   `json:"version"`
		PreviousHash  string  `json:"previousblockhash"`
		MerkleRoot    string  `json:"merkleroot"`
		Time          int64   `json:"time"`
		Bits          string  `json:"bits"`
		Nonce         uint32  `json:"nonce"`
		Difficulty    float64 `json:"difficulty"`
		Confirmations int64   `json:"confirmations"`
		NTx           int64   `json:"nTx"`
	}

	if err := json.Unmarshal(result, &header); err != nil {
		return nil, err
	}

	return &BlockHeader{
		Hash:         header.Hash,
		Height:       header.Height,
		Version:      header.Version,
		PreviousHash: header.PreviousHash,
		MerkleRoot:   header.MerkleRoot,
		Timestamp:    header.Time,
		Nonce:        header.Nonce,
		Difficulty:   header.Difficulty,
		TxCount:      header.NTx,
	}, nil
}

func (j *JSONRPCBackend) bitcoinGetTransaction(ctx context.Context, txID string) (*Transaction, error) {
	result, err := j.bitcoinCall(ctx, "getrawtransaction", []interface{}{txID, true})
	if err != nil {
		return nil, ErrTxNotFound
	}

	var btcTx struct {
		TxID          string `json:"txid"`
		Hash          string `json:"hash"`
		Version       int32  `json:"version"`
		Size          int64  `json:"size"`
		VSize         int64  `json:"vsize"`
		Weight        int64  `json:"weight"`
		LockTime      uint32 `json:"locktime"`
		Hex           string `json:"hex"`
		BlockHash     string `json:"blockhash"`
		Confirmations int64  `json:"confirmations"`
		Time          int64  `json:"time"`
		BlockTime     int64  `json:"blocktime"`
	}

	if err := json.Unmarshal(result, &btcTx); err != nil {
		return nil, err
	}

	return &Transaction{
		TxID:          btcTx.TxID,
		Version:       btcTx.Version,
		Size:          btcTx.Size,
		VSize:         btcTx.VSize,
		Weight:        btcTx.Weight,
		LockTime:      btcTx.LockTime,
		Hex:           btcTx.Hex,
		BlockHash:     btcTx.BlockHash,
		Confirmations: btcTx.Confirmations,
		BlockTime:     btcTx.BlockTime,
		Confirmed:     btcTx.Confirmations > 0,
	}, nil
}

func (j *JSONRPCBackend) bitcoinGetRawTransaction(ctx context.Context, txID string) ([]byte, error) {
	result, err := j.bitcoinCall(ctx, "getrawtransaction", []interface{}{txID, false})
	if err != nil {
		return nil, ErrTxNotFound
	}

	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

func (j *JSONRPCBackend) bitcoinBroadcast(ctx context.Context, rawTxHex string) (string, error) {
	result, err := j.bitcoinCall(ctx, "sendrawtransaction", []interface{}{rawTxHex})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBroadcastFailed, err)
	}

	var txID string
	if err := json.Unmarshal(result, &txID); err != nil {
		return "", err
	}

	return txID, nil
}

func (j *JSONRPCBackend) bitcoinGetUTXOs(ctx context.Context, address string) ([]UTXO, error) {
	// Use scantxoutset for address-based UTXO lookup
	result, err := j.bitcoinCall(ctx, "scantxoutset", []interface{}{
		"start",
		[]string{"addr(" + address + ")"},
	})
	if err != nil {
		return nil, err
	}

	var scan struct {
		Success bool `json:"success"`
		Unspent []struct {
			TxID         string  `json:"txid"`
			Vout         uint32  `json:"vout"`
			ScriptPubKey string  `json:"scriptPubKey"`
			Amount       float64 `json:"amount"`
			Height       int64   `json:"height"`
		} `json:"unspents"`
	}

	if err := json.Unmarshal(result, &scan); err != nil {
		return nil, err
	}

	utxos := make([]UTXO, len(scan.Unspent))
	for i, u := range scan.Unspent {
		utxos[i] = UTXO{
			TxID:         u.TxID,
			Vout:         u.Vout,
			Amount:       uint64(u.Amount * 1e8),
			ScriptPubKey: u.ScriptPubKey,
			BlockHeight:  u.Height,
		}
	}

	return utxos, nil
}

func (j *JSONRPCBackend) bitcoinGetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	estimates := &FeeEstimate{MinimumFee: 1}

	// estimatesmartfee returns BTC/kB
	for _, target := range []struct {
		blocks int
		field  *uint64
	}{
		{1, &estimates.FastestFee},
		{3, &estimates.HalfHourFee},
		{6, &estimates.HourFee},
		{144, &estimates.EconomyFee},
	} {
		result, err := j.bitcoinCall(ctx, "estimatesmartfee", []interface{}{target.blocks})
		if err != nil {
			continue
		}

		var feeResult struct {
			FeeRate float64 `json:"feerate"`
		}
		if err := json.Unmarshal(result, &feeResult); err != nil {
			continue
		}

		if feeResult.FeeRate > 0 {
			*target.field = uint64(feeResult.FeeRate * 1e8 / 1000) // BTC/kB to sat/vB
		}
	}

	return estimates, nil
}

// ============ EVM RPC Methods ============

func (j *JSONRPCBackend) evmCall(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	return j.call(ctx, method, params, false)
}

func (j *JSONRPCBackend) evmGetBlockHeight(ctx context.Context) (int64, error) {
	result, err := j.evmCall(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var hexHeight string
	if err := json.Unmarshal(result, &hexHeight); err != nil {
		return 0, err
	}

	return helpers.HexToInt64(hexHeight), nil
}

func (j *JSONRPCBackend) evmGetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	result, err := j.evmCall(ctx, "eth_getBalance", []interface{}{address, "latest"})
	if err != nil {
		return nil, err
	}

	var hexBalance string
	if err := json.Unmarshal(result, &hexBalance); err != nil {
		return nil, err
	}

	balance := helpers.HexToUint64(hexBalance)

	// Get nonce (tx count)
	result, err = j.evmCall(ctx, "eth_getTransactionCount", []interface{}{address, "latest"})
	if err != nil {
		return nil, err
	}

	var hexNonce string
	if err := json.Unmarshal(result, &hexNonce); err != nil {
		return nil, err
	}

	return &AddressInfo{
		Address: address,
		Balance: balance,
		TxCount: helpers.HexToInt64(hexNonce),
	}, nil
}

func (j *JSONRPCBackend) evmGetTransaction(ctx context.Context, txHash string) (*Transaction, error) {
	result, err := j.evmCall(ctx, "eth_getTransactionByHash", []interface{}{txHash})
	if err != nil {
		return nil, ErrTxNotFound
	}

	var evmTx struct {
		Hash        string `json:"hash"`
		BlockHash   string `json:"blockHash"`
		BlockNumber string `json:"blockNumber"`
		Input       string `json:"input"`
	}

	if err := json.Unmarshal(result, &evmTx); err != nil {
		return nil, err
	}

	if evmTx.Hash == "" {
		return nil, ErrTxNotFound
	}

	tx := &Transaction{
		TxID:      evmTx.Hash,
		BlockHash: evmTx.BlockHash,
		Hex:       evmTx.Input,
		Confirmed: evmTx.BlockNumber != "",
	}

	if evmTx.BlockNumber != "" {
		tx.BlockHeight = helpers.HexToInt64(evmTx.BlockNumber)
	}

	return tx, nil
}

func (j *JSONRPCBackend) evmBroadcast(ctx context.Context, rawTxHex string) (string, error) {
	// Ensure 0x prefix
	if !strings.HasPrefix(rawTxHex, "0x") {
		rawTxHex = "0x" + rawTxHex
	}

	result, err := j.evmCall(ctx, "eth_sendRawTransaction", []interface{}{rawTxHex})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBroadcastFailed, err)
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", err
	}

	return txHash, nil
}

func (j *JSONRPCBackend) evmGetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	// Convert to hex if numeric
	blockID := hashOrHeight
	if !strings.HasPrefix(hashOrHeight, "0x") {
		var height int64
		if _, err := fmt.Sscanf(hashOrHeight, "%d", &height); err == nil {
			blockID = fmt.Sprintf("0x%x", height)
		}
	}

	result, err := j.evmCall(ctx, "eth_getBlockByNumber", []interface{}{blockID, false})
	if err != nil {
		return nil, err
	}

	var block struct {
		Hash       string        `json:"hash"`
		Number     string        `json:"number"`
		ParentHash string        `json:"parentHash"`
		Timestamp  string        `json:"timestamp"`
		Difficulty string        `json:"difficulty"`
		TxCount    int64         `json:"-"`
		Txs        []interface{} `json:"transactions"`
	}

	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return &BlockHeader{
		Hash:         block.Hash,
		Height:       helpers.HexToInt64(block.Number),
		PreviousHash: block.ParentHash,
		Timestamp:    helpers.HexToInt64(block.Timestamp),
		TxCount:      int64(len(block.Txs)),
	}, nil
}

func (j *JSONRPCBackend) evmGetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	result, err := j.evmCall(ctx, "eth_gasPrice", []interface{}{})
	if err != nil {
		return nil, err
	}

	var hexGasPrice string
	if err := json.Unmarshal(result, &hexGasPrice); err != nil {
		return nil, err
	}

	gasPrice := helpers.HexToUint64(hexGasPrice)

	// EVM uses gas price in wei, not sat/vB
	// We return gas price in gwei for consistency
	gwei := gasPrice / 1e9

	return &FeeEstimate{
		FastestFee:  gwei,
		HalfHourFee: gwei,
		HourFee:     gwei,
		EconomyFee:  gwei,
		MinimumFee:  1,
	}, nil
}

// ============ EVM-Specific Public Methods ============

// EVMGetNonce returns the current nonce (transaction count) for an address.
func (j *JSONRPCBackend) EVMGetNonce(ctx context.Context, address string) (uint64, error) {
	if j.rpcType != RPCTypeEVM {
		return 0, fmt.Errorf("EVMGetNonce only available for EVM backends")
	}

	result, err := j.evmCall(ctx, "eth_getTransactionCount", []interface{}{address, "pending"})
	if err != nil {
		return 0, err
	}

	var hexNonce string
	if err := json.Unmarshal(result, &hexNonce); err != nil {
		return 0, err
	}

	return helpers.HexToUint64(hexNonce), nil
}

// EVMEstimateGas estimates gas for a transaction.
func (j *JSONRPCBackend) EVMEstimateGas(ctx context.Context, from, to string, value *big.Int, data []byte) (uint64, error) {
	if j.rpcType != RPCTypeEVM {
		return 0, fmt.Errorf("EVMEstimateGas only available for EVM backends")
	}

	callObj := map[string]interface{}{
		"from": from,
		"to":   to,
	}

	if value != nil && value.Sign() > 0 {
		callObj["value"] = fmt.Sprintf("0x%x", value)
	}

	if len(data) > 0 {
		callObj["data"] = "0x" + hex.EncodeToString(data)
	}

	result, err := j.evmCall(ctx, "eth_estimateGas", []interface{}{callObj})
	if err != nil {
		return 0, err
	}

	var hexGas string
	if err := json.Unmarshal(result, &hexGas); err != nil {
		return 0, err
	}

	return helpers.HexToUint64(hexGas), nil
}

// EVMGetGasPrice returns the current gas price in wei.
func (j *JSONRPCBackend) EVMGetGasPrice(ctx context.Context) (*big.Int, error) {
	if j.rpcType != RPCTypeEVM {
		return nil, fmt.Errorf("EVMGetGasPrice only available for EVM backends")
	}

	result, err := j.evmCall(ctx, "eth_gasPrice", []interface{}{})
	if err != nil {
		return nil, err
	}

	var hexGasPrice string
	if err := json.Unmarshal(result, &hexGasPrice); err != nil {
		return nil, err
	}

	return helpers.HexToBigInt(hexGasPrice), nil
}

// EVMCall executes a read-only contract call (eth_call).
func (j *JSONRPCBackend) EVMCall(ctx context.Context, to string, data []byte) ([]byte, error) {
	if j.rpcType != RPCTypeEVM {
		return nil, fmt.Errorf("EVMCall only available for EVM backends")
	}

	callObj := map[string]interface{}{
		"to":   to,
		"data": "0x" + hex.EncodeToString(data),
	}

	result, err := j.evmCall(ctx, "eth_call", []interface{}{callObj, "latest"})
	if err != nil {
		return nil, err
	}

	var hexResult string
	if err := json.Unmarshal(result, &hexResult); err != nil {
		return nil, err
	}

	return hex.DecodeString(strings.TrimPrefix(hexResult, "0x"))
}

// EVMGetChainID returns the chain ID.
func (j *JSONRPCBackend) EVMGetChainID(ctx context.Context) (uint64, error) {
	if j.rpcType != RPCTypeEVM {
		return 0, fmt.Errorf("EVMGetChainID only available for EVM backends")
	}

	result, err := j.evmCall(ctx, "eth_chainId", []interface{}{})
	if err != nil {
		return 0, err
	}

	var hexChainID string
	if err := json.Unmarshal(result, &hexChainID); err != nil {
		return 0, err
	}

	return helpers.HexToUint64(hexChainID), nil
}

// IsEVM returns true if this is an EVM backend.
func (j *JSONRPCBackend) IsEVM() bool {
	return j.rpcType == RPCTypeEVM
}

// ============ Common Methods ============

func (j *JSONRPCBackend) call(ctx context.Context, method string, params []interface{}, useAuth bool) (json.RawMessage, error) {
	id := j.requestID.Add(1)

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

	req, err := http.NewRequestWithContext(ctx, "POST", j.rpcURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if useAuth && j.rpcUser != "" {
		req.SetBasicAuth(j.rpcUser, j.rpcPass)
	}

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      uint64          `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// Ensure JSONRPCBackend implements Backend
var _ Backend = (*JSONRPCBackend)(nil)
