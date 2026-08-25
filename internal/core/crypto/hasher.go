package crypto

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/blake2s"
)

// Supported hash algorithms in the allowlist.
const (
	AlgorithmBlake2s = "blake2s"
	AlgorithmBlake2b = "blake2b"

	// DefaultHashAlgorithm is the default engine used when none is specified.
	DefaultHashAlgorithm = AlgorithmBlake2s
)

// Hasher provides keyed cryptographic hashing for blind indexing,
// session token fingerprints, and verification with cryptographic agility.
type Hasher interface {
	// Hash computes the keyed digest of data using the configured algorithm,
	// returning a formatted string: "<algorithm>$<hex_encoded_hash>".
	Hash(data []byte) (string, error)

	// HashString computes the keyed digest of a string using the configured algorithm,
	// returning a formatted string: "<algorithm>$<hex_encoded_hash>".
	HashString(s string) (string, error)

	// BlindIndex computes a deterministic blind index for exact matching,
	// combining system and value (system || '\x00' || value),
	// returning a formatted string: "<algorithm>$<hex_encoded_hash>".
	BlindIndex(system, value string) (string, error)

	// Verify checks if the provided data matches an encoded hash string.
	// Supports prefixed strings ("<algorithm>$<hex_hash>") and legacy raw hex hashes.
	// Uses constant-time comparison to prevent timing attacks.
	Verify(data []byte, encodedHash string) (bool, error)

	// VerifyString checks if the provided string matches an encoded hash string.
	VerifyString(s string, encodedHash string) (bool, error)

	// Algorithm returns the active algorithm name configured for this Hasher.
	Algorithm() string
}

// HasherOption configures a Hasher instance.
type HasherOption func(*hasherOptions)

type hasherOptions struct {
	algorithm string
}

// WithHashAlgorithm configures the default algorithm for the Hasher.
func WithHashAlgorithm(algo string) HasherOption {
	return func(o *hasherOptions) {
		o.algorithm = algo
	}
}

// KeyedHasher implements Hasher with hybrid routing and allowlist enforcement.
type KeyedHasher struct {
	key       []byte
	algorithm string
}

var _ Hasher = (*KeyedHasher)(nil)

// NewHasher creates a new KeyedHasher instance.
// Fails fast if the key is empty or smaller than 32 bytes, or if an unsupported algorithm is specified.
func NewHasher(key []byte, opts ...HasherOption) (*KeyedHasher, error) {
	if len(key) < 32 {
		return nil, ErrWeakKey
	}

	options := hasherOptions{
		algorithm: DefaultHashAlgorithm,
	}
	for _, opt := range opts {
		opt(&options)
	}

	normalizedAlgo, err := normalizeAlgorithm(options.algorithm)
	if err != nil {
		return nil, err
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &KeyedHasher{
		key:       keyCopy,
		algorithm: normalizedAlgo,
	}, nil
}

// NewHasherFromDEK derives the blind-index key from a clinic DEK via HKDF
// (InfoBlindIndex) and returns a Hasher. Isolation between clinics is this key,
// not a per-clinic catalog URN.
func NewHasherFromDEK(dek []byte, opts ...HasherOption) (*KeyedHasher, error) {
	if len(dek) < SizeDEK {
		return nil, ErrWeakKey
	}
	blindKey := hkdfExpand(dek, InfoBlindIndex)
	defer ZeroBytes(blindKey)
	return NewHasher(blindKey, opts...)
}

// NewHasherFromBase64 creates a new KeyedHasher from a base64-encoded key string.
func NewHasherFromBase64(keyB64 string, opts ...HasherOption) (*KeyedHasher, error) {
	if keyB64 == "" {
		return nil, ErrWeakKey
	}
	raw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid base64 key: %w", err)
	}
	defer ZeroBytes(raw)
	return NewHasher(raw, opts...)
}

// Algorithm returns the configured hash algorithm.
func (h *KeyedHasher) Algorithm() string {
	return h.algorithm
}

// Hash computes the keyed digest for data and returns "<algorithm>$<hex_encoded_hash>".
func (h *KeyedHasher) Hash(data []byte) (string, error) {
	digest, err := h.computeDigest(h.algorithm, data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%s", h.algorithm, hex.EncodeToString(digest)), nil
}

// HashString is a helper that hashes string data.
func (h *KeyedHasher) HashString(s string) (string, error) {
	return h.Hash([]byte(s))
}

// BlindIndex computes a deterministic blind index for exact matching.
func (h *KeyedHasher) BlindIndex(system, value string) (string, error) {
	payload := make([]byte, 0, len(system)+1+len(value))
	payload = append(payload, []byte(system)...)
	payload = append(payload, 0)
	payload = append(payload, []byte(value)...)
	return h.Hash(payload)
}

// Verify checks if data matches encodedHash.
func (h *KeyedHasher) Verify(data []byte, encodedHash string) (bool, error) {
	if encodedHash == "" {
		return false, ErrInvalidHashFormat
	}

	var targetAlgo string
	var expectedHex string

	if strings.Contains(encodedHash, "$") {
		parts := strings.SplitN(encodedHash, "$", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return false, ErrInvalidHashFormat
		}
		var err error
		targetAlgo, err = normalizeAlgorithm(parts[0])
		if err != nil {
			return false, err
		}
		expectedHex = parts[1]
	} else {
		// Legacy format without prefix: use the default/configured algorithm
		targetAlgo = h.algorithm
		expectedHex = encodedHash
	}

	expectedBytes, err := hex.DecodeString(expectedHex)
	if err != nil {
		return false, fmt.Errorf("%w: invalid hex encoding", ErrInvalidHashFormat)
	}

	actualDigest, err := h.computeDigest(targetAlgo, data)
	if err != nil {
		return false, err
	}

	if len(actualDigest) != len(expectedBytes) {
		return false, nil
	}

	if subtle.ConstantTimeCompare(actualDigest, expectedBytes) == 1 {
		return true, nil
	}
	return false, nil
}

// VerifyString checks if string s matches encodedHash.
func (h *KeyedHasher) VerifyString(s string, encodedHash string) (bool, error) {
	return h.Verify([]byte(s), encodedHash)
}

// computeDigest executes keyed hashing using the allowlisted algorithm engine.
func (h *KeyedHasher) computeDigest(algorithm string, data []byte) ([]byte, error) {
	engine, err := createHashEngine(algorithm, h.key)
	if err != nil {
		return nil, err
	}
	engine.Write(data)
	return engine.Sum(nil), nil
}

func createHashEngine(algorithm string, key []byte) (hash.Hash, error) {
	switch algorithm {
	case AlgorithmBlake2s:
		h, err := blake2s.New256(key)
		if err != nil {
			return nil, fmt.Errorf("crypto: blake2s init: %w", err)
		}
		return h, nil

	case AlgorithmBlake2b:
		h, err := blake2b.New256(key)
		if err != nil {
			return nil, fmt.Errorf("crypto: blake2b init: %w", err)
		}
		return h, nil

	default:
		return nil, ErrUnsupportedAlgorithm
	}
}

func normalizeAlgorithm(algo string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(algo))
	switch normalized {
	case AlgorithmBlake2s:
		return AlgorithmBlake2s, nil
	case AlgorithmBlake2b:
		return AlgorithmBlake2b, nil
	default:
		return "", ErrUnsupportedAlgorithm
	}
}
