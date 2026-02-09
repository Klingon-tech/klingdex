# EVM HTLC Smart Contract Design

## Overview

This document outlines the design for HTLC (Hash Time-Locked Contract) smart contracts that enable atomic swaps on EVM chains. The contracts support:

1. **Native tokens** (ETH, BNB, MATIC, AVAX, etc.)
2. **ERC20 tokens** (USDC, USDT, DAI, etc.)
3. **Same-chain swaps** (e.g., ETH ↔ USDC on Ethereum)
4. **Cross-EVM chain swaps** (e.g., ETH on Ethereum ↔ BNB on BSC)
5. **Cross-chain to Bitcoin** (e.g., ETH ↔ BTC, USDC ↔ LTC)

## Architecture Decision

### Single Contract vs Factory Pattern

| Approach | Pros | Cons |
|----------|------|------|
| **Single Contract** | One address per chain, lower gas for users, simpler integration | Shared state, slightly larger contract |
| **Factory Pattern** | Isolated per swap, cleaner auditing | Higher gas (deploy per swap), multiple addresses |

**Decision: Single Contract** - One deployment per chain with all swaps managed in a mapping. This is gas-efficient and easier to integrate with the Go backend.

## Contract Structure

```
contracts/
├── KlingonHTLC.sol           # Main HTLC contract
├── interfaces/
│   └── IKlingonHTLC.sol      # Interface for external integrations
├── libraries/
│   └── SwapLib.sol           # Shared logic (optional)
└── test/
    └── KlingonHTLC.t.sol     # Foundry tests
```

## Data Structures

```solidity
enum SwapState {
    Empty,      // 0 - Swap doesn't exist
    Active,     // 1 - Funds locked, awaiting claim or refund
    Claimed,    // 2 - Receiver claimed with secret
    Refunded    // 3 - Sender refunded after timeout
}

struct Swap {
    address sender;           // Who locked the funds
    address receiver;         // Who can claim with secret
    address token;            // Token address (address(0) for native)
    uint256 amount;           // Gross amount locked
    uint256 daoFee;           // Fee to DAO (deducted on claim)
    bytes32 secretHash;       // SHA256(secret)
    uint256 timelock;         // Unix timestamp when refund becomes available
    SwapState state;          // Current state
}
```

## Swap ID Generation

The swap ID should be deterministic to allow both parties to compute it independently:

```solidity
bytes32 swapId = keccak256(abi.encodePacked(
    sender,
    receiver,
    token,
    amount,
    secretHash,
    timelock,
    block.chainid,    // Prevents cross-chain replay
    nonce             // User-provided nonce for uniqueness
));
```

This allows:
- Both parties to verify they're looking at the same swap
- Prevention of replay attacks across chains
- Coordination in cross-chain scenarios

## Security Features

### 1. Reentrancy Protection
```solidity
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

contract KlingonHTLC is ReentrancyGuard {
    function claim(...) external nonReentrant { ... }
    function refund(...) external nonReentrant { ... }
}
```

### 2. Safe ERC20 Transfers
```solidity
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

using SafeERC20 for IERC20;

// Handles non-standard tokens (USDT, etc.)
IERC20(token).safeTransferFrom(sender, address(this), amount);
IERC20(token).safeTransfer(receiver, amount);
```

### 3. Check-Effects-Interactions Pattern
```solidity
function claim(bytes32 swapId, bytes32 secret) external nonReentrant {
    Swap storage swap = swaps[swapId];

    // CHECKS
    require(swap.state == SwapState.Active, "Not active");
    require(swap.receiver == msg.sender, "Not receiver");
    require(sha256(abi.encodePacked(secret)) == swap.secretHash, "Bad secret");

    // EFFECTS (state change BEFORE external call)
    swap.state = SwapState.Claimed;

    // INTERACTIONS (external call LAST)
    _transfer(swap.token, swap.receiver, swap.amount - swap.daoFee);
    _transfer(swap.token, daoAddress, swap.daoFee);

    emit SwapClaimed(swapId, secret);
}
```

### 4. Minimum Timelock Enforcement
```solidity
uint256 public constant MIN_TIMELOCK = 1 hours;
uint256 public constant MAX_TIMELOCK = 30 days;

require(timelock >= block.timestamp + MIN_TIMELOCK, "Timelock too short");
require(timelock <= block.timestamp + MAX_TIMELOCK, "Timelock too long");
```

### 5. Amount Validation
```solidity
uint256 public constant MIN_SWAP_AMOUNT = 1000; // Prevent dust attacks

require(amount >= MIN_SWAP_AMOUNT, "Amount too small");
```

## Fee Structure Integration

The Klingon exchange uses 0.2% maker + 0.2% taker fees (0.4% total).

### Option A: Fee on Claim (Recommended)
- Fee deducted when receiver claims
- Cleaner UX for sender (locks exact amount)
- DAO receives fee atomically

```solidity
function createSwap(...) external payable {
    // Sender locks full amount
    uint256 daoFee = (amount * FEE_BPS) / 10000; // 20 bps = 0.2%

    swaps[swapId] = Swap({
        ...
        amount: amount,
        daoFee: daoFee,
        ...
    });
}

function claim(...) external {
    // Receiver gets amount - fee
    uint256 receiverAmount = swap.amount - swap.daoFee;
    _transfer(swap.token, swap.receiver, receiverAmount);
    _transfer(swap.token, daoAddress, swap.daoFee);
}

function refund(...) external {
    // Sender gets full amount back (no fee on refund)
    _transfer(swap.token, swap.sender, swap.amount);
}
```

### Option B: Configurable Fee (More Flexible)
```solidity
address public daoAddress;
uint256 public feeBps = 20; // 0.2%

function setDaoAddress(address _dao) external onlyOwner { ... }
function setFeeBps(uint256 _feeBps) external onlyOwner { ... }
```

## Cross-Chain Considerations

### Same Hash Function
Both Bitcoin HTLCs and EVM HTLCs use **SHA256**:

```solidity
// EVM
require(sha256(abi.encodePacked(secret)) == swap.secretHash, "Invalid secret");
```

```go
// Bitcoin (Go)
hash := sha256.Sum256(secret)
```

### Timelock Coordination

For BTC ↔ ETH swap:

| Party | Chain | Role | Timelock |
|-------|-------|------|----------|
| Alice | BTC | Initiator/Sender | 24 hours |
| Alice | ETH | Responder/Receiver | - |
| Bob | ETH | Responder/Sender | 12 hours |
| Bob | BTC | Initiator/Receiver | - |

**Critical**: The initiator's timelock must be LONGER than the responder's to prevent the initiator from claiming on one chain and refunding on the other.

```
Alice's BTC timelock: 24 hours
Bob's ETH timelock:   12 hours

Timeline:
0h   - Both parties lock funds
0-12h - Alice can claim ETH (reveals secret)
        Bob can then claim BTC (using revealed secret)
12-24h - Bob can refund ETH if Alice didn't claim
24h+  - Alice can refund BTC if swap failed
```

### Event Monitoring

Events are crucial for cross-chain coordination:

```solidity
event SwapCreated(
    bytes32 indexed swapId,
    address indexed sender,
    address indexed receiver,
    address token,
    uint256 amount,
    bytes32 secretHash,
    uint256 timelock,
    uint256 chainId
);

event SwapClaimed(
    bytes32 indexed swapId,
    address indexed receiver,
    bytes32 secret          // SECRET IS REVEALED HERE!
);

event SwapRefunded(
    bytes32 indexed swapId,
    address indexed sender
);
```

The Go backend monitors `SwapClaimed` events to learn the secret for claiming on the other chain.

## Complete Contract Implementation

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

/**
 * @title KlingonHTLC
 * @notice Hash Time-Locked Contract for atomic swaps
 * @dev Supports native tokens (ETH/BNB) and ERC20 tokens
 */
contract KlingonHTLC is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    // ============ Constants ============

    uint256 public constant MIN_TIMELOCK = 1 hours;
    uint256 public constant MAX_TIMELOCK = 30 days;
    uint256 public constant MIN_SWAP_AMOUNT = 1000; // Prevent dust
    uint256 public constant FEE_DENOMINATOR = 10000;

    // ============ State Variables ============

    enum SwapState { Empty, Active, Claimed, Refunded }

    struct Swap {
        address sender;
        address receiver;
        address token;        // address(0) for native
        uint256 amount;
        uint256 daoFee;
        bytes32 secretHash;
        uint256 timelock;
        SwapState state;
    }

    mapping(bytes32 => Swap) public swaps;

    address public daoAddress;
    uint256 public feeBps = 20; // 0.2% default
    bool public paused;

    // ============ Events ============

    event SwapCreated(
        bytes32 indexed swapId,
        address indexed sender,
        address indexed receiver,
        address token,
        uint256 amount,
        uint256 daoFee,
        bytes32 secretHash,
        uint256 timelock
    );

    event SwapClaimed(
        bytes32 indexed swapId,
        address indexed receiver,
        bytes32 secret
    );

    event SwapRefunded(
        bytes32 indexed swapId,
        address indexed sender
    );

    event DaoAddressUpdated(address indexed oldDao, address indexed newDao);
    event FeeBpsUpdated(uint256 oldFee, uint256 newFee);
    event Paused(bool isPaused);

    // ============ Errors ============

    error SwapAlreadyExists();
    error SwapNotActive();
    error InvalidReceiver();
    error InvalidAmount();
    error InvalidTimelock();
    error InvalidSecret();
    error NotReceiver();
    error NotSender();
    error TimelockNotExpired();
    error TransferFailed();
    error ContractPaused();
    error InvalidDaoAddress();

    // ============ Constructor ============

    constructor(address _daoAddress) Ownable(msg.sender) {
        if (_daoAddress == address(0)) revert InvalidDaoAddress();
        daoAddress = _daoAddress;
    }

    // ============ Modifiers ============

    modifier whenNotPaused() {
        if (paused) revert ContractPaused();
        _;
    }

    // ============ External Functions ============

    /**
     * @notice Create a swap with native token (ETH/BNB/etc)
     * @param swapId Unique identifier for the swap
     * @param receiver Address that can claim with secret
     * @param secretHash SHA256 hash of the secret
     * @param timelock Unix timestamp when refund becomes available
     */
    function createSwapNative(
        bytes32 swapId,
        address receiver,
        bytes32 secretHash,
        uint256 timelock
    ) external payable nonReentrant whenNotPaused {
        _validateSwapParams(swapId, receiver, msg.value, timelock);

        uint256 daoFee = (msg.value * feeBps) / FEE_DENOMINATOR;

        swaps[swapId] = Swap({
            sender: msg.sender,
            receiver: receiver,
            token: address(0),
            amount: msg.value,
            daoFee: daoFee,
            secretHash: secretHash,
            timelock: timelock,
            state: SwapState.Active
        });

        emit SwapCreated(
            swapId,
            msg.sender,
            receiver,
            address(0),
            msg.value,
            daoFee,
            secretHash,
            timelock
        );
    }

    /**
     * @notice Create a swap with ERC20 token
     * @param swapId Unique identifier for the swap
     * @param receiver Address that can claim with secret
     * @param token ERC20 token address
     * @param amount Amount of tokens to lock
     * @param secretHash SHA256 hash of the secret
     * @param timelock Unix timestamp when refund becomes available
     */
    function createSwapERC20(
        bytes32 swapId,
        address receiver,
        address token,
        uint256 amount,
        bytes32 secretHash,
        uint256 timelock
    ) external nonReentrant whenNotPaused {
        if (token == address(0)) revert InvalidAmount();
        _validateSwapParams(swapId, receiver, amount, timelock);

        // Transfer tokens to contract
        IERC20(token).safeTransferFrom(msg.sender, address(this), amount);

        uint256 daoFee = (amount * feeBps) / FEE_DENOMINATOR;

        swaps[swapId] = Swap({
            sender: msg.sender,
            receiver: receiver,
            token: token,
            amount: amount,
            daoFee: daoFee,
            secretHash: secretHash,
            timelock: timelock,
            state: SwapState.Active
        });

        emit SwapCreated(
            swapId,
            msg.sender,
            receiver,
            token,
            amount,
            daoFee,
            secretHash,
            timelock
        );
    }

    /**
     * @notice Claim swap funds by revealing the secret
     * @param swapId The swap identifier
     * @param secret The 32-byte secret that hashes to secretHash
     */
    function claim(bytes32 swapId, bytes32 secret) external nonReentrant {
        Swap storage swap = swaps[swapId];

        // Checks
        if (swap.state != SwapState.Active) revert SwapNotActive();
        if (swap.receiver != msg.sender) revert NotReceiver();
        if (sha256(abi.encodePacked(secret)) != swap.secretHash) revert InvalidSecret();

        // Effects
        swap.state = SwapState.Claimed;

        // Interactions
        uint256 receiverAmount = swap.amount - swap.daoFee;

        if (swap.token == address(0)) {
            // Native token
            (bool success, ) = swap.receiver.call{value: receiverAmount}("");
            if (!success) revert TransferFailed();

            if (swap.daoFee > 0) {
                (success, ) = daoAddress.call{value: swap.daoFee}("");
                if (!success) revert TransferFailed();
            }
        } else {
            // ERC20 token
            IERC20(swap.token).safeTransfer(swap.receiver, receiverAmount);
            if (swap.daoFee > 0) {
                IERC20(swap.token).safeTransfer(daoAddress, swap.daoFee);
            }
        }

        emit SwapClaimed(swapId, msg.sender, secret);
    }

    /**
     * @notice Refund swap funds after timelock expires
     * @param swapId The swap identifier
     */
    function refund(bytes32 swapId) external nonReentrant {
        Swap storage swap = swaps[swapId];

        // Checks
        if (swap.state != SwapState.Active) revert SwapNotActive();
        if (swap.sender != msg.sender) revert NotSender();
        if (block.timestamp < swap.timelock) revert TimelockNotExpired();

        // Effects
        swap.state = SwapState.Refunded;

        // Interactions - full amount returned (no fee on refund)
        if (swap.token == address(0)) {
            (bool success, ) = swap.sender.call{value: swap.amount}("");
            if (!success) revert TransferFailed();
        } else {
            IERC20(swap.token).safeTransfer(swap.sender, swap.amount);
        }

        emit SwapRefunded(swapId, msg.sender);
    }

    // ============ View Functions ============

    /**
     * @notice Get swap details
     * @param swapId The swap identifier
     * @return The Swap struct
     */
    function getSwap(bytes32 swapId) external view returns (Swap memory) {
        return swaps[swapId];
    }

    /**
     * @notice Check if a secret is valid for a swap
     * @param swapId The swap identifier
     * @param secret The secret to verify
     * @return True if the secret matches
     */
    function verifySecret(bytes32 swapId, bytes32 secret) external view returns (bool) {
        return sha256(abi.encodePacked(secret)) == swaps[swapId].secretHash;
    }

    /**
     * @notice Check if a swap can be refunded
     * @param swapId The swap identifier
     * @return True if refund is available
     */
    function canRefund(bytes32 swapId) external view returns (bool) {
        Swap storage swap = swaps[swapId];
        return swap.state == SwapState.Active && block.timestamp >= swap.timelock;
    }

    /**
     * @notice Calculate swap ID from parameters
     * @dev Useful for coordinating cross-chain swaps
     */
    function computeSwapId(
        address sender,
        address receiver,
        address token,
        uint256 amount,
        bytes32 secretHash,
        uint256 timelock,
        uint256 nonce
    ) external view returns (bytes32) {
        return keccak256(abi.encodePacked(
            sender,
            receiver,
            token,
            amount,
            secretHash,
            timelock,
            block.chainid,
            nonce
        ));
    }

    // ============ Admin Functions ============

    function setDaoAddress(address _daoAddress) external onlyOwner {
        if (_daoAddress == address(0)) revert InvalidDaoAddress();
        emit DaoAddressUpdated(daoAddress, _daoAddress);
        daoAddress = _daoAddress;
    }

    function setFeeBps(uint256 _feeBps) external onlyOwner {
        require(_feeBps <= 500, "Fee too high"); // Max 5%
        emit FeeBpsUpdated(feeBps, _feeBps);
        feeBps = _feeBps;
    }

    function setPaused(bool _paused) external onlyOwner {
        paused = _paused;
        emit Paused(_paused);
    }

    // ============ Internal Functions ============

    function _validateSwapParams(
        bytes32 swapId,
        address receiver,
        uint256 amount,
        uint256 timelock
    ) internal view {
        if (swaps[swapId].state != SwapState.Empty) revert SwapAlreadyExists();
        if (receiver == address(0)) revert InvalidReceiver();
        if (amount < MIN_SWAP_AMOUNT) revert InvalidAmount();
        if (timelock < block.timestamp + MIN_TIMELOCK) revert InvalidTimelock();
        if (timelock > block.timestamp + MAX_TIMELOCK) revert InvalidTimelock();
    }
}
```

## Go Integration

### ABI Binding Generation

```bash
# Generate Go bindings
abigen --sol contracts/KlingonHTLC.sol \
       --pkg htlc \
       --out internal/contracts/htlc/htlc.go
```

### Go Interface

```go
// internal/contracts/htlc/interface.go
package htlc

import (
    "context"
    "math/big"

    "github.com/ethereum/go-ethereum/common"
)

type HTLCClient interface {
    // Create a native token swap (ETH/BNB)
    CreateSwapNative(
        ctx context.Context,
        swapID [32]byte,
        receiver common.Address,
        secretHash [32]byte,
        timelock *big.Int,
        amount *big.Int,
    ) (common.Hash, error)

    // Create an ERC20 token swap
    CreateSwapERC20(
        ctx context.Context,
        swapID [32]byte,
        receiver common.Address,
        token common.Address,
        amount *big.Int,
        secretHash [32]byte,
        timelock *big.Int,
    ) (common.Hash, error)

    // Claim with secret
    Claim(ctx context.Context, swapID [32]byte, secret [32]byte) (common.Hash, error)

    // Refund after timeout
    Refund(ctx context.Context, swapID [32]byte) (common.Hash, error)

    // Get swap details
    GetSwap(ctx context.Context, swapID [32]byte) (*Swap, error)

    // Watch for SwapClaimed events (to learn secret)
    WatchClaimed(ctx context.Context, swapID [32]byte) (<-chan ClaimedEvent, error)
}
```

### Event Monitoring for Secret Learning

```go
// internal/contracts/htlc/monitor.go

// WatchForSecret monitors the chain for a claim event to learn the secret
func (c *Client) WatchForSecret(ctx context.Context, swapID [32]byte) ([]byte, error) {
    // Create filter for SwapClaimed events with this swapID
    filter := c.contract.WatchSwapClaimed(nil, [][32]byte{swapID}, nil)

    for {
        select {
        case event := <-filter.C:
            // Secret is revealed in the event!
            return event.Secret[:], nil
        case err := <-filter.Err():
            return nil, err
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

## Deployment Addresses

| Chain | Network | Contract Address |
|-------|---------|------------------|
| Ethereum | Mainnet | TBD |
| Ethereum | Sepolia | TBD |
| BSC | Mainnet | TBD |
| BSC | Testnet | TBD |
| Polygon | Mainnet | TBD |
| Polygon | Mumbai | TBD |
| Arbitrum | Mainnet | TBD |
| Arbitrum | Sepolia | TBD |

## Swap Flow Examples

### Example 1: Same-Chain (ETH ↔ USDC on Ethereum)

```
1. Alice wants to swap 1 ETH for 2000 USDC
2. Bob wants to swap 2000 USDC for 1 ETH

Flow:
- Alice generates secret, computes secretHash
- Alice calls createSwapNative(swapId_A, Bob, secretHash, timelock_12h) with 1 ETH
- Bob calls createSwapERC20(swapId_B, Alice, USDC, 2000e6, secretHash, timelock_6h)
- Alice calls claim(swapId_B, secret) to get USDC (reveals secret)
- Bob sees SwapClaimed event, extracts secret
- Bob calls claim(swapId_A, secret) to get ETH
```

### Example 2: Cross-EVM (ETH on Ethereum ↔ BNB on BSC)

```
1. Alice has ETH on Ethereum, wants BNB on BSC
2. Bob has BNB on BSC, wants ETH on Ethereum

Flow:
- Alice generates secret, computes secretHash
- Alice calls Ethereum.createSwapNative(swapId, Bob, secretHash, timelock_24h) with 1 ETH
- Bob calls BSC.createSwapNative(swapId, Alice, secretHash, timelock_12h) with 3 BNB
- Alice calls BSC.claim(swapId, secret) to get BNB (reveals secret on BSC)
- Bob monitors BSC for SwapClaimed event, extracts secret
- Bob calls Ethereum.claim(swapId, secret) to get ETH
```

### Example 3: Cross-Chain (ETH ↔ BTC)

```
1. Alice has ETH, wants BTC
2. Bob has BTC, wants ETH

Flow:
- Alice generates secret, computes secretHash
- Alice creates HTLC on Ethereum (24h timelock)
- Bob creates HTLC on Bitcoin (12h timelock)
- Alice claims Bitcoin HTLC (reveals secret on Bitcoin blockchain)
- Bob extracts secret from Bitcoin transaction witness
- Bob claims Ethereum HTLC using the secret

Go backend coordinates:
- Watches Bitcoin for claim tx, extracts secret from witness
- Uses secret to claim on EVM side
```

## Security Checklist

- [x] ReentrancyGuard on all state-changing functions
- [x] SafeERC20 for token transfers
- [x] Check-Effects-Interactions pattern
- [x] Minimum timelock enforcement
- [x] Maximum timelock enforcement
- [x] Minimum amount enforcement (dust prevention)
- [x] Zero-address validation
- [x] State validation before operations
- [x] Proper event emission
- [x] No external calls in loops
- [x] No unbounded iterations
- [x] Owner functions limited to non-critical parameters
- [x] Pause functionality for emergencies
- [ ] Professional audit before mainnet

## Gas Estimates

| Operation | Estimated Gas |
|-----------|---------------|
| createSwapNative | ~80,000 |
| createSwapERC20 | ~100,000 (includes approve) |
| claim (native) | ~50,000 |
| claim (ERC20) | ~65,000 |
| refund (native) | ~45,000 |
| refund (ERC20) | ~60,000 |

## Testing Requirements

1. **Unit Tests**
   - Create swap with valid params
   - Reject swap with invalid params
   - Claim with correct secret
   - Reject claim with wrong secret
   - Refund after timelock
   - Reject refund before timelock
   - Fee calculation correctness
   - Reentrancy attack prevention

2. **Integration Tests**
   - Full swap flow (native)
   - Full swap flow (ERC20)
   - Non-standard ERC20 tokens (USDT)
   - Cross-chain simulation

3. **Fuzz Tests**
   - Random amounts
   - Random timelocks
   - Random secrets

## Next Steps

1. [ ] Set up Foundry project structure
2. [ ] Write contract tests
3. [ ] Deploy to testnets (Sepolia, BSC Testnet)
4. [ ] Generate Go bindings with abigen
5. [ ] Implement Go client wrapper
6. [ ] Add EVM swap support to coordinator
7. [ ] Integration testing with Bitcoin HTLCs
8. [ ] Security audit
9. [ ] Mainnet deployment
