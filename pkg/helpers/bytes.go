// Package helpers provides common utility functions used across the codebase.
package helpers

import (
	"crypto/rand"
	"crypto/subtle"
)

// CompareBytes compares two byte slices lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareBytes(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// BytesEqual checks if two byte slices are equal.
func BytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// IsZeroBytes checks if all bytes in the slice are zero.
func IsZeroBytes(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// GenerateSecureRandom generates n cryptographically secure random bytes.
func GenerateSecureRandom(n int) ([]byte, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

// ConstantTimeCompare compares two byte slices in constant time.
// Returns true if they are equal, false otherwise.
// This is safe against timing attacks.
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
