#!/bin/bash
# Check balances for all supported chains and tokens
# Usage: ./scripts/check-balances.sh [--testnet] [API_URL]
#
# Options:
#   --testnet   Start testnet nodes, restore wallets, check balances, cleanup on exit
#   API_URL     API endpoint (default: http://127.0.0.1:8080 or 18080 for testnet)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
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

# Mainnet token contract addresses per chain
declare -a MAINNET_TOKENS=(
    # Ethereum (chainID 1)
    "ETH:USDT:0xdAC17F958D2ee523a2206206994597C13D831ec7:6"
    "ETH:USDC:0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48:6"
    "ETH:WETH:0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2:18"
    "ETH:WBTC:0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599:8"
    # Arbitrum (chainID 42161)
    "ARBITRUM:USDT:0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9:6"
    "ARBITRUM:USDC:0xaf88d065e77c8cC2239327C5EDb3A432268e5831:6"
    "ARBITRUM:WETH:0x82aF49447D8a07e3bd95BD0d56f35241523fBab1:18"
    # Optimism (chainID 10)
    "OPTIMISM:USDT:0x94b008aA00579c1307B0EF2c499aD98a8ce58e58:6"
    "OPTIMISM:USDC:0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85:6"
    # Base (chainID 8453)
    "BASE:USDC:0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913:6"
    # BSC (chainID 56)
    "BSC:USDT:0x55d398326f99059fF775485246999027B3197955:18"
    "BSC:USDC:0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d:18"
    # Polygon (chainID 137)
    "POLYGON:USDT:0xc2132D05D31c914a87C6611C10748AEb04B58e8F:6"
    "POLYGON:USDC:0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359:6"
    # Avalanche (chainID 43114)
    "AVAX:USDT:0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7:6"
    "AVAX:USDC:0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E:6"
)

# Testnet token contract addresses
declare -a TESTNET_TOKENS=(
    # Sepolia (chainID 11155111)
    "ETH:KGX:0x805a55da2ff4c72ecabd4e496dd8ce4ea923f25d:6"
)

# Use appropriate token list based on network mode
get_tokens() {
    if [ "$TESTNET_MODE" = true ]; then
        echo "${TESTNET_TOKENS[@]}"
    else
        echo "${MAINNET_TOKENS[@]}"
    fi
}

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

# Format amount with decimals
format_amount() {
    local amount="$1"
    local decimals="$2"

    if [ -z "$amount" ] || [ "$amount" = "null" ] || [ "$amount" = "0" ]; then
        echo "0"
        return
    fi

    if command -v bc &> /dev/null; then
        echo "scale=8; $amount / (10^$decimals)" | bc 2>/dev/null || echo "$amount"
    else
        echo "$amount (raw)"
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
        # Try unlock if already exists
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
    echo -e "${CYAN}  Klingon Wallet Balance Report${NC}"
    echo -e "${CYAN}======================================${NC}"
    if [ "$TESTNET_MODE" = true ]; then
        echo -e "Mode: ${YELLOW}TESTNET${NC}"
    fi
    echo ""
}

# Get UTXO chain balance (aggregated)
get_utxo_balance() {
    local url="$1"
    local symbol="$2"
    local decimals="$3"

    local result=$(rpc "$url" "wallet_getAggregatedBalance" "{\"symbol\":\"$symbol\"}")
    local error=$(echo "$result" | jq -r '.error.message // empty')

    if [ -n "$error" ]; then
        echo -e "  ${RED}Error: $error${NC}"
        return
    fi

    local confirmed=$(echo "$result" | jq -r '.result.confirmed // 0')
    local unconfirmed=$(echo "$result" | jq -r '.result.unconfirmed // 0')
    local total=$(echo "$result" | jq -r '.result.total // 0')

    local formatted=$(format_amount "$total" "$decimals")
    echo -e "  Balance: ${GREEN}$formatted $symbol${NC} (${total} satoshis)"

    if [ "$unconfirmed" != "0" ]; then
        local unconf_formatted=$(format_amount "$unconfirmed" "$decimals")
        echo -e "  Unconfirmed: ${YELLOW}$unconf_formatted $symbol${NC}"
    fi
}

# Get EVM native balance
get_evm_balance() {
    local url="$1"
    local symbol="$2"
    local native_token="$3"

    local addr_result=$(rpc "$url" "wallet_getAddress" "{\"symbol\":\"$symbol\"}")
    local address=$(echo "$addr_result" | jq -r '.result.address // empty')

    if [ -z "$address" ]; then
        echo -e "  ${RED}Could not get address${NC}"
        return
    fi

    local result=$(rpc "$url" "wallet_getBalance" "{\"symbol\":\"$symbol\",\"address\":\"$address\"}")
    local error=$(echo "$result" | jq -r '.error.message // empty')

    if [ -n "$error" ]; then
        echo -e "  ${RED}Error: $error${NC}"
        return
    fi

    local balance=$(echo "$result" | jq -r '.result.balance // .result.total // .result.confirmed // 0')
    local formatted=$(format_amount "$balance" "18")
    echo -e "  Address: ${BLUE}$address${NC}"
    echo -e "  Balance: ${GREEN}$formatted $native_token${NC}"
}

# Get ERC-20 token balance
get_erc20_balance() {
    local url="$1"
    local chain="$2"
    local token="$3"
    local contract="$4"
    local decimals="$5"

    local addr_result=$(rpc "$url" "wallet_getAddress" "{\"symbol\":\"$chain\"}")
    local address=$(echo "$addr_result" | jq -r '.result.address // empty')

    if [ -z "$address" ]; then
        return
    fi

    local result=$(rpc "$url" "wallet_getERC20Balance" "{\"symbol\":\"$chain\",\"token\":\"$contract\",\"address\":\"$address\"}")
    local error=$(echo "$result" | jq -r '.error.message // empty')

    if [ -n "$error" ]; then
        echo -e "    ${token}: ${RED}Error - $error${NC}"
        return
    fi

    local balance=$(echo "$result" | jq -r '.result.balance // "0"')
    if [ "$balance" != "0" ] && [ "$balance" != "null" ]; then
        local formatted=$(format_amount "$balance" "$decimals")
        echo -e "    ${token}: ${GREEN}$formatted${NC}"
    else
        echo -e "    ${token}: ${YELLOW}0${NC}"
    fi
}

# Check balances for a node
check_node_balances() {
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
    echo -e "${YELLOW}=== UTXO Chains ===${NC}"
    echo ""

    echo -e "${BLUE}BTC (Bitcoin):${NC}"
    get_utxo_balance "$url" "BTC" "8"
    echo ""

    echo -e "${BLUE}LTC (Litecoin):${NC}"
    get_utxo_balance "$url" "LTC" "8"
    echo ""

    echo -e "${BLUE}DOGE (Dogecoin):${NC}"
    get_utxo_balance "$url" "DOGE" "8"
    echo ""

    # EVM Chains
    if [ "$TESTNET_MODE" = true ]; then
        echo -e "${YELLOW}=== EVM Chains (Testnet) ===${NC}"
    else
        echo -e "${YELLOW}=== EVM Chains ===${NC}"
    fi
    echo ""

    declare -A native_tokens=(
        ["ETH"]="ETH"
        ["BSC"]="BNB"
        ["POLYGON"]="POL"
        ["ARBITRUM"]="ETH"
        ["OPTIMISM"]="ETH"
        ["BASE"]="ETH"
        ["AVAX"]="AVAX"
    )

    # Mainnet chain names
    declare -A mainnet_names=(
        ["ETH"]="Ethereum (chainID: 1)"
        ["BSC"]="BNB Smart Chain (chainID: 56)"
        ["POLYGON"]="Polygon (chainID: 137)"
        ["ARBITRUM"]="Arbitrum One (chainID: 42161)"
        ["OPTIMISM"]="Optimism (chainID: 10)"
        ["BASE"]="Base (chainID: 8453)"
        ["AVAX"]="Avalanche C-Chain (chainID: 43114)"
    )

    # Testnet chain names
    declare -A testnet_names=(
        ["ETH"]="Sepolia (chainID: 11155111)"
        ["BSC"]="BSC Testnet (chainID: 97)"
        ["POLYGON"]="Polygon Amoy (chainID: 80002)"
        ["ARBITRUM"]="Arbitrum Sepolia (chainID: 421614)"
        ["OPTIMISM"]="Optimism Sepolia (chainID: 11155420)"
        ["BASE"]="Base Sepolia (chainID: 84532)"
        ["AVAX"]="Avalanche Fuji (chainID: 43113)"
    )

    # Get the appropriate token list
    local tokens
    if [ "$TESTNET_MODE" = true ]; then
        tokens=("${TESTNET_TOKENS[@]}")
    else
        tokens=("${MAINNET_TOKENS[@]}")
    fi

    for chain in ETH BSC POLYGON ARBITRUM OPTIMISM BASE AVAX; do
        if [ "$TESTNET_MODE" = true ]; then
            echo -e "${BLUE}$chain - ${testnet_names[$chain]}:${NC}"
        else
            echo -e "${BLUE}$chain - ${mainnet_names[$chain]}:${NC}"
        fi
        get_evm_balance "$url" "$chain" "${native_tokens[$chain]}"

        # Show tokens for this chain
        echo -e "  ${MAGENTA}Tokens:${NC}"
        local has_tokens=false
        for token_info in "${tokens[@]}"; do
            IFS=':' read -r t_chain t_symbol t_contract t_decimals <<< "$token_info"
            if [ "$t_chain" = "$chain" ]; then
                has_tokens=true
                get_erc20_balance "$url" "$chain" "$t_symbol" "$t_contract" "$t_decimals"
            fi
        done
        if [ "$has_tokens" = false ]; then
            echo -e "    ${YELLOW}(no tokens configured)${NC}"
        fi
        echo ""
    done
}

# Main
print_header

if [ "$TESTNET_MODE" = true ]; then
    start_testnet_nodes
    restore_wallets
    echo ""

    # Check both nodes
    check_node_balances "$NODE1_API" "Node 1 (Maker - BTC)"
    echo ""
    check_node_balances "$NODE2_API" "Node 2 (Taker - LTC)"
else
    # Single node mode
    check_node_balances "$API_URL" "Wallet"
fi

echo ""
echo -e "${GREEN}Balance check complete!${NC}"
echo ""
echo "Press Enter to exit and cleanup..."
read -r
