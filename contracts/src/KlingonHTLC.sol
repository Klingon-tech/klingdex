// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

/**
 * @title KlingonHTLC
 * @author Klingon Exchange
 * @notice Hash Time-Locked Contract for atomic swaps
 * @dev Supports native tokens (ETH/BNB/MATIC) and ERC20 tokens
 *      Used for same-chain, cross-EVM, and cross-chain (Bitcoin) swaps
 */
contract KlingonHTLC is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    // ============ Constants ============

    /// @notice Minimum timelock duration (1 hour)
    uint256 public constant MIN_TIMELOCK = 1 hours;

    /// @notice Maximum timelock duration (30 days)
    uint256 public constant MAX_TIMELOCK = 30 days;

    /// @notice Minimum swap amount to prevent dust attacks
    uint256 public constant MIN_SWAP_AMOUNT = 1000;

    /// @notice Fee denominator for basis points (10000 = 100%)
    uint256 public constant FEE_DENOMINATOR = 10000;

    /// @notice Maximum fee in basis points (5% = 500 bps)
    uint256 public constant MAX_FEE_BPS = 500;

    // ============ Enums ============

    /// @notice Possible states for a swap
    enum SwapState {
        Empty,      // Swap doesn't exist
        Active,     // Funds locked, awaiting claim or refund
        Claimed,    // Receiver claimed with secret
        Refunded    // Sender refunded after timeout
    }

    // ============ Structs ============

    /// @notice Swap data structure
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

    // ============ State Variables ============

    /// @notice Mapping of swap ID to Swap struct
    mapping(bytes32 => Swap) public swaps;

    /// @notice Address that receives DAO fees
    address public daoAddress;

    /// @notice Fee in basis points (default 0.2% = 20 bps)
    uint256 public feeBps = 20;

    /// @notice Emergency pause flag
    bool public paused;

    // ============ Events ============

    /// @notice Emitted when a new swap is created
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

    /// @notice Emitted when a swap is claimed (SECRET IS REVEALED HERE!)
    event SwapClaimed(
        bytes32 indexed swapId,
        address indexed receiver,
        bytes32 secret
    );

    /// @notice Emitted when a swap is refunded
    event SwapRefunded(
        bytes32 indexed swapId,
        address indexed sender
    );

    /// @notice Emitted when DAO address is updated
    event DaoAddressUpdated(address indexed oldDao, address indexed newDao);

    /// @notice Emitted when fee is updated
    event FeeBpsUpdated(uint256 oldFee, uint256 newFee);

    /// @notice Emitted when pause state changes
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
    error FeeTooHigh();

    // ============ Constructor ============

    /**
     * @notice Initialize the HTLC contract
     * @param _daoAddress Address to receive fees
     */
    constructor(address _daoAddress) Ownable(msg.sender) {
        if (_daoAddress == address(0)) revert InvalidDaoAddress();
        daoAddress = _daoAddress;
    }

    // ============ Modifiers ============

    /// @notice Reverts if contract is paused
    modifier whenNotPaused() {
        if (paused) revert ContractPaused();
        _;
    }

    // ============ External Functions ============

    /**
     * @notice Create a swap with native token (ETH/BNB/MATIC/etc)
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
     * @dev Requires prior approval for token transfer
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

        // Transfer tokens to contract (requires prior approval)
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
     * @dev The secret is emitted in the event for cross-chain coordination
     * @param swapId The swap identifier
     * @param secret The 32-byte secret that hashes to secretHash
     */
    function claim(bytes32 swapId, bytes32 secret) external nonReentrant {
        Swap storage swap = swaps[swapId];

        // Checks
        if (swap.state != SwapState.Active) revert SwapNotActive();
        if (swap.receiver != msg.sender) revert NotReceiver();
        if (sha256(abi.encodePacked(secret)) != swap.secretHash) revert InvalidSecret();

        // Effects (state change BEFORE external calls)
        swap.state = SwapState.Claimed;

        // Calculate amounts
        uint256 receiverAmount = swap.amount - swap.daoFee;

        // Interactions (external calls LAST)
        if (swap.token == address(0)) {
            // Native token transfer
            (bool success, ) = swap.receiver.call{value: receiverAmount}("");
            if (!success) revert TransferFailed();

            if (swap.daoFee > 0) {
                (success, ) = daoAddress.call{value: swap.daoFee}("");
                if (!success) revert TransferFailed();
            }
        } else {
            // ERC20 token transfer
            IERC20(swap.token).safeTransfer(swap.receiver, receiverAmount);
            if (swap.daoFee > 0) {
                IERC20(swap.token).safeTransfer(daoAddress, swap.daoFee);
            }
        }

        // Emit event with SECRET (critical for cross-chain swaps!)
        emit SwapClaimed(swapId, msg.sender, secret);
    }

    /**
     * @notice Refund swap funds after timelock expires
     * @dev Only the original sender can refund, and only after timelock
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
     * @notice Get full swap details
     * @param swapId The swap identifier
     * @return The Swap struct
     */
    function getSwap(bytes32 swapId) external view returns (Swap memory) {
        return swaps[swapId];
    }

    /**
     * @notice Check if a secret is valid for a swap (without claiming)
     * @param swapId The swap identifier
     * @param secret The secret to verify
     * @return True if the secret matches the stored hash
     */
    function verifySecret(bytes32 swapId, bytes32 secret) external view returns (bool) {
        return sha256(abi.encodePacked(secret)) == swaps[swapId].secretHash;
    }

    /**
     * @notice Check if a swap can be refunded
     * @param swapId The swap identifier
     * @return True if refund is available (active + timelock expired)
     */
    function canRefund(bytes32 swapId) external view returns (bool) {
        Swap storage swap = swaps[swapId];
        return swap.state == SwapState.Active && block.timestamp >= swap.timelock;
    }

    /**
     * @notice Check if a swap can be claimed
     * @param swapId The swap identifier
     * @return True if claim is possible (active + timelock not expired)
     */
    function canClaim(bytes32 swapId) external view returns (bool) {
        Swap storage swap = swaps[swapId];
        return swap.state == SwapState.Active && block.timestamp < swap.timelock;
    }

    /**
     * @notice Get time remaining until refund is possible
     * @param swapId The swap identifier
     * @return Seconds until refund (0 if already possible)
     */
    function timeUntilRefund(bytes32 swapId) external view returns (uint256) {
        Swap storage swap = swaps[swapId];
        if (swap.state != SwapState.Active || block.timestamp >= swap.timelock) {
            return 0;
        }
        return swap.timelock - block.timestamp;
    }

    /**
     * @notice Calculate swap ID from parameters (for cross-chain coordination)
     * @dev Both parties can compute the same ID independently
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

    /**
     * @notice Get the current chain ID
     * @return The chain ID
     */
    function getChainId() external view returns (uint256) {
        return block.chainid;
    }

    // ============ Admin Functions ============

    /**
     * @notice Update the DAO address that receives fees
     * @param _daoAddress New DAO address
     */
    function setDaoAddress(address _daoAddress) external onlyOwner {
        if (_daoAddress == address(0)) revert InvalidDaoAddress();
        emit DaoAddressUpdated(daoAddress, _daoAddress);
        daoAddress = _daoAddress;
    }

    /**
     * @notice Update the fee in basis points
     * @param _feeBps New fee (max 500 = 5%)
     */
    function setFeeBps(uint256 _feeBps) external onlyOwner {
        if (_feeBps > MAX_FEE_BPS) revert FeeTooHigh();
        emit FeeBpsUpdated(feeBps, _feeBps);
        feeBps = _feeBps;
    }

    /**
     * @notice Emergency pause/unpause
     * @param _paused New pause state
     */
    function setPaused(bool _paused) external onlyOwner {
        paused = _paused;
        emit Paused(_paused);
    }

    // ============ Internal Functions ============

    /**
     * @notice Validate swap creation parameters
     */
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
