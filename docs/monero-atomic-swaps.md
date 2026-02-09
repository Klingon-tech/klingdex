# Monero (XMR) Atomic Swaps Research

## Table of Contents

1. [Overview](#1-overview)
2. [The Monero Challenge: No Scripting](#2-the-monero-challenge-no-scripting)
3. [Adaptor Signatures Explained](#3-adaptor-signatures-explained)
4. [Cross-Group DLEQ Proofs](#4-cross-group-dleq-proofs)
5. [BTC-XMR Swap Protocol Flow](#5-btc-xmr-swap-protocol-flow)
6. [ETH-XMR Swap Considerations](#6-eth-xmr-swap-considerations)
7. [Implementation Libraries](#7-implementation-libraries)
8. [Security Considerations](#8-security-considerations)
9. [Code Examples](#9-code-examples)
10. [Resources](#10-resources)

---

## 1. Overview

Monero (XMR) presents unique challenges for atomic swaps due to its privacy-focused architecture:

- **No scripting language** - Unlike Bitcoin, Monero has no script capabilities
- **No timelocks** - Cannot implement traditional HTLCs
- **RingCT design** - Control of UTXOs is solely based on private key knowledge
- **Different elliptic curve** - Uses Ed25519 (edwards25519) instead of secp256k1

The solution: **Adaptor Signatures** (also called "Scriptless Scripts" or "One-Time Verifiable Encrypted Signatures")

### Key Advantages Over HTLCs

1. **Better privacy** - Transactions cannot be linked across chains (no shared hash)
2. **Lower on-chain footprint** - No scripts, just normal transactions
3. **Cheaper** - Reduced transaction size means lower fees
4. **Cross-curve compatible** - Works between ed25519 (Monero) and secp256k1 (Bitcoin)

---

## 2. The Monero Challenge: No Scripting

### Traditional HTLCs Don't Work

HTLCs (Hash Time-Locked Contracts) rely on scripting capabilities:

```
OP_IF
    OP_SHA256 <hash> OP_EQUALVERIFY <pubkey> OP_CHECKSIG
OP_ELSE
    <timeout> OP_CHECKSEQUENCEVERIFY OP_DROP <pubkey> OP_CHECKSIG
OP_ENDIF
```

Monero has none of these opcodes. It only has:
- Pure transactions
- Signatures to unlock UTXOs
- Private spend keys

### The Monero Approach

Instead of locking funds with scripts, Monero swaps use **split private keys**:

```
Monero Lock Address:
├── Public Key: P = P_a + P_b
└── Private Spend Key: s = s_a + s_b

Alice knows: s_a (her key share)
Bob knows:   s_b (his key share)

To spend: Need BOTH s_a AND s_b
```

The swap protocol ensures that claiming Bitcoin automatically reveals the key share needed to claim Monero, and vice versa.

---

## 3. Adaptor Signatures Explained

### What is an Adaptor Signature?

An adaptor signature is an encrypted signature that:
1. Can be proven to decrypt to a valid signature
2. Reveals a secret when combined with the final signature
3. Requires the secret to be adapted into a valid signature

### Mathematical Foundation

#### For Schnorr Signatures (Linear)

```
Normal Schnorr signature:
s = r + H(P, R, m) * x

Adaptor signature with secret t:
s' = r + t                    (pre-signature, not valid)
s = s' + t                    (valid signature)

When you see both s' and s, you can compute:
t = s - s'                    (secret revealed!)
```

#### For ECDSA Signatures (Non-Linear)

ECDSA is more complex due to the r^-1 term:

```
ECDSA signature: (r, s) where
r = (k*G).x
s = k^-1(H(m) + r*x) mod n

Adaptor signature uses multiplicative twist:
s' = k^-1(H(m) + r*x) * t^-1  (encrypted)
s = s' * t                     (valid signature)

Requires DLEQ proof to prove relationship
```

### How Adaptor Signatures Enable Atomic Swaps

```
Setup:
======
Alice generates secret: t (kept private)
Alice publishes: T = t*G (adaptor point)

Alice creates adaptor signature on Bitcoin transaction
Bob verifies the adaptor signature using T

Swap Execution:
===============
1. Bob publishes valid signature on Bitcoin (reveals t)
2. Alice extracts t from Bob's signature
3. Alice uses t to complete her signature on Monero
4. Both parties have their funds

Atomicity:
==========
- Bob can't claim Bitcoin without revealing t
- Alice needs t to claim Monero
- Either both claim or neither claims
```

### Code Example: Basic Adaptor Signature

```go
import (
    "crypto/sha256"
    "github.com/btcsuite/btcd/btcec/v2"
)

// Alice creates adaptor signature
func CreateAdaptorSignature(
    privKey *btcec.PrivateKey,
    adaptorPoint *btcec.PublicKey, // T = t*G
    msg []byte,
) (*btcec.ModNScalar, error) {
    // Generate nonce
    k := generateSecureNonce()
    R := btcec.ScalarBaseMultiply(k)

    // Create encrypted signature
    // s' = k^-1 * (H(m) + r*x) * t^-1
    // (simplified - actual implementation more complex)

    adaptorSig := encryptSignature(k, privKey, msg, adaptorPoint)
    return adaptorSig, nil
}

// Bob verifies adaptor signature
func VerifyAdaptorSignature(
    pubKey *btcec.PublicKey,
    adaptorPoint *btcec.PublicKey,
    adaptorSig *btcec.ModNScalar,
    msg []byte,
) bool {
    // Verify that adaptor sig will decrypt to valid sig
    // when combined with secret t
    return verifyEncryptedSignature(pubKey, adaptorPoint, adaptorSig, msg)
}

// Alice extracts secret from completed signature
func ExtractSecret(
    adaptorSig *btcec.ModNScalar,
    completedSig *btcec.Signature,
) *btcec.ModNScalar {
    // t = s / s'
    secret := completedSig.S.Div(adaptorSig)
    return secret
}
```

---

## 4. Cross-Group DLEQ Proofs

### The Cross-Curve Problem

```
Bitcoin uses:  secp256k1 curve (order n1)
Monero uses:   ed25519 curve   (order n2 = 2^252 + ...)

Challenge: Prove that the same secret x is used in both:
  Bitcoin: P1 = x*G1  (on secp256k1)
  Monero:  P2 = x*G2  (on ed25519)

Without revealing x!
```

### DLEQ (Discrete Logarithm Equality) Proof

DLEQ proves: "I know x such that P1 = x*G1 AND P2 = x*G2"

Specified in **MRL-0010** (Monero Research Lab paper)

### How DLEQ Works

The proof decomposes the secret into bits and proves each bit is valid on both curves:

```
1. Decompose x into 252 bits: x = b₀ + 2*b₁ + 4*b₂ + ... + 2²⁵¹*b₂₅₁
   where each bᵢ ∈ {0, 1}

2. For each bit bᵢ, create commitments on both curves:
   C1ᵢ = bᵢ*G1  (on secp256k1)
   C2ᵢ = bᵢ*G2  (on ed25519)

3. Use ring signatures to prove each bᵢ is either 0 or 1
   and is the same value on both curves

4. Verify that sum of commitments equals public keys:
   P1 = Σ(2ⁱ * C1ᵢ)
   P2 = Σ(2ⁱ * C2ᵢ)
```

### DLEQ Proof Libraries

#### Go Implementation (AthanorLabs)

```go
import "github.com/athanorlabs/go-dleq"

// Generate proof that same secret is used on both curves
func GenerateDLEQProof(
    secret *big.Int,
    secp256k1Pub *btcec.PublicKey,
    ed25519Pub *edwards25519.Point,
) (*dleq.Proof, error) {

    proof, err := dleq.NewProof(
        secret,
        secp256k1Pub,
        ed25519Pub,
    )

    return proof, err
}

// Verify DLEQ proof
func VerifyDLEQProof(
    proof *dleq.Proof,
    secp256k1Pub *btcec.PublicKey,
    ed25519Pub *edwards25519.Point,
) bool {

    return proof.Verify(secp256k1Pub, ed25519Pub)
}
```

#### Rust Implementation (COMIT/secp256kfun)

```rust
use secp256kfun::marker::*;
use sigma_fun::ext::dl_secp256k1_ed25519_eq::DLEqProof;

// Generate cross-curve DLEQ proof
fn generate_dleq_proof(
    secret: &Scalar,
    secp_point: &Point,
    ed25519_point: &ed25519_dalek::PublicKey,
) -> DLEqProof {

    DLEqProof::prove(
        secret,
        secp_point,
        ed25519_point,
    )
}

// Verify cross-curve DLEQ proof
fn verify_dleq_proof(
    proof: &DLEqProof,
    secp_point: &Point,
    ed25519_point: &ed25519_dalek::PublicKey,
) -> bool {

    proof.verify(secp_point, ed25519_point)
}
```

---

## 5. BTC-XMR Swap Protocol Flow

### Protocol Overview

Based on the research paper by Joël Gugger (h4sh3d), implemented by COMIT Network.

### Participants

- **Alice**: Has XMR, wants BTC
- **Bob**: Has BTC, wants XMR

### Key Constraint

**Only the BTC owner can initiate the swap** because Bitcoin has scripting capabilities (timelocks, hashlocks) while Monero doesn't. This asymmetry is fundamental to the protocol.

### Complete Protocol Flow

```
PHASE 1: SETUP & KEY EXCHANGE
══════════════════════════════════════════════════════════════

Alice and Bob exchange:
├── Public keys on both chains
├── DLEQ proofs (proving same secret on both curves)
├── Adaptor signatures
└── Zero-knowledge proofs of correct initialization

Alice generates Monero lock address:
  Public key: P_lock = P_alice + P_bob
  Private key: s_lock = s_alice + s_bob (split between them)

Neither Alice nor Bob can spend alone - need both key shares!


PHASE 2: BOB LOCKS BTC (24 hour timeout)
══════════════════════════════════════════════════════════════

Bob creates 2-of-2 multisig on Bitcoin:

BTC Blockchain:
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│   TX_lock: 1 BTC → 2-of-2 multisig(Alice, Bob)              │
│                                                             │
│   Spendable by:                                             │
│     • MuSig2(Alice, Bob) - cooperative close                │
│     • Alice after 24h (refund path)                         │
│     • Bob with cancel + refund after timelock               │
│                                                             │
└─────────────────────────────────────────────────────────────┘

Bob sends Alice an adaptor signature for the BTC spend
This adaptor is encrypted with Alice's Monero key share s_alice


PHASE 3: ALICE LOCKS XMR (no timeout needed!)
══════════════════════════════════════════════════════════════

Alice verifies Bob's BTC lock, then locks XMR:

XMR Blockchain:
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│   XMR_lock: 100 XMR → Address(P_alice + P_bob)              │
│                                                             │
│   Can be spent only with:                                   │
│     Private key = s_alice + s_bob                           │
│                                                             │
│   Alice knows: s_alice                                      │
│   Bob knows:   s_bob                                        │
│   Neither can spend alone!                                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘


PHASE 4: ALICE CLAIMS BTC (HAPPY PATH)
══════════════════════════════════════════════════════════════

Alice completes Bob's adaptor signature and claims BTC:

1. Alice combines:
   - Bob's adaptor signature
   - Her own signature
   - Her secret s_alice (THIS IS REVEALED ON-CHAIN!)

2. Alice broadcasts TX_redeem on Bitcoin

3. Bob monitors Bitcoin blockchain and extracts s_alice from the
   transaction witness data

Result: Alice has BTC, Bob now knows s_alice


PHASE 5: BOB CLAIMS XMR
══════════════════════════════════════════════════════════════

Bob now has both key shares:
  - s_alice (extracted from Alice's BTC claim)
  - s_bob (his own)

Bob computes full private key:
  s_lock = s_alice + s_bob

Bob spends XMR from the lock address to his own address.

Result: Bob has XMR

SWAP COMPLETE! Both parties have their funds.


REFUND PATH (IF ALICE DOESN'T CLAIM)
══════════════════════════════════════════════════════════════

If Alice never claims BTC:

After 24 hours:
  Bob publishes TX_cancel (invalidates adaptor signature)

Then Bob publishes TX_refund:
  Bob gets his BTC back
  Bob reveals s_bob in the process

Alice monitors and sees s_bob revealed:
  Alice computes s_lock = s_alice + s_bob
  Alice recovers her XMR

Result: Both parties get refunds, no loss.
```

### Detailed Transaction Structure

```
Bitcoin Side (Taproot):
=======================

TX_lock:
├── Input: Bob's funding UTXO
└── Output: P2TR(MuSig2(Alice, Bob))
    ├── Key path: Cooperative spend
    └── Script tree:
        ├── Leaf 1: <144 blocks> CSV <Alice> CHECKSIG  (Alice refund)
        └── Leaf 2: <72 blocks> CSV <Bob> CHECKSIG     (Bob cancel)

TX_redeem (Alice claims - reveals s_alice):
├── Input: TX_lock output
│   └── Witness: [Alice_sig, Bob_adaptor_completed_with_s_alice]
└── Output: Alice's address (minus fees)

TX_cancel (Bob cancels):
├── Input: TX_lock output (after 72 blocks)
│   └── Witness: [Bob_sig]
└── Output: Intermediate cancel state

TX_refund (Bob refunds - reveals s_bob):
├── Input: TX_cancel output
│   └── Witness: [Bob_sig_revealing_s_bob]
└── Output: Bob's address


Monero Side (No scripts!):
==========================

XMR_lock:
└── Output: Public key P = s_alice*G + s_bob*G
    Can only be spent with private key s = s_alice + s_bob

XMR_spend (Bob claims):
├── Input: XMR_lock output
│   └── Signature using s_lock = s_alice + s_bob
└── Output: Bob's Monero address

XMR_refund (Alice claims if swap cancelled):
├── Input: XMR_lock output
│   └── Signature using s_lock = s_alice + s_bob
└── Output: Alice's Monero address
```

### Why This Works: The Atomic Link

```
The atomicity comes from forced revelation of secrets:

1. Alice claims BTC → MUST reveal s_alice (part of signature)
   └──> Bob can now compute s_lock and claim XMR

2. Bob refunds BTC → MUST reveal s_bob (part of refund sig)
   └──> Alice can now compute s_lock and recover XMR

The blockchain enforces secret revelation!
It's cryptographically impossible to claim without revealing.
```

---

## 6. ETH-XMR Swap Considerations

### Key Differences from BTC-XMR

1. **EVM has different primitives** - Smart contracts instead of Bitcoin scripts
2. **Block times differ** - Ethereum ~12s vs Bitcoin ~10min
3. **Gas costs** - Must account for variable gas prices
4. **Adaptor signatures on ECDSA** - Ethereum uses ECDSA (secp256k1), not Schnorr

### ETH-XMR Protocol Adjustments

```
Ethereum Side:
==============

Smart Contract HTLC + Adaptor Signature Hybrid:

contract ETHXMRSwap {
    struct Swap {
        address initiator;
        uint256 amount;
        bytes32 hashedAdaptorPoint;  // H(T) where T = t*G
        uint256 timelock;
        bool claimed;
    }

    function lock(
        bytes32 hashedAdaptorPoint,
        uint256 timelock
    ) external payable {
        // Bob locks ETH
    }

    function claim(
        bytes adaptorSecret,  // secret t
        bytes signature
    ) external {
        // Alice claims with adaptor secret
        // Secret t is revealed in transaction data
    }

    function refund() external {
        // Bob refunds after timelock
    }
}

Monero Side:
============
(Same as BTC-XMR - split key approach)
```

### Timeline for ETH-XMR Swap

```
Hour 0                Hour 2              Hour 24
  │                      │                    │
  │                      │                    │
  ▼                      ▼                    ▼
  ├──────────────────────┼────────────────────┤
  │                      │                    │
  │   SWAP WINDOW        │                    │
  │                      │                    │
  │ Bob locks ETH        │ Alice can          │ Bob can
  │ Alice locks XMR      │ claim ETH          │ refund ETH
  │                      │ (reveals secret)   │ and recover XMR
  │                      │                    │
  └──────────────────────┴────────────────────┘

ETH timelock: 2 hours (fast finality)
XMR lock: No native timelock needed
```

### Gas Optimization for ETH-XMR

```solidity
// Optimized ETH-XMR atomic swap contract
contract OptimizedETHXMRSwap {

    // Packed storage
    struct Swap {
        address initiator;      // 20 bytes
        uint96 amount;          // 12 bytes (enough for ETH amounts)
        bytes32 hashedSecret;   // 32 bytes
        uint40 timelock;        // 5 bytes (timestamp)
        bool claimed;           // 1 byte
        bool refunded;          // 1 byte
    }

    mapping(bytes32 => Swap) public swaps;

    // Events for monitoring
    event Locked(bytes32 indexed swapId, address indexed initiator);
    event Claimed(bytes32 indexed swapId, bytes secret);
    event Refunded(bytes32 indexed swapId);

    function lock(
        bytes32 swapId,
        bytes32 hashedSecret,
        uint40 timelock
    ) external payable {
        require(swaps[swapId].initiator == address(0), "Exists");
        require(timelock > block.timestamp, "Invalid timelock");

        swaps[swapId] = Swap({
            initiator: msg.sender,
            amount: uint96(msg.value),
            hashedSecret: hashedSecret,
            timelock: timelock,
            claimed: false,
            refunded: false
        });

        emit Locked(swapId, msg.sender);
    }

    function claim(bytes32 swapId, bytes calldata secret) external {
        Swap storage swap = swaps[swapId];
        require(!swap.claimed && !swap.refunded, "Done");
        require(keccak256(secret) == swap.hashedSecret, "Invalid secret");

        swap.claimed = true;
        payable(msg.sender).transfer(swap.amount);

        emit Claimed(swapId, secret);
    }

    function refund(bytes32 swapId) external {
        Swap storage swap = swaps[swapId];
        require(!swap.claimed && !swap.refunded, "Done");
        require(block.timestamp >= swap.timelock, "Locked");
        require(msg.sender == swap.initiator, "Not initiator");

        swap.refunded = true;
        payable(swap.initiator).transfer(swap.amount);

        emit Refunded(swapId);
    }
}
```

---

## 7. Implementation Libraries

### Rust Implementations

#### 1. COMIT Network xmr-btc-swap

**Repository**: https://github.com/comit-network/xmr-btc-swap

```bash
# Production-ready BTC-XMR atomic swap implementation
# Requires Rust 1.74+

git clone https://github.com/comit-network/xmr-btc-swap.git
cd xmr-btc-swap
cargo build --release

# Run as swap provider (ASB - Automated Swap Backend)
./target/release/asb --config config.toml

# Run as swap taker (CLI)
./target/release/swap buy-xmr --seller <multiaddr>
```

**Key Features**:
- Full BTC-XMR swap protocol
- Libp2p for peer discovery (rendezvous protocol)
- Monero wallet integration
- Bitcoin wallet (electrum)
- Automatic retry and recovery

**Current Status**: Maintained by binarybaron, lescuer97, and delta1 (COMIT is no longer active)

#### 2. Farcaster Project

**Repository**: https://github.com/farcaster-project

```rust
// Farcaster node implementation
// Supports BTC-XMR swaps with electrum and monero nodes

use farcaster_core::swap::SwapRole;
use farcaster_node::SwapManager;

async fn run_swap() {
    let swap_manager = SwapManager::new(config).await;

    let swap = swap_manager.create_swap(
        SwapRole::Maker,
        "BTC",
        amount_btc,
        "XMR",
        amount_xmr,
    ).await;

    swap.execute().await?;
}
```

#### 3. Monero-Starknet Atomic Swap

**Repository**: https://github.com/omarespejel/monero-starknet-atomic-swap

Demonstrates cross-chain atomic swap between Monero and Starknet using:
- Adaptor signatures
- ED25519 MSM verification on Cairo
- DLEQ proofs

### Go Implementations

#### 1. AthanorLabs ETH-XMR Swap

**Repository**: https://github.com/AthanorLabs/atomic-swap

```go
import (
    "github.com/athanorlabs/atomic-swap/swap"
    "github.com/athanorlabs/atomic-swap/protocol/xmrmaker"
    "github.com/athanorlabs/atomic-swap/protocol/xmrtaker"
)

// Run as ETH-XMR maker (has XMR, wants ETH)
func runMaker() {
    backend := xmrmaker.NewSwapManager(config)

    offer := &swap.Offer{
        Provides:    swap.ProvidesXMR,
        MinimumAmount: eth.EtherToWei(0.1),
        MaximumAmount: eth.EtherToWei(10),
        ExchangeRate: calculateRate(),
    }

    backend.MakeOffer(offer)
    backend.Run()
}

// Run as ETH-XMR taker (has ETH, wants XMR)
func runTaker() {
    backend := xmrtaker.NewSwapManager(config)

    swap, err := backend.InitiateSwap(
        peerID,
        ethAmount,
        xmrAmount,
    )

    swap.Execute()
}
```

**Key Features**:
- JSON-RPC API
- WebSocket support
- Mainnet and testnet (Sepolia/Stagenet) support
- Development environment with two local nodes

#### 2. Go-DLEQ Library

**Repository**: https://github.com/AthanorLabs/go-dleq

```go
import (
    "github.com/athanorlabs/go-dleq"
    "crypto/rand"
)

// Generate DLEQ proof for cross-curve key
func generateCrossGroupProof() (*dleq.Proof, error) {
    // Secret scalar (same on both curves)
    secret := generateSecret()

    // Generate public keys on both curves
    secp256k1Pub := secret.ScalarBaseMult() // Bitcoin
    ed25519Pub := secret.ScalarBaseMult()    // Monero

    // Create DLEQ proof
    proof, err := dleq.NewProof(
        secret,
        secp256k1Pub,
        ed25519Pub,
    )

    return proof, err
}

// Verify DLEQ proof
func verifyCrossGroupProof(
    proof *dleq.Proof,
    secp256k1Pub *PublicKey,
    ed25519Pub *PublicKey,
) bool {
    return proof.Verify(secp256k1Pub, ed25519Pub)
}
```

---

## 8. Security Considerations

### Critical Security Requirements

#### 1. Interactive Protocol Constraint

**WARNING**: The BTC owner MUST remain online from funding until reclaim deadline!

```
Bob's Requirement (BTC owner):
═══════════════════════════════

If refund transaction is broadcast:
  Bob MUST spend the refund before timelock expires
  Otherwise: Bob loses BTC without getting XMR

This is an INTERACTIVE protocol - Bob cannot go offline!
```

#### 2. Confirmation Requirements

```go
const (
    // Bitcoin confirmations before proceeding
    BTCLockConfirmations = 3  // ~30 minutes

    // Monero confirmations (10 blocks recommended)
    XMRLockConfirmations = 10  // ~20 minutes

    // Safety margin before timelock expiration
    SafetyMarginBlocks = 10  // Don't wait until last second
)

func waitForConfirmations(chain Chain, txid string, required int) error {
    for {
        confirmations := getConfirmations(chain, txid)
        if confirmations >= required {
            return nil
        }
        time.Sleep(30 * time.Second)
    }
}
```

#### 3. Timelock Safety

```
Timelock Structure:
═══════════════════════════════════════════════

Bitcoin cancel timelock:    72 blocks  (~12 hours)
Bitcoin refund timelock:    144 blocks (~24 hours)
Safety margin:              10 blocks  (~100 minutes)

Alice MUST claim before: 72 - 10 = 62 blocks (~10 hours)
Bob MUST refund before: 144 - 10 = 134 blocks (~22 hours)

Never wait until the last block!
```

#### 4. Key Share Security

```go
// NEVER store both key shares together
type MoneroLockKeys struct {
    // Alice's private key share
    privateKeyShare_a *big.Int  // KEEP SECRET

    // Bob's public key share (safe to share)
    publicKeyShare_b *btcec.PublicKey

    // Combined public key (lock address)
    combinedPublicKey *btcec.PublicKey
}

// Only compute full key when ready to spend
func (k *MoneroLockKeys) computeFullPrivateKey(
    bobsRevealedShare *big.Int,
) *big.Int {
    // s_full = s_a + s_b
    fullKey := new(big.Int).Add(
        k.privateKeyShare_a,
        bobsRevealedShare,
    )

    // Immediately spend, then zero the key
    defer fullKey.Set(big.NewInt(0))

    return fullKey
}
```

#### 5. Adaptor Signature Nonce Safety

```go
// CRITICAL: Never reuse nonces in adaptor signatures
type AdaptorSession struct {
    sessionID   string
    nonce       *big.Int
    usedNonces  map[string]bool  // Track used nonces
}

func (s *AdaptorSession) generateNonce() (*big.Int, error) {
    for {
        nonce := generateSecureRandom()
        nonceStr := hex.EncodeToString(nonce.Bytes())

        if !s.usedNonces[nonceStr] {
            s.usedNonces[nonceStr] = true
            return nonce, nil
        }
    }
}

// NEVER persist nonce mid-session
// If session fails, regenerate completely
```

#### 6. Cross-Chain Race Conditions

```
Potential Attack: Bob tries to claim both BTC and XMR
═════════════════════════════════════════════════════

Defense:
1. Asymmetric timelocks ensure Alice claims first
2. Monitoring detects unexpected transactions
3. Automatic refund if protocol violated

Monitor both chains simultaneously:
```

```go
type SwapMonitor struct {
    btcWatcher *BTCWatcher
    xmrWatcher *XMRWatcher
    alertChan  chan Alert
}

func (m *SwapMonitor) detectRaceCondition(swap *Swap) {
    go m.btcWatcher.Watch(swap.btcTxID, func(event BTCEvent) {
        if event.Type == ClaimAttempt {
            m.alertChan <- Alert{
                Type: "BTC_CLAIM_DETECTED",
                Data: event,
            }
        }
    })

    go m.xmrWatcher.Watch(swap.xmrTxID, func(event XMREvent) {
        if event.Type == SpendAttempt {
            m.alertChan <- Alert{
                Type: "XMR_SPEND_DETECTED",
                Data: event,
            }
        }
    })
}
```

#### 7. Monero RingCT Privacy Leaks

```
Privacy Consideration:
══════════════════════

Atomic swap addresses on Monero are deterministic:
  P_lock = P_alice + P_bob

This could potentially be linked to the swap!

Mitigation:
- Use subaddresses where possible
- Don't reuse swap addresses
- Consider churning XMR after swap
```

### Attack Vectors & Mitigations

| Attack Vector | Risk Level | Mitigation |
|---------------|------------|------------|
| Bob goes offline during swap | High | Alice refunds BTC after timelock |
| Alice claims BTC but corrupted data | Medium | Bob extracts s_alice from blockchain |
| DLEQ proof forgery | Low | Verify proof cryptographically |
| Timelock manipulation | Low | Use block height, not timestamps |
| Front-running on Ethereum | Medium | Use commit-reveal or private mempool |
| Monero key share leak | Critical | Never store both shares together |
| Reused nonces in adaptor sigs | Critical | Generate fresh nonces per session |

---

## 9. Code Examples

### Complete BTC-XMR Swap Implementation (Go)

```go
package xmrswap

import (
    "crypto/sha256"
    "math/big"

    "github.com/btcsuite/btcd/btcec/v2"
    "github.com/athanorlabs/go-dleq"
)

// Participant represents a swap party
type Participant struct {
    // Bitcoin keys
    btcPrivKey *btcec.PrivateKey
    btcPubKey  *btcec.PublicKey

    // Monero key share
    xmrKeyShare *big.Int
    xmrPubShare *btcec.PublicKey  // ed25519 point

    // Swap state
    role SwapRole
}

type SwapRole int

const (
    RoleAlice SwapRole = iota  // Has XMR, wants BTC
    RoleBob                     // Has BTC, wants XMR
)

// Phase1: Key Exchange and DLEQ Proof
func (p *Participant) Phase1_ExchangeKeys() (*Phase1Data, error) {
    // Generate Monero key share
    p.xmrKeyShare = generateSecureScalar()
    p.xmrPubShare = scalarMult(p.xmrKeyShare, ed25519Generator)

    // Generate DLEQ proof (same secret on both curves)
    proof, err := dleq.NewProof(
        p.xmrKeyShare,
        p.btcPubKey,     // secp256k1
        p.xmrPubShare,   // ed25519
    )
    if err != nil {
        return nil, err
    }

    return &Phase1Data{
        BTCPubKey:    p.btcPubKey.SerializeCompressed(),
        XMRPubShare:  p.xmrPubShare.Serialize(),
        DLEQProof:    proof.Serialize(),
    }, nil
}

// Phase2: Bob Locks BTC
func (p *Participant) Phase2_BobLocksBTC(
    aliceData *Phase1Data,
    amount int64,
) (*BTCLockTx, error) {

    if p.role != RoleBob {
        return nil, errors.New("only Bob can lock BTC")
    }

    // Verify Alice's DLEQ proof
    aliceBTCPub := parsePublicKey(aliceData.BTCPubKey)
    aliceXMRPub := parseEd25519Point(aliceData.XMRPubShare)
    proof := parseDLEQProof(aliceData.DLEQProof)

    if !proof.Verify(aliceBTCPub, aliceXMRPub) {
        return nil, errors.New("invalid DLEQ proof from Alice")
    }

    // Create 2-of-2 multisig with adaptor signature
    multisigAddr := createMuSig2Address(p.btcPubKey, aliceBTCPub)

    // Create adaptor signature encrypted with Alice's XMR key share
    adaptorSig := p.createAdaptorSignature(aliceXMRPub)

    // Build and broadcast lock transaction
    lockTx := createBTCLockTransaction(
        p.btcPrivKey,
        multisigAddr,
        amount,
    )

    return &BTCLockTx{
        Transaction:  lockTx,
        AdaptorSig:   adaptorSig,
        MultisigAddr: multisigAddr,
    }, nil
}

// Phase3: Alice Locks XMR
func (p *Participant) Phase3_AliceLocksXMR(
    bobData *Phase1Data,
    bobBTCLock *BTCLockTx,
    amount uint64,
) (*XMRLockTx, error) {

    if p.role != RoleAlice {
        return nil, errors.New("only Alice can lock XMR")
    }

    // Verify Bob's BTC lock is valid
    if !verifyBTCLock(bobBTCLock) {
        return nil, errors.New("invalid BTC lock")
    }

    // Verify Bob's adaptor signature
    bobBTCPub := parsePublicKey(bobData.BTCPubKey)
    bobXMRPub := parseEd25519Point(bobData.XMRPubShare)

    if !verifyAdaptorSignature(bobBTCLock.AdaptorSig, bobXMRPub) {
        return nil, errors.New("invalid adaptor signature")
    }

    // Create Monero lock address (P = P_alice + P_bob)
    combinedPubKey := addEd25519Points(p.xmrPubShare, bobXMRPub)
    lockAddress := pubKeyToMoneroAddress(combinedPubKey)

    // Create and broadcast XMR lock transaction
    xmrLockTx := createXMRTransaction(
        p.xmrKeyShare,
        lockAddress,
        amount,
    )

    return &XMRLockTx{
        Transaction:   xmrLockTx,
        LockAddress:   lockAddress,
        CombinedPubKey: combinedPubKey,
    }, nil
}

// Phase4: Alice Claims BTC (reveals s_alice)
func (p *Participant) Phase4_AliceClaimsBTC(
    bobBTCLock *BTCLockTx,
    adaptorSig *AdaptorSignature,
) (*BTCClaimTx, error) {

    if p.role != RoleAlice {
        return nil, errors.New("only Alice can claim BTC")
    }

    // Complete adaptor signature using Alice's XMR key share
    // This reveals s_alice on-chain!
    completedSig := completeAdaptorSignature(
        adaptorSig,
        p.xmrKeyShare,  // This will be public!
    )

    // Build claim transaction
    claimTx := createBTCClaimTransaction(
        bobBTCLock.MultisigAddr,
        p.btcPubKey,
        completedSig,
    )

    // Broadcast (reveals s_alice to Bob)
    return &BTCClaimTx{
        Transaction: claimTx,
        RevealedSecret: p.xmrKeyShare,  // Now public
    }, nil
}

// Phase5: Bob Extracts Secret and Claims XMR
func (p *Participant) Phase5_BobClaimsXMR(
    aliceBTCClaim *BTCClaimTx,
    xmrLockAddr string,
) (*XMRClaimTx, error) {

    if p.role != RoleBob {
        return nil, errors.New("only Bob can claim XMR")
    }

    // Extract Alice's key share from BTC transaction
    s_alice := extractSecretFromWitness(aliceBTCClaim.Transaction)

    // Compute full Monero private key
    s_full := new(big.Int).Add(p.xmrKeyShare, s_alice)
    defer s_full.Set(big.NewInt(0))  // Zero after use

    // Create XMR claim transaction
    xmrClaimTx := createXMRTransaction(
        s_full,
        p.getXMRAddress(),
        getXMRBalance(xmrLockAddr),
    )

    return &XMRClaimTx{
        Transaction: xmrClaimTx,
    }, nil
}

// Refund path: Bob refunds BTC (reveals s_bob)
func (p *Participant) RefundBTC(
    btcLock *BTCLockTx,
    timeout uint32,
) (*BTCRefundTx, error) {

    if p.role != RoleBob {
        return nil, errors.New("only Bob can refund")
    }

    // Wait for timelock
    if !timelockExpired(timeout) {
        return nil, errors.New("timelock not expired")
    }

    // Create refund transaction (reveals s_bob)
    refundTx := createBTCRefundTransaction(
        btcLock,
        p.btcPubKey,
        p.xmrKeyShare,  // Reveals s_bob
    )

    return &BTCRefundTx{
        Transaction: refundTx,
        RevealedSecret: p.xmrKeyShare,
    }, nil
}

// Refund path: Alice recovers XMR
func (p *Participant) RefundXMR(
    bobRefund *BTCRefundTx,
    xmrLockAddr string,
) (*XMRRefundTx, error) {

    if p.role != RoleAlice {
        return nil, errors.New("only Alice can refund XMR")
    }

    // Extract Bob's revealed secret
    s_bob := extractSecretFromWitness(bobRefund.Transaction)

    // Compute full Monero private key
    s_full := new(big.Int).Add(p.xmrKeyShare, s_bob)
    defer s_full.Set(big.NewInt(0))

    // Recover XMR
    xmrRefundTx := createXMRTransaction(
        s_full,
        p.getXMRAddress(),
        getXMRBalance(xmrLockAddr),
    )

    return &XMRRefundTx{
        Transaction: xmrRefundTx,
    }, nil
}

// Helper: Create adaptor signature
func (p *Participant) createAdaptorSignature(
    recipientXMRPubKey *btcec.PublicKey,
) *AdaptorSignature {

    // Create signature encrypted with recipient's XMR key
    nonce := generateSecureNonce()

    adaptorSig := encryptSignature(
        p.btcPrivKey,
        nonce,
        recipientXMRPubKey,
    )

    return adaptorSig
}

// Helper: Verify adaptor signature
func verifyAdaptorSignature(
    sig *AdaptorSignature,
    adaptorPoint *btcec.PublicKey,
) bool {
    // Verify that signature will be valid when decrypted
    return sig.Verify(adaptorPoint)
}

// Helper: Complete adaptor signature
func completeAdaptorSignature(
    adaptorSig *AdaptorSignature,
    secret *big.Int,
) *btcec.Signature {
    // Decrypt adaptor signature using secret
    return adaptorSig.Complete(secret)
}

// Helper: Extract secret from transaction witness
func extractSecretFromWitness(tx *BTCTransaction) *big.Int {
    // Parse witness data to extract revealed secret
    witness := tx.Inputs[0].Witness
    secret := parseScalar(witness[1])  // Secret is in witness[1]
    return secret
}
```

### Monitoring and Recovery

```go
package monitor

import (
    "context"
    "time"
)

// SwapMonitor watches both chains for swap progress
type SwapMonitor struct {
    btcClient *BTCClient
    xmrClient *XMRClient
    swap      *Swap
    alerts    chan Alert
}

func NewSwapMonitor(swap *Swap) *SwapMonitor {
    return &SwapMonitor{
        btcClient: NewBTCClient(),
        xmrClient: NewXMRClient(),
        swap:      swap,
        alerts:    make(chan Alert, 10),
    }
}

func (m *SwapMonitor) MonitorSwap(ctx context.Context) error {
    // Monitor BTC chain
    go m.monitorBTC(ctx)

    // Monitor XMR chain
    go m.monitorXMR(ctx)

    // Handle alerts
    for {
        select {
        case alert := <-m.alerts:
            m.handleAlert(alert)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (m *SwapMonitor) monitorBTC(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // Check BTC lock confirmations
            confirmations := m.btcClient.GetConfirmations(
                m.swap.BTCLockTxID,
            )

            if confirmations >= RequiredBTCConfirmations {
                m.alerts <- Alert{
                    Type: "BTC_LOCK_CONFIRMED",
                    Data: confirmations,
                }
            }

            // Check for claim transaction
            claimTx := m.btcClient.GetSpendingTx(
                m.swap.BTCLockTxID,
            )

            if claimTx != nil {
                secret := extractSecretFromWitness(claimTx)
                m.alerts <- Alert{
                    Type: "BTC_CLAIMED",
                    Data: secret,
                }
            }

            // Check timelock status
            if m.timelockNearExpiry() {
                m.alerts <- Alert{
                    Type: "TIMELOCK_WARNING",
                    Data: m.getRemainingBlocks(),
                }
            }

        case <-ctx.Done():
            return
        }
    }
}

func (m *SwapMonitor) monitorXMR(ctx context.Context) {
    ticker := time.NewTicker(20 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // Check XMR lock confirmations
            confirmations := m.xmrClient.GetConfirmations(
                m.swap.XMRLockTxID,
            )

            if confirmations >= RequiredXMRConfirmations {
                m.alerts <- Alert{
                    Type: "XMR_LOCK_CONFIRMED",
                    Data: confirmations,
                }
            }

            // Check if XMR was spent
            spent := m.xmrClient.IsSpent(m.swap.XMRLockAddress)

            if spent {
                m.alerts <- Alert{
                    Type: "XMR_CLAIMED",
                    Data: nil,
                }
            }

        case <-ctx.Done():
            return
        }
    }
}

func (m *SwapMonitor) handleAlert(alert Alert) {
    switch alert.Type {
    case "BTC_LOCK_CONFIRMED":
        log.Info("BTC lock confirmed, proceeding to XMR lock")

    case "XMR_LOCK_CONFIRMED":
        log.Info("XMR lock confirmed, ready to claim")

    case "BTC_CLAIMED":
        secret := alert.Data.(*big.Int)
        log.Info("BTC claimed, extracting secret", "secret", secret)
        // Use secret to claim XMR

    case "XMR_CLAIMED":
        log.Info("XMR claimed, swap complete")

    case "TIMELOCK_WARNING":
        blocks := alert.Data.(int)
        log.Warn("Timelock near expiry!", "remaining_blocks", blocks)
        // Initiate refund if necessary
    }
}
```

---

## 10. Resources

### Academic Papers

- **Bitcoin–Monero Cross-chain Atomic Swap** by Joël Gugger (h4sh3d)
  - https://eprint.iacr.org/2020/1126.pdf
  - The foundational paper describing BTC-XMR atomic swaps

- **Atomic Swaps between Bitcoin and Monero** (Extended Version)
  - https://arxiv.org/abs/2101.12332
  - https://ar5iv.labs.arxiv.org/html/2101.12332

- **MRL-0010: Discrete Logarithm Equality Across Groups** by Sarang Noether
  - https://web.getmonero.org/resources/research-lab/pubs/MRL-0010.pdf
  - Monero Research Lab specification for DLEQ proofs

- **One-Time Verifiably Encrypted Signatures (Adaptor Signatures)** by Lloyd Fournier
  - Foundation for adaptor signature schemes

### Implementation Repositories

#### Rust

- **COMIT xmr-btc-swap**: https://github.com/comit-network/xmr-btc-swap
  - Production BTC-XMR swap implementation

- **Farcaster Project**: https://github.com/farcaster-project
  - Alternative BTC-XMR implementation

- **secp256kfun DLEQ**: https://github.com/LLFourn/secp256kfun
  - Cross-curve DLEQ proof library

- **Monero-Starknet Swap**: https://github.com/omarespejel/monero-starknet-atomic-swap
  - XMR-Starknet atomic swap proof of concept

#### Go

- **AthanorLabs atomic-swap**: https://github.com/AthanorLabs/atomic-swap
  - ETH-XMR atomic swap implementation

- **AthanorLabs go-dleq**: https://github.com/AthanorLabs/go-dleq
  - Go implementation of cross-group DLEQ proofs

- **COMIT cross-curve-dleq**: https://github.com/comit-network/cross-curve-dleq
  - Proof of concept DLEQ for secp256k1 and ed25519

### Documentation & Guides

- **Bitcoin Optech - Adaptor Signatures**: https://bitcoinops.org/en/topics/adaptor-signatures/
- **Conduition's Adaptor Signature Tutorial**: https://conduition.io/scriptless/adaptorsigs/
- **Bitcoin-S Adaptor Sig Docs**: https://github.com/bitcoin-s/bitcoin-s/blob/master/docs/crypto/adaptor-signatures.md
- **Tari RFC-0241 (XMR Atomic Swap)**: https://rfc.tari.com/RFC-0241_AtomicSwapXMR
- **Crypto Garage Blog Series**:
  - https://medium.com/crypto-garage/adaptor-signature-schnorr-signature-and-ecdsa-da0663c2adc4
  - https://medium.com/crypto-garage/adaptor-signature-on-schnorr-cross-chain-atomic-swaps-3f41c8fb221b

### Community Resources

- **Monero Atomic Swaps Announcement**: https://www.getmonero.org/2021/08/20/atomic-swaps.html
- **COMIT Project Updates**: https://comit.network/blog/
- **Monero CCS Proposals**:
  - h4sh3d Research: https://ccs.getmonero.org/proposals/h4sh3d-atomic-swap-research.html
  - Implementation Funding: https://ccs.getmonero.org/proposals/h4sh3d-atomic-swap-implementation.html
  - ETH-XMR Development: https://ccs.getmonero.org/proposals/noot-eth-xmr-atomic-swap.html

### Blog Posts & Tutorials

- **Bitlayer - Adaptor Signatures and Cross-Chain Atomic Swaps**:
  https://blog.bitlayer.org/Adaptor_Signatures_and_Its_Application_to_Cross-Chain_Atomic_Swaps/

- **Particl - HTLC vs Adaptor Signature Swaps**:
  https://particl.news/atomic-swap-style-showdown/

- **LocalMonero - How Atomic Swaps Will Work in Monero**:
  https://localmonero.co/knowledge/monero-atomic-swaps

### Tools & Services

- **xmrswap.me**: Bitcoin to Monero atomic swap provider
- **UnstoppableSwap**: GUI for COMIT's xmr-btc-swap
- **Samourai Wallet**: Integrated BTC-XMR atomic swaps (privacy-focused)

---

## Summary

Monero atomic swaps are now a reality, enabling trustless exchange between XMR and BTC/ETH without centralized exchanges. Key takeaways:

1. **Adaptor signatures** enable atomic swaps without scripting capabilities
2. **DLEQ proofs** allow the same secret to be used across different elliptic curves
3. **Split key approach** on Monero side replaces traditional hash locks
4. **Current implementations** exist in Rust (COMIT, Farcaster) and Go (AthanorLabs)
5. **Security is critical** - interactive protocol, timelock management, key share protection

For Klingon-v2 implementation:
- Use Go libraries: `github.com/athanorlabs/atomic-swap` and `github.com/athanorlabs/go-dleq`
- Implement BTC-XMR first (most mature), then ETH-XMR
- Consider SOL-XMR later (similar to ETH but different VM)
- All XMR swaps will use adaptor signatures, not HTLCs
