// Package helpers provides common utility functions used across the codebase.
package helpers

import (
	"encoding/hex"
	"math/big"
	"strings"
)

// HexToInt64 converts a hex string (with or without 0x prefix) to int64.
func HexToInt64(s string) int64 {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return 0
	}
	val, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return 0
	}
	return val.Int64()
}

// HexToUint64 converts a hex string (with or without 0x prefix) to uint64.
func HexToUint64(s string) uint64 {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return 0
	}
	val, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return 0
	}
	return val.Uint64()
}

// HexToBigInt converts a hex string (with or without 0x prefix) to *big.Int.
func HexToBigInt(s string) *big.Int {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return big.NewInt(0)
	}
	val, ok := new(big.Int).SetString(s, 16)
	if !ok || val == nil {
		return big.NewInt(0)
	}
	return val
}

// BigIntToHex converts a *big.Int to a hex string with 0x prefix.
func BigIntToHex(n *big.Int) string {
	if n == nil || n.Sign() == 0 {
		return "0x0"
	}
	return "0x" + n.Text(16)
}

// Uint64ToHex converts a uint64 to a hex string with 0x prefix.
func Uint64ToHex(n uint64) string {
	if n == 0 {
		return "0x0"
	}
	return "0x" + new(big.Int).SetUint64(n).Text(16)
}

// HexToBytes converts a hex string (with or without 0x prefix) to bytes.
func HexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	return hex.DecodeString(s)
}

// BytesToHex converts bytes to a hex string with 0x prefix.
func BytesToHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}

// PadLeft pads a byte slice with zeros on the left to reach the specified length.
func PadLeft(b []byte, length int) []byte {
	if len(b) >= length {
		return b
	}
	result := make([]byte, length)
	copy(result[length-len(b):], b)
	return result
}

// PadRight pads a byte slice with zeros on the right to reach the specified length.
func PadRight(b []byte, length int) []byte {
	if len(b) >= length {
		return b
	}
	result := make([]byte, length)
	copy(result, b)
	return result
}
