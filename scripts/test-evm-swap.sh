#!/bin/bash
# Klingon EVM HTLC Atomic Swap Test Script
# Tests the full EVM swap flow between two local nodes
# Uses Sepolia (ETH testnet) and BSC Testnet

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

# EVM Test Configuration
# Node 1 offers ETH (Sepolia), wants BSC (Testnet)
OFFER_CHAIN="ETH"
REQUEST_CHAIN="BSC"
OFFER_AMOUNT="10000000000000000"    # 0.01 ETH in wei
REQUEST_AMOUNT="10000000000000000"  # 0.01 BNB in wei

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
    log "Starting Node 1 (Maker - ETH) with log-level=$LOG_LEVEL..."
    $BIN --testnet --data-dir $NODE1_DIR --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/$NODE1_PORT --log-level $LOG_LEVEL > /tmp/node1.log 2>&1 &
    NODE1_PID=$!
    sleep 2

    log "Starting Node 2 (Taker - BSC) with log-level=$LOG_LEVEL..."
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

show_evm_addresses() {
    log "Getting EVM wallet addresses..."

    # Get ETH addresses
    local eth1=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"ETH"},"id":1}' | jq -r '.result.address')
    local eth2=$(curl -s $NODE2_API -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"ETH"},"id":1}' | jq -r '.result.address')

    echo ""
    info "=== Node 1 EVM Addresses ==="
    info "ETH (Sepolia): $eth1"

    echo ""
    info "=== Node 2 EVM Addresses ==="
    info "ETH (Sepolia): $eth2"

    # Save for later
    echo "$eth1" > /tmp/node1_eth_addr
    echo "$eth2" > /tmp/node2_eth_addr
}

show_contracts() {
    log "Getting deployed HTLC contract addresses..."

    local contracts=$(curl -s $NODE1_API -d '{"jsonrpc":"2.0","method":"swap_evmGetContracts","id":1}')
    echo ""
    info "=== Deployed HTLC Contracts ==="
    echo "$contracts" | jq -r '.result.contracts[] | "  Chain ID \(.chain_id): \(.contract_address)"'
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
    log "Creating order on Node 1 (offering $OFFER_AMOUNT wei ETH for $REQUEST_AMOUNT wei BSC)..."

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
    echo "$ORDER_ID" > /tmp/evm_order_id
    sleep 2
}

take_order() {
    log "Taking order on Node 2..."

    ORDER_ID=$(cat /tmp/evm_order_id)
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
    echo "$TRADE_ID" > /tmp/evm_trade_id
    sleep 2
}

init_cross_chain_swap() {
    log "Initializing cross-chain EVM swap..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    # Initialize cross-chain swap on Node 1 (initiator)
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
        log "Node 1 cross-chain swap initialized"
    fi

    # Initialize cross-chain swap on Node 2 (responder)
    local init2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_initCrossChain\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"role\":\"responder\"
        },
        \"id\":1
    }")

    if echo "$init2" | grep -q '"error"'; then
        warn "Cross-chain init on Node 2: $init2"
    else
        log "Node 2 cross-chain swap initialized"
    fi
}

create_initiator_htlc() {
    log "Creating HTLC on initiator's chain (ETH)..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmCreate\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$OFFER_CHAIN\"
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

create_responder_htlc() {
    log "Creating HTLC on responder's chain (BSC)..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmCreate\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$REQUEST_CHAIN\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "BSC HTLC created: $tx_hash"
        echo "$tx_hash" > /tmp/bsc_htlc_tx
    else
        warn "BSC HTLC creation: $result"
    fi
}

claim_initiator() {
    log "Initiator claiming on responder's chain (BSC - reveals secret)..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmClaim\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$REQUEST_CHAIN\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.claim_tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "Initiator claimed BSC: $tx_hash"
        echo "$tx_hash" > /tmp/initiator_claim_tx
    else
        warn "Initiator claim: $result"
    fi
}

wait_for_secret() {
    log "Responder waiting for secret reveal..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local result=$(curl -s --max-time 60 $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmWaitSecret\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$REQUEST_CHAIN\"
        },
        \"id\":1
    }")

    local secret=$(echo "$result" | jq -r '.result.secret // empty')
    if [ -n "$secret" ]; then
        log "Secret revealed: ${secret:0:16}..."
        echo "$secret" > /tmp/evm_secret
    else
        warn "Wait for secret: $result"
    fi
}

get_secret_from_tx() {
    log "Extracting secret from BSC claim transaction..."

    local claim_tx=$(cat /tmp/initiator_claim_tx 2>/dev/null || echo "")
    if [ -z "$claim_tx" ]; then
        error "No claim transaction found. Run 'claim' first."
    fi

    # BSC Testnet RPC
    local BSC_RPC="https://bsc-testnet-rpc.publicnode.com"

    log "Fetching transaction $claim_tx..."
    local tx_data=$(curl -s -X POST "$BSC_RPC" -H "Content-Type: application/json" -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"eth_getTransactionByHash\",
        \"params\":[\"$claim_tx\"],
        \"id\":1
    }")

    local input=$(echo "$tx_data" | jq -r '.result.input // empty')
    if [ -z "$input" ] || [ "$input" == "null" ]; then
        warn "Could not fetch transaction input. The transaction may not be mined yet."
        warn "You can also get the secret from BSCScan: https://testnet.bscscan.com/tx/$claim_tx"
        return 1
    fi

    # Extract secret from calldata
    # claim(bytes32 swapId, bytes32 secret)
    # selector (4 bytes) + swapId (32 bytes) + secret (32 bytes)
    # Total: 4 + 64 + 64 = 132 hex chars (with 0x = 138)
    if [ ${#input} -ge 138 ]; then
        local secret="${input:74:64}"
        log "Secret extracted: ${secret:0:16}..."
        echo "$secret" > /tmp/evm_secret
        echo "$secret"
    else
        warn "Input data too short to extract secret"
        return 1
    fi
}

set_secret() {
    log "Setting secret on Node 2 (responder)..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local secret=$(cat /tmp/evm_secret 2>/dev/null || echo "")
    if [ -z "$secret" ]; then
        warn "No secret found. Trying to extract from claim tx..."
        secret=$(get_secret_from_tx)
        if [ -z "$secret" ]; then
            error "Could not get secret. Please provide it manually."
        fi
    fi

    log "Setting secret: ${secret:0:16}..."
    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmSetSecret\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"secret\":\"$secret\"
        },
        \"id\":1
    }")

    if echo "$result" | grep -q '"error"'; then
        warn "Failed to set secret: $result"
        return 1
    else
        log "Secret set successfully on Node 2"
    fi
}

set_secret_manual() {
    local secret=$1
    if [ -z "$secret" ]; then
        error "Usage: $0 set-secret-manual <hex_secret>"
    fi

    TRADE_ID=$(cat /tmp/evm_trade_id)
    log "Setting secret manually: ${secret:0:16}..."

    # Remove 0x prefix if present
    secret="${secret#0x}"

    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmSetSecret\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"secret\":\"$secret\"
        },
        \"id\":1
    }")

    if echo "$result" | grep -q '"error"'; then
        warn "Failed to set secret: $result"
    else
        log "Secret set successfully"
        echo "$secret" > /tmp/evm_secret
    fi
}

claim_responder() {
    log "Responder claiming on initiator's chain (ETH - using revealed secret)..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    # First ensure we have the secret
    local secret=$(cat /tmp/evm_secret 2>/dev/null || echo "")
    if [ -z "$secret" ]; then
        warn "No secret found. Attempting to extract and set..."
        set_secret || true
    fi

    local result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmClaim\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$OFFER_CHAIN\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.claim_tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "Responder claimed ETH: $tx_hash"
        echo "$tx_hash" > /tmp/responder_claim_tx
    else
        warn "Responder claim: $result"
        echo ""
        info "If the secret is not available, try:"
        info "  1. $0 get-secret        - Extract secret from claim tx"
        info "  2. $0 set-secret        - Set the secret on Node 2"
        info "  3. $0 complete          - Retry claiming"
        info ""
        info "Or manually set the secret:"
        info "  $0 set-secret-manual <hex_secret>"
    fi
}

show_evm_status() {
    log "Checking EVM HTLC status..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    echo ""
    info "=== ETH HTLC Status (Node 1) ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_evmStatus\",\"params\":{\"trade_id\":\"$TRADE_ID\",\"chain\":\"$OFFER_CHAIN\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== BSC HTLC Status (Node 2) ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_evmStatus\",\"params\":{\"trade_id\":\"$TRADE_ID\",\"chain\":\"$REQUEST_CHAIN\"},\"id\":1}" | jq '.result'
}

show_status() {
    log "Current swap status..."
    TRADE_ID=$(cat /tmp/evm_trade_id 2>/dev/null || echo "")

    if [ -z "$TRADE_ID" ]; then
        warn "No active trade found"
        return
    fi

    echo ""
    info "=== Node 1 (Maker/ETH) Status ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Node 2 (Taker/BSC) Status ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'
}

refund_htlc() {
    local chain=$1
    local node_api=$2

    log "Refunding HTLC on $chain..."
    TRADE_ID=$(cat /tmp/evm_trade_id)

    local result=$(curl -s $node_api -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_evmRefund\",
        \"params\":{
            \"trade_id\":\"$TRADE_ID\",
            \"chain\":\"$chain\"
        },
        \"id\":1
    }")

    local tx_hash=$(echo "$result" | jq -r '.result.refund_tx_hash // empty')
    if [ -n "$tx_hash" ]; then
        log "$chain refund tx: $tx_hash"
    else
        warn "$chain refund: $result"
    fi
}

show_next_steps() {
    echo ""
    echo "========================================"
    echo " EVM SWAP SETUP COMPLETE - NEXT STEPS"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/evm_trade_id 2>/dev/null || echo "N/A")

    echo "Trade ID: $TRADE_ID"
    echo ""
    echo "IMPORTANT: Make sure you have testnet funds!"
    echo "  - Sepolia ETH faucet: https://sepoliafaucet.com/"
    echo "  - BSC Testnet faucet: https://testnet.bnbchain.org/faucet-smart"
    echo ""
    echo "NEXT STEPS:"
    echo "  1. ./scripts/test-evm-swap.sh create-htlcs  - Create HTLCs on both chains"
    echo "  2. ./scripts/test-evm-swap.sh claim         - Initiator claims (reveals secret)"
    echo "  3. ./scripts/test-evm-swap.sh complete      - Responder claims with secret"
    echo ""
    echo "Or run all at once:"
    echo "  ./scripts/test-evm-swap.sh full"
    echo ""
}

# Full swap flow
full_swap() {
    log "=== STARTING FULL EVM SWAP ==="

    create_initiator_htlc
    sleep 5  # Wait for tx confirmation

    create_responder_htlc
    sleep 5

    claim_initiator
    sleep 10  # Wait for claim tx to be mined

    # Extract and set the secret for the responder
    log "Extracting secret from claim transaction..."
    get_secret_from_tx || true
    sleep 2

    log "Setting secret on responder..."
    set_secret || true
    sleep 2

    # Responder claims with the secret
    claim_responder

    echo ""
    log "=== EVM SWAP COMPLETE ==="
    show_evm_status
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
        show_evm_addresses
        show_contracts
        connect_peers
        create_order
        take_order
        init_cross_chain_swap
        show_status
        show_next_steps
        ;;
    create-htlcs)
        create_initiator_htlc
        sleep 3
        create_responder_htlc
        ;;
    claim)
        claim_initiator
        ;;
    wait-secret)
        wait_for_secret
        ;;
    get-secret)
        get_secret_from_tx
        ;;
    set-secret)
        set_secret
        ;;
    set-secret-manual)
        set_secret_manual "$2"
        ;;
    complete)
        claim_responder
        ;;
    full)
        full_swap
        ;;
    status)
        show_status
        ;;
    evm-status)
        show_evm_status
        ;;
    contracts)
        show_contracts
        ;;
    addresses)
        show_evm_addresses
        ;;
    refund-eth)
        refund_htlc "ETH" "$NODE1_API"
        ;;
    refund-bsc)
        refund_htlc "BSC" "$NODE2_API"
        ;;
    all)
        cleanup
        start_nodes
        setup_wallets
        show_evm_addresses
        show_contracts
        connect_peers
        create_order
        take_order
        init_cross_chain_swap
        create_initiator_htlc
        sleep 3
        create_responder_htlc
        show_evm_status
        ;;
    *)
        echo "Klingon EVM HTLC Atomic Swap Test Script"
        echo ""
        echo "Usage: $0 {command}"
        echo ""
        echo "Commands:"
        echo "  clean              - Kill all klingond processes"
        echo "  start              - Start both test nodes"
        echo "  setup              - Full setup: start nodes, wallets, order, cross-chain init"
        echo "  create-htlcs       - Create HTLCs on both chains"
        echo "  claim              - Initiator claims (reveals secret)"
        echo "  wait-secret        - Responder waits for secret reveal (requires RPC notifications)"
        echo "  get-secret         - Extract secret from initiator's claim tx"
        echo "  set-secret         - Set extracted secret on responder node"
        echo "  set-secret-manual  - Manually set secret: $0 set-secret-manual <hex_secret>"
        echo "  complete           - Responder claims with revealed secret"
        echo "  full               - Run full swap after setup"
        echo "  status             - Show current swap status"
        echo "  evm-status         - Show EVM HTLC on-chain status"
        echo "  contracts          - Show deployed HTLC contract addresses"
        echo "  addresses          - Show EVM wallet addresses"
        echo "  refund-eth         - Refund ETH HTLC (after timeout)"
        echo "  refund-bsc         - Refund BSC HTLC (after timeout)"
        echo "  all                - Setup and create HTLCs (stops before claiming)"
        echo ""
        echo "Environment variables:"
        echo "  LOG_LEVEL     - Log level for nodes (debug, info, warn, error). Default: info"
        echo ""
        echo "Example workflow:"
        echo "  1. $0 setup           # Start nodes, create order, init swap"
        echo "  2. $0 create-htlcs    # Create HTLCs on both chains"
        echo "  3. $0 claim           # Initiator claims BSC (reveals secret)"
        echo "  4. $0 get-secret      # Extract secret from claim tx"
        echo "  5. $0 set-secret      # Set secret on responder"
        echo "  6. $0 complete        # Responder claims ETH"
        echo ""
        echo "Or simply: $0 setup && $0 full"
        echo ""
        echo "Note: The 'complete' command will automatically try to extract and set"
        echo "      the secret if not already available."
        exit 1
        ;;
esac
