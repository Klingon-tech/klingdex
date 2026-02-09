// Package chain defines chain parameters and derivation paths for supported cryptocurrencies.
// All chain-specific values are hardcoded here - no external configuration needed.
package chain

// Network represents mainnet or testnet.
type Network string

const (
	Mainnet Network = "mainnet"
	Testnet Network = "testnet"
)

// ChainType represents the blockchain family.
type ChainType string

const (
	ChainTypeBitcoin ChainType = "bitcoin"  // BTC and forks (LTC, BCH, DOGE)
	ChainTypeEVM     ChainType = "evm"      // Ethereum and EVM chains
	ChainTypeMonero  ChainType = "monero"   // Monero
	ChainTypeSolana  ChainType = "solana"   // Solana
)

// AddressType represents the address encoding format.
type AddressType string

const (
	// Bitcoin address types
	AddressP2PKH       AddressType = "p2pkh"       // Legacy (1...)
	AddressP2SH        AddressType = "p2sh"        // Script hash (3...)
	AddressP2WPKH      AddressType = "p2wpkh"      // Native SegWit (bc1q...)
	AddressP2WSH       AddressType = "p2wsh"       // SegWit script (bc1q...)
	AddressP2SH_P2WPKH AddressType = "p2sh-p2wpkh" // Nested SegWit (3...)
	AddressP2TR        AddressType = "p2tr"        // Taproot (bc1p...)

	// EVM address type
	AddressEVM AddressType = "evm" // 0x...

	// Solana address type
	AddressSolana AddressType = "solana" // Base58

	// Monero address type
	AddressMonero AddressType = "monero" // Base58 (4...)
)

// Params contains all parameters for a blockchain.
type Params struct {
	// Identity
	Symbol   string    // BTC, LTC, ETH, etc.
	Name     string    // Bitcoin, Litecoin, etc.
	Type     ChainType // bitcoin, evm, monero, solana
	Decimals uint8     // 8 for BTC, 18 for ETH, etc.

	// BIP44 derivation
	CoinType       uint32 // BIP44 coin type (0=BTC, 2=LTC, 60=ETH, etc.)
	DefaultPurpose uint32 // 44, 49, 84, or 86 (Taproot)

	// Network params (Bitcoin-like)
	PubKeyHashAddrID        byte   // Address prefix for P2PKH
	ScriptHashAddrID        byte   // Address prefix for P2SH
	WitnessPubKeyHashAddrID byte   // SegWit P2WPKH version
	WitnessScriptHashAddrID byte   // SegWit P2WSH version
	Bech32HRP               string // Bech32 human-readable prefix
	WIF                     byte   // Private key prefix

	// BIP32 HD key magic bytes (for xpub/xprv serialization)
	HDPrivateKeyID [4]byte // Extended private key prefix (e.g., xprv, Ltpv)
	HDPublicKeyID  [4]byte // Extended public key prefix (e.g., xpub, Ltub)

	// EVM params
	ChainID     uint64 // EVM chain ID
	NativeToken string // Native token symbol (ETH, BNB, MATIC) - empty means same as Symbol

	// Features
	SupportsSegWit  bool // Native SegWit support
	SupportsTaproot bool // Taproot/MuSig2 support

	// Default address type for this chain
	DefaultAddressType AddressType
}

// DerivationPath returns the BIP44/49/84 derivation path for this chain.
// Format: m/purpose'/coin'/account'/change/index
func (p *Params) DerivationPath(account, change, index uint32) []uint32 {
	return []uint32{
		p.DefaultPurpose + 0x80000000, // purpose' (hardened)
		p.CoinType + 0x80000000,       // coin_type' (hardened)
		account + 0x80000000,          // account' (hardened)
		change,                        // change (0=external, 1=internal)
		index,                         // address_index
	}
}

// DerivationPathString returns the derivation path as a string.
func (p *Params) DerivationPathString(account, change, index uint32) string {
	return formatPath(p.DefaultPurpose, p.CoinType, account, change, index)
}

func formatPath(purpose, coinType, account, change, index uint32) string {
	return "m/" +
		itoa(purpose) + "'/" +
		itoa(coinType) + "'/" +
		itoa(account) + "'/" +
		itoa(change) + "/" +
		itoa(index)
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Registry holds all chain parameters indexed by symbol.
var registry = make(map[string]map[Network]*Params)

// Register adds chain params to the registry.
func Register(symbol string, network Network, params *Params) {
	if registry[symbol] == nil {
		registry[symbol] = make(map[Network]*Params)
	}
	registry[symbol][network] = params
}

// Get returns chain params for a symbol and network.
func Get(symbol string, network Network) (*Params, bool) {
	nets, ok := registry[symbol]
	if !ok {
		return nil, false
	}
	params, ok := nets[network]
	return params, ok
}

// List returns all registered chain symbols.
func List() []string {
	symbols := make([]string, 0, len(registry))
	for symbol := range registry {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// ListByType returns all chains of a specific type.
func ListByType(chainType ChainType) []string {
	var symbols []string
	for symbol, nets := range registry {
		for _, params := range nets {
			if params.Type == chainType {
				symbols = append(symbols, symbol)
				break
			}
		}
	}
	return symbols
}

// IsSupported returns true if the chain is registered.
func IsSupported(symbol string) bool {
	_, ok := registry[symbol]
	return ok
}

// GetByChainID returns chain params for an EVM chain ID.
func GetByChainID(chainID uint64, network Network) (*Params, bool) {
	for _, nets := range registry {
		if params, ok := nets[network]; ok {
			if params.Type == ChainTypeEVM && params.ChainID == chainID {
				return params, true
			}
		}
	}
	return nil, false
}

// GetNativeToken returns the native token symbol for a chain.
// For most EVM chains, this returns "ETH", "BNB", "MATIC", etc.
func (p *Params) GetNativeToken() string {
	if p.NativeToken != "" {
		return p.NativeToken
	}
	return p.Symbol
}

// ListEVMChains returns all EVM chains with their chain IDs.
func ListEVMChains(network Network) map[string]uint64 {
	result := make(map[string]uint64)
	for symbol, nets := range registry {
		if params, ok := nets[network]; ok {
			if params.Type == ChainTypeEVM {
				result[symbol] = params.ChainID
			}
		}
	}
	return result
}
