package swap

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
)

func TestBuildRefundScript(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKey := privKey.PubKey()

	tests := []struct {
		name          string
		pubKey        *btcec.PublicKey
		timeoutBlocks uint32
		wantErr       bool
		errContains   string
	}{
		{
			name:          "valid BTC timeout",
			pubKey:        pubKey,
			timeoutBlocks: DefaultTakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "valid maker timeout",
			pubKey:        pubKey,
			timeoutBlocks: DefaultMakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "valid LTC timeout",
			pubKey:        pubKey,
			timeoutBlocks: LTCTakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "nil pubkey",
			pubKey:        nil,
			timeoutBlocks: 72,
			wantErr:       true,
			errContains:   "pubkey cannot be nil",
		},
		{
			name:          "zero timeout",
			pubKey:        pubKey,
			timeoutBlocks: 0,
			wantErr:       true,
			errContains:   "timeout blocks must be > 0",
		},
		{
			name:          "timeout too large",
			pubKey:        pubKey,
			timeoutBlocks: 0x10000, // > 0xFFFF
			wantErr:       true,
			errContains:   "timeout blocks too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := BuildRefundScript(tt.pubKey, tt.timeoutBlocks)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %q, should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(script) == 0 {
				t.Error("script should not be empty")
			}

			// Verify script structure: should contain OP_CHECKSEQUENCEVERIFY
			// Script: <timeout> OP_CSV OP_DROP <pubkey> OP_CHECKSIG
			foundCSV := false
			for _, op := range script {
				if op == txscript.OP_CHECKSEQUENCEVERIFY {
					foundCSV = true
					break
				}
			}
			if !foundCSV {
				t.Error("script should contain OP_CHECKSEQUENCEVERIFY")
			}

			t.Logf("Script length: %d bytes", len(script))
		})
	}
}

func TestBuildTaprootScriptTree(t *testing.T) {
	// Generate keys
	aggPrivKey, _ := btcec.NewPrivateKey()
	refundPrivKey, _ := btcec.NewPrivateKey()

	aggPubKey := aggPrivKey.PubKey()
	refundPubKey := refundPrivKey.PubKey()

	tests := []struct {
		name          string
		aggKey        *btcec.PublicKey
		refundKey     *btcec.PublicKey
		timeoutBlocks uint32
		wantErr       bool
		errContains   string
	}{
		{
			name:          "valid tree - BTC taker",
			aggKey:        aggPubKey,
			refundKey:     refundPubKey,
			timeoutBlocks: DefaultTakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "valid tree - BTC maker",
			aggKey:        aggPubKey,
			refundKey:     refundPubKey,
			timeoutBlocks: DefaultMakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "valid tree - LTC",
			aggKey:        aggPubKey,
			refundKey:     refundPubKey,
			timeoutBlocks: LTCTakerTimeoutBlocks,
			wantErr:       false,
		},
		{
			name:          "nil aggregated key",
			aggKey:        nil,
			refundKey:     refundPubKey,
			timeoutBlocks: 72,
			wantErr:       true,
			errContains:   "aggregated key cannot be nil",
		},
		{
			name:          "nil refund key",
			aggKey:        aggPubKey,
			refundKey:     nil,
			timeoutBlocks: 72,
			wantErr:       true,
			errContains:   "refund pubkey cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := BuildTaprootScriptTree(tt.aggKey, tt.refundKey, tt.timeoutBlocks)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %q, should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify tree structure
			if tree.InternalKey == nil {
				t.Error("InternalKey should not be nil")
			}
			if tree.TweakedKey == nil {
				t.Error("TweakedKey should not be nil")
			}
			if len(tree.RefundScript) == 0 {
				t.Error("RefundScript should not be empty")
			}
			if len(tree.MerkleRoot) != 32 {
				t.Errorf("MerkleRoot length = %d, want 32", len(tree.MerkleRoot))
			}
			if len(tree.ControlBlock) == 0 {
				t.Error("ControlBlock should not be empty")
			}
			if tree.TimeoutBlocks != tt.timeoutBlocks {
				t.Errorf("TimeoutBlocks = %d, want %d", tree.TimeoutBlocks, tt.timeoutBlocks)
			}

			// Tweaked key should be different from internal key
			if tree.TweakedKey.IsEqual(tree.InternalKey) {
				t.Error("TweakedKey should be different from InternalKey")
			}
		})
	}
}

func TestTaprootScriptTreeAddress(t *testing.T) {
	aggPrivKey, _ := btcec.NewPrivateKey()
	refundPrivKey, _ := btcec.NewPrivateKey()

	tree, err := BuildTaprootScriptTree(
		aggPrivKey.PubKey(),
		refundPrivKey.PubKey(),
		DefaultTakerTimeoutBlocks,
	)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}

	tests := []struct {
		name       string
		hrp        string
		wantPrefix string
	}{
		{
			name:       "BTC mainnet",
			hrp:        "bc",
			wantPrefix: "bc1p",
		},
		{
			name:       "BTC testnet",
			hrp:        "tb",
			wantPrefix: "tb1p",
		},
		{
			name:       "LTC mainnet",
			hrp:        "ltc",
			wantPrefix: "ltc1p",
		},
		{
			name:       "LTC testnet",
			hrp:        "tltc",
			wantPrefix: "tltc1p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := tree.TaprootAddress(tt.hrp)
			if err != nil {
				t.Fatalf("TaprootAddress() error: %v", err)
			}

			if len(addr) < len(tt.wantPrefix) || addr[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Errorf("address %s should start with %s", addr, tt.wantPrefix)
			}

			t.Logf("%s address: %s", tt.name, addr)
		})
	}
}

func TestTaprootScriptTreeScriptPubKey(t *testing.T) {
	aggPrivKey, _ := btcec.NewPrivateKey()
	refundPrivKey, _ := btcec.NewPrivateKey()

	tree, err := BuildTaprootScriptTree(
		aggPrivKey.PubKey(),
		refundPrivKey.PubKey(),
		DefaultTakerTimeoutBlocks,
	)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}

	scriptPubKey, err := tree.ScriptPubKey()
	if err != nil {
		t.Fatalf("ScriptPubKey() error: %v", err)
	}

	// P2TR scriptPubKey is 34 bytes: OP_1 (1 byte) + OP_DATA_32 (1 byte) + 32-byte key
	if len(scriptPubKey) != 34 {
		t.Errorf("scriptPubKey length = %d, want 34", len(scriptPubKey))
	}

	// First byte should be OP_1 (0x51)
	if scriptPubKey[0] != txscript.OP_1 {
		t.Errorf("first byte = 0x%x, want 0x%x (OP_1)", scriptPubKey[0], txscript.OP_1)
	}

	// Second byte should be OP_DATA_32 (0x20)
	if scriptPubKey[1] != txscript.OP_DATA_32 {
		t.Errorf("second byte = 0x%x, want 0x%x (OP_DATA_32)", scriptPubKey[1], txscript.OP_DATA_32)
	}
}

func TestTaprootScriptTreeHexMethods(t *testing.T) {
	aggPrivKey, _ := btcec.NewPrivateKey()
	refundPrivKey, _ := btcec.NewPrivateKey()

	tree, err := BuildTaprootScriptTree(
		aggPrivKey.PubKey(),
		refundPrivKey.PubKey(),
		DefaultTakerTimeoutBlocks,
	)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}

	// Test RefundScriptHex
	refundHex := tree.RefundScriptHex()
	if len(refundHex) == 0 {
		t.Error("RefundScriptHex should not be empty")
	}
	if len(refundHex) != len(tree.RefundScript)*2 {
		t.Errorf("RefundScriptHex length = %d, want %d", len(refundHex), len(tree.RefundScript)*2)
	}

	// Test ControlBlockHex
	controlHex := tree.ControlBlockHex()
	if len(controlHex) == 0 {
		t.Error("ControlBlockHex should not be empty")
	}
	if len(controlHex) != len(tree.ControlBlock)*2 {
		t.Errorf("ControlBlockHex length = %d, want %d", len(controlHex), len(tree.ControlBlock)*2)
	}
}

func TestGetTimeoutBlocks(t *testing.T) {
	tests := []struct {
		symbol  string
		isMaker bool
		want    uint32
	}{
		{"BTC", true, DefaultMakerTimeoutBlocks},
		{"BTC", false, DefaultTakerTimeoutBlocks},
		{"LTC", true, LTCMakerTimeoutBlocks},
		{"LTC", false, LTCTakerTimeoutBlocks},
		{"UNKNOWN", true, DefaultMakerTimeoutBlocks},  // Falls back to default
		{"UNKNOWN", false, DefaultTakerTimeoutBlocks}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.symbol+"_isMaker="+boolStr(tt.isMaker), func(t *testing.T) {
			got := GetTimeoutBlocks(tt.symbol, tt.isMaker)
			if got != tt.want {
				t.Errorf("GetTimeoutBlocks(%s, %v) = %d, want %d", tt.symbol, tt.isMaker, got, tt.want)
			}
		})
	}
}

func TestValidateTimeoutRelationship(t *testing.T) {
	tests := []struct {
		name        string
		makerBlocks uint32
		takerBlocks uint32
		wantErr     bool
	}{
		{
			name:        "valid BTC defaults",
			makerBlocks: DefaultMakerTimeoutBlocks, // 144
			takerBlocks: DefaultTakerTimeoutBlocks, // 72
			wantErr:     false,
		},
		{
			name:        "valid LTC defaults",
			makerBlocks: LTCMakerTimeoutBlocks, // 576
			takerBlocks: LTCTakerTimeoutBlocks, // 288
			wantErr:     false,
		},
		{
			name:        "maker equals taker - invalid",
			makerBlocks: 100,
			takerBlocks: 100,
			wantErr:     true,
		},
		{
			name:        "maker less than taker - invalid",
			makerBlocks: 50,
			takerBlocks: 100,
			wantErr:     true,
		},
		{
			name:        "insufficient margin",
			makerBlocks: 102,
			takerBlocks: 100, // Only 2 blocks difference, need at least 10 (10% of 100)
			wantErr:     true,
		},
		{
			name:        "small values - minimum 6 block diff",
			makerBlocks: 20,
			takerBlocks: 10, // 10 blocks diff, but need at least 6 (minimum)
			wantErr:     false,
		},
		{
			name:        "small values - insufficient diff",
			makerBlocks: 15,
			takerBlocks: 10, // Only 5 blocks diff, need 6 minimum
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeoutRelationship(tt.makerBlocks, tt.takerBlocks)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTimeoutConstants(t *testing.T) {
	// Verify timeout constants make sense

	// BTC: maker > taker
	if DefaultMakerTimeoutBlocks <= DefaultTakerTimeoutBlocks {
		t.Error("BTC maker timeout should be > taker timeout")
	}

	// LTC: maker > taker
	if LTCMakerTimeoutBlocks <= LTCTakerTimeoutBlocks {
		t.Error("LTC maker timeout should be > taker timeout")
	}

	// BTC: ~24h maker, ~12h taker (at ~10 min blocks)
	btcMakerHours := float64(DefaultMakerTimeoutBlocks) * 10 / 60
	btcTakerHours := float64(DefaultTakerTimeoutBlocks) * 10 / 60
	t.Logf("BTC timeouts: maker=%.1fh, taker=%.1fh", btcMakerHours, btcTakerHours)

	// LTC: ~24h maker, ~12h taker (at ~2.5 min blocks)
	ltcMakerHours := float64(LTCMakerTimeoutBlocks) * 2.5 / 60
	ltcTakerHours := float64(LTCTakerTimeoutBlocks) * 2.5 / 60
	t.Logf("LTC timeouts: maker=%.1fh, taker=%.1fh", ltcMakerHours, ltcTakerHours)

	// Validate default relationships pass validation
	if err := ValidateTimeoutRelationship(DefaultMakerTimeoutBlocks, DefaultTakerTimeoutBlocks); err != nil {
		t.Errorf("BTC default timeouts should be valid: %v", err)
	}
	if err := ValidateTimeoutRelationship(LTCMakerTimeoutBlocks, LTCTakerTimeoutBlocks); err != nil {
		t.Errorf("LTC default timeouts should be valid: %v", err)
	}
}

// Helper functions

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
