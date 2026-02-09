package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MempoolBackend implements Backend using the mempool.space API.
// Compatible with mempool.space, litecoinspace.org, and self-hosted instances.
type MempoolBackend struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	connected  bool
}

// NewMempoolBackend creates a new mempool.space backend.
func NewMempoolBackend(baseURL string) *MempoolBackend {
	// Remove trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &MempoolBackend{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns TypeMempool.
func (m *MempoolBackend) Type() Type {
	return TypeMempool
}

// Connect tests the connection to the API.
func (m *MempoolBackend) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Test connection by getting block height
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL+"/blocks/tip/height", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConnected, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrNotConnected, resp.StatusCode)
	}

	m.connected = true
	return nil
}

// Close closes the connection.
func (m *MempoolBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected returns true if connected.
func (m *MempoolBackend) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// GetAddressInfo returns address balance and tx count.
func (m *MempoolBackend) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	var result struct {
		Address    string `json:"address"`
		ChainStats struct {
			FundedTxoCount int64  `json:"funded_txo_count"`
			FundedTxoSum   uint64 `json:"funded_txo_sum"`
			SpentTxoCount  int64  `json:"spent_txo_count"`
			SpentTxoSum    uint64 `json:"spent_txo_sum"`
			TxCount        int64  `json:"tx_count"`
		} `json:"chain_stats"`
		MempoolStats struct {
			FundedTxoCount int64  `json:"funded_txo_count"`
			FundedTxoSum   uint64 `json:"funded_txo_sum"`
			SpentTxoCount  int64  `json:"spent_txo_count"`
			SpentTxoSum    uint64 `json:"spent_txo_sum"`
			TxCount        int64  `json:"tx_count"`
		} `json:"mempool_stats"`
	}

	if err := m.get(ctx, "/address/"+address, &result); err != nil {
		return nil, err
	}

	balance := result.ChainStats.FundedTxoSum - result.ChainStats.SpentTxoSum
	mempoolDelta := int64(result.MempoolStats.FundedTxoSum) - int64(result.MempoolStats.SpentTxoSum)

	return &AddressInfo{
		Address:        result.Address,
		TxCount:        result.ChainStats.TxCount + result.MempoolStats.TxCount,
		FundedTxCount:  result.ChainStats.FundedTxoCount,
		SpentTxCount:   result.ChainStats.SpentTxoCount,
		FundedSum:      result.ChainStats.FundedTxoSum,
		SpentSum:       result.ChainStats.SpentTxoSum,
		Balance:        balance,
		MempoolBalance: mempoolDelta,
	}, nil
}

// GetAddressUTXOs returns unspent outputs for an address.
func (m *MempoolBackend) GetAddressUTXOs(ctx context.Context, address string) ([]UTXO, error) {
	var result []struct {
		TxID   string `json:"txid"`
		Vout   uint32 `json:"vout"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height"`
		} `json:"status"`
		Value uint64 `json:"value"`
	}

	if err := m.get(ctx, "/address/"+address+"/utxo", &result); err != nil {
		return nil, err
	}

	// Fetch current block height for confirmation calculation
	currentHeight, err := m.GetBlockHeight(ctx)
	if err != nil {
		// If we can't get block height, fall back to simple confirmed/unconfirmed
		currentHeight = 0
	}

	utxos := make([]UTXO, len(result))
	for i, u := range result {
		var confirmations int64
		if u.Status.Confirmed && u.Status.BlockHeight > 0 {
			if currentHeight > 0 {
				// Calculate exact confirmations: current_height - block_height + 1
				confirmations = currentHeight - u.Status.BlockHeight + 1
			} else {
				// Fallback: at least 1 confirmation if confirmed
				confirmations = 1
			}
		}
		utxos[i] = UTXO{
			TxID:          u.TxID,
			Vout:          u.Vout,
			Amount:        u.Value,
			Confirmations: confirmations,
			BlockHeight:   u.Status.BlockHeight,
		}
	}

	return utxos, nil
}

// GetAddressTxs returns transactions for an address.
func (m *MempoolBackend) GetAddressTxs(ctx context.Context, address string, lastSeenTxID string) ([]Transaction, error) {
	endpoint := "/address/" + address + "/txs"
	if lastSeenTxID != "" {
		endpoint += "/chain/" + lastSeenTxID
	}

	var result []mempoolTx
	if err := m.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return m.convertTxs(result), nil
}

// GetTransaction returns a transaction by ID.
func (m *MempoolBackend) GetTransaction(ctx context.Context, txID string) (*Transaction, error) {
	var result mempoolTx
	if err := m.get(ctx, "/tx/"+txID, &result); err != nil {
		return nil, err
	}

	txs := m.convertTxs([]mempoolTx{result})
	if len(txs) == 0 {
		return nil, ErrTxNotFound
	}

	tx := &txs[0]

	// Calculate confirmations if transaction is confirmed
	// mempool.space API returns block_height but not confirmations directly
	if tx.Confirmed && tx.BlockHeight > 0 {
		currentHeight, err := m.GetBlockHeight(ctx)
		if err == nil && currentHeight >= tx.BlockHeight {
			tx.Confirmations = currentHeight - tx.BlockHeight + 1
		}
	}

	return tx, nil
}

// GetRawTransaction returns raw transaction hex.
func (m *MempoolBackend) GetRawTransaction(ctx context.Context, txID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL+"/tx/"+txID+"/hex", nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrTxNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// BroadcastTransaction broadcasts a raw transaction.
func (m *MempoolBackend) BroadcastTransaction(ctx context.Context, rawTxHex string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/tx", strings.NewReader(rawTxHex))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBroadcastFailed, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s", ErrBroadcastFailed, string(body))
	}

	// Response is the txid
	return strings.TrimSpace(string(body)), nil
}

// GetBlockHeight returns the current block height.
func (m *MempoolBackend) GetBlockHeight(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL+"/blocks/tip/height", nil)
	if err != nil {
		return 0, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var height int64
	if err := json.Unmarshal(body, &height); err != nil {
		return 0, err
	}

	return height, nil
}

// GetBlockHeader returns block header info.
func (m *MempoolBackend) GetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	var result struct {
		ID           string  `json:"id"`
		Height       int64   `json:"height"`
		Version      int32   `json:"version"`
		Timestamp    int64   `json:"timestamp"`
		Bits         uint32  `json:"bits"`
		Nonce        uint32  `json:"nonce"`
		Difficulty   float64 `json:"difficulty"`
		MerkleRoot   string  `json:"merkle_root"`
		PreviousHash string  `json:"previousblockhash"`
		TxCount      int64   `json:"tx_count"`
	}

	if err := m.get(ctx, "/block/"+hashOrHeight, &result); err != nil {
		return nil, err
	}

	return &BlockHeader{
		Hash:         result.ID,
		Height:       result.Height,
		Version:      result.Version,
		PreviousHash: result.PreviousHash,
		MerkleRoot:   result.MerkleRoot,
		Timestamp:    result.Timestamp,
		Bits:         result.Bits,
		Nonce:        result.Nonce,
		Difficulty:   result.Difficulty,
		TxCount:      result.TxCount,
	}, nil
}

// GetFeeEstimates returns fee estimates for different confirmation targets.
func (m *MempoolBackend) GetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	var result map[string]float64
	if err := m.get(ctx, "/v1/fees/recommended", &result); err != nil {
		return nil, err
	}

	return &FeeEstimate{
		FastestFee:  uint64(result["fastestFee"]),
		HalfHourFee: uint64(result["halfHourFee"]),
		HourFee:     uint64(result["hourFee"]),
		EconomyFee:  uint64(result["economyFee"]),
		MinimumFee:  uint64(result["minimumFee"]),
	}, nil
}

// get performs a GET request and decodes JSON response.
func (m *MempoolBackend) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL+path, nil)
	if err != nil {
		return err
	}

	// Add cache-busting headers to avoid stale CDN responses
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrAddressNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// mempoolTx is the mempool.space transaction format.
type mempoolTx struct {
	TxID     string `json:"txid"`
	Version  int32  `json:"version"`
	LockTime uint32 `json:"locktime"`
	Size     int64  `json:"size"`
	Weight   int64  `json:"weight"`
	Fee      uint64 `json:"fee"`
	Status   struct {
		Confirmed   bool   `json:"confirmed"`
		BlockHeight int64  `json:"block_height"`
		BlockHash   string `json:"block_hash"`
		BlockTime   int64  `json:"block_time"`
	} `json:"status"`
	Vin []struct {
		TxID         string   `json:"txid"`
		Vout         uint32   `json:"vout"`
		ScriptSig    string   `json:"scriptsig"`
		ScriptSigAsm string   `json:"scriptsig_asm"`
		Witness      []string `json:"witness"`
		Sequence     uint32   `json:"sequence"`
		Prevout      *struct {
			ScriptPubKey     string `json:"scriptpubkey"`
			ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
			ScriptPubKeyType string `json:"scriptpubkey_type"`
			ScriptPubKeyAddr string `json:"scriptpubkey_address"`
			Value            uint64 `json:"value"`
		} `json:"prevout"`
	} `json:"vin"`
	Vout []struct {
		ScriptPubKey     string `json:"scriptpubkey"`
		ScriptPubKeyAsm  string `json:"scriptpubkey_asm"`
		ScriptPubKeyType string `json:"scriptpubkey_type"`
		ScriptPubKeyAddr string `json:"scriptpubkey_address"`
		Value            uint64 `json:"value"`
	} `json:"vout"`
}

// convertTxs converts mempool format to our Transaction format.
func (m *MempoolBackend) convertTxs(mTxs []mempoolTx) []Transaction {
	txs := make([]Transaction, len(mTxs))
	for i, mt := range mTxs {
		tx := Transaction{
			TxID:        mt.TxID,
			Version:     mt.Version,
			Size:        mt.Size,
			Weight:      mt.Weight,
			VSize:       (mt.Weight + 3) / 4, // Calculate vsize from weight
			LockTime:    mt.LockTime,
			Fee:         mt.Fee,
			Confirmed:   mt.Status.Confirmed,
			BlockHash:   mt.Status.BlockHash,
			BlockHeight: mt.Status.BlockHeight,
			BlockTime:   mt.Status.BlockTime,
			Inputs:      make([]TxInput, len(mt.Vin)),
			Outputs:     make([]TxOutput, len(mt.Vout)),
		}

		for j, vin := range mt.Vin {
			input := TxInput{
				TxID:         vin.TxID,
				Vout:         vin.Vout,
				ScriptSig:    vin.ScriptSig,
				ScriptSigAsm: vin.ScriptSigAsm,
				Witness:      vin.Witness,
				Sequence:     vin.Sequence,
			}
			if vin.Prevout != nil {
				input.PrevOut = &TxOutput{
					ScriptPubKey:     vin.Prevout.ScriptPubKey,
					ScriptPubKeyAsm:  vin.Prevout.ScriptPubKeyAsm,
					ScriptPubKeyType: vin.Prevout.ScriptPubKeyType,
					ScriptPubKeyAddr: vin.Prevout.ScriptPubKeyAddr,
					Value:            vin.Prevout.Value,
				}
			}
			tx.Inputs[j] = input
		}

		for j, vout := range mt.Vout {
			tx.Outputs[j] = TxOutput{
				ScriptPubKey:     vout.ScriptPubKey,
				ScriptPubKeyAsm:  vout.ScriptPubKeyAsm,
				ScriptPubKeyType: vout.ScriptPubKeyType,
				ScriptPubKeyAddr: vout.ScriptPubKeyAddr,
				Value:            vout.Value,
			}
		}

		txs[i] = tx
	}
	return txs
}

// Ensure MempoolBackend implements Backend
var _ Backend = (*MempoolBackend)(nil)
