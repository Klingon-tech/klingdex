#!/bin/bash
# Klingon MuSig2 Atomic Swap Test Script
# Tests the full swap flow between two local nodes

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NODE1_API="http://127.0.0.1:18080"
NODE2_API="http://127.0.0.1:18081"
NODE1_DIR="/tmp/klingon-node1"
NODE2_DIR="/tmp/klingon-node2"
NODE1_PORT="14001"
NODE2_PORT="14002"
BIN="./bin/klingond"
LOG_LEVEL="${LOG_LEVEL:-info}"  # Set via env: LOG_LEVEL=debug ./scripts/test-swap.sh

# Load credentials from .env file
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/.env" ]; then
    source "$SCRIPT_DIR/.env"
fi

# Credentials (from .env or environment)
PASSWORD="${WALLET_PASSWORD:?ERROR: Set WALLET_PASSWORD in scripts/.env (see scripts/.env.example)}"
MNEMONIC1="${MNEMONIC1:?ERROR: Set MNEMONIC1 in scripts/.env (see scripts/.env.example)}"
MNEMONIC2="${MNEMONIC2:?ERROR: Set MNEMONIC2 in scripts/.env (see scripts/.env.example)}"

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[-]${NC} $1"; exit 1; }
info() { echo -e "${BLUE}[i]${NC} $1"; }

rpc() {
    local url=$1
    local method=$2
    local params=$3
    local result

    if [ -z "$params" ]; then
        result=$(curl -s "$url" -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"id\":1}")
    else
        result=$(curl -s "$url" -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}")
    fi

    # Check for JSON-RPC error (has "error" key with "code" inside)
    if echo "$result" | jq -e '.error.code' > /dev/null 2>&1; then
        echo "$result"
        return 1
    fi
    echo "$result"
}

# Step functions
cleanup() {
    log "Cleaning up existing processes..."
    pkill -9 -f klingond 2>/dev/null || true
    sleep 1
}

start_nodes() {
    log "Starting Node 1 (Maker - BTC) with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE1_DIR --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/$NODE1_PORT --log-level $LOG_LEVEL > /tmp/node1.log 2>&1 &
    NODE1_PID=$!
    sleep 2

    log "Starting Node 2 (Taker - LTC) with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE2_DIR --api 127.0.0.1:18081 --listen /ip4/0.0.0.0/tcp/$NODE2_PORT --log-level $LOG_LEVEL > /tmp/node2.log 2>&1 &
    NODE2_PID=$!
    sleep 2

    # Verify nodes are running
    if ! curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"node_info","id":1}' | grep -q peer_id; then
        error "Node 1 failed to start"
    fi
    if ! curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"node_info","id":1}' | grep -q peer_id; then
        error "Node 2 failed to start"
    fi
    log "Both nodes started successfully"
}

setup_wallets() {
    log "Setting up wallets..."

    # Check if wallets exist
    local status1=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_status","id":1}')
    local status2=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_status","id":1}')

    # Create/restore wallets if needed
    if echo "$status1" | grep -q '"has_wallet":false'; then
        log "Restoring wallet 1..."
        curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_create\",\"params\":{\"mnemonic\":\"$MNEMONIC1\",\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    fi

    if echo "$status2" | grep -q '"has_wallet":false'; then
        log "Restoring wallet 2..."
        curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_create\",\"params\":{\"mnemonic\":\"$MNEMONIC2\",\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    fi

    # Unlock wallets
    log "Unlocking wallets..."
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_unlock\",\"params\":{\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_unlock\",\"params\":{\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null

    log "Wallets ready"
}

show_balances() {
    log "Scanning wallet balances (including change addresses)..."

    # Scan BTC on Node 1
    local btc_scan=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_scanBalance","params":{"symbol":"BTC","gap_limit":20},"id":1}')
    local btc_total=$(echo "$btc_scan" | jq -r '.result.total_balance // 0')
    local btc_external=$(echo "$btc_scan" | jq -r '.result.external_balance // 0')
    local btc_change=$(echo "$btc_scan" | jq -r '.result.change_balance // 0')

    # Scan LTC on Node 2
    local ltc_scan=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_scanBalance","params":{"symbol":"LTC","gap_limit":20},"id":1}')
    local ltc_total=$(echo "$ltc_scan" | jq -r '.result.total_balance // 0')
    local ltc_external=$(echo "$ltc_scan" | jq -r '.result.external_balance // 0')
    local ltc_change=$(echo "$ltc_scan" | jq -r '.result.change_balance // 0')

    echo ""
    info "=== Node 1 (BTC) Wallet ==="
    info "Total: $btc_total sats (external: $btc_external, change: $btc_change)"
    echo "$btc_scan" | jq -r '.result.addresses[] | "  \(.path): \(.address) = \(.balance) sats"' 2>/dev/null || echo "  (no addresses with funds)"

    echo ""
    info "=== Node 2 (LTC) Wallet ==="
    info "Total: $ltc_total litoshis (external: $ltc_external, change: $ltc_change)"
    echo "$ltc_scan" | jq -r '.result.addresses[] | "  \(.path): \(.address) = \(.balance) litoshis"' 2>/dev/null || echo "  (no addresses with funds)"

    # Save scan results for later use
    echo "$btc_scan" > /tmp/node1_btc_scan.json
    echo "$ltc_scan" > /tmp/node2_ltc_scan.json

    # Extract first funded address info for each (prefer highest balance)
    echo "$btc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/node1_btc_funded.json
    echo "$ltc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/node2_ltc_funded.json
}

connect_peers() {
    log "Connecting peers..."

    # Get Node 1 peer info
    local node1_info=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"node_info","id":1}')
    local node1_addr=$(echo "$node1_info" | jq -r '.result.addrs[0]')

    # Connect Node 2 to Node 1
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"peers_connect\",\"params\":{\"addr\":\"$node1_addr\"},\"id\":1}" > /dev/null
    sleep 1

    local peers=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"peers_count","id":1}' | jq -r '.result.connected')
    log "Connected peers: $peers"
}

create_order() {
    log "Creating order on Node 1 (offering 10000 sats BTC for 100000 litoshis LTC)..."

    local result=$(curl -s $NODE1_API -d '{
        "jsonrpc":"2.0",
        "method":"orders_create",
        "params":{
            "offer_chain":"BTC",
            "offer_amount":10000,
            "request_chain":"LTC",
            "request_amount":100000,
            "preferred_methods":["musig2"]
        },
        "id":1
    }')

    ORDER_ID=$(echo "$result" | jq -r '.result.id // .result.order_id // empty')
    if [ -z "$ORDER_ID" ]; then
        error "Failed to create order: $result"
    fi

    log "Order created: $ORDER_ID"
    echo "$ORDER_ID" > /tmp/order_id
    sleep 2  # Wait for order to sync
}

take_order() {
    log "Taking order on Node 2..."

    ORDER_ID=$(cat /tmp/order_id)
    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_take\",
        \"params\":{
            \"order_id\":\"$ORDER_ID\",
            \"preferred_method\":\"musig2\"
        },
        \"id\":1
    }")

    TRADE_ID=$(echo "$result" | jq -r '.result.trade_id // empty')
    if [ -z "$TRADE_ID" ]; then
        error "Failed to take order: $result"
    fi

    log "Trade initiated: $TRADE_ID"
    echo "$TRADE_ID" > /tmp/trade_id
    sleep 2  # Wait for trade to sync
}

init_swaps() {
    log "Initializing swaps on both nodes..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Initialize on Node 1 (initiator/maker - provides BTC)
    local init1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_init\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"initiator\"
        },
        \"id\":1
    }")

    # Initialize on Node 2 (responder/taker - provides LTC)
    local init2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_init\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"responder\"
        },
        \"id\":1
    }")

    if echo "$init1" | grep -q '"error"'; then
        error "Swap init failed on Node 1: $init1"
    fi
    if echo "$init2" | grep -q '"error"'; then
        error "Swap init failed on Node 2: $init2"
    fi

    log "Swaps initialized on both nodes"
}

exchange_keys() {
    log "Exchanging public keys..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Get keys from both nodes
    local status1=$(curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
    local status2=$(curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")

    local key1=$(echo "$status1" | jq -r '.result.local_pubkey')
    local key2=$(echo "$status2" | jq -r '.result.local_pubkey')

    # Exchange keys
    curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setRemoteKey\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"remote_pubkey\":\"$key2\"
        },
        \"id\":1
    }" > /dev/null

    curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setRemoteKey\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"remote_pubkey\":\"$key1\"
        },
        \"id\":1
    }" > /dev/null

    log "Keys exchanged successfully"
}

get_escrow_addresses() {
    log "Getting escrow addresses..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Node 1 (initiator) gets BTC escrow address
    local btc_escrow=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }" | jq -r '.result.taproot_address')

    # Node 2 (responder) gets LTC escrow address
    local ltc_escrow=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }" | jq -r '.result.taproot_address')

    echo "$btc_escrow" > /tmp/btc_escrow
    echo "$ltc_escrow" > /tmp/ltc_escrow

    info "BTC Escrow (Taproot): $btc_escrow"
    info "LTC Escrow (Taproot): $ltc_escrow"
}

show_status() {
    log "Current swap status..."
    TRADE_ID=$(cat /tmp/trade_id)

    echo ""
    info "=== Node 1 (Maker/BTC) Status ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Node 2 (Taker/LTC) Status ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'
}

show_next_steps() {
    echo ""
    echo "========================================"
    echo " SWAP SETUP COMPLETE - NEXT STEPS"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/trade_id 2>/dev/null || echo "N/A")
    BTC_ESCROW=$(cat /tmp/btc_escrow 2>/dev/null || echo "N/A")
    LTC_ESCROW=$(cat /tmp/ltc_escrow 2>/dev/null || echo "N/A")

    echo "Trade ID: $TRADE_ID"
    echo ""
    echo "1. FUND BTC ESCROW (Node 1 sends 10000 sats):"
    echo "   curl -s $NODE1_API -d '{\"jsonrpc\":\"2.0\",\"method\":\"wallet_send\",\"params\":{\"symbol\":\"BTC\",\"to\":\"$BTC_ESCROW\",\"amount\":10000,\"fee_rate\":2},\"id\":1}'"
    echo ""
    echo "2. FUND LTC ESCROW (Node 2 sends 100000 litoshis):"
    echo "   curl -s $NODE2_API -d '{\"jsonrpc\":\"2.0\",\"method\":\"wallet_send\",\"params\":{\"symbol\":\"LTC\",\"to\":\"$LTC_ESCROW\",\"amount\":100000,\"fee_rate\":1},\"id\":1}'"
    echo ""
    echo "3. After funding, exchange nonces and complete the swap"
    echo ""
}

fund_escrows() {
    log "Auto-funding escrow addresses using swap_fund..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Use the new swap_fund method which automatically:
    # 1. Scans wallet UTXOs
    # 2. Builds and signs funding tx
    # 3. Broadcasts to network
    # 4. Sets funding info on swap

    info "Auto-funding BTC escrow on Node 1..."
    local btc_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_fund\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local btc_txid=$(echo "$btc_result" | jq -r '.result.txid // empty')
    local btc_error=$(echo "$btc_result" | jq -r '.error.message // empty')
    if [ -n "$btc_txid" ]; then
        log "BTC funding tx: $btc_txid"
        echo "$btc_txid" > /tmp/btc_funding_txid
        local btc_amount=$(echo "$btc_result" | jq -r '.result.amount // 0')
        local btc_fee=$(echo "$btc_result" | jq -r '.result.fee // 0')
        info "  Amount: $btc_amount sats, Fee: $btc_fee sats"
    elif [ -n "$btc_error" ]; then
        warn "BTC funding failed: $btc_error"
    else
        warn "BTC funding result: $btc_result"
    fi

    info "Auto-funding LTC escrow on Node 2..."
    local ltc_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_fund\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local ltc_txid=$(echo "$ltc_result" | jq -r '.result.txid // empty')
    local ltc_error=$(echo "$ltc_result" | jq -r '.error.message // empty')
    if [ -n "$ltc_txid" ]; then
        log "LTC funding tx: $ltc_txid"
        echo "$ltc_txid" > /tmp/ltc_funding_txid
        local ltc_amount=$(echo "$ltc_result" | jq -r '.result.amount // 0')
        local ltc_fee=$(echo "$ltc_result" | jq -r '.result.fee // 0')
        info "  Amount: $ltc_amount litoshis, Fee: $ltc_fee litoshis"
    elif [ -n "$ltc_error" ]; then
        warn "LTC funding failed: $ltc_error"
    else
        warn "LTC funding result: $ltc_result"
    fi
}

fund_escrows_manual() {
    log "Manually funding escrow addresses using wallet_send..."
    TRADE_ID=$(cat /tmp/trade_id)
    BTC_ESCROW=$(cat /tmp/btc_escrow)
    LTC_ESCROW=$(cat /tmp/ltc_escrow)

    # Read funded address info from scan results
    local btc_funded="/tmp/node1_btc_funded.json"
    local ltc_funded="/tmp/node2_ltc_funded.json"

    # Extract BTC source address info
    local btc_change=0
    local btc_index=0
    if [ -f "$btc_funded" ] && [ -s "$btc_funded" ]; then
        local btc_is_change=$(jq -r '.is_change // false' "$btc_funded")
        btc_index=$(jq -r '.index // 0' "$btc_funded" | sed 's/null/0/')
        if [ "$btc_is_change" = "true" ]; then
            btc_change=1
        fi
        local btc_addr=$(jq -r '.address // "unknown"' "$btc_funded")
        local btc_bal=$(jq -r '.balance // 0' "$btc_funded")
        info "Using BTC address: $btc_addr (change=$btc_change, index=$btc_index, balance=$btc_bal sats)"
    else
        warn "No BTC scan results found, using default address (change=0, index=0)"
    fi

    # Extract LTC source address info
    local ltc_change=0
    local ltc_index=0
    if [ -f "$ltc_funded" ] && [ -s "$ltc_funded" ]; then
        local ltc_is_change=$(jq -r '.is_change // false' "$ltc_funded")
        ltc_index=$(jq -r '.index // 0' "$ltc_funded" | sed 's/null/0/')
        if [ "$ltc_is_change" = "true" ]; then
            ltc_change=1
        fi
        local ltc_addr=$(jq -r '.address // "unknown"' "$ltc_funded")
        local ltc_bal=$(jq -r '.balance // 0' "$ltc_funded")
        info "Using LTC address: $ltc_addr (change=$ltc_change, index=$ltc_index, balance=$ltc_bal litoshis)"
    else
        warn "No LTC scan results found, using default address (change=0, index=0)"
    fi

    info "Funding BTC escrow with 10000 sats..."
    local btc_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"wallet_send\",
        \"params\":{
            \"symbol\":\"BTC\",
            \"to\":\"$BTC_ESCROW\",
            \"amount\":10000,
            \"change\":$btc_change,
            \"index\":$btc_index,
            \"fee_rate\":2
        },
        \"id\":1
    }")

    local btc_txid=$(echo "$btc_result" | jq -r '.result.txid // empty')
    if [ -n "$btc_txid" ]; then
        log "BTC funding tx: $btc_txid"
    else
        warn "BTC funding result: $btc_result"
    fi

    info "Funding LTC escrow with 100000 litoshis..."
    local ltc_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"wallet_send\",
        \"params\":{
            \"symbol\":\"LTC\",
            \"to\":\"$LTC_ESCROW\",
            \"amount\":100000,
            \"change\":$ltc_change,
            \"index\":$ltc_index,
            \"fee_rate\":1
        },
        \"id\":1
    }")

    local ltc_txid=$(echo "$ltc_result" | jq -r '.result.txid // empty')
    if [ -n "$ltc_txid" ]; then
        log "LTC funding tx: $ltc_txid"
        echo "$ltc_txid" > /tmp/ltc_funding_txid
    else
        warn "LTC funding result: $ltc_result"
    fi

    # Save BTC txid too
    if [ -n "$btc_txid" ]; then
        echo "$btc_txid" > /tmp/btc_funding_txid
    fi
}

exchange_nonces() {
    log "Exchanging nonces..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Generate and exchange nonces on both nodes
    local nonce1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_exchangeNonce\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local nonce1_val=$(echo "$nonce1" | jq -r '.result.local_nonce // empty')
    if [ -n "$nonce1_val" ]; then
        log "Node 1 nonce generated"
    else
        warn "Node 1 nonce exchange: $nonce1"
    fi

    local nonce2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_exchangeNonce\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local nonce2_val=$(echo "$nonce2" | jq -r '.result.local_nonce // empty')
    if [ -n "$nonce2_val" ]; then
        log "Node 2 nonce generated"
    else
        warn "Node 2 nonce exchange: $nonce2"
    fi

    # Exchange again to ensure both have each other's nonces
    sleep 1
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_exchangeNonce\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" > /dev/null

    log "Nonces exchanged successfully"
}

set_funding_info() {
    log "Setting funding info..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Get funding txids
    local btc_txid=$(cat /tmp/btc_funding_txid 2>/dev/null)
    local ltc_txid=$(cat /tmp/ltc_funding_txid 2>/dev/null)

    if [ -z "$btc_txid" ] || [ -z "$ltc_txid" ]; then
        error "Funding txids not found. Run 'fund' first."
    fi

    # Set BTC funding on Node 1
    local btc_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setFunding\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"txid\":\"$btc_txid\",
            \"vout\":0
        },
        \"id\":1
    }")

    local btc_state=$(echo "$btc_result" | jq -r '.result.state // empty')
    if [ -n "$btc_state" ]; then
        log "Node 1 funding set (state: $btc_state)"
    else
        warn "Node 1 set funding: $btc_result"
    fi

    # Set LTC funding on Node 2
    local ltc_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setFunding\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"txid\":\"$ltc_txid\",
            \"vout\":0
        },
        \"id\":1
    }")

    local ltc_state=$(echo "$ltc_result" | jq -r '.result.state // empty')
    if [ -n "$ltc_state" ]; then
        log "Node 2 funding set (state: $ltc_state)"
    else
        warn "Node 2 set funding: $ltc_result"
    fi

    sleep 1  # Wait for P2P messages to propagate
    log "Funding info exchanged via P2P"
}

check_confirmations() {
    log "Checking confirmations..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Check funding on both nodes
    local check1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_checkFunding\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local ready1=$(echo "$check1" | jq -r '.result.ready_to_sign // false')
    local local1=$(echo "$check1" | jq -r '.result.local_funded // false')
    local remote1=$(echo "$check1" | jq -r '.result.remote_funded // false')

    info "Node 1: local_funded=$local1, remote_funded=$remote1, ready_to_sign=$ready1"

    local check2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_checkFunding\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local ready2=$(echo "$check2" | jq -r '.result.ready_to_sign // false')
    local local2=$(echo "$check2" | jq -r '.result.local_funded // false')
    local remote2=$(echo "$check2" | jq -r '.result.remote_funded // false')

    info "Node 2: local_funded=$local2, remote_funded=$remote2, ready_to_sign=$ready2"

    if [ "$ready1" = "true" ] && [ "$ready2" = "true" ]; then
        log "Both nodes ready to sign!"
        return 0
    else
        warn "Waiting for confirmations..."
        return 1
    fi
}

wait_for_confirmations() {
    log "Waiting for funding confirmations..."

    local max_attempts=30  # 5 minutes with 10s intervals
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if check_confirmations; then
            return 0
        fi

        attempt=$((attempt + 1))
        info "Attempt $attempt/$max_attempts - waiting 10 seconds..."
        sleep 10
    done

    error "Timeout waiting for confirmations"
}

sign_swap() {
    log "Signing swap..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Sign on Node 1
    local sign1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_sign\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local sig1=$(echo "$sign1" | jq -r '.result.partial_sig // empty')
    if [ -n "$sig1" ]; then
        log "Node 1 signed: ${sig1:0:32}..."
    else
        local err1=$(echo "$sign1" | jq -r '.error.message // empty')
        if [ -n "$err1" ]; then
            warn "Node 1 sign failed: $err1"
            return 1
        fi
    fi

    # Sign on Node 2
    local sign2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_sign\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local sig2=$(echo "$sign2" | jq -r '.result.partial_sig // empty')
    if [ -n "$sig2" ]; then
        log "Node 2 signed: ${sig2:0:32}..."
    else
        local err2=$(echo "$sign2" | jq -r '.error.message // empty')
        if [ -n "$err2" ]; then
            warn "Node 2 sign failed: $err2"
            return 1
        fi
    fi

    sleep 1  # Wait for P2P signature exchange
    log "Partial signatures exchanged"
}

redeem_swap() {
    log "Redeeming swap..."
    TRADE_ID=$(cat /tmp/trade_id)

    # Redeem on Node 1 (gets LTC)
    local redeem1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_redeem\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local txid1=$(echo "$redeem1" | jq -r '.result.txid // empty')
    if [ -n "$txid1" ]; then
        log "Node 1 redeem tx: $txid1"
    else
        local err1=$(echo "$redeem1" | jq -r '.error.message // empty')
        warn "Node 1 redeem: ${err1:-$redeem1}"
    fi

    # Redeem on Node 2 (gets BTC)
    local redeem2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_redeem\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local txid2=$(echo "$redeem2" | jq -r '.result.txid // empty')
    if [ -n "$txid2" ]; then
        log "Node 2 redeem tx: $txid2"
    else
        local err2=$(echo "$redeem2" | jq -r '.error.message // empty')
        warn "Node 2 redeem: ${err2:-$redeem2}"
    fi
}

complete_swap() {
    log "=== COMPLETING SWAP ==="

    exchange_nonces

    # Check if ready to sign (may need to wait for confirmations)
    if ! check_confirmations; then
        warn "Not ready to sign yet. Run 'wait' to wait for confirmations, or 'sign' once confirmed."
        return 1
    fi

    sign_swap
    redeem_swap

    echo ""
    log "=== SWAP COMPLETE ==="
    show_status
}

complete_swap_manual() {
    log "=== COMPLETING SWAP (MANUAL FLOW) ==="

    exchange_nonces
    set_funding_info

    # Check if ready to sign (may need to wait for confirmations)
    if ! check_confirmations; then
        warn "Not ready to sign yet. Run 'wait' to wait for confirmations, or 'sign' once confirmed."
        return 1
    fi

    sign_swap
    redeem_swap

    echo ""
    log "=== SWAP COMPLETE ==="
    show_status
}

# Main
case "${1:-setup}" in
    clean)
        cleanup
        ;;
    start)
        start_nodes
        ;;
    setup)
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_swaps
        exchange_keys
        get_escrow_addresses
        show_status
        show_next_steps
        ;;
    fund)
        fund_escrows
        ;;
    nonces)
        exchange_nonces
        ;;
    setfunding)
        set_funding_info
        ;;
    check)
        check_confirmations
        ;;
    wait)
        wait_for_confirmations
        ;;
    sign)
        sign_swap
        ;;
    redeem)
        redeem_swap
        ;;
    complete)
        complete_swap
        ;;
    status)
        show_status
        ;;
    balances)
        show_balances
        ;;
    all)
        # Auto-fund flow: swap_fund handles signing, broadcast, and sets funding info
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_swaps
        exchange_keys
        get_escrow_addresses
        fund_escrows
        exchange_nonces
        check_confirmations || true
        show_status
        ;;
    full)
        # Full swap including waiting for confirmations (auto-fund)
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_swaps
        exchange_keys
        get_escrow_addresses
        fund_escrows
        exchange_nonces
        wait_for_confirmations
        sign_swap
        redeem_swap
        show_status
        ;;
    manual-fund)
        # Manual funding using wallet_send (old method)
        fund_escrows_manual
        ;;
    manual-complete)
        # Manual flow that requires setfunding
        complete_swap_manual
        ;;
    *)
        echo "Usage: $0 {clean|start|setup|fund|nonces|setfunding|check|wait|sign|redeem|complete|status|balances|all|full}"
        echo ""
        echo "  clean         - Kill all klingond processes"
        echo "  start         - Start both test nodes"
        echo "  setup         - Full setup: start nodes, wallets, order, swap init (default)"
        echo "  fund          - Auto-fund escrows using swap_fund (scan UTXOs, sign, broadcast)"
        echo "  manual-fund   - Manual funding using wallet_send (old method)"
        echo "  nonces        - Exchange nonces between nodes"
        echo "  setfunding    - Set funding transaction info (only needed for manual-fund)"
        echo "  check         - Check confirmation status"
        echo "  wait          - Wait for funding confirmations"
        echo "  sign          - Create partial signatures"
        echo "  redeem        - Broadcast redeem transactions"
        echo "  complete      - Complete swap (nonces + check + sign + redeem)"
        echo "  manual-complete - Complete swap with manual funding flow"
        echo "  status        - Show current swap status"
        echo "  balances      - Show wallet balances"
        echo "  all           - Everything up to confirmation check (auto-fund)"
        echo "  full          - Full swap including waiting for confirmations (auto-fund)"
        echo ""
        echo "Environment variables:"
        echo "  LOG_LEVEL     - Log level for nodes (debug, info, warn, error). Default: info"
        echo ""
        echo "Example: LOG_LEVEL=debug $0 setup"
        exit 1
        ;;
esac
