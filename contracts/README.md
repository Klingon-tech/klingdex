# Klingon HTLC Smart Contracts

Hash Time-Locked Contracts for atomic swaps on EVM chains.

## Features

- Native token swaps (ETH, BNB, MATIC, etc.)
- ERC20 token swaps
- Cross-chain compatible (same SHA256 hash as Bitcoin HTLCs)
- DAO fee collection (0.2% default)
- Reentrancy protection
- SafeERC20 for non-standard tokens
- Pause functionality for emergencies

## Requirements

- [Foundry](https://book.getfoundry.sh/getting-started/installation)

## Setup

```bash
# Install Foundry (if not already installed)
curl -L https://foundry.paradigm.xyz | bash
foundryup

# Install dependencies
cd contracts
forge install OpenZeppelin/openzeppelin-contracts --no-commit

# Build
forge build

# Test
forge test

# Test with verbosity
forge test -vvv

# Gas report
forge test --gas-report
```

## Project Structure

```
contracts/
├── src/
│   └── KlingonHTLC.sol       # Main HTLC contract
├── test/
│   └── KlingonHTLC.t.sol     # Comprehensive tests
├── script/
│   └── Deploy.s.sol          # Deployment scripts
├── foundry.toml              # Foundry configuration
└── README.md                 # This file
```

## Deployment

### Local Testing

```bash
# Start local Anvil node
anvil

# Deploy locally
forge script script/Deploy.s.sol:DeployKlingonHTLC --rpc-url http://localhost:8545 --broadcast
```

### Testnet Deployment

```bash
# Set environment variables
export PRIVATE_KEY=your_private_key_here
export SEPOLIA_RPC_URL=https://eth-sepolia.g.alchemy.com/v2/YOUR_API_KEY

# Deploy to Sepolia
forge script script/Deploy.s.sol:DeployKlingonHTLC \
    --rpc-url $SEPOLIA_RPC_URL \
    --broadcast \
    --verify

# Deploy to BSC Testnet
export BSC_TESTNET_RPC_URL=https://data-seed-prebsc-1-s1.binance.org:8545
forge script script/Deploy.s.sol:DeployKlingonHTLC \
    --rpc-url $BSC_TESTNET_RPC_URL \
    --broadcast
```

### Mainnet Deployment

**WARNING**: Update DAO addresses in `Deploy.s.sol` before mainnet deployment!

```bash
# Set mainnet RPC
export MAINNET_RPC_URL=https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY

# Deploy with --slow for safety
forge script script/Deploy.s.sol:DeployKlingonHTLC \
    --rpc-url $MAINNET_RPC_URL \
    --broadcast \
    --slow \
    --verify
```

## Contract Interface

### Create Swap (Native Token)

```solidity
function createSwapNative(
    bytes32 swapId,      // Unique swap identifier
    address receiver,     // Who can claim with secret
    bytes32 secretHash,   // SHA256(secret)
    uint256 timelock      // Unix timestamp for refund
) external payable;
```

### Create Swap (ERC20)

```solidity
function createSwapERC20(
    bytes32 swapId,
    address receiver,
    address token,        // ERC20 token address
    uint256 amount,
    bytes32 secretHash,
    uint256 timelock
) external;
```

### Claim

```solidity
function claim(
    bytes32 swapId,
    bytes32 secret        // The preimage that hashes to secretHash
) external;
```

### Refund

```solidity
function refund(bytes32 swapId) external;
```

## Usage Example

### Creating a Swap (JavaScript/ethers.js)

```javascript
const { ethers } = require("ethers");

// Generate secret (32 bytes)
const secret = ethers.utils.randomBytes(32);
const secretHash = ethers.utils.sha256(secret);

// Create swap ID
const swapId = ethers.utils.keccak256(
  ethers.utils.solidityPack(
    ["address", "address", "uint256"],
    [sender, receiver, Date.now()]
  )
);

// Timelock: 12 hours from now
const timelock = Math.floor(Date.now() / 1000) + 12 * 60 * 60;

// Create swap
await htlc.createSwapNative(swapId, receiver, secretHash, timelock, {
  value: ethers.utils.parseEther("1.0")
});

// Store secret securely!
console.log("Secret:", ethers.utils.hexlify(secret));
```

### Claiming a Swap

```javascript
// Receiver claims with secret
await htlc.claim(swapId, secret);
```

### Monitoring for Secret (Cross-chain)

```javascript
// Watch for SwapClaimed events to learn the secret
htlc.on("SwapClaimed", (swapId, receiver, secret) => {
  console.log("Secret revealed:", secret);
  // Use this secret to claim on the other chain!
});
```

## Cross-Chain Swap Flow

### ETH <-> BTC Example

1. **Alice** (has ETH, wants BTC) generates secret
2. **Alice** creates HTLC on Ethereum (24h timelock)
3. **Bob** (has BTC, wants ETH) creates HTLC on Bitcoin (12h timelock)
4. **Alice** claims Bitcoin (reveals secret on Bitcoin blockchain)
5. **Bob** extracts secret from Bitcoin transaction
6. **Bob** claims Ethereum using the secret

The Go backend monitors both chains and coordinates the swap.

## Security

### Audited Features

- ReentrancyGuard on all state-changing functions
- Check-Effects-Interactions pattern
- SafeERC20 for token transfers
- Minimum/maximum timelock enforcement
- Amount validation
- Proper access control

### Pre-Production Checklist

- [ ] Professional security audit
- [ ] Testnet deployment and testing
- [ ] Mainnet DAO addresses configured
- [ ] Owner key secured (hardware wallet recommended)
- [ ] Monitoring set up for events

## Gas Costs

| Operation | Estimated Gas |
|-----------|---------------|
| createSwapNative | ~80,000 |
| createSwapERC20 | ~100,000 |
| claim (native) | ~50,000 |
| claim (ERC20) | ~65,000 |
| refund (native) | ~45,000 |
| refund (ERC20) | ~60,000 |

## License

MIT
