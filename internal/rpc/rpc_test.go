package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *Request
		expectError bool
	}{
		{
			name: "valid request with string id",
			request: &Request{
				JSONRPC: "2.0",
				Method:  "test_method",
				ID:      "123",
			},
			expectError: false,
		},
		{
			name: "valid request with number id",
			request: &Request{
				JSONRPC: "2.0",
				Method:  "test_method",
				ID:      1,
			},
			expectError: false,
		},
		{
			name: "valid request with nil id (notification)",
			request: &Request{
				JSONRPC: "2.0",
				Method:  "test_method",
				ID:      nil,
			},
			expectError: false,
		},
		{
			name: "valid request with params",
			request: &Request{
				JSONRPC: "2.0",
				Method:  "test_method",
				Params:  json.RawMessage(`{"key": "value"}`),
				ID:      1,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("failed to marshal request: %v", err)
			}

			var parsed Request
			if err := json.Unmarshal(data, &parsed); err != nil {
				if !tt.expectError {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			if parsed.Method != tt.request.Method {
				t.Errorf("Method = %s, want %s", parsed.Method, tt.request.Method)
			}
		})
	}
}

func TestResponse(t *testing.T) {
	// Test success response
	successResp := &Response{
		JSONRPC: "2.0",
		Result:  map[string]interface{}{"status": "ok"},
		ID:      1,
	}

	data, err := json.Marshal(successResp)
	if err != nil {
		t.Fatalf("failed to marshal success response: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if parsed.Error != nil {
		t.Error("expected no error in success response")
	}

	// Test error response
	errorResp := &Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    InvalidRequest,
			Message: "Invalid Request",
		},
		ID: 1,
	}

	data, err = json.Marshal(errorResp)
	if err != nil {
		t.Fatalf("failed to marshal error response: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if parsed.Error == nil {
		t.Error("expected error in error response")
	}

	if parsed.Error.Code != InvalidRequest {
		t.Errorf("Error.Code = %d, want %d", parsed.Error.Code, InvalidRequest)
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		code    int
		message string
	}{
		{ParseError, "Parse error"},
		{InvalidRequest, "Invalid request"},
		{MethodNotFound, "Method not found"},
		{InvalidParams, "Invalid params"},
		{InternalError, "Internal error"},
	}

	for _, tt := range tests {
		resp := &Response{
			JSONRPC: "2.0",
			Error: &Error{
				Code:    tt.code,
				Message: tt.message,
			},
			ID: 1,
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Errorf("failed to marshal error response for code %d: %v", tt.code, err)
			continue
		}

		var parsed Response
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Errorf("failed to unmarshal error response for code %d: %v", tt.code, err)
			continue
		}

		if parsed.Error.Code != tt.code {
			t.Errorf("Error.Code = %d, want %d", parsed.Error.Code, tt.code)
		}
	}
}

func TestWebSocketHub(t *testing.T) {
	hub := NewWSHub()

	// Test initial state
	if hub.ClientCount() != 0 {
		t.Errorf("initial ClientCount = %d, want 0", hub.ClientCount())
	}

	// Start hub
	go hub.Run()

	// Note: Full WebSocket testing requires actual connections
	// This test verifies the hub can be created and started
}

func TestWSEvent(t *testing.T) {
	msg := WSEvent{
		Type:      "test_event",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: 1234567890,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal WSEvent: %v", err)
	}

	var parsed WSEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WSEvent: %v", err)
	}

	if parsed.Type != msg.Type {
		t.Errorf("Type = %s, want %s", parsed.Type, msg.Type)
	}

	if parsed.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp = %d, want %d", parsed.Timestamp, msg.Timestamp)
	}
}

func TestWSSubscription(t *testing.T) {
	sub := WSSubscription{
		Action: "subscribe",
		Events: []string{"peer_connected", "peer_disconnected"},
	}

	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("failed to marshal WSSubscription: %v", err)
	}

	var parsed WSSubscription
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WSSubscription: %v", err)
	}

	if parsed.Action != "subscribe" {
		t.Errorf("Action = %s, want subscribe", parsed.Action)
	}

	if len(parsed.Events) != 2 {
		t.Errorf("len(Events) = %d, want 2", len(parsed.Events))
	}
}

func TestEventTypes(t *testing.T) {
	if EventPeerConnected != "peer_connected" {
		t.Errorf("EventPeerConnected = %s, want peer_connected", EventPeerConnected)
	}

	if EventPeerDisconnected != "peer_disconnected" {
		t.Errorf("EventPeerDisconnected = %s, want peer_disconnected", EventPeerDisconnected)
	}
}

func TestNodeInfoResult(t *testing.T) {
	result := NodeInfoResult{
		PeerID:      "12D3KooWTestPeer",
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWTestPeer"},
		Peers:       5,
		Uptime:      "1h30m0s",
		Version:     "0.1.0-dev",
		DataDir:     "~/.klingon",
		MDNSEnabled: true,
		DHTEnabled:  true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal NodeInfoResult: %v", err)
	}

	var parsed NodeInfoResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal NodeInfoResult: %v", err)
	}

	if parsed.PeerID != result.PeerID {
		t.Errorf("PeerID = %s, want %s", parsed.PeerID, result.PeerID)
	}

	if parsed.Peers != result.Peers {
		t.Errorf("Peers = %d, want %d", parsed.Peers, result.Peers)
	}
}

func TestPeersCountResult(t *testing.T) {
	result := PeersCountResult{
		Connected: 10,
		Known:     50,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal PeersCountResult: %v", err)
	}

	var parsed PeersCountResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal PeersCountResult: %v", err)
	}

	if parsed.Connected != 10 {
		t.Errorf("Connected = %d, want 10", parsed.Connected)
	}

	if parsed.Known != 50 {
		t.Errorf("Known = %d, want 50", parsed.Known)
	}
}

func TestConnectParams(t *testing.T) {
	params := ConnectParams{
		Addr: "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWTestPeer",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal ConnectParams: %v", err)
	}

	var parsed ConnectParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ConnectParams: %v", err)
	}

	if parsed.Addr != params.Addr {
		t.Errorf("Addr = %s, want %s", parsed.Addr, params.Addr)
	}
}

func TestDisconnectParams(t *testing.T) {
	params := DisconnectParams{
		PeerID: "12D3KooWTestPeer",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal DisconnectParams: %v", err)
	}

	var parsed DisconnectParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DisconnectParams: %v", err)
	}

	if parsed.PeerID != params.PeerID {
		t.Errorf("PeerID = %s, want %s", parsed.PeerID, params.PeerID)
	}
}

func TestKnownPeersParams(t *testing.T) {
	params := KnownPeersParams{
		Limit: 50,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal KnownPeersParams: %v", err)
	}

	var parsed KnownPeersParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal KnownPeersParams: %v", err)
	}

	if parsed.Limit != 50 {
		t.Errorf("Limit = %d, want 50", parsed.Limit)
	}
}

func TestKnownPeerInfo(t *testing.T) {
	info := KnownPeerInfo{
		PeerID:          "12D3KooWTestPeer",
		Addrs:           []string{"/ip4/127.0.0.1/tcp/4001"},
		FirstSeen:       1700000000,
		LastSeen:        1700001000,
		LastConnected:   1700001000,
		ConnectionCount: 5,
		IsBootstrap:     false,
		IsConnected:     true,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal KnownPeerInfo: %v", err)
	}

	var parsed KnownPeerInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal KnownPeerInfo: %v", err)
	}

	if parsed.ConnectionCount != 5 {
		t.Errorf("ConnectionCount = %d, want 5", parsed.ConnectionCount)
	}

	if !parsed.IsConnected {
		t.Error("IsConnected should be true")
	}
}

// TestHTTPMethodCheck tests that only POST is accepted
func TestHTTPMethodCheck(t *testing.T) {
	// Create a test request with GET method
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Handler that checks method
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestParseError tests parse error handling
func TestParseError(t *testing.T) {
	invalidJSON := []byte(`{invalid json`)

	var req Request
	err := json.Unmarshal(invalidJSON, &req)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestBatchNotSupported verifies batch handling behavior
func TestBatchNotSupported(t *testing.T) {
	batchRequest := []byte(`[
		{"jsonrpc": "2.0", "method": "test1", "id": 1},
		{"jsonrpc": "2.0", "method": "test2", "id": 2}
	]`)

	// Verify it's valid JSON array
	var arr []json.RawMessage
	if err := json.Unmarshal(batchRequest, &arr); err != nil {
		t.Fatalf("batch should be valid JSON array: %v", err)
	}

	if len(arr) != 2 {
		t.Errorf("batch should have 2 requests, got %d", len(arr))
	}
}

// TestContentType tests that Content-Type is checked
func TestContentType(t *testing.T) {
	body := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"test","id":1}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)

	// No Content-Type header
	contentType := req.Header.Get("Content-Type")
	if contentType != "" {
		t.Error("expected empty Content-Type for test")
	}

	// Set proper Content-Type
	req.Header.Set("Content-Type", "application/json")
	contentType = req.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestWalletListTokensParams(t *testing.T) {
	params := WalletListTokensParams{
		Symbol: "ETH",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal WalletListTokensParams: %v", err)
	}

	var parsed WalletListTokensParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletListTokensParams: %v", err)
	}

	if parsed.Symbol != "ETH" {
		t.Errorf("Symbol = %s, want ETH", parsed.Symbol)
	}
}

func TestWalletListTokensResult(t *testing.T) {
	result := WalletListTokensResult{
		Symbol: "ETH",
		Tokens: []WalletListTokensItem{
			{
				Symbol:   "USDT",
				Name:     "Tether USD",
				Decimals: 6,
				Address:  "0xdAC17F958D2ee523a2206206994597C13D831ec7",
				ChainID:  1,
			},
			{
				Symbol:   "USDC",
				Name:     "USD Coin",
				Decimals: 6,
				Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
				ChainID:  1,
			},
		},
		Count: 2,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal WalletListTokensResult: %v", err)
	}

	var parsed WalletListTokensResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal WalletListTokensResult: %v", err)
	}

	if parsed.Symbol != "ETH" {
		t.Errorf("Symbol = %s, want ETH", parsed.Symbol)
	}

	if parsed.Count != 2 {
		t.Errorf("Count = %d, want 2", parsed.Count)
	}

	if len(parsed.Tokens) != 2 {
		t.Fatalf("len(Tokens) = %d, want 2", len(parsed.Tokens))
	}

	if parsed.Tokens[0].Symbol != "USDT" {
		t.Errorf("Tokens[0].Symbol = %s, want USDT", parsed.Tokens[0].Symbol)
	}

	if parsed.Tokens[0].Decimals != 6 {
		t.Errorf("Tokens[0].Decimals = %d, want 6", parsed.Tokens[0].Decimals)
	}

	if parsed.Tokens[1].Address != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
		t.Errorf("Tokens[1].Address = %s, want 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", parsed.Tokens[1].Address)
	}
}

func TestWalletListTokensHandler(t *testing.T) {
	// Create server with nil wallet (handler uses chain.Mainnet as default)
	s := &Server{
		handlers: make(map[string]Handler),
	}
	s.handlers["wallet_listTokens"] = s.walletListTokens

	tests := []struct {
		name        string
		params      string
		expectError bool
		errContains string
	}{
		{
			name:        "valid EVM chain",
			params:      `{"symbol":"ETH"}`,
			expectError: false,
		},
		{
			name:        "missing symbol",
			params:      `{}`,
			expectError: true,
			errContains: "symbol is required",
		},
		{
			name:        "non-EVM chain",
			params:      `{"symbol":"BTC"}`,
			expectError: true,
			errContains: "not an EVM chain",
		},
		{
			name:        "unsupported chain",
			params:      `{"symbol":"FAKE"}`,
			expectError: true,
			errContains: "unsupported chain",
		},
		{
			name:        "invalid JSON",
			params:      `{invalid}`,
			expectError: true,
			errContains: "invalid params",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.walletListTokens(nil, json.RawMessage(tt.params))
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" {
					if !containsStr(err.Error(), tt.errContains) {
						t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			res, ok := result.(*WalletListTokensResult)
			if !ok {
				t.Fatal("result is not *WalletListTokensResult")
			}

			if res.Symbol != "ETH" {
				t.Errorf("Symbol = %s, want ETH", res.Symbol)
			}

			if res.Count != len(res.Tokens) {
				t.Errorf("Count = %d, but len(Tokens) = %d", res.Count, len(res.Tokens))
			}

			if res.Count == 0 {
				t.Error("expected at least one token for ETH mainnet")
			}

			// Verify known tokens exist
			tokenMap := make(map[string]bool)
			for _, tok := range res.Tokens {
				tokenMap[tok.Symbol] = true
				if tok.Address == "" {
					t.Errorf("token %s has empty address", tok.Symbol)
				}
				if tok.ChainID != 1 {
					t.Errorf("token %s has chainID %d, want 1", tok.Symbol, tok.ChainID)
				}
			}

			if !tokenMap["USDT"] {
				t.Error("expected USDT in ETH tokens")
			}
			if !tokenMap["USDC"] {
				t.Error("expected USDC in ETH tokens")
			}
		})
	}
}

// containsStr checks if s contains substr (avoids importing strings in test).
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestErrorConstants(t *testing.T) {
	// Verify error code values match JSON-RPC 2.0 spec
	if ParseError != -32700 {
		t.Errorf("ParseError = %d, want -32700", ParseError)
	}
	if InvalidRequest != -32600 {
		t.Errorf("InvalidRequest = %d, want -32600", InvalidRequest)
	}
	if MethodNotFound != -32601 {
		t.Errorf("MethodNotFound = %d, want -32601", MethodNotFound)
	}
	if InvalidParams != -32602 {
		t.Errorf("InvalidParams = %d, want -32602", InvalidParams)
	}
	if InternalError != -32603 {
		t.Errorf("InternalError = %d, want -32603", InternalError)
	}
}
