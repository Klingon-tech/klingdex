// Package wallet provides EVM (Ethereum/compatible chains) address generation.
package wallet

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"golang.org/x/crypto/sha3"
)

// PublicKeyToEVMAddress converts a secp256k1 public key to an EVM address.
// Address = "0x" + last 20 bytes of Keccak256(uncompressed pubkey without 0x04 prefix)
func PublicKeyToEVMAddress(pubKey *btcec.PublicKey) string {
	// Get uncompressed public key bytes (65 bytes starting with 0x04)
	pubKeyBytes := pubKey.SerializeUncompressed()

	// Hash without the 0x04 prefix
	hash := Keccak256(pubKeyBytes[1:])

	// Take last 20 bytes
	address := hash[12:]

	return ChecksumAddress(hex.EncodeToString(address))
}

// PrivateKeyToEVMAddress converts a private key to an EVM address.
func PrivateKeyToEVMAddress(privKey *btcec.PrivateKey) string {
	return PublicKeyToEVMAddress(privKey.PubKey())
}

// Keccak256 computes the Keccak-256 hash (used by Ethereum).
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// ChecksumAddress applies EIP-55 checksum to an address.
func ChecksumAddress(addr string) string {
	addr = strings.ToLower(strings.TrimPrefix(addr, "0x"))
	hash := hex.EncodeToString(Keccak256([]byte(addr)))

	result := "0x"
	for i, c := range addr {
		if c >= '0' && c <= '9' {
			result += string(c)
		} else {
			// If the ith digit of the hash is >= 8, uppercase
			if hash[i] >= '8' {
				result += strings.ToUpper(string(c))
			} else {
				result += string(c)
			}
		}
	}
	return result
}

// ValidateEVMAddress checks if an EVM address is valid.
func ValidateEVMAddress(address string) bool {
	address = strings.TrimPrefix(address, "0x")
	if len(address) != 40 {
		return false
	}
	_, err := hex.DecodeString(address)
	return err == nil
}

// IsChecksumValid checks if an EVM address has valid EIP-55 checksum.
func IsChecksumValid(address string) bool {
	address = strings.TrimPrefix(address, "0x")
	if len(address) != 40 {
		return false
	}

	// If all lowercase or all uppercase, checksum doesn't apply
	lower := strings.ToLower(address)
	upper := strings.ToUpper(address)
	if address == lower || address == upper {
		return true
	}

	// Verify checksum
	checksummed := ChecksumAddress(address)
	return checksummed == "0x"+address
}

// EVMSign signs a message hash (32 bytes) and returns the signature.
// Returns signature in Ethereum format: r || s || v (65 bytes)
func EVMSign(privKey *btcec.PrivateKey, hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash must be 32 bytes, got %d", len(hash))
	}

	// Use btcec for signing (same secp256k1 curve as Ethereum)
	sig := btcecdsa.SignCompact(privKey, hash, false)

	// SignCompact returns: v || r || s (65 bytes) where v is 27 or 28
	// Ethereum wants: r || s || v where v is 0 or 1
	if len(sig) != 65 {
		return nil, fmt.Errorf("invalid signature length")
	}

	// Rearrange: move v from front to back
	ethSig := make([]byte, 65)
	copy(ethSig[:64], sig[1:65]) // r || s
	ethSig[64] = sig[0] - 27    // v (convert from Bitcoin's 27/28 to Ethereum's 0/1)

	return ethSig, nil
}

// EVMSignTypedData signs EIP-712 typed data.
func EVMSignTypedData(privKey *btcec.PrivateKey, domainSeparator, structHash []byte) ([]byte, error) {
	// EIP-712: hash = keccak256("\x19\x01" || domainSeparator || structHash)
	prefix := []byte{0x19, 0x01}
	data := append(prefix, domainSeparator...)
	data = append(data, structHash...)
	hash := Keccak256(data)

	return EVMSign(privKey, hash)
}

// PersonalSign signs a message with Ethereum's personal_sign format.
// Prepends "\x19Ethereum Signed Message:\n" + len(message) + message
func PersonalSign(privKey *btcec.PrivateKey, message []byte) ([]byte, error) {
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	data := append([]byte(prefix), message...)
	hash := Keccak256(data)

	return EVMSign(privKey, hash)
}

// ToECDSA converts a btcec private key to crypto/ecdsa format.
func ToECDSA(privKey *btcec.PrivateKey) *ecdsa.PrivateKey {
	return privKey.ToECDSA()
}

// PrivateKeyHex returns the private key as a hex string (without 0x prefix).
func PrivateKeyHex(privKey *btcec.PrivateKey) string {
	return hex.EncodeToString(privKey.Serialize())
}

// PrivateKeyFromHex creates a private key from a hex string.
func PrivateKeyFromHex(hexStr string) (*btcec.PrivateKey, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	privKey, _ := btcec.PrivKeyFromBytes(bytes)
	return privKey, nil
}

// FormatWei formats wei to readable ETH string.
func FormatWei(wei *big.Int) string {
	// 1 ETH = 10^18 wei
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return eth.Text('f', 6) + " ETH"
}

// ParseETH parses ETH string to wei.
func ParseETH(eth string) (*big.Int, error) {
	f, _, err := big.ParseFloat(eth, 10, 256, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	// Multiply by 10^18
	f.Mul(f, big.NewFloat(1e18))
	wei, _ := f.Int(nil)
	return wei, nil
}

// FormatGwei formats wei to Gwei.
func FormatGwei(wei *big.Int) string {
	gwei := new(big.Float).SetInt(wei)
	gwei.Quo(gwei, big.NewFloat(1e9))
	return gwei.Text('f', 2) + " Gwei"
}

// ParseGwei parses Gwei string to wei.
func ParseGwei(gweiStr string) (*big.Int, error) {
	f, _, err := big.ParseFloat(gweiStr, 10, 256, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	// Multiply by 10^9
	f.Mul(f, big.NewFloat(1e9))
	wei, _ := f.Int(nil)
	return wei, nil
}
