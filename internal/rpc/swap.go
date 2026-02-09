// Package rpc - Swap RPC handlers.
//
// The swap handlers are organized into separate files by functionality:
//
//   - swap_types.go:    All param/result type definitions
//   - swap_init.go:     swapInit handler
//   - swap_nonce.go:    swapExchangeNonce handler
//   - swap_funding.go:  swapGetAddress, swapSetFunding, swapCheckFunding
//   - swap_signing.go:  swapSign, getDestinationAddressForChain
//   - swap_redeem.go:   swapRedeem, parseSchnorrSignature
//   - swap_status.go:   swapStatus, swapList
//   - swap_timeout.go:  swapRecover, swapTimeout, swapRefund, swapCheckTimeouts
//   - swap_htlc.go:     HTLC-specific handlers (reveal, get, claim, refund, extract)
//   - swap_p2p.go:      P2P message handlers (pubkey, nonce, funding, partial sig, HTLC)
package rpc
