#!/bin/bash
# Klingon EVM ↔ Bitcoin Cross-Chain Atomic Swap Test Script
# Tests swaps between EVM chains and Bitcoin-family chains
# Uses Sepolia (ETH testnet) and BTC Testnet

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

# Cross-Chain Test Configuration
# Default: Node 1 offers BTC, wants ETH
# Set SWAP_DIRECTION=eth-btc to reverse (Node 1 offers ETH, wants BTC)
SWAP_DIRECTION="${SWAP_DIRECTION:-btc-eth}"

if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
    # Node 1 offers ETH, wants BTC
    OFFER_CHAIN="ETH"
    REQUEST_CHAIN="BTC"
    OFFER_AMOUNT="10000000000000000"  # 0.01 ETH in wei
    REQUEST_AMOUNT="10000"            # 10000 sats
else
    # Node 1 offers BTC, wants ETH (default)
    OFFER_CHAIN="BTC"
    REQUEST_CHAIN="ETH"
    OFFER_AMOUNT="10000"                # 10000 sats
    REQUEST_AMOUNT="10000000000000000"  # 0.01 ETH in wei
fi

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
    log "Starting Node 1 (Maker - BTC) with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE1_DIR --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/$NODE1_PORT --log-level $LOG_LEVEL > /tmp/node1.log 2>&1 &
    sleep 2

    log "Starting Node 2 (Taker - ETH) with log-level=$LOG_LEVEL..."
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

show_balances() {
    log "Getting wallet balances..."

    # BTC balance on Node 1
    local btc_scan=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_scanBalance","params":{"symbol":"BTC","gap_limit":20},"id":1}')
    local btc_total=$(echo "$btc_scan" | jq -r '.result.total_balance // 0')

    # ETH address on Node 2
    local eth_addr=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"ETH"},"id":1}' | jq -r '.result.address')

    echo ""
    info "=== Node 1 (BTC) Wallet ==="
    info "Total BTC: $btc_total sats"
    echo "$btc_scan" | jq -r '.result.addresses[] | "  \(.path): \(.address) = \(.balance) sats"' 2>/dev/null || echo "  (no addresses with funds)"

    echo ""
    info "=== Node 2 (ETH) Wallet ==="
    info "ETH Address (Sepolia): $eth_addr"
    info "(Check balance on https://sepolia.etherscan.io)"

    echo "$eth_addr" > /tmp/node2_eth_addr
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
    log "Creating order on Node 1 (offering $OFFER_AMOUNT sats BTC for $REQUEST_AMOUNT wei ETH)..."

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_create\",
        \"params\":{
            \"offer_chain\":\"$OFFER_CHAIN\",
            \"offer_amount\":$OFFER_AMOUNT,
            \"request_chain\":\"$REQUEST_CHAIN\",
            \"request_amount\":$REQUEST_AMOUNT,
            \"preferred_methods\":[\"htlc\"]
        },
        \"id\":1
    }")

    ORDER_ID=$(echo "$result" | jq -r '.result.id // .result.order_id // empty')
    if [ -z "$ORDER_ID" ]; then
        error "Failed to create order: $result"
    fi

    log "Order created: $ORDER_ID"
    echo "$ORDER_ID" > /tmp/cross_order_id
    sleep 2
}

take_order() {
    log "Taking order on Node 2..."

    ORDER_ID=$(cat /tmp/cross_order_id)
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
    echo "$TRADE_ID" > /tmp/cross_trade_id
    sleep 2
}

init_cross_chain_swap() {
    log "Initializing cross-chain swap (BTC ↔ ETH)..."
    TRADE_ID=$(cat /tmp/cross_trade_id)

    # Node 1 is initiator (offers BTC)
    local init1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_initCrossChain\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"initiator\"
        },
        \"id\":1
    }")

    if echo "$init1" | grep -q '"error"'; then
        warn "Cross-chain init on Node 1: $init1"
    else
        log "Node 1 (BTC) cross-chain swap initialized"
        local swap_type=$(echo "$init1" | jq -r '.result.swap_type // empty')
        info "Swap type: $swap_type"
    fi

    # Node 2 is responder (offers ETH)
    # Retry loop: wait for P2P message with secret hash and pubkey from Node 1
    log "Waiting for Node 2 to receive secret hash and pubkey from Node 1..."
    local max_retries=30
    local retry=0
    local init2=""

    while [ $retry -lt $max_retries ]; do
        init2=$(curl -s $NODE2_API -d "{
            \"jsonrpc\":\"2.0\",
            \"method\":\"swap_initCrossChain\",
            \"params\":{
                \"trade_id\":\"$TRADE_ID\",
                \"role\":\"responder\"
            },
            \"id\":1
        }")

        if echo "$init2" | grep -q '"result"'; then
            log "Node 2 (ETH) cross-chain swap initialized"
            break
        fi

        # Check for specific error (waiting for P2P message)
        if echo "$init2" | grep -q 'not yet received'; then
            echo -n "."
            sleep 1
            retry=$((retry + 1))
        elif echo "$init2" | grep -q 'malformed public key: invalid length: 0'; then
            echo -n "."
            sleep 1
            retry=$((retry + 1))
        else
            # Unknown error
            warn "Cross-chain init on Node 2: $init2"
            break
        fi
    done

    if [ $retry -eq $max_retries ]; then
        warn "Node 2 init timed out after ${max_retries}s - P2P message may not have arrived"
        echo "$init2"
    fi

    # Wait for pubkey exchange to complete (HTLC address becomes available)
    sleep 2
    log "Waiting for pubkey exchange..."
    wait_for_htlc_address || warn "HTLC address not immediately available, will retry during funding"
}

get_swap_type() {
    TRADE_ID=$(cat /tmp/cross_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getSwapType\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local swap_type=$(echo "$result" | jq -r '.result.swap_type // empty')
    info "Swap type: $swap_type"
}

# Wait for HTLC address to become available (pubkey exchange)
# For BTC→ETH: offer_htlc_address on Node 1
# For ETH→BTC: request_htlc_address on Node 2
wait_for_htlc_address() {
    TRADE_ID=$(cat /tmp/cross_trade_id)
    local max_retries=30
    local retry=0

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        # ETH→BTC: BTC is request chain, funded by Node 2 (responder)
        log "Waiting for BTC HTLC address on Node 2 (pubkey exchange)..."
        local api=$NODE2_API
        local field="request_htlc_address"
    else
        # BTC→ETH: BTC is offer chain, funded by Node 1 (initiator)
        log "Waiting for BTC HTLC address on Node 1 (pubkey exchange)..."
        local api=$NODE1_API
        local field="offer_htlc_address"
    fi

    while [ $retry -lt $max_retries ]; do
        local status=$(curl -s $api -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
        local htlc_addr=$(echo "$status" | jq -r ".result.$field // empty")

        if [ -n "$htlc_addr" ]; then
            info "BTC HTLC Address available: $htlc_addr"
            echo "$htlc_addr" > /tmp/btc_htlc_address
            return 0
        fi

        retry=$((retry + 1))
        echo -n "."
        sleep 1
    done

    echo ""
    warn "HTLC address not available after $max_retries seconds"
    return 1
}

# BTC HTLC operations
# For BTC→ETH: Node 1 (initiator) funds offer chain
# For ETH→BTC: Node 2 (responder) funds request chain
fund_btc_htlc() {
    TRADE_ID=$(cat /tmp/cross_trade_id)

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        log "Funding BTC HTLC (Node 2 - responder)..."
        local api=$NODE2_API
        local field="request_htlc_address"
    else
        log "Funding BTC HTLC (Node 1 - initiator)..."
        local api=$NODE1_API
        local field="offer_htlc_address"
    fi

    # First, get the HTLC address (may need to wait for pubkey exchange)
    local htlc_addr=$(cat /tmp/btc_htlc_address 2>/dev/null)
    if [ -z "$htlc_addr" ]; then
        local status=$(curl -s $api -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
        htlc_addr=$(echo "$status" | jq -r ".result.$field // empty")
    fi

    if [ -z "$htlc_addr" ]; then
        warn "No BTC HTLC address found. Waiting for pubkey exchange..."
        if ! wait_for_htlc_address; then
            error "Failed to get HTLC address. Make sure both nodes have initialized."
        fi
        htlc_addr=$(cat /tmp/btc_htlc_address)
    fi

    info "BTC HTLC Address: $htlc_addr"

    # Fund using swap_fund
    local result=$(curl -s $api -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_fund\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local txid=$(echo "$result" | jq -r '.result.txid // empty')
    if [ -n "$txid" ]; then
        log "BTC funding tx: $txid"
        echo "$txid" > /tmp/btc_htlc_funding_tx
    else
        warn "BTC funding: $result"
    fi
}

# EVM HTLC operations
# For BTC→ETH: Node 2 (responder) creates ETH HTLC on request chain
# For ETH→BTC: Node 1 (initiator) creates ETH HTLC on offer chain
create_eth_htlc() {
    TRADE_ID=$(cat /tmp/cross_trade_id)

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        log "Creating ETH HTLC (Node 1 - initiator)..."
        local api=$NODE1_API
        local chain=$OFFER_CHAIN
    else
        log "Creating ETH HTLC (Node 2 - responder)..."
        local api=$NODE2_API
        local chain=$REQUEST_CHAIN
    fi

    local result=$(curl -s $api -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmCreate\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$chain\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "ETH HTLC created: $tx_hash"
        echo "$tx_hash" > /tmp/eth_htlc_tx
    else
        warn "ETH HTLC creation: $result"
    fi
}

# Claim operations
# For BTC→ETH: Initiator (Node 1) claims ETH, Responder (Node 2) claims BTC
# For ETH→BTC: Initiator (Node 1) claims BTC, Responder (Node 2) claims ETH
claim_eth() {
    TRADE_ID=$(cat /tmp/cross_trade_id)

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        log "Responder (Node 2) claiming ETH..."
        local api=$NODE2_API
        local chain=$OFFER_CHAIN
    else
        log "Initiator (Node 1) claiming ETH (reveals secret)..."
        local api=$NODE1_API
        local chain=$REQUEST_CHAIN
    fi

    local result=$(curl -s $api -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmClaim\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$chain\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.claim_tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "ETH claimed: $tx_hash"
        if [ "$SWAP_DIRECTION" != "eth-btc" ]; then
            info "Secret has been revealed on-chain!"
        fi
    else
        warn "ETH claim: $result"
    fi
}

claim_btc() {
    TRADE_ID=$(cat /tmp/cross_trade_id)

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        log "Initiator (Node 1) claiming BTC (reveals secret)..."
        local api=$NODE1_API
        local chain=$REQUEST_CHAIN
    else
        log "Responder (Node 2) claiming BTC..."
        local api=$NODE2_API
        local chain=$OFFER_CHAIN
    fi

    local result=$(curl -s $api -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcClaim\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$chain\"
        },
        \"id\":1
    }")

    local txid=$(echo "$result" | jq -r '.result.claim_txid // empty')
    if [ -n "$txid" ]; then
        log "BTC claimed: $txid"
        if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
            info "Secret has been revealed on-chain!"
        fi
    else
        warn "BTC claim: $result"
    fi
}

# Legacy function names for compatibility
claim_eth_initiator() { claim_eth; }
claim_btc_responder() { claim_btc; }

show_status() {
    log "Current swap status..."
    TRADE_ID=$(cat /tmp/cross_trade_id 2>/dev/null || echo "")

    if [ -z "$TRADE_ID" ]; then
        warn "No active trade found"
        return
    fi

    echo ""
    info "=== Node 1 (BTC Initiator) Status ==="
    local status1=$(curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
    echo "$status1" | jq '.result'

    # Highlight key fields
    local htlc_addr=$(echo "$status1" | jq -r '.result.offer_htlc_address // "not available"')
    local swap_type=$(echo "$status1" | jq -r '.result.swap_type // "unknown"')
    info "BTC HTLC Address: $htlc_addr"
    info "Swap Type: $swap_type"

    echo ""
    info "=== Node 2 (ETH Responder) Status ==="
    local status2=$(curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}")
    echo "$status2" | jq '.result'

    # Highlight key fields
    local evm_addr=$(echo "$status2" | jq -r '.result.request_evm_address // "not available"')
    info "ETH Wallet Address: $evm_addr"
}

show_next_steps() {
    echo ""
    echo "========================================"
    echo " CROSS-CHAIN SWAP SETUP COMPLETE"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/cross_trade_id 2>/dev/null || echo "N/A")

    echo "Trade ID: $TRADE_ID"
    echo "Swap Type: BTC ↔ ETH (Bitcoin to EVM)"
    echo ""
    echo "IMPORTANT: Make sure you have testnet funds!"
    echo "  - BTC Testnet: Already funded (see wallet scan above)"
    echo "  - Sepolia ETH: https://sepoliafaucet.com/"
    echo ""
    echo "SWAP FLOW:"
    echo "  1. Initiator (Node 1) funds BTC HTLC"
    echo "  2. Responder (Node 2) creates ETH HTLC"
    echo "  3. Initiator claims ETH (reveals secret)"
    echo "  4. Responder claims BTC (uses revealed secret)"
    echo ""
    echo "COMMANDS:"
    echo "  ./scripts/test-evm-btc-swap.sh fund-btc    - Fund BTC HTLC"
    echo "  ./scripts/test-evm-btc-swap.sh create-eth  - Create ETH HTLC"
    echo "  ./scripts/test-evm-btc-swap.sh claim-eth   - Initiator claims ETH"
    echo "  ./scripts/test-evm-btc-swap.sh claim-btc   - Responder claims BTC"
    echo ""
}

full_swap() {
    log "=== STARTING FULL CROSS-CHAIN SWAP ($OFFER_CHAIN ↔ $REQUEST_CHAIN) ==="

    if [ "$SWAP_DIRECTION" = "eth-btc" ]; then
        # ETH→BTC: Create ETH first, then fund BTC
        create_eth_htlc
        info "Waiting for ETH HTLC confirmation..."
        sleep 10

        fund_btc_htlc
        info "Waiting for BTC funding confirmation..."
        sleep 5

        # Initiator claims BTC (reveals secret)
        claim_btc
        info "Waiting for BTC claim confirmation..."
        sleep 5

        # Responder claims ETH using revealed secret
        claim_eth
    else
        # BTC→ETH: Fund BTC first, then create ETH
        fund_btc_htlc
        info "Waiting for BTC funding confirmation..."
        sleep 5

        create_eth_htlc
        info "Waiting for ETH HTLC confirmation..."
        sleep 10

        # Initiator claims ETH (reveals secret)
        claim_eth
        info "Waiting for ETH claim confirmation..."
        sleep 10

        # Responder claims BTC using revealed secret
        claim_btc
    fi

    echo ""
    log "=== CROSS-CHAIN SWAP COMPLETE ==="
    show_status
}

# Main
case "${1:-help}" in
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
        init_cross_chain_swap
        get_swap_type
        show_status
        show_next_steps
        ;;
    fund-btc)
        fund_btc_htlc
        ;;
    create-eth)
        create_eth_htlc
        ;;
    claim-eth)
        claim_eth_initiator
        ;;
    claim-btc)
        claim_btc_responder
        ;;
    full)
        full_swap
        ;;
    status)
        show_status
        ;;
    balances)
        show_balances
        ;;
    wait-htlc)
        wait_for_htlc_address
        ;;
    type)
        get_swap_type
        ;;
    all)
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_cross_chain_swap
        fund_btc_htlc
        sleep 3
        create_eth_htlc
        show_status
        ;;
    *)
        echo "Klingon EVM ↔ Bitcoin Cross-Chain Atomic Swap Test Script"
        echo ""
        echo "Current direction: $SWAP_DIRECTION ($OFFER_CHAIN → $REQUEST_CHAIN)"
        echo ""
        echo "Usage: $0 {command}"
        echo ""
        echo "Commands:"
        echo "  clean       - Kill all klingond processes"
        echo "  start       - Start both test nodes"
        echo "  setup       - Full setup: nodes, wallets, order, cross-chain init"
        echo "  fund-btc    - Fund BTC HTLC"
        echo "  create-eth  - Create ETH HTLC"
        echo "  claim-eth   - Claim ETH"
        echo "  claim-btc   - Claim BTC"
        echo "  full        - Run full swap after setup"
        echo "  status      - Show current swap status"
        echo "  balances    - Show wallet balances"
        echo "  wait-htlc   - Wait for HTLC address to become available"
        echo "  type        - Show swap type"
        echo "  all         - Setup and create both HTLCs"
        echo ""
        echo "Environment variables:"
        echo "  SWAP_DIRECTION - 'btc-eth' (default) or 'eth-btc'"
        echo "  LOG_LEVEL      - Log level (debug, info, warn, error). Default: info"
        echo ""
        echo "Example workflow (BTC→ETH):"
        echo "  1. $0 setup      # Start nodes, create order, init swap"
        echo "  2. $0 fund-btc   # Fund BTC HTLC (Node 1)"
        echo "  3. $0 create-eth # Create ETH HTLC (Node 2)"
        echo "  4. $0 claim-eth  # Node 1 claims ETH (reveals secret)"
        echo "  5. $0 claim-btc  # Node 2 claims BTC"
        echo ""
        echo "Example workflow (ETH→BTC):"
        echo "  1. SWAP_DIRECTION=eth-btc $0 setup"
        echo "  2. $0 create-eth # Create ETH HTLC (Node 1)"
        echo "  3. $0 fund-btc   # Fund BTC HTLC (Node 2)"
        echo "  4. $0 claim-btc  # Node 1 claims BTC (reveals secret)"
        echo "  5. $0 claim-eth  # Node 2 claims ETH"
        echo ""
        exit 1
        ;;
esac
