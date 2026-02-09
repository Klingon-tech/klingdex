package chain

func init() {
	// Litecoin Mainnet
	Register("LTC", Mainnet, &Params{
		Symbol:   "LTC",
		Name:     "Litecoin",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		// BIP44 coin type 2
		CoinType:       2,
		DefaultPurpose: 84, // Native SegWit (ltc1q...)

		// Mainnet address prefixes
		PubKeyHashAddrID: 0x30, // L...
		ScriptHashAddrID: 0x32, // M...
		Bech32HRP:        "ltc",
		WIF:              0xB0,

		// BIP32 HD key prefixes (Ltpv/Ltub)
		HDPrivateKeyID: [4]byte{0x01, 0x9d, 0x9c, 0xfe}, // Ltpv
		HDPublicKeyID:  [4]byte{0x01, 0x9d, 0xa4, 0x62}, // Ltub

		// Features
		SupportsSegWit:  true,
		SupportsTaproot: true, // MWEB upgrade added Taproot

		DefaultAddressType: AddressP2WPKH,
	})

	// Litecoin Testnet
	Register("LTC", Testnet, &Params{
		Symbol:   "LTC",
		Name:     "Litecoin Testnet",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		CoinType:       1, // Testnet uses coin type 1
		DefaultPurpose: 84,

		// Testnet address prefixes
		PubKeyHashAddrID: 0x6F, // m or n
		ScriptHashAddrID: 0x3A, // Q...
		Bech32HRP:        "tltc",
		WIF:              0xEF,

		// BIP32 HD key prefixes (ttpv/ttub - Litecoin testnet)
		HDPrivateKeyID: [4]byte{0x04, 0x36, 0xef, 0x7d}, // ttpv
		HDPublicKeyID:  [4]byte{0x04, 0x36, 0xf6, 0xe1}, // ttub

		SupportsSegWit:  true,
		SupportsTaproot: true,

		DefaultAddressType: AddressP2WPKH,
	})
}
