package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// Supported encryption ciphers and magic byte identifiers.
const (
	// CipherXChaCha20Poly1305 is the canonical identifier for XChaCha20-Poly1305 AEAD.
	CipherXChaCha20Poly1305 = "xchacha20-poly1305"

	// DefaultEncryptionCipher is the default encryption cipher.
	DefaultEncryptionCipher = CipherXChaCha20Poly1305

	// MagicByteXChaCha20Poly1305 indicates XChaCha20-Poly1305 AEAD with a 24-byte nonce.
	MagicByteXChaCha20Poly1305 byte = 0x01

	// DefaultEncryptionVersion is the default encryption Magic Byte (XChaCha20-Poly1305).
	DefaultEncryptionVersion = MagicByteXChaCha20Poly1305
)

// Encryptor provides symmetric AEAD encryption and decryption with cryptographic agility.
type Encryptor interface {
	// Encrypt encrypts plaintext with authenticated associated data (AAD),
	// injecting a Magic Byte version into the first position ([0]) of the returned slice:
	//   [0]: Magic Byte (e.g. 0x01 for XChaCha20-Poly1305, 0x02 for AES-256-GCM)
	//   [1 : 1+NonceSize]: Random cryptographic nonce
	//   [1+NonceSize : ]: Ciphertext and authentication tag
	Encrypt(plaintext, aad []byte) ([]byte, error)

	// Decrypt authenticates and decrypts ciphertext using the AAD.
	// It inspects ciphertext[0] (Magic Byte) to dynamically route to the matching
	// decryption engine and nonce size.
	Decrypt(ciphertext, aad []byte) ([]byte, error)

	// EncryptStruct serializes value to JSON and encrypts it with AAD.
	// Wipes transient plaintext JSON buffers with ZeroBytes upon completion.
	EncryptStruct(value any, aad []byte) ([]byte, error)

	// DecryptInto decrypts ciphertext with AAD and unmarshals JSON into target.
	// Wipes transient plaintext buffers with ZeroBytes upon completion.
	DecryptInto(ciphertext, aad []byte, target any) error

	// Version returns the default Magic Byte version configured for this Encryptor.
	Version() byte

	// Cipher returns the canonical cipher name configured for this Encryptor.
	Cipher() string
}

// EncryptorOption configures an Encryptor instance.
type EncryptorOption func(*encryptorOptions)

type encryptorOptions struct {
	cipher  string
	version byte
}

// WithEncryptionCipher sets the default encryption cipher by name (e.g. "xchacha20-poly1305").
func WithEncryptionCipher(cipher string) EncryptorOption {
	return func(o *encryptorOptions) {
		o.cipher = cipher
	}
}

// WithEncryptionVersion sets the default encryption version.
func WithEncryptionVersion(v byte) EncryptorOption {
	return func(o *encryptorOptions) {
		o.version = v
	}
}

// AEADEncryptor implements Encryptor with multi-engine AEAD agility and memory safety.
type AEADEncryptor struct {
	key     []byte
	cipher  string
	version byte
}

var _ Encryptor = (*AEADEncryptor)(nil)

// NewEncryptor creates an AEADEncryptor from a raw 32-byte key.
// Fails fast if key size is less than 32 bytes or if cipher/version is unsupported.
func NewEncryptor(key []byte, opts ...EncryptorOption) (*AEADEncryptor, error) {
	if len(key) < 32 {
		return nil, ErrWeakKey
	}

	options := encryptorOptions{
		cipher:  DefaultEncryptionCipher,
		version: DefaultEncryptionVersion,
	}
	for _, opt := range opts {
		opt(&options)
	}

	version, cipher, err := resolveCipherAndVersion(options.cipher, options.version)
	if err != nil {
		return nil, err
	}

	if !isValidVersion(version) {
		return nil, ErrUnsupportedVersion
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &AEADEncryptor{
		key:     keyCopy,
		cipher:  cipher,
		version: version,
	}, nil
}

// NewEncryptorFromBase64 creates an AEADEncryptor from a base64-encoded key string.
func NewEncryptorFromBase64(keyB64 string, opts ...EncryptorOption) (*AEADEncryptor, error) {
	if keyB64 == "" {
		return nil, ErrWeakKey
	}
	raw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid base64 key: %w", err)
	}
	defer ZeroBytes(raw)
	return NewEncryptor(raw, opts...)
}

// Version returns the default Magic Byte version.
func (e *AEADEncryptor) Version() byte {
	return e.version
}

// Encrypt encrypts plaintext with AAD, injecting the Magic Byte version at index [0].
func (e *AEADEncryptor) Encrypt(plaintext, aad []byte) ([]byte, error) {
	switch e.version {
	case MagicByteXChaCha20Poly1305:
		aead, err := chacha20poly1305.NewX(e.key)
		if err != nil {
			return nil, fmt.Errorf("crypto: xchacha20poly1305 init: %w", err)
		}
		nonce := make([]byte, chacha20poly1305.NonceSizeX)
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("crypto: generate nonce: %w", err)
		}

		out := make([]byte, 1+len(nonce), 1+len(nonce)+len(plaintext)+aead.Overhead())
		out[0] = MagicByteXChaCha20Poly1305
		copy(out[1:], nonce)
		out = aead.Seal(out, nonce, plaintext, aad)
		return out, nil

	default:
		return nil, ErrUnsupportedVersion
	}
}

// Decrypt inspects the Magic Byte at ciphertext[0] and decrypts the payload.
func (e *AEADEncryptor) Decrypt(ciphertext, aad []byte) ([]byte, error) {
	// Minimum possible payload: 1 byte version + 24 byte nonce + 16 byte tag = 41 bytes
	if len(ciphertext) < 1+chacha20poly1305.NonceSizeX+16 {
		return nil, ErrCiphertextTooShort
	}

	magicByte := ciphertext[0]
	switch magicByte {
	case MagicByteXChaCha20Poly1305:
		nonceSize := chacha20poly1305.NonceSizeX
		nonce := ciphertext[1 : 1+nonceSize]
		payload := ciphertext[1+nonceSize:]

		aead, err := chacha20poly1305.NewX(e.key)
		if err != nil {
			return nil, fmt.Errorf("crypto: xchacha20poly1305 init: %w", err)
		}

		plaintext, err := aead.Open(nil, nonce, payload, aad)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
		}
		return plaintext, nil

	default:
		return nil, ErrUnsupportedVersion
	}
}

// EncryptStruct serializes a Go struct to JSON and encrypts it.
func (e *AEADEncryptor) EncryptStruct(value any, aad []byte) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("crypto: marshal struct: %w", err)
	}
	defer ZeroBytes(data)
	return e.Encrypt(data, aad)
}

// DecryptInto decrypts ciphertext and unmarshals JSON into target.
func (e *AEADEncryptor) DecryptInto(ciphertext, aad []byte, target any) error {
	plaintext, err := e.Decrypt(ciphertext, aad)
	if err != nil {
		return err
	}
	defer ZeroBytes(plaintext)

	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("crypto: unmarshal payload: %w", err)
	}
	return nil
}

// Cipher returns the canonical cipher name configured for this Encryptor.
func (e *AEADEncryptor) Cipher() string {
	return e.cipher
}

func isValidVersion(v byte) bool {
	// Active version in the current phase
	return v == MagicByteXChaCha20Poly1305
}

func resolveCipherAndVersion(cipherName string, version byte) (byte, string, error) {
	if cipherName != "" && cipherName != DefaultEncryptionCipher {
		normalized := strings.ToLower(strings.TrimSpace(cipherName))
		switch normalized {
		case CipherXChaCha20Poly1305, "xchacha20poly1305":
			return MagicByteXChaCha20Poly1305, CipherXChaCha20Poly1305, nil
		default:
			return 0, "", fmt.Errorf("%w: invalid encryption cipher %q", ErrUnsupportedVersion, cipherName)
		}
	}

	switch version {
	case MagicByteXChaCha20Poly1305:
		return MagicByteXChaCha20Poly1305, CipherXChaCha20Poly1305, nil
	default:
		return 0, "", ErrUnsupportedVersion
	}
}
