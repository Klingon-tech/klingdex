package chain

func init() {
	// Solana Mainnet
	Register("SOL", Mainnet, &Params{
		Symbol:   "SOL",
		Name:     "Solana",
		Type:     ChainTypeSolana,
		Decimals: 9,

		// BIP44 coin type 501
		CoinType:       501,
		DefaultPurpose: 44,

		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressSolana,
	})

	// Solana Devnet
	Register("SOL", Testnet, &Params{
		Symbol:   "SOL",
		Name:     "Solana Devnet",
		Type:     ChainTypeSolana,
		Decimals: 9,

		CoinType:       501,
		DefaultPurpose: 44,

		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressSolana,
	})
}
