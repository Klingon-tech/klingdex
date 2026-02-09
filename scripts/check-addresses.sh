#!/bin/bash
# Check addresses for all supported chains
# Usage: ./scripts/check-addresses.sh [--testnet] [API_URL]
#
# Options:
#   --testnet   Start testnet nodes, restore wallets, check addresses, cleanup on exit
#   API_URL     API endpoint (default: http://127.0.0.1:8080 or 18080 for testnet)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default settings
TESTNET_MODE=false
API_URL=""
NODE1_API="http://127.0.0.1:18080"
NODE2_API="http://127.0.0.1:18081"

# Load credentials from .env file
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/.env" ]; then
    source "$SCRIPT_DIR/.env"
fi

# Credentials (from .env or environment)
WALLET_PASSWORD="${WALLET_PASSWORD:?ERROR: Set WALLET_PASSWORD in scripts/.env (see scripts/.env.example)}"
NODE1_MNEMONIC="${MNEMONIC1:?ERROR: Set MNEMONIC1 in scripts/.env (see scripts/.env.example)}"
NODE2_MNEMONIC="${MNEMONIC2:?ERROR: Set MNEMONIC2 in scripts/.env (see scripts/.env.example)}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --testnet)
            TESTNET_MODE=true
            API_URL="$NODE1_API"
            shift
            ;;
        *)
            API_URL="$1"
            shift
            ;;
    esac
done

# Default API URL
if [ -z "$API_URL" ]; then
    API_URL="http://127.0.0.1:8080"
fi

# Cleanup function
cleanup() {
    if [ "$TESTNET_MODE" = true ]; then
        echo ""
        echo -e "${YELLOW}Cleaning up testnet nodes...${NC}"
        pkill -f "klingond.*--testnet" 2>/dev/null || true
        sleep 1
        echo -e "${GREEN}Cleanup complete${NC}"
    fi
}

# Set trap for cleanup on exit
trap cleanup EXIT INT TERM

# RPC helper
rpc() {
    local url="$1"
    local method="$2"
    local params="$3"
    if [ -z "$params" ]; then
        curl -s "$url" -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"id\":1}"
    else
        curl -s "$url" -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}"
    fi
}

# Start testnet nodes
start_testnet_nodes() {
    echo -e "${CYAN}Starting testnet nodes...${NC}"

    # Kill any existing nodes
    pkill -f "klingond.*--testnet" 2>/dev/null || true
    sleep 1

    # Build if needed
    if [ ! -f "./bin/klingond" ]; then
        echo "Building klingond..."
        make build
    fi

    # Clean old data
    rm -rf /tmp/klingon-node1 /tmp/klingon-node2

    # Start Node 1
    echo "Starting Node 1 on port 18080..."
    ./bin/klingond --testnet --data-dir /tmp/klingon-node1 --api 127.0.0.1:18080 --listen /ip4/0.0.0.0/tcp/14001 > /tmp/node1.log 2>&1 &
    sleep 2

    # Start Node 2
    echo "Starting Node 2 on port 18081..."
    ./bin/klingond --testnet --data-dir /tmp/klingon-node2 --api 127.0.0.1:18081 --listen /ip4/0.0.0.0/tcp/14002 > /tmp/node2.log 2>&1 &
    sleep 3

    echo -e "${GREEN}Nodes started${NC}"
}

# Restore wallets
restore_wallets() {
    echo -e "${CYAN}Restoring testnet wallets...${NC}"

    # Node 1 wallet
    echo "Restoring Node 1 wallet..."
    local result1=$(rpc "$NODE1_API" "wallet_create" "{\"mnemonic\":\"$NODE1_MNEMONIC\",\"password\":\"$WALLET_PASSWORD\"}")
    local error1=$(echo "$result1" | jq -r '.error.message // empty')
    if [ -n "$error1" ]; then
        rpc "$NODE1_API" "wallet_unlock" "{\"password\":\"$WALLET_PASSWORD\"}" > /dev/null
    fi

    # Node 2 wallet
    echo "Restoring Node 2 wallet..."
    local result2=$(rpc "$NODE2_API" "wallet_create" "{\"mnemonic\":\"$NODE2_MNEMONIC\",\"password\":\"$WALLET_PASSWORD\"}")
    local error2=$(echo "$result2" | jq -r '.error.message // empty')
    if [ -n "$error2" ]; then
        rpc "$NODE2_API" "wallet_unlock" "{\"password\":\"$WALLET_PASSWORD\"}" > /dev/null
    fi

    echo -e "${GREEN}Wallets restored${NC}"
}

# Check if wallet is unlocked
check_wallet() {
    local url="$1"
    local result=$(rpc "$url" "wallet_status")
    local unlocked=$(echo "$result" | jq -r '.result.unlocked // false')
    if [ "$unlocked" != "true" ]; then
        echo -e "${RED}Error: Wallet is not unlocked at $url${NC}"
        return 1
    fi
    return 0
}

# Print header
print_header() {
    echo -e "${CYAN}======================================${NC}"
    echo -e "${CYAN}  Klingon Wallet Address Report${NC}"
    echo -e "${CYAN}======================================${NC}"
    if [ "$TESTNET_MODE" = true ]; then
        echo -e "Mode: ${YELLOW}TESTNET${NC}"
    fi
    echo ""
}

# Get address for a chain
get_address() {
    local url="$1"
    local symbol="$2"
    local result=$(rpc "$url" "wallet_getAddress" "{\"symbol\":\"$symbol\"}")
    local address=$(echo "$result" | jq -r '.result.address // "N/A"')
    local path=$(echo "$result" | jq -r '.result.path // "N/A"')
    local error=$(echo "$result" | jq -r '.error.message // empty')

    if [ -n "$error" ]; then
        echo -e "  ${RED}Error: $error${NC}"
    else
        echo -e "  Address: ${GREEN}$address${NC}"
        echo -e "  Path:    ${YELLOW}$path${NC}"
    fi
}

# Get all address types for Bitcoin-family chains
get_all_addresses() {
    local url="$1"
    local symbol="$2"
    local result=$(rpc "$url" "wallet_getAllAddresses" "{\"symbol\":\"$symbol\"}")
    local error=$(echo "$result" | jq -r '.error.message // empty')

    if [ -n "$error" ]; then
        echo -e "  ${RED}Error: $error${NC}"
        return
    fi

    echo "$result" | jq -r '.result.addresses | to_entries[] | "  \(.key): \(.value)"' 2>/dev/null || echo "  (no addresses)"
}

# Check addresses for a node
check_node_addresses() {
    local url="$1"
    local name="$2"

    echo -e "${CYAN}======================================${NC}"
    echo -e "${CYAN}  $name${NC}"
    echo -e "${CYAN}  API: $url${NC}"
    echo -e "${CYAN}======================================${NC}"
    echo ""

    if ! check_wallet "$url"; then
        return
    fi

    # UTXO Chains
    echo -e "${YELLOW}=== UTXO Chains (Bitcoin-family) ===${NC}"
    echo ""

    for chain in BTC LTC DOGE; do
        echo -e "${BLUE}$chain:${NC}"
        get_address "$url" "$chain"
        echo ""
        echo "  All address types:"
        get_all_addresses "$url" "$chain"
        echo ""
    done

    # EVM Chains
    if [ "$TESTNET_MODE" = true ]; then
        echo -e "${YELLOW}=== EVM Chains (Testnet) ===${NC}"
    else
        echo -e "${YELLOW}=== EVM Chains ===${NC}"
    fi
    echo ""

    # Mainnet chain info
    declare -A mainnet_chains=(
        ["ETH"]="ETH (Ethereum, chainID: 1)"
        ["BSC"]="BNB (BNB Smart Chain, chainID: 56)"
        ["POLYGON"]="POL (Polygon, chainID: 137)"
        ["ARBITRUM"]="ETH (Arbitrum One, chainID: 42161)"
        ["OPTIMISM"]="ETH (Optimism, chainID: 10)"
        ["BASE"]="ETH (Base, chainID: 8453)"
        ["AVAX"]="AVAX (Avalanche C-Chain, chainID: 43114)"
    )

    # Testnet chain info
    declare -A testnet_chains=(
        ["ETH"]="ETH (Sepolia, chainID: 11155111)"
        ["BSC"]="BNB (BSC Testnet, chainID: 97)"
        ["POLYGON"]="POL (Polygon Amoy, chainID: 80002)"
        ["ARBITRUM"]="ETH (Arbitrum Sepolia, chainID: 421614)"
        ["OPTIMISM"]="ETH (Optimism Sepolia, chainID: 11155420)"
        ["BASE"]="ETH (Base Sepolia, chainID: 84532)"
        ["AVAX"]="AVAX (Avalanche Fuji, chainID: 43113)"
    )

    for chain in ETH BSC POLYGON ARBITRUM OPTIMISM BASE AVAX; do
        if [ "$TESTNET_MODE" = true ]; then
            echo -e "${BLUE}$chain - ${testnet_chains[$chain]}:${NC}"
        else
            echo -e "${BLUE}$chain - ${mainnet_chains[$chain]}:${NC}"
        fi
        get_address "$url" "$chain"
        echo ""
    done

    # Other Chains
    echo -e "${YELLOW}=== Other Chains ===${NC}"
    echo ""

    echo -e "${BLUE}SOL (Solana):${NC}"
    get_address "$url" "SOL"
    echo ""

    echo -e "${BLUE}XMR (Monero):${NC}"
    get_address "$url" "XMR"
    echo ""
}

# Main
print_header

if [ "$TESTNET_MODE" = true ]; then
    start_testnet_nodes
    restore_wallets
    echo ""

    # Check both nodes
    check_node_addresses "$NODE1_API" "Node 1 (Maker - BTC)"
    echo ""
    check_node_addresses "$NODE2_API" "Node 2 (Taker - LTC)"
else
    # Single node mode
    check_node_addresses "$API_URL" "Wallet"
fi

echo ""
echo -e "${GREEN}Address check complete!${NC}"
echo ""
echo "Press Enter to exit and cleanup..."
read -r
