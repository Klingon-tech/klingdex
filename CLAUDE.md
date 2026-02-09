# Klingon-v2 Developer Guide

## Development Rules

**EVERY NEW FEATURE OR CHANGE MUST INCLUDE UNIT TESTS.**
- Tests are located in `*_test.go` files alongside the code
- Run `go test ./...` before committing any changes
- Test coverage should include both success and error cases

**NO HARDCODED VALUES SCATTERED AROUND THE CODEBASE.**
- ALL configuration values (coins, addresses, fees, timeouts, etc.) MUST be defined in `internal/config/config.go`
- Use constants and config structs with mainnet/testnet variants
- Never hardcode addresses, amounts, or chain-specific values in business logic
- If you need a value, add it to config.go first

**CRYPTO-TO-CRYPTO ONLY — NO FIAT.**

**NEVER USE FLOATS — PREFER BIG INTS OR DECIMALS**

**ALWAYS WRITE `TODO` IN COMMENTS IF ANY FEATURE NEEDS TO BE COMPLETED LATER**

**TESTNET TESTING**
- Use `./scripts/test-swap.sh` to set up test nodes — it handles wallet restoration
- Test credentials are loaded from environment variables (see `scripts/.env.example`)
- Never commit real mnemonics, passwords, or funded wallet details

## Project Overview

Klingon-v2 is a P2P atomic swap exchange for crypto-to-crypto trades. Key principles:

- **P2P networking** via libp2p (DHT, PubSub, mDNS)
- **Atomic swaps** — trustless cross-chain trading
- **No fiat** — crypto-to-crypto only
- **Peer persistence** via SQLite
- **JSON-RPC API** for external control
- **WebSocket** for real-time events
- **Centralized config** — all params in config.go (mainnet/testnet)

## Architecture

```
klingon-v2/
├── cmd/klingond/main.go        # Daemon entry point
├── internal/
│   ├── config/                 # Exchange configuration (ALL params here)
│   ├── chain/                  # Chain parameters (USE THIS for chain info)
│   ├── backend/                # Blockchain APIs (USE THIS for blockchain ops)
│   ├── wallet/                 # Key management (USE THIS for keys/signing)
│   ├── node/                   # P2P node (libp2p, DHT, PubSub, mDNS)
│   ├── rpc/                    # JSON-RPC 2.0 server + WebSocket
│   ├── storage/                # SQLite persistence
│   ├── swap/                   # Atomic swap (MuSig2, HTLC, state machine)
│   ├── contracts/              # EVM smart contract integration
│   └── sync/                   # Order and trade sync services
├── contracts/                  # Solidity smart contracts (Foundry)
├── pkg/
│   ├── helpers/                # Common utilities (USE THIS)
│   └── logging/                # Structured logging
├── scripts/                    # Integration test scripts
├── docs/                       # Documentation
└── Makefile
```

## Supported Coins

| Symbol | Type | Native Token | Swap Method |
|--------|------|--------------|-------------|
| BTC | Bitcoin | BTC | MuSig2 / HTLC |
| LTC | Bitcoin fork | LTC | MuSig2 / HTLC |
| BCH | Bitcoin fork | BCH | HTLC |
| DOGE | Bitcoin fork | DOGE | HTLC |
| XMR | Monero | XMR | Adaptor signatures |
| ETH | EVM (chainID 1) | ETH | Smart contract |
| BSC | EVM (chainID 56) | BNB | Smart contract |
| POLYGON | EVM (chainID 137) | POL | Smart contract |
| ARBITRUM | EVM (chainID 42161) | ETH | Smart contract |
| OPTIMISM | EVM (chainID 10) | ETH | Smart contract |
| BASE | EVM (chainID 8453) | ETH | Smart contract |
| AVAX | EVM (chainID 43114) | AVAX | Smart contract |
| SOL | Solana | SOL | Program |

## Fee Structure

- **Maker fee:** 0.2% (20 basis points)
- **Taker fee:** 0.2% (20 basis points)
- **Distribution:** 50% DAO / 50% Node operators

## Swap Method Priority

1. **MuSig2** (Taproot) — preferred, more private
2. **HTLC** — fallback if no Taproot support

## Existing Infrastructure — DO NOT REINVENT

**Before writing new code, check if functionality already exists.**

### `internal/chain/` — Chain Parameters

```go
params, ok := chain.Get("BTC", chain.Testnet)
params.SupportsTaproot  // true/false
params.Bech32HRP        // "bc", "ltc", "tltc" etc.
params.Type             // chain.ChainTypeBitcoin, chain.ChainTypeEVM, etc.

bitcoinChains := chain.ListByType(chain.ChainTypeBitcoin)  // ["BTC", "LTC", "DOGE"]
chain.IsSupported("BTC")  // true
```

### `internal/backend/` — Blockchain APIs

```go
cfg := backend.DefaultConfigs()["BTC"]
b := backend.NewMempoolBackend(cfg.TestnetURL)
b.Connect(ctx)

utxos, _ := b.GetAddressUTXOs(ctx, address)
info, _ := b.GetAddressInfo(ctx, address)
txid, _ := b.BroadcastTransaction(ctx, rawHex)
fees, _ := b.GetFeeEstimates(ctx)
tx, _ := b.GetTransaction(ctx, txid)
height, _ := b.GetBlockHeight(ctx)
```

### `internal/wallet/` — Key Management & Signing

```go
w, _ := wallet.NewFromMnemonic(mnemonic, "", chain.Testnet)

privKey, _ := w.DerivePrivateKey("BTC", account, index)
pubKey, _ := w.DerivePublicKey("LTC", account, index)
address, _ := w.DeriveAddress("BTC", account, index)

addrs, _ := wallet.AllAddressTypes(pubKey, params)
```

### `internal/config/` — Exchange Configuration

```go
cfg := config.NewExchangeConfig(config.Testnet)

fees := config.DefaultFeeConfig()
fees.CalculateFee(amount, isMaker)
fees.CalculateDAOShare(feeAmount)

daoAddr := cfg.GetDAOAddress("BTC")
chainParams, _ := cfg.GetChainParams("BTC")
```

### `pkg/helpers/` — Common Utilities

```go
helpers.FormatAmount(100000000, 8)    // "1" (BTC)
helpers.ParseAmount("0.5", 8)         // 50000000
helpers.SatoshisToBTC(100000000)      // "1"
helpers.BTCToSatoshis("1")            // 100000000
helpers.WeiToETH(1000000000000000000) // "1"
helpers.ETHToWei("1")                 // 1000000000000000000
```

### Integration Rules

1. **For UTXOs**: Use `backend.Backend.GetAddressUTXOs()` directly
2. **For broadcasting**: Use `backend.Backend.BroadcastTransaction()` directly
3. **For fees**: Use `backend.Backend.GetFeeEstimates()` or `config.DefaultFeeConfig()`
4. **For key derivation**: Use `wallet.Wallet.DerivePrivateKey()`
5. **For addresses**: Use `wallet.DeriveAddress()` or `wallet.AllAddressTypes()`
6. **For chain info**: Use `chain.Get(symbol, network)`
7. **For DAO addresses**: Use `config.ExchangeConfig.GetDAOAddress()`

### What NEW packages should contain

New packages should ONLY contain:
- Protocol-specific logic (MuSig2 aggregation, nonce exchange, adaptor signatures)
- Transaction structure (what outputs to create, timelocks)
- State machine (swap states, transitions)
- P2P messages (protocol messages between peers)

They should NOT contain:
- UTXO fetching wrappers
- Fee calculation wrappers
- Broadcast wrappers
- Key derivation wrappers
- Hardcoded chain names or values

## RPC API Quick Reference

**Use these exact parameter names. Do NOT guess parameter names.**

### Wallet Methods

```bash
# Check wallet status
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_status","id":1}'

# Create/restore wallet from mnemonic
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_create","params":{"mnemonic":"word1 word2...","password":"your_password"},"id":1}'

# Get address — PARAM IS "symbol" NOT "chain"
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"BTC"},"id":1}'

# Get balance — REQUIRES "symbol" AND "address"
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_getBalance","params":{"symbol":"BTC","address":"bc1q..."},"id":1}'

# Send transaction
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_send","params":{"symbol":"BTC","to":"bc1q...","amount":50000,"fee_rate":2},"id":1}'
```

### Multi-Address Wallet Methods

```bash
# List ALL UTXOs from all addresses
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_listAllUTXOs","params":{"symbol":"BTC"},"id":1}'

# Get aggregated balance across ALL addresses
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_getAggregatedBalance","params":{"symbol":"BTC"},"id":1}'

# Send from ALL addresses (aggregates UTXOs)
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_sendAll","params":{"symbol":"BTC","to":"bc1q...","amount":200000},"id":1}'

# Send ENTIRE wallet balance
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_sendMax","params":{"symbol":"BTC","to":"bc1q..."},"id":1}'
```

**Send method choice:**
- `wallet_send` — Single address (UTXO chains)
- `wallet_sendAll` — Aggregates from all addresses
- `wallet_sendMax` — Empties entire wallet
- `wallet_sendEVM` — Native EVM token (ETH, BNB, etc.)
- `wallet_sendERC20` — ERC-20 tokens

### EVM Wallet Methods

```bash
# Send native EVM token — amount in wei
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_sendEVM","params":{"symbol":"ETH","to":"0x...","amount":"1000000000000000000"},"id":1}'

# Send ERC-20 token
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_sendERC20","params":{"symbol":"ETH","token":"0x...","to":"0x...","amount":"1000000"},"id":1}'

# Get ERC-20 balance
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"wallet_getERC20Balance","params":{"symbol":"ETH","token":"0x...","address":"0x..."},"id":1}'
```

### Order & Trade Methods

```bash
# Create order (maker)
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"orders_create","params":{"offer_chain":"BTC","offer_amount":50000,"request_chain":"LTC","request_amount":500000,"preferred_methods":["musig2"]},"id":1}'

# Take order (taker)
curl -s http://127.0.0.1:8081 -d '{"jsonrpc":"2.0","method":"orders_take","params":{"order_id":"uuid...","preferred_method":"musig2"},"id":1}'

# List trades
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"trades_list","id":1}'
```

### Swap Methods

```bash
# Initialize swap (both nodes)
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_init","params":{"trade_id":"..."},"id":1}'

# Auto-fund swap from wallet
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_fund","params":{"trade_id":"..."},"id":1}'

# Check funding status
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_checkFunding","params":{"trade_id":"..."},"id":1}'

# Exchange nonces → Sign → Redeem
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_exchangeNonce","params":{"trade_id":"..."},"id":1}'
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_sign","params":{"trade_id":"..."},"id":1}'
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_redeem","params":{"trade_id":"..."},"id":1}'

# Refund (after timeout)
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_refund","params":{"trade_id":"..."},"id":1}'

# HTLC-specific
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_htlcRevealSecret","params":{"trade_id":"..."},"id":1}'
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_htlcClaim","params":{"trade_id":"...","chain":"LTC"},"id":1}'
```

## Testing

```bash
# Run all tests
go test ./...

# Run specific package
go test -v ./internal/swap/
go test -v ./internal/wallet/
go test -v ./internal/rpc/
```

### Integration Test Scripts

| Script | Purpose |
|--------|---------|
| `./scripts/test-swap.sh` | MuSig2 (Taproot) atomic swap |
| `./scripts/test-htlc-swap.sh` | HTLC (P2WSH) atomic swap |
| `./scripts/test-refund.sh` | MuSig2 refund path |
| `./scripts/test-htlc-refund.sh` | HTLC refund path |
| `./scripts/test-evm-swap.sh` | EVM HTLC swap |
| `./scripts/test-evm-refund.sh` | EVM refund path |
| `./scripts/test-evm-btc-swap.sh` | Cross-chain ETH ↔ BTC |

Scripts load credentials from `scripts/.env`. Copy `scripts/.env.example` and fill in your testnet wallet details.

## Key Dependencies

- `github.com/libp2p/go-libp2p` — P2P networking
- `github.com/btcsuite/btcd` — Bitcoin primitives, MuSig2
- `github.com/ethereum/go-ethereum` — EVM support
- `github.com/mattn/go-sqlite3` — SQLite driver
- `github.com/charmbracelet/log` — Structured logging
- `github.com/gorilla/websocket` — WebSocket support

## References

- https://github.com/lightninglabs/taproot-assets/blob/main/itest/multisig.go
- https://github.com/bisq-network/bisq-musig
- https://bitcoinops.org/en/topics/musig/
- https://pkg.go.dev/github.com/btcsuite/btcd/btcec/v2/schnorr/musig2
