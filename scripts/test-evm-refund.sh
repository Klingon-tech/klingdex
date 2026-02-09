#!/bin/bash
# Klingon EVM HTLC Refund Test Script
# Tests the refund flow when a swap times out

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
LOG_LEVEL="${LOG_LEVEL:-info}"

# Load credentials from .env file
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/.env" ]; then
    source "$SCRIPT_DIR/.env"
fi

# Credentials (from .env or environment)
PASSWORD="${WALLET_PASSWORD:?ERROR: Set WALLET_PASSWORD in scripts/.env (see scripts/.env.example)}"
MNEMONIC1="${MNEMONIC1:?ERROR: Set MNEMONIC1 in scripts/.env (see scripts/.env.example)}"
MNEMONIC2="${MNEMONIC2:?ERROR: Set MNEMONIC2 in scripts/.env (see scripts/.env.example)}"

# Short timelock for testing refunds (5 minutes)
SHORT_TIMELOCK=300

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[-]${NC} $1"; exit 1; }
info() { echo -e "${BLUE}[i]${NC} $1"; }

cleanup() {
    log "Cleaning up existing processes..."
    pkill -9 -f klingond 2>/dev/null || true
    sleep 1
}

start_nodes() {
    log "Starting Node 1 with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE1_DIR --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/$NODE1_PORT --log-level $LOG_LEVEL > /tmp/node1.log 2>&1 &
    sleep 2

    log "Starting Node 2 with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE2_DIR --api 127.0.0.1:18081 --listen /ip4/0.0.0.0/tcp/$NODE2_PORT --log-level $LOG_LEVEL > /tmp/node2.log 2>&1 &
    sleep 2

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

    local status1=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_status","id":1}')
    local status2=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_status","id":1}')

    if echo "$status1" | grep -q '"has_wallet":false'; then
        log "Restoring wallet 1..."
        curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_create\",\"params\":{\"mnemonic\":\"$MNEMONIC1\",\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    fi

    if echo "$status2" | grep -q '"has_wallet":false'; then
        log "Restoring wallet 2..."
        curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_create\",\"params\":{\"mnemonic\":\"$MNEMONIC2\",\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    fi

    log "Unlocking wallets..."
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_unlock\",\"params\":{\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"wallet_unlock\",\"params\":{\"password\":\"$PASSWORD\"},\"id\":1}" > /dev/null

    log "Wallets ready"
}

connect_peers() {
    log "Connecting peers..."

    local node1_info=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"node_info","id":1}')
    local node1_addr=$(echo "$node1_info" | jq -r '.result.addrs[0]')

    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"peers_connect\",\"params\":{\"addr\":\"$node1_addr\"},\"id\":1}" > /dev/null
    sleep 1

    local peers=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"peers_count","id":1}' | jq -r '.result.connected')
    log "Connected peers: $peers"
}

create_order() {
    log "Creating order on Node 1 (ETH ↔ BSC with short timelock for refund test)..."

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_create\",
        \"params\":{
            \"offer_chain\":\"ETH\",
            \"offer_amount\":10000000000000000,
            \"request_chain\":\"BSC\",
            \"request_amount\":10000000000000000,
            \"preferred_methods\":[\"htlc\"]
        },
        \"id\":1
    }")

    ORDER_ID=$(echo "$result" | jq -r '.result.id // .result.order_id // empty')
    if [ -z "$ORDER_ID" ]; then
        error "Failed to create order: $result"
    fi

    log "Order created: $ORDER_ID"
    echo "$ORDER_ID" > /tmp/refund_order_id
    sleep 2
}

take_order() {
    log "Taking order on Node 2..."

    ORDER_ID=$(cat /tmp/refund_order_id)
    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_take\",
        \"params\":{
            \"order_id\":\"$ORDER_ID\",
            \"preferred_method\":\"htlc\"
        },
        \"id\":1
    }")

    TRADE_ID=$(echo "$result" | jq -r '.result.trade_id // empty')
    if [ -z "$TRADE_ID" ]; then
        error "Failed to take order: $result"
    fi

    log "Trade initiated: $TRADE_ID"
    echo "$TRADE_ID" > /tmp/refund_trade_id
    sleep 2
}

init_cross_chain_swap() {
    log "Initializing cross-chain EVM swap..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_initCrossChain\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"initiator\"
        },
        \"id\":1
    }" > /dev/null

    curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_initCrossChain\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"responder\"
        },
        \"id\":1
    }" > /dev/null

    log "Cross-chain swap initialized"
}

create_htlc_for_refund() {
    log "Creating ETH HTLC (will be refunded after timeout)..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmCreate\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"ETH\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "ETH HTLC created: $tx_hash"
        echo "$tx_hash" > /tmp/refund_htlc_tx
    else
        warn "ETH HTLC creation: $result"
    fi
}

check_htlc_status() {
    log "Checking HTLC status..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmStatus\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"ETH\"
        },
        \"id\":1
    }")

    echo ""
    info "=== ETH HTLC Status ==="
    echo "$result" | jq '.result'

    local state=$(echo "$result" | jq -r '.result.state')
    local timelock=$(echo "$result" | jq -r '.result.timelock')
    local now=$(date +%s)

    if [ "$state" = "active" ]; then
        local remaining=$((timelock - now))
        if [ $remaining -gt 0 ]; then
            info "Time until refund available: ${remaining}s"
        else
            log "Refund is NOW available!"
        fi
    elif [ "$state" = "refunded" ]; then
        log "HTLC has already been refunded"
    elif [ "$state" = "claimed" ]; then
        warn "HTLC has been claimed (cannot refund)"
    fi
}

wait_for_timeout() {
    log "Waiting for HTLC timeout..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    while true; do
        local result=$(curl -s $NODE1_API -d "{
            \"jsonrpc\":\"2.0\",
            \"method\":\"swap_evmStatus\",
            \"params\":{
                \"trade_id\":\"$TRADE_ID\",
                \"chain\":\"ETH\"
            },
            \"id\":1
        }")

        local state=$(echo "$result" | jq -r '.result.state')
        local timelock=$(echo "$result" | jq -r '.result.timelock')
        local now=$(date +%s)

        if [ "$state" != "active" ]; then
            warn "HTLC state changed to: $state"
            return 1
        fi

        local remaining=$((timelock - now))
        if [ $remaining -le 0 ]; then
            log "Timeout reached! Refund is now available."
            return 0
        fi

        info "Waiting for timeout... ${remaining}s remaining"
        sleep 30
    done
}

refund_htlc() {
    log "Refunding ETH HTLC..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmRefund\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"ETH\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.refund_tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "ETH refund tx: $tx_hash"
        echo "$tx_hash" > /tmp/refund_tx

        echo ""
        log "=== REFUND SUCCESSFUL ==="
        info "Check transaction: https://sepolia.etherscan.io/tx/$tx_hash"
    else
        local err=$(echo "$result" | jq -r '.error.message // empty')
        if [ -n "$err" ]; then
            warn "Refund failed: $err"
        else
            warn "Refund result: $result"
        fi
    fi
}

show_status() {
    TRADE_ID=$(cat /tmp/refund_trade_id 2>/dev/null || echo "")

    if [ -z "$TRADE_ID" ]; then
        warn "No active trade found"
        return
    fi

    check_htlc_status
}

# Main
case "${1:-help}" in
    clean)
        cleanup
        ;;
    setup)
        cleanup
        start_nodes
        setup_wallets
        connect_peers
        create_order
        take_order
        init_cross_chain_swap
        create_htlc_for_refund
        check_htlc_status
        echo ""
        log "Setup complete. HTLC created and waiting for timeout."
        info "Use '$0 wait' to wait for timeout, then '$0 refund' to claim refund."
        ;;
    status)
        show_status
        ;;
    wait)
        wait_for_timeout
        ;;
    refund)
        refund_htlc
        ;;
    full)
        cleanup
        start_nodes
        setup_wallets
        connect_peers
        create_order
        take_order
        init_cross_chain_swap
        create_htlc_for_refund
        check_htlc_status
        wait_for_timeout
        refund_htlc
        ;;
    *)
        echo "Klingon EVM HTLC Refund Test Script"
        echo ""
        echo "Usage: $0 {command}"
        echo ""
        echo "Commands:"
        echo "  clean   - Kill all klingond processes"
        echo "  setup   - Create HTLC for refund testing"
        echo "  status  - Check HTLC status and time remaining"
        echo "  wait    - Wait for HTLC timeout"
        echo "  refund  - Execute refund after timeout"
        echo "  full    - Run full refund test (setup + wait + refund)"
        echo ""
        echo "Note: Testnet HTLCs typically have 12-24 hour timelocks."
        echo "For faster testing, you may need to modify timelock settings."
        exit 1
        ;;
esac
