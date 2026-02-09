package helpers

import (
	"testing"
)

func TestCompareBytes(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
		want int
	}{
		{"equal", []byte{1, 2, 3}, []byte{1, 2, 3}, 0},
		{"a less", []byte{1, 2, 3}, []byte{1, 2, 4}, -1},
		{"a greater", []byte{1, 2, 4}, []byte{1, 2, 3}, 1},
		{"a shorter", []byte{1, 2}, []byte{1, 2, 3}, -1},
		{"a longer", []byte{1, 2, 3}, []byte{1, 2}, 1},
		{"empty equal", []byte{}, []byte{}, 0},
		{"a empty", []byte{}, []byte{1}, -1},
		{"b empty", []byte{1}, []byte{}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareBytes(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareBytes = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBytesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
		want bool
	}{
		{"equal", []byte{1, 2, 3}, []byte{1, 2, 3}, true},
		{"not equal", []byte{1, 2, 3}, []byte{1, 2, 4}, false},
		{"different length", []byte{1, 2}, []byte{1, 2, 3}, false},
		{"empty equal", []byte{}, []byte{}, true},
		{"nil equal", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BytesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("BytesEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsZeroBytes(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want bool
	}{
		{"all zeros", []byte{0, 0, 0}, true},
		{"has non-zero", []byte{0, 1, 0}, false},
		{"empty", []byte{}, true},
		{"single zero", []byte{0}, true},
		{"single non-zero", []byte{1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsZeroBytes(tt.b)
			if got != tt.want {
				t.Errorf("IsZeroBytes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		amount   uint64
		decimals uint8
		want     string
	}{
		{100000000, 8, "1"},          // 1 BTC
		{50000000, 8, "0.5"},         // 0.5 BTC
		{12345678, 8, "0.12345678"},  // All decimals
		{100000, 8, "0.001"},         // Small amount
		{1, 8, "0.00000001"},         // 1 satoshi
		{0, 8, "0"},                  // Zero
		{1000000000000000000, 18, "1"}, // 1 ETH
		{500000000000000000, 18, "0.5"}, // 0.5 ETH
		{123, 0, "123"},             // No decimals
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatAmount(tt.amount, tt.decimals)
			if got != tt.want {
				t.Errorf("FormatAmount(%d, %d) = %s, want %s", tt.amount, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		input    string
		decimals uint8
		want     uint64
		wantErr  bool
	}{
		{"1", 8, 100000000, false},
		{"0.5", 8, 50000000, false},
		{"0.12345678", 8, 12345678, false},
		{"0.001", 8, 100000, false},
		{"0.00000001", 8, 1, false},
		{"0", 8, 0, false},
		{"1", 18, 1000000000000000000, false},
		{"123", 0, 123, false},
		{"invalid", 8, 0, true},
		{"1.2.3", 8, 0, true},
		{"", 8, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAmount(tt.input, tt.decimals)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseAmount(%s, %d) = %d, want %d", tt.input, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestFormatParseRoundtrip(t *testing.T) {
	amounts := []uint64{1, 100, 12345678, 100000000, 999999999}

	for _, amount := range amounts {
		formatted := FormatAmount(amount, 8)
		parsed, err := ParseAmount(formatted, 8)
		if err != nil {
			t.Errorf("ParseAmount(%s) failed: %v", formatted, err)
			continue
		}
		if parsed != amount {
			t.Errorf("roundtrip failed: %d -> %s -> %d", amount, formatted, parsed)
		}
	}
}

func TestSatoshisBTCConversion(t *testing.T) {
	// Test SatoshisToBTC
	if got := SatoshisToBTC(100000000); got != "1" {
		t.Errorf("SatoshisToBTC(100000000) = %s, want 1", got)
	}

	// Test BTCToSatoshis
	if got, err := BTCToSatoshis("1"); err != nil || got != 100000000 {
		t.Errorf("BTCToSatoshis(1) = %d, %v, want 100000000, nil", got, err)
	}
}
