// Package rpc - Type definitions for swap RPC handlers.
package rpc

// =============================================================================
// Swap Init Types
// =============================================================================

// SwapInitParams is the parameters for swap_init.
type SwapInitParams struct {
	TradeID string `json:"trade_id"`
}

// SwapInitResult is the response for swap_init.
type SwapInitResult struct {
	TradeID        string `json:"trade_id"`
	LocalPubKey    string `json:"local_pubkey"`    // Hex-encoded
	TaprootAddress string `json:"taproot_address"` // Only if remote pubkey already set
	State          string `json:"state"`
}

// =============================================================================
// Nonce Exchange Types
// =============================================================================

// SwapExchangeNonceParams is the parameters for swap_exchangeNonce.
type SwapExchangeNonceParams struct {
	TradeID string `json:"trade_id"`
}

// SwapExchangeNonceResult is the response for swap_exchangeNonce.
type SwapExchangeNonceResult struct {
	TradeID          string `json:"trade_id"`
	OfferNonce       string `json:"offer_nonce"`
	RequestNonce     string `json:"request_nonce"`
	HasRemoteNonces  bool   `json:"has_remote_nonces"`
}

// =============================================================================
// Address Types
// =============================================================================

// SwapGetAddressParams is the parameters for swap_getAddress.
type SwapGetAddressParams struct {
	TradeID string `json:"trade_id"`
}

// SwapGetAddressResult is the response for swap_getAddress.
type SwapGetAddressResult struct {
	TradeID        string `json:"trade_id"`
	TaprootAddress string `json:"taproot_address,omitempty"` // For MuSig2 (P2TR)
	HTLCAddress    string `json:"htlc_address,omitempty"`    // For HTLC (P2WSH)
	Address        string `json:"address"`                   // Generic - contains the funding address
	Chain          string `json:"chain"`
	Amount         uint64 `json:"amount"`
	Method         string `json:"method"` // "musig2" or "htlc"
}

// =============================================================================
// Funding Types
// =============================================================================

// SwapSetFundingParams is the parameters for swap_setFunding.
type SwapSetFundingParams struct {
	TradeID string `json:"trade_id"`
	TxID    string `json:"txid"`
	Vout    uint32 `json:"vout"`
}

// SwapSetFundingResult is the response for swap_setFunding.
type SwapSetFundingResult struct {
	TradeID string `json:"trade_id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// SwapCheckFundingParams is the parameters for swap_checkFunding.
type SwapCheckFundingParams struct {
	TradeID string `json:"trade_id"`
}

// SwapCheckFundingResult is the response for swap_checkFunding.
type SwapCheckFundingResult struct {
	TradeID              string `json:"trade_id"`
	LocalFunded          bool   `json:"local_funded"`
	LocalConfirmations   uint32 `json:"local_confirmations"`
	RemoteFunded         bool   `json:"remote_funded"`
	RemoteConfirmations  uint32 `json:"remote_confirmations"`
	BothFunded           bool   `json:"both_funded"`
	ReadyForNonceExchange bool  `json:"ready_for_nonce_exchange"`
	State                string `json:"state"`
}

// SwapFundParams is the parameters for swap_fund (auto-fund).
type SwapFundParams struct {
	TradeID string `json:"trade_id"`
}

// SwapFundResult is the response for swap_fund.
type SwapFundResult struct {
	TradeID      string `json:"trade_id"`
	TxID         string `json:"txid"`
	Chain        string `json:"chain"`
	Amount       uint64 `json:"amount"`
	Fee          uint64 `json:"fee"`
	EscrowVout   uint32 `json:"escrow_vout"`
	EscrowAddr   string `json:"escrow_address"`
	InputCount   int    `json:"input_count"`
	TotalInput   uint64 `json:"total_input"`
	Change       uint64 `json:"change"`
	State        string `json:"state"`
}

// =============================================================================
// Signing Types
// =============================================================================

// SwapSignParams is the parameters for swap_sign.
type SwapSignParams struct {
	TradeID string `json:"trade_id"`
}

// SwapSignResult is the response for swap_sign.
type SwapSignResult struct {
	TradeID          string `json:"trade_id"`
	OfferSigSent     bool   `json:"offer_sig_sent,omitempty"`
	RequestSigSent   bool   `json:"request_sig_sent,omitempty"`
	OfferSigReceived bool   `json:"offer_sig_received,omitempty"`
	RequestSigReceived bool `json:"request_sig_received,omitempty"`
	ReadyToRedeem    bool   `json:"ready_to_redeem"`
	State            string `json:"state"`
}

// =============================================================================
// Redeem Types
// =============================================================================

// SwapRedeemParams is the parameters for swap_redeem.
type SwapRedeemParams struct {
	TradeID string `json:"trade_id"`
}

// SwapRedeemResult is the response for swap_redeem.
type SwapRedeemResult struct {
	TradeID     string `json:"trade_id"`
	RedeemTxID  string `json:"redeem_txid"`
	RedeemChain string `json:"redeem_chain"`
	State       string `json:"state"`
	Message     string `json:"message"`
}

// =============================================================================
// Status Types
// =============================================================================

// SwapStatusParams is the parameters for swap_status.
type SwapStatusParams struct {
	TradeID string `json:"trade_id"`
}

// SwapStatusResult is the detailed status of a swap.
type SwapStatusResult struct {
	TradeID               string         `json:"trade_id"`
	State                 string         `json:"state"`
	Role                  string         `json:"role"`
	SwapType              string         `json:"swap_type,omitempty"` // "evm_to_evm", "bitcoin_to_evm", "evm_to_bitcoin", "bitcoin_to_bitcoin"
	Method                string         `json:"method,omitempty"`    // "htlc" or "musig2"
	OfferTaprootAddress   string         `json:"offer_taproot_address,omitempty"`
	RequestTaprootAddress string         `json:"request_taproot_address,omitempty"`
	OfferHTLCAddress      string         `json:"offer_htlc_address,omitempty"`   // HTLC address for offer chain (Bitcoin)
	RequestHTLCAddress    string         `json:"request_htlc_address,omitempty"` // HTLC address for request chain (Bitcoin)
	OfferEVMAddress       string         `json:"offer_evm_address,omitempty"`    // Local wallet address for offer chain (EVM)
	RequestEVMAddress     string         `json:"request_evm_address,omitempty"`  // Local wallet address for request chain (EVM)
	LocalPubKey           string         `json:"local_pubkey,omitempty"`
	RemotePubKey          string         `json:"remote_pubkey,omitempty"`
	LocalFunding          *FundingStatus `json:"local_funding,omitempty"`
	RemoteFunding         *FundingStatus `json:"remote_funding,omitempty"`
	HasOfferNonces        bool           `json:"has_offer_nonces"`
	HasRequestNonces      bool           `json:"has_request_nonces"`
	HasOfferSigs          bool           `json:"has_offer_sigs"`
	HasRequestSigs        bool           `json:"has_request_sigs"`
	ReadyToRedeem         bool           `json:"ready_to_redeem"`
}

// FundingStatus represents the status of a funding transaction.
type FundingStatus struct {
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Amount        uint64 `json:"amount"`
	Confirmations uint32 `json:"confirmations"`
	Confirmed     bool   `json:"confirmed"`
}

// =============================================================================
// List Types
// =============================================================================

// SwapListParams is the parameters for swap_list.
type SwapListParams struct {
	IncludeCompleted bool `json:"include_completed"`
}

// SwapListItem represents a swap in the list.
type SwapListItem struct {
	TradeID       string `json:"trade_id"`
	State         string `json:"state"`
	Role          string `json:"role"`
	OfferChain    string `json:"offer_chain"`
	OfferAmount   uint64 `json:"offer_amount"`
	RequestChain  string `json:"request_chain"`
	RequestAmount uint64 `json:"request_amount"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at,omitempty"`
}

// SwapListResult is the response for swap_list.
type SwapListResult struct {
	Swaps []SwapListItem `json:"swaps"`
	Count int            `json:"count"`
}

// =============================================================================
// Recovery Types
// =============================================================================

// SwapRecoverParams is the parameters for swap_recover.
type SwapRecoverParams struct {
	TradeID string `json:"trade_id"`
}

// SwapRecoverResult is the response for swap_recover.
type SwapRecoverResult struct {
	TradeID string `json:"trade_id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// =============================================================================
// Timeout and Refund Types
// =============================================================================

// SwapTimeoutParams is the parameters for swap_timeout.
type SwapTimeoutParams struct {
	TradeID string `json:"trade_id"`
}

// SwapRefundParams is the parameters for swap_refund.
type SwapRefundParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
}

// SwapRefundResult is the response for swap_refund.
type SwapRefundResult struct {
	TradeID    string `json:"trade_id"`
	RefundTxID string `json:"refund_txid"`
	Chain      string `json:"chain"`
	State      string `json:"state"`
}

// SwapCheckTimeoutsResult is the response for swap_checkTimeouts.
type SwapCheckTimeoutsResult struct {
	Results []interface{} `json:"results"`
	Count   int           `json:"count"`
}

// =============================================================================
// HTLC Types
// =============================================================================

// SwapHTLCRevealSecretParams is the parameters for swap_htlcRevealSecret.
type SwapHTLCRevealSecretParams struct {
	TradeID string `json:"trade_id"`
}

// SwapHTLCRevealSecretResult is the response for swap_htlcRevealSecret.
type SwapHTLCRevealSecretResult struct {
	TradeID    string `json:"trade_id"`
	Secret     string `json:"secret"`      // Hex-encoded secret
	SecretHash string `json:"secret_hash"` // Hex-encoded SHA256 of secret
	Message    string `json:"message"`
}

// SwapHTLCGetSecretParams is the parameters for swap_htlcGetSecret.
type SwapHTLCGetSecretParams struct {
	TradeID string `json:"trade_id"`
}

// SwapHTLCGetSecretResult is the response for swap_htlcGetSecret.
type SwapHTLCGetSecretResult struct {
	TradeID       string `json:"trade_id"`
	SecretHash    string `json:"secret_hash"` // Hex-encoded SHA256 of secret
	Secret        string `json:"secret,omitempty"`
	SecretRevealed bool  `json:"secret_revealed"`
}

// SwapHTLCClaimParams is the parameters for swap_htlcClaim.
type SwapHTLCClaimParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"` // Which chain to claim on
}

// SwapHTLCClaimResult is the result of swap_htlcClaim.
type SwapHTLCClaimResult struct {
	TradeID   string `json:"trade_id"`
	ClaimTxID string `json:"claim_txid"`
	Chain     string `json:"chain"`
	State     string `json:"state"`
}

// SwapHTLCRefundParams is the parameters for swap_htlcRefund.
type SwapHTLCRefundParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"` // Which chain to refund on
}

// SwapHTLCRefundResult is the result of swap_htlcRefund.
type SwapHTLCRefundResult struct {
	TradeID    string `json:"trade_id"`
	RefundTxID string `json:"refund_txid"`
	Chain      string `json:"chain"`
	State      string `json:"state"`
}

// SwapHTLCExtractSecretParams is the parameters for swap_htlcExtractSecret.
type SwapHTLCExtractSecretParams struct {
	TradeID string `json:"trade_id"`
	TxID    string `json:"txid"`
	Chain   string `json:"chain"`
}

// SwapHTLCExtractSecretResult is the result of swap_htlcExtractSecret.
type SwapHTLCExtractSecretResult struct {
	TradeID string `json:"trade_id"`
	Secret  string `json:"secret"`
	Message string `json:"message"`
}

// =============================================================================
// EVM HTLC Types
// =============================================================================

// SwapEVMCreateParams is the parameters for swap_evmCreate.
type SwapEVMCreateParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"` // "ETH", "BSC", etc.
}

// SwapEVMCreateResult is the result of swap_evmCreate.
type SwapEVMCreateResult struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
	TxHash  string `json:"tx_hash"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// SwapEVMClaimParams is the parameters for swap_evmClaim.
type SwapEVMClaimParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
}

// SwapEVMClaimResult is the result of swap_evmClaim.
type SwapEVMClaimResult struct {
	TradeID     string `json:"trade_id"`
	Chain       string `json:"chain"`
	ClaimTxHash string `json:"claim_tx_hash"`
	State       string `json:"state"`
}

// SwapEVMRefundParams is the parameters for swap_evmRefund.
type SwapEVMRefundParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
}

// SwapEVMRefundResult is the result of swap_evmRefund.
type SwapEVMRefundResult struct {
	TradeID      string `json:"trade_id"`
	Chain        string `json:"chain"`
	RefundTxHash string `json:"refund_tx_hash"`
	State        string `json:"state"`
}

// SwapEVMStatusParams is the parameters for swap_evmStatus.
type SwapEVMStatusParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
}

// SwapEVMStatusResult is the result of swap_evmStatus.
type SwapEVMStatusResult struct {
	TradeID      string `json:"trade_id"`
	Chain        string `json:"chain"`
	State        string `json:"state"` // "invalid", "active", "refunded", "claimed"
	Initiator    string `json:"initiator"`
	Receiver     string `json:"receiver"`
	TokenAddress string `json:"token_address"`
	Amount       string `json:"amount"`
	SecretHash   string `json:"secret_hash"`
	Timelock     int64  `json:"timelock"`
	IsNative     bool   `json:"is_native"`
}

// SwapEVMWaitSecretParams is the parameters for swap_evmWaitSecret.
type SwapEVMWaitSecretParams struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
}

// SwapEVMWaitSecretResult is the result of swap_evmWaitSecret.
type SwapEVMWaitSecretResult struct {
	TradeID string `json:"trade_id"`
	Chain   string `json:"chain"`
	Secret  string `json:"secret"`
	Message string `json:"message"`
}

// SwapEVMSetSecretParams is the parameters for swap_evmSetSecret.
type SwapEVMSetSecretParams struct {
	TradeID string `json:"trade_id"`
	Secret  string `json:"secret"` // Hex-encoded 32-byte secret
}

// SwapEVMSetSecretResult is the result of swap_evmSetSecret.
type SwapEVMSetSecretResult struct {
	TradeID string `json:"trade_id"`
	Message string `json:"message"`
}

// SwapEVMGetContractsResult is the result of swap_evmGetContracts.
type SwapEVMGetContractsResult struct {
	Contracts []EVMContractInfo `json:"contracts"`
	Count     int               `json:"count"`
}

// EVMContractInfo holds info about a deployed EVM contract.
type EVMContractInfo struct {
	ChainID         uint64 `json:"chain_id"`
	ContractAddress string `json:"contract_address"`
}

// SwapEVMGetContractParams is the parameters for swap_evmGetContract.
type SwapEVMGetContractParams struct {
	ChainID uint64 `json:"chain_id"`
}

// SwapEVMGetContractResult is the result of swap_evmGetContract.
type SwapEVMGetContractResult struct {
	ChainID         uint64 `json:"chain_id"`
	ContractAddress string `json:"contract_address"`
	Deployed        bool   `json:"deployed"`
}

// SwapEVMComputeSwapIDParams is the parameters for swap_evmComputeSwapID.
type SwapEVMComputeSwapIDParams struct {
	Initiator    string `json:"initiator"`
	Receiver     string `json:"receiver"`
	TokenAddress string `json:"token_address"` // Empty or "0x0" for native
	Amount       string `json:"amount"`        // Wei as string
	SecretHash   string `json:"secret_hash"`   // Hex-encoded 32 bytes
	Timelock     int64  `json:"timelock"`      // Unix timestamp
}

// SwapEVMComputeSwapIDResult is the result of swap_evmComputeSwapID.
type SwapEVMComputeSwapIDResult struct {
	SwapID string `json:"swap_id"`
}

// =============================================================================
// Cross-Chain Swap Init Types
// =============================================================================

// SwapGetSwapTypeParams is the parameters for swap_getSwapType.
type SwapGetSwapTypeParams struct {
	TradeID string `json:"trade_id"`
}

// SwapGetSwapTypeResult is the result of swap_getSwapType.
type SwapGetSwapTypeResult struct {
	TradeID      string `json:"trade_id"`
	SwapType     string `json:"swap_type"`      // "evm_to_evm", "bitcoin_to_evm", "evm_to_bitcoin", "bitcoin_to_bitcoin"
	OfferChain   string `json:"offer_chain"`
	RequestChain string `json:"request_chain"`
	Method       string `json:"method"` // "htlc" or "musig2"
}

// SwapInitCrossChainParams is the parameters for swap_initCrossChain.
type SwapInitCrossChainParams struct {
	TradeID string `json:"trade_id"`
	Role    string `json:"role"` // "initiator" or "responder"
}

// SwapInitCrossChainResult is the result of swap_initCrossChain.
type SwapInitCrossChainResult struct {
	TradeID      string `json:"trade_id"`
	Role         string `json:"role"`
	SwapType     string `json:"swap_type"`  // "evm_to_evm", "bitcoin_to_evm", "evm_to_bitcoin"
	OfferChain   string `json:"offer_chain"`
	RequestChain string `json:"request_chain"`
	SecretHash   string `json:"secret_hash,omitempty"`
	LocalEVMAddr string `json:"local_evm_address,omitempty"`
	State        string `json:"state"`
	Message      string `json:"message"`
}
