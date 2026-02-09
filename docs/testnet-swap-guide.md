# Testnet MuSig2 Atomic Swap Guide

This guide walks through executing a MuSig2 atomic swap between BTC and LTC on testnet using two Klingon nodes.

## Current Implementation Status

| Feature | Status | Notes |
|---------|--------|-------|
| Node setup | Working | |
| Wallet creation | Working | |
| Address generation | Working | |
| Balance checking | Working | via mempool.space API |
| Order creation | Working | |
| Order broadcasting (P2P) | Working | |
| Order taking | Working | |
| Trade creation (both sides) | Working | |
| Order/Trade persistence | Working | survives restart |
| Order/Trade sync on reconnect | Working | |
| **MuSig2 key exchange (swap_init)** | **Working** | Fixed deadlock bug |
| Pubkey storage in trades | Working | maker_pubkey, taker_pubkey columns |
| **Taproot address generation** | **Working** | Both nodes compute same address |
| Funding transactions | **Working** | swap_setFunding RPC |
| Nonce exchange | **Working** | swap_exchangeNonce RPC |
| Signature exchange | **Working** | swap_sign RPC |
| Redemption | **Working** | swap_redeem RPC |

## Recent Bug Fixes (2025-12-14)

### 1. Deadlock in emitEvent (coordinator.go)
**Problem:** `swap_init` RPC was hanging/timing out. The `emitEvent()` function tried to acquire `RLock` while the caller already held the write lock, causing a deadlock.

**Fix:** Removed the lock acquisition in `emitEvent()` since all callers already hold the mutex. See `internal/swap/coordinator.go:134-151`.

### 2. Missing Method field in swap offer
**Problem:** "BTC does not support " error (empty method string).

**Fix:** Added `Method: swap.Method(trade.Method)` to the Offer struct in `internal/rpc/swap.go:76`.

### 3. Backend not available for chain
**Problem:** Coordinator wasn't receiving blockchain backends.

**Fix:** Added `All()` method to backend Registry and passed `backendRegistry.All()` to coordinator config in `cmd/klingond/main.go:147`.

### 4. Pubkey storage for P2P exchange
**Problem:** Taker couldn't initialize swap because maker's pubkey wasn't stored.

**Fix:** Added `maker_pubkey` and `taker_pubkey` columns to trades table, and updated storage/RPC to save pubkeys when exchanged.

### 5. Taker taproot address not computed (coordinator.go)
**Problem:** `swap_getAddress` returned "taproot address not yet generated" error for taker, even after calling `swap_init`.

**Fix:** In `RespondToSwap()`, compute the taproot address after setting the remote pubkey. Added `taprootAddr, err := session.TaprootAddress()` and stored it in `MuSig2SwapData`. See `internal/swap/coordinator.go:298-302,321`.

## Prerequisites

- Built klingond binary (`make build`)
- Access to BTC testnet4 faucet (https://mempool.space/testnet4/faucet)
- Access to LTC testnet faucet (https://testnet-faucet.com/ltc-testnet/)

## Test Wallets

You need two funded testnet wallets. Set up your credentials in `scripts/.env` (see `scripts/.env.example`).

Each node needs:
- A 24-word BIP39 mnemonic
- A password
- Funded testnet addresses (BTC testnet4 for Node 1, LTC testnet for Node 2)

Fund your wallets via testnet faucets before running the swap.

## Overview

We'll set up two nodes:
- **Node 1 (Maker/Alice)**: Creates an order to swap BTC for LTC
- **Node 2 (Taker/Bob)**: Takes the order, swaps LTC for BTC

## Step 1: Start Node 1 (Maker)

```bash
# Terminal 1: Start Node 1
./bin/klingond --testnet \
  --data-dir /tmp/klingon-node1 \
  --api 127.0.0.1:18080 \
  --listen /ip4/0.0.0.0/tcp/14001
```

Node 1 will listen on:
- P2P: port 14001
- RPC API: http://127.0.0.1:18080

## Step 2: Start Node 2 (Taker)

```bash
# Terminal 2: Start Node 2
./bin/klingond --testnet \
  --data-dir /tmp/klingon-node2 \
  --api 127.0.0.1:18081 \
  --listen /ip4/0.0.0.0/tcp/14002
```

Node 2 will listen on:
- P2P: port 14002
- RPC API: http://127.0.0.1:18081

**Note:** Nodes on the same network will auto-discover each other via mDNS.

## Step 3: Create/Restore Wallets

Use JSON files to avoid shell escaping issues with mnemonics.

### Node 1 Wallet (Maker)

```bash
# Create wallet (use your own mnemonic and password)
curl -s -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0","method":"wallet_create",
  "params":{"mnemonic":"your 24 word mnemonic here...","password":"your_password"},
  "id":1
}' | jq .

# Unlock wallet
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"wallet_unlock","params":{"password":"your_password"},"id":1}' | jq .
```

### Node 2 Wallet (Taker)

```bash
# Create wallet (use your own mnemonic and password)
curl -s -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0","method":"wallet_create",
  "params":{"mnemonic":"your 24 word mnemonic here...","password":"your_password"},
  "id":1
}' | jq .

# Unlock wallet
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"wallet_unlock","params":{"password":"your_password"},"id":1}' | jq .
```

## Step 4: Verify Peer Connection

Nodes should auto-discover via mDNS. Verify connection:

```bash
# Check Node 1 peers
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"peers_count","id":1}' | jq .

# Should show: {"connected": 1, "known": 1}
```

## Step 5: Check Balances

```bash
# Node 1 BTC balance
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"wallet_getBalance","params":{"symbol":"BTC","address":"YOUR_BTC_ADDRESS"},"id":1}' | jq .

# Node 2 LTC balance
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"wallet_getBalance","params":{"symbol":"LTC","address":"YOUR_LTC_ADDRESS"},"id":1}' | jq .
```

## Step 6: Create an Order (Node 1 - Maker)

Node 1 creates an order offering BTC for LTC:

```bash
curl -s -X POST http://127.0.0.1:18080 \
  -d '{
    "jsonrpc":"2.0",
    "method":"orders_create",
    "params":{
      "offer_chain": "BTC",
      "offer_amount": 100000,
      "request_chain": "LTC",
      "request_amount": 5000000,
      "preferred_methods": ["musig2"]
    },
    "id":1
  }' | jq .
```

Save the order ID from the response.

## Step 7: Take the Order (Node 2 - Taker)

Wait 1-2 seconds for P2P propagation, then:

```bash
# List orders to see available orders
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"orders_list","id":1}' | jq .

# Take the order
curl -s -X POST http://127.0.0.1:18081 \
  -d '{
    "jsonrpc":"2.0",
    "method":"orders_take",
    "params":{"order_id": "ORDER_ID_HERE"},
    "id":1
  }' | jq .
```

This creates a trade on both nodes.

## Step 8: List Trades

```bash
# Node 1 trades
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"trades_list","id":1}' | jq .

# Node 2 trades
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"trades_list","id":1}' | jq .
```

## Step 9: Initialize Swap (MuSig2 Key Exchange)

**IMPORTANT: Maker must call swap_init FIRST to broadcast their pubkey.**

### 9a. Maker (Node 1) initializes swap first:

```bash
curl -s -X POST http://127.0.0.1:18080 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_init",
    "params":{"trade_id": "TRADE_ID_HERE"},
    "id":1
  }' | jq .
```

Response:
```json
{
  "trade_id": "...",
  "local_pubkey": "03...",  // Maker's ephemeral pubkey (hex)
  "taproot_address": "",     // Empty until taker responds
  "state": "init"
}
```

The maker's pubkey is broadcast via P2P to the taker.

### 9b. Taker (Node 2) initializes swap:

Wait ~1 second for maker's pubkey to arrive via P2P, then:

```bash
curl -s -X POST http://127.0.0.1:18081 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_init",
    "params":{"trade_id": "TRADE_ID_HERE"},
    "id":1
  }' | jq .
```

If successful, both nodes now have:
- Local ephemeral keypair
- Remote pubkey
- Taproot 2-of-2 address for the swap

### 9c. Verify pubkeys stored in database:

```bash
sqlite3 /tmp/klingon-node1/testnet/klingon.db \
  "SELECT id, maker_pubkey, taker_pubkey FROM trades;"
```

## Step 10: Get Taproot Address

After both parties exchange pubkeys, get the funding address:

```bash
curl -s -X POST http://127.0.0.1:18080 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_getAddress",
    "params":{"trade_id": "TRADE_ID_HERE"},
    "id":1
  }' | jq .
```

Response:
```json
{
  "trade_id": "...",
  "taproot_address": "tb1p...",  // 2-of-2 MuSig2 Taproot address
  "chain": "BTC",
  "amount": 100000
}
```

## Step 11: Fund the Taproot Address

Each party needs to send their funds to the taproot address. On testnet, use faucets:

### 11a. Maker (Node 1) funds with BTC

Send 100000 satoshis (0.001 BTC) to the taproot address from a BTC testnet4 faucet.
After the transaction confirms, record the txid and vout:

```bash
curl -s -X POST http://127.0.0.1:18080 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_setFunding",
    "params":{
      "trade_id": "TRADE_ID_HERE",
      "txid": "YOUR_FUNDING_TXID",
      "vout": 0
    },
    "id":1
  }' | jq .
```

### 11b. Taker (Node 2) funds with LTC

Send 5000000 litoshis (0.05 LTC) to the taproot address from an LTC testnet faucet.
After the transaction confirms:

```bash
curl -s -X POST http://127.0.0.1:18081 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_setFunding",
    "params":{
      "trade_id": "TRADE_ID_HERE",
      "txid": "YOUR_FUNDING_TXID",
      "vout": 0
    },
    "id":1
  }' | jq .
```

### 11c. Check funding status

```bash
# Node 1
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"swap_checkFunding","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .

# Node 2
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"swap_checkFunding","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .
```

## Step 12: Exchange Nonces

Once both sides are funded, exchange MuSig2 nonces:

```bash
# Node 1 (Maker) generates and broadcasts nonce
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"swap_exchangeNonce","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .

# Wait 1-2 seconds for P2P propagation

# Node 2 (Taker) generates and broadcasts nonce
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"swap_exchangeNonce","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .
```

## Step 13: Create and Exchange Partial Signatures

Both parties create their partial signatures:

```bash
# Node 1 creates partial signature
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"swap_sign","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .

# Save the partial_sig from the response

# Node 2 creates partial signature
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"swap_sign","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .

# Save the partial_sig from the response
```

## Step 14: Redeem

Each party combines signatures and broadcasts the redemption transaction:

```bash
# Node 1 redeems (gets LTC)
curl -s -X POST http://127.0.0.1:18080 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_redeem",
    "params":{
      "trade_id": "TRADE_ID",
      "remote_partial_sig": "NODE2_PARTIAL_SIG_HEX"
    },
    "id":1
  }' | jq .

# Node 2 redeems (gets BTC)
curl -s -X POST http://127.0.0.1:18081 \
  -d '{
    "jsonrpc":"2.0",
    "method":"swap_redeem",
    "params":{
      "trade_id": "TRADE_ID",
      "remote_partial_sig": "NODE1_PARTIAL_SIG_HEX"
    },
    "id":1
  }' | jq .
```

## Step 15: Check Swap Status

```bash
curl -s -X POST http://127.0.0.1:18080 \
  -d '{"jsonrpc":"2.0","method":"swap_status","params":{"trade_id":"TRADE_ID"},"id":1}' | jq .
```

## Trade State Flow

```
init -> accepted -> funding -> funded -> signing -> redeemed
                                      \-> refunded (timeout)
```

## Troubleshooting

### swap_init hangs or times out
This was caused by a deadlock bug (fixed). Rebuild with latest code.

### "maker pubkey not yet received" error
The taker called `swap_init` before the maker. The maker must initialize first.

### "backend not available for chain: BTC"
Rebuild with latest code - backends are now properly passed to coordinator.

### Wallet creation fails with "Parse error"
Use JSON files with `curl -d @file.json` instead of inline JSON to avoid shell escaping issues.

### Nodes can't discover each other
mDNS should auto-discover. If not, manually connect:
```bash
curl -s -X POST http://127.0.0.1:18081 \
  -d '{"jsonrpc":"2.0","method":"peers_connect","params":{"addr":"/ip4/127.0.0.1/tcp/14001/p2p/NODE1_PEER_ID"},"id":1}'
```

## Reference: RPC Methods

| Method | Description |
|--------|-------------|
| `node_info` | Get node peer ID and addresses |
| `peers_connect` | Connect to a peer |
| `peers_count` | Get connected/known peer counts |
| `wallet_create` | Create wallet from mnemonic |
| `wallet_unlock` | Unlock wallet with password |
| `wallet_getAddress` | Get address for a chain |
| `wallet_getBalance` | Get balance for address |
| `orders_create` | Create a new order |
| `orders_list` | List all orders |
| `orders_take` | Take an order |
| `trades_list` | List all trades |
| `trades_status` | Get detailed trade status |
| `swap_init` | Initialize swap, exchange pubkeys |
| `swap_getAddress` | Get Taproot funding address |
| `swap_setFunding` | Record external funding tx |
| `swap_checkFunding` | Check funding status |
| `swap_exchangeNonce` | Generate and exchange MuSig2 nonces |
| `swap_sign` | Create and broadcast partial signature |
| `swap_redeem` | Combine sigs and broadcast redemption |
| `swap_status` | Get detailed swap status |

## Key Files Modified

- `internal/swap/coordinator.go` - Fixed deadlock in emitEvent(), added taproot address computation in RespondToSwap() for taker
- `internal/swap/coordinator_test.go` - Updated tests for ephemeral key behavior (wallet not required for MuSig2)
- `internal/rpc/swap.go` - swap_init, pubkey exchange handlers, pubkey storage
- `internal/storage/trades.go` - Added maker_pubkey, taker_pubkey columns and UpdateTradePubKey()
- `internal/storage/storage.go` - Updated trades table schema with pubkey columns
- `internal/backend/backend.go` - Added All() method to Registry
- `cmd/klingond/main.go` - Pass backends to coordinator

## Testnet Resources

- **BTC Testnet4 Explorer**: https://mempool.space/testnet4/
- **LTC Testnet Explorer**: https://litecoinspace.org/testnet/
- **BTC Testnet4 Faucet**: https://mempool.space/testnet4/faucet
- **LTC Testnet Faucet**: https://testnet-faucet.com/ltc-testnet/
