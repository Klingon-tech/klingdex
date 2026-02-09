# Swap Path Configuration

This document defines which coin pairs can be swapped and the methods available for each path.

## Why Path Restrictions?

Not all coin combinations are technically feasible or practical:

1. **Different scripting systems** - Bitcoin Script vs EVM Solidity vs Solana Programs
2. **Cryptographic requirements** - Monero adaptor signatures only work with secp256k1 curves
3. **Timelock coordination** - Cross-ecosystem swaps need careful timeout alignment
4. **Implementation complexity** - Some paths require significantly more work

## Supported Ecosystems

| Ecosystem | Coins | Scripting | Signature |
|-----------|-------|-----------|-----------|
| Bitcoin-family | BTC, LTC, DOGE, BCH | Bitcoin Script | ECDSA/Schnorr |
| Monero | XMR | None (privacy) | Ed25519 |
| EVM | ETH, BSC, MATIC, ARB | Solidity | ECDSA (secp256k1) |
| Solana | SOL | Rust Programs | Ed25519 |

## Swap Path Matrix

### Legend
- **MuSig2** - Taproot MuSig2 (most private, requires Taproot support)
- **HTLC** - Hash Time-Locked Contract (Bitcoin Script)
- **Adaptor** - Adaptor signatures (for Monero)
- **Contract** - EVM smart contract
- **Program** - Solana program
- ❌ - Not supported
- 🔮 - Future (technically possible but not implemented)

### Bitcoin-Family ↔ Bitcoin-Family

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| BTC ↔ LTC | ✅ | MuSig2, HTLC | Both have Taproot |
| BTC ↔ DOGE | ✅ | HTLC | DOGE no Taproot |
| BTC ↔ BCH | ✅ | HTLC | BCH no Taproot |
| LTC ↔ DOGE | ✅ | HTLC | DOGE no Taproot |
| LTC ↔ BCH | ✅ | HTLC | BCH no Taproot |
| DOGE ↔ BCH | ✅ | HTLC | Neither has Taproot |

**Same ecosystem, straightforward implementation.**

### Bitcoin-Family ↔ Monero

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| BTC ↔ XMR | ✅ | Adaptor | DLEQ proofs (ed25519 ↔ secp256k1) |
| LTC ↔ XMR | ✅ | Adaptor | Same as BTC-XMR |
| DOGE ↔ XMR | 🔮 | Adaptor | Possible but lower priority |
| BCH ↔ XMR | 🔮 | Adaptor | Possible but lower priority |

**Requires adaptor signatures with DLEQ proofs for cross-curve verification.**
See: `docs/monero-atomic-swaps.md`

### EVM ↔ EVM

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| ETH ↔ BSC | ✅ | Contract | Same HTLC contract |
| ETH ↔ MATIC | ✅ | Contract | Same HTLC contract |
| ETH ↔ ARB | ✅ | Contract | Same HTLC contract |
| BSC ↔ MATIC | ✅ | Contract | Same HTLC contract |
| BSC ↔ ARB | ✅ | Contract | Same HTLC contract |
| MATIC ↔ ARB | ✅ | Contract | Same HTLC contract |

**Same contract system, deploy identical HTLC contract to all chains.**

### Cross-Ecosystem: Bitcoin-Family ↔ EVM

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| BTC ↔ ETH | 🔮 | HTLC + Contract | Cross-ecosystem coordination |
| BTC ↔ BSC | 🔮 | HTLC + Contract | Cross-ecosystem coordination |
| LTC ↔ ETH | 🔮 | HTLC + Contract | Cross-ecosystem coordination |
| LTC ↔ BSC | 🔮 | HTLC + Contract | Cross-ecosystem coordination |

**Technically feasible but complex:**
- Bitcoin side uses HTLC script (P2WSH)
- EVM side uses HTLC smart contract
- Must coordinate timelocks carefully (block time vs timestamp)
- Secret hash shared between script and contract

### Cross-Ecosystem: Monero ↔ EVM

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| XMR ↔ ETH | ❌ | - | No practical method |
| XMR ↔ BSC | ❌ | - | No practical method |

**Not feasible:**
- Monero has no scripting capability
- Adaptor signatures require Bitcoin-style scripts for the other side
- EVM contracts can't verify Ed25519 adaptor proofs efficiently

### Solana Paths

| Path | Supported | Methods | Notes |
|------|-----------|---------|-------|
| SOL ↔ BTC | 🔮 | Program + HTLC | Requires Solana HTLC program |
| SOL ↔ ETH | 🔮 | Program + Contract | Both have scripting |
| SOL ↔ XMR | ❌ | - | No practical method |

**Future work:**
- Solana uses Ed25519, same as Monero
- Could potentially do SOL ↔ XMR with adaptor sigs (research needed)
- SOL ↔ Bitcoin-family needs custom program

## Implementation Plan

### Phase 1: MVP (Same Ecosystem)

```
Bitcoin-family ↔ Bitcoin-family
├── BTC ↔ LTC (MuSig2 preferred, HTLC fallback) ✅ DONE
├── BTC ↔ DOGE (HTLC only)
├── BTC ↔ BCH (HTLC only)
├── LTC ↔ DOGE (HTLC only)
├── LTC ↔ BCH (HTLC only)
└── DOGE ↔ BCH (HTLC only)

EVM ↔ EVM
├── ETH ↔ BSC
├── ETH ↔ MATIC
├── ETH ↔ ARB
├── BSC ↔ MATIC
├── BSC ↔ ARB
└── MATIC ↔ ARB
```

### Phase 2: Monero Integration

```
Bitcoin-family ↔ XMR
├── BTC ↔ XMR (adaptor signatures)
└── LTC ↔ XMR (adaptor signatures)
```

### Phase 3: Cross-Ecosystem

```
Bitcoin-family ↔ EVM
├── BTC ↔ ETH
├── BTC ↔ BSC
├── LTC ↔ ETH
└── LTC ↔ BSC
```

### Phase 4: Solana

```
Solana paths (TBD based on demand)
```

## Config Implementation

### Data Structures

```go
// internal/config/swap_paths.go

type SwapPathStatus int

const (
    PathSupported SwapPathStatus = iota
    PathFuture                    // Technically possible, not implemented
    PathNotSupported              // Not feasible
)

type SwapPath struct {
    From      string
    To        string
    Status    SwapPathStatus
    Methods   []SwapMethod      // Available methods for this path
    Preferred SwapMethod        // Recommended method
    Notes     string
}

// Bidirectional - BTC-LTC same as LTC-BTC
func normalizePathKey(from, to string) string {
    if from > to {
        return to + "-" + from
    }
    return from + "-" + to
}
```

### Path Registry

```go
var SwapPaths = map[string]SwapPath{
    // Bitcoin-family ↔ Bitcoin-family
    "BTC-LTC":  {Status: PathSupported, Methods: []SwapMethod{SwapMethodMuSig2, SwapMethodHTLC}, Preferred: SwapMethodMuSig2},
    "BTC-DOGE": {Status: PathSupported, Methods: []SwapMethod{SwapMethodHTLC}, Preferred: SwapMethodHTLC},
    "BTC-BCH":  {Status: PathSupported, Methods: []SwapMethod{SwapMethodHTLC}, Preferred: SwapMethodHTLC},
    "DOGE-LTC": {Status: PathSupported, Methods: []SwapMethod{SwapMethodHTLC}, Preferred: SwapMethodHTLC},
    "BCH-LTC":  {Status: PathSupported, Methods: []SwapMethod{SwapMethodHTLC}, Preferred: SwapMethodHTLC},
    "BCH-DOGE": {Status: PathSupported, Methods: []SwapMethod{SwapMethodHTLC}, Preferred: SwapMethodHTLC},

    // Bitcoin-family ↔ XMR
    "BTC-XMR": {Status: PathSupported, Methods: []SwapMethod{SwapMethodAdaptor}, Preferred: SwapMethodAdaptor},
    "LTC-XMR": {Status: PathSupported, Methods: []SwapMethod{SwapMethodAdaptor}, Preferred: SwapMethodAdaptor},
    "DOGE-XMR": {Status: PathFuture, Notes: "Adaptor sigs possible but not prioritized"},
    "BCH-XMR":  {Status: PathFuture, Notes: "Adaptor sigs possible but not prioritized"},

    // EVM ↔ EVM
    "ARB-BSC":   {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},
    "ARB-ETH":   {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},
    "ARB-MATIC": {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},
    "BSC-ETH":   {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},
    "BSC-MATIC": {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},
    "ETH-MATIC": {Status: PathSupported, Methods: []SwapMethod{SwapMethodContract}, Preferred: SwapMethodContract},

    // Cross-ecosystem (future)
    "BTC-ETH":  {Status: PathFuture, Notes: "Cross-ecosystem HTLC coordination"},
    "BTC-BSC":  {Status: PathFuture, Notes: "Cross-ecosystem HTLC coordination"},
    "LTC-ETH":  {Status: PathFuture, Notes: "Cross-ecosystem HTLC coordination"},
    "LTC-BSC":  {Status: PathFuture, Notes: "Cross-ecosystem HTLC coordination"},
    "BTC-SOL":  {Status: PathFuture, Notes: "Requires Solana HTLC program"},
    "ETH-SOL":  {Status: PathFuture, Notes: "Requires Solana HTLC program"},

    // Not supported
    "ETH-XMR":   {Status: PathNotSupported, Notes: "No practical atomic swap method"},
    "BSC-XMR":   {Status: PathNotSupported, Notes: "No practical atomic swap method"},
    "MATIC-XMR": {Status: PathNotSupported, Notes: "No practical atomic swap method"},
    "ARB-XMR":   {Status: PathNotSupported, Notes: "No practical atomic swap method"},
    "SOL-XMR":   {Status: PathNotSupported, Notes: "No practical atomic swap method"},
}
```

### Helper Functions

```go
// IsSwapPathSupported checks if a swap between two coins is supported
func IsSwapPathSupported(from, to string) bool {
    key := normalizePathKey(from, to)
    path, exists := SwapPaths[key]
    if !exists {
        return false
    }
    return path.Status == PathSupported
}

// GetSwapPathMethods returns available methods for a path
func GetSwapPathMethods(from, to string) []SwapMethod {
    key := normalizePathKey(from, to)
    path, exists := SwapPaths[key]
    if !exists || path.Status != PathSupported {
        return nil
    }
    return path.Methods
}

// GetPreferredPathMethod returns the recommended method for a path
func GetPreferredPathMethod(from, to string) (SwapMethod, error) {
    key := normalizePathKey(from, to)
    path, exists := SwapPaths[key]
    if !exists {
        return "", fmt.Errorf("unknown swap path: %s", key)
    }
    if path.Status != PathSupported {
        return "", fmt.Errorf("swap path not supported: %s (%s)", key, path.Notes)
    }
    return path.Preferred, nil
}

// GetSwapPathStatus returns the status and notes for a path
func GetSwapPathStatus(from, to string) (SwapPathStatus, string) {
    key := normalizePathKey(from, to)
    path, exists := SwapPaths[key]
    if !exists {
        return PathNotSupported, "Unknown coin pair"
    }
    return path.Status, path.Notes
}

// ListSupportedPaths returns all currently supported swap paths
func ListSupportedPaths() []SwapPath {
    var paths []SwapPath
    for _, path := range SwapPaths {
        if path.Status == PathSupported {
            paths = append(paths, path)
        }
    }
    return paths
}
```

## Integration Points

### Order Validation

```go
// internal/swap/swap.go - Offer.Validate()

func (o *Offer) Validate(network chain.Network) error {
    // Check path is supported
    if !config.IsSwapPathSupported(o.OfferChain, o.RequestChain) {
        status, notes := config.GetSwapPathStatus(o.OfferChain, o.RequestChain)
        if status == config.PathFuture {
            return fmt.Errorf("%s ↔ %s swaps not yet implemented: %s", o.OfferChain, o.RequestChain, notes)
        }
        return fmt.Errorf("%s ↔ %s swaps not supported: %s", o.OfferChain, o.RequestChain, notes)
    }

    // Check method is valid for this path
    methods := config.GetSwapPathMethods(o.OfferChain, o.RequestChain)
    if o.Method != "" && !slices.Contains(methods, o.Method) {
        return fmt.Errorf("method %s not available for %s ↔ %s (available: %v)",
            o.Method, o.OfferChain, o.RequestChain, methods)
    }

    // ... rest of validation
}
```

### RPC: List Supported Paths

```go
// internal/rpc/handlers.go

// swap_supportedPaths - returns all supported swap paths
func (s *Server) handleSwapSupportedPaths(params json.RawMessage) (interface{}, error) {
    paths := config.ListSupportedPaths()
    return map[string]interface{}{
        "paths": paths,
        "count": len(paths),
    }, nil
}
```

### CLI/UI Guidance

When user tries unsupported path:
```
Error: ETH ↔ XMR swaps are not supported.
Reason: No practical atomic swap method exists between EVM and Monero.

Supported paths for ETH:
  - ETH ↔ BSC (HTLC contract)
  - ETH ↔ MATIC (HTLC contract)
  - ETH ↔ ARB (HTLC contract)

Supported paths for XMR:
  - XMR ↔ BTC (adaptor signatures)
  - XMR ↔ LTC (adaptor signatures)
```

## Summary

| Ecosystem Pair | Status | Method |
|----------------|--------|--------|
| Bitcoin ↔ Bitcoin | ✅ MVP | MuSig2 / HTLC |
| Bitcoin ↔ Monero | ✅ Phase 2 | Adaptor |
| EVM ↔ EVM | ✅ MVP | Contract |
| Bitcoin ↔ EVM | 🔮 Phase 3 | Cross-HTLC |
| Solana ↔ Any | 🔮 Phase 4 | Program |
| Monero ↔ EVM | ❌ Never | - |
| Monero ↔ Solana | ❌ Never | - |
