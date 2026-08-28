package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

// NewDigest creates an unkeyed 256-bit hash engine using the specified or
// default hash algorithm (blake2s).
func NewDigest(opts ...HasherOption) (hash.Hash, error) {
	return NewDigestWithKey(nil, opts...)
}

// NewDigestWithKey creates a 256-bit hash engine with an optional key using the
// specified or default hash algorithm (blake2s). If key is nil or empty, it
// functions as an unkeyed digest.
func NewDigestWithKey(key []byte, opts ...HasherOption) (hash.Hash, error) {
	options := hasherOptions{
		algorithm: DefaultHashAlgorithm,
	}
	for _, opt := range opts {
		opt(&options)
	}
	normalized, err := normalizeAlgorithm(options.algorithm)
	if err != nil {
		return nil, err
	}
	return createHashEngine(normalized, key)
}

// Digest256 computes the hex-encoded digest of data using the specified or
// default hash algorithm (blake2s).
func Digest256(data []byte, opts ...HasherOption) string {
	h, err := NewDigest(opts...)
	if err != nil {
		panic(fmt.Sprintf("crypto: digest init: %v", err))
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// DigestReader streams r through the hash engine and returns the hex-encoded digest.
func DigestReader(r io.Reader, opts ...HasherOption) (string, error) {
	h, err := NewDigest(opts...)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("crypto: digest stream: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RandomBytes generates n cryptographically secure random bytes.
func RandomBytes(n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("crypto: invalid random byte length %d", n)
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("crypto: random bytes: %w", err)
	}
	return buf, nil
}

// RandomHex generates a hex-encoded string of n cryptographically secure random bytes.
func RandomHex(n int) (string, error) {
	buf, err := RandomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ConstantTimeCompare performs a constant-time comparison of two strings to prevent timing attacks.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ConstantTimeCompareBytes performs a constant-time comparison of two byte slices.
func ConstantTimeCompareBytes(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
