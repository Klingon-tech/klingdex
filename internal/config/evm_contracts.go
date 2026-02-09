// Package config provides EVM contract addresses for the Klingon exchange.
//
// ALL EVM contract addresses MUST be defined here. Do not scatter contract
// addresses throughout the codebase.
package config

import "github.com/ethereum/go-ethereum/common"

// EVMContractAddresses holds contract addresses for a specific EVM chain.
type EVMContractAddresses struct {
	// HTLCContract is the KlingonHTLC contract address for atomic swaps
	HTLCContract common.Address

	// TODO: Future contract addresses
	// DEXRouter    common.Address // For potential DEX integration
	// Staking      common.Address // For KGX staking
}

// evmContractRegistry maps chainID -> contract addresses
var evmContractRegistry = map[uint64]*EVMContractAddresses{
	// ==========================================================================
	// Testnets
	// ==========================================================================

	// Ethereum Sepolia (chainID 11155111)
	11155111: {
		HTLCContract: common.HexToAddress("0x628c677e7b8889e64564d3f381565a9e6656aade"),
	},

	// BSC Testnet (chainID 97)
	97: {
		HTLCContract: common.HexToAddress("0xC8515f07b08b586a2Fd6A389585D9a182D03adFB"),
	},

	// Polygon Amoy (chainID 80002)
	80002: {
		HTLCContract: common.Address{}, // TODO: Deploy
	},

	// Arbitrum Sepolia (chainID 421614)
	421614: {
		HTLCContract: common.Address{}, // TODO: Deploy
	},

	// Optimism Sepolia (chainID 11155420)
	11155420: {
		HTLCContract: common.Address{}, // TODO: Deploy
	},

	// Base Sepolia (chainID 84532)
	84532: {
		HTLCContract: common.Address{}, // TODO: Deploy
	},

	// Avalanche Fuji (chainID 43113)
	43113: {
		HTLCContract: common.Address{}, // TODO: Deploy
	},

	// ==========================================================================
	// Mainnets (DO NOT DEPLOY UNTIL AUDIT COMPLETE)
	// ==========================================================================

	// Ethereum Mainnet (chainID 1)
	1: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// BSC Mainnet (chainID 56)
	56: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// Polygon Mainnet (chainID 137)
	137: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// Arbitrum One (chainID 42161)
	42161: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// Optimism Mainnet (chainID 10)
	10: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// Base Mainnet (chainID 8453)
	8453: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},

	// Avalanche C-Chain (chainID 43114)
	43114: {
		HTLCContract: common.Address{}, // TODO: Deploy after audit
	},
}

// GetEVMContracts returns contract addresses for a given chain ID.
// Returns nil if the chain is not registered.
func GetEVMContracts(chainID uint64) *EVMContractAddresses {
	return evmContractRegistry[chainID]
}

// GetHTLCContract returns the HTLC contract address for a given chain ID.
// Returns zero address if the chain is not registered or contract not deployed.
func GetHTLCContract(chainID uint64) common.Address {
	if contracts := evmContractRegistry[chainID]; contracts != nil {
		return contracts.HTLCContract
	}
	return common.Address{}
}

// IsHTLCDeployed returns true if the HTLC contract is deployed on the given chain.
func IsHTLCDeployed(chainID uint64) bool {
	contract := GetHTLCContract(chainID)
	return contract != (common.Address{})
}

// ListDeployedHTLCChains returns all chain IDs where HTLC is deployed.
func ListDeployedHTLCChains() []uint64 {
	var chains []uint64
	for chainID, contracts := range evmContractRegistry {
		if contracts.HTLCContract != (common.Address{}) {
			chains = append(chains, chainID)
		}
	}
	return chains
}

// RegisterEVMContracts registers or updates contract addresses for a chain.
// This can be used at runtime to update addresses (e.g., from config file).
func RegisterEVMContracts(chainID uint64, contracts *EVMContractAddresses) {
	evmContractRegistry[chainID] = contracts
}

// SetHTLCContract sets the HTLC contract address for a specific chain.
// Creates a new entry if the chain doesn't exist.
func SetHTLCContract(chainID uint64, address common.Address) {
	if evmContractRegistry[chainID] == nil {
		evmContractRegistry[chainID] = &EVMContractAddresses{}
	}
	evmContractRegistry[chainID].HTLCContract = address
}
