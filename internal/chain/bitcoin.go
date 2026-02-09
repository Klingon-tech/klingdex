package chain

func init() {
	// Bitcoin Mainnet
	Register("BTC", Mainnet, &Params{
		Symbol:   "BTC",
		Name:     "Bitcoin",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		// BIP44 coin type 0, BIP84 for native SegWit
		CoinType:       0,
		DefaultPurpose: 84, // Native SegWit (bc1q...)

		// Mainnet address prefixes
		PubKeyHashAddrID: 0x00, // 1...
		ScriptHashAddrID: 0x05, // 3...
		Bech32HRP:        "bc",
		WIF:              0x80,

		// BIP32 HD key prefixes (xprv/xpub)
		HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // xprv
		HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // xpub

		// Features
		SupportsSegWit:  true,
		SupportsTaproot: true,

		DefaultAddressType: AddressP2WPKH,
	})

	// Bitcoin Testnet (testnet3)
	Register("BTC", Testnet, &Params{
		Symbol:   "BTC",
		Name:     "Bitcoin Testnet",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		// Testnet uses coin type 1 for all coins
		CoinType:       1,
		DefaultPurpose: 84,

		// Testnet address prefixes
		PubKeyHashAddrID: 0x6F, // m or n
		ScriptHashAddrID: 0xC4, // 2...
		Bech32HRP:        "tb",
		WIF:              0xEF,

		// BIP32 HD key prefixes (tprv/tpub)
		HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // tprv
		HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // tpub

		// Features
		SupportsSegWit:  true,
		SupportsTaproot: true,

		DefaultAddressType: AddressP2WPKH,
	})
}
