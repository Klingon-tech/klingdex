#!/bin/bash
# Klingon HTLC Atomic Swap Test Script
# Tests the full HTLC swap flow between two local nodes
#
# HTLC Flow (different from MuSig2):
# 1. Initiator creates secret + hash
# 2. Initiator sends hash to responder
# 3. Both create P2WSH addresses using the hash
# 4. Both fund their P2WSH addresses
# 5. Initiator reveals secret
# 6. Responder claims with secret (gets initiator's coins)
# 7. Initiator claims with secret (gets responder's coins)

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

# Swap amounts
BTC_AMOUNT=10000     # 10000 sats
LTC_AMOUNT=100000    # 100000 litoshis

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

    # Check for JSON-RPC error
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
    info "=== Node 1 (BTC) Wallet ==="
    info "Total: $btc_total sats"

    echo ""
    info "=== Node 2 (LTC) Wallet ==="
    info "Total: $ltc_total litoshis"

    echo "$btc_scan" > /tmp/htlc_node1_btc_scan.json
    echo "$ltc_scan" > /tmp/htlc_node2_ltc_scan.json

    echo "$btc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/htlc_node1_btc_funded.json
    echo "$ltc_scan" | jq -r '.result.addresses | sort_by(.balance) | reverse | .[0] // empty' > /tmp/htlc_node2_ltc_funded.json
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
    log "Creating HTLC order on Node 1 (offering $BTC_AMOUNT sats BTC for $LTC_AMOUNT litoshis LTC)..."

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"orders_create\",
        \"params\":{
            \"offer_chain\":\"BTC\",
            \"offer_amount\":$BTC_AMOUNT,
            \"request_chain\":\"LTC\",
            \"request_amount\":$LTC_AMOUNT,
            \"preferred_methods\":[\"htlc\"]
        },
        \"id\":1
    }")

    ORDER_ID=$(echo "$result" | jq -r '.result.id // .result.order_id // empty')
    if [ -z "$ORDER_ID" ]; then
        error "Failed to create order: $result"
    fi

    log "Order created: $ORDER_ID"
    echo "$ORDER_ID" > /tmp/htlc_order_id
    sleep 2
}

take_order() {
    log "Taking HTLC order on Node 2..."

    ORDER_ID=$(cat /tmp/htlc_order_id)
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
    echo "$TRADE_ID" > /tmp/htlc_trade_id
    sleep 2
}

init_swaps() {
    log "Initializing HTLC swaps on both nodes..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    # Initialize on Node 1 (initiator/maker - generates secret)
    local init1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_init\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    if echo "$init1" | grep -q '"error"'; then
        error "Swap init failed on Node 1: $init1"
    fi
    log "Node 1 (initiator) initialized - secret generated"

    # Wait for P2P message to propagate, then retry Node 2 init
    local max_retries=5
    local retry=0
    local init2=""

    while [ $retry -lt $max_retries ]; do
        sleep 2

        # Initialize on Node 2 (responder/taker - receives secret hash)
        init2=$(curl -s $NODE2_API -d "{
            \"jsonrpc\":\"2.0\",
            \"method\":\"swap_init\",
            \"params\":{\"trade_id\":\"$TRADE_ID\"},
            \"id\":1
        }")

        if echo "$init2" | grep -q '"error"'; then
            retry=$((retry + 1))
            if [ $retry -lt $max_retries ]; then
                warn "Waiting for maker pubkey... (attempt $retry/$max_retries)"
            fi
        else
            break
        fi
    done

    if echo "$init2" | grep -q '"error"'; then
        error "Swap init failed on Node 2 after $max_retries retries: $init2"
    fi
    log "Node 2 (responder) initialized - received secret hash"

    log "HTLC swaps initialized on both nodes"
}

get_secret_info() {
    log "Getting secret/hash info..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    # Get secret info from Node 1 (initiator has the secret)
    local secret_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcGetSecret\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local has_secret=$(echo "$secret_result" | jq -r '.result.has_secret // false')
    local secret=$(echo "$secret_result" | jq -r '.result.secret // "N/A"')
    local secret_hash=$(echo "$secret_result" | jq -r '.result.secret_hash // "N/A"')

    echo ""
    info "=== HTLC Secret Info (Node 1 - Initiator) ==="
    info "Has Secret: $has_secret"
    if [ "$has_secret" = "true" ]; then
        info "Secret: ${secret:0:16}..."
    fi
    info "Secret Hash: ${secret_hash:0:16}..."

    echo "$secret" > /tmp/htlc_secret
    echo "$secret_hash" > /tmp/htlc_secret_hash
}

get_escrow_addresses() {
    log "Getting HTLC P2WSH escrow addresses..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    # Node 1 (initiator) gets BTC escrow address
    local btc_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local btc_escrow=$(echo "$btc_result" | jq -r '.result.address // .result.htlc_address // empty')
    local btc_method=$(echo "$btc_result" | jq -r '.result.method // "unknown"')

    # Node 2 (responder) gets LTC escrow address
    local ltc_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_getAddress\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local ltc_escrow=$(echo "$ltc_result" | jq -r '.result.address // .result.htlc_address // empty')
    local ltc_method=$(echo "$ltc_result" | jq -r '.result.method // "unknown"')

    if [ -z "$btc_escrow" ] || [ "$btc_escrow" = "null" ]; then
        warn "BTC escrow address not ready yet. Wait for secret hash exchange."
        return 1
    fi

    if [ -z "$ltc_escrow" ] || [ "$ltc_escrow" = "null" ]; then
        warn "LTC escrow address not ready yet. Wait for secret hash exchange."
        return 1
    fi

    echo "$btc_escrow" > /tmp/htlc_btc_escrow
    echo "$ltc_escrow" > /tmp/htlc_ltc_escrow

    echo ""
    info "BTC Escrow (P2WSH - $btc_method): $btc_escrow"
    info "LTC Escrow (P2WSH - $ltc_method): $ltc_escrow"
}

fund_escrows() {
    log "Auto-funding HTLC escrow addresses using swap_fund..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

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
        echo "$btc_txid" > /tmp/htlc_btc_funding_txid
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
        echo "$ltc_txid" > /tmp/htlc_ltc_funding_txid
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
    log "Manually funding HTLC escrow addresses using wallet_send..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)
    BTC_ESCROW=$(cat /tmp/htlc_btc_escrow)
    LTC_ESCROW=$(cat /tmp/htlc_ltc_escrow)

    # Get funded address info
    local btc_funded="/tmp/htlc_node1_btc_funded.json"
    local ltc_funded="/tmp/htlc_node2_ltc_funded.json"

    local btc_change=0
    local btc_index=0
    if [ -f "$btc_funded" ] && [ -s "$btc_funded" ]; then
        local btc_is_change=$(jq -r '.is_change // false' "$btc_funded")
        btc_index=$(jq -r '.index // 0' "$btc_funded" | sed 's/null/0/')
        if [ "$btc_is_change" = "true" ]; then
            btc_change=1
        fi
    fi

    local ltc_change=0
    local ltc_index=0
    if [ -f "$ltc_funded" ] && [ -s "$ltc_funded" ]; then
        local ltc_is_change=$(jq -r '.is_change // false' "$ltc_funded")
        ltc_index=$(jq -r '.index // 0' "$ltc_funded" | sed 's/null/0/')
        if [ "$ltc_is_change" = "true" ]; then
            ltc_change=1
        fi
    fi

    info "Funding BTC escrow with $BTC_AMOUNT sats..."
    local btc_result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"wallet_send\",
        \"params\":{
            \"symbol\":\"BTC\",
            \"to\":\"$BTC_ESCROW\",
            \"amount\":$BTC_AMOUNT,
            \"change\":$btc_change,
            \"index\":$btc_index,
            \"fee_rate\":2
        },
        \"id\":1
    }")

    local btc_txid=$(echo "$btc_result" | jq -r '.result.txid // empty')
    if [ -n "$btc_txid" ]; then
        log "BTC funding tx: $btc_txid"
        echo "$btc_txid" > /tmp/htlc_btc_funding_txid
    else
        warn "BTC funding result: $btc_result"
    fi

    info "Funding LTC escrow with $LTC_AMOUNT litoshis..."
    local ltc_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"wallet_send\",
        \"params\":{
            \"symbol\":\"LTC\",
            \"to\":\"$LTC_ESCROW\",
            \"amount\":$LTC_AMOUNT,
            \"change\":$ltc_change,
            \"index\":$ltc_index,
            \"fee_rate\":2
        },
        \"id\":1
    }")

    local ltc_txid=$(echo "$ltc_result" | jq -r '.result.txid // empty')
    if [ -n "$ltc_txid" ]; then
        log "LTC funding tx: $ltc_txid"
        echo "$ltc_txid" > /tmp/htlc_ltc_funding_txid
    else
        warn "LTC funding result: $ltc_result"
    fi
}

set_funding_info() {
    log "Setting funding info..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    local btc_txid=$(cat /tmp/htlc_btc_funding_txid 2>/dev/null)
    local ltc_txid=$(cat /tmp/htlc_ltc_funding_txid 2>/dev/null)

    if [ -z "$btc_txid" ] || [ -z "$ltc_txid" ]; then
        error "Funding txids not found. Run 'fund' first."
    fi

    # Set BTC funding on Node 1
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

    # Set LTC funding on Node 2
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
    log "Funding info exchanged via P2P"
}

check_funding() {
    log "Checking funding status..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    local check1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_checkFunding\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local local1=$(echo "$check1" | jq -r '.result.local_funded // false')
    local remote1=$(echo "$check1" | jq -r '.result.remote_funded // false')

    info "Node 1: local_funded=$local1, remote_funded=$remote1"

    local check2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_checkFunding\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local local2=$(echo "$check2" | jq -r '.result.local_funded // false')
    local remote2=$(echo "$check2" | jq -r '.result.remote_funded // false')

    info "Node 2: local_funded=$local2, remote_funded=$remote2"

    if [ "$local1" = "true" ] && [ "$remote1" = "true" ] && [ "$local2" = "true" ] && [ "$remote2" = "true" ]; then
        log "Both nodes see all funding!"
        return 0
    else
        warn "Waiting for funding to be confirmed on both sides..."
        return 1
    fi
}

reveal_secret() {
    log "Revealing secret (initiator broadcasts to responder)..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    local result=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcRevealSecret\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local secret=$(echo "$result" | jq -r '.result.secret // empty')
    if [ -n "$secret" ]; then
        log "Secret revealed: ${secret:0:16}..."
        echo "$secret" > /tmp/htlc_revealed_secret
    else
        local err=$(echo "$result" | jq -r '.error.message // empty')
        warn "Failed to reveal secret: ${err:-$result}"
        return 1
    fi

    # Check if responder received the secret
    sleep 2
    local resp_result=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcGetSecret\",
        \"params\":{\"trade_id\":\"$TRADE_ID\"},
        \"id\":1
    }")

    local resp_secret=$(echo "$resp_result" | jq -r '.result.secret // empty')
    if [ -n "$resp_secret" ] && [ "$resp_secret" != "null" ]; then
        log "Responder received secret!"
    else
        warn "Responder hasn't received secret yet - check P2P connection"
    fi
}

claim_htlc() {
    log "Claiming HTLC outputs..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    # Initiator claims on LTC (responder's chain)
    info "Initiator (Node 1) claiming on LTC..."
    local claim1=$(curl -s $NODE1_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcClaim\",
        \"params\":{\"trade_id\":\"$TRADE_ID\", \"chain\":\"LTC\"},
        \"id\":1
    }")

    local txid1=$(echo "$claim1" | jq -r '.result.txid // empty')
    if [ -n "$txid1" ]; then
        log "Initiator claimed LTC: $txid1"
        echo "$txid1" > /tmp/htlc_claim1_txid
    else
        local err=$(echo "$claim1" | jq -r '.error.message // empty')
        warn "Initiator claim failed: ${err:-$claim1}"
    fi

    # Wait for transaction to propagate
    sleep 3

    # Responder claims on BTC (initiator's chain)
    # First, responder needs to extract secret from initiator's claim tx
    if [ -n "$txid1" ]; then
        info "Responder extracting secret from claim tx..."
        curl -s $NODE2_API -d "{
            \"jsonrpc\":\"2.0\",
            \"method\":\"swap_htlcExtractSecret\",
            \"params\":{\"trade_id\":\"$TRADE_ID\", \"txid\":\"$txid1\", \"chain\":\"LTC\"},
            \"id\":1
        }" > /dev/null
    fi

    info "Responder (Node 2) claiming on BTC..."
    local claim2=$(curl -s $NODE2_API -d "{
        \"jsonrpc\":\"2.0\",
        \"method\":\"swap_htlcClaim\",
        \"params\":{\"trade_id\":\"$TRADE_ID\", \"chain\":\"BTC\"},
        \"id\":1
    }")

    local txid2=$(echo "$claim2" | jq -r '.result.txid // empty')
    if [ -n "$txid2" ]; then
        log "Responder claimed BTC: $txid2"
        echo "$txid2" > /tmp/htlc_claim2_txid
    else
        local err=$(echo "$claim2" | jq -r '.error.message // empty')
        warn "Responder claim failed: ${err:-$claim2}"
    fi

    echo ""
    if [ -n "$txid1" ] && [ -n "$txid2" ]; then
        highlight "HTLC SWAP COMPLETE!"
        echo "  LTC claim: $txid1"
        echo "  BTC claim: $txid2"
    else
        warn "HTLC swap incomplete - check errors above"
    fi
}

show_status() {
    log "Current HTLC swap status..."
    TRADE_ID=$(cat /tmp/htlc_trade_id)

    echo ""
    info "=== Node 1 (Maker/BTC - Initiator) Status ==="
    curl -s $NODE1_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Node 2 (Taker/LTC - Responder) Status ==="
    curl -s $NODE2_API -d "{\"jsonrpc\":\"2.0\",\"method\":\"swap_status\",\"params\":{\"trade_id\":\"$TRADE_ID\"},\"id\":1}" | jq '.result'

    echo ""
    info "=== Secret Info ==="
    get_secret_info 2>/dev/null || true
}

show_next_steps() {
    echo ""
    echo "========================================"
    echo " HTLC SWAP SETUP COMPLETE - NEXT STEPS"
    echo "========================================"
    echo ""

    TRADE_ID=$(cat /tmp/htlc_trade_id 2>/dev/null || echo "N/A")
    BTC_ESCROW=$(cat /tmp/htlc_btc_escrow 2>/dev/null || echo "N/A")
    LTC_ESCROW=$(cat /tmp/htlc_ltc_escrow 2>/dev/null || echo "N/A")
    SECRET=$(cat /tmp/htlc_secret 2>/dev/null || echo "N/A")
    SECRET_HASH=$(cat /tmp/htlc_secret_hash 2>/dev/null || echo "N/A")

    echo "Trade ID: $TRADE_ID"
    echo ""
    echo "HTLC P2WSH Escrow Addresses:"
    echo "  BTC: $BTC_ESCROW"
    echo "  LTC: $LTC_ESCROW"
    echo ""
    echo "Secret Hash: ${SECRET_HASH:0:32}..."
    echo ""
    echo "HTLC Flow:"
    echo "  1. Fund escrows:     ./scripts/test-htlc-swap.sh fund"
    echo "  2. Set funding info: ./scripts/test-htlc-swap.sh setfunding"
    echo "  3. Check funding:    ./scripts/test-htlc-swap.sh check"
    echo "  4. Reveal secret:    ./scripts/test-htlc-swap.sh reveal"
    echo "  5. Claim:            ./scripts/test-htlc-swap.sh claim"
    echo ""
    highlight "After claiming, both parties receive their swapped coins!"
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
        echo " HTLC ATOMIC SWAP TEST"
        echo "========================================"
        echo ""
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_swaps
        sleep 2  # Wait for P2P secret hash exchange
        get_secret_info
        get_escrow_addresses
        show_status
        show_next_steps
        ;;
    fund)
        fund_escrows
        ;;
    setfunding)
        set_funding_info
        ;;
    check)
        check_funding
        ;;
    reveal)
        reveal_secret
        ;;
    claim)
        claim_htlc
        ;;
    secret)
        get_secret_info
        ;;
    status)
        show_status
        ;;
    balances)
        show_balances
        ;;
    all)
        cleanup
        start_nodes
        setup_wallets
        show_balances
        connect_peers
        create_order
        take_order
        init_swaps
        sleep 2
        get_secret_info
        get_escrow_addresses
        fund_escrows
        set_funding_info
        check_funding || true
        show_status
        ;;
    *)
        echo "Usage: $0 {clean|start|setup|fund|setfunding|check|reveal|claim|secret|status|balances|all}"
        echo ""
        echo "  clean      - Kill all klingond processes"
        echo "  start      - Start both test nodes"
        echo "  setup      - Full setup: start nodes, wallets, order, swap init (default)"
        echo "  fund       - Fund the P2WSH escrow addresses"
        echo "  setfunding - Set funding transaction info"
        echo "  check      - Check funding status"
        echo "  reveal     - Reveal secret (initiator only)"
        echo "  claim      - Claim HTLC outputs (both parties)"
        echo "  secret     - Show secret/hash info"
        echo "  status     - Show current swap status"
        echo "  balances   - Show wallet balances"
        echo "  all        - Everything up to funding check"
        echo ""
        echo "HTLC Flow:"
        echo "  1. setup      - Initialize HTLC swap"
        echo "  2. fund       - Fund escrow addresses"
        echo "  3. setfunding - Register funding txs"
        echo "  4. check      - Verify funding"
        echo "  5. reveal     - Initiator reveals secret"
        echo "  6. claim      - Both parties claim their coins"
        exit 1
        ;;
esac
