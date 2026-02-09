# Klingon Exchange

A decentralized peer-to-peer cryptocurrency exchange that enables trustless cross-chain atomic swaps. No intermediaries, no custody, no fiat — just crypto-to-crypto trading.

Built on [libp2p](https://libp2p.io) with an HD wallet, multi-chain support, and a JSON-RPC API.

## Features

- **Atomic Swaps** — Trustless cross-chain trading via MuSig2 (Taproot) and HTLC
- **P2P Networking** — Decentralized peer discovery using Kademlia DHT and mDNS
- **HD Wallet** — BIP39/BIP44/BIP84/BIP86 key derivation for all supported chains
- **Multi-Chain** — Bitcoin, Litecoin, Dogecoin, Ethereum, and 6 more EVM chains
- **EVM Smart Contracts** — HTLC contracts for ETH/ERC-20 atomic swaps
- **Multiple Backends** — Mempool.space, Esplora, Electrum, Blockbook, JSON-RPC
- **JSON-RPC 2.0 API** — Full node control over HTTP and WebSocket
- **NAT Traversal** — UPnP, NAT-PMP, and hole punching
- **Peer Persistence** — SQLite-backed storage with smart reconnection

## Supported Chains

| Chain | Symbol | Type | Swap Method |
|-------|--------|------|-------------|
| Bitcoin | BTC | UTXO | MuSig2 / HTLC |
| Litecoin | LTC | UTXO | MuSig2 / HTLC |
| Dogecoin | DOGE | UTXO | HTLC |
| Bitcoin Cash | BCH | UTXO | HTLC |
| Ethereum | ETH | EVM | Smart Contract |
| BNB Smart Chain | BSC | EVM | Smart Contract |
| Polygon | POLYGON | EVM | Smart Contract |
| Arbitrum | ARBITRUM | EVM | Smart Contract |
| Optimism | OPTIMISM | EVM | Smart Contract |
| Base | BASE | EVM | Smart Contract |
| Avalanche | AVAX | EVM | Smart Contract |
| Solana | SOL | Solana | Program (planned) |
| Monero | XMR | Monero | Adaptor Signatures (planned) |

## Quick Start

### Requirements

- Go 1.24+
- GCC (for SQLite CGO bindings)

### Build & Run

```bash
git clone https://github.com/user/klingon-v2.git
cd klingon-v2
make build

# Run on mainnet (default)
./bin/klingond

# Run on testnet
./bin/klingond --testnet

# Custom settings
./bin/klingond --listen /ip4/0.0.0.0/tcp/5001 --api 127.0.0.1:9000 --log-level debug
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `~/.klingon` | Data directory for keys, config, and database |
| `--config` | `<data-dir>/config.yaml` | Config file path |
| `--listen` | *(from config)* | P2P listen address (multiaddr) |
| `--api` | `127.0.0.1:8080` | JSON-RPC API address |
| `--mdns` | `true` | Enable mDNS local discovery |
| `--dht` | `true` | Enable DHT discovery |
| `--testnet` | `false` | Run on testnet (separate network) |
| `--bootstrap` | `""` | Bootstrap peers (comma-separated) |
| `--log-level` | `info` | Log level: debug, info, warn, error |
| `--version` | — | Show version and exit |

## Architecture

```
klingon-v2/
├── cmd/klingond/              # Daemon entry point
├── internal/
│   ├── backend/               # Blockchain API backends (Mempool, Esplora, Electrum, Blockbook, JSON-RPC)
│   ├── chain/                 # Chain parameters and derivation paths
│   ├── config/                # Exchange configuration (fees, limits, addresses)
│   ├── contracts/             # EVM smart contract integration (HTLC)
│   ├── node/                  # libp2p node (DHT, PubSub, mDNS, direct messaging)
│   ├── rpc/                   # JSON-RPC 2.0 server + WebSocket
│   ├── storage/               # SQLite persistence (peers, orders, trades, UTXOs)
│   ├── swap/                  # Atomic swap engine (MuSig2, HTLC, state machine)
│   ├── sync/                  # Order and trade sync services
│   └── wallet/                # HD wallet (BIP39/44/84/86, multi-address, EVM)
├── contracts/                 # Solidity smart contracts (Foundry)
├── pkg/
│   ├── helpers/               # Common utilities (amount formatting, byte ops)
│   └── logging/               # Structured logging
├── scripts/                   # Integration test scripts
└── docs/                      # Technical documentation
```

## How Atomic Swaps Work

Klingon supports two swap methods with automatic fallback:

### MuSig2 (Taproot) — Preferred

Uses Schnorr signature aggregation for maximum privacy. The swap looks like a regular single-signature transaction on-chain.

1. Both parties generate ephemeral keys and compute a shared Taproot address
2. Each party funds their respective escrow address
3. MuSig2 nonces and partial signatures are exchanged via P2P
4. Combined signatures unlock both escrows atomically
5. If either party disappears, CSV timelocks enable refunds

### HTLC (Hash Time-Locked Contracts) — Fallback

Standard hash-locked contracts for chains without Taproot support or for cross-chain swaps involving EVM chains.

1. Initiator generates a secret and creates an HTLC on their chain
2. Responder creates a matching HTLC (shorter timelock) on their chain
3. Initiator claims the responder's HTLC, revealing the secret
4. Responder uses the revealed secret to claim the initiator's HTLC
5. If either party disappears, timelocks enable refunds

## JSON-RPC API

The node exposes a JSON-RPC 2.0 API over HTTP and WebSocket.

- **HTTP**: `POST http://127.0.0.1:8080/`
- **WebSocket**: `ws://127.0.0.1:8080/ws`

### Node & Peers

| Method | Description |
|--------|-------------|
| `node_info` | Get node info (peer ID, addresses, uptime) |
| `node_status` | Get node status |
| `peers_list` | List connected peers |
| `peers_count` | Get connected/known peer counts |
| `peers_connect` | Connect to a peer by multiaddr |
| `peers_disconnect` | Disconnect from a peer |
| `peers_known` | List known peers from database |

### Wallet

| Method | Description |
|--------|-------------|
| `wallet_status` | Check wallet status (exists, unlocked) |
| `wallet_generate` | Generate new 24-word mnemonic |
| `wallet_create` | Create/restore wallet from mnemonic |
| `wallet_unlock` | Unlock wallet with password |
| `wallet_lock` | Lock wallet |
| `wallet_getAddress` | Get address for a chain |
| `wallet_getAllAddresses` | Get all address types for a chain |
| `wallet_getPublicKey` | Get public key |
| `wallet_getBalance` | Get address balance |
| `wallet_getAggregatedBalance` | Get total balance across all addresses |
| `wallet_listAllUTXOs` | List all UTXOs with derivation paths |
| `wallet_send` | Send from single address (UTXO chains) |
| `wallet_sendAll` | Send aggregating UTXOs from all addresses |
| `wallet_sendMax` | Send entire wallet balance |
| `wallet_sendEVM` | Send native EVM token (ETH, BNB, etc.) |
| `wallet_sendERC20` | Send ERC-20 tokens |
| `wallet_getERC20Balance` | Get ERC-20 token balance |
| `wallet_getFeeEstimates` | Get fee estimates |
| `wallet_syncUTXOs` | Force UTXO sync to database |
| `wallet_supportedChains` | List supported chains |
| `wallet_validateMnemonic` | Validate a mnemonic phrase |

### Orders & Trades

| Method | Description |
|--------|-------------|
| `orders_create` | Create and broadcast an order |
| `orders_list` | List orders |
| `orders_get` | Get order details |
| `orders_cancel` | Cancel own order |
| `orders_take` | Take an order (starts swap) |
| `trades_list` | List trades |
| `trades_get` | Get trade details |
| `trades_status` | Get detailed trade status |

### Swaps

| Method | Description |
|--------|-------------|
| `swap_init` | Initialize swap (key exchange) |
| `swap_getAddress` | Get escrow address for funding |
| `swap_fund` | Auto-fund swap from wallet |
| `swap_setFunding` | Set funding info manually |
| `swap_checkFunding` | Check funding confirmations |
| `swap_exchangeNonce` | Exchange MuSig2 nonces |
| `swap_sign` | Exchange partial signatures |
| `swap_redeem` | Complete swap (broadcast) |
| `swap_refund` | Refund after timeout |
| `swap_status` | Get swap status |
| `swap_list` | List all swaps |
| `swap_recover` | Recover swap from database |
| `swap_timeout` | Get timeout info |
| `swap_checkTimeouts` | Check all swaps for timeouts |
| `swap_htlcRevealSecret` | Reveal HTLC secret (initiator) |
| `swap_htlcClaim` | Claim HTLC output |
| `swap_htlcRefund` | Refund HTLC after timeout |
| `swap_htlcExtractSecret` | Extract secret from claim tx |

### WebSocket Events

Connect to `ws://127.0.0.1:8080/ws` and subscribe:

```json
{"action": "subscribe", "events": ["peer_connected", "order_created", "trade_started"]}
```

Events: `peer_connected`, `peer_disconnected`, `order_created`, `order_cancelled`, `order_received`, `trade_started`, `swap_refunded`

### Example: Full Swap Flow

```bash
# Node 1: Create an order (offer 50k sats BTC for 500k litoshis LTC)
curl -s http://127.0.0.1:8080 -d '{
  "jsonrpc":"2.0","method":"orders_create",
  "params":{"offer_chain":"BTC","offer_amount":50000,"request_chain":"LTC","request_amount":500000},
  "id":1
}'

# Node 2: Take the order
curl -s http://127.0.0.1:8081 -d '{
  "jsonrpc":"2.0","method":"orders_take",
  "params":{"order_id":"ORDER_ID"},
  "id":1
}'

# Both nodes: Initialize swap
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_init","params":{"trade_id":"TRADE_ID"},"id":1}'
curl -s http://127.0.0.1:8081 -d '{"jsonrpc":"2.0","method":"swap_init","params":{"trade_id":"TRADE_ID"},"id":1}'

# Both nodes: Fund escrow
curl -s http://127.0.0.1:8080 -d '{"jsonrpc":"2.0","method":"swap_fund","params":{"trade_id":"TRADE_ID"},"id":1}'
curl -s http://127.0.0.1:8081 -d '{"jsonrpc":"2.0","method":"swap_fund","params":{"trade_id":"TRADE_ID"},"id":1}'

# Wait for confirmations, then exchange nonces → sign → redeem
```

## Wallet Security

- **24-word BIP39 mnemonic** — Industry standard seed phrases
- **Argon2id** — OWASP-recommended password hashing
- **AES-256-GCM** — Authenticated encryption for seed storage
- **Memory clearing** — Sensitive data wiped when wallet is locked
- **File permissions** — Wallet files stored with `0600` permissions
- **Multi-address** — Aggregates UTXOs from all derived addresses

## Configuration

On first run, a `config.yaml` is auto-generated at `~/.klingon/config.yaml`:

```yaml
network_type: mainnet
identity:
  key_file: node.key
network:
  listen_addrs:
    - /ip4/0.0.0.0/tcp/4001
    - /ip4/0.0.0.0/udp/4001/quic-v1
  enable_mdns: true
  enable_dht: true
  enable_relay: true
  enable_nat: true
  enable_hole_punching: true
  conn_mgr:
    low_water: 100
    high_water: 400
    grace_period: 1m
storage:
  data_dir: ~/.klingon
logging:
  level: info
```

CLI flags override config file settings. Testnet uses `~/.klingon/testnet/` with isolated DHT and discovery namespaces.

## Smart Contracts

EVM HTLC contracts live in `/contracts` (Foundry project):

```bash
cd contracts
forge install
forge build
forge test
```

See [contracts/README.md](contracts/README.md) for deployment instructions.

## Development

```bash
# Build
make build

# Run tests
make test

# Verbose tests
make test-v

# Coverage report
make test-cover

# Debug mode
make debug

# Two local nodes
make test-local

# Tidy dependencies
make tidy
```

### Integration Testing

Test scripts in `scripts/` automate full swap flows on testnet:

| Script | Description |
|--------|-------------|
| `test-swap.sh` | MuSig2 atomic swap (BTC ↔ LTC) |
| `test-htlc-swap.sh` | HTLC atomic swap |
| `test-refund.sh` | MuSig2 refund path |
| `test-htlc-refund.sh` | HTLC refund path |
| `test-evm-swap.sh` | EVM HTLC swap |
| `test-evm-refund.sh` | EVM refund path |
| `test-evm-btc-swap.sh` | Cross-chain ETH ↔ BTC |

Scripts read wallet credentials from environment variables. Copy `scripts/.env.example` to `scripts/.env` and fill in your testnet wallet details before running.

## Fee Structure

| Fee | Rate |
|-----|------|
| Maker | 0.2% |
| Taker | 0.2% |
| Distribution | 50% DAO / 50% Node operators |

## Documentation

- [Atomic Swap Explained](docs/swap-explained-simply.md) — How swaps work in plain language
- [Atomic Swaps Technical](docs/atomic-swaps.md) — Technical deep dive
- [Swap Paths](docs/swap-paths.md) — Supported chain pair matrix
- [HTLC Implementation](docs/htlc-implementation.md) — HTLC technical details
- [EVM HTLC Design](docs/evm-htlc-design.md) — Smart contract design
- [Monero Swaps](docs/monero-atomic-swaps.md) — Adaptor signature approach for XMR
- [P2P Messaging](docs/P2P_MESSAGING_PLAN.md) — Message delivery architecture

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure `go test ./...` passes
5. Submit a pull request

## License

[MIT](LICENSE)
