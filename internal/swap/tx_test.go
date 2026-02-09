package swap

import (
	"testing"

	"github.com/klingon-exchange/klingon-v2/internal/backend"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
	"github.com/klingon-exchange/klingon-v2/internal/config"
)

func TestCalculateDAOFee(t *testing.T) {
	tests := []struct {
		name     string
		amount   uint64
		isMaker  bool
		wantFee  uint64
	}{
		{
			name:    "maker fee 1 BTC",
			amount:  100000000, // 1 BTC in satoshis
			isMaker: true,
			// 0.2% = 200000 satoshis
			// DAO gets 50% = 100000 satoshis
			wantFee: 100000,
		},
		{
			name:    "taker fee 1 BTC",
			amount:  100000000,
			isMaker: false,
			wantFee: 100000,
		},
		{
			name:    "small amount",
			amount:  10000, // 0.0001 BTC
			isMaker: true,
			// 0.2% of 10000 = 20
			// DAO gets 50% = 10
			// But MinDAOFee = 546 to avoid dust
			wantFee: 546,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDAOFee(tt.amount, tt.isMaker)
			if got != tt.wantFee {
				t.Errorf("CalculateDAOFee = %d, want %d", got, tt.wantFee)
			}
		})
	}

	// Verify it uses config values
	feeCfg := config.DefaultFeeConfig()
	if feeCfg.MakerFeeBPS != 20 {
		t.Errorf("expected maker fee 20 BPS, got %d", feeCfg.MakerFeeBPS)
	}
	if feeCfg.DAOShareBPS != 5000 {
		t.Errorf("expected DAO share 5000 BPS, got %d", feeCfg.DAOShareBPS)
	}
}

func TestSelectUTXOs(t *testing.T) {
	utxos := []backend.UTXO{
		{TxID: "tx1", Vout: 0, Amount: 100000},   // 0.001 BTC
		{TxID: "tx2", Vout: 0, Amount: 500000},   // 0.005 BTC
		{TxID: "tx3", Vout: 1, Amount: 50000},    // 0.0005 BTC
		{TxID: "tx4", Vout: 0, Amount: 1000000},  // 0.01 BTC
	}

	tests := []struct {
		name         string
		targetAmount uint64
		feeRate      uint64
		wantCount    int
		wantErr      bool
	}{
		{
			name:         "select largest first",
			targetAmount: 500000,
			feeRate:      10, // sat/vB
			wantCount:    1,  // Should only need the 1M sat UTXO
			wantErr:      false,
		},
		{
			name:         "need multiple UTXOs",
			targetAmount: 1200000,
			feeRate:      10,
			wantCount:    2, // Need 1M + 500K
			wantErr:      false,
		},
		{
			name:         "insufficient funds",
			targetAmount: 10000000, // More than all UTXOs combined
			feeRate:      10,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, _, err := SelectUTXOs(utxos, tt.targetAmount, tt.feeRate)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(selected) != tt.wantCount {
				t.Errorf("selected %d UTXOs, want %d", len(selected), tt.wantCount)
			}
		})
	}
}

func TestSelectUTXOsEmpty(t *testing.T) {
	_, _, err := SelectUTXOs(nil, 100000, 10)
	if err != ErrNoUTXOs {
		t.Errorf("expected ErrNoUTXOs, got %v", err)
	}

	_, _, err = SelectUTXOs([]backend.UTXO{}, 100000, 10)
	if err != ErrNoUTXOs {
		t.Errorf("expected ErrNoUTXOs, got %v", err)
	}
}

func TestSortUTXOs(t *testing.T) {
	utxos := []backend.UTXO{
		{TxID: "tx1", Amount: 100},
		{TxID: "tx2", Amount: 500},
		{TxID: "tx3", Amount: 50},
		{TxID: "tx4", Amount: 1000},
	}

	sortUTXOs(utxos)

	// Should be sorted descending by amount
	expected := []uint64{1000, 500, 100, 50}
	for i, utxo := range utxos {
		if utxo.Amount != expected[i] {
			t.Errorf("utxos[%d].Amount = %d, want %d", i, utxo.Amount, expected[i])
		}
	}
}

func TestBuildFundingTxValidation(t *testing.T) {
	// Test with invalid chain
	params := &FundingTxParams{
		Symbol:        "INVALID",
		Network:       chain.Testnet,
		UTXOs:         []backend.UTXO{{TxID: "abc", Vout: 0, Amount: 100000}},
		ChangeAddress: "tb1q...",
		SwapAddress:   "tb1p...",
		SwapAmount:    50000,
		FeeRate:       10,
	}

	_, err := BuildFundingTx(params)
	if err == nil {
		t.Error("expected error for invalid chain")
	}

	// Test with no UTXOs
	params.Symbol = "BTC"
	params.UTXOs = nil
	_, err = BuildFundingTx(params)
	if err != ErrNoUTXOs {
		t.Errorf("expected ErrNoUTXOs, got %v", err)
	}
}

func TestBuildSpendingTxValidation(t *testing.T) {
	// Test with missing TaprootAddress (required)
	params := &SpendingTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		FundingTxID:   "0000000000000000000000000000000000000000000000000000000000000001",
		FundingVout:   0,
		FundingAmount: 100000,
		DestAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		FeeRate:       10,
		// TaprootAddress is missing - should fail
	}

	_, _, err := BuildSpendingTx(params)
	if err == nil {
		t.Error("expected error for missing TaprootAddress")
	}

	// Test with invalid chain
	params.TaprootAddress = "tb1pqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesf3hn0c"
	params.Symbol = "INVALID"
	_, _, err = BuildSpendingTx(params)
	if err == nil {
		t.Error("expected error for invalid chain")
	}

	// Test with invalid txid
	params.Symbol = "BTC"
	params.FundingTxID = "not-a-valid-txid!"
	_, _, err = BuildSpendingTx(params)
	if err == nil {
		t.Error("expected error for invalid txid")
	}
}

func TestSerializeDeserializeTx(t *testing.T) {
	// Create a simple transaction
	utxos := []backend.UTXO{
		{
			TxID:   "0000000000000000000000000000000000000000000000000000000000000001",
			Vout:   0,
			Amount: 100000,
		},
	}

	params := &FundingTxParams{
		Symbol:        "BTC",
		Network:       chain.Testnet,
		UTXOs:         utxos,
		ChangeAddress: "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		SwapAddress:   "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
		SwapAmount:    50000,
		FeeRate:       10,
	}

	tx, err := BuildFundingTx(params)
	if err != nil {
		t.Fatalf("BuildFundingTx failed: %v", err)
	}

	// Serialize
	hexStr, err := SerializeTx(tx)
	if err != nil {
		t.Fatalf("SerializeTx failed: %v", err)
	}

	// Deserialize
	txBack, err := DeserializeTx(hexStr)
	if err != nil {
		t.Fatalf("DeserializeTx failed: %v", err)
	}

	// Verify they match
	if txBack.TxHash() != tx.TxHash() {
		t.Error("deserialized tx hash doesn't match original")
	}
}

func TestDeserializeTxInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not hex", "not-valid-hex!"},
		{"truncated", "0100000001"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeTx(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
