package swap

import (
	"context"
	"testing"
	"time"

	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

func TestNewCoordinator(t *testing.T) {
	cfg := &CoordinatorConfig{
		Network: chain.Testnet,
	}

	coord := NewCoordinator(cfg)
	if coord == nil {
		t.Fatal("NewCoordinator returned nil")
	}

	if coord.network != chain.Testnet {
		t.Errorf("network = %v, want %v", coord.network, chain.Testnet)
	}

	if coord.swaps == nil {
		t.Error("swaps map not initialized")
	}

	if coord.eventHandlers == nil {
		t.Error("eventHandlers not initialized")
	}

	// Close coordinator
	if err := coord.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestCoordinatorEventHandlers(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	// Track events received
	var receivedEvents []SwapEvent
	eventCh := make(chan SwapEvent, 10)

	coord.OnEvent(func(event SwapEvent) {
		eventCh <- event
	})

	// Emit test event
	coord.emitEvent("test-trade", "test_event", map[string]string{"key": "value"})

	// Wait for event
	select {
	case event := <-eventCh:
		receivedEvents = append(receivedEvents, event)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.TradeID != "test-trade" {
		t.Errorf("TradeID = %s, want test-trade", event.TradeID)
	}
	if event.EventType != "test_event" {
		t.Errorf("EventType = %s, want test_event", event.EventType)
	}
}

func TestInitiateSwapWithoutBackend(t *testing.T) {
	// MuSig2 swaps use ephemeral keys, so wallet is not required for init.
	// However, backends ARE required to get block heights.
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 5000000,
		Method:        MethodMuSig2,
	}

	_, err := coord.InitiateSwap(context.Background(), "trade-1", "order-1", offer, MethodMuSig2)
	if err == nil {
		t.Error("InitiateSwap without backend: expected error, got nil")
	}
	// Should fail because no backend configured for BTC
	if err != nil && err.Error() != "failed to get offer chain height: backend not available for chain: BTC" {
		t.Logf("InitiateSwap without backend: got expected error type: %v", err)
	}
}

func TestRespondToSwapWithoutBackend(t *testing.T) {
	// MuSig2 swaps use ephemeral keys, so wallet is not required for respond.
	// However, backends ARE required to get block heights.
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	offer := Offer{
		OfferChain:    "BTC",
		OfferAmount:   100000,
		RequestChain:  "LTC",
		RequestAmount: 5000000,
		Method:        MethodMuSig2,
	}

	// Need a valid pubkey for RespondToSwap
	remotePubKey := make([]byte, 33)
	remotePubKey[0] = 0x02 // Compressed pubkey prefix

	_, err := coord.RespondToSwap(context.Background(), "trade-1", offer, remotePubKey, nil, MethodMuSig2)
	if err == nil {
		t.Error("RespondToSwap without backend: expected error, got nil")
	}
	// Should fail because invalid pubkey or no backend
	t.Logf("RespondToSwap without backend: got expected error: %v", err)
}

func TestGetSwapNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.GetSwap("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("GetSwap(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestGetLocalPubKeyNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.GetLocalPubKey("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("GetLocalPubKey(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestGetTaprootAddressNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.GetTaprootAddress("nonexistent", "BTC")
	if err != ErrSwapNotFound {
		t.Errorf("GetTaprootAddress(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestGenerateNoncesNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, _, err := coord.GenerateNonces("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("GenerateNonces(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestSetRemoteNoncesNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.SetRemoteNonces("nonexistent", make([]byte, 66), make([]byte, 66))
	if err != ErrSwapNotFound {
		t.Errorf("SetRemoteNonces(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestSetRemoteNoncesInvalidSize(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	// Add a mock swap to test invalid nonce size
	coord.mu.Lock()
	coord.swaps["trade-1"] = &ActiveSwap{
		MuSig2: &MuSig2SwapData{
			OfferChain:   &ChainMuSig2Data{Session: &MuSig2Session{}},
			RequestChain: &ChainMuSig2Data{Session: &MuSig2Session{}},
		},
	}
	coord.mu.Unlock()

	err := coord.SetRemoteNonces("trade-1", make([]byte, 32), make([]byte, 66)) // Wrong offer nonce size
	if err == nil {
		t.Error("SetRemoteNonces with wrong offer nonce size should error")
	}

	err = coord.SetRemoteNonces("trade-1", make([]byte, 66), make([]byte, 32)) // Wrong request nonce size
	if err == nil {
		t.Error("SetRemoteNonces with wrong request nonce size should error")
	}
}

func TestSetRemotePubKeyNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.SetRemotePubKey("nonexistent", make([]byte, 33))
	if err != ErrSwapNotFound {
		t.Errorf("SetRemotePubKey(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestSetFundingTxNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.SetFundingTx("nonexistent", "txid", 0, true)
	if err != ErrSwapNotFound {
		t.Errorf("SetFundingTx(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestCompleteSwapNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.CompleteSwap("nonexistent", "txid")
	if err != ErrSwapNotFound {
		t.Errorf("CompleteSwap(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestRefundSwapNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.RefundSwap(context.Background(), "nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("RefundSwap(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestGetSecretHashNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.GetSecretHash("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("GetSecretHash(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestRevealSecretNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.RevealSecret("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("RevealSecret(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestCreatePartialSignaturesNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, _, err := coord.CreatePartialSignatures(context.Background(), "nonexistent", make([]byte, 32), make([]byte, 32))
	if err != ErrSwapNotFound {
		t.Errorf("CreatePartialSignatures(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestCombineSignaturesNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.CombineSignatures("nonexistent", "BTC", make([]byte, 32))
	if err != ErrSwapNotFound {
		t.Errorf("CombineSignatures(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestUpdateConfirmationsNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	err := coord.UpdateConfirmations(context.Background(), "nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("UpdateConfirmations(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestCreateFundingTxNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.CreateFundingTx(context.Background(), "nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("CreateFundingTx(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestGetMuSig2StorageDataNotFound(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	_, err := coord.GetMuSig2StorageData("nonexistent")
	if err != ErrSwapNotFound {
		t.Errorf("GetMuSig2StorageData(nonexistent): got %v, want ErrSwapNotFound", err)
	}
}

func TestSetBackend(t *testing.T) {
	coord := NewCoordinator(&CoordinatorConfig{
		Network: chain.Testnet,
	})
	defer coord.Close()

	// Initially no backends
	if len(coord.backends) != 0 {
		t.Errorf("expected 0 backends initially, got %d", len(coord.backends))
	}

	// SetBackend with nil backends map should still work
	coord.backends = nil
	coord.SetBackend("BTC", nil)

	if coord.backends == nil {
		t.Error("backends map should be initialized after SetBackend")
	}
}
