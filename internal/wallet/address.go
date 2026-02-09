// Package wallet provides Bitcoin-family address encoding.
package wallet

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/klingon-exchange/klingon-v2/internal/chain"
)

// DeriveAddressFromKey derives the appropriate address type from an HD key.
func DeriveAddressFromKey(key *hdkeychain.ExtendedKey, params *chain.Params) (string, error) {
	pubKey, err := key.ECPubKey()
	if err != nil {
		return "", fmt.Errorf("failed to get public key: %w", err)
	}

	// Convert chain params to btcd chaincfg
	chainParams := toChainCfgParams(params)

	switch params.DefaultAddressType {
	case chain.AddressP2PKH:
		return deriveP2PKH(pubKey, chainParams)
	case chain.AddressP2WPKH:
		return deriveP2WPKH(pubKey, chainParams)
	case chain.AddressP2TR:
		return deriveP2TR(pubKey, chainParams)
	default:
		// Default to P2WPKH for SegWit-capable chains, P2PKH otherwise
		if params.SupportsSegWit {
			return deriveP2WPKH(pubKey, chainParams)
		}
		return deriveP2PKH(pubKey, chainParams)
	}
}

// deriveP2PKH derives a legacy P2PKH address (1... for BTC, D... for DOGE, etc.)
func deriveP2PKH(pubKey *btcec.PublicKey, params *chaincfg.Params) (string, error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, params)
	if err != nil {
		return "", fmt.Errorf("failed to create P2PKH address: %w", err)
	}
	return addr.EncodeAddress(), nil
}

// deriveP2WPKH derives a native SegWit address (bc1q... for BTC, ltc1q... for LTC)
func deriveP2WPKH(pubKey *btcec.PublicKey, params *chaincfg.Params) (string, error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
	if err != nil {
		return "", fmt.Errorf("failed to create P2WPKH address: %w", err)
	}
	return addr.EncodeAddress(), nil
}

// deriveP2TR derives a Taproot address (bc1p...)
func deriveP2TR(pubKey *btcec.PublicKey, params *chaincfg.Params) (string, error) {
	taprootKey := txscript.ComputeTaprootKeyNoScript(pubKey)
	addr, err := btcutil.NewAddressTaproot(taprootKey.SerializeCompressed()[1:], params)
	if err != nil {
		return "", fmt.Errorf("failed to create Taproot address: %w", err)
	}
	return addr.EncodeAddress(), nil
}

// DeriveP2SH_P2WPKH derives a nested SegWit address (3... for BTC)
func DeriveP2SH_P2WPKH(pubKey *btcec.PublicKey, params *chaincfg.Params) (string, error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	witnessAddr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
	if err != nil {
		return "", fmt.Errorf("failed to create witness address: %w", err)
	}

	witnessScript, err := txscript.PayToAddrScript(witnessAddr)
	if err != nil {
		return "", fmt.Errorf("failed to create witness script: %w", err)
	}

	scriptHash := btcutil.Hash160(witnessScript)
	addr, err := btcutil.NewAddressScriptHashFromHash(scriptHash, params)
	if err != nil {
		return "", fmt.Errorf("failed to create P2SH address: %w", err)
	}
	return addr.EncodeAddress(), nil
}

// AllAddressTypes derives all supported address types for a public key.
func AllAddressTypes(pubKey *btcec.PublicKey, params *chain.Params) (map[chain.AddressType]string, error) {
	chainParams := toChainCfgParams(params)
	addresses := make(map[chain.AddressType]string)

	// P2PKH is always available
	p2pkh, err := deriveP2PKH(pubKey, chainParams)
	if err == nil {
		addresses[chain.AddressP2PKH] = p2pkh
	}

	// SegWit addresses only for chains that support it
	if params.SupportsSegWit {
		p2wpkh, err := deriveP2WPKH(pubKey, chainParams)
		if err == nil {
			addresses[chain.AddressP2WPKH] = p2wpkh
		}

		p2shP2wpkh, err := DeriveP2SH_P2WPKH(pubKey, chainParams)
		if err == nil {
			addresses[chain.AddressP2SH_P2WPKH] = p2shP2wpkh
		}
	}

	// Taproot only for chains that support it
	if params.SupportsTaproot {
		p2tr, err := deriveP2TR(pubKey, chainParams)
		if err == nil {
			addresses[chain.AddressP2TR] = p2tr
		}
	}

	return addresses, nil
}

// ValidateAddress checks if an address is valid for a chain/network.
func ValidateAddress(address string, params *chain.Params) bool {
	chainParams := toChainCfgParams(params)
	_, err := btcutil.DecodeAddress(address, chainParams)
	return err == nil
}

// ParseAddress decodes a Bitcoin-family address.
func ParseAddress(address string, params *chain.Params) (btcutil.Address, chain.AddressType, error) {
	chainParams := toChainCfgParams(params)

	decoded, err := btcutil.DecodeAddress(address, chainParams)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode address: %w", err)
	}

	var addrType chain.AddressType
	switch decoded.(type) {
	case *btcutil.AddressPubKeyHash:
		addrType = chain.AddressP2PKH
	case *btcutil.AddressScriptHash:
		addrType = chain.AddressP2SH
	case *btcutil.AddressWitnessPubKeyHash:
		addrType = chain.AddressP2WPKH
	case *btcutil.AddressWitnessScriptHash:
		addrType = chain.AddressP2WSH
	case *btcutil.AddressTaproot:
		addrType = chain.AddressP2TR
	default:
		addrType = "unknown"
	}

	return decoded, addrType, nil
}

// PrivateKeyToWIF converts a private key to Wallet Import Format.
func PrivateKeyToWIF(privKey *btcec.PrivateKey, params *chain.Params) (string, error) {
	chainParams := toChainCfgParams(params)
	wif, err := btcutil.NewWIF(privKey, chainParams, true)
	if err != nil {
		return "", fmt.Errorf("failed to create WIF: %w", err)
	}
	return wif.String(), nil
}

// WIFToPrivateKey converts a WIF string to a private key.
func WIFToPrivateKey(wifStr string, params *chain.Params) (*btcec.PrivateKey, error) {
	chainParams := toChainCfgParams(params)
	wif, err := btcutil.DecodeWIF(wifStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WIF: %w", err)
	}

	// Verify network
	if !wif.IsForNet(chainParams) {
		return nil, fmt.Errorf("WIF is for different network")
	}

	return wif.PrivKey, nil
}

// toChainCfgParams converts our chain.Params to btcd's chaincfg.Params.
func toChainCfgParams(params *chain.Params) *chaincfg.Params {
	// Use chain-specific HD magic bytes, fallback to Bitcoin mainnet if not set
	hdPrivateKeyID := params.HDPrivateKeyID
	hdPublicKeyID := params.HDPublicKeyID
	if hdPrivateKeyID == [4]byte{} {
		hdPrivateKeyID = [4]byte{0x04, 0x88, 0xad, 0xe4} // xprv
	}
	if hdPublicKeyID == [4]byte{} {
		hdPublicKeyID = [4]byte{0x04, 0x88, 0xb2, 0x1e} // xpub
	}

	return &chaincfg.Params{
		Name: params.Name,

		// Address encoding
		PubKeyHashAddrID:        params.PubKeyHashAddrID,
		ScriptHashAddrID:        params.ScriptHashAddrID,
		WitnessPubKeyHashAddrID: params.WitnessPubKeyHashAddrID,
		WitnessScriptHashAddrID: params.WitnessScriptHashAddrID,

		// Bech32
		Bech32HRPSegwit: params.Bech32HRP,

		// BIP32 HD key magic bytes (chain-specific)
		HDPrivateKeyID: hdPrivateKeyID,
		HDPublicKeyID:  hdPublicKeyID,
	}
}
