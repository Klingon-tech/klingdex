// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "../src/KlingonHTLC.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

/// @notice Mock ERC20 token for testing
contract MockERC20 is ERC20 {
    constructor() ERC20("Mock Token", "MOCK") {
        _mint(msg.sender, 1_000_000 * 10**18);
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}

/// @notice Mock non-standard ERC20 (like USDT) that doesn't return bool
contract MockUSDT {
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function transfer(address to, uint256 amount) external {
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
    }

    function transferFrom(address from, address to, uint256 amount) external {
        allowance[from][msg.sender] -= amount;
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
    }

    function approve(address spender, uint256 amount) external {
        allowance[msg.sender][spender] = amount;
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }
}

/// @notice Reentrancy attacker contract
contract ReentrancyAttacker {
    KlingonHTLC public htlc;
    bytes32 public swapId;
    bytes32 public secret;
    bool public attacked;

    constructor(KlingonHTLC _htlc) {
        htlc = _htlc;
    }

    function setAttackParams(bytes32 _swapId, bytes32 _secret) external {
        swapId = _swapId;
        secret = _secret;
    }

    function attack() external {
        htlc.claim(swapId, secret);
    }

    receive() external payable {
        if (!attacked) {
            attacked = true;
            // Try to re-enter claim
            try htlc.claim(swapId, secret) {} catch {}
        }
    }
}

contract KlingonHTLCTest is Test {
    KlingonHTLC public htlc;
    MockERC20 public token;

    address public dao = address(0xDA0);
    address public alice = address(0xA11CE);
    address public bob = address(0xB0B);

    bytes32 public secret = bytes32(uint256(123456789));
    bytes32 public secretHash;

    uint256 public constant SWAP_AMOUNT = 1 ether;
    uint256 public constant TIMELOCK = 12 hours;

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

    function setUp() public {
        htlc = new KlingonHTLC(dao);
        token = new MockERC20();

        // Compute secret hash using SHA256 (same as Bitcoin)
        secretHash = sha256(abi.encodePacked(secret));

        // Fund test accounts
        vm.deal(alice, 100 ether);
        vm.deal(bob, 100 ether);
        token.mint(alice, 1_000_000 * 10**18);
        token.mint(bob, 1_000_000 * 10**18);
    }

    // ============ Helper Functions ============

    function _createSwapId(address sender, address receiver, uint256 nonce) internal view returns (bytes32) {
        return htlc.computeSwapId(
            sender,
            receiver,
            address(0),
            SWAP_AMOUNT,
            secretHash,
            block.timestamp + TIMELOCK,
            nonce
        );
    }

    // ============ Native Token Tests ============

    function test_CreateSwapNative() public {
        bytes32 swapId = keccak256("test-swap-1");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        vm.expectEmit(true, true, true, true);
        emit SwapCreated(swapId, alice, bob, address(0), SWAP_AMOUNT, 2000000000000000, secretHash, timelock);

        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(swap.sender, alice);
        assertEq(swap.receiver, bob);
        assertEq(swap.token, address(0));
        assertEq(swap.amount, SWAP_AMOUNT);
        assertEq(swap.secretHash, secretHash);
        assertEq(swap.timelock, timelock);
        assertEq(uint8(swap.state), uint8(KlingonHTLC.SwapState.Active));

        // Fee should be 0.2% = 20 bps
        assertEq(swap.daoFee, SWAP_AMOUNT * 20 / 10000);
    }

    function test_ClaimNative() public {
        bytes32 swapId = keccak256("test-swap-claim");
        uint256 timelock = block.timestamp + TIMELOCK;

        // Alice creates swap
        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        uint256 bobBalanceBefore = bob.balance;
        uint256 daoBalanceBefore = dao.balance;

        // Bob claims with secret
        vm.prank(bob);
        vm.expectEmit(true, true, false, true);
        emit SwapClaimed(swapId, bob, secret);

        htlc.claim(swapId, secret);

        // Verify balances
        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        uint256 expectedFee = SWAP_AMOUNT * 20 / 10000;
        uint256 expectedReceived = SWAP_AMOUNT - expectedFee;

        assertEq(uint8(swap.state), uint8(KlingonHTLC.SwapState.Claimed));
        assertEq(bob.balance, bobBalanceBefore + expectedReceived);
        assertEq(dao.balance, daoBalanceBefore + expectedFee);
    }

    function test_RefundNative() public {
        bytes32 swapId = keccak256("test-swap-refund");
        uint256 timelock = block.timestamp + TIMELOCK;

        // Alice creates swap
        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        uint256 aliceBalanceBefore = alice.balance;

        // Fast forward past timelock
        vm.warp(timelock + 1);

        // Alice refunds
        vm.prank(alice);
        vm.expectEmit(true, true, false, false);
        emit SwapRefunded(swapId, alice);

        htlc.refund(swapId);

        // Verify full amount returned (no fee on refund)
        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(uint8(swap.state), uint8(KlingonHTLC.SwapState.Refunded));
        assertEq(alice.balance, aliceBalanceBefore + SWAP_AMOUNT);
    }

    // ============ ERC20 Token Tests ============

    function test_CreateSwapERC20() public {
        bytes32 swapId = keccak256("test-swap-erc20");
        uint256 timelock = block.timestamp + TIMELOCK;
        uint256 amount = 1000 * 10**18;

        // Approve tokens
        vm.prank(alice);
        token.approve(address(htlc), amount);

        // Create swap
        vm.prank(alice);
        htlc.createSwapERC20(swapId, bob, address(token), amount, secretHash, timelock);

        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(swap.sender, alice);
        assertEq(swap.receiver, bob);
        assertEq(swap.token, address(token));
        assertEq(swap.amount, amount);
        assertEq(token.balanceOf(address(htlc)), amount);
    }

    function test_ClaimERC20() public {
        bytes32 swapId = keccak256("test-swap-erc20-claim");
        uint256 timelock = block.timestamp + TIMELOCK;
        uint256 amount = 1000 * 10**18;

        // Alice creates ERC20 swap
        vm.startPrank(alice);
        token.approve(address(htlc), amount);
        htlc.createSwapERC20(swapId, bob, address(token), amount, secretHash, timelock);
        vm.stopPrank();

        uint256 bobBalanceBefore = token.balanceOf(bob);
        uint256 daoBalanceBefore = token.balanceOf(dao);

        // Bob claims
        vm.prank(bob);
        htlc.claim(swapId, secret);

        uint256 expectedFee = amount * 20 / 10000;
        uint256 expectedReceived = amount - expectedFee;

        assertEq(token.balanceOf(bob), bobBalanceBefore + expectedReceived);
        assertEq(token.balanceOf(dao), daoBalanceBefore + expectedFee);
    }

    // ============ Validation Tests ============

    function test_RevertOnDuplicateSwapId() public {
        bytes32 swapId = keccak256("duplicate-swap");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.SwapAlreadyExists.selector);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);
    }

    function test_RevertOnInvalidReceiver() public {
        bytes32 swapId = keccak256("invalid-receiver");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.InvalidReceiver.selector);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, address(0), secretHash, timelock);
    }

    function test_RevertOnInvalidAmount() public {
        bytes32 swapId = keccak256("invalid-amount");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.InvalidAmount.selector);
        htlc.createSwapNative{value: 100}(swapId, bob, secretHash, timelock); // Less than MIN_SWAP_AMOUNT
    }

    function test_RevertOnTimelockTooShort() public {
        bytes32 swapId = keccak256("short-timelock");
        uint256 timelock = block.timestamp + 30 minutes; // Less than MIN_TIMELOCK (1 hour)

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.InvalidTimelock.selector);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);
    }

    function test_RevertOnTimelockTooLong() public {
        bytes32 swapId = keccak256("long-timelock");
        uint256 timelock = block.timestamp + 60 days; // More than MAX_TIMELOCK (30 days)

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.InvalidTimelock.selector);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);
    }

    function test_RevertOnClaimWrongSecret() public {
        bytes32 swapId = keccak256("wrong-secret");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        bytes32 wrongSecret = bytes32(uint256(999999));

        vm.prank(bob);
        vm.expectRevert(KlingonHTLC.InvalidSecret.selector);
        htlc.claim(swapId, wrongSecret);
    }

    function test_RevertOnClaimNotReceiver() public {
        bytes32 swapId = keccak256("not-receiver");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        vm.prank(alice); // Alice is not the receiver
        vm.expectRevert(KlingonHTLC.NotReceiver.selector);
        htlc.claim(swapId, secret);
    }

    function test_RevertOnRefundBeforeTimelock() public {
        bytes32 swapId = keccak256("early-refund");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.TimelockNotExpired.selector);
        htlc.refund(swapId);
    }

    function test_RevertOnRefundNotSender() public {
        bytes32 swapId = keccak256("not-sender");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        vm.warp(timelock + 1);

        vm.prank(bob); // Bob is not the sender
        vm.expectRevert(KlingonHTLC.NotSender.selector);
        htlc.refund(swapId);
    }

    function test_RevertOnDoubleClaim() public {
        bytes32 swapId = keccak256("double-claim");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        vm.prank(bob);
        htlc.claim(swapId, secret);

        vm.prank(bob);
        vm.expectRevert(KlingonHTLC.SwapNotActive.selector);
        htlc.claim(swapId, secret);
    }

    // ============ Reentrancy Tests ============

    function test_ReentrancyProtection() public {
        ReentrancyAttacker attacker = new ReentrancyAttacker(htlc);
        bytes32 swapId = keccak256("reentrancy-test");
        uint256 timelock = block.timestamp + TIMELOCK;

        // Create swap for attacker
        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, address(attacker), secretHash, timelock);

        // Set up attack
        attacker.setAttackParams(swapId, secret);

        // Attack should complete first claim but fail reentrancy
        attacker.attack();

        // Verify swap is claimed (only once)
        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(uint8(swap.state), uint8(KlingonHTLC.SwapState.Claimed));
    }

    // ============ Admin Tests ============

    function test_SetDaoAddress() public {
        address newDao = address(0xDA02);
        htlc.setDaoAddress(newDao);
        assertEq(htlc.daoAddress(), newDao);
    }

    function test_RevertOnSetDaoAddressZero() public {
        vm.expectRevert(KlingonHTLC.InvalidDaoAddress.selector);
        htlc.setDaoAddress(address(0));
    }

    function test_SetFeeBps() public {
        htlc.setFeeBps(50); // 0.5%
        assertEq(htlc.feeBps(), 50);
    }

    function test_RevertOnFeeTooHigh() public {
        vm.expectRevert(KlingonHTLC.FeeTooHigh.selector);
        htlc.setFeeBps(600); // 6% > MAX_FEE_BPS
    }

    function test_Pause() public {
        htlc.setPaused(true);
        assertTrue(htlc.paused());

        bytes32 swapId = keccak256("paused-swap");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        vm.expectRevert(KlingonHTLC.ContractPaused.selector);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);
    }

    function test_OnlyOwnerCanSetDaoAddress() public {
        vm.prank(alice);
        vm.expectRevert();
        htlc.setDaoAddress(alice);
    }

    // ============ View Function Tests ============

    function test_VerifySecret() public {
        bytes32 swapId = keccak256("verify-secret");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        assertTrue(htlc.verifySecret(swapId, secret));
        assertFalse(htlc.verifySecret(swapId, bytes32(uint256(999))));
    }

    function test_CanRefund() public {
        bytes32 swapId = keccak256("can-refund");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        assertFalse(htlc.canRefund(swapId));

        vm.warp(timelock + 1);
        assertTrue(htlc.canRefund(swapId));
    }

    function test_CanClaim() public {
        bytes32 swapId = keccak256("can-claim");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        assertTrue(htlc.canClaim(swapId));

        vm.warp(timelock + 1);
        assertFalse(htlc.canClaim(swapId));
    }

    function test_TimeUntilRefund() public {
        bytes32 swapId = keccak256("time-until");
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, secretHash, timelock);

        assertEq(htlc.timeUntilRefund(swapId), TIMELOCK);

        vm.warp(block.timestamp + 6 hours);
        assertEq(htlc.timeUntilRefund(swapId), 6 hours);

        vm.warp(timelock + 1);
        assertEq(htlc.timeUntilRefund(swapId), 0);
    }

    // ============ Fuzz Tests ============

    function testFuzz_CreateSwapNative(uint256 amount, uint256 timelockOffset) public {
        // Bound inputs to valid ranges
        amount = bound(amount, htlc.MIN_SWAP_AMOUNT(), 1000 ether);
        timelockOffset = bound(timelockOffset, htlc.MIN_TIMELOCK(), htlc.MAX_TIMELOCK());

        bytes32 swapId = keccak256(abi.encodePacked(amount, timelockOffset));
        uint256 timelock = block.timestamp + timelockOffset;

        vm.deal(alice, amount);
        vm.prank(alice);
        htlc.createSwapNative{value: amount}(swapId, bob, secretHash, timelock);

        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(swap.amount, amount);
        assertEq(swap.timelock, timelock);
    }

    function testFuzz_ClaimWithRandomSecret(bytes32 randomSecret) public {
        bytes32 randomSecretHash = sha256(abi.encodePacked(randomSecret));
        bytes32 swapId = keccak256(abi.encodePacked(randomSecret));
        uint256 timelock = block.timestamp + TIMELOCK;

        vm.prank(alice);
        htlc.createSwapNative{value: SWAP_AMOUNT}(swapId, bob, randomSecretHash, timelock);

        vm.prank(bob);
        htlc.claim(swapId, randomSecret);

        KlingonHTLC.Swap memory swap = htlc.getSwap(swapId);
        assertEq(uint8(swap.state), uint8(KlingonHTLC.SwapState.Claimed));
    }
}
