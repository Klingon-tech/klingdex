// Package wallet provides EVM transaction building and signing.
package wallet

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// EVMTxType represents the transaction type.
type EVMTxType uint8

const (
	// EVMTxTypeLegacy is a legacy (pre-EIP-2718) transaction.
	EVMTxTypeLegacy EVMTxType = 0
	// EVMTxTypeEIP1559 is an EIP-1559 transaction with dynamic fees.
	EVMTxTypeEIP1559 EVMTxType = 2
)

// EVMTransaction represents an EVM transaction.
type EVMTransaction struct {
	Type EVMTxType

	// Common fields
	Nonce    uint64
	To       string   // 0x-prefixed address (nil for contract creation)
	Value    *big.Int // in wei
	Data     []byte   // input data
	ChainID  uint64
	GasLimit uint64

	// Legacy transaction fields
	GasPrice *big.Int // in wei (for legacy tx)

	// EIP-1559 fields
	MaxFeePerGas         *big.Int // in wei
	MaxPriorityFeePerGas *big.Int // in wei
}

// EVMTxParams are parameters for building an EVM transaction.
type EVMTxParams struct {
	Nonce    uint64
	To       string   // destination address
	Value    *big.Int // amount in wei
	Data     []byte   // contract data (nil for simple transfer)
	ChainID  uint64
	GasLimit uint64
	GasPrice *big.Int // gas price in wei (for legacy tx)

	// EIP-1559 (optional - if set, creates type 2 tx)
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
}

// EVMTxResult is the result of building and signing a transaction.
type EVMTxResult struct {
	TxHash  string // transaction hash (0x-prefixed)
	RawTx   string // signed raw transaction hex (0x-prefixed)
	Nonce   uint64
	GasUsed uint64
}

// DefaultGasLimit for simple ETH transfers.
const DefaultGasLimit = uint64(21000)

// DefaultERC20GasLimit for ERC-20 token transfers.
const DefaultERC20GasLimit = uint64(65000)

// BuildAndSignEVMTx builds and signs an EVM transaction.
func BuildAndSignEVMTx(privKey *btcec.PrivateKey, params *EVMTxParams) (*EVMTxResult, error) {
	if params == nil {
		return nil, fmt.Errorf("params required")
	}

	// Validate destination address
	if params.To != "" && !ValidateEVMAddress(params.To) {
		return nil, fmt.Errorf("invalid destination address: %s", params.To)
	}

	// Default gas limit
	gasLimit := params.GasLimit
	if gasLimit == 0 {
		if len(params.Data) > 0 {
			gasLimit = DefaultERC20GasLimit
		} else {
			gasLimit = DefaultGasLimit
		}
	}

	// Build transaction
	tx := &EVMTransaction{
		Nonce:    params.Nonce,
		To:       params.To,
		Value:    params.Value,
		Data:     params.Data,
		ChainID:  params.ChainID,
		GasLimit: gasLimit,
	}

	// Determine transaction type
	if params.MaxFeePerGas != nil && params.MaxPriorityFeePerGas != nil {
		tx.Type = EVMTxTypeEIP1559
		tx.MaxFeePerGas = params.MaxFeePerGas
		tx.MaxPriorityFeePerGas = params.MaxPriorityFeePerGas
	} else {
		tx.Type = EVMTxTypeLegacy
		tx.GasPrice = params.GasPrice
		if tx.GasPrice == nil {
			return nil, fmt.Errorf("gas price required for legacy transaction")
		}
	}

	// Sign transaction
	signedTx, txHash, err := signEVMTransaction(privKey, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return &EVMTxResult{
		TxHash:  txHash,
		RawTx:   signedTx,
		Nonce:   params.Nonce,
		GasUsed: gasLimit,
	}, nil
}

// signEVMTransaction signs an EVM transaction and returns the raw signed transaction hex and hash.
func signEVMTransaction(privKey *btcec.PrivateKey, tx *EVMTransaction) (string, string, error) {
	var unsignedTx []byte
	var chainIDForSig *big.Int

	if tx.Type == EVMTxTypeEIP1559 {
		// EIP-1559 transaction
		unsignedTx = encodeEIP1559Unsigned(tx)
		chainIDForSig = big.NewInt(int64(tx.ChainID))
	} else {
		// Legacy transaction with EIP-155 replay protection
		unsignedTx = encodeLegacyUnsigned(tx)
		chainIDForSig = big.NewInt(int64(tx.ChainID))
	}

	// Hash the unsigned transaction
	msgHash := Keccak256(unsignedTx)

	// Sign using btcec (same secp256k1 curve as Ethereum)
	sig := btcecdsa.SignCompact(privKey, msgHash, false)
	if len(sig) != 65 {
		return "", "", fmt.Errorf("invalid signature length")
	}

	// SignCompact returns: v || r || s (65 bytes) where v is 27 or 28
	v := sig[0]
	r := new(big.Int).SetBytes(sig[1:33])
	s := new(big.Int).SetBytes(sig[33:65])

	var signedTx []byte

	if tx.Type == EVMTxTypeEIP1559 {
		// EIP-1559: v is 0 or 1 (recovery id)
		recoveryID := v - 27
		signedTx = encodeEIP1559Signed(tx, recoveryID, r, s)
	} else {
		// Legacy EIP-155: v = chainId * 2 + 35 + recoveryId
		recoveryID := uint64(v - 27)
		eip155V := chainIDForSig.Uint64()*2 + 35 + recoveryID
		signedTx = encodeLegacySigned(tx, eip155V, r, s)
	}

	// Calculate transaction hash
	txHash := "0x" + hex.EncodeToString(Keccak256(signedTx))
	rawTx := "0x" + hex.EncodeToString(signedTx)

	return rawTx, txHash, nil
}

// =============================================================================
// RLP Encoding
// =============================================================================

// RLP encoding for EVM transactions.
// See: https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/

// rlpEncode encodes data using RLP.
func rlpEncode(data interface{}) []byte {
	switch v := data.(type) {
	case []byte:
		return rlpEncodeBytes(v)
	case string:
		return rlpEncodeBytes([]byte(v))
	case uint64:
		return rlpEncodeUint(v)
	case *big.Int:
		if v == nil || v.Sign() == 0 {
			return rlpEncodeBytes(nil)
		}
		return rlpEncodeBytes(v.Bytes())
	case []interface{}:
		return rlpEncodeList(v)
	default:
		return nil
	}
}

// rlpEncodeBytes encodes bytes.
func rlpEncodeBytes(b []byte) []byte {
	if len(b) == 0 {
		return []byte{0x80}
	}
	if len(b) == 1 && b[0] < 0x80 {
		return b
	}
	if len(b) < 56 {
		return append([]byte{byte(0x80 + len(b))}, b...)
	}
	// Long string
	lenBytes := encodeLength(uint64(len(b)))
	prefix := append([]byte{byte(0xb7 + len(lenBytes))}, lenBytes...)
	return append(prefix, b...)
}

// rlpEncodeUint encodes an unsigned integer.
func rlpEncodeUint(n uint64) []byte {
	if n == 0 {
		return []byte{0x80}
	}
	// Convert to bytes, stripping leading zeros
	var buf [8]byte
	i := 7
	for n > 0 {
		buf[i] = byte(n & 0xff)
		n >>= 8
		i--
	}
	return rlpEncodeBytes(buf[i+1:])
}

// rlpEncodeList encodes a list.
func rlpEncodeList(items []interface{}) []byte {
	var encoded []byte
	for _, item := range items {
		encoded = append(encoded, rlpEncode(item)...)
	}
	if len(encoded) < 56 {
		return append([]byte{byte(0xc0 + len(encoded))}, encoded...)
	}
	// Long list
	lenBytes := encodeLength(uint64(len(encoded)))
	prefix := append([]byte{byte(0xf7 + len(lenBytes))}, lenBytes...)
	return append(prefix, encoded...)
}

// encodeLength encodes a length as big-endian bytes.
func encodeLength(n uint64) []byte {
	if n < 256 {
		return []byte{byte(n)}
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte(n & 0xff)}, buf...)
		n >>= 8
	}
	return buf
}

// =============================================================================
// Transaction Encoding
// =============================================================================

// encodeLegacyUnsigned encodes an unsigned legacy transaction for signing (EIP-155).
func encodeLegacyUnsigned(tx *EVMTransaction) []byte {
	// [nonce, gasPrice, gasLimit, to, value, data, chainId, 0, 0]
	to := addressToBytes(tx.To)
	items := []interface{}{
		tx.Nonce,
		tx.GasPrice,
		tx.GasLimit,
		to,
		tx.Value,
		tx.Data,
		tx.ChainID,
		uint64(0),
		uint64(0),
	}
	return rlpEncode(items)
}

// encodeLegacySigned encodes a signed legacy transaction.
func encodeLegacySigned(tx *EVMTransaction, v uint64, r, s *big.Int) []byte {
	// [nonce, gasPrice, gasLimit, to, value, data, v, r, s]
	to := addressToBytes(tx.To)
	items := []interface{}{
		tx.Nonce,
		tx.GasPrice,
		tx.GasLimit,
		to,
		tx.Value,
		tx.Data,
		v,
		r,
		s,
	}
	return rlpEncode(items)
}

// encodeEIP1559Unsigned encodes an unsigned EIP-1559 transaction for signing.
func encodeEIP1559Unsigned(tx *EVMTransaction) []byte {
	// 0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList])
	to := addressToBytes(tx.To)
	items := []interface{}{
		tx.ChainID,
		tx.Nonce,
		tx.MaxPriorityFeePerGas,
		tx.MaxFeePerGas,
		tx.GasLimit,
		to,
		tx.Value,
		tx.Data,
		[]interface{}{}, // accessList (empty)
	}
	encoded := rlpEncode(items)
	return append([]byte{0x02}, encoded...)
}

// encodeEIP1559Signed encodes a signed EIP-1559 transaction.
func encodeEIP1559Signed(tx *EVMTransaction, v byte, r, s *big.Int) []byte {
	// 0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList, v, r, s])
	to := addressToBytes(tx.To)
	items := []interface{}{
		tx.ChainID,
		tx.Nonce,
		tx.MaxPriorityFeePerGas,
		tx.MaxFeePerGas,
		tx.GasLimit,
		to,
		tx.Value,
		tx.Data,
		[]interface{}{}, // accessList (empty)
		uint64(v),
		r,
		s,
	}
	encoded := rlpEncode(items)
	return append([]byte{0x02}, encoded...)
}

// addressToBytes converts an address string to bytes.
func addressToBytes(addr string) []byte {
	if addr == "" {
		return nil
	}
	addr = strings.TrimPrefix(addr, "0x")
	b, _ := hex.DecodeString(addr)
	return b
}

// =============================================================================
// ERC-20 Helpers
// =============================================================================

// ERC20Transfer function selector: keccak256("transfer(address,uint256)")[:4]
var erc20TransferSelector = []byte{0xa9, 0x05, 0x9c, 0xbb}

// ERC20BalanceOf function selector: keccak256("balanceOf(address)")[:4]
var erc20BalanceOfSelector = []byte{0x70, 0xa0, 0x82, 0x31}

// ERC20Approve function selector: keccak256("approve(address,uint256)")[:4]
var erc20ApproveSelector = []byte{0x09, 0x5e, 0xa7, 0xb3}

// ERC20Allowance function selector: keccak256("allowance(address,address)")[:4]
var erc20AllowanceSelector = []byte{0xdd, 0x62, 0xed, 0x3e}

// EncodeERC20Transfer encodes an ERC-20 transfer call.
func EncodeERC20Transfer(to string, amount *big.Int) ([]byte, error) {
	if !ValidateEVMAddress(to) {
		return nil, fmt.Errorf("invalid recipient address")
	}

	// Function selector (4 bytes) + address (32 bytes) + amount (32 bytes)
	data := make([]byte, 68)
	copy(data[:4], erc20TransferSelector)

	// Address (20 bytes, right-padded in 32-byte slot)
	toBytes := addressToBytes(to)
	copy(data[16:36], toBytes)

	// Amount (32 bytes, big-endian)
	amountBytes := amount.Bytes()
	if len(amountBytes) > 32 {
		return nil, fmt.Errorf("amount too large")
	}
	copy(data[68-len(amountBytes):68], amountBytes)

	return data, nil
}

// EncodeERC20BalanceOf encodes an ERC-20 balanceOf call.
func EncodeERC20BalanceOf(address string) ([]byte, error) {
	if !ValidateEVMAddress(address) {
		return nil, fmt.Errorf("invalid address")
	}

	// Function selector (4 bytes) + address (32 bytes)
	data := make([]byte, 36)
	copy(data[:4], erc20BalanceOfSelector)

	// Address (20 bytes, right-padded in 32-byte slot)
	addrBytes := addressToBytes(address)
	copy(data[16:36], addrBytes)

	return data, nil
}

// EncodeERC20Approve encodes an ERC-20 approve call.
func EncodeERC20Approve(spender string, amount *big.Int) ([]byte, error) {
	if !ValidateEVMAddress(spender) {
		return nil, fmt.Errorf("invalid spender address")
	}

	// Function selector (4 bytes) + address (32 bytes) + amount (32 bytes)
	data := make([]byte, 68)
	copy(data[:4], erc20ApproveSelector)

	// Spender address (20 bytes, right-padded in 32-byte slot)
	spenderBytes := addressToBytes(spender)
	copy(data[16:36], spenderBytes)

	// Amount (32 bytes, big-endian)
	amountBytes := amount.Bytes()
	if len(amountBytes) > 32 {
		return nil, fmt.Errorf("amount too large")
	}
	copy(data[68-len(amountBytes):68], amountBytes)

	return data, nil
}

// DecodeERC20BalanceResult decodes the result of an ERC-20 balanceOf call.
func DecodeERC20BalanceResult(data []byte) (*big.Int, error) {
	if len(data) < 32 {
		return nil, fmt.Errorf("invalid balance result length: %d", len(data))
	}
	return new(big.Int).SetBytes(data[:32]), nil
}

// =============================================================================
// Transaction Building Helpers
// =============================================================================

// BuildSimpleETHTransfer builds a simple ETH transfer transaction.
func BuildSimpleETHTransfer(params *EVMTxParams) *EVMTxParams {
	if params.GasLimit == 0 {
		params.GasLimit = DefaultGasLimit
	}
	params.Data = nil
	return params
}

// BuildERC20Transfer builds an ERC-20 token transfer transaction.
func BuildERC20Transfer(tokenContract, recipient string, amount *big.Int, params *EVMTxParams) (*EVMTxParams, error) {
	data, err := EncodeERC20Transfer(recipient, amount)
	if err != nil {
		return nil, err
	}

	params.To = tokenContract
	params.Value = big.NewInt(0) // No ETH value for token transfers
	params.Data = data
	if params.GasLimit == 0 {
		params.GasLimit = DefaultERC20GasLimit
	}

	return params, nil
}
