package chain

func init() {
	// ==========================================================================
	// Ethereum
	// ==========================================================================

	// Ethereum Mainnet (chainID 1)
	Register("ETH", Mainnet, &Params{
		Symbol:      "ETH",
		Name:        "Ethereum",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 1,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Ethereum Sepolia Testnet (chainID 11155111)
	Register("ETH", Testnet, &Params{
		Symbol:      "ETH",
		Name:        "Ethereum Sepolia",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 11155111,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// BNB Smart Chain (BSC)
	// ==========================================================================

	// BSC Mainnet (chainID 56)
	Register("BSC", Mainnet, &Params{
		Symbol:      "BSC",
		Name:        "BNB Smart Chain",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "BNB",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 56,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// BSC Testnet (chainID 97)
	Register("BSC", Testnet, &Params{
		Symbol:      "BSC",
		Name:        "BNB Smart Chain Testnet",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "BNB",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 97,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// Polygon
	// ==========================================================================

	// Polygon Mainnet (chainID 137)
	Register("POLYGON", Mainnet, &Params{
		Symbol:      "POLYGON",
		Name:        "Polygon",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "POL", // Rebranded from MATIC to POL in 2024

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 137,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Polygon Amoy Testnet (chainID 80002)
	Register("POLYGON", Testnet, &Params{
		Symbol:      "POLYGON",
		Name:        "Polygon Amoy",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "POL", // Rebranded from MATIC to POL in 2024

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 80002,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// Arbitrum
	// ==========================================================================

	// Arbitrum One Mainnet (chainID 42161)
	Register("ARBITRUM", Mainnet, &Params{
		Symbol:      "ARBITRUM",
		Name:        "Arbitrum One",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH", // Arbitrum uses ETH as native token

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 42161,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Arbitrum Sepolia Testnet (chainID 421614)
	Register("ARBITRUM", Testnet, &Params{
		Symbol:      "ARBITRUM",
		Name:        "Arbitrum Sepolia",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 421614,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// Optimism
	// ==========================================================================

	// Optimism Mainnet (chainID 10)
	Register("OPTIMISM", Mainnet, &Params{
		Symbol:      "OPTIMISM",
		Name:        "Optimism",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH", // Optimism uses ETH as native token

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 10,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Optimism Sepolia Testnet (chainID 11155420)
	Register("OPTIMISM", Testnet, &Params{
		Symbol:      "OPTIMISM",
		Name:        "Optimism Sepolia",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 11155420,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// Base
	// ==========================================================================

	// Base Mainnet (chainID 8453)
	Register("BASE", Mainnet, &Params{
		Symbol:      "BASE",
		Name:        "Base",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH", // Base uses ETH as native token

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 8453,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Base Sepolia Testnet (chainID 84532)
	Register("BASE", Testnet, &Params{
		Symbol:      "BASE",
		Name:        "Base Sepolia",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "ETH",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 84532,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// ==========================================================================
	// Avalanche
	// ==========================================================================

	// Avalanche C-Chain Mainnet (chainID 43114)
	Register("AVAX", Mainnet, &Params{
		Symbol:      "AVAX",
		Name:        "Avalanche C-Chain",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "AVAX",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 43114,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})

	// Avalanche Fuji Testnet (chainID 43113)
	Register("AVAX", Testnet, &Params{
		Symbol:      "AVAX",
		Name:        "Avalanche Fuji",
		Type:        ChainTypeEVM,
		Decimals:    18,
		NativeToken: "AVAX",

		CoinType:       60,
		DefaultPurpose: 44,

		ChainID: 43113,

		SupportsSegWit:     false,
		SupportsTaproot:    false,
		DefaultAddressType: AddressEVM,
	})
}
