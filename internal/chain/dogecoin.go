package chain

func init() {
	// Dogecoin Mainnet
	Register("DOGE", Mainnet, &Params{
		Symbol:   "DOGE",
		Name:     "Dogecoin",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		// BIP44 coin type 3
		CoinType:       3,
		DefaultPurpose: 44, // Legacy only

		// Mainnet address prefixes
		PubKeyHashAddrID: 0x1E, // D...
		ScriptHashAddrID: 0x16, // 9 or A
		Bech32HRP:        "",   // No SegWit
		WIF:              0x9E,

		// BIP32 HD key prefixes (dgpv/dgub)
		HDPrivateKeyID: [4]byte{0x02, 0xfa, 0xc3, 0x98}, // dgpv
		HDPublicKeyID:  [4]byte{0x02, 0xfa, 0xca, 0xfd}, // dgub

		// Features - DOGE does not support SegWit
		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressP2PKH,
	})

	// Dogecoin Testnet
	Register("DOGE", Testnet, &Params{
		Symbol:   "DOGE",
		Name:     "Dogecoin Testnet",
		Type:     ChainTypeBitcoin,
		Decimals: 8,

		CoinType:       1,
		DefaultPurpose: 44,

		PubKeyHashAddrID: 0x71, // n...
		ScriptHashAddrID: 0xC4,
		Bech32HRP:        "",
		WIF:              0xF1,

		// BIP32 HD key prefixes (tprv/tpub - uses Bitcoin testnet)
		HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // tprv
		HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // tpub

		SupportsSegWit:  false,
		SupportsTaproot: false,

		DefaultAddressType: AddressP2PKH,
	})
}
