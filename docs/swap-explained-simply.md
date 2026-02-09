# How the Swap Code Works (Simple Explanation)

## What is an Atomic Swap?

Imagine you and your friend want to trade toys. You have a red car and your friend has a blue truck. But you don't trust each other - what if they take your car and run away without giving you the truck?

An **atomic swap** solves this! It's like a magic trading box that only opens when BOTH toys are inside. Either the trade happens completely, or nothing happens at all. No one can cheat!

## How Does It Work?

### Step 1: Creating a Special Lock (MuSig2)

Think of this like creating a special padlock that needs TWO keys to open.

```
You have your key:   🔑 (Alice's key)
Friend has their key: 🗝️ (Bob's key)

Together you create a SUPER KEY: 🔐 (Combined key)
```

This is called **MuSig2 Key Aggregation**. Neither of you alone can unlock the box - you BOTH need to agree!

In our code (`musig2.go`):
```go
// Alice and Bob both create keys
aliceSession := NewMuSig2Session("BTC", testnet, aliceKey)
bobSession := NewMuSig2Session("BTC", testnet, bobKey)

// They share their public keys
aliceSession.SetRemotePubKey(bobPubKey)
bobSession.SetRemotePubKey(alicePubKey)

// Both compute the SAME combined key!
address := aliceSession.TaprootAddress()  // "tb1p..."
```

### Step 2: The Trading Address

The combined key creates a special **Taproot address** (starts with `bc1p` or `tb1p`). This is like the magic trading box - funds sent here can ONLY be spent when both parties agree.

```
Regular address: Only YOU can spend from it
Taproot address: BOTH of you must agree to spend
```

### Step 3: The Swap Dance

Here's how Alice (has BTC) and Bob (has LTC) trade:

```
1. Alice creates an "offer"
   "I want to trade 0.1 BTC for 1 LTC"

2. Bob accepts the offer

3. BOTH create combined addresses:
   - BTC address (Alice + Bob's combined key)
   - LTC address (Alice + Bob's combined key)

4. Alice sends her BTC to the BTC combined address
   Bob sends his LTC to the LTC combined address

5. To complete the trade:
   - Alice signs to release the LTC to herself
   - Bob signs to release the BTC to himself

   They exchange signatures, and BOTH get what they wanted!
```

## The Code Files

### `swap.go` - The Rules of the Game

This file defines:
- **What can be traded** (BTC, LTC, etc.)
- **The states of a swap** (like checkpoints in a game)
- **Validation** (making sure you're not cheating)

```
States:
┌──────┐    ┌─────────┐    ┌────────┐    ┌──────────┐
│ init │ -> │ funding │ -> │ funded │ -> │ redeemed │
└──────┘    └─────────┘    └────────┘    └──────────┘
                                              OR
                                         ┌──────────┐
                                         │ refunded │
                                         └──────────┘
```

Example:
```go
// Create a swap offer
offer := Offer{
    OfferChain:    "BTC",      // I'm giving BTC
    OfferAmount:   100000,     // 0.001 BTC (in satoshis)
    RequestChain:  "LTC",      // I want LTC
    RequestAmount: 1000000,    // 0.01 LTC
    Method:        MethodMuSig2,
}

// Create the swap
swap, _ := NewSwap(testnet, MethodMuSig2, RoleInitiator, offer)

// Move through states
swap.TransitionTo(StateFunding)  // Now we're funding
swap.TransitionTo(StateFunded)   // Both funded!
swap.TransitionTo(StateRedeemed) // Trade complete!
```

### `musig2.go` - The Magic Key Combining

This is where the cryptographic magic happens:

1. **Key Aggregation** - Combine two keys into one
2. **Nonce Generation** - Random numbers for security
3. **Signing Sessions** - Creating signatures together
4. **Taproot Addresses** - The special combined addresses

```go
// Create a session
session := NewMuSig2Session("BTC", testnet, myPrivateKey)

// Share keys
session.SetRemotePubKey(theirPubKey)

// Get the combined address
address := session.TaprootAddress()
// Result: "tb1pec9jc3g3cs5lx..."  (Taproot address!)

// Generate nonces (for signing later)
nonces := session.GenerateNonces()

// Exchange nonces with partner, then sign together!
```

### `tx.go` - Building the Actual Transactions

This file creates the Bitcoin/Litecoin transactions:

```go
// Build a funding transaction
params := FundingTxParams{
    Symbol:        "BTC",
    Network:       testnet,
    UTXOs:         myUTXOs,          // Coins I'm spending
    SwapAddress:   combinedAddress,  // The magic trading box
    SwapAmount:    100000,           // How much to put in
    DAOFee:        100,              // Small fee for the service
    FeeRate:       10,               // Mining fee rate
}

tx := BuildFundingTx(params)
```

## Quick Summary

| File | What It Does |
|------|--------------|
| `swap.go` | Rules, states, validation - "what can happen" |
| `musig2.go` | Cryptography - "how keys combine" |
| `tx.go` | Transactions - "the actual money movements" |

## Why Is This Cool?

1. **No Trust Needed** - You don't need to trust your trading partner
2. **No Middleman** - No exchange or company in the middle
3. **Can't Cheat** - Either both get what they want, or nobody does
4. **Private** - Looks like normal transactions on the blockchain

## The Safety Net: Timelocks

What if Bob disappears after Alice funds? There's a timeout!

```
Alice's funds locked for: 48 hours
Bob's funds locked for:   24 hours

If Bob doesn't complete the trade within 24 hours,
he can get his LTC back.

If the whole thing fails, Alice can get her BTC back
after 48 hours.
```

This is configured in `config.DefaultSwapConfig()`:
```go
InitiatorLockTime: 48 * time.Hour  // Alice waits longer
ResponderLockTime: 24 * time.Hour  // Bob can refund earlier
```

## That's It!

The swap code is like a trustless vending machine:
1. Both people put in their coins
2. The machine checks everything is correct
3. Both people get what they wanted
4. If something goes wrong, everyone gets their coins back!
