package swap

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/Klingon-tech/klingdex/internal/chain"
	"github.com/Klingon-tech/klingdex/internal/config"
)

func TestNewChainConfig(t *testing.T) {
	tests := []struct {
		name            string
		symbol          string
		network         chain.Network
		wantTaproot     bool
		wantErr         bool
	}{
		{
			name:        "BTC testnet supports taproot",
			symbol:      "BTC",
			network:     chain.Testnet,
			wantTaproot: true,
			wantErr:     false,
		},
		{
			name:        "LTC testnet supports taproot",
			symbol:      "LTC",
			network:     chain.Testnet,
			wantTaproot: true,
			wantErr:     false,
		},
		{
			name:        "DOGE does not support taproot",
			symbol:      "DOGE",
			network:     chain.Mainnet,
			wantTaproot: false,
			wantErr:     false,
		},
		{
			name:    "unsupported chain",
			symbol:  "INVALID",
			network: chain.Testnet,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewChainConfig(tt.symbol, tt.network)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SupportsTaproot != tt.wantTaproot {
				t.Errorf("SupportsTaproot = %v, want %v", cfg.SupportsTaproot, tt.wantTaproot)
			}
		})
	}
}

func TestChainConfigSupportsMethod(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		network chain.Network
		method  Method
		want    bool
	}{
		{
			name:    "BTC supports MuSig2",
			symbol:  "BTC",
			network: chain.Testnet,
			method:  MethodMuSig2,
			want:    true,
		},
		{
			name:    "BTC supports HTLC",
			symbol:  "BTC",
			network: chain.Testnet,
			method:  MethodHTLC,
			want:    true,
		},
		{
			name:    "DOGE does not support MuSig2",
			symbol:  "DOGE",
			network: chain.Mainnet,
			method:  MethodMuSig2,
			want:    false,
		},
		{
			name:    "DOGE supports HTLC",
			symbol:  "DOGE",
			network: chain.Mainnet,
			method:  MethodHTLC,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewChainConfig(tt.symbol, tt.network)
			if err != nil {
				t.Fatalf("NewChainConfig failed: %v", err)
			}
			got := cfg.SupportsMethod(tt.method)
			if got != tt.want {
				t.Errorf("SupportsMethod(%s) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestOfferValidation(t *testing.T) {
	tests := []struct {
		name    string
		offer   Offer
		network chain.Network
		wantErr bool
	}{
		{
			name: "valid BTC-LTC offer",
			offer: Offer{
				OfferChain:    "BTC",
				OfferAmount:   100000, // 0.001 BTC
				RequestChain:  "LTC",
				RequestAmount: 1000000, // 0.01 LTC
				Method:        MethodMuSig2,
				ExpiresAt:     time.Now().Add(time.Hour),
			},
			network: chain.Testnet,
			wantErr: false,
		},
		{
			name: "amount below minimum",
			offer: Offer{
				OfferChain:    "BTC",
				OfferAmount:   100, // Too small
				RequestChain:  "LTC",
				RequestAmount: 1000000,
				Method:        MethodMuSig2,
				ExpiresAt:     time.Now().Add(time.Hour),
			},
			network: chain.Testnet,
			wantErr: true,
		},
		{
			name: "unsupported offer chain",
			offer: Offer{
				OfferChain:    "INVALID",
				OfferAmount:   100000,
				RequestChain:  "LTC",
				RequestAmount: 1000000,
				Method:        MethodMuSig2,
				ExpiresAt:     time.Now().Add(time.Hour),
			},
			network: chain.Testnet,
			wantErr: true,
		},
		{
			name: "method not supported on chain",
			offer: Offer{
				OfferChain:    "DOGE",
				OfferAmount:   100000000, // 1 DOGE
				RequestChain:  "BTC",
				RequestAmount: 100000,
				Method:        MethodMuSig2, // DOGE doesn't support MuSig2
				ExpiresAt:     time.Now().Add(time.Hour),
			},
			network: chain.Mainnet,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.offer.Validate(tt.network)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewSwap(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, err := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	if err != nil {
		t.Fatalf("NewSwap failed: %v", err)
	}

	if swap.ID == "" {
		t.Error("swap ID should not be empty")
	}
	if swap.State != StateInit {
		t.Errorf("initial state should be StateInit, got %s", swap.State)
	}
	if swap.Role != RoleInitiator {
		t.Errorf("role should be RoleInitiator, got %s", swap.Role)
	}

	// Check timeouts from config
	swapCfg := config.DefaultSwapConfig()
	if swap.InitiatorLock != swapCfg.InitiatorLockTime {
		t.Errorf("InitiatorLock = %v, want %v", swap.InitiatorLock, swapCfg.InitiatorLockTime)
	}
	if swap.ResponderLock != swapCfg.ResponderLockTime {
		t.Errorf("ResponderLock = %v, want %v", swap.ResponderLock, swapCfg.ResponderLockTime)
	}
}

func TestSwapStateTransitions(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)

	// Valid transitions
	tests := []struct {
		from    State
		to      State
		wantErr bool
	}{
		{StateInit, StateFunding, false},
		{StateFunding, StateFunded, false},
		{StateFunded, StateRedeemed, false},
	}

	for _, tt := range tests {
		swap.State = tt.from
		err := swap.TransitionTo(tt.to)
		if tt.wantErr && err == nil {
			t.Errorf("transition %s -> %s: expected error", tt.from, tt.to)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("transition %s -> %s: unexpected error: %v", tt.from, tt.to, err)
		}
	}

	// Invalid transitions
	swap.State = StateRedeemed
	err := swap.TransitionTo(StateInit)
	if err == nil {
		t.Error("should not allow transition from terminal state")
	}
}

func TestSwapPubKeyHandling(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)

	// Generate test key
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKey := privKey.PubKey()

	// Set local key
	swap.SetLocalPubKey(pubKey)

	// Get it back
	gotPubKey, err := swap.GetLocalPubKey()
	if err != nil {
		t.Fatalf("GetLocalPubKey failed: %v", err)
	}

	if !pubKey.IsEqual(gotPubKey) {
		t.Error("retrieved public key doesn't match original")
	}

	// Test remote key
	remotePrivKey, _ := btcec.NewPrivateKey()
	err = swap.SetRemotePubKey(remotePrivKey.PubKey())
	if err != nil {
		t.Errorf("SetRemotePubKey failed: %v", err)
	}

	gotRemote, err := swap.GetRemotePubKey()
	if err != nil {
		t.Fatalf("GetRemotePubKey failed: %v", err)
	}
	if !remotePrivKey.PubKey().IsEqual(gotRemote) {
		t.Error("retrieved remote public key doesn't match original")
	}
}

func TestSwapSecretGeneration(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	// Initiator can generate secret
	initiator, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	err := initiator.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	if len(initiator.Secret) != 32 {
		t.Errorf("secret length = %d, want 32", len(initiator.Secret))
	}
	if len(initiator.SecretHash) != 32 {
		t.Errorf("secret hash length = %d, want 32", len(initiator.SecretHash))
	}

	// Verify secret
	if !initiator.VerifySecret(initiator.Secret) {
		t.Error("secret should verify against its hash")
	}

	// Wrong secret should not verify
	wrongSecret := make([]byte, 32)
	if initiator.VerifySecret(wrongSecret) {
		t.Error("wrong secret should not verify")
	}

	// Responder cannot generate secret
	responder, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleResponder, offer)
	err = responder.GenerateSecret()
	if err == nil {
		t.Error("responder should not be able to generate secret")
	}
}

func TestSwapTerminalStates(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)

	terminalStates := []State{StateRedeemed, StateRefunded, StateFailed, StateCancelled}
	nonTerminalStates := []State{StateInit, StateFunding, StateFunded}

	for _, state := range terminalStates {
		swap.State = state
		if !swap.IsTerminal() {
			t.Errorf("%s should be terminal", state)
		}
	}

	for _, state := range nonTerminalStates {
		swap.State = state
		if swap.IsTerminal() {
			t.Errorf("%s should not be terminal", state)
		}
	}
}

func TestHashSecret(t *testing.T) {
	secret := []byte("test secret that is exactly 32 b")
	hash := HashSecret(secret)

	if len(hash) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash))
	}

	// Same secret should produce same hash
	hash2 := HashSecret(secret)
	for i := range hash {
		if hash[i] != hash2[i] {
			t.Error("same secret should produce same hash")
			break
		}
	}

	// Different secret should produce different hash
	differentSecret := []byte("different secret xxxxxxxxxxxxx")
	hash3 := HashSecret(differentSecret)
	same := true
	for i := range hash {
		if hash[i] != hash3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different secrets should produce different hashes")
	}
}

func TestComputeSwapID(t *testing.T) {
	privKey1, _ := btcec.NewPrivateKey()
	privKey2, _ := btcec.NewPrivateKey()
	pubKey1 := privKey1.PubKey()
	pubKey2 := privKey2.PubKey()

	// ID should be deterministic
	id1 := ComputeSwapID(pubKey1, pubKey2)
	id2 := ComputeSwapID(pubKey1, pubKey2)
	if id1 != id2 {
		t.Error("swap ID should be deterministic")
	}

	// Order shouldn't matter
	id3 := ComputeSwapID(pubKey2, pubKey1)
	if id1 != id3 {
		t.Error("swap ID should be order-independent")
	}

	// Different keys should produce different ID
	privKey3, _ := btcec.NewPrivateKey()
	id4 := ComputeSwapID(pubKey1, privKey3.PubKey())
	if id1 == id4 {
		t.Error("different keys should produce different swap ID")
	}
}

// =============================================================================
// Safety Margin and Timeout Tests
// =============================================================================

func TestSetBlockHeights(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	// Test initiator
	initiator, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	initiator.SetBlockHeights(100000, 50000) // BTC at 100000, LTC at 50000

	// Initiator has longer timeout on offer chain (BTC)
	btcTestTimeout, _ := config.GetChainTimeout("BTC", true)
	ltcTestTimeout, _ := config.GetChainTimeout("LTC", true)

	expectedBTCTimeout := uint32(100000) + btcTestTimeout.MakerBlocks
	expectedLTCTimeout := uint32(50000) + ltcTestTimeout.TakerBlocks

	if initiator.OfferChainTimeoutHeight != expectedBTCTimeout {
		t.Errorf("OfferChainTimeoutHeight = %d, want %d", initiator.OfferChainTimeoutHeight, expectedBTCTimeout)
	}
	if initiator.RequestChainTimeoutHeight != expectedLTCTimeout {
		t.Errorf("RequestChainTimeoutHeight = %d, want %d", initiator.RequestChainTimeoutHeight, expectedLTCTimeout)
	}

	// Test responder (opposite timeouts)
	responder, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleResponder, offer)
	responder.SetBlockHeights(100000, 50000)

	expectedBTCTimeoutResp := uint32(100000) + btcTestTimeout.TakerBlocks
	expectedLTCTimeoutResp := uint32(50000) + ltcTestTimeout.MakerBlocks

	if responder.OfferChainTimeoutHeight != expectedBTCTimeoutResp {
		t.Errorf("Responder OfferChainTimeoutHeight = %d, want %d", responder.OfferChainTimeoutHeight, expectedBTCTimeoutResp)
	}
	if responder.RequestChainTimeoutHeight != expectedLTCTimeoutResp {
		t.Errorf("Responder RequestChainTimeoutHeight = %d, want %d", responder.RequestChainTimeoutHeight, expectedLTCTimeoutResp)
	}
}

func TestIsSafeToComplete(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	swap.SetBlockHeights(100000, 50000)

	btcTimeout, _ := config.GetChainTimeout("BTC", true)

	tests := []struct {
		name            string
		btcHeight       uint32
		ltcHeight       uint32
		expectSafe      bool
	}{
		{
			name:       "well before timeout - safe",
			btcHeight:  100005,
			ltcHeight:  50005,
			expectSafe: true,
		},
		{
			name:       "at start - safe",
			btcHeight:  100000,
			ltcHeight:  50000,
			expectSafe: true,
		},
		{
			name:       "within safety margin - not safe",
			btcHeight:  swap.OfferChainTimeoutHeight - btcTimeout.SafetyMarginBlocks + 1,
			ltcHeight:  50005,
			expectSafe: false,
		},
		{
			name:       "past timeout - not safe",
			btcHeight:  swap.OfferChainTimeoutHeight + 1,
			ltcHeight:  50005,
			expectSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := swap.IsSafeToComplete(tt.btcHeight, tt.ltcHeight)
			if tt.expectSafe && err != nil {
				t.Errorf("expected safe, got error: %v", err)
			}
			if !tt.expectSafe && err == nil {
				t.Error("expected not safe, got nil error")
			}
		})
	}
}

func TestCanRefundByBlock(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	// Initiator checks offer chain
	initiator, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	initiator.SetBlockHeights(100000, 50000)

	// Before timeout
	if initiator.CanRefundByBlock(100005, 50005) {
		t.Error("should not be able to refund before timeout")
	}

	// At timeout
	if !initiator.CanRefundByBlock(initiator.OfferChainTimeoutHeight, 50005) {
		t.Error("should be able to refund at timeout")
	}

	// After timeout
	if !initiator.CanRefundByBlock(initiator.OfferChainTimeoutHeight+10, 50005) {
		t.Error("should be able to refund after timeout")
	}

	// Responder checks request chain
	responder, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleResponder, offer)
	responder.SetBlockHeights(100000, 50000)

	// Before timeout
	if responder.CanRefundByBlock(100005, 50005) {
		t.Error("responder should not be able to refund before timeout")
	}

	// At timeout
	if !responder.CanRefundByBlock(100005, responder.RequestChainTimeoutHeight) {
		t.Error("responder should be able to refund at timeout")
	}
}

func TestBlocksUntilRefund(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)
	swap.SetBlockHeights(100000, 50000)

	// At start
	blocksLeft := swap.BlocksUntilRefund(100000)
	btcTimeout, _ := config.GetChainTimeout("BTC", true)
	if blocksLeft != btcTimeout.MakerBlocks {
		t.Errorf("BlocksUntilRefund at start = %d, want %d", blocksLeft, btcTimeout.MakerBlocks)
	}

	// Partway through
	blocksLeft = swap.BlocksUntilRefund(100005)
	if blocksLeft != btcTimeout.MakerBlocks-5 {
		t.Errorf("BlocksUntilRefund = %d, want %d", blocksLeft, btcTimeout.MakerBlocks-5)
	}

	// At timeout
	blocksLeft = swap.BlocksUntilRefund(swap.OfferChainTimeoutHeight)
	if blocksLeft != 0 {
		t.Errorf("BlocksUntilRefund at timeout = %d, want 0", blocksLeft)
	}

	// Past timeout
	blocksLeft = swap.BlocksUntilRefund(swap.OfferChainTimeoutHeight + 10)
	if blocksLeft != 0 {
		t.Errorf("BlocksUntilRefund past timeout = %d, want 0", blocksLeft)
	}
}

// =============================================================================
// Confirmation Tracking Tests
// =============================================================================

func TestFundingStatus(t *testing.T) {
	// Use Mainnet for this test since testnet has MinConfirmations=0
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Mainnet, MethodMuSig2, RoleInitiator, offer)

	// No funding tx set yet
	status := swap.GetLocalFundingStatus()
	if status != nil {
		t.Error("should return nil when no funding tx is set")
	}

	// Set funding tx
	swap.LocalFundingTxID = "abc123"
	swap.LocalFundingConfirms = 0

	status = swap.GetLocalFundingStatus()
	if status == nil {
		t.Fatal("status should not be nil after setting tx")
	}

	btcTimeout, _ := config.GetChainTimeout("BTC", false) // Mainnet
	if status.Required != btcTimeout.MinConfirmations {
		t.Errorf("Required = %d, want %d", status.Required, btcTimeout.MinConfirmations)
	}
	if status.IsFinal {
		t.Error("should not be final with 0 confirmations")
	}

	// Update confirmations
	swap.UpdateLocalConfirmations(btcTimeout.MinConfirmations)
	status = swap.GetLocalFundingStatus()
	if !status.IsFinal {
		t.Error("should be final with sufficient confirmations")
	}
}

func TestIsFundingConfirmed(t *testing.T) {
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Testnet, MethodMuSig2, RoleInitiator, offer)

	// Not confirmed without funding txs
	if swap.IsFundingConfirmed() {
		t.Error("should not be confirmed without funding txs")
	}

	// Set local funding
	swap.LocalFundingTxID = "abc123"
	btcTimeout, _ := config.GetChainTimeout("BTC", true)
	swap.UpdateLocalConfirmations(btcTimeout.MinConfirmations)

	// Still not confirmed without remote
	if swap.IsFundingConfirmed() {
		t.Error("should not be confirmed without remote funding tx")
	}

	// Set remote funding
	swap.RemoteFundingTxID = "def456"
	ltcTimeout, _ := config.GetChainTimeout("LTC", true)
	swap.UpdateRemoteConfirmations(ltcTimeout.MinConfirmations)

	// Now confirmed
	if !swap.IsFundingConfirmed() {
		t.Error("should be confirmed with both funding txs having sufficient confirmations")
	}
}

func TestCheckConfirmations(t *testing.T) {
	// Use Mainnet for this test since testnet has MinConfirmations=0
	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 1000000,
		Method:        MethodMuSig2,
		ExpiresAt:     time.Now().Add(time.Hour),
	}

	swap, _ := NewSwap(chain.Mainnet, MethodMuSig2, RoleInitiator, offer)

	// No funding tx
	err := swap.CheckConfirmations()
	if err == nil {
		t.Error("should error without local funding tx")
	}

	// Set local with insufficient confirmations
	swap.LocalFundingTxID = "abc123"
	swap.UpdateLocalConfirmations(0)

	err = swap.CheckConfirmations()
	if err == nil {
		t.Error("should error with insufficient local confirmations")
	}

	// Set local with sufficient confirmations
	btcTimeout, _ := config.GetChainTimeout("BTC", false) // Mainnet
	swap.UpdateLocalConfirmations(btcTimeout.MinConfirmations)

	err = swap.CheckConfirmations()
	if err == nil {
		t.Error("should error without remote funding tx")
	}

	// Set remote with insufficient confirmations
	swap.RemoteFundingTxID = "def456"
	swap.UpdateRemoteConfirmations(0)

	err = swap.CheckConfirmations()
	if err == nil {
		t.Error("should error with insufficient remote confirmations")
	}

	// Set remote with sufficient confirmations
	ltcTimeout, _ := config.GetChainTimeout("LTC", false) // Mainnet
	swap.UpdateRemoteConfirmations(ltcTimeout.MinConfirmations)

	err = swap.CheckConfirmations()
	if err != nil {
		t.Errorf("should succeed with sufficient confirmations: %v", err)
	}
}
