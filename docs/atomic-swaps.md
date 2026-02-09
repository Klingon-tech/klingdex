# Comprehensive Atomic Swap Implementation Guide

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Chain Classification & Swap Methods](#2-chain-classification--swap-methods)
3. [UTXO Chains: Taproot vs Non-Taproot](#3-utxo-chains-taproot-vs-non-taproot)
4. [EVM Chains: Smart Contract HTLCs](#4-evm-chains-smart-contract-htlcs)
5. [CryptoNote Chains: Adaptor Signatures](#5-cryptonote-chains-adaptor-signatures)
6. [Maker vs Taker: Role Asymmetry](#6-maker-vs-taker-role-asymmetry)
7. [Cross-Category Swap Matrix](#7-cross-category-swap-matrix)
8. [Complete Protocol Flows](#8-complete-protocol-flows)
9. [2-of-2 Escrow Without Mediators](#9-2-of-2-escrow-without-mediators)
10. [Claim Failure Protection](#10-claim-failure-protection)
11. [Security Considerations](#11-security-considerations)
12. [Implementation Architecture](#12-implementation-architecture)
13. [Lessons from Bisq-MuSig Protocol](#13-lessons-from-bisq-musig-protocol)
14. [JSON-RPC API Reference](#14-json-rpc-api-reference)
15. [References & Resources](#15-references--resources)

---

## 1. Executive Summary

### The Core Challenge

Different blockchains have fundamentally different capabilities:

| Chain Type | Scripting | Timelocks | Signature Scheme | Example |
|------------|-----------|-----------|------------------|---------|
| **UTXO + Taproot** | Full | Native | Schnorr | BTC, LTC |
| **UTXO Legacy** | Limited | Native | ECDSA | BCH, DOGE |
| **EVM** | Smart Contracts | Native | ECDSA | ETH, BSC, MATIC |
| **CryptoNote** | None | None | EdDSA | XMR |

### Key Insight from Original Document

The original `conversation.md` correctly identifies that **crypto-to-crypto swaps eliminate arbitration** because blockchain mechanics enforce atomicity. However, it doesn't address:

1. What happens when one chain lacks Taproot/scripting
2. How role (maker vs taker) affects the protocol
3. CryptoNote-specific challenges (no scripts, no timelocks)

### Critical Corrections & Additions

**Issue 1: MuSig2 isn't always available**
The original doc prefers MuSig2 for BTC swaps, but the counterparty chain must also support compatible signatures. For BTC ↔ ETH, you **must** use HTLCs because EVM uses ECDSA.

**Issue 2: Timeout asymmetry has constraints**
The 24h/12h example works for similar chains, but BTC ↔ ETH needs careful consideration of block times and finality.

**Issue 3: Monero requires completely different approach**
XMR has no scripting—adaptor signatures are the **only** option, not an alternative.

---

## 2. Chain Classification & Swap Methods

### Decision Tree

```
                    ┌─────────────────────────┐
                    │   What chains are we    │
                    │      swapping?          │
                    └───────────┬─────────────┘
                                │
            ┌───────────────────┼───────────────────┐
            │                   │                   │
            ▼                   ▼                   ▼
    ┌───────────────┐   ┌───────────────┐   ┌───────────────┐
    │ Both support  │   │ One or both   │   │ One chain is  │
    │ Schnorr +     │   │ are EVM       │   │ CryptoNote    │
    │ Taproot?      │   │               │   │ (Monero)?     │
    └───────┬───────┘   └───────┬───────┘   └───────┬───────┘
            │                   │                   │
            ▼                   ▼                   ▼
    ┌───────────────┐   ┌───────────────┐   ┌───────────────┐
    │   MuSig2 +    │   │  HTLC with    │   │   Adaptor     │
    │   Adaptor     │   │  Smart        │   │   Signatures  │
    │   Signatures  │   │  Contracts    │   │   + DLEQ      │
    └───────────────┘   └───────────────┘   └───────────────┘
```

### Supported Swap Methods by Chain

```go
// internal/config/swap_methods.go

type SwapMethod uint8

const (
    SwapMethodMuSig2     SwapMethod = iota // Taproot + Schnorr
    SwapMethodHTLC                          // Traditional hash-locked
    SwapMethodAdaptor                       // Scriptless (for XMR)
    SwapMethodEVMHTLC                       // Smart contract HTLC
)

var ChainCapabilities = map[string]ChainCapability{
    "BTC": {
        SupportsTaproot:   true,
        SupportsHTLC:      true,
        SupportsAdaptor:   true,
        SignatureScheme:   SignatureSchnorr,
        HasNativeTimelock: true,
    },
    "LTC": {
        SupportsTaproot:   true,  // MWEB upgrade
        SupportsHTLC:      true,
        SupportsAdaptor:   true,
        SignatureScheme:   SignatureSchnorr,
        HasNativeTimelock: true,
    },
    "BCH": {
        SupportsTaproot:   false, // No Schnorr
        SupportsHTLC:      true,
        SupportsAdaptor:   false,
        SignatureScheme:   SignatureECDSA,
        HasNativeTimelock: true,
    },
    "DOGE": {
        SupportsTaproot:   false,
        SupportsHTLC:      true,
        SupportsAdaptor:   false,
        SignatureScheme:   SignatureECDSA,
        HasNativeTimelock: true,
    },
    "ETH": {
        SupportsTaproot:   false,
        SupportsHTLC:      true,  // Via smart contract
        SupportsAdaptor:   false, // Account model incompatible
        SignatureScheme:   SignatureECDSA,
        HasNativeTimelock: true,  // Via smart contract
    },
    "XMR": {
        SupportsTaproot:   false,
        SupportsHTLC:      false, // No scripting!
        SupportsAdaptor:   true,  // Only option
        SignatureScheme:   SignatureEdDSA,
        HasNativeTimelock: false, // No timelocks!
    },
}

// GetSwapMethod determines the appropriate swap method for a pair
func GetSwapMethod(chainA, chainB string) SwapMethod {
    capA := ChainCapabilities[chainA]
    capB := ChainCapabilities[chainB]

    // If either chain is CryptoNote, must use adaptor signatures
    if !capA.HasNativeTimelock || !capB.HasNativeTimelock {
        return SwapMethodAdaptor
    }

    // If both support Taproot and Schnorr, prefer MuSig2
    if capA.SupportsTaproot && capB.SupportsTaproot {
        return SwapMethodMuSig2
    }

    // If either is EVM, use smart contract HTLC
    if isEVM(chainA) || isEVM(chainB) {
        return SwapMethodEVMHTLC
    }

    // Fallback to traditional HTLC
    return SwapMethodHTLC
}
```

---

## 3. UTXO Chains: Taproot vs Non-Taproot

### When Both Chains Support Taproot (BTC ↔ LTC)

**Preferred: MuSig2 + Adaptor Signatures**

This is the most private option—transactions look like regular single-sig spends.

```
BTC Chain                           LTC Chain
─────────                           ─────────
    │                                   │
    │  1. Alice & Bob create           │
    │     MuSig2 aggregate key         │
    │                                   │
    ▼                                   │
┌────────────┐                          │
│ P2TR Output│ ◄── Alice locks BTC     │
│ (2-of-2)   │                          │
└────────────┘                          │
    │                                   │
    │  2. Bob verifies, then           │
    │     locks on LTC                 │
    │                                   ▼
    │                          ┌────────────┐
    │                          │ P2TR Output│
    │                          │ (2-of-2)   │
    │                          └────────────┘
    │                                   │
    │  3. Alice claims LTC using       │
    │     adaptor signature            │
    │     (reveals secret)             │
    │                                   ▼
    │                          ┌────────────┐
    │                          │ Alice's    │
    │                          │ Wallet     │
    │                          └────────────┘
    │                                   │
    │  4. Bob extracts secret,         │
    │     claims BTC                   │
    ▼                                   │
┌────────────┐                          │
│ Bob's      │                          │
│ Wallet     │                          │
└────────────┘                          │
```

**Key Advantage**: No visible hash on-chain, transactions cannot be linked.

### When One Chain Lacks Taproot (BTC ↔ BCH or BTC ↔ DOGE)

**Required: Traditional HTLC**

BCH and DOGE don't have Schnorr signatures, so we fall back to hash-based HTLCs.

```
BTC (Taproot)                       BCH (Legacy)
─────────────                       ────────────
      │                                  │
      │  Can use P2TR with              │
      │  script path for HTLC           │
      │                                  │
      ▼                                  │
┌──────────────┐                         │
│ Taproot      │                         │
│ Key: MuSig2  │                         │
│ Script: HTLC │ ◄── Alice locks        │
└──────────────┘                         │
      │                                  │
      │  Bob locks using               │
      │  P2SH HTLC (legacy)            │
      │                                  ▼
      │                         ┌──────────────┐
      │                         │ P2SH HTLC    │
      │                         │ (legacy)     │
      │                         └──────────────┘
      │                                  │
      │  Same hash on both chains       │
      │  (linkable, less private)       │
      │                                  │
```

**HTLC Script for BCH/DOGE (P2SH)**:

```
OP_IF
    OP_SHA256 <hash> OP_EQUALVERIFY
    <recipient_pubkey> OP_CHECKSIG
OP_ELSE
    <timeout> OP_CHECKLOCKTIMEVERIFY OP_DROP
    <sender_pubkey> OP_CHECKSIG
OP_ENDIF
```

### Taproot HTLC with Script Path (Hybrid)

When swapping BTC ↔ non-Taproot chain, BTC side can still use Taproot for efficiency:

```go
// Create Taproot output with HTLC in script tree
func createTaprootHTLC(
    buyerPub, sellerPub *btcec.PublicKey,
    hash [32]byte,
    timeout uint32,
) (*btcec.PublicKey, []byte) {

    // Key path: 2-of-2 MuSig2 (cooperative close)
    aggKey, _, _, _ := musig2.AggregateKeys(
        []*btcec.PublicKey{buyerPub, sellerPub},
        true,
    )

    // Script path: HTLC conditions
    htlcScript := buildHTLCScript(hash, sellerPub, buyerPub, timeout)
    refundScript := buildRefundScript(buyerPub, timeout + 1000)

    // Build tap tree
    tapLeaf1 := txscript.NewBaseTapLeaf(htlcScript)
    tapLeaf2 := txscript.NewBaseTapLeaf(refundScript)
    tapBranch := txscript.NewTapBranch(tapLeaf1, tapLeaf2)

    // Tweak aggregate key with script root
    tapRoot := tapBranch.TapHash()
    outputKey := txscript.ComputeTaprootOutputKey(aggKey.PreTweakedKey, tapRoot[:])

    return outputKey, tapRoot[:]
}
```

---

## 4. EVM Chains: Smart Contract HTLCs

### Why EVM is Different

1. **Account Model**: No UTXOs, transactions modify state
2. **No Pre-signing**: Can't pre-sign refund tx (nonce unknown)
3. **Gas Costs**: Every operation costs gas
4. **Reentrancy Risk**: Must guard against recursive calls

### Critical Security: Reentrancy Protection

The original `conversation.md` Solidity contract is **vulnerable to reentrancy**. Here's the fixed version:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/security/ReentrancyGuard.sol";

contract AtomicSwapERC20 is ReentrancyGuard {
    using SafeERC20 for IERC20;

    struct Swap {
        address initiator;
        address participant;
        address token;
        uint256 amount;
        bytes32 hashlock;
        uint256 timelock;
        bool completed;
        bool refunded;
    }

    mapping(bytes32 => Swap) public swaps;
    address public immutable daoTreasury;
    uint256 public constant DAO_FEE_BPS = 20; // 0.2%

    event SwapInitiated(bytes32 indexed swapId, address initiator, address participant);
    event SwapCompleted(bytes32 indexed swapId, bytes32 preimage);
    event SwapRefunded(bytes32 indexed swapId);

    constructor(address _daoTreasury) {
        require(_daoTreasury != address(0), "Invalid treasury");
        daoTreasury = _daoTreasury;
    }

    function initiate(
        bytes32 swapId,
        address participant,
        address token,
        uint256 amount,
        bytes32 hashlock,
        uint256 timelock
    ) external nonReentrant {
        require(swaps[swapId].initiator == address(0), "Exists");
        require(participant != address(0), "Invalid participant");
        require(amount > 0, "Zero amount");
        require(timelock > block.timestamp, "Past timelock");

        // Pull tokens (requires prior approval)
        IERC20(token).safeTransferFrom(msg.sender, address(this), amount);

        swaps[swapId] = Swap({
            initiator: msg.sender,
            participant: participant,
            token: token,
            amount: amount,
            hashlock: hashlock,
            timelock: timelock,
            completed: false,
            refunded: false
        });

        emit SwapInitiated(swapId, msg.sender, participant);
    }

    function complete(bytes32 swapId, bytes32 preimage) external nonReentrant {
        Swap storage swap = swaps[swapId];

        // CHECKS
        require(!swap.completed && !swap.refunded, "Already settled");
        require(sha256(abi.encodePacked(preimage)) == swap.hashlock, "Bad preimage");

        // EFFECTS (state change BEFORE external calls)
        swap.completed = true;

        // Calculate fees
        uint256 daoFee = (swap.amount * DAO_FEE_BPS) / 10000;
        uint256 participantAmount = swap.amount - daoFee;

        // INTERACTIONS (external calls LAST)
        IERC20(swap.token).safeTransfer(swap.participant, participantAmount);
        IERC20(swap.token).safeTransfer(daoTreasury, daoFee);

        emit SwapCompleted(swapId, preimage);
    }

    function refund(bytes32 swapId) external nonReentrant {
        Swap storage swap = swaps[swapId];

        require(!swap.completed && !swap.refunded, "Already settled");
        require(block.timestamp >= swap.timelock, "Too early");
        require(msg.sender == swap.initiator, "Not initiator");

        swap.refunded = true;
        IERC20(swap.token).safeTransfer(swap.initiator, swap.amount);

        emit SwapRefunded(swapId);
    }
}
```

### ERC20 vs Native ETH: Two-Step Approval

ERC20 swaps require **two transactions** from the initiator:

```
Transaction 1: Approve
──────────────────────
User calls: ERC20.approve(swapContract, amount)
Gas: ~46,000

Transaction 2: Initiate
───────────────────────
User calls: SwapContract.initiate(...)
Contract calls: ERC20.transferFrom(user, contract, amount)
Gas: ~95,000
```

For **native ETH**, only one transaction is needed:

```solidity
function initiateETH(
    bytes32 swapId,
    address participant,
    bytes32 hashlock,
    uint256 timelock
) external payable nonReentrant {
    require(msg.value > 0, "No ETH sent");
    // ... rest of logic
}
```

### Gas Optimization Strategies

```solidity
// 1. Cache storage reads
function complete(bytes32 swapId, bytes32 preimage) external {
    Swap memory swap = swaps[swapId]; // Copy to memory once
    // Use swap.amount instead of swaps[swapId].amount repeatedly
}

// 2. Use bytes32 for swap IDs (cheaper than string)
// 3. Pack struct fields to minimize storage slots
// 4. Use immutable for constants set in constructor
```

---

## 5. CryptoNote Chains: Adaptor Signatures

### The Monero Challenge

Monero is fundamentally different:

| Feature | Bitcoin | Monero |
|---------|---------|--------|
| Scripting | Yes (Script) | **No** |
| Timelocks | Yes (OP_CLTV) | **No** |
| Multi-sig | Native 2-of-2 | Shared key only |
| Curve | secp256k1 | **ed25519** |

**You cannot implement HTLCs on Monero.** There's no way to say "this output can be spent if you reveal preimage X."

### Solution: Adaptor Signatures

Instead of on-chain scripts, we use **cryptographic tricks** where revealing a signature automatically reveals a secret.

**Core Concept**:
```
Normal Schnorr signature:   s = r + e*x         (where x is private key)
Adaptor signature:          s' = r + t + e*x    (encrypted with secret t)

To create valid signature:  s = s' - t          (must know t)
When published on-chain:    s is visible
Secret extraction:          t = s' - s          (anyone can compute)
```

### Cross-Curve DLEQ Proofs

Bitcoin uses secp256k1, Monero uses ed25519. To link them, we need a **Discrete Log Equality (DLEQ) proof** showing the same secret works on both curves.

```go
import "github.com/athanorlabs/go-dleq"

// Prove knowledge of secret s such that:
// P_btc = s * G_secp256k1  AND
// P_xmr = s * G_ed25519

func createCrossGroupProof(secret *big.Int) (*DLEQProof, error) {
    // Generate points on both curves
    btcPoint := secp256k1.ScalarBaseMult(secret)
    xmrPoint := ed25519.ScalarBaseMult(secret)

    // Create DLEQ proof
    proof := dleq.Prove(secret, btcPoint, xmrPoint)
    return proof, nil
}

// Verify the proof
func verifyCrossGroupProof(proof *DLEQProof, btcPoint, xmrPoint []byte) bool {
    return dleq.Verify(proof, btcPoint, xmrPoint)
}
```

### BTC ↔ XMR Swap Protocol

**Critical Constraint**: Only the Bitcoin holder can initiate (BTC has scripting for refunds).

```
SETUP
═══════════════════════════════════════════════════════════════

Alice has: 1 XMR, wants BTC
Bob has:   0.01 BTC, wants XMR

Both generate keypairs:
  Alice: (a, A) on ed25519    [for XMR]
         (a', A') on secp256k1 [for BTC adaptor]
  Bob:   (b, B) on ed25519
         (b', B') on secp256k1

They exchange public keys and DLEQ proofs proving
same discrete log across curves.


STEP 1: Bob Locks BTC (24h timeout)
═══════════════════════════════════════════════════════════════

Bob creates Bitcoin output spendable by:
  • Alice's adaptor signature (if she reveals a')
  • Bob after 24 hours (refund)

┌────────────────────────────────────────────────────────────┐
│  BTC Output (0.01 BTC)                                     │
│                                                            │
│  Spend conditions:                                         │
│    Key path: 2-of-2 (A' + B')                             │
│    Script path:                                            │
│      Leaf 1: <Alice adaptor> + <Bob sig>                  │
│      Leaf 2: <24h> CSV + <Bob refund>                     │
│                                                            │
│  Bob pre-signs adaptor signature encrypted with a'        │
└────────────────────────────────────────────────────────────┘


STEP 2: Alice Verifies and Locks XMR
═══════════════════════════════════════════════════════════════

Alice verifies:
  ✓ Bob's BTC is locked
  ✓ Adaptor signature is valid (encrypted with her key)
  ✓ Timeout is reasonable

Alice locks XMR to address with combined key:
  XMR_address = H(A + B)   [where A, B are ed25519 points]

Private key is SPLIT:
  Full key = a + b
  Alice knows: a
  Bob knows:   b
  Neither can spend alone!

┌────────────────────────────────────────────────────────────┐
│  XMR Output (1 XMR)                                        │
│                                                            │
│  Spendable by: whoever knows (a + b)                       │
│                                                            │
│  Currently:                                                │
│    Alice knows: a                                          │
│    Bob knows:   b                                          │
│    Neither can spend!                                      │
└────────────────────────────────────────────────────────────┘


STEP 3: Alice Claims BTC (Reveals Secret)
═══════════════════════════════════════════════════════════════

Alice decrypts Bob's adaptor signature using a':
  valid_sig = adaptor_sig - a'

She broadcasts claim transaction with valid_sig.

THE SIGNATURE IS NOW PUBLIC ON BITCOIN BLOCKCHAIN!

Bob can compute: a' = adaptor_sig - valid_sig

Since Alice proved (via DLEQ) that a' on secp256k1 corresponds
to a on ed25519, Bob now knows a.


STEP 4: Bob Claims XMR
═══════════════════════════════════════════════════════════════

Bob computes full XMR spending key:
  full_key = a + b   (he knows both now!)

Bob spends XMR to his wallet.

SWAP COMPLETE!


REFUND SCENARIOS
═══════════════════════════════════════════════════════════════

If Alice doesn't claim BTC within 24h:
  • Bob refunds his BTC (timeout script)
  • Bob reveals b to Alice (or broadcasts refund revealing b)
  • Alice computes full_key = a + b, recovers her XMR

If Bob disappears after locking BTC:
  • Alice never locks XMR (she verified BTC first)
  • No loss for Alice

If Alice disappears after locking XMR:
  • Bob refunds BTC after timeout
  • Alice still has her XMR key share (a)
  • Bob's refund tx reveals b
  • Alice recovers XMR with a + b
```

### Go Implementation for XMR Swaps

```go
package xmr

import (
    "github.com/athanorlabs/atomic-swap/monero"
    "github.com/athanorlabs/go-dleq"
)

type XMRSwapSession struct {
    // Our keys
    spendKey    *monero.PrivateSpendKey  // ed25519
    adaptorKey  *btcec.PrivateKey         // secp256k1

    // Counterparty public keys
    theirSpendPub  *monero.PublicKey
    theirAdaptorPub *btcec.PublicKey

    // DLEQ proof linking our keys across curves
    dleqProof *dleq.Proof

    // Shared XMR address (neither party can spend alone)
    sharedAddress string
}

// Alice (XMR seller) initiates by creating keys
func NewXMRSellerSession() (*XMRSwapSession, error) {
    // Generate ed25519 keypair for Monero
    spendKey, err := monero.GenerateKeys()
    if err != nil {
        return nil, err
    }

    // Generate secp256k1 keypair for Bitcoin adaptor
    adaptorKey, err := btcec.NewPrivateKey()
    if err != nil {
        return nil, err
    }

    // Create DLEQ proof showing same secret on both curves
    proof, err := dleq.Prove(
        spendKey.Secret(),
        adaptorKey.PubKey(),
        spendKey.Public(),
    )
    if err != nil {
        return nil, err
    }

    return &XMRSwapSession{
        spendKey:   spendKey,
        adaptorKey: adaptorKey,
        dleqProof:  proof,
    }, nil
}

// After receiving Bob's keys, compute shared XMR address
func (s *XMRSwapSession) ComputeSharedAddress(theirSpendPub *monero.PublicKey) string {
    // Combined public key: A + B
    combinedPub := monero.AddPublicKeys(s.spendKey.Public(), theirSpendPub)

    // Derive address from combined key
    s.sharedAddress = monero.PublicKeyToAddress(combinedPub, monero.Mainnet)
    return s.sharedAddress
}

// Extract counterparty's secret from Bitcoin transaction
func (s *XMRSwapSession) ExtractSecretFromBTC(
    adaptorSig []byte,
    publishedSig []byte,
) (*monero.PrivateSpendKey, error) {
    // secret = adaptor_sig - published_sig
    secret := subtractSignatures(adaptorSig, publishedSig)

    // Convert secp256k1 scalar to ed25519
    xmrSecret, err := dleq.ConvertScalar(secret, dleq.Secp256k1ToEd25519)
    if err != nil {
        return nil, err
    }

    return monero.NewPrivateSpendKey(xmrSecret), nil
}

// Compute full XMR spending key and claim
func (s *XMRSwapSession) ClaimXMR(theirSecret *monero.PrivateSpendKey) error {
    // full_key = our_secret + their_secret
    fullKey := monero.AddPrivateKeys(s.spendKey, theirSecret)

    // Verify it matches the shared address
    if monero.PrivateToAddress(fullKey) != s.sharedAddress {
        return errors.New("key mismatch")
    }

    // Sweep XMR to our wallet
    return s.sweepToWallet(fullKey)
}
```

---

## 6. Maker vs Taker: Role Asymmetry

### Does the Flow Change Based on Role?

**Short answer: YES, significantly.**

The party who **generates the secret** (or locks first) has different responsibilities and risks than the responder.

### Role Definitions

| Role | Also Called | Locks First? | Generates Secret? | Longer Timeout? |
|------|-------------|--------------|-------------------|-----------------|
| **Maker** | Initiator | Yes | Yes (for HTLC) | Yes |
| **Taker** | Responder | No (verifies first) | No | No (shorter) |

### Why Timeout Asymmetry Exists

```
Timeline
══════════════════════════════════════════════════════════════
0h              12h              24h
│               │                │
├───────────────┼────────────────┤
│               │                │
│   SWAP        │   Taker can    │   Maker can
│   WINDOW      │   refund       │   refund
│               │                │

If Maker timeout = Taker timeout:
  Maker could claim taker's funds, then refund own funds!

Solution: Taker timeout < Maker timeout
  Taker refunds BEFORE maker can, preventing theft.
```

### Chain-Specific Role Constraints

#### UTXO ↔ UTXO (BTC ↔ LTC)

Either party can be maker or taker. The protocol is symmetric.

```
Alice (Maker, has BTC)          Bob (Taker, has LTC)
─────────────────────           ───────────────────
1. Generates secret
2. Locks BTC (24h timeout)
                                3. Verifies BTC lock
                                4. Locks LTC (12h timeout)
5. Claims LTC (reveals secret)
                                6. Extracts secret, claims BTC
```

#### UTXO ↔ EVM (BTC ↔ ETH)

Either can be maker, but EVM has faster finality:

```
Scenario A: BTC Maker (Alice has BTC, wants ETH)
────────────────────────────────────────────────
Alice locks BTC (144 blocks ≈ 24h)
Bob locks ETH (7200 seconds ≈ 2h)
Alice claims ETH (reveals preimage)
Bob claims BTC

Scenario B: ETH Maker (Alice has ETH, wants BTC)
────────────────────────────────────────────────
Alice locks ETH (86400 seconds ≈ 24h)
Bob locks BTC (72 blocks ≈ 12h)
Alice claims BTC (reveals preimage)
Bob claims ETH
```

**Important**: ETH confirmations are faster, so ETH-side timeouts can be shorter.

#### UTXO ↔ CryptoNote (BTC ↔ XMR)

**ONLY the Bitcoin holder can be maker!**

Monero has no scripting, so:
- No timelocks for refunds
- No HTLC possible on XMR side
- BTC holder must lock first to enable refund path

```
ALWAYS:
Bob (has BTC) = Maker (must lock first)
Alice (has XMR) = Taker (locks second)

Why?
- BTC has script: Bob can create timeout refund
- XMR has no script: Alice cannot create timeout
- Solution: Split XMR key, Bob's refund reveals his share

If Alice wants to sell XMR for BTC:
  Alice = XMR seller = Taker
  Bob = BTC seller = Maker
```

### Bisq's Approach to Role Asymmetry

Bisq uses a **security deposit model** where both parties have economic skin in the game:

```
Bisq Trade (BTC ↔ Fiat/Altcoin)
══════════════════════════════════════════════════════════════

Both parties deposit into 2-of-2 multisig:
┌────────────────────────────────────────────────────────────┐
│  Escrow (2-of-2 multisig)                                  │
│                                                            │
│  Contents:                                                 │
│    • Maker security deposit (15-50%)                       │
│    • Taker security deposit (same amount)                  │
│    • Trade amount (from BTC seller)                        │
│    • Trading fees                                          │
└────────────────────────────────────────────────────────────┘

Key insight: Security deposits make fraud expensive.
Even if you could steal the trade amount, you lose your deposit.

Fee Asymmetry:
  Maker: 0.15% fee
  Taker: 1.15% fee

Why? Makers provide liquidity, takers consume it.
```

### Implementing Role-Based Logic

```go
// internal/swap/roles.go

type SwapRole uint8

const (
    RoleMaker SwapRole = iota
    RoleTaker
)

type SwapParticipant struct {
    Role          SwapRole
    Chain         string
    Amount        *big.Int
    PublicKey     []byte
    Timeout       uint32
    HasSecret     bool  // Only maker has this initially
}

// DetermineRoles assigns maker/taker based on chain constraints
func DetermineRoles(chainA, chainB string, initiatorChain string) (maker, taker string) {
    // CryptoNote constraint: BTC holder must be maker
    if isCryptoNote(chainA) {
        return chainB, chainA
    }
    if isCryptoNote(chainB) {
        return chainA, chainB
    }

    // Otherwise, initiator is maker
    if initiatorChain == chainA {
        return chainA, chainB
    }
    return chainB, chainA
}

// CalculateTimeouts returns appropriate timeouts based on chains
func CalculateTimeouts(makerChain, takerChain string) (makerTimeout, takerTimeout uint32) {
    makerBlocks := getBlocksForDuration(makerChain, 24*time.Hour)

    // Taker timeout must be less than half maker timeout
    // This ensures maker can't claim then refund
    takerBlocks := getBlocksForDuration(takerChain, 12*time.Hour)

    // For EVM, use seconds instead of blocks
    if isEVM(takerChain) {
        takerTimeout = uint32(7200) // 2 hours in seconds
    } else {
        takerTimeout = uint32(takerBlocks)
    }

    if isEVM(makerChain) {
        makerTimeout = uint32(86400) // 24 hours in seconds
    } else {
        makerTimeout = uint32(makerBlocks)
    }

    return makerTimeout, takerTimeout
}
```

---

## 7. Cross-Category Swap Matrix

### Complete Compatibility Matrix

```
            │ BTC   │ LTC   │ BCH   │ DOGE  │ ETH   │ BSC   │ XMR   │
────────────┼───────┼───────┼───────┼───────┼───────┼───────┼───────┤
BTC         │   -   │ MuSig │ HTLC  │ HTLC  │ HTLC* │ HTLC* │ Adapt │
LTC         │ MuSig │   -   │ HTLC  │ HTLC  │ HTLC* │ HTLC* │ Adapt │
BCH         │ HTLC  │ HTLC  │   -   │ HTLC  │ HTLC* │ HTLC* │   ✗   │
DOGE        │ HTLC  │ HTLC  │ HTLC  │   -   │ HTLC* │ HTLC* │   ✗   │
ETH         │ HTLC* │ HTLC* │ HTLC* │ HTLC* │   -   │ HTLC* │ Adapt†│
BSC         │ HTLC* │ HTLC* │ HTLC* │ HTLC* │ HTLC* │   -   │ Adapt†│
XMR         │ Adapt │ Adapt │   ✗   │   ✗   │ Adapt†│ Adapt†│   -   │

Legend:
  MuSig  = MuSig2 + Adaptor Signatures (most private)
  HTLC   = Traditional Hash Time-Locked Contract
  HTLC*  = HTLC with Smart Contract
  Adapt  = Adaptor Signatures (required for XMR)
  Adapt† = Adaptor Signatures (experimental for EVM)
  ✗      = Not supported (no scripting on responder side)
```

### Why Some Pairs Don't Work

**BCH/DOGE ↔ XMR: Not Supported**

Both sides lack advanced capabilities:
- XMR: No scripting, no timelocks
- BCH/DOGE: No Schnorr signatures for adaptor signatures

Without either HTLCs or adaptor signatures, there's no atomic mechanism.

**Solution**: Route through BTC

```
User wants: BCH → XMR

Atomic path:
  1. BCH → BTC (HTLC)
  2. BTC → XMR (Adaptor signatures)

Or use Bisq-style escrow with security deposits.
```

### Configuration

```go
// internal/config/swap_matrix.go

type SwapPair struct {
    ChainA     string
    ChainB     string
    Method     SwapMethod
    Supported  bool
    Notes      string
}

var SupportedPairs = []SwapPair{
    // MuSig2 pairs (Taproot + Schnorr)
    {"BTC", "LTC", SwapMethodMuSig2, true, "Most private, Taproot key path"},

    // HTLC pairs (UTXO)
    {"BTC", "BCH", SwapMethodHTLC, true, "BTC uses Taproot script path"},
    {"BTC", "DOGE", SwapMethodHTLC, true, "BTC uses Taproot script path"},
    {"LTC", "BCH", SwapMethodHTLC, true, "Standard HTLC"},
    {"LTC", "DOGE", SwapMethodHTLC, true, "Standard HTLC"},
    {"BCH", "DOGE", SwapMethodHTLC, true, "Standard HTLC"},

    // EVM pairs
    {"BTC", "ETH", SwapMethodEVMHTLC, true, "Smart contract on ETH"},
    {"BTC", "BSC", SwapMethodEVMHTLC, true, "Smart contract on BSC"},
    {"BTC", "MATIC", SwapMethodEVMHTLC, true, "Smart contract on Polygon"},
    {"LTC", "ETH", SwapMethodEVMHTLC, true, "Smart contract on ETH"},
    {"ETH", "BSC", SwapMethodEVMHTLC, true, "Both smart contract"},

    // Monero pairs (adaptor signatures)
    {"BTC", "XMR", SwapMethodAdaptor, true, "BTC must be maker"},
    {"LTC", "XMR", SwapMethodAdaptor, true, "LTC must be maker"},
    {"ETH", "XMR", SwapMethodAdaptor, true, "Experimental, ETH must be maker"},

    // Unsupported
    {"BCH", "XMR", SwapMethodAdaptor, false, "BCH lacks Schnorr"},
    {"DOGE", "XMR", SwapMethodAdaptor, false, "DOGE lacks Schnorr"},
}
```

---

## 8. Complete Protocol Flows

### Flow 1: BTC ↔ LTC (MuSig2 + Adaptor Signatures)

**Best case: Both chains support Taproot**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    BTC ↔ LTC ATOMIC SWAP (MuSig2)                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  PHASE 1: KEY EXCHANGE                                                  │
│  ═══════════════════════                                                │
│                                                                         │
│  Alice (BTC Maker)              Bob (LTC Taker)                        │
│  ────────────────               ───────────────                        │
│  Generate keypair               Generate keypair                        │
│  (a, A)                         (b, B)                                  │
│                                                                         │
│  Generate adaptor secret        │                                       │
│  t (random scalar)              │                                       │
│  T = t·G (adaptor point)        │                                       │
│                                                                         │
│        ─────── Exchange A, B, T ───────►                                │
│        ◄────── Exchange B ─────────────                                 │
│                                                                         │
│                                                                         │
│  PHASE 2: AGGREGATE KEYS                                                │
│  ═══════════════════════                                                │
│                                                                         │
│  Both compute:                                                          │
│    BTC output key = MuSig2(A, B) with BIP86 tweak                      │
│    LTC output key = MuSig2(A, B) with BIP86 tweak                      │
│                                                                         │
│                                                                         │
│  PHASE 3: LOCK FUNDS                                                    │
│  ═══════════════════                                                    │
│                                                                         │
│  Alice locks BTC:                                                       │
│  ┌───────────────────────────────────────┐                              │
│  │ BTC P2TR Output                       │                              │
│  │ Key: MuSig2(A, B)                    │                              │
│  │ Script: CSV(144) + A (refund)        │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│  Bob verifies, then locks LTC:                                          │
│  ┌───────────────────────────────────────┐                              │
│  │ LTC P2TR Output                       │                              │
│  │ Key: MuSig2(A, B)                    │                              │
│  │ Script: CSV(72) + B (refund)         │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│                                                                         │
│  PHASE 4: CREATE ADAPTOR SIGNATURES                                     │
│  ═══════════════════════════════════                                    │
│                                                                         │
│  For LTC claim tx (Alice will claim):                                   │
│    Bob creates adaptor sig encrypted with T                             │
│    adaptor_bob = Sign(ltc_claim_tx) + T                                │
│                                                                         │
│        ◄─── Bob sends adaptor_bob ────                                  │
│                                                                         │
│  For BTC claim tx (Bob will claim):                                     │
│    Alice creates normal partial sig                                     │
│    partial_alice = Sign(btc_claim_tx)                                  │
│                                                                         │
│        ─── Alice sends partial_alice ───►                               │
│                                                                         │
│                                                                         │
│  PHASE 5: CLAIM (SECRET REVELATION)                                     │
│  ═══════════════════════════════════                                    │
│                                                                         │
│  Alice claims LTC:                                                      │
│    valid_sig = adaptor_bob - T + Alice_partial                         │
│    (valid_sig reveals t because: valid = adaptor - t)                  │
│                                                                         │
│  Alice broadcasts LTC claim tx with valid_sig.                          │
│  The signature is now PUBLIC on LTC blockchain!                         │
│                                                                         │
│  Bob extracts t:                                                        │
│    t = adaptor_bob - valid_sig                                         │
│                                                                         │
│  Bob claims BTC:                                                        │
│    complete_sig = partial_alice + Bob_partial + t                      │
│                                                                         │
│  Bob broadcasts BTC claim tx.                                           │
│                                                                         │
│  SWAP COMPLETE - Both chains show normal Taproot spends!               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Flow 2: BTC ↔ ETH (HTLC)

**EVM chain requires smart contract**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    BTC ↔ ETH ATOMIC SWAP (HTLC)                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Alice (BTC Maker)              Bob (ETH Taker)                        │
│  ────────────────               ───────────────                        │
│                                                                         │
│  PHASE 1: SECRET GENERATION                                             │
│  ══════════════════════════                                             │
│                                                                         │
│  Alice generates:                                                       │
│    preimage = random(32 bytes)                                          │
│    hash = SHA256(preimage)                                              │
│                                                                         │
│        ────── Send hash to Bob ──────►                                  │
│                                                                         │
│                                                                         │
│  PHASE 2: ALICE LOCKS BTC (MAKER, LONGER TIMEOUT)                      │
│  ═══════════════════════════════════════════════                        │
│                                                                         │
│  Alice creates Taproot HTLC:                                            │
│  ┌───────────────────────────────────────┐                              │
│  │ BTC P2TR Output (0.1 BTC)             │                              │
│  │                                       │                              │
│  │ Key path: disabled (NUMS point)       │                              │
│  │ Script tree:                          │                              │
│  │   Leaf 1: SHA256 check + Bob's key   │                              │
│  │   Leaf 2: CSV(144) + Alice refund    │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│                                                                         │
│  PHASE 3: BOB VERIFIES AND LOCKS ETH (TAKER, SHORTER TIMEOUT)          │
│  ════════════════════════════════════════════════════════════           │
│                                                                         │
│  Bob verifies on BTC:                                                   │
│    ✓ Amount correct                                                     │
│    ✓ Hash matches agreed hash                                           │
│    ✓ Timeout is 144 blocks (~24h)                                       │
│    ✓ His pubkey can claim                                               │
│                                                                         │
│  Bob calls ETH smart contract:                                          │
│    AtomicSwap.initiate(                                                 │
│      swapId,                                                            │
│      alice_eth_address,                                                 │
│      hash,                 // Same hash!                                │
│      block.timestamp + 7200  // 2 hour timeout                          │
│    )                                                                    │
│                                                                         │
│  ┌───────────────────────────────────────┐                              │
│  │ ETH Smart Contract (10 ETH)           │                              │
│  │                                       │                              │
│  │ hashlock: SHA256(preimage)            │                              │
│  │ timelock: now + 2 hours              │                              │
│  │ participant: Alice                    │                              │
│  │ initiator: Bob                        │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│                                                                         │
│  PHASE 4: ALICE CLAIMS ETH (REVEALS PREIMAGE)                          │
│  ════════════════════════════════════════════                           │
│                                                                         │
│  Alice calls:                                                           │
│    AtomicSwap.complete(swapId, preimage)                               │
│                                                                         │
│  Contract:                                                              │
│    ✓ Verifies SHA256(preimage) == hashlock                             │
│    ✓ Transfers ETH to Alice                                             │
│    ✓ Emits SwapCompleted(swapId, preimage)                             │
│                                                                         │
│  PREIMAGE IS NOW PUBLIC IN ETH TRANSACTION CALLDATA!                   │
│                                                                         │
│                                                                         │
│  PHASE 5: BOB CLAIMS BTC                                                │
│  ═══════════════════════                                                │
│                                                                         │
│  Bob's monitor detects SwapCompleted event.                             │
│  Bob extracts preimage from event data.                                 │
│                                                                         │
│  Bob creates BTC claim tx:                                              │
│    Input: Alice's HTLC output                                           │
│    Witness: [bob_signature, preimage, htlc_script, control_block]      │
│                                                                         │
│  Bob broadcasts, receives 0.1 BTC.                                      │
│                                                                         │
│  SWAP COMPLETE!                                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Flow 3: BTC ↔ XMR (Adaptor Signatures)

**CryptoNote chain requires adaptor signatures**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  BTC ↔ XMR ATOMIC SWAP (ADAPTOR)                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  CONSTRAINT: Bob (BTC holder) MUST be maker!                           │
│  XMR has no scripting, so BTC provides the refund mechanism.           │
│                                                                         │
│  Bob (BTC Maker)                Alice (XMR Taker)                       │
│  ──────────────                 ────────────────                        │
│                                                                         │
│  PHASE 1: KEY GENERATION AND DLEQ PROOFS                               │
│  ═══════════════════════════════════════                                │
│                                                                         │
│  Bob generates:                 Alice generates:                        │
│    b_btc (secp256k1)             a_xmr (ed25519)                       │
│    b_xmr (ed25519)               a_btc (secp256k1)                     │
│                                                                         │
│  DLEQ proof: same b works       DLEQ proof: same a works               │
│  on both curves                 on both curves                          │
│                                                                         │
│        ─────── Exchange public keys + DLEQ proofs ──────►              │
│        ◄────── Exchange public keys + DLEQ proofs ──────               │
│                                                                         │
│  Both verify DLEQ proofs!                                               │
│                                                                         │
│                                                                         │
│  PHASE 2: COMPUTE SHARED XMR ADDRESS                                   │
│  ═══════════════════════════════════                                    │
│                                                                         │
│  Both compute:                                                          │
│    XMR_pubkey = A_xmr + B_xmr                                          │
│    XMR_address = Address(XMR_pubkey)                                   │
│                                                                         │
│  Private key is SPLIT:                                                  │
│    Full key = a_xmr + b_xmr                                            │
│    Alice knows: a_xmr                                                   │
│    Bob knows:   b_xmr                                                   │
│    Neither can spend alone!                                             │
│                                                                         │
│                                                                         │
│  PHASE 3: BOB LOCKS BTC (WITH ADAPTOR SIGNATURE)                       │
│  ═══════════════════════════════════════════════                        │
│                                                                         │
│  Bob creates BTC output:                                                │
│  ┌───────────────────────────────────────┐                              │
│  │ BTC P2TR Output (0.1 BTC)             │                              │
│  │                                       │                              │
│  │ Key: 2-of-2 MuSig2(A_btc, B_btc)     │                              │
│  │ Script: CSV(144) + B_btc (refund)    │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│  Bob creates ADAPTOR SIGNATURE for Alice to claim:                      │
│    adaptor_sig = partial_sig ENCRYPTED with a_btc                      │
│                                                                         │
│  Bob sends adaptor_sig to Alice.                                        │
│                                                                         │
│                                                                         │
│  PHASE 4: ALICE VERIFIES AND LOCKS XMR                                 │
│  ═════════════════════════════════════                                  │
│                                                                         │
│  Alice verifies:                                                        │
│    ✓ BTC is locked with correct amount                                  │
│    ✓ Adaptor signature is valid (encrypted with her key)               │
│    ✓ Timeout is reasonable (144 blocks)                                │
│    ✓ She can claim by revealing a_btc                                  │
│                                                                         │
│  Alice sends XMR to shared address:                                     │
│  ┌───────────────────────────────────────┐                              │
│  │ XMR Output (1 XMR)                    │                              │
│  │                                       │                              │
│  │ Address: XMR_address                  │                              │
│  │ Spendable by: whoever knows a+b      │                              │
│  │                                       │                              │
│  │ Alice knows: a_xmr                    │                              │
│  │ Bob knows:   b_xmr                    │                              │
│  └───────────────────────────────────────┘                              │
│                                                                         │
│  Alice waits for 10 XMR confirmations.                                  │
│                                                                         │
│                                                                         │
│  PHASE 5: ALICE CLAIMS BTC (REVEALS SECRET)                            │
│  ═══════════════════════════════════════════                            │
│                                                                         │
│  Alice decrypts adaptor signature:                                      │
│    valid_sig = adaptor_sig - a_btc                                     │
│                                                                         │
│  Alice broadcasts BTC claim tx with valid_sig.                          │
│                                                                         │
│  THE SIGNATURE IS NOW PUBLIC ON BTC BLOCKCHAIN!                        │
│                                                                         │
│                                                                         │
│  PHASE 6: BOB EXTRACTS SECRET AND CLAIMS XMR                           │
│  ═══════════════════════════════════════════                            │
│                                                                         │
│  Bob monitors BTC chain, sees Alice's claim tx.                         │
│  Bob extracts a_btc:                                                    │
│    a_btc = adaptor_sig - valid_sig                                     │
│                                                                         │
│  Bob converts a_btc (secp256k1) to a_xmr (ed25519)                     │
│  using the DLEQ relationship Alice proved earlier.                      │
│                                                                         │
│  Bob computes full XMR key:                                             │
│    full_key = a_xmr + b_xmr                                            │
│                                                                         │
│  Bob sweeps XMR to his wallet.                                          │
│                                                                         │
│  SWAP COMPLETE!                                                         │
│                                                                         │
│                                                                         │
│  REFUND SCENARIOS                                                       │
│  ════════════════                                                       │
│                                                                         │
│  If Alice never claims BTC (timeout):                                   │
│    Bob broadcasts refund tx after 144 blocks.                           │
│    Bob's refund tx reveals b_xmr (in adaptor form).                    │
│    Alice extracts b_xmr, computes full_key = a + b.                    │
│    Alice recovers her XMR.                                              │
│                                                                         │
│  If Alice never locks XMR:                                              │
│    Bob waits for timeout, refunds BTC.                                  │
│    No XMR was locked, no loss for Alice.                               │
│                                                                         │
│  If Bob disappears after locking BTC:                                   │
│    Alice never locks XMR.                                               │
│    Bob's BTC is stuck until timeout, then auto-refunded.               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 9. 2-of-2 Escrow Without Mediators

Following Bisq's evolution from 2-of-3 (with mediator) to 2-of-2 (trustless), Klingon-v2 implements a fully decentralized escrow system with no trusted third parties.

### Old vs New Approach

```
OLD BISQ (2-of-3)                    NEW APPROACH (2-of-2)
─────────────────                    ────────────────────

┌─────────────────────┐              ┌─────────────────────┐
│   2-of-3 Multisig   │              │   2-of-2 MuSig2     │
│                     │              │                     │
│  • Buyer            │              │  • Buyer            │
│  • Seller           │              │  • Seller           │
│  • Mediator ←───────┼── Trusted    │                     │
│                     │   Party!     │  No third party!    │
└─────────────────────┘              └─────────────────────┘
         │                                    │
         │                                    │
    Dispute?                             Dispute?
         │                                    │
         ▼                                    ▼
┌─────────────────────┐              ┌─────────────────────┐
│ Mediator + 1 party  │              │ Pre-signed timelock │
│ sign to resolve     │              │ burns to DAO        │
│                     │              │                     │
│ Trust mediator!     │              │ Trustless!          │
└─────────────────────┘              └─────────────────────┘
```

### The Key Innovation: Timelock Burn

The trick to making 2-of-2 work without a mediator: **Pre-sign a "nuclear option" transaction** that burns funds to the DAO if parties don't cooperate within a timeframe.

```
┌─────────────────────────────────────────────────────────────────┐
│                    2-of-2 ESCROW STRUCTURE                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Taproot Output (contains security deposits + trade amount)     │
│                                                                 │
│  ├── Key Path: MuSig2(Buyer, Seller)                           │
│  │             ↑                                                │
│  │        Happy path - both sign, instant release              │
│  │                                                              │
│  └── Script Path:                                               │
│      ├── Leaf 1: <7 days> CSV + DAO_PUBKEY                     │
│      │           ↑                                              │
│      │      Burn to DAO if no cooperation                      │
│      │                                                          │
│      ├── Leaf 2: <14 days> CSV + BUYER_PUBKEY                  │
│      │           ↑                                              │
│      │      Emergency buyer recovery                            │
│      │                                                          │
│      └── Leaf 3: <14 days> CSV + SELLER_PUBKEY                 │
│                  ↑                                              │
│             Emergency seller recovery                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Why This Works (Game Theory)

```
Both parties have skin in the game (security deposits).

If they don't cooperate:
  → Funds burn to DAO after 7 days
  → BOTH lose their deposits
  → Neither party "wins" by being stubborn

Rational behavior:
  → Cooperate and release funds properly
  → Even if you're angry, burning money hurts you too
  → Incentive to negotiate and settle

The DAO burn is the "mutually assured destruction" that
forces cooperation without needing a trusted mediator.
```

### Escrow Implementation

```go
// internal/escrow/escrow.go

package escrow

import (
    "github.com/btcsuite/btcd/btcec/v2"
    "github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
    "github.com/btcsuite/btcd/txscript"
)

type EscrowConfig struct {
    // Parties
    BuyerPubKey  *btcec.PublicKey
    SellerPubKey *btcec.PublicKey

    // DAO for burn destination
    DAOPubKey    *btcec.PublicKey

    // Timeouts (in blocks)
    DAOBurnTimeout      uint32  // 7 days ≈ 1008 blocks
    EmergencyTimeout    uint32  // 14 days ≈ 2016 blocks

    // Amounts
    TradeAmount         int64
    BuyerDeposit        int64
    SellerDeposit       int64
}

type Escrow struct {
    config       *EscrowConfig

    // MuSig2 aggregate key (key path)
    aggregateKey *musig2.AggregateKey

    // Taproot output key
    outputKey    *btcec.PublicKey

    // Script tree
    tapTree      *txscript.IndexedTapScriptTree

    // Pre-signed transactions
    burnTx       *wire.MsgTx  // Timelock burn to DAO
    buyerRefund  *wire.MsgTx  // Emergency buyer recovery
    sellerRefund *wire.MsgTx  // Emergency seller recovery
}

// CreateEscrow builds the 2-of-2 escrow with timelock fallbacks
func CreateEscrow(cfg *EscrowConfig) (*Escrow, error) {
    // 1. Create MuSig2 aggregate key for key path (happy path)
    allSigners := []*btcec.PublicKey{cfg.BuyerPubKey, cfg.SellerPubKey}

    aggKey, _, _, err := musig2.AggregateKeys(
        allSigners,
        true, // sort keys for determinism
    )
    if err != nil {
        return nil, fmt.Errorf("aggregate keys: %w", err)
    }

    // 2. Build script tree leaves

    // Leaf 1: DAO burn after 7 days
    daoBurnScript, err := buildTimelockScript(
        cfg.DAOBurnTimeout,
        cfg.DAOPubKey,
    )
    if err != nil {
        return nil, err
    }

    // Leaf 2: Buyer emergency after 14 days
    buyerEmergencyScript, err := buildTimelockScript(
        cfg.EmergencyTimeout,
        cfg.BuyerPubKey,
    )
    if err != nil {
        return nil, err
    }

    // Leaf 3: Seller emergency after 14 days
    sellerEmergencyScript, err := buildTimelockScript(
        cfg.EmergencyTimeout,
        cfg.SellerPubKey,
    )
    if err != nil {
        return nil, err
    }

    // 3. Build tap tree
    daoBurnLeaf := txscript.NewBaseTapLeaf(daoBurnScript)
    buyerLeaf := txscript.NewBaseTapLeaf(buyerEmergencyScript)
    sellerLeaf := txscript.NewBaseTapLeaf(sellerEmergencyScript)

    tapTree := txscript.AssembleTaprootScriptTree(
        daoBurnLeaf,
        buyerLeaf,
        sellerLeaf,
    )

    // 4. Compute output key (internal key + tree root)
    treeRoot := tapTree.RootNode.TapHash()
    outputKey := txscript.ComputeTaprootOutputKey(
        aggKey.PreTweakedKey,
        treeRoot[:],
    )

    return &Escrow{
        config:       cfg,
        aggregateKey: aggKey,
        outputKey:    outputKey,
        tapTree:      tapTree,
    }, nil
}

func buildTimelockScript(blocks uint32, pubkey *btcec.PublicKey) ([]byte, error) {
    builder := txscript.NewScriptBuilder()

    // <timeout> OP_CHECKSEQUENCEVERIFY OP_DROP <pubkey> OP_CHECKSIG
    builder.AddInt64(int64(blocks))
    builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)
    builder.AddOp(txscript.OP_DROP)
    builder.AddData(schnorr.SerializePubKey(pubkey))
    builder.AddOp(txscript.OP_CHECKSIG)

    return builder.Script()
}

// TotalAmount returns the total escrow amount
func (e *Escrow) TotalAmount() int64 {
    return e.config.TradeAmount + e.config.BuyerDeposit + e.config.SellerDeposit
}
```

### Cooperative Release (Happy Path)

```go
// internal/escrow/release.go

// CooperativeRelease - both parties agree on fund distribution
func (e *Escrow) CooperativeRelease(
    escrowOutpoint wire.OutPoint,
    buyerAmount    int64,  // What buyer receives
    sellerAmount   int64,  // What seller receives
    buyerAddr      string,
    sellerAddr     string,
) (*wire.MsgTx, error) {

    tx := wire.NewMsgTx(2)

    // Input: escrow
    tx.AddTxIn(&wire.TxIn{
        PreviousOutPoint: escrowOutpoint,
        Sequence:         wire.MaxTxInSequenceNum, // No timelock for key path
    })

    // Output 1: Buyer
    if buyerAmount > 0 {
        buyerScript, _ := addressToScript(buyerAddr)
        tx.AddTxOut(&wire.TxOut{
            Value:    buyerAmount,
            PkScript: buyerScript,
        })
    }

    // Output 2: Seller
    if sellerAmount > 0 {
        sellerScript, _ := addressToScript(sellerAddr)
        tx.AddTxOut(&wire.TxOut{
            Value:    sellerAmount,
            PkScript: sellerScript,
        })
    }

    return tx, nil
}

// MuSig2 signing session for cooperative release
type CooperativeSession struct {
    escrow        *Escrow
    releaseTx     *wire.MsgTx
    buyerSession  *musig2.Session
    sellerSession *musig2.Session
}

// Round 1: Exchange nonces
func (cs *CooperativeSession) ExchangeNonces() (
    buyerNonce, sellerNonce [musig2.PubNonceSize]byte,
) {
    buyerNonce = cs.buyerSession.PublicNonce()
    sellerNonce = cs.sellerSession.PublicNonce()

    // Register each other's nonces
    cs.buyerSession.RegisterPubNonce(sellerNonce)
    cs.sellerSession.RegisterPubNonce(buyerNonce)

    return buyerNonce, sellerNonce
}

// Round 2: Create partial signatures and combine
func (cs *CooperativeSession) Sign(sighash [32]byte) (*schnorr.Signature, error) {
    // Both sign
    buyerPartial, _ := cs.buyerSession.Sign(sighash)
    sellerPartial, _ := cs.sellerSession.Sign(sighash)

    // Combine
    cs.buyerSession.CombineSig(sellerPartial)

    return cs.buyerSession.FinalSig(), nil
}
```

### Complete Swap Flow with 2-of-2 Escrow

```
COMPLETE FLOW WITH SECURITY DEPOSITS
════════════════════════════════════

1. NEGOTIATION
   ─────────────
   Buyer and Seller agree on:
   - Trade amounts
   - Security deposit amounts (e.g., 15% of trade)
   - Timeout periods

2. ESCROW SETUP (parallel on both chains)
   ──────────────────────────────────────

   Chain A (Buyer's chain):
   ┌─────────────────────────────────────┐
   │ Buyer deposits:                     │
   │   • Security deposit (15%)          │
   │   • Trade amount                    │
   │                                     │
   │ Into 2-of-2 escrow                  │
   └─────────────────────────────────────┘

   Chain B (Seller's chain):
   ┌─────────────────────────────────────┐
   │ Seller deposits:                    │
   │   • Security deposit (15%)          │
   │   • Trade amount                    │
   │                                     │
   │ Into 2-of-2 escrow                  │
   └─────────────────────────────────────┘

3. PRE-SIGN BURN TRANSACTIONS
   ───────────────────────────
   Both parties sign burn txs for both escrows.
   These are held but not broadcast.

   If cooperation fails → either party can
   broadcast burn tx after timeout.

4. EXECUTE ATOMIC SWAP
   ────────────────────
   Now do the actual HTLC/adaptor swap:

   a) Buyer creates HTLC with hash
   b) Seller verifies, creates counter-HTLC
   c) Buyer claims (reveals preimage)
   d) Seller claims using preimage

   This is the standard atomic swap.

5. COOPERATIVE RELEASE
   ────────────────────
   After successful swap:

   Both sign to release escrows:
   - Buyer gets: their deposit back + seller's trade amount
   - Seller gets: their deposit back + buyer's trade amount

   (Deposits returned, trade amounts swapped)

6. IF SOMETHING GOES WRONG
   ────────────────────────

   Scenario A: Swap completed but party won't sign release
   → Other party waits for burn timeout
   → Burns to DAO (both lose deposits)
   → Atomic swap already happened, so trade amounts exchanged
   → Only deposits lost as "penalty" for non-cooperation

   Scenario B: Swap failed/timed out
   → Both parties should cooperatively refund
   → If one won't sign, burn timeout activates
   → Both lose deposits (mutual punishment)

   Scenario C: One party disappears
   → Other party waits for emergency timeout (14 days)
   → Can recover their own funds
```

### Escrow Configuration

```go
// internal/config/escrow.go

type EscrowConfig struct {
    // Deposit requirements
    MinDepositPercent     uint8   // Minimum security deposit (e.g., 15%)
    MaxDepositPercent     uint8   // Maximum (e.g., 50%)
    DefaultDepositPercent uint8   // Default (e.g., 15%)

    // Timeouts by chain (in blocks or seconds)
    DAOBurnTimeouts     map[string]uint32
    EmergencyTimeouts   map[string]uint32

    // DAO addresses for burn destination
    DAOAddresses        map[string]string
}

var DefaultEscrowConfig = EscrowConfig{
    MinDepositPercent:     15,
    MaxDepositPercent:     50,
    DefaultDepositPercent: 15,

    DAOBurnTimeouts: map[string]uint32{
        "BTC":   1008,   // ~7 days
        "LTC":   4032,   // ~7 days (2.5 min blocks)
        "ETH":   604800, // 7 days in seconds
        "BSC":   604800,
        "MATIC": 604800,
    },

    EmergencyTimeouts: map[string]uint32{
        "BTC":   2016,    // ~14 days
        "LTC":   8064,    // ~14 days
        "ETH":   1209600, // 14 days in seconds
        "BSC":   1209600,
        "MATIC": 1209600,
    },

    DAOAddresses: map[string]string{
        "BTC":   "bc1q...",  // DAO treasury
        "LTC":   "ltc1q...",
        "ETH":   "0x...",
        "BSC":   "0x...",
        "MATIC": "0x...",
    },
}
```

### P2P Protocol Messages for Escrow

```go
// internal/protocol/escrow_messages.go

// Escrow setup messages
type MsgEscrowProposal struct {
    SwapID          string
    BuyerDeposit    string  // Decimal amount
    SellerDeposit   string
    DAOBurnTimeout  uint32
    BuyerPubKey     []byte
}

type MsgEscrowAccept struct {
    SwapID        string
    SellerPubKey  []byte
    SellerEscrowAddr string  // Where buyer sends to seller's escrow
}

type MsgEscrowFunded struct {
    SwapID    string
    Chain     string
    TxID      string
    Outpoint  string
}

// Burn transaction exchange
type MsgBurnTxRequest struct {
    SwapID      string
    BurnTx      []byte  // Serialized transaction
}

type MsgBurnTxSignature struct {
    SwapID    string
    Chain     string  // Which escrow this is for
    Signature []byte
}

// Cooperative release
type MsgReleaseProposal struct {
    SwapID        string
    BuyerAmount   string
    SellerAmount  string
    ReleaseTx     []byte
}

type MsgReleaseNonce struct {
    SwapID  string
    Nonce   [66]byte  // MuSig2 public nonce
}

type MsgReleasePartialSig struct {
    SwapID      string
    PartialSig  []byte
}
```

### Summary: 2-of-2 vs 2-of-3

```
┌─────────────────────────────────────────────────────────────────────────┐
│              KLINGON-V2: 2-of-2 ESCROW WITHOUT MEDIATOR                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  HAPPY PATH (99% of trades)                                            │
│  ══════════════════════════                                            │
│                                                                         │
│    1. Both deposit into 2-of-2 escrows                                 │
│    2. Pre-sign burn transactions (held, not broadcast)                 │
│    3. Execute atomic swap (HTLC/adaptor)                               │
│    4. Both sign cooperative release                                     │
│    5. Everyone gets their funds + trade amounts                        │
│                                                                         │
│    Total time: ~1 hour                                                 │
│    No mediator needed!                                                 │
│                                                                         │
│                                                                         │
│  UNHAPPY PATH (rare)                                                   │
│  ══════════════════                                                    │
│                                                                         │
│    If one party refuses to sign release:                               │
│                                                                         │
│    Day 0-7:   Try to negotiate                                         │
│    Day 7:     DAO burn timeout activates                               │
│               → Funds burn to DAO treasury                             │
│               → BOTH parties lose deposits                             │
│               → No winner = no incentive to grief                      │
│                                                                         │
│    Day 14:    Emergency recovery available                             │
│               → Each party can recover their own chain's funds         │
│                                                                         │
│                                                                         │
│  WHY THIS WORKS                                                        │
│  ══════════════                                                        │
│                                                                         │
│    • No trusted mediator in the signing path                           │
│    • Game theory: being stubborn = losing money                        │
│    • DAO burn is "mutually assured destruction"                        │
│    • Rational actors always cooperate                                  │
│    • Irrational actors punish themselves equally                       │
│                                                                         │
│                                                                         │
│  COMPARISON                                                            │
│  ══════════════                                                        │
│                                                                         │
│    Old 2-of-3:                     New 2-of-2:                         │
│    ───────────                     ───────────                         │
│    • Mediator can collude          • No mediator                       │
│    • Mediator is single point      • Fully decentralized               │
│      of failure                    • Trustless                         │
│    • Must trust mediator           • Game theory enforces honesty      │
│      selection                                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 10. Claim Failure Protection

### The Problem

What happens if Alice claims on one chain (revealing the preimage) but Bob's claim transaction fails to broadcast on the other chain?

### Good News: Partially Mitigated by Design

The HTLC script requires **both** the preimage AND a valid signature:

```
OP_SHA256 <hash> OP_EQUALVERIFY <bob_pubkey> OP_CHECKSIG
                                 ↑
                        Only Bob can sign
```

Even if the preimage is public, **only Bob can claim** because only he has the private key.

### Real Risks

| Risk | Description |
|------|-------------|
| **Infrastructure fails** | Wallet crashes, node down, key inaccessible |
| **Network congestion** | Bob's tx stuck in mempool, can't get confirmed |
| **Timeout approaches** | Alice could refund if Bob doesn't claim in time |
| **Key loss** | Funds locked forever (until Alice's refund timeout) |

### Mitigation Strategies

#### 1. Pre-signed Claim Transactions

Bob creates and signs the claim transaction **before** Alice reveals the preimage:

```go
// internal/swap/presigned.go

type PreSignedClaim struct {
    PartialTx     *wire.MsgTx
    InputIndex    int
    HTLCScript    []byte
    ControlBlock  []byte
    Signature     []byte
}

// PrepareClaimTransaction creates a pre-signed tx ready for broadcast
func (b *Bob) PrepareClaimTransaction(
    htlcOutpoint wire.OutPoint,
    htlcAmount   int64,
    destAddress  string,
) (*PreSignedClaim, error) {

    tx := wire.NewMsgTx(2)

    tx.AddTxIn(&wire.TxIn{
        PreviousOutPoint: htlcOutpoint,
        Sequence:         wire.MaxTxInSequenceNum,
    })

    destScript, _ := addressToScript(destAddress)
    tx.AddTxOut(&wire.TxOut{
        Value:    htlcAmount - estimateFee(tx),
        PkScript: destScript,
    })

    // Sign NOW (before we know preimage)
    sighash := computeSighash(tx, 0, b.htlcScript, htlcAmount)
    signature, _ := schnorr.Sign(b.privateKey, sighash)

    return &PreSignedClaim{
        PartialTx:    tx,
        InputIndex:   0,
        HTLCScript:   b.htlcScript,
        ControlBlock: b.controlBlock,
        Signature:    signature.Serialize(),
    }, nil
}

// FinalizeAndBroadcast adds preimage and broadcasts
func (p *PreSignedClaim) FinalizeAndBroadcast(
    preimage []byte,
    client BTCClient,
) (string, error) {

    p.PartialTx.TxIn[p.InputIndex].Witness = wire.TxWitness{
        p.Signature,
        preimage,
        p.HTLCScript,
        p.ControlBlock,
    }

    return client.BroadcastTransaction(p.PartialTx)
}
```

#### 2. Watchtower Service

A third-party service monitors for preimage revelation and broadcasts on Bob's behalf:

```go
// internal/watchtower/watchtower.go

type WatchtowerRequest struct {
    // What to watch for
    WatchChain    string
    WatchTxID     string
    ExpectedHash  [32]byte

    // What to broadcast when preimage found
    ClaimChain    string
    PreSignedTx   *PreSignedClaim

    // Authentication
    BobPubKey     []byte
    Signature     []byte

    // Deadline
    Timeout       time.Time
}

func (w *Watchtower) watchAndClaim(req *WatchtowerRequest) {
    ctx, cancel := context.WithDeadline(context.Background(), req.Timeout)
    defer cancel()

    // Monitor for Alice's claim
    preimage, err := w.monitors[req.WatchChain].WatchForPreimage(
        ctx,
        req.WatchTxID,
        req.ExpectedHash,
    )
    if err != nil {
        return
    }

    // Immediately broadcast Bob's claim
    txid, err := req.PreSignedTx.FinalizeAndBroadcast(
        preimage,
        w.monitors[req.ClaimChain].Client(),
    )
    if err != nil {
        w.retryWithRBF(req, preimage)
        return
    }

    log.Info("watchtower claimed for user", "txid", txid)
}
```

#### 3. Redundant Broadcast Infrastructure

Bob broadcasts through multiple channels simultaneously:

```go
// internal/broadcast/redundant.go

var DefaultBTCEndpoints = []BroadcastEndpoint{
    {Name: "local-node", Type: "rpc", URL: "localhost:8332"},
    {Name: "blockstream", Type: "api", URL: "https://blockstream.info/api"},
    {Name: "mempool-space", Type: "api", URL: "https://mempool.space/api"},
}

func (r *RedundantBroadcaster) Broadcast(tx *wire.MsgTx) error {
    var wg sync.WaitGroup
    results := make(chan error, len(r.endpoints))

    // Broadcast to ALL endpoints simultaneously
    for _, endpoint := range r.endpoints {
        wg.Add(1)
        go func(ep BroadcastEndpoint) {
            defer wg.Done()
            results <- r.broadcastTo(ep, tx)
        }(endpoint)
    }

    // Wait for at least one success
    // ...
}
```

#### 4. Replace-By-Fee (RBF) Support

If Bob's transaction gets stuck, bump the fee:

```go
// internal/swap/rbf.go

func (b *Bob) BumpClaimFee(
    originalTx *wire.MsgTx,
    preimage []byte,
    newFeeRate int64,
) (*wire.MsgTx, error) {

    bumpedTx := originalTx.Copy()

    // Ensure RBF is signaled
    bumpedTx.TxIn[0].Sequence = wire.MaxTxInSequenceNum - 2

    // Calculate new fee
    txSize := estimateVSize(bumpedTx)
    newFee := txSize * newFeeRate

    // Reduce output value to increase fee
    bumpedTx.TxOut[0].Value = b.htlcAmount - newFee

    // Re-sign and update witness
    // ...

    return bumpedTx, nil
}
```

#### 5. EVM-Specific: Anyone Can Complete

For EVM contracts, design it so **anyone** can call `complete()` but funds always go to the designated recipient:

```solidity
function complete(bytes32 swapId, bytes32 preimage) external nonReentrant {
    Swap storage swap = swaps[swapId];

    require(sha256(abi.encodePacked(preimage)) == swap.hashlock, "Bad preimage");

    swap.completed = true;

    // Funds ALWAYS go to participant, regardless of msg.sender
    IERC20(swap.token).safeTransfer(swap.participant, swap.amount);
}
```

### Defense in Depth Summary

```
┌─────────────────────────────────────────────────────────────────┐
│                    CLAIM FAILURE PROTECTION                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Layer 1: Cryptographic                                         │
│  • Claim requires Bob's signature (can't be stolen)            │
│                                                                 │
│  Layer 2: Pre-signed Transactions                               │
│  • Sign claim tx BEFORE Alice reveals                          │
│  • Just add preimage and broadcast                              │
│                                                                 │
│  Layer 3: Redundant Infrastructure                              │
│  • Multiple broadcast endpoints                                 │
│  • Local node + public APIs                                     │
│                                                                 │
│  Layer 4: Watchtower Service                                    │
│  • Third-party monitors and broadcasts                          │
│  • Bob provides pre-signed tx                                   │
│                                                                 │
│  Layer 5: Fee Management                                        │
│  • RBF-enabled transactions                                     │
│  • Auto-bump loop                                               │
│                                                                 │
│  Layer 6: Safety Margins                                        │
│  • Don't claim near timeout                                     │
│  • Alert if time running out                                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 11. Security Considerations

### Critical Rules by Chain Type

#### UTXO Chains (BTC, LTC, BCH, DOGE)

1. **Never reuse nonces** in MuSig2 sessions
2. **Verify HTLC scripts** before locking funds
3. **Wait for confirmations** before proceeding (BTC: 3+, LTC: 6+)
4. **Use CSV over CLTV** for relative timelocks (more flexible)

#### EVM Chains (ETH, BSC, MATIC, ARB)

1. **ReentrancyGuard** on all state-changing functions
2. **Check-Effect-Interaction** pattern (update state before calls)
3. **SafeERC20** for token transfers (handles non-standard returns)
4. **Gas estimation with buffer** (network conditions vary)

#### CryptoNote (XMR)

1. **Never store both key shares** (a and b) together
2. **Verify DLEQ proofs** before accepting counterparty keys
3. **Wait for 10 XMR confirmations** (reorg protection)
4. **BTC holder must be maker** (for refund capability)

### Timeout Safety Margins

```go
// internal/config/timeouts.go

type TimeoutConfig struct {
    MakerBlocks      uint32
    TakerBlocks      uint32
    SafetyMargin     uint32  // Blocks before timeout to stop accepting
    MinConfirmations uint32
}

var ChainTimeouts = map[string]TimeoutConfig{
    "BTC": {
        MakerBlocks:      144,  // ~24 hours
        TakerBlocks:      72,   // ~12 hours
        SafetyMargin:     6,    // Stop 1 hour before timeout
        MinConfirmations: 3,
    },
    "LTC": {
        MakerBlocks:      576,  // ~24 hours (2.5 min blocks)
        TakerBlocks:      288,  // ~12 hours
        SafetyMargin:     24,   // Stop 1 hour before timeout
        MinConfirmations: 6,
    },
    "ETH": {
        MakerBlocks:      7200,  // 24 hours in seconds
        TakerBlocks:      3600,  // 1 hour in seconds (faster finality)
        SafetyMargin:     600,   // 10 min before timeout
        MinConfirmations: 12,
    },
    "XMR": {
        MakerBlocks:      0,     // No timelock on XMR
        TakerBlocks:      0,
        SafetyMargin:     0,
        MinConfirmations: 10,
    },
}
```

### Attack Vectors and Mitigations

| Attack | Description | Mitigation |
|--------|-------------|------------|
| **Free Option** | Maker waits to see price movement before claiming | Shorter taker timeout, security deposits |
| **Preimage Front-Running** | Attacker sees preimage in mempool, races to claim | Private mempool (Flashbots), fast claiming |
| **Timeout Race** | Claim and refund both valid near timeout | Safety margin, stop accepting before timeout |
| **Nonce Reuse** | Reusing MuSig2 nonces leaks private key | Generate fresh nonces per session |
| **DLEQ Forgery** | Fake proof linking wrong keys | Verify proofs before locking funds |

### Monitoring Requirements

```go
// internal/swap/monitor.go

type SwapMonitor struct {
    btcClient  BTCClient
    evmClients map[string]EVMClient
    xmrClient  XMRClient
}

// MonitorForClaim watches for counterparty claim and extracts secret
func (m *SwapMonitor) MonitorForClaim(ctx context.Context, swap *Swap) ([]byte, error) {
    switch swap.TakerChain {
    case "BTC", "LTC":
        return m.monitorUTXOClaim(ctx, swap)
    case "ETH", "BSC", "MATIC", "ARB":
        return m.monitorEVMClaim(ctx, swap)
    case "XMR":
        return m.monitorXMRClaim(ctx, swap)
    default:
        return nil, fmt.Errorf("unsupported chain: %s", swap.TakerChain)
    }
}

func (m *SwapMonitor) monitorUTXOClaim(ctx context.Context, swap *Swap) ([]byte, error) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-ticker.C:
            // Check if HTLC output was spent
            spendTx, err := m.btcClient.GetSpendingTransaction(swap.HTLCOutpoint)
            if err != nil {
                continue
            }
            if spendTx != nil {
                // Extract preimage from witness
                preimage := spendTx.TxIn[0].Witness[1]

                // Verify hash matches
                hash := sha256.Sum256(preimage)
                if bytes.Equal(hash[:], swap.Hashlock[:]) {
                    return preimage, nil
                }
            }
        }
    }
}

func (m *SwapMonitor) monitorEVMClaim(ctx context.Context, swap *Swap) ([]byte, error) {
    client := m.evmClients[swap.TakerChain]

    // Subscribe to SwapCompleted events
    query := ethereum.FilterQuery{
        Addresses: []common.Address{swap.ContractAddress},
        Topics: [][]common.Hash{
            {crypto.Keccak256Hash([]byte("SwapCompleted(bytes32,bytes32)"))},
            {swap.SwapID},
        },
    }

    logs := make(chan types.Log)
    sub, err := client.SubscribeFilterLogs(ctx, query, logs)
    if err != nil {
        return nil, err
    }
    defer sub.Unsubscribe()

    for {
        select {
        case err := <-sub.Err():
            return nil, err
        case log := <-logs:
            // Preimage is second topic (after event signature)
            preimage := log.Topics[2].Bytes()
            return preimage, nil
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

---

## 12. Implementation Architecture

### Module Structure

```
internal/
├── swap/
│   ├── swap.go           # Core swap types and state machine
│   ├── maker.go          # Maker-specific logic
│   ├── taker.go          # Taker-specific logic
│   ├── monitor.go        # Chain monitoring for claims
│   ├── presigned.go      # Pre-signed claim transactions
│   ├── rbf.go            # Replace-by-fee support
│   └── swap_test.go
├── escrow/
│   ├── escrow.go         # 2-of-2 escrow creation
│   ├── release.go        # Cooperative release logic
│   ├── burn.go           # DAO burn transaction
│   └── escrow_test.go
├── watchtower/
│   ├── watchtower.go     # Claim monitoring service
│   ├── broadcast.go      # Redundant broadcast
│   └── watchtower_test.go
├── blockchain/
│   ├── btc/
│   │   ├── client.go     # Bitcoin RPC client
│   │   ├── htlc.go       # HTLC script building
│   │   ├── musig2.go     # MuSig2 signing
│   │   ├── taproot.go    # Taproot address creation
│   │   └── btc_test.go
│   ├── ltc/
│   │   ├── client.go
│   │   └── htlc.go
│   ├── evm/
│   │   ├── client.go     # go-ethereum client
│   │   ├── contract.go   # Contract bindings
│   │   ├── gas.go        # Gas estimation
│   │   └── evm_test.go
│   └── xmr/
│       ├── client.go     # Monero wallet RPC
│       ├── adaptor.go    # Adaptor signature logic
│       ├── dleq.go       # Cross-curve proofs
│       └── xmr_test.go
├── protocol/
│   ├── messages.go       # P2P message types
│   ├── negotiation.go    # Swap negotiation protocol
│   └── protocol_test.go
└── config/
    ├── config.go         # Main config
    ├── chains.go         # Chain capabilities
    ├── timeouts.go       # Timeout configurations
    └── swap_matrix.go    # Supported pairs
```

### State Machine

```go
// internal/swap/swap.go

type SwapState uint8

const (
    StateCreated SwapState = iota
    StateKeysExchanged
    StateMakerLocked
    StateTakerLocked
    StateMakerClaimed      // Maker claimed, secret revealed
    StateTakerClaimed      // Taker claimed using revealed secret
    StateCompleted         // Both sides claimed
    StateMakerRefunded
    StateTakerRefunded
    StateFailed
)

type Swap struct {
    ID            string
    State         SwapState

    // Parties
    MakerChain    string
    MakerAmount   *big.Int
    MakerPubKey   []byte
    MakerTimeout  uint32

    TakerChain    string
    TakerAmount   *big.Int
    TakerPubKey   []byte
    TakerTimeout  uint32

    // Secret (only maker knows initially)
    Preimage      [32]byte
    Hashlock      [32]byte

    // Chain-specific data
    MakerTxID     string
    TakerTxID     string
    MakerOutpoint string
    TakerOutpoint string

    // For adaptor signature swaps
    AdaptorPoint  []byte
    DLEQProof     []byte

    // Timestamps
    CreatedAt     time.Time
    CompletedAt   time.Time
}

// StateTransitions defines valid state transitions
var StateTransitions = map[SwapState][]SwapState{
    StateCreated:       {StateKeysExchanged, StateFailed},
    StateKeysExchanged: {StateMakerLocked, StateFailed},
    StateMakerLocked:   {StateTakerLocked, StateMakerRefunded},
    StateTakerLocked:   {StateMakerClaimed, StateTakerRefunded},
    StateMakerClaimed:  {StateTakerClaimed},
    StateTakerClaimed:  {StateCompleted},
    StateMakerRefunded: {StateTakerRefunded, StateCompleted},
    StateTakerRefunded: {StateMakerRefunded, StateCompleted},
}

func (s *Swap) CanTransitionTo(newState SwapState) bool {
    validStates, ok := StateTransitions[s.State]
    if !ok {
        return false
    }
    for _, valid := range validStates {
        if valid == newState {
            return true
        }
    }
    return false
}
```

### P2P Protocol Messages

```go
// internal/protocol/messages.go

type MessageType uint8

const (
    MsgSwapRequest MessageType = iota
    MsgSwapAccept
    MsgSwapReject
    MsgKeyExchange
    MsgLockConfirm
    MsgClaimNotify
    MsgRefundNotify
)

// SwapRequest - Maker initiates swap
type SwapRequest struct {
    SwapID       string
    MakerChain   string
    MakerAmount  string  // Decimal string
    TakerChain   string
    TakerAmount  string
    MakerPubKey  []byte
    MakerTimeout uint32
    Hashlock     []byte  // For HTLC swaps
}

// KeyExchange - Exchange keys and proofs
type KeyExchange struct {
    SwapID      string
    PubKey      []byte
    AdaptorPub  []byte   // For MuSig2/adaptor swaps
    DLEQProof   []byte   // For XMR swaps
}

// LockConfirm - Confirm funds locked
type LockConfirm struct {
    SwapID    string
    Chain     string
    TxID      string
    Outpoint  string
    Amount    string
}

// ClaimNotify - Notify counterparty of claim
type ClaimNotify struct {
    SwapID   string
    Chain    string
    TxID     string
    Preimage []byte  // Secret revealed
}
```

---

## 13. Lessons from Bisq-MuSig Protocol

After reviewing the `bisq-musig` repository, here are critical insights and considerations we should incorporate:

### Key Innovation: Private Key Transfer (No Extra Transaction)

Bisq's MuSig2 protocol eliminates the need for a claim transaction in the happy path:

```
TRADITIONAL APPROACH (4 transactions):
1. Maker fee tx
2. Taker fee tx
3. Deposit tx (2-of-2 multisig)
4. Payout tx (requires both signatures)

BISQ MUSIG2 APPROACH (1 transaction):
1. DepositTx (outputs locked by aggregated keys P' and Q')

Happy path completion: Exchange private keys off-chain!
```

**How it works:**

```
Alice has: p_a (her secret)
Bob has:   p_b (his secret)

Aggregated key: P' = P_a + P_b (public, locks the output)
Aggregated secret: p' = p_a + p_b

When Alice sends p_a to Bob:
  → Bob computes p' = p_a + p_b
  → Bob now has FULL control of P'-locked output
  → No transaction needed!
  → Instant, private transfer of ownership
```

### Critical: Ephemeral Keys (NOT from HD Wallet)

```go
// WRONG - HD wallet keys leak information
func generateKeyFromHD(wallet *HDWallet, index uint32) *btcec.PrivateKey {
    return wallet.DeriveKey(index) // DANGEROUS!
}

// CORRECT - Random ephemeral keys
func generateEphemeralKey() (*btcec.PrivateKey, error) {
    // Generate truly random key, not derived from anything
    var seed [32]byte
    if _, err := rand.Read(seed[:]); err != nil {
        return nil, err
    }
    return btcec.PrivKeyFromBytes(seed[:]), nil
}
```

**Why this matters:**
- HD wallets derive keys from a master seed using a chaincode
- Revealing one derived key can leak information about other keys
- Ephemeral keys are independent - revealing them is safe

### Transaction Hierarchy: Warning → Redirect → Claim

Bisq uses a more sophisticated fallback structure than our simple "burn to DAO":

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    BISQ TRANSACTION HIERARCHY                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  DepositTx                                                              │
│  ─────────                                                              │
│  • 2 inputs (buyer + seller funds)                                     │
│  • 2 outputs: P' (seller deposit + trade) and Q' (buyer deposit)       │
│  • Scriptless - just aggregated keys                                   │
│                                                                         │
│       │                                                                 │
│       │ If happy path fails...                                         │
│       ▼                                                                 │
│                                                                         │
│  WarningTx (Alice's version)                                           │
│  ───────────────────────────                                           │
│  • Inputs: Both DepositTx outputs                                      │
│  • Output 0: P' (if Bob has key, he takes everything)                 │
│  • Output 1: Anchor (for CPFP fee bumping)                            │
│  • Purpose: Warn other party, give them chance to respond             │
│                                                                         │
│       │                                                                 │
│       │ If Bob doesn't respond in t1 (~10 days)                        │
│       ▼                                                                 │
│                                                                         │
│  RedirectTx                                                             │
│  ──────────                                                             │
│  • Input: WarningTx output                                             │
│  • Outputs: Multiple "Burning Men" (DAO contributors)                  │
│  • nSequence enforces t1 delay (1440 blocks)                          │
│  • Purpose: Escalate to DAO arbitration                                │
│                                                                         │
│       │                                                                 │
│       │ If RedirectTx not broadcast by t2 (~15 days)                   │
│       ▼                                                                 │
│                                                                         │
│  ClaimTx                                                                │
│  ────────                                                               │
│  • Input: WarningTx output (script path)                               │
│  • Output: Claimant's wallet                                           │
│  • nSequence enforces t2 delay (2160 blocks)                          │
│  • OP_CSV script spend (not keyspend)                                  │
│  • Purpose: Recovery if other party completely unresponsive            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### nSequence vs OP_CSV: Privacy Optimization

Bisq enforces timelocks via **nSequence** in pre-signed transactions, not OP_CSV scripts:

```go
// BISQ APPROACH: nSequence enforcement
// Both parties agree to only sign txs with specific nSequence values
// No script needed - timelock is in the transaction itself

type PreSignedTx struct {
    Tx           *wire.MsgTx
    RequiredSeq  uint32  // Enforced during signing, not by script
}

func (p *PreSignedTx) SetTimelock(blocks uint32) {
    // Set nSequence on the input
    // Other party won't sign if sequence is wrong
    p.Tx.TxIn[0].Sequence = blocks
}

// WHY THIS IS BETTER:
// 1. No visible script on chain
// 2. Transaction looks like any other Taproot spend
// 3. More private - can't identify as Bisq/Klingon trade
```

### SwapTx with Adaptor Signatures

The SwapTx provides atomic guarantee without revealing the secret until claim:

```go
// SwapTx: Seller can claim buyer's deposit
// BUT: Using SwapTx reveals seller's key share to buyer

type SwapTx struct {
    // Input: DepositTx seller payout (P' locked)
    // Output: Seller's wallet
    AdaptorSig    []byte  // Encrypted with seller's p_a
    BuyerPartial  []byte  // Buyer's partial signature
}

// When seller broadcasts SwapTx:
// 1. Must complete the adaptor signature with p_a
// 2. p_a becomes public on blockchain
// 3. Buyer extracts p_a from the signature
// 4. Buyer computes p' = p_a + p_b
// 5. Buyer now controls P' output

// This is atomic:
// - Seller gets buyer's deposit (via SwapTx)
// - Buyer gets seller's deposit + trade amount (via revealed key)
```

### 7-Message Protocol Structure

Bisq uses a specific message exchange pattern:

```
      Maker               Taker
        │                   │
        │         A         │  Pubkeys, fee ranges
        │◄──────────────────│
        │                   │
        │         B         │  Pubkeys, inputs, nonces
        │──────────────────►│
        │                   │
        │         C         │  Inputs, nonces, partial sigs
        │◄──────────────────│
        │                   │
        │         D         │  Partial sigs, adaptor sig, deposit sigs
        │──────────────────►│
        │                   │
        │     DepositTx     │  Published to blockchain
        │         ·         │
        │         ·         │
      Buyer              Seller
        │                   │
        │    E (mailbox)    │  Fiat sent confirmation
        │──────────────────►│
        │                   │
        │    F (mailbox)    │  Key share reveal (p_a)
        │◄──────────────────│
        │                   │
        │    G (mailbox)    │  Courtesy key share (q_b)
        │──────────────────►│
        │                   │

Messages A-D: Real-time P2P (must be online)
Messages E-G: Async mailbox (can be offline)
```

### Post-Trade Attack Vector (Watchtower Problem)

**Critical insight from Bisq docs:**

After happy path completion, **WarningTx is still valid** and can be broadcast:

```
Timeline:
─────────────────────────────────────────────────────────
Day 0: Trade completes, keys exchanged
Day 1-30: Funds sit in P' and Q' outputs
Day 31: Malicious Bob broadcasts WarningTx

If Alice is offline:
  → Bob waits for t1 timeout
  → Bob broadcasts RedirectTx (or waits for ClaimTx timeout)
  → Bob steals funds

If Alice is online:
  → Alice sees WarningTx
  → Alice uses her key p' to sweep P' output immediately
  → Bob loses his deposit (punishment)
```

**Mitigations:**

```go
// internal/watchtower/postrade.go

type PostTradeMonitor struct {
    completedTrades map[string]*CompletedTrade
    btcClient       BTCClient
}

type CompletedTrade struct {
    DepositTxID     string
    OurKeyShare     *btcec.PrivateKey
    TheirKeyShare   *btcec.PrivateKey  // Received after completion
    AggregatedKey   *btcec.PrivateKey  // p' = p_a + p_b
    WarningTxID     string             // Pre-computed for monitoring
}

func (m *PostTradeMonitor) WatchForAttack(trade *CompletedTrade) {
    go func() {
        for {
            tx, _ := m.btcClient.GetTransaction(trade.WarningTxID)
            if tx != nil && tx.Confirmations > 0 {
                // ATTACK DETECTED! Sweep immediately
                m.executeCounterAttack(trade)
                return
            }
            time.Sleep(10 * time.Second)
        }
    }()
}

func (m *PostTradeMonitor) executeCounterAttack(trade *CompletedTrade) {
    // Use aggregated key to sweep before attacker
    sweepTx := m.createSweepTx(trade)
    m.btcClient.BroadcastTransaction(sweepTx)
    log.Warn("POST-TRADE ATTACK DETECTED AND COUNTERED")
}
```

**User guidance:**
> "Withdraw funds to an external wallet within [claim TX timelock] to put them in safer storage"

### Anchor Outputs for CPFP Fee Bumping

Since all transactions are pre-signed, fee rates might be wrong by broadcast time:

```go
// Bisq solution: Add small anchor output that owner can spend
// When broadcasting WarningTx with outdated fee:
// 1. Create child tx spending anchor output
// 2. Child tx pays high fee
// 3. Miners see combined fee rate (CPFP)
// 4. Both txs confirm together

type AnchorOutput struct {
    Value     int64            // Small amount (546 sats)
    OwnerKey  *btcec.PublicKey // Only owner can spend
}
```

### Summary: What We Should Adopt

| Feature | Our Current | Bisq Approach | Recommendation |
|---------|-------------|---------------|----------------|
| Happy path | Release tx | Key transfer | **Adopt** - more private |
| Key generation | HD derived | Ephemeral | **Adopt** - critical security |
| Timelock enforcement | OP_CSV script | nSequence | **Adopt** - more private |
| Fee bumping | RBF | Anchor + CPFP | **Adopt** - more reliable |
| Fallback structure | Single burn | Warning→Redirect→Claim | **Consider** - more flexible |
| Post-trade monitoring | None | Watchtower | **Adopt** - critical security |

### Updated State Machine

```go
type SwapState uint8

const (
    // Setup phase (both online)
    StateCreated SwapState = iota
    StateRound1Complete     // Pubkeys exchanged
    StateRound2Complete     // Nonces exchanged
    StateRound3Complete     // Partial sigs exchanged
    StateRound4Complete     // Deposit sigs exchanged
    StateDepositBroadcast
    StateDepositConfirmed

    // Async phase (can be offline)
    StateFiatSent           // Buyer sent fiat (message E)
    StateKeyRevealed        // Seller sent key (message F)
    StateCourtesyClosed     // Buyer sent courtesy key (message G)
    StateCompleted

    // Fallback paths
    StateWarningBroadcast
    StateRedirectBroadcast  // Arbitration
    StateClaimBroadcast     // Recovery

    // Post-trade monitoring
    StatePostTradeWatch
    StateFundsSwept
)
```

---

## 14. JSON-RPC API Reference

The Klingon daemon exposes a JSON-RPC 2.0 API for controlling swaps, wallets, and node operations. All methods use underscore notation.

### Connection

- **HTTP POST**: `http://127.0.0.1:8080/` (mainnet) or `http://127.0.0.1:18080/` (testnet)
- **WebSocket**: `ws://127.0.0.1:8080/ws` for real-time events

### Wallet Methods

#### `wallet_create` - Create a new HD wallet

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_create",
  "params": {"password": "your-secure-password"},
  "id": 1
}'
```

Response:
```json
{
  "mnemonic": "abandon abandon abandon ... about",
  "message": "Wallet created. Save your mnemonic securely!"
}
```

#### `wallet_unlock` - Unlock an existing wallet

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_unlock",
  "params": {"password": "your-secure-password"},
  "id": 1
}'
```

#### `wallet_getAddress` - Get receiving address for a coin

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_getAddress",
  "params": {"symbol": "BTC"},
  "id": 1
}'
```

Response:
```json
{
  "address": "tb1q...",
  "symbol": "BTC"
}
```

#### `wallet_getBalance` - Get balance for an address

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_getBalance",
  "params": {
    "symbol": "BTC",
    "address": "tb1q..."
  },
  "id": 1
}'
```

Response:
```json
{
  "confirmed": 652282,
  "unconfirmed": 0,
  "total": 652282
}
```

#### `wallet_getUTXOs` - Get UTXOs for an address

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_getUTXOs",
  "params": {
    "symbol": "BTC",
    "address": "tb1q..."
  },
  "id": 1
}'
```

Response:
```json
{
  "utxos": [
    {
      "txid": "abc123...",
      "vout": 0,
      "amount": 652282,
      "confirmations": 6
    }
  ],
  "count": 1
}
```

#### `wallet_send` - Build, sign, and broadcast a transaction

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "wallet_send",
  "params": {
    "symbol": "BTC",
    "to": "tb1p...",
    "amount": 50000
  },
  "id": 1
}'
```

Optional parameters:
- `account` (uint32): HD account index (default: 0)
- `index` (uint32): HD address index (default: 0)

Response:
```json
{
  "txid": "def456..."
}
```

### Swap Methods

#### `swap_init` - Initialize a new atomic swap

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_init",
  "params": {
    "offer_coin": "BTC",
    "offer_amount": 50000,
    "want_coin": "LTC",
    "want_amount": 1000000,
    "role": "maker"
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "status": "created",
  "my_pubkey": "02abc...",
  "offer": {
    "coin": "BTC",
    "amount": 50000
  },
  "want": {
    "coin": "LTC",
    "amount": 1000000
  }
}
```

#### `swap_exchangeKeys` - Exchange public keys with counterparty

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_exchangeKeys",
  "params": {
    "swap_id": "swap-abc123",
    "counterparty_pubkey": "03def..."
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "status": "keys_exchanged",
  "aggregated_pubkey": "02agg...",
  "my_address": "tb1p...",
  "counterparty_address": "tltc1p..."
}
```

#### `swap_getAddress` - Get the Taproot escrow address for the swap

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_getAddress",
  "params": {
    "swap_id": "swap-abc123",
    "coin": "BTC"
  },
  "id": 1
}'
```

Response:
```json
{
  "address": "tb1p...",
  "coin": "BTC",
  "type": "taproot"
}
```

#### `swap_setFunding` - Record funding transaction details

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_setFunding",
  "params": {
    "swap_id": "swap-abc123",
    "coin": "BTC",
    "txid": "abc123...",
    "vout": 0,
    "amount": 50000
  },
  "id": 1
}'
```

#### `swap_checkFunding` - Check if escrow addresses are funded

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_checkFunding",
  "params": {
    "swap_id": "swap-abc123"
  },
  "id": 1
}'
```

Response:
```json
{
  "btc_funded": true,
  "btc_amount": 50000,
  "btc_confirmations": 3,
  "ltc_funded": true,
  "ltc_amount": 1000000,
  "ltc_confirmations": 6
}
```

#### `swap_exchangeNonce` - Exchange MuSig2 nonces

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_exchangeNonce",
  "params": {
    "swap_id": "swap-abc123",
    "counterparty_nonce": "04nonce..."
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "my_nonce": "04mynonce...",
  "status": "nonces_exchanged"
}
```

#### `swap_sign` - Exchange partial signatures

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_sign",
  "params": {
    "swap_id": "swap-abc123",
    "counterparty_partial_sig": "sig..."
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "my_partial_sig": "mysig...",
  "status": "signatures_exchanged"
}
```

#### `swap_redeem` - Finalize and broadcast redemption transactions

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_redeem",
  "params": {
    "swap_id": "swap-abc123"
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "btc_txid": "btctx...",
  "ltc_txid": "ltctx...",
  "status": "completed"
}
```

#### `swap_status` - Get current swap status

```bash
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc": "2.0",
  "method": "swap_status",
  "params": {
    "swap_id": "swap-abc123"
  },
  "id": 1
}'
```

Response:
```json
{
  "swap_id": "swap-abc123",
  "status": "funded",
  "role": "maker",
  "offer": {"coin": "BTC", "amount": 50000},
  "want": {"coin": "LTC", "amount": 1000000},
  "my_address": "tb1p...",
  "counterparty_address": "tltc1p...",
  "btc_funded": true,
  "ltc_funded": true
}
```

### Complete Swap Flow Example

```bash
# 1. Create wallets on both nodes
curl -X POST http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"wallet_create","params":{"password":"pass1"},"id":1}'
curl -X POST http://127.0.0.1:18081 -d '{"jsonrpc":"2.0","method":"wallet_create","params":{"password":"pass2"},"id":1}'

# 2. Get addresses and fund them (external)
curl -X POST http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"BTC"},"id":1}'
curl -X POST http://127.0.0.1:18081 -d '{"jsonrpc":"2.0","method":"wallet_getAddress","params":{"symbol":"LTC"},"id":1}'

# 3. Node 1 (Maker): Initialize swap offering 50k sats BTC for 1M litoshis LTC
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0",
  "method":"swap_init",
  "params":{"offer_coin":"BTC","offer_amount":50000,"want_coin":"LTC","want_amount":1000000,"role":"maker"},
  "id":1
}'
# Save swap_id and my_pubkey from response

# 4. Node 2 (Taker): Initialize matching swap
curl -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0",
  "method":"swap_init",
  "params":{"offer_coin":"LTC","offer_amount":1000000,"want_coin":"BTC","want_amount":50000,"role":"taker"},
  "id":1
}'
# Save swap_id and my_pubkey from response

# 5. Exchange public keys (both nodes)
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0",
  "method":"swap_exchangeKeys",
  "params":{"swap_id":"<maker-swap-id>","counterparty_pubkey":"<taker-pubkey>"},
  "id":1
}'
curl -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0",
  "method":"swap_exchangeKeys",
  "params":{"swap_id":"<taker-swap-id>","counterparty_pubkey":"<maker-pubkey>"},
  "id":1
}'

# 6. Get Taproot escrow addresses
curl -X POST http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"swap_getAddress","params":{"swap_id":"<maker-swap-id>","coin":"BTC"},"id":1}'
curl -X POST http://127.0.0.1:18081 -d '{"jsonrpc":"2.0","method":"swap_getAddress","params":{"swap_id":"<taker-swap-id>","coin":"LTC"},"id":1}'

# 7. Fund escrow addresses using wallet_send
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0",
  "method":"wallet_send",
  "params":{"symbol":"BTC","to":"<btc-escrow-address>","amount":50000},
  "id":1
}'
curl -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0",
  "method":"wallet_send",
  "params":{"symbol":"LTC","to":"<ltc-escrow-address>","amount":1000000},
  "id":1
}'

# 8. Wait for confirmations and verify funding
curl -X POST http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"swap_checkFunding","params":{"swap_id":"<maker-swap-id>"},"id":1}'

# 9. Exchange nonces
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0",
  "method":"swap_exchangeNonce",
  "params":{"swap_id":"<maker-swap-id>","counterparty_nonce":"<taker-nonce>"},
  "id":1
}'
curl -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0",
  "method":"swap_exchangeNonce",
  "params":{"swap_id":"<taker-swap-id>","counterparty_nonce":"<maker-nonce>"},
  "id":1
}'

# 10. Exchange partial signatures
curl -X POST http://127.0.0.1:18080 -d '{
  "jsonrpc":"2.0",
  "method":"swap_sign",
  "params":{"swap_id":"<maker-swap-id>","counterparty_partial_sig":"<taker-sig>"},
  "id":1
}'
curl -X POST http://127.0.0.1:18081 -d '{
  "jsonrpc":"2.0",
  "method":"swap_sign",
  "params":{"swap_id":"<taker-swap-id>","counterparty_partial_sig":"<maker-sig>"},
  "id":1
}'

# 11. Redeem (both nodes can broadcast)
curl -X POST http://127.0.0.1:18080 -d '{"jsonrpc":"2.0","method":"swap_redeem","params":{"swap_id":"<maker-swap-id>"},"id":1}'
curl -X POST http://127.0.0.1:18081 -d '{"jsonrpc":"2.0","method":"swap_redeem","params":{"swap_id":"<taker-swap-id>"},"id":1}'
```

### Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32600 | Invalid Request | Malformed JSON-RPC request |
| -32601 | Method not found | Unknown RPC method |
| -32602 | Invalid params | Missing or invalid parameters |
| -32603 | Internal error | Server-side error |
| -1 | Wallet not loaded | Wallet must be created/unlocked first |
| -2 | Insufficient funds | Not enough balance for transaction |
| -3 | Swap not found | Invalid swap ID |
| -4 | Invalid state | Operation not valid in current swap state |

---

## 15. References & Resources

### Academic Papers

- Gugger, J. (2020). "Bitcoin–Monero Cross-chain Atomic Swap" - https://eprint.iacr.org/2020/1126
- Fournier, L. (2019). "One-Time Verifiably Encrypted Signatures" (Adaptor Signatures)
- Noether, S. (2018). "MRL-0010: Discrete Logarithm Equality Across Groups"

### Implementation References

- **Bisq MuSig2 Protocol**: https://github.com/bisq-network/bisq-musig (Single-tx MuSig2 swaps)
- **COMIT xmr-btc-swap**: https://github.com/comit-network/xmr-btc-swap
- **AthanorLabs atomic-swap (Go)**: https://github.com/AthanorLabs/atomic-swap
- **AthanorLabs go-dleq**: https://github.com/AthanorLabs/go-dleq
- **Bisq**: https://bisq.network
- **Lightning Labs Taproot Assets**: https://github.com/lightninglabs/taproot-assets

### Go Libraries

```go
// Bitcoin
"github.com/btcsuite/btcd/btcec/v2"
"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
"github.com/btcsuite/btcd/txscript"

// Ethereum
"github.com/ethereum/go-ethereum"
"github.com/ethereum/go-ethereum/accounts/abi/bind"

// Monero
"github.com/monero-ecosystem/go-monero-rpc-client"

// Cross-curve DLEQ
"github.com/athanorlabs/go-dleq"
```

### Bitcoin Optech Resources

- MuSig2: https://bitcoinops.org/en/topics/musig/
- Adaptor Signatures: https://bitcoinops.org/en/topics/adaptor-signatures/
- PTLCs: https://bitcoinops.org/en/topics/ptlc/

---

*Comprehensive Atomic Swap Implementation Guide for Klingon-v2*
*Covering UTXO, EVM, and CryptoNote chains*
