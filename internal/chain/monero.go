package chain

func init() {
	// Monero Mainnet
	Register("XMR", Mainnet, &Params{
		Symbol:   "XMR",
		Name:     "Monero",
		Type:     ChainTypeMonero,
		Decimals: 12,

		// BIP44 coin type 128
		// Note: Monero uses its own key derivation, not standard BIP44
		// but we use 128 for compatibility with hardware wallets
		CoinType:       128,
		DefaultPurpose: 44,

		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressMonero,
	})

	// Monero Stagenet (testnet)
	Register("XMR", Testnet, &Params{
		Symbol:   "XMR",
		Name:     "Monero Stagenet",
		Type:     ChainTypeMonero,
		Decimals: 12,

		CoinType:       128,
		DefaultPurpose: 44,

		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressMonero,
	})
}
