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

// BlockbookBackend implements Backend using Trezor's Blockbook API.
// API docs: https://github.com/trezor/blockbook/blob/master/docs/api.md
type BlockbookBackend struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	connected  bool
}

// NewBlockbookBackend creates a new Blockbook backend.
// baseURL should be like "https://btc1.trezor.io/api/v2" or "https://doge1.trezor.io/api/v2"
func NewBlockbookBackend(baseURL string) *BlockbookBackend {
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &BlockbookBackend{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns TypeBlockbook.
func (b *BlockbookBackend) Type() Type {
	return TypeBlockbook
}

// Connect tests the connection to the API.
func (b *BlockbookBackend) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Test with status endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL, nil)
	if err != nil {
		return err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConnected, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrNotConnected, resp.StatusCode)
	}

	b.connected = true
	return nil
}

// Close closes the connection.
func (b *BlockbookBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = false
	return nil
}

// IsConnected returns true if connected.
func (b *BlockbookBackend) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// GetAddressInfo returns address balance and tx count.
func (b *BlockbookBackend) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	var result struct {
		Address            string `json:"address"`
		Balance            string `json:"balance"`
		UnconfirmedBalance string `json:"unconfirmedBalance"`
		TxCount            int64  `json:"txs"`
		UnconfirmedTxs     int64  `json:"unconfirmedTxs"`
	}

	if err := b.get(ctx, "/address/"+address, &result); err != nil {
		return nil, err
	}

	balance := parseAmount(result.Balance)
	unconfirmed := parseAmountSigned(result.UnconfirmedBalance)

	return &AddressInfo{
		Address:        result.Address,
		TxCount:        result.TxCount,
		Balance:        balance,
		MempoolBalance: unconfirmed,
	}, nil
}

// GetAddressUTXOs returns unspent outputs for an address.
func (b *BlockbookBackend) GetAddressUTXOs(ctx context.Context, address string) ([]UTXO, error) {
	var result []struct {
		TxID          string `json:"txid"`
		Vout          uint32 `json:"vout"`
		Value         string `json:"value"`
		Height        int64  `json:"height"`
		Confirmations int64  `json:"confirmations"`
	}

	if err := b.get(ctx, "/utxo/"+address, &result); err != nil {
		return nil, err
	}

	utxos := make([]UTXO, len(result))
	for i, u := range result {
		utxos[i] = UTXO{
			TxID:          u.TxID,
			Vout:          u.Vout,
			Amount:        parseAmount(u.Value),
			BlockHeight:   u.Height,
			Confirmations: u.Confirmations,
		}
	}

	return utxos, nil
}

// GetAddressTxs returns transactions for an address.
func (b *BlockbookBackend) GetAddressTxs(ctx context.Context, address string, lastSeenTxID string) ([]Transaction, error) {
	endpoint := "/address/" + address + "?details=txs"
	if lastSeenTxID != "" {
		endpoint += "&from=" + lastSeenTxID
	}

	var result struct {
		Transactions []blockbookTx `json:"transactions"`
	}

	if err := b.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return b.convertTxs(result.Transactions), nil
}

// GetTransaction returns a transaction by ID.
func (b *BlockbookBackend) GetTransaction(ctx context.Context, txID string) (*Transaction, error) {
	var result blockbookTx

	if err := b.get(ctx, "/tx/"+txID, &result); err != nil {
		return nil, err
	}

	txs := b.convertTxs([]blockbookTx{result})
	if len(txs) == 0 {
		return nil, ErrTxNotFound
	}

	return &txs[0], nil
}

// GetRawTransaction returns raw transaction hex.
func (b *BlockbookBackend) GetRawTransaction(ctx context.Context, txID string) ([]byte, error) {
	var result struct {
		Hex string `json:"hex"`
	}

	if err := b.get(ctx, "/tx/"+txID, &result); err != nil {
		return nil, err
	}

	return []byte(result.Hex), nil
}

// BroadcastTransaction broadcasts a raw transaction.
func (b *BlockbookBackend) BroadcastTransaction(ctx context.Context, rawTxHex string) (string, error) {
	// Blockbook uses GET with hex in URL for broadcast
	var result struct {
		Result string `json:"result"`
	}

	if err := b.get(ctx, "/sendtx/"+rawTxHex, &result); err != nil {
		return "", fmt.Errorf("%w: %v", ErrBroadcastFailed, err)
	}

	return result.Result, nil
}

// GetBlockHeight returns the current block height.
func (b *BlockbookBackend) GetBlockHeight(ctx context.Context) (int64, error) {
	var result struct {
		Blockbook struct {
			BestHeight int64 `json:"bestHeight"`
		} `json:"blockbook"`
	}

	if err := b.get(ctx, "", &result); err != nil {
		return 0, err
	}

	return result.Blockbook.BestHeight, nil
}

// GetBlockHeader returns block header info.
func (b *BlockbookBackend) GetBlockHeader(ctx context.Context, hashOrHeight string) (*BlockHeader, error) {
	var result struct {
		Hash          string  `json:"hash"`
		Height        int64   `json:"height"`
		PreviousHash  string  `json:"previousBlockHash"`
		Time          int64   `json:"time"`
		TxCount       int64   `json:"txCount"`
		Confirmations int64   `json:"confirmations"`
		Difficulty    float64 `json:"difficulty"`
	}

	if err := b.get(ctx, "/block/"+hashOrHeight, &result); err != nil {
		return nil, err
	}

	return &BlockHeader{
		Hash:         result.Hash,
		Height:       result.Height,
		PreviousHash: result.PreviousHash,
		Timestamp:    result.Time,
		TxCount:      result.TxCount,
		Difficulty:   result.Difficulty,
	}, nil
}

// GetFeeEstimates returns fee estimates.
func (b *BlockbookBackend) GetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	var result struct {
		Result string `json:"result"`
	}

	estimates := &FeeEstimate{MinimumFee: 1}

	// Blockbook uses /estimatefee/{blocks}
	// Returns BTC/kB, we need sat/vB

	if err := b.get(ctx, "/estimatefee/1", &result); err == nil {
		estimates.FastestFee = btcKBToSatVB(result.Result)
	}

	if err := b.get(ctx, "/estimatefee/3", &result); err == nil {
		estimates.HalfHourFee = btcKBToSatVB(result.Result)
	}

	if err := b.get(ctx, "/estimatefee/6", &result); err == nil {
		estimates.HourFee = btcKBToSatVB(result.Result)
	}

	if err := b.get(ctx, "/estimatefee/144", &result); err == nil {
		estimates.EconomyFee = btcKBToSatVB(result.Result)
	}

	return estimates, nil
}

// get performs a GET request and decodes JSON response.
func (b *BlockbookBackend) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := b.httpClient.Do(req)
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

// blockbookTx is Blockbook's transaction format.
type blockbookTx struct {
	TxID          string `json:"txid"`
	Version       int32  `json:"version"`
	LockTime      uint32 `json:"lockTime"`
	Size          int64  `json:"size"`
	VSize         int64  `json:"vsize"`
	Hex           string `json:"hex"`
	BlockHash     string `json:"blockHash"`
	BlockHeight   int64  `json:"blockHeight"`
	BlockTime     int64  `json:"blockTime"`
	Confirmations int64  `json:"confirmations"`
	Fees          string `json:"fees"`
	Vin           []struct {
		TxID      string   `json:"txid"`
		Vout      uint32   `json:"vout"`
		Sequence  uint32   `json:"sequence"`
		Addresses []string `json:"addresses"`
		Value     string   `json:"value"`
	} `json:"vin"`
	Vout []struct {
		Value     string   `json:"value"`
		N         uint32   `json:"n"`
		Addresses []string `json:"addresses"`
		Hex       string   `json:"hex"`
	} `json:"vout"`
}

// convertTxs converts Blockbook transactions to our format.
func (b *BlockbookBackend) convertTxs(bbTxs []blockbookTx) []Transaction {
	txs := make([]Transaction, len(bbTxs))
	for i, bt := range bbTxs {
		tx := Transaction{
			TxID:          bt.TxID,
			Version:       bt.Version,
			LockTime:      bt.LockTime,
			Size:          bt.Size,
			VSize:         bt.VSize,
			Hex:           bt.Hex,
			BlockHash:     bt.BlockHash,
			BlockHeight:   bt.BlockHeight,
			BlockTime:     bt.BlockTime,
			Confirmations: bt.Confirmations,
			Confirmed:     bt.Confirmations > 0,
			Fee:           parseAmount(bt.Fees),
			Inputs:        make([]TxInput, len(bt.Vin)),
			Outputs:       make([]TxOutput, len(bt.Vout)),
		}

		for j, vin := range bt.Vin {
			addr := ""
			if len(vin.Addresses) > 0 {
				addr = vin.Addresses[0]
			}
			tx.Inputs[j] = TxInput{
				TxID:     vin.TxID,
				Vout:     vin.Vout,
				Sequence: vin.Sequence,
				PrevOut: &TxOutput{
					ScriptPubKeyAddr: addr,
					Value:            parseAmount(vin.Value),
				},
			}
		}

		for j, vout := range bt.Vout {
			addr := ""
			if len(vout.Addresses) > 0 {
				addr = vout.Addresses[0]
			}
			tx.Outputs[j] = TxOutput{
				ScriptPubKey:     vout.Hex,
				ScriptPubKeyAddr: addr,
				Value:            parseAmount(vout.Value),
			}
		}

		txs[i] = tx
	}
	return txs
}

// parseAmount parses a string amount to uint64 (satoshis).
func parseAmount(s string) uint64 {
	var amount uint64
	fmt.Sscanf(s, "%d", &amount)
	return amount
}

// parseAmountSigned parses a string amount that might be negative.
func parseAmountSigned(s string) int64 {
	var amount int64
	fmt.Sscanf(s, "%d", &amount)
	return amount
}

// btcKBToSatVB converts BTC/kB to sat/vB.
func btcKBToSatVB(s string) uint64 {
	var btcPerKB float64
	fmt.Sscanf(s, "%f", &btcPerKB)
	if btcPerKB <= 0 {
		return 1
	}
	// BTC/kB to sat/vB: multiply by 1e8 (sat/BTC) and divide by 1000 (bytes/kB)
	return uint64(btcPerKB * 1e8 / 1000)
}

// Ensure BlockbookBackend implements Backend
var _ Backend = (*BlockbookBackend)(nil)
