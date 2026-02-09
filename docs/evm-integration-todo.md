# EVM HTLC Integration TODO

## Overview

This document tracks the integration of EVM HTLC smart contracts with the Klingon Go codebase.

**Goal:** Enable atomic swaps between:
- EVM ↔ EVM (ETH ↔ BSC, ETH ↔ USDC, etc.)
- EVM ↔ Bitcoin-family (ETH ↔ BTC, USDC ↔ LTC, etc.)

---

## Phase 1: Contract Deployment

### 1.1 Setup Foundry Environment ✅ COMPLETED
- [x] Install Foundry (`curl -L https://foundry.paradigm.xyz | bash && foundryup`)
- [x] Install OpenZeppelin (`cd contracts && forge install OpenZeppelin/openzeppelin-contracts --no-commit`)
- [x] Run tests (`forge test`) - **28 tests passing**
- [x] Run tests with gas report (`forge test --gas-report`)

### 1.2 Deploy to Testnets ✅ COMPLETED
- [x] Create `.env` from `.env.example` with private key and RPC URLs
- [x] Deploy to Sepolia (Ethereum testnet) - **0x628c677e7b8889e64564d3f381565a9e6656aade**
- [x] Deploy to BSC Testnet - **0xC8515f07b08b586a2Fd6A389585D9a182D03adFB**
- [ ] Deploy to Polygon Amoy (optional)
- [ ] Deploy to Arbitrum Sepolia (optional)
- [x] Record deployed addresses in `internal/config/evm_contracts.go`

### 1.3 Verify Contracts (Pending)
- [x] Verify on Etherscan (Sepolia)
- [x] Verify on BSCScan (Testnet)
- [ ] Test basic operations via block explorer

---

## Phase 2: Go Bindings

### 2.1 Generate ABI Bindings ✅ COMPLETED
- [x] Install abigen (`go install github.com/ethereum/go-ethereum/cmd/abigen@latest`)
- [x] Extract ABI from compiled contract
- [x] Generate Go bindings to `internal/contracts/htlc/klingon_htlc.go` - **2024 lines**
- [x] Verify bindings compile
- [x] Add `github.com/ethereum/go-ethereum` dependency to go.mod

### 2.2 Create Client Wrapper ✅ COMPLETED
- [x] Create `internal/contracts/htlc/client.go` - **~700 lines**
  - [x] `NewClient(rpcURL, contractAddress)` - constructor
  - [x] `GenerateSecret()` - create secret + hash
  - [x] `ComputeSwapID()` - deterministic swap ID
  - [x] `CreateSwapNative()` - lock ETH/BNB
  - [x] `CreateSwapERC20()` - lock ERC20 tokens
  - [x] `ApproveERC20()` - approve token spending
  - [x] `Claim()` - claim with secret
  - [x] `Refund()` - refund after timeout
  - [x] `GetSwap()` - get swap details
  - [x] `CanClaim()` / `CanRefund()` - check status
  - [x] `WatchSwapCreated()` - monitor new swaps
  - [x] `WatchSwapClaimed()` - monitor claims (for secret extraction)
  - [x] `WatchSwapRefunded()` - monitor refunds
  - [x] `GetSecretFromClaim()` - extract secret from claim tx
  - [x] `WaitForSecret()` - wait for secret on claim event
  - [x] `GetSwapCreatedEvents()` / `GetSwapClaimedEvents()` - historical queries

### 2.3 Unit Tests ✅ COMPLETED
- [x] Create `internal/contracts/htlc/client_test.go` - **8 unit tests + 8 integration tests**
- [x] Test secret generation and verification
- [x] Test swap state helpers
- [x] Test private key parsing
- [x] Integration tests for Anvil node:
  - [x] Test ComputeSwapID
  - [x] Test CreateSwapNative + Claim flow
  - [x] Test CreateSwapNative + Refund flow
  - [x] Test event queries (SwapCreated, SwapClaimed)
  - [x] Test view functions (DAO address, fee, paused)
  - [x] Test gas estimation
  - [x] Test TimeUntilRefund

---

## Phase 3: Contract Configuration ✅ COMPLETED

### 3.1 Contract Address Configuration ✅ COMPLETED
- [x] Create `internal/config/evm_contracts.go`
- [x] Add testnet addresses (Sepolia: `0x628c677e7b8889e64564d3f381565a9e6656aade`)
- [x] Add mainnet addresses (placeholder until audit)
- [x] Helper functions: `GetHTLCContract()`, `IsHTLCDeployed()`, `ListDeployedHTLCChains()`
- [x] Unit tests for contract address lookup
- Note: Contract addresses are hardcoded (not configurable via YAML) for security

---

## Phase 4: Coordinator Integration ✅ COMPLETED

### 4.1 EVM HTLC Data Structures ✅ COMPLETED
- [x] Create `internal/swap/evm_types.go` - **~470 lines**
  - [x] `EVMHTLCSession` - manages EVM HTLC operations
  - [x] `EVMHTLCStorageData` - serializable for persistence
  - [x] `ChainEVMHTLCData` - per-chain data holder
  - [x] `EVMHTLCSwapData` - swap-level data
  - [x] `CrossChainType` - Bitcoin/EVM combination detection
  - [x] `GetCrossChainSwapType()` - determine swap type

### 4.2 Coordinator Methods ✅ COMPLETED
- [x] Create `internal/swap/coordinator_evm.go` - **~510 lines**
  - [x] `CreateEVMHTLC()` - create swap on EVM chain
  - [x] `ClaimEVMHTLC()` - claim with secret
  - [x] `RefundEVMHTLC()` - refund after timeout
  - [x] `GetEVMHTLCStatus()` - check on-chain status
  - [x] `WaitForEVMSecret()` - wait for secret reveal
  - [x] `getOrCreateEVMSession()` - session management
  - [x] `getEVMPrivateKey()` - key derivation with ToECDSA()
  - [x] `computeEVMSwapParams()` - swap parameter calculation

### 4.3 Cross-Chain Secret Monitoring ✅ COMPLETED
- [x] Create `internal/swap/secret_monitor.go` - **~440 lines**
  - [x] `SecretMonitor` - unified secret monitoring service
  - [x] `StartMonitoring()` - start monitoring for a swap
  - [x] `monitorEVMChain()` - watch EVM claim events
  - [x] `monitorBitcoinChain()` - watch Bitcoin witness data
  - [x] `checkBitcoinClaim()` - extract secret from witness
  - [x] `propagateSecret()` - share secret across sessions
  - [x] Unified `SecretRevealEvent` for both chain types

### 4.4 Storage Updates ✅ COMPLETED
- [x] Add `CoordinatorEVMHTLCStorageData` struct
- [x] Add `EVMHTLCChainStorageData` struct
- [x] `getEVMHTLCStorageDataUnlocked()` - serialize EVM data
- [x] `getCrossChainStorageDataUnlocked()` - serialize cross-chain data
- [x] Updated `saveSwapState()` to handle EVM and cross-chain swaps
- Note: No DB migration needed - uses existing `method_data` JSON column

---

## Phase 5: RPC Handlers ✅ COMPLETED

### 5.1 EVM Swap RPC Methods ✅ COMPLETED
- [x] Create `internal/rpc/swap_evm.go` - **~300 lines**
  - [x] `swap_evmCreate` - create EVM HTLC
  - [x] `swap_evmClaim` - claim EVM HTLC
  - [x] `swap_evmRefund` - refund EVM HTLC
  - [x] `swap_evmStatus` - get EVM swap status
  - [x] `swap_evmWaitSecret` - wait for secret reveal
  - [x] `swap_evmGetContracts` - list all deployed contracts
  - [x] `swap_evmGetContract` - get contract for specific chain
  - [x] `swap_evmComputeSwapID` - compute swap ID

### 5.2 RPC Types ✅ COMPLETED
- [x] Added EVM types to `internal/rpc/swap_types.go`
  - `SwapEVMCreateParams/Result`
  - `SwapEVMClaimParams/Result`
  - `SwapEVMRefundParams/Result`
  - `SwapEVMStatusParams/Result`
  - `SwapEVMWaitSecretParams/Result`
  - `SwapEVMGetContractsResult`
  - `SwapEVMGetContractParams/Result`
  - `SwapEVMComputeSwapIDParams/Result`
  - `EVMContractInfo`

### 5.3 Register Handlers ✅ COMPLETED
- [x] Added EVM handlers to `server.go`
- [x] All 8 EVM handlers registered

---

## Phase 6: P2P Message Updates ✅ COMPLETED

### 6.1 New Message Types ✅ COMPLETED
- [x] Add `SwapMsgEVMFundingInfo` message type - EVM HTLC created on-chain
- [x] Add `SwapMsgEVMClaimed` message type - includes secret for counterparty
- [x] Add `SwapMsgEVMRefunded` message type - refund notification
- [x] Updated `internal/node/swap_handler.go`

### 6.2 Payload Structs ✅ COMPLETED
- [x] `EVMFundingInfoPayload` - chain, chainID, txHash, swapID, contract, sender, receiver, token, amount, secretHash, timelock
- [x] `EVMClaimPayload` - chain, chainID, txHash, swapID, secret
- [x] `EVMRefundPayload` - chain, chainID, txHash, swapID

### 6.3 Helper Functions ✅ COMPLETED
- [x] `NewEVMFundingInfoMessage()` - create funding info message
- [x] `NewEVMClaimMessage()` - create claim notification message
- [x] `NewEVMRefundMessage()` - create refund notification message

---

## Phase 7: Cross-Chain Swap Flow ✅ COMPLETED

### 7.1 Cross-Chain Orchestration ✅ COMPLETED
- [x] Create `internal/swap/coordinator_cross_chain.go` - **~600 lines**
- [x] `InitiateCrossChainSwap()` - auto-detect swap type and route
- [x] `RespondToCrossChainSwap()` - respond with correct chain type
- [x] `GetSwapType()` - query swap's cross-chain type

### 7.2 EVM ↔ EVM Flow ✅ COMPLETED
- [x] `initiateEVMToEVMSwap()` - initiator starts EVM-to-EVM swap
- [x] `respondEVMToEVMSwap()` - responder joins EVM-to-EVM swap
- [x] Creates EVM sessions for both chains
- [x] Secret generation and propagation

### 7.3 Bitcoin ↔ EVM Flow ✅ COMPLETED
- [x] `initiateBitcoinToEVMSwap()` - BTC offer, EVM request
- [x] `initiateEVMToBitcoinSwap()` - EVM offer, BTC request
- [x] `respondBitcoinToEVMSwap()` - respond to BTC→EVM swap
- [x] `respondEVMToBitcoinSwap()` - respond to EVM→BTC swap
- [x] Creates mixed HTLC sessions (Bitcoin P2WSH + EVM contract)
- [x] Secret hash propagation between chain types

### 7.4 Timeout Coordination ✅ COMPLETED
- [x] `GetTimelockForChain()` - role-based timelock calculation
- [x] `ValidateTimelockSafety()` - verify safe timelock ordering
- [x] Initiator chain: 24h (testnet) / 48h (mainnet)
- [x] Responder chain: 12h (testnet) / 24h (mainnet)

### 7.5 EVM Session Enhancements ✅ COMPLETED
- [x] Added `SetRemoteAddress()` method to EVMHTLCSession
- [x] Added `GetRemoteAddress()` method to EVMHTLCSession

---

## Phase 7.5: Code Quality Fixes ✅ COMPLETED (2025-12-22)

### TODOs Fixed
- [x] `coordinator_funding.go` - Sign funding transactions with wallet keys
  - Now uses `buildAndSignFundingTx()` for proper signing
  - Supports P2WPKH, P2TR, P2PKH input types
- [x] `coordinator_funding.go` - Proper multi-output DAO support
  - Creates separate escrow and DAO fee outputs (vout 0 and vout 1)
  - Works on both testnet and mainnet
  - Added UTXO selection and fee estimation
- [x] `coordinator_evm.go` - Remove hardcoded RPC URLs
  - Now uses `backend.DefaultConfigs()` for consistent RPC endpoints
  - Single source of truth in `internal/backend/backend.go`
- [x] `coordinator_cross_chain.go` - Implement timelock safety checks
  - `ValidateTimelockSafety()` validates initiator > responder + margin
  - Converts Bitcoin block heights to timestamps for comparison
  - 2-hour safety margin for EVM chains
- [x] `coordinator_timeout.go` - Improve address management
  - Uses stored wallet addresses (`LocalOfferWalletAddr`, `LocalRequestWalletAddr`)
  - Falls back to deterministic derivation based on trade ID

### New Exported Functions
- [x] `wallet.ParseAddressToScript()` - Convert address to output script

---

## Phase 8: Testing

### 8.1 Unit Tests
- [ ] `internal/contracts/htlc/client_test.go`
- [ ] `internal/swap/coordinator_evm_test.go`
- [ ] `internal/rpc/swap_evm_test.go`

### 8.2 Integration Tests
- [ ] Create `scripts/test-evm-swap.sh` (same-chain)
- [ ] Create `scripts/test-evm-cross-swap.sh` (cross-EVM)
- [ ] Create `scripts/test-evm-btc-swap.sh` (EVM ↔ BTC)
- [ ] Create `scripts/test-evm-refund.sh` (timeout refund)

### 8.3 Testnet Validation
- [ ] Run full swap on Sepolia ↔ BSC Testnet
- [ ] Run full swap on Sepolia ↔ BTC Testnet
- [ ] Verify gas costs match estimates
- [ ] Verify fee collection works

---

## Phase 9: Documentation

### 9.1 Update CLAUDE.md
- [ ] Add EVM HTLC section
- [ ] Document new RPC methods
- [ ] Add EVM swap examples

### 9.2 API Documentation
- [ ] Document `swap_evmCreate` params/response
- [ ] Document `swap_evmClaim` params/response
- [ ] Document `swap_evmRefund` params/response
- [ ] Add cross-chain swap examples

---

## Phase 10: Production Readiness

### 10.1 Security
- [ ] Professional audit of smart contract
- [ ] Review Go integration code
- [ ] Test reentrancy protection
- [ ] Test edge cases (gas exhaustion, etc.)

### 10.2 Mainnet Deployment
- [ ] Set mainnet DAO addresses in `Deploy.s.sol`
- [ ] Deploy to Ethereum mainnet
- [ ] Deploy to BSC mainnet
- [ ] Deploy to Polygon mainnet
- [ ] Deploy to Arbitrum mainnet
- [ ] Verify all contracts
- [ ] Update config with mainnet addresses

### 10.3 Monitoring
- [ ] Set up event monitoring for mainnet contracts
- [ ] Alert on failed swaps
- [ ] Dashboard for swap statistics

---

## Contract Addresses

### Testnets
| Chain | Network | Address |
|-------|---------|---------|
| Ethereum | Sepolia | `0x628c677e7b8889e64564d3f381565a9e6656aade` |
| BSC | Testnet | `0xC8515f07b08b586a2Fd6A389585D9a182D03adFB` |
| Polygon | Amoy | TBD |
| Arbitrum | Sepolia | TBD |

### Mainnets
| Chain | Network | Address |
|-------|---------|---------|
| Ethereum | Mainnet | TBD (after audit) |
| BSC | Mainnet | TBD |
| Polygon | Mainnet | TBD |
| Arbitrum | Mainnet | TBD |

---

## Dependencies

```go
// go.mod additions (DONE)
require (
    github.com/ethereum/go-ethereum v1.16.7  // ✅ Added
)
```

---

## Commands Reference

```bash
# Foundry
forge build                    # Compile contracts
forge test                     # Run tests
forge test -vvv                # Verbose tests
forge test --gas-report        # Gas usage

# Deployment
forge script script/Deploy.s.sol --rpc-url $RPC_URL --broadcast

# Verification
forge verify-contract $ADDRESS src/KlingonHTLC.sol:KlingonHTLC --chain sepolia

# Go bindings
abigen --abi KlingonHTLC.abi --pkg htlc --type KlingonHTLC --out klingon_htlc.go

# Testing
go test ./internal/contracts/htlc/... -v
./scripts/test-evm-swap.sh
```

---

## Estimated Effort

| Phase | Effort | Status |
|-------|--------|--------|
| Phase 1: Contract Deployment | 1-2 hours | ✅ Sepolia + BSC deployed |
| Phase 2: Go Bindings | 2-3 hours | ✅ Complete |
| Phase 3: Contract Configuration | 0.5 hours | ✅ Complete |
| Phase 4: Coordinator Integration | 4-6 hours | ✅ Complete |
| Phase 5: RPC Handlers | 2-3 hours | ✅ Complete |
| Phase 6: P2P Messages | 1-2 hours | ✅ Complete |
| Phase 7: Cross-Chain Flow | 4-6 hours | ✅ Complete |
| Phase 8: Testing | 3-4 hours | **Next** |
| Phase 9: Documentation | 1-2 hours | Pending |
| Phase 10: Production | Depends on audit | Pending |

**Completed:** Phases 1-7 (~18-24 hours)
**Remaining (excluding audit):** ~6-8 hours
