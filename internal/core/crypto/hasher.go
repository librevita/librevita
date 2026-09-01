package crypto

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"github.com/cockroachdb/errors"
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

// Hash key purposes (orthogonal to KeyScope). The values are the ASCII
// letters also used in hash tokens. Byte 0 is invalid so a zero-value
// hasher cannot stamp a purpose. Ciphertext envelopes do not carry
// purpose: wrap, FLE, and LVFE already use distinct formats.
const (
	// KeyPurposeIndex is HKDF InfoBlindIndex (global or clinic-derived).
	KeyPurposeIndex byte = 'i'
	// KeyPurposeSession is the PASETO session-store fingerprint key.
	KeyPurposeSession byte = 's'
)

const (
	hashFormatFields    = 4
	keyContextTokenSize = 2
)

// Hasher provides keyed cryptographic hashing for blind indexing,
// session token fingerprints, and verification with cryptographic agility.
type Hasher interface {
	// Hash computes the keyed digest of data using the configured algorithm,
	// returning "<algorithm>$<scope><purpose>$<kid>$<hex_encoded_hash>".
	Hash(data []byte) (string, error)

	// HashString computes the keyed digest of a string using the configured algorithm,
	// returning "<algorithm>$<scope><purpose>$<kid>$<hex_encoded_hash>".
	HashString(s string) (string, error)

	// BlindIndex computes a deterministic blind index for exact matching,
	// combining system and value (system || '\x00' || value),
	// returning "<algorithm>$<scope><purpose>$<kid>$<hex_encoded_hash>".
	BlindIndex(system, value string) (string, error)

	// Verify checks if the provided data matches an encoded hash string.
	// Encoded hashes must be "<algorithm>$<scope><purpose>$<kid>$<hex_hash>".
	// Scope, purpose, and key id must match this Hasher. Uses constant-time comparison.
	Verify(data []byte, encodedHash string) (bool, error)

	// VerifyString checks if the provided string matches an encoded hash string.
	VerifyString(s string, encodedHash string) (bool, error)

	// Algorithm returns the active algorithm name configured for this Hasher.
	Algorithm() string

	// KeyScope returns the key hierarchy tier stamped into hashes.
	KeyScope() byte

	// KeyPurpose returns the key purpose stamped into hashes.
	KeyPurpose() byte

	// KeyID returns the key generation stamped into hashes.
	KeyID() byte
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
	scope     byte
	purpose   byte
	kid       byte
}

var _ Hasher = (*KeyedHasher)(nil)

func newHasher(key []byte, scope, purpose, kid byte, opts ...HasherOption) (*KeyedHasher, error) {
	if len(key) < 32 {
		return nil, ErrWeakKey
	}
	if !validDataKeyScope(scope) {
		return nil, ErrInvalidKeyScope
	}
	if !validKeyPurpose(purpose) {
		return nil, ErrInvalidKeyPurpose
	}
	if purpose == KeyPurposeSession && scope != KeyScopeMaster {
		return nil, ErrInvalidKeyPurpose
	}
	if !validKeyID(kid) {
		return nil, ErrInvalidKeyID
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
		scope:     scope,
		purpose:   purpose,
		kid:       kid,
	}, nil
}

// NewMasterIndexHasher derives the blind-index key from the master IKM via
// HKDF InfoBlindIndex and stamps master scope, index purpose, and DefaultKeyID.
func NewMasterIndexHasher(ikm []byte, opts ...HasherOption) (*KeyedHasher, error) {
	return newIndexHasher(ikm, KeyScopeMaster, DefaultKeyID, opts...)
}

// NewClinicIndexHasher derives the blind-index key from a clinic DEK via
// HKDF InfoBlindIndex and stamps clinic scope, index purpose, and DefaultKeyID.
func NewClinicIndexHasher(ikm []byte, opts ...HasherOption) (*KeyedHasher, error) {
	return newIndexHasher(ikm, KeyScopeClinic, DefaultKeyID, opts...)
}

// NewHasherFromDEK is NewClinicIndexHasher. Isolation between clinics is this
// derived key, not a per-clinic catalog URN.
func NewHasherFromDEK(dek []byte, opts ...HasherOption) (*KeyedHasher, error) {
	return NewClinicIndexHasher(dek, opts...)
}

// NewSessionHasher stamps master scope, session purpose, and DefaultKeyID.
// key is the PASETO MAC material; it is not HKDF-derived from the master key.
func NewSessionHasher(key []byte, opts ...HasherOption) (*KeyedHasher, error) {
	return newHasher(key, KeyScopeMaster, KeyPurposeSession, DefaultKeyID, opts...)
}

// NewMasterIndexHasherFromBase64 decodes a base64 master IKM and calls NewMasterIndexHasher.
func NewMasterIndexHasherFromBase64(keyB64 string, opts ...HasherOption) (*KeyedHasher, error) {
	raw, err := decodeKeyBase64(keyB64)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(raw)
	return NewMasterIndexHasher(raw, opts...)
}

func newIndexHasher(ikm []byte, scope, kid byte, opts ...HasherOption) (*KeyedHasher, error) {
	if len(ikm) < SizeDEK {
		return nil, ErrWeakKey
	}
	if !validDataKeyScope(scope) {
		return nil, ErrInvalidKeyScope
	}
	if !validKeyID(kid) {
		return nil, ErrInvalidKeyID
	}
	blindKey := hkdfExpand(ikm, InfoBlindIndex)
	defer ZeroBytes(blindKey)
	return newHasher(blindKey, scope, KeyPurposeIndex, kid, opts...)
}

// Algorithm returns the configured hash algorithm.
func (h *KeyedHasher) Algorithm() string {
	return h.algorithm
}

// KeyScope returns the key hierarchy tier stamped into hashes.
func (h *KeyedHasher) KeyScope() byte {
	return h.scope
}

// KeyPurpose returns the key purpose stamped into hashes.
func (h *KeyedHasher) KeyPurpose() byte {
	return h.purpose
}

// KeyID returns the key generation stamped into hashes.
func (h *KeyedHasher) KeyID() byte {
	return h.kid
}

// Hash computes the keyed digest for data and returns "<algorithm>$<scope><purpose>$<kid>$<hex>".
func (h *KeyedHasher) Hash(data []byte) (string, error) {
	digest, err := h.computeDigest(h.algorithm, data)
	if err != nil {
		return "", err
	}
	contextToken, err := keyContextToken(h.scope, h.purpose)
	if err != nil {
		return "", err
	}
	kidToken, err := keyIDToken(h.kid)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%s$%s$%s", h.algorithm, contextToken, kidToken, hex.EncodeToString(digest)), nil
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

	parts := strings.SplitN(encodedHash, "$", hashFormatFields)
	if len(parts) != hashFormatFields || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return false, ErrInvalidHashFormat
	}

	targetAlgo, err := normalizeAlgorithm(parts[0])
	if err != nil {
		return false, err
	}
	scope, purpose, err := parseKeyContextToken(parts[1])
	if err != nil {
		return false, err
	}
	if scope != h.scope {
		return false, ErrKeyScopeMismatch
	}
	if purpose != h.purpose {
		return false, ErrKeyPurposeMismatch
	}
	kid, err := parseKeyIDToken(parts[2])
	if err != nil {
		return false, err
	}
	if kid != h.kid {
		return false, ErrKeyIDMismatch
	}

	expectedBytes, err := hex.DecodeString(parts[3])
	if err != nil {
		return false, errors.Wrap(ErrInvalidHashFormat, "invalid hex encoding")
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

func keyScopeToken(scope byte) (string, error) {
	if !validDataKeyScope(scope) {
		return "", ErrInvalidKeyScope
	}
	return string([]byte{scope}), nil
}

func parseKeyScopeToken(token string) (byte, error) {
	if len(token) != 1 {
		return 0, ErrInvalidKeyScope
	}
	scope := token[0]
	if !validDataKeyScope(scope) {
		return 0, ErrInvalidKeyScope
	}
	return scope, nil
}

func keyPurposeToken(purpose byte) (string, error) {
	if !validKeyPurpose(purpose) {
		return "", ErrInvalidKeyPurpose
	}
	return string([]byte{purpose}), nil
}

func parseKeyPurposeToken(token string) (byte, error) {
	if len(token) != 1 {
		return 0, ErrInvalidKeyPurpose
	}
	purpose := token[0]
	if !validKeyPurpose(purpose) {
		return 0, ErrInvalidKeyPurpose
	}
	return purpose, nil
}

func keyContextToken(scope, purpose byte) (string, error) {
	if purpose == KeyPurposeSession && scope != KeyScopeMaster {
		return "", ErrInvalidKeyPurpose
	}
	scopeToken, err := keyScopeToken(scope)
	if err != nil {
		return "", err
	}
	purposeToken, err := keyPurposeToken(purpose)
	if err != nil {
		return "", err
	}
	return scopeToken + purposeToken, nil
}

func parseKeyContextToken(token string) (byte, byte, error) {
	if len(token) != keyContextTokenSize {
		return 0, 0, ErrInvalidHashFormat
	}
	scope, err := parseKeyScopeToken(token[:1])
	if err != nil {
		return 0, 0, err
	}
	purpose, err := parseKeyPurposeToken(token[1:])
	if err != nil {
		return 0, 0, err
	}
	if purpose == KeyPurposeSession && scope != KeyScopeMaster {
		return 0, 0, ErrInvalidKeyPurpose
	}
	return scope, purpose, nil
}

func validKeyPurpose(purpose byte) bool {
	return purpose == KeyPurposeIndex || purpose == KeyPurposeSession
}

func keyIDToken(kid byte) (string, error) {
	if !validKeyID(kid) {
		return "", ErrInvalidKeyID
	}
	return hex.EncodeToString([]byte{kid}), nil
}

func parseKeyIDToken(token string) (byte, error) {
	if len(token) != 2 {
		return 0, ErrInvalidKeyID
	}
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 1 {
		return 0, ErrInvalidKeyID
	}
	if !validKeyID(raw[0]) {
		return 0, ErrInvalidKeyID
	}
	return raw[0], nil
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
			return nil, errors.Wrap(err, "crypto: blake2s init")
		}
		return h, nil

	case AlgorithmBlake2b:
		h, err := blake2b.New256(key)
		if err != nil {
			return nil, errors.Wrap(err, "crypto: blake2b init")
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
