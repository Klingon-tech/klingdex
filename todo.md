# Klingon-v2 Development Roadmap

## Current Status

✅ P2P Node Infrastructure
- libp2p networking (DHT, PubSub, mDNS)
- Peer persistence (SQLite)
- JSON-RPC API with WebSocket events
- CORS support for cross-origin requests (Electron apps, web clients)
- Testnet/mainnet separation
- YAML config file auto-generation
- Unit tests for core packages

✅ **MuSig2 Atomic Swaps WORKING (BTC ↔ LTC Testnet)**
- Successfully completed BTC/LTC swap on 2024-12-17
- Full flow: order create → take → init → nonces → fund → sign → redeem
- Both parties received funds correctly

## Phase 1: Exchange Configuration ✅

### 1.1 Create Exchange Config (`internal/config/`) ✅

- [x] Create `internal/config/config.go` with all exchange parameters
- [x] Define supported coins with chain params (mainnet/testnet)
- [x] Define DAO fee addresses (mainnet/testnet)
- [x] Define fee rates (maker/taker fees)
- [x] Define atomic swap timeouts
- [x] Define minimum/maximum trade amounts per coin
- [x] Write unit tests for config

**Supported Coins:**
| Coin | Type | Atomic Swap Method |
|------|------|-------------------|
| BTC  | Bitcoin | MuSig2 (priority) / HTLC fallback |
| LTC  | Bitcoin fork | MuSig2 / HTLC fallback |
| DOGE | Bitcoin fork | HTLC |
| XMR  | Monero | Adaptor signatures |
| ETH  | EVM | Smart contract |
| BSC  | EVM | Smart contract |
| MATIC | EVM | Smart contract |
| ARB  | EVM | Smart contract |
| SOL  | Solana | Program-based |

**Fee Structure:**
- Maker fee: 0.2%
- Taker fee: 0.2%
- Fee distribution: 50% DAO / 50% Node operators

**Swap Priority:**
1. MuSig2 (Taproot) - preferred, more private
2. HTLC fallback - if Taproot not supported

### 1.1.1 Swap Path Configuration

**See:** `docs/swap-paths.md` for detailed swap path matrix and implementation plan.

Not all coin pairs can be swapped due to different scripting systems and cryptographic requirements.

**Supported Paths (MVP):**
| Path | Method | Status |
|------|--------|--------|
| BTC ↔ LTC | MuSig2/HTLC | ✅ Working |
| BTC ↔ DOGE | HTLC | Pending HTLC impl |
| BTC ↔ BCH | HTLC | Pending HTLC impl |
| LTC ↔ DOGE | HTLC | Pending HTLC impl |
| ETH ↔ BSC/MATIC/ARB | Contract | Pending EVM impl |

**Phase 2:**
| Path | Method | Status |
|------|--------|--------|
| BTC ↔ XMR | Adaptor | Pending adaptor impl |
| LTC ↔ XMR | Adaptor | Pending adaptor impl |

**Not Supported:**
| Path | Reason |
|------|--------|
| XMR ↔ ETH | No cross-curve adaptor for EVM |
| XMR ↔ SOL | No practical method |

**Implementation Tasks:**
- [ ] Create `internal/config/swap_paths.go` with SwapPath struct and registry
- [ ] Add `IsSwapPathSupported(from, to)` helper
- [ ] Add `GetSwapPathMethods(from, to)` helper
- [ ] Add `GetPreferredPathMethod(from, to)` helper
- [ ] Integrate path validation in `Offer.Validate()`
- [ ] Add `swap_supportedPaths` RPC method
- [ ] Unit tests for swap path validation

### 1.2 Chain Parameters (`internal/chain/`) ✅

- [x] Create `chain.go` with base types and interfaces
- [x] Create `bitcoin.go` - BTC params, derivation paths
- [x] Create `litecoin.go` - LTC params
- [x] Create `dogecoin.go` - DOGE params
- [x] Create `ethereum.go` - ETH/EVM params
- [x] Create `solana.go` - SOL params
- [x] Create `monero.go` - XMR params
- [x] Unit tests for chain params (17 tests)

### 1.3 Backend Interface (`internal/backend/`) ✅

- [x] Create `backend.go` - interface (GetUTXOs, Broadcast, GetBalance, etc.)
- [x] Implement `mempool.go` - mempool.space REST API
- [x] Implement `esplora.go` - blockstream.info REST API
- [x] Implement `electrum.go` - Electrum protocol (TCP/SSL)
- [x] Implement `blockbook.go` - Trezor Blockbook API
- [x] Implement `jsonrpc.go` - Bitcoin + EVM chains (partial: SOL/XMR later)
- [x] Add backend config to YAML with public API defaults
- [x] Unit tests for backends (30 tests)

### 1.4 Wallet (`internal/wallet/`) ✅

- [x] Create `wallet.go` - BIP39 mnemonic, HD key derivation with chain.Network integration
- [x] Create `crypto.go` - Argon2id + AES-256-GCM encryption (no scrypt)
- [x] Create `address.go` - Bitcoin-family address encoding (P2PKH, P2WPKH, P2TR)
- [x] Create `evm.go` - EVM address generation with keccak256, EIP-55 checksum
- [x] Unit tests for wallet (36 tests)
- [x] Create `service.go` - Wallet lifecycle management
- [x] Add wallet RPC methods (generate, create, getAddress, etc.)

**Wallet Features (implemented):**
- 24-word BIP39 mnemonic generation
- BIP44/84/86 HD key derivation for all supported chains
- Chain-specific derivation paths from `internal/chain`
- Address generation: BTC (bc1q), LTC (ltc1q), DOGE (D), ETH (0x)
- Argon2id password-based encryption for seed storage
- EVM signing (personal_sign, EIP-712)
- Key caching for performance
- Transaction building and signing (P2WPKH, P2TR, P2PKH)
- UTXO selection (greedy algorithm - largest first)
- Fee estimation via mempool.space API
- RPC: `wallet_send`, `wallet_getUTXOs`, `wallet_getBalance`, `wallet_getAddress`

**Wallet Features (completed):**
- [x] Change address derivation (BIP44 change=1 path) - `DeriveAddressWithChange()`
- [x] `wallet_scanBalance` - Scan all addresses (external + change) for total balance with gap limit
- [x] `wallet_getAddressWithChange` - Get specific external/change address
- [x] Gap limit scanning for address discovery (default: 20)
- [x] Track used addresses in database - `wallet_addresses` table
- [x] **Multi-Address Spending (2024-12-18)** - Aggregate UTXOs from all addresses

**Multi-Address Wallet (NEW - 2024-12-18):**

Proper non-custodial wallet behavior: aggregate UTXOs from multiple addresses, sign each input with its own derived key.

| Feature | Description |
|---------|-------------|
| `wallet_sendAll` | Send from ALL addresses (aggregates UTXOs) |
| `wallet_sendMax` | Send maximum amount from all addresses |
| `wallet_getAggregatedBalance` | Total balance across all addresses |
| `wallet_listAllUTXOs` | List all UTXOs with derivation paths |
| `wallet_syncUTXOs` | Force UTXO sync to database |

**Implementation:**
- `internal/storage/wallet_utxos.go` - UTXO and address persistence
- `internal/wallet/multi_address_tx.go` - Multi-key transaction building
- `internal/wallet/utxo_sync.go` - Gap limit scanning and UTXO sync service

**Database Schema:**
```sql
wallet_addresses (address, chain, account, change, address_index, ...)
wallet_utxos (txid, vout, amount, address, derivation_path, status, ...)
wallet_sync_state (chain, last_external_index, last_change_index, gap_limit, ...)
```

**Example usage:**
```bash
# List all UTXOs from all addresses
curl -s http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"wallet_listAllUTXOs","params":{"symbol":"BTC"},"id":1}'

# Send 0.002 BTC aggregating UTXOs from multiple addresses
curl -s http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"wallet_sendAll","params":{"symbol":"BTC","to":"tb1q...","amount":200000},"id":1}'

# Send entire wallet balance
curl -s http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"wallet_sendMax","params":{"symbol":"BTC","to":"tb1q..."},"id":1}'
```

## Phase 2: Atomic Swap Implementation

### 2.1 HTLC-based Swaps (Bitcoin-family) ✅ COMPLETED

See section 2.3 for detailed implementation.

- [x] Implement HTLC contract generation (`htlc_script.go`)
- [x] Implement secret generation and hashing (`GenerateSecret`, `VerifySecret`)
- [x] Implement contract redemption (`ClaimHTLC`, `BuildHTLCClaimTx`)
- [x] Implement contract refund (timeout) (`RefundHTLC`, `BuildHTLCRefundTx`)
- [x] Cross-chain swap state machine (coordinator with HTLC support)
- [x] Unit tests for swap logic (16 HTLC tests passing)

### 2.2 Taproot/MuSig2 Swaps (BTC/LTC) ✅

- [x] Research: Review taproot-assets multisig implementation
- [x] Implement MuSig2 key aggregation (`internal/swap/musig2.go`)
- [x] Implement MuSig2 signing session (nonce generation, signature combining)
- [x] Implement Taproot address generation (P2TR bech32m encoding)
- [x] Implement swap state machine (`internal/swap/swap.go`)
- [x] Implement transaction building (`internal/swap/tx.go`)
- [x] Unit tests (35 tests passing)

**Security Features (IMPLEMENTED):**
- [x] **Nonce reuse prevention** - MuSig2Session tracks used nonces, invalidates session after signing
- [x] **Safety margin enforcement** - ChainTimeoutConfig with safety margins to prevent timeout race
- [x] **Confirmation tracking** - FundingStatus, IsFundingConfirmed() to protect against reorg attacks
- [x] **Block-based timeouts** - More precise than time-based for blockchain operations

**Implementation Details:**
- Generic swap package uses `chain.Get()` for chain params
- Uses `config.DefaultFeeConfig()` and `config.GetDAOAddress()` for fees
- Supports any Bitcoin-family chain with `SupportsTaproot: true`
- Ephemeral key generation for swap privacy
- BIP-86 tweaked output keys for key-path spending

**References:**
- https://github.com/lightninglabs/taproot-assets/blob/main/itest/multisig.go
- https://github.com/bisq-network/bisq-musig
- https://bitcoinops.org/en/topics/musig/
- github.com/btcsuite/btcd/btcec/v2/schnorr/musig2

### 2.3 HTLC Implementation (Fallback) ✅ COMPLETED (Bitcoin-family)

**See:** `docs/htlc-implementation.md` for detailed implementation plan.

**Files Created:** ✅
- [x] `internal/swap/method.go` - SwapMethodHandler interface + factory (109 lines)
- [x] `internal/swap/htlc.go` - HTLCSession implementation (486 lines)
- [x] `internal/swap/htlc_script.go` - HTLC script generation (P2WSH) (357 lines)
- [x] HTLC tx builders in `tx.go` - BuildHTLCClaimTx, BuildHTLCRefundTx
- [x] `internal/swap/htlc_test.go` - Unit tests (840 lines, 16 tests passing)

**Files Modified:** ✅
- [x] `internal/swap/coordinator_init.go` - initiateHTLCSwap(), respondHTLCSwap()
- [x] `internal/swap/coordinator_htlc.go` - ClaimHTLC(), RefundHTLC(), ExtractSecretFromTx()
- [x] `internal/swap/htlc.go` - HTLCStorageData struct

**Implementation Tasks:** ✅
- [x] Create SwapMethodHandler interface (Method, GetLocalPubKey, SetRemotePubKey, GenerateSwapAddress, Sign)
- [x] Make MuSig2Session implement SwapMethodHandler (musig2.go:854)
- [x] Implement HTLCSession with ECDSA signing
- [x] Build HTLC script: `OP_IF <hash> <claim_key> OP_ELSE <timeout> CSV <refund_key> OP_ENDIF`
- [x] P2WSH address generation from HTLC script (BuildHTLCScriptData)
- [x] Claim witness: `<sig> <secret> <1> <script>` (BuildHTLCClaimWitness)
- [x] Refund witness: `<sig> <0> <script>` (BuildHTLCRefundWitness)
- [x] Update coordinator to select method based on offer preference + chain support
- [x] Add `initiateHTLCSwap()` and `respondToHTLCSwap()` to coordinator
- [x] RPC handlers: swap_htlc.go (swapHTLCRevealSecret, swapHTLCGetSecret, swapHTLCClaim, swapHTLCRefund, swapHTLCExtractSecret)
- [x] Integration test scripts: test-htlc-swap.sh, test-htlc-refund.sh

**HTLC Contract for EVM chains:** (In Progress)

**Design & Implementation:** ✅
- [x] Design document: `docs/evm-htlc-design.md`
- [x] Solidity contract: `contracts/src/KlingonHTLC.sol`
  - Native token (ETH/BNB) + ERC20 support
  - ReentrancyGuard + SafeERC20
  - DAO fee collection (0.2%)
  - Pause functionality
  - SHA256 hash (compatible with Bitcoin HTLCs)
- [x] Foundry test suite: `contracts/test/KlingonHTLC.t.sol`
- [x] Deployment scripts: `contracts/script/Deploy.s.sol`

**Deployment & Integration:** (Pending)

**See:** `docs/evm-integration-todo.md` for detailed 10-phase implementation plan.

Summary:
- [ ] Phase 1: Deploy contracts to testnets
- [ ] Phase 2: Generate Go bindings with abigen
- [ ] Phase 3: Backend integration
- [ ] Phase 4: Coordinator integration
- [ ] Phase 5: RPC handlers
- [ ] Phase 6: P2P message updates
- [ ] Phase 7: Cross-chain swap flow (EVM↔EVM, EVM↔BTC)
- [ ] Phase 8: Testing
- [ ] Phase 9: Documentation
- [ ] Phase 10: Security audit & mainnet deployment

### 2.3.5 Security Requirements (Critical)

**Must-Have Before Production:**

| Requirement | Status | Notes |
|-------------|--------|-------|
| Nonce reuse prevention | ✅ | MuSig2Session tracks used nonces, invalidates after signing |
| Safety margin enforcement | ✅ | ChainTimeoutConfig with margins to prevent timeout race |
| Confirmation tracking | ✅ | FundingStatus, IsFundingConfirmed() for reorg protection |
| Adaptor signatures | ⏳ | Needed for atomic key revelation (Monero swaps) |

**High Priority:**

| Requirement | Status | Notes |
|-------------|--------|-------|
| Private key transfer (Bisq happy path) | ❌ | Exchange key shares off-chain |
| Swap monitoring | ✅ | Monitor for counterparty actions |
| Post-trade watchtower | ❌ | Monitor for WarningTx attacks |

**Medium Priority:**

| Requirement | Status | Notes |
|-------------|--------|-------|
| Warning/Redirect/Claim tx hierarchy | ❌ | Bisq-style dispute resolution |
| Anchor outputs for CPFP | ❌ | Enable fee bumping |
| nSequence enforcement | ❌ | Transaction-level timelocks |

**Attack Mitigations:**

| Attack | Status | Mitigation |
|--------|--------|------------|
| Nonce Reuse | ✅ | Track used nonces, reject reuse |
| Free Option | ❌ | Security deposits, shorter timeouts |
| Timeout Race | ✅ | Safety margin before timeout |
| Preimage Front-Running | ❌ | Private mempool, fast claiming |
| Post-Trade Attack | ❌ | Watchtower monitoring |
| Reorg Attack | ✅ | Wait for confirmations |

### 2.4 Protocol & Storage Layer ✅ COMPLETED

**Design Goals:**
- Method-agnostic core (orders, trades work the same regardless of swap method)
- Extensible for future methods (adaptor sigs, new chains)
- Track both legs of swap independently (different chains, different states)

#### 2.4.1 P2P Message Protocol (`internal/protocol/`) ✅

- [x] Define message envelope (type, version, payload, signature) - `message.go`
- [x] Order messages: `OrderAnnounce`, `OrderCancel`, `OrderTake`, `OrderTaken` - `order.go`
- [x] Swap messages: `SwapInit`, `SwapAccept`, `SwapFunding`, `SwapFunded`, `SwapSecret`, `SwapComplete`, `SwapRefund`, `SwapAbort` - `swap.go`
- [x] Method-specific payloads:
  - MuSig2: `MuSig2Data` (pubkey, nonce)
  - HTLC Bitcoin: `HTLCBitcoinData` (pubkey, script, address)
  - HTLC EVM: `HTLCEVMData` (addresses, contract, token)
  - Adaptor XMR: `AdaptorXMRData` (btc/xmr keys, adaptor point, DLEQ proof)
  - HTLC Solana: `HTLCSolanaData` (pubkeys, program ID, escrow PDA)
- [x] Utility messages: `Ping`, `Pong`, `Error` with error codes - `util.go`
- [ ] Message validation and routing (TODO: integrate with P2P node)

#### 2.4.2 SQLite Schema (`internal/storage/`) ✅

**Orders Table** (simplified, all-or-nothing):
```sql
orders (
    id, peer_id, status,
    offer_chain, offer_amount,     -- What maker offers
    request_chain, request_amount, -- What maker wants (price = ratio)
    preferred_methods[],           -- ["musig2", "htlc"]
    created_at, expires_at,
    is_local,                      -- Our order or remote
    signature                      -- Proves ownership
)
```

**Trades Table** (links order to actual swap):
```sql
trades (
    id, order_id,
    maker_peer_id, taker_peer_id,
    method,                        -- Actual method used
    state,                         -- Overall trade state
    offer_amount, request_amount,  -- Actual amounts (may differ from order)
    created_at, completed_at,
    failure_reason
)
```

**Swap Legs Table** (each side of the swap tracked separately):
```sql
swap_legs (
    id, trade_id,
    leg_type,                      -- "offer" or "request"
    chain, amount,
    our_role,                      -- "sender" or "receiver"
    state,                         -- Leg-specific state
    funding_txid, funding_vout, funding_confirms,
    redeem_txid, refund_txid,
    timeout_height,
    method_data                    -- JSON: method-specific (see below)
)
```

**Method-Specific Data (JSON in swap_legs.method_data):**

```json
// MuSig2
{
    "type": "musig2",
    "local_privkey_enc": "...",    // Encrypted ephemeral key
    "local_pubkey": "...",
    "remote_pubkey": "...",
    "aggregated_pubkey": "...",
    "taproot_address": "...",
    "local_nonce": "...",
    "remote_nonce": "...",
    "nonce_used": false,
    "partial_sig": "..."
}

// HTLC (Bitcoin-family)
{
    "type": "htlc_bitcoin",
    "secret": "...",               // Only if we're initiator
    "secret_hash": "...",
    "sender_pubkey": "...",
    "receiver_pubkey": "...",
    "timelock_height": 123456,
    "htlc_script": "...",
    "htlc_address": "...",
    "redeem_script": "..."
}

// HTLC (EVM)
{
    "type": "htlc_evm",
    "secret": "...",
    "secret_hash": "...",
    "contract_address": "...",
    "token_address": "...",        // null for native ETH
    "sender_address": "...",
    "receiver_address": "...",
    "timelock_timestamp": 1234567890,
    "deploy_txid": "...",
    "claim_txid": "...",
    "refund_txid": "..."
}

// Adaptor Signatures (XMR)
{
    "type": "adaptor_xmr",
    "btc_pubkey": "...",
    "xmr_pubkey": "...",           // Ed25519
    "adaptor_point": "...",
    "dleq_proof": "...",
    "encrypted_signature": "...",
    "xmr_address": "...",
    "view_key_share": "..."
}

// Solana
{
    "type": "htlc_solana",
    "secret_hash": "...",
    "program_id": "...",
    "escrow_pda": "...",
    "sender_pubkey": "...",
    "receiver_pubkey": "...",
    "timelock_slot": 123456
}
```

**Secrets Table** (separate for security):
```sql
secrets (
    id, trade_id,
    secret_hash,
    secret,                        -- Only stored after reveal
    created_at, revealed_at
)
```

#### 2.4.3 Storage Operations ✅

- [x] Order CRUD - `orders.go` (CreateOrder, GetOrder, UpdateOrderStatus, ListOrders, DeleteOrder, GetOpenOrders, GetMyOrders, ExpireOldOrders, CountOrders)
- [x] Trade lifecycle - `trades.go` (CreateTrade, GetTrade, GetTradeByOrderID, UpdateTradeState, UpdateTradeFailure, ListTrades, GetActiveTrades, CountTrades, DeleteTrade)
- [x] Swap leg tracking - `swap_legs.go` (CreateSwapLeg, GetSwapLeg, GetSwapLegsByTradeID, GetSwapLegByTradeAndType, UpdateSwapLegState, UpdateSwapLegFunding, UpdateSwapLegConfirmations, UpdateSwapLegRedeemed, UpdateSwapLegRefunded, UpdateSwapLegMethodData, ListSwapLegs, GetPendingSwapLegs)
- [x] Secret management - `secrets.go` (CreateSecret, GetSecret, GetSecretByHash, GetSecretByTradeID, RevealSecret, RevealSecretByHash, ListSecretsByTrade, GetUnrevealedSecrets, HasSecretPreimage)
- [x] Unit tests (47 tests for storage operations)

### 2.5 Swap Coordinator (MuSig2 Testnet) ✅ FULLY WORKING

**Goal:** Execute a real MuSig2 atomic swap on BTC/LTC testnet. **ACHIEVED!**

**Existing Helpers (already implemented):**
- `swap/tx.go`: BuildFundingTx, BuildSpendingTx, SelectUTXOs, AddWitness, BuildRefundTx
- `swap/musig2.go`: MuSig2Session (key aggregation, nonces, signing)
- `swap/script.go`: BuildRefundScript, BuildTaprootScriptTree
- `wallet/`: DerivePrivateKey, DeriveAddress
- `backend/`: GetAddressUTXOs, BroadcastTransaction, GetTransaction
- `storage/`: Orders, Trades, SwapLegs, Swaps CRUD

#### 2.5.1 Swap Coordinator (`internal/swap/coordinator.go`) ✅

- [x] `Coordinator` struct - manages active swaps
- [x] State machine: init → funding → funded → signing → complete/refunded
- [x] `InitiateSwap()` - Start swap as maker (generates secret, ephemeral key)
- [x] `RespondToSwap()` - Join swap as taker
- [x] Key exchange: `SetRemotePubKey()`, `GetLocalPubKey()`, `GetTaprootAddress()`
- [x] Nonce exchange: `GenerateNonces()`, `SetRemoteNonce()`
- [x] Funding: `CreateFundingTx()`, `SetFundingTx()`
- [x] Confirmation tracking: `UpdateConfirmations()`
- [x] Signing: `CreatePartialSignature()`, `CombineSignatures()`
- [x] Completion: `CompleteSwap()`, `RefundSwap()`
- [x] Event system for swap lifecycle notifications
- [x] Storage serialization: `GetMuSig2StorageData()`
- [x] Unit tests (20+ tests for coordinator)

#### 2.5.2 Confirmation Monitor (`internal/swap/monitor.go`) ✅

- [x] Poll backends for funding tx confirmations
- [x] Update swap confirmations via coordinator
- [x] WaitForConfirmations() helper for blocking waits
- [x] GetConfirmations() for instant checks
- [x] Configurable polling interval

#### 2.5.3 P2P Message Handler (`internal/node/swap_handler.go`) ✅

- [x] Register PubSub topic for swap messages (`/klingon/swap/1.0.0`)
- [x] SwapHandler with OnMessage() for registering handlers
- [x] SendMessage() and SendToPeer() for outgoing messages
- [x] Message types: order_announce, order_take, pubkey_exchange, nonce_exchange, funding_info, partial_sig, etc.
- [x] Auto-start with node, cleanup on shutdown

#### 2.5.4 RPC Methods (`internal/rpc/`) ✅

- [x] `orders_create` - Create and announce order
- [x] `orders_list` - List orders with filters
- [x] `orders_get` - Get single order
- [x] `orders_cancel` - Cancel own order
- [x] `orders_take` - Take an order, start swap
- [x] `trades_list` - List active/completed trades
- [x] `trades_get` - Get single trade with legs
- [x] `trades_status` - Get detailed trade status with next action
- [x] `swap_list` - List all swaps (active + historical)
- [x] `swap_recover` - Recover swap from database
- [x] `swap_timeout` - Get timeout info for a swap
- [x] `swap_refund` - Force refund if timeout reached
- [x] `swap_checkTimeouts` - Check all pending swaps for timeouts

#### 2.5.5 Swap State Persistence & Timelock Refunds ✅ COMPLETED

**Goal:** Survive node restarts and enable refunds if counterparty disappears.

- [x] `storage/swaps.go` - CRUD for active swap persistence
  - SaveSwap, GetSwap, GetPendingSwaps, GetSwapsNearingTimeout, GetSwapsPastTimeout
  - UpdateSwapState, UpdateSwapMethodData, UpdateSwapFunding, DeleteSwap, ListSwaps
- [x] Extended MuSig2StorageData - Full session recovery data
  - Local private key (encrypted), used nonces, nonce_used flag, session_invalid
  - Refund script, timeout blocks, control block for script path spending
- [x] `swap/script.go` - Taproot script tree with timelock refunds
  - BuildRefundScript: `<timeout_blocks> OP_CSV OP_DROP <pubkey> OP_CHECKSIG`
  - BuildTaprootScriptTree: 2-path tree (key path + refund script path)
  - Control block generation for script path proofs
- [x] Updated TaprootAddress generation - Now includes script tree (not just key path)
- [x] `swap/tx.go` - BuildRefundTx for script path spending
  - RefundTxParams struct, CSV sequence setting, script path witness
- [x] Timeout monitoring in coordinator
  - CheckTimeouts(), StartTimeoutMonitor(), GetSwapTimeoutInfo(), ForceRefund()
  - Auto-refund when timeout reached
- [x] Unit tests for persistence and scripts (script_test.go, swaps_test.go)
- [x] **BUG FIX:** Dual-chain timeout storage (request_timeout_height column)
  - Previously only stored one timeout_height, causing LTC timeout to be 0
  - Now stores both offer chain and request chain timeouts separately
- [x] **BUG FIX:** Taproot addresses now include refund script path
  - Changed coordinator to use `TaprootAddressWithRefund()` instead of `TaprootAddress()`
  - Addresses now have 2-path tree: key path (MuSig2) + refund script path (CSV timelock)
- [x] **BUG FIX:** CombineSignatures nil pointer dereference
  - Fixed uninitialized `secp256k1.ModNScalar` when parsing remote partial signature
  - Changed from `var remoteSig musig2.PartialSignature` to properly initialized scalar
- [x] **BUG FIX:** MuSig2 key ordering in InitSigningSession
  - Keys must be sorted for both parties to produce valid combined signatures
  - Added lexicographic key sorting to match `computeAggregatedKey()` behavior
- [x] **BUG FIX:** RemotePartialSig not persisted/restored
  - Added `RemotePartialSig` field to `MuSig2SwapData` and `MuSig2StorageData` structs
  - Save counterparty's partial signature in `getMuSig2StorageDataUnlocked()`
  - Restore `RemotePartialSig` in `recoverSwapFromRecord()` for node restart recovery
  - Exposed `local_partial_sig` and `remote_partial_sig` in `swap_status` RPC response

**Timeout Values:**
| Chain | Maker Blocks | Taker Blocks | ~Time |
|-------|-------------|--------------|-------|
| BTC   | 144         | 72           | 24h/12h |
| LTC   | 576         | 288          | 24h/12h |

#### 2.5.6 Integration Testing ✅ COMPLETED

- [x] Manual testnet swap_init between two nodes (MuSig2 key exchange working)
- [x] Both nodes compute identical taproot address: verified
- [x] Complete funding transaction flow
- [x] Complete signing flow
- [x] Complete redemption flow
- [x] Verify refund path works on timeout

**✅ SUCCESSFUL TESTNET SWAP (2024-12-17):**
- Trade: 10,000 BTC sats ↔ 100,000 LTC litoshis
- Node 1 (Maker): Sent BTC, received 98,890 LTC litoshis
- Node 2 (Taker): Sent LTC, received 8,890 BTC sats
- Both redemption transactions broadcast successfully

**✅ SUCCESSFUL CSV TIMELOCK REFUND (2024-12-19):**
- Trade ID: `3ddc624d-3ad4-47ce-a427-c3b1541dc5b8`
- Test: 15,000 BTC sats ↔ 150,000 LTC litoshis, counterparty abandoned swap
- BTC refund TX: `a40bcb855229a73abfc01d9687ef0268b569c0ed49b18e29b96e4dba63ea14e9`
- Refunded: 14,710 sats (after mining fees) back to original wallet
- Bug fixes applied:
  1. `tx.go`: Changed tx version from 1 to 2 (required for BIP 68 CSV)
  2. `coordinator.go`: Added wallet integration for deriving refund addresses
  3. `service.go`: Added `GetWallet()` method to expose wallet to coordinator
  4. `wallet_handlers.go`: Set wallet on coordinator after unlock/create

#### 2.5.7 Test Maintenance ✅ FIXED

**All tests now pass:**
```
ok  	internal/backend
ok  	internal/chain
ok  	internal/config
ok  	internal/node
ok  	internal/rpc
ok  	internal/storage
ok  	internal/swap
ok  	internal/wallet
ok  	pkg/helpers
```

**Fixes applied:**
- Updated `coordinator_test.go` for dual-chain API changes
- Updated `swap_test.go` confirmation tests to use Mainnet (testnet has MinConfirmations=0)
- Updated `tx_test.go` to require TaprootAddress parameter

### 2.6 Future Atomic Swap Enhancements

- [ ] Adaptor signatures for Monero swaps
- [ ] HTLC fallback for non-Taproot chains
- [ ] EVM HTLC smart contract

#### 2.6.1 Reliable Message Delivery ✅ COMPLETED (2025-12-21)

- [x] **Hybrid Delivery System** - Guaranteed message delivery
  - Strategy: Direct stream (fast) → DHT lookup → Encrypted PubSub (fallback)
  - Messages reach peers even without direct connection
  - Works across the entire P2P network

- [x] **Direct P2P Messaging** - Private streams for swap messages
  - Protocol: `/klingon/swap/direct/1.0.0`
  - Length-prefixed message framing
  - ACK-based delivery confirmation
  - Files: `stream_handler.go`, `message_sender.go`, `peer_monitor.go`

- [x] **Encrypted PubSub Fallback** - Messages through gossip network
  - Topic: `/klingon/swap/encrypted/1.0.0`
  - End-to-end encryption using NaCl box (X25519 + XSalsa20-Poly1305)
  - Forward secrecy via ephemeral keys per message
  - Only recipient can decrypt (broadcast to all, read by one)
  - Files: `crypto.go`, `swap_handler.go` (processEncryptedMessages)

- [x] **DHT Peer Discovery** - Connect to peers via DHT
  - Automatic DHT lookup when peer not directly connected
  - 30s timeout for peer lookup, 15s for connection attempt
  - Falls back to encrypted PubSub if connection fails

- [x] **Message Persistence & Retry**
  - SQLite outbox/inbox pattern (`storage/message_queue.go`)
  - Exponential backoff: 10s → 20s → 40s → ... → 10min max
  - MaxRetries: 50 (~7.5 hours of retry time)
  - Auto-flush pending messages when peer reconnects

- [x] **Message Cleanup**
  - Periodic cleanup job (hourly)
  - 7-day retention for completed/failed messages
  - Files: `retry_worker.go`

- [x] **Unit Tests** - Full test coverage
  - `message_queue_test.go` (12 tests)
  - `stream_handler_test.go` (12 tests)
  - `message_sender_test.go` (8 tests)
  - `retry_worker_test.go` (9 tests)
  - `crypto_test.go` (5 tests) - encryption round-trip, wrong recipient, validation

#### 2.6.2 Order Management ✅ COMPLETED (2025-12-21)

- [x] **Broadcast Order Cancellations** - PubSub notification when orders cancelled
- [x] **Track Order/Peer IDs in Storage** - Proper swap recovery with trade context

#### 2.6.3 Auto-Funding & Dynamic Fees ✅ COMPLETED (2025-12-20)

- [x] **Sign Funding Transactions** - Auto-fund with `swap_fund` RPC
  - New `FundSwap()` coordinator method
  - New `swap_fund` RPC handler
  - Scans wallet UTXOs, signs, broadcasts, sets funding info automatically
  - Test scripts updated (`test-swap.sh`, `test-htlc-swap.sh`)

- [x] **Dynamic Fee Rate** - Uses `backend.GetFeeEstimates()` with HalfHourFee
  - `getFeeRateForChain()` helper in RPC handlers
  - Falls back to `HourFee` or default (10 sat/vB) if unavailable

#### 2.6.3.1 EVM ↔ Bitcoin Cross-Chain Swaps ✅ COMPLETED (2025-12-23)

- [x] **Cross-Chain Swap Types** - BTC↔EVM, EVM↔BTC, EVM↔EVM, BTC↔BTC
  - Added `swap_getSwapType` RPC method
  - Extended `swap_status` with `swap_type`, `method`, `offer_htlc_address`, `request_htlc_address`, `offer_evm_address`, `request_evm_address` fields
- [x] **HTLC Address Generation** - Fixed for both cross-chain directions
  - `respondBitcoinToEVMSwap()` generates HTLC address for BTC→EVM (offer chain)
  - `respondEVMToBitcoinSwap()` generates HTLC address for ETH→BTC (request chain)
  - Proper sender/receiver role assignment based on direction
- [x] **Test Script** - `test-evm-btc-swap.sh` for bidirectional swaps
  - Supports both BTC→ETH and ETH→BTC via `SWAP_DIRECTION` env var
  - Wait for pubkey exchange with retry loop
  - Proper timing between initialization and funding
  - Commands: setup, fund-btc, create-eth, claim-eth, claim-btc, full

#### 2.6.4 EVM Wallet Support ✅ COMPLETED (2025-12-20)

- [x] **Full EVM Wallet Support** - Native ETH/ERC-20 transactions
  - New files:
    - `internal/wallet/evm_tx.go` - EVM transaction building (RLP encoding, signing)
    - `internal/wallet/service_evm.go` - EVM service methods
    - `pkg/helpers/hex.go` - Hex conversion utilities
  - New backend methods: `EVMGetNonce()`, `EVMEstimateGas()`, `EVMGetGasPrice()`, `EVMCall()`, `EVMGetChainID()`
  - New RPC methods: `wallet_sendEVM`, `wallet_sendERC20`, `wallet_getERC20Balance`, `wallet_getChainType`
  - Supports legacy (type 0) and EIP-1559 (type 2) transactions

#### 2.6.5 Code Quality ✅ COMPLETED (2025-12-19)

- [x] **Refactor coordinator.go** (2786 lines) - Split into 12 files
- [x] **Refactor rpc/swap.go** (2103 lines) - Split into 11 files
- [x] **Proper Address Index Management** - Tracks used indices in `wallet_addresses` table

## Phase 3: Orders & Trading ✅ COMPLETED

Simple buy/sell offers - no full orderbook needed.

- [x] `orders_create` - Create and announce order
- [x] `orders_take` - Take an order, start swap
- [x] `orders_list` - List orders with filters
- [x] `orders_cancel` - Cancel own order
- [x] `trades_list` - List active/completed trades
- [x] `trades_status` - Get detailed trade status
- [x] WebSocket events for order/trade updates
- [x] Trade state machine (in coordinator)
- [x] P2P trade messages (swap_handler + protocol/)
- [x] Trade persistence (storage/trades.go)

**Design Decision:** All-or-nothing orders only (no partial fills, no price-time matching). Price is implicit from offer_amount/request_amount ratio.

## Phase 4: Security & Production Readiness

### 4.1 Critical - Pre-Production

These MUST be completed before mainnet launch.

- [ ] **Set Mainnet DAO Addresses** (`internal/config/config.go:295-302`)
  - [ ] BTC mainnet DAO address
  - [ ] LTC mainnet DAO address
  - [ ] DOGE mainnet DAO address
  - [ ] XMR mainnet DAO address
  - [ ] EVM mainnet DAO address (shared across all EVM chains)
  - [ ] SOL mainnet DAO address

### 4.2 Security Audit

- [ ] Security audit of swap logic
- [ ] Fuzz testing for protocol handlers
- [ ] Production documentation

## Decisions Made

1. **Supported coins:** BTC, LTC, DOGE, XMR, ETH, BSC, MATIC, ARB, SOL

2. **Swap method:** MuSig2 priority, HTLC fallback (no Taproot support)

3. **Fees:** 0.2% maker + 0.2% taker = 0.4% total per trade

4. **Fee distribution:** 50% DAO / 50% Node operators (rules TBD)

5. **DAO addresses:** Configured in `internal/config/config.go` (testnet and mainnet variants)

6. **Order simplicity:** All-or-nothing orders only (no partial fills)
   - Each order must be taken in full
   - Price is implicit: offer_amount/request_amount ratio
   - No price_type field (all orders are fixed price)
   - Simpler for atomic swaps where each trade is a separate swap

7. **Timelock refunds (already implemented):**
   - Taproot script tree with 2 spending paths:
     - Key path: MuSig2 aggregated signature (happy path)
     - Script path: CSV refund after timeout (if counterparty disappears)
   - Refund script: `<timeout_blocks> OP_CHECKSEQUENCEVERIFY OP_DROP <pubkey> OP_CHECKSIG`
   - Timeout values: BTC (144/72 blocks ~24h/12h), LTC (576/288 blocks ~24h/12h)
   - Auto-monitoring via `CheckTimeouts()` and `StartTimeoutMonitor()`
   - RPC: `swap_timeout`, `swap_refund`, `swap_checkTimeouts`

8. **No orderbook - simple buy/sell offers only:**
   - Users create offers with explicit amounts (no price matching)
   - Other users take offers as-is (no partial fills)
   - No price-time priority, no market orders
   - Simpler, more predictable for atomic swaps

## Open Questions

1. **Node operator reward distribution** - How to track and distribute 50% to operators?

2. ~~**Monero atomic swaps** - Adaptor signature implementation details~~ ✅ **RESEARCHED**
   - See `docs/monero-atomic-swaps.md` for comprehensive technical details
   - Uses adaptor signatures (not HTLCs) due to lack of scripting
   - Requires DLEQ proofs for cross-curve (ed25519 ↔ secp256k1) verification
   - Go libraries: `github.com/athanorlabs/atomic-swap` and `github.com/athanorlabs/go-dleq`
   - Rust reference: COMIT Network `xmr-btc-swap`

3. **EVM contract** - Deploy same contract to all EVM chains?

4. **Solana program** - Native or use existing swap program?

## Notes

- All config values go in `internal/config/config.go`
- No hardcoded values in business logic
- Unit tests required for every new feature
- Crypto-to-crypto only, no fiat support
