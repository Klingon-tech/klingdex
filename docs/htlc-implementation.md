# HTLC Implementation Plan

This document outlines what's needed to add HTLC (Hash Time-Locked Contract) support as an alternative to MuSig2 for atomic swaps.

## Why HTLC?

1. **Fallback for non-Taproot chains** - DOGE, BCH don't support Taproot
2. **User choice** - Some users may prefer HTLC for compatibility
3. **Broader wallet support** - More wallets support standard P2WSH than Taproot

## Current State

The **config layer** already supports both methods:

```go
// internal/config/config.go
Coins: map[string]Coin{
    "BTC": {SwapMethods: []SwapMethod{SwapMethodMuSig2, SwapMethodHTLC}},
    "LTC": {SwapMethods: []SwapMethod{SwapMethodMuSig2, SwapMethodHTLC}},
    "DOGE": {SwapMethods: []SwapMethod{SwapMethodHTLC}},  // No Taproot
}
```

The **implementation layer** only handles MuSig2.

## Key Differences: MuSig2 vs HTLC

| Aspect | MuSig2 | HTLC |
|--------|--------|------|
| Address type | P2TR (Taproot) | P2WSH (SegWit) |
| Signature | Schnorr (aggregated) | ECDSA (standard) |
| Nonce exchange | Required | Not needed |
| Privacy | Better (looks like regular spend) | Visible script |
| Script complexity | Hidden in Taproot tree | Visible in witness |
| Chain support | BTC, LTC (Taproot-enabled) | All Bitcoin-family |

## HTLC Script Structure

Standard atomic swap HTLC:

```
OP_IF
    // Claim path (counterparty reveals secret)
    OP_SHA256 <secret_hash> OP_EQUALVERIFY
    <counterparty_pubkey> OP_CHECKSIG
OP_ELSE
    // Refund path (timeout reached)
    <timelock_blocks> OP_CHECKSEQUENCEVERIFY OP_DROP
    <local_pubkey> OP_CHECKSIG
OP_ENDIF
```

**Claim witness:** `<signature> <secret> <1> <script>`
**Refund witness:** `<signature> <0> <script>`

## Files to Create

### 1. `internal/swap/method.go` - Abstraction Interface

```go
// SwapMethodHandler - common interface for MuSig2 and HTLC
type SwapMethodHandler interface {
    Method() Method
    GetLocalPubKey() []byte
    SetRemotePubKey([]byte) error
    GenerateSwapAddress(secretHash []byte, timelock uint32) (string, error)
    Sign(msgHash []byte) ([]byte, error)
    SupportsNonces() bool  // MuSig2=true, HTLC=false
}

// Factory function
func NewSwapMethodHandler(method Method, symbol string, network chain.Network, privKey *btcec.PrivateKey) (SwapMethodHandler, error) {
    switch method {
    case MethodMuSig2:
        return NewMuSig2Session(symbol, network, privKey)
    case MethodHTLC:
        return NewHTLCSession(symbol, network, privKey)
    default:
        return nil, fmt.Errorf("unsupported method: %s", method)
    }
}
```

### 2. `internal/swap/htlc.go` - HTLC Session Handler

```go
type HTLCSession struct {
    symbol       string
    network      chain.Network
    localPrivKey *btcec.PrivateKey
    localPubKey  *btcec.PublicKey
    remotePubKey *btcec.PublicKey

    // HTLC-specific
    secretHash   []byte
    timelock     uint32
    htlcScript   []byte
    htlcAddress  string
}

func NewHTLCSession(symbol string, network chain.Network, privKey *btcec.PrivateKey) (*HTLCSession, error)

// Interface implementation
func (s *HTLCSession) Method() Method { return MethodHTLC }
func (s *HTLCSession) GetLocalPubKey() []byte
func (s *HTLCSession) SetRemotePubKey(pub []byte) error
func (s *HTLCSession) GenerateSwapAddress(secretHash []byte, timelock uint32) (string, error)
func (s *HTLCSession) Sign(msgHash []byte) ([]byte, error)  // ECDSA
func (s *HTLCSession) SupportsNonces() bool { return false }

// HTLC-specific methods
func (s *HTLCSession) GetHTLCScript() []byte
func (s *HTLCSession) BuildClaimWitness(sig, secret []byte) wire.TxWitness
func (s *HTLCSession) BuildRefundWitness(sig []byte) wire.TxWitness
```

### 3. `internal/swap/htlc_script.go` - Script Generation

```go
// BuildHTLCScript creates standard atomic swap HTLC script
func BuildHTLCScript(secretHash []byte, claimPubKey, refundPubKey *btcec.PublicKey, timelock uint32) ([]byte, error) {
    builder := txscript.NewScriptBuilder()

    // Claim path
    builder.AddOp(txscript.OP_IF)
    builder.AddOp(txscript.OP_SHA256)
    builder.AddData(secretHash)
    builder.AddOp(txscript.OP_EQUALVERIFY)
    builder.AddData(claimPubKey.SerializeCompressed())
    builder.AddOp(txscript.OP_CHECKSIG)

    // Refund path
    builder.AddOp(txscript.OP_ELSE)
    builder.AddInt64(int64(timelock))
    builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)
    builder.AddOp(txscript.OP_DROP)
    builder.AddData(refundPubKey.SerializeCompressed())
    builder.AddOp(txscript.OP_CHECKSIG)
    builder.AddOp(txscript.OP_ENDIF)

    return builder.Script()
}

// HTLCScriptToP2WSHAddress converts script to P2WSH address
func HTLCScriptToP2WSHAddress(script []byte, chainParams *chaincfg.Params) (string, error) {
    scriptHash := sha256.Sum256(script)
    addr, err := btcutil.NewAddressWitnessScriptHash(scriptHash[:], chainParams)
    if err != nil {
        return "", err
    }
    return addr.EncodeAddress(), nil
}

// BuildHTLCClaimWitness creates witness for claiming with secret
func BuildHTLCClaimWitness(sig, secret, script []byte) wire.TxWitness {
    return wire.TxWitness{
        sig,          // Signature
        secret,       // Preimage
        {0x01},       // OP_TRUE for IF branch
        script,       // Redeem script
    }
}

// BuildHTLCRefundWitness creates witness for refund after timeout
func BuildHTLCRefundWitness(sig, script []byte) wire.TxWitness {
    return wire.TxWitness{
        sig,          // Signature
        {},           // Empty for ELSE branch
        script,       // Redeem script
    }
}
```

### 4. `internal/swap/htlc_tx.go` - Transaction Builders

```go
type HTLCFundingParams struct {
    UTXOs         []UTXO
    HTLCAddress   string
    Amount        uint64
    ChangeAddress string
    FeeRate       uint64
    ChainParams   *chaincfg.Params
}

type HTLCClaimParams struct {
    FundingTxID   string
    FundingVout   uint32
    Amount        uint64
    HTLCScript    []byte
    Secret        []byte
    ClaimPrivKey  *btcec.PrivateKey
    ToAddress     string
    FeeRate       uint64
    ChainParams   *chaincfg.Params
}

type HTLCRefundParams struct {
    FundingTxID   string
    FundingVout   uint32
    Amount        uint64
    HTLCScript    []byte
    Timelock      uint32
    RefundPrivKey *btcec.PrivateKey
    ToAddress     string
    FeeRate       uint64
    ChainParams   *chaincfg.Params
}

func BuildHTLCFundingTx(params *HTLCFundingParams) (*wire.MsgTx, error)
func BuildHTLCClaimTx(params *HTLCClaimParams) (*wire.MsgTx, error)
func BuildHTLCRefundTx(params *HTLCRefundParams) (*wire.MsgTx, error)
```

## Files to Modify

### 1. `internal/swap/coordinator.go`

Change from hardcoded MuSig2 to method-agnostic:

```go
// Before
type ActiveSwap struct {
    Swap   *Swap
    MuSig2 *MuSig2SwapData  // Only MuSig2
}

// After
type ActiveSwap struct {
    Swap       *Swap
    MethodData interface{}  // Either *MuSig2SwapData or *HTLCSwapData
}

type HTLCSwapData struct {
    OfferSession   *HTLCSession
    RequestSession *HTLCSession
}

func (c *Coordinator) InitiateSwap(offer Offer) (*Swap, error) {
    method := c.selectSwapMethod(offer)

    switch method {
    case MethodMuSig2:
        return c.initiateMuSig2Swap(offer)
    case MethodHTLC:
        return c.initiateHTLCSwap(offer)
    }
}

func (c *Coordinator) selectSwapMethod(offer Offer) Method {
    // Check offer preference
    if offer.Method != "" && config.SupportsSwapMethod(offer.OfferChain, offer.Method) {
        return offer.Method
    }
    // Fall back to preferred method for chain
    return config.GetPreferredSwapMethod(offer.OfferChain)
}
```

### 2. `internal/swap/swap.go`

Add method-specific data field:

```go
type Swap struct {
    // ... existing fields ...

    // Method-specific data (serialized for storage)
    MethodData []byte  // JSON: MuSig2StorageData or HTLCStorageData
}

type HTLCStorageData struct {
    Type            string `json:"type"`  // "htlc"
    LocalPrivKeyEnc []byte `json:"local_privkey_enc"`
    LocalPubKey     []byte `json:"local_pubkey"`
    RemotePubKey    []byte `json:"remote_pubkey"`
    SecretHash      []byte `json:"secret_hash"`
    Secret          []byte `json:"secret,omitempty"`  // Only for initiator
    Timelock        uint32 `json:"timelock"`
    HTLCScript      []byte `json:"htlc_script"`
    HTLCAddress     string `json:"htlc_address"`
}
```

### 3. `internal/storage/swap_legs.go`

Already supports method-specific JSON data in `method_data` column. Just need to add HTLC type handling.

## Swap Flow Comparison

### MuSig2 Flow (Current)

```
1. Maker creates order → broadcast
2. Taker takes order → creates trade
3. Both init swap → generate ephemeral keys
4. Exchange public keys → compute aggregated key → taproot address
5. Exchange nonces
6. Both fund taproot addresses
7. Wait for confirmations
8. Exchange partial signatures
9. Combine signatures → broadcast spend txs
```

### HTLC Flow (To Implement)

```
1. Maker creates order → broadcast
2. Taker takes order → creates trade
3. Initiator generates secret + hash
4. Exchange public keys
5. Both generate HTLC scripts with shared secret hash
6. Both fund P2WSH addresses
7. Wait for confirmations
8. Initiator claims counterparty's HTLC (reveals secret)
9. Responder sees secret on-chain → claims initiator's HTLC
   OR: Timeout reached → refund
```

**Key Difference:** HTLC reveals the secret on-chain when claiming. The counterparty learns the secret by watching the blockchain.

## Implementation Order

### Phase 1: Abstraction Layer
1. Create `method.go` with `SwapMethodHandler` interface
2. Make `MuSig2Session` implement the interface
3. No changes to existing behavior

### Phase 2: HTLC Core
1. Create `htlc.go` with `HTLCSession`
2. Create `htlc_script.go` with script builders
3. Create `htlc_tx.go` with transaction builders
4. Add unit tests

### Phase 3: Coordinator Integration
1. Modify `coordinator.go` to use method abstraction
2. Add `initiateHTLCSwap()` and `respondToHTLCSwap()`
3. Add HTLC-specific state transitions

### Phase 4: RPC Integration
1. Update `swap_init` to respect method preference
2. Add HTLC-specific status fields to `swap_status`
3. Secret handling in claim flow

### Phase 5: Testing
1. Unit tests for all new components
2. Integration test: HTLC swap between two nodes
3. Test fallback: MuSig2 preferred → HTLC fallback

## Security Considerations

### Secret Handling
- Secret must be 32 bytes (256 bits) of cryptographic randomness
- Use `crypto/rand` not `math/rand`
- Secret hash uses SHA256 (same as MuSig2 secret)
- Initiator MUST claim first (to reveal secret on-chain)

### Timelock Settings
- Same block-based timelocks as MuSig2:
  - BTC: 144 blocks (maker), 72 blocks (taker) ~24h/12h
  - LTC: 576 blocks (maker), 288 blocks (taker) ~24h/12h
- Initiator (secret holder) gets shorter timeout
- Safety margin before claiming near timeout

### On-Chain Privacy
- HTLC scripts are visible on-chain (less private than MuSig2)
- Secret hash links the two transactions
- Amounts and addresses publicly visible
- Consider: privacy-conscious users should prefer MuSig2

## References

- [Bitcoin Script Reference](https://en.bitcoin.it/wiki/Script)
- [BIP-199: Hashed Time-Locked Contract](https://github.com/bitcoin/bips/blob/master/bip-0199.mediawiki)
- [Atomic Swaps Explained](https://bitcoinops.org/en/topics/atomic-swaps/)
- [btcsuite txscript](https://pkg.go.dev/github.com/btcsuite/btcd/txscript)

## Estimated Effort

| Component | Complexity | Notes |
|-----------|------------|-------|
| `method.go` | Low | Interface + factory |
| `htlc.go` | Medium | Session handler |
| `htlc_script.go` | Medium | Script generation |
| `htlc_tx.go` | Medium | Transaction builders |
| `coordinator.go` changes | Medium | Method abstraction |
| Unit tests | Medium | All new code |
| Integration test | Low | Similar to MuSig2 test |

**Total:** Medium complexity. Core abstractions exist in config, main work is implementing HTLC-specific logic parallel to MuSig2.
