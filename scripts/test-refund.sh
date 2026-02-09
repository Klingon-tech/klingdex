#!/bin/bash
# Klingon Refund Test Script
# Tests the CSV timelock refund path when counterparty disappears
#
# Flow:
# 1. Setup swap between Node 1 (Maker/BTC) and Node 2 (Taker/LTC)
# 2. Both sides fund the escrow
# 3. Node 2 "disappears" (simulated by not completing the swap)
# 4. Wait for CSV timelock to expire
# 5. Node 1 refunds their BTC via script path
#
# Timeouts (testnet):
# - BTC: 72 blocks for maker (~12 hours), 36 blocks for taker (~6 hours)
# - LTC: 288 blocks for maker (~12 hours), 144 blocks for taker (~6 hours)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
NODE1_API="http://127.0.0.1:18080"
NODE2_API="http://127.0.0.1:18081"
NODE1_DIR="/tmp/klingon-node1"
NODE2_DIR="/tmp/klingon-node2"
NODE1_PORT="14001"
NODE2_PORT="14002"
BIN="./bin/klingond"

# Load credentials from .env file
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/.env" ]; then
    source "$SCRIPT_DIR/.env"
fi

# Credentials (from .env or environment)
PASSWORD="${WALLET_PASSWORD:?ERROR: Set WALLET_PASSWORD in scripts/.env (see scripts/.env.example)}"
MNEMONIC1="${MNEMONIC1:?ERROR: Set MNEMONIC1 in scripts/.env (see scripts/.env.example)}"
MNEMONIC2="${MNEMONIC2:?ERROR: Set MNEMONIC2 in scripts/.env (see scripts/.env.example)}"

# Swap amounts (must be above minimum: 10000 sats / 100000 litoshis)
BTC_AMOUNT=15000     # 15000 sats
LTC_AMOUNT=150000    # 150000 litoshis

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[-]${NC} $1"; exit 1; }
info() { echo -e "${BLUE}[i]${NC} $1"; }
highlight() { echo -e "${CYAN}[*]${NC} $1"; }

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

    if echo "$result" | jq -e '.error.code' > /dev/null 2>&1; then
        echo "$result"
        return 1
    fi
    echo "$result"
}

cleanup() {
    log "Cleaning up existing processes..."
    pkill -9 -f klingond 2>/dev/null || true
    sleep 1
}

start_nodes() {
    log "Starting Node 1 (Maker - BTC)..."
    $BIN --testnet --data-dir $NODE1_DIR --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/$NODE1_PORT > /tmp/node1.log 2>&1 &
    NODE1_PID=$!
    sleep 2

    log "Starting Node 2 (Taker - LTC)..."
    $BIN --testnet --data-dir $NODE2_DIR --api 127.0.0.1:18081 --listen /ip4/0.0.0.0/tcp/$NODE2_PORT > /tmp/node2.log 2>&1 &
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

show_balances() {
    log "Scanning wallet balances..."

    local btc_scan=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_scanBalance","params":{"symbol":"BTC","gap_limit":20},"id":1}')
    local btc_total=$(echo "$btc_scan" | jq -r '.result.total_balance // 0')

    local ltc_scan=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_scanBalance","params":{"symbol":"LTC","gap_limit":20},"id":1}')
    local ltc_total=$(echo "$ltc_scan" | jq -r '.result.total_balance // 0')

    echo ""
    info "Node 1 (BTC): $btc_total sats"
    info "Node 2 (LTC): $ltc_total litoshis"

    echo "$btc_scan" > /tmp/node1_btc_scan.json
    echo "$ltc_scan" > /tmp/node2_ltc_scan.json

    echo "$btc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/node1_btc_funded.json
    echo "$ltc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/node2_ltc_funded.json
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
    log "Creating order on Node 1 (offering $BTC_AMOUNT sats BTC for $LTC_AMOUNT litoshis LTC)..."

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_create\",
        \"params\":{
            \"offer_chain\":\"BTC\",
            \"offer_amount\":$BTC_AMOUNT,
            \"request_chain\":\"LTC\",
            \"request_amount\":$LTC_AMOUNT,
            \"preferred_methods\":[\"musig2\"]
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
            \"preferred_method\":\"musig2\"
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

init_swaps() {
    log "Initializing swaps on both nodes..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local init1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_init\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"initiator\"
        },
        \"id\":1
    }")

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
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local status1=$(curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
    local status2=$(curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")

    local key1=$(echo "$status1" | jq -r '.result.local_pubkey')
    local key2=$(echo "$status2" | jq -r '.result.local_pubkey')

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
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local btc_escrow=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }" | jq -r '.result.taproot_address')

    local ltc_escrow=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }" | jq -r '.result.taproot_address')

    echo "$btc_escrow" > /tmp/refund_btc_escrow
    echo "$ltc_escrow" > /tmp/refund_ltc_escrow

    info "BTC Escrow (Taproot): $btc_escrow"
    info "LTC Escrow (Taproot): $ltc_escrow"
}

fund_escrows() {
    log "Auto-funding escrow addresses using swap_fund..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

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
        echo "$btc_txid" > /tmp/refund_btc_funding_txid
        local btc_amount=$(echo "$btc_result" | jq -r '.result.amount // 0')
        local btc_fee=$(echo "$btc_result" | jq -r '.result.fee // 0')
        info "  Amount: $btc_amount sats, Fee: $btc_fee sats"
    elif [ -n "$btc_error" ]; then
        warn "BTC funding failed: $btc_error"
        return 1
    else
        warn "BTC funding failed: $btc_result"
        return 1
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
        echo "$ltc_txid" > /tmp/refund_ltc_funding_txid
        local ltc_amount=$(echo "$ltc_result" | jq -r '.result.amount // 0')
        local ltc_fee=$(echo "$ltc_result" | jq -r '.result.fee // 0')
        info "  Amount: $ltc_amount litoshis, Fee: $ltc_fee litoshis"
    elif [ -n "$ltc_error" ]; then
        warn "LTC funding failed: $ltc_error"
        return 1
    else
        warn "LTC funding failed: $ltc_result"
        return 1
    fi
}

set_funding_info() {
    log "Setting funding info..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local btc_txid=$(cat /tmp/refund_btc_funding_txid 2>/dev/null)
    local ltc_txid=$(cat /tmp/refund_ltc_funding_txid 2>/dev/null)

    if [ -z "$btc_txid" ] || [ -z "$ltc_txid" ]; then
        error "Funding txids not found. Run 'fund' first."
    fi

    curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setFunding\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"txid\":\"$btc_txid\",
            \"vout\":0
        },
        \"id\":1
    }" > /dev/null

    curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_setFunding\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"txid\":\"$ltc_txid\",
            \"vout\":0
        },
        \"id\":1
    }" > /dev/null

    sleep 1
    log "Funding info set on both nodes"
}

show_timeout_info() {
    echo ""
    echo "========================================"
    echo " TIMEOUT INFORMATION"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/refund_trade_id)

    info "=== Node 1 (Maker/BTC) Timeout Info ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_timeout\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Node 2 (Taker/LTC) Timeout Info ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_timeout\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'
}

check_timeouts() {
    log "Checking timeout status..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    echo ""
    info "=== Node 1 Timeout Check ==="
    local check1=$(curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_checkTimeouts\",\"id\":1}")
    echo "$check1" | jq '.result'

    echo ""
    info "=== Node 2 Timeout Check ==="
    local check2=$(curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_checkTimeouts\",\"id\":1}")
    echo "$check2" | jq '.result'
}

attempt_refund() {
    log "Attempting refund on Node 1 (BTC)..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_refund\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    echo ""
    if echo "$result" | jq -e '.result.success' | grep -q true; then
        highlight "REFUND SUCCESSFUL!"
        local txid=$(echo "$result" | jq -r '.result.txid')
        log "Refund txid: $txid"
        echo ""
        log "View on explorer: https://mempool.space/testnet/tx/$txid"
    else
        local err=$(echo "$result" | jq -r '.result.error // .error.message // "unknown error"')
        warn "Refund not possible yet: $err"
        echo ""
        info "The CSV timelock hasn't expired yet. Check timeout info for when refund becomes available."
    fi
}

attempt_refund_ltc() {
    log "Attempting refund on Node 2 (LTC)..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_refund\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    echo ""
    if echo "$result" | jq -e '.result.success' | grep -q true; then
        highlight "REFUND SUCCESSFUL!"
        local txid=$(echo "$result" | jq -r '.result.txid')
        log "Refund txid: $txid"
        echo ""
        log "View on explorer: https://litecoinspace.org/testnet/tx/$txid"
    else
        local err=$(echo "$result" | jq -r '.result.error // .error.message // "unknown error"')
        warn "Refund not possible yet: $err"
        echo ""
        info "The CSV timelock hasn't expired yet. Check timeout info for when refund becomes available."
    fi
}

show_status() {
    log "Current swap status..."
    TRADE_ID=$(cat /tmp/refund_trade_id)

    echo ""
    info "=== Node 1 (Maker/BTC) Status ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Node 2 (Taker/LTC) Status ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'
}

show_refund_instructions() {
    echo ""
    echo "========================================"
    echo " REFUND TEST - SWAP ABANDONED"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/refund_trade_id 2>/dev/null || echo "N/A")
    BTC_ESCROW=$(cat /tmp/refund_btc_escrow 2>/dev/null || echo "N/A")
    LTC_ESCROW=$(cat /tmp/refund_ltc_escrow 2>/dev/null || echo "N/A")
    BTC_TXID=$(cat /tmp/refund_btc_funding_txid 2>/dev/null || echo "N/A")
    LTC_TXID=$(cat /tmp/refund_ltc_funding_txid 2>/dev/null || echo "N/A")

    echo "Trade ID: $TRADE_ID"
    echo ""
    echo "Escrow Addresses:"
    echo "  BTC: $BTC_ESCROW"
    echo "  LTC: $LTC_ESCROW"
    echo ""
    echo "Funding Transactions:"
    echo "  BTC: $BTC_TXID"
    echo "  LTC: $LTC_TXID"
    echo ""
    highlight "THE SWAP HAS BEEN ABANDONED - COUNTERPARTY WILL NOT COMPLETE"
    echo ""
    echo "Timeouts (from funding confirmation):"
    echo "  - BTC (Maker/Node1): 72 blocks (~12 hours)"
    echo "  - LTC (Taker/Node2): 144 blocks (~6 hours)"
    echo ""
    echo "Commands to monitor and refund:"
    echo ""
    echo "1. Check timeout status:"
    echo "   ./scripts/test-refund.sh timeout"
    echo ""
    echo "2. Attempt refund (will fail until timeout):"
    echo "   ./scripts/test-refund.sh refund-btc    # Node 1 refunds BTC"
    echo "   ./scripts/test-refund.sh refund-ltc    # Node 2 refunds LTC"
    echo ""
    echo "3. Check swap status:"
    echo "   ./scripts/test-refund.sh status"
    echo ""
    echo "4. Check balances after refund:"
    echo "   ./scripts/test-refund.sh balances"
    echo ""
    warn "NOTE: Keep the nodes running! Use 'screen' or 'tmux' if needed."
    echo ""
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
        echo ""
        echo "========================================"
        echo " REFUND TEST SETUP"
        echo "========================================"
        echo ""
        echo "This script sets up a swap and then ABANDONS it to test the refund path."
        echo "One side will 'disappear' and the other will need to wait for the"
        echo "CSV timelock to expire before they can get their funds back."
        echo ""

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
        set_funding_info
        show_timeout_info
        show_refund_instructions
        ;;
    fund)
        fund_escrows
        set_funding_info
        ;;
    timeout)
        show_timeout_info
        ;;
    check)
        check_timeouts
        ;;
    refund-btc)
        attempt_refund
        ;;
    refund-ltc)
        attempt_refund_ltc
        ;;
    refund)
        attempt_refund
        attempt_refund_ltc
        ;;
    status)
        show_status
        ;;
    balances)
        show_balances
        ;;
    *)
        echo "Usage: $0 {clean|start|setup|fund|timeout|check|refund-btc|refund-ltc|refund|status|balances}"
        echo ""
        echo "  clean       - Kill all klingond processes"
        echo "  start       - Start both test nodes"
        echo "  setup       - Full setup: start nodes, create swap, fund, then ABANDON (default)"
        echo "  fund        - Fund the escrow addresses"
        echo "  timeout     - Show timeout information for the swap"
        echo "  check       - Check if timeout has been reached"
        echo "  refund-btc  - Attempt to refund BTC (Node 1/Maker)"
        echo "  refund-ltc  - Attempt to refund LTC (Node 2/Taker)"
        echo "  refund      - Attempt to refund both sides"
        echo "  status      - Show current swap status"
        echo "  balances    - Show wallet balances"
        echo ""
        echo "Typical flow:"
        echo "  1. ./scripts/test-refund.sh setup    # Setup and fund, then abandon"
        echo "  2. ./scripts/test-refund.sh timeout  # Check how long until refund available"
        echo "  3. [wait for timeout to expire]"
        echo "  4. ./scripts/test-refund.sh refund   # Get funds back"
        echo "  5. ./scripts/test-refund.sh balances # Verify refund succeeded"
        exit 1
        ;;
esac
