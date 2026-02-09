// Package helpers provides common utility functions used across the codebase.
package helpers

import (
	"fmt"
	"math/big"
)

// FormatAmount formats an amount in smallest units as a decimal string.
// For example, FormatAmount(100000000, 8) returns "1" (1 BTC).
func FormatAmount(amount uint64, decimals uint8) string {
	if decimals == 0 {
		return fmt.Sprintf("%d", amount)
	}

	amountBig := new(big.Int).SetUint64(amount)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)

	whole := new(big.Int).Div(amountBig, divisor)
	frac := new(big.Int).Mod(amountBig, divisor)

	if frac.Sign() == 0 {
		return whole.String()
	}

	fracStr := fmt.Sprintf("%0*d", int(decimals), frac)
	// Trim trailing zeros
	for len(fracStr) > 0 && fracStr[len(fracStr)-1] == '0' {
		fracStr = fracStr[:len(fracStr)-1]
	}

	return fmt.Sprintf("%s.%s", whole.String(), fracStr)
}

// ParseAmount parses a decimal string to smallest units.
// For example, ParseAmount("1", 8) returns 100000000 (1 BTC in satoshis).
func ParseAmount(s string, decimals uint8) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty amount string")
	}

	// Find decimal point
	var wholeStr, fracStr string
	for i, c := range s {
		if c == '.' {
			wholeStr = s[:i]
			fracStr = s[i+1:]
			break
		}
	}
	if wholeStr == "" {
		wholeStr = s
	}

	// Validate characters
	for _, c := range wholeStr {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid character in amount: %c", c)
		}
	}
	for _, c := range fracStr {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid character in amount: %c", c)
		}
	}

	// Pad or truncate fractional part
	for len(fracStr) < int(decimals) {
		fracStr += "0"
	}
	if len(fracStr) > int(decimals) {
		fracStr = fracStr[:decimals]
	}

	// Parse combined value
	combined := wholeStr + fracStr
	amount := new(big.Int)
	_, ok := amount.SetString(combined, 10)
	if !ok {
		return 0, fmt.Errorf("invalid amount: %s", s)
	}

	if !amount.IsUint64() {
		return 0, fmt.Errorf("amount overflow: %s", s)
	}

	return amount.Uint64(), nil
}

// SatoshisToBTC converts satoshis to BTC string (8 decimals).
func SatoshisToBTC(satoshis uint64) string {
	return FormatAmount(satoshis, 8)
}

// BTCToSatoshis converts a BTC string to satoshis.
func BTCToSatoshis(btc string) (uint64, error) {
	return ParseAmount(btc, 8)
}

// WeiToETH converts wei to ETH string (18 decimals).
func WeiToETH(wei uint64) string {
	return FormatAmount(wei, 18)
}

// ETHToWei converts an ETH string to wei.
func ETHToWei(eth string) (uint64, error) {
	return ParseAmount(eth, 18)
}
