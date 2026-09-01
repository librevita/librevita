package crypto

import (
	"encoding/base64"
	"encoding/json"

	"github.com/cockroachdb/errors"
)

// Encryptor provides symmetric AEAD encryption and decryption with cryptographic agility.
type Encryptor interface {
	// Encrypt encrypts plaintext with authenticated associated data (AAD).
	// The returned envelope is:
	//   [0]: Magic Byte (e.g. 0x01 for XChaCha20-Poly1305)
	//   [1]: Key scope (master / clinic / patient)
	//   [2]: Key id (generation)
	//   [3 : 3+NonceSize]: Random cryptographic nonce
	//   [3+NonceSize : ]: Ciphertext and authentication tag
	Encrypt(plaintext, aad []byte) ([]byte, error)

	// Decrypt authenticates and decrypts ciphertext using the AAD.
	// It inspects ciphertext[0] (Magic Byte) to route to the matching engine
	// and rejects ciphertext[1]/[2] when they do not match this Encryptor's
	// key scope and key id.
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

	// KeyScope returns the key hierarchy tier stamped into ciphertext.
	KeyScope() byte

	// KeyID returns the key generation stamped into ciphertext.
	KeyID() byte

	// IsCiphertext reports whether the provided data begins with a recognized ciphertext envelope.
	IsCiphertext(data []byte) bool
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
	scope   byte
	kid     byte
}

var _ Encryptor = (*AEADEncryptor)(nil)

// NewMasterEncryptor stamps master scope (`m`) and DefaultKeyID.
// Public constructors do not take a free scope or kid.
func NewMasterEncryptor(key []byte, opts ...EncryptorOption) (*AEADEncryptor, error) {
	return newEncryptor(key, KeyScopeMaster, DefaultKeyID, opts...)
}

// NewClinicEncryptor stamps clinic scope and DefaultKeyID.
func NewClinicEncryptor(key []byte, opts ...EncryptorOption) (*AEADEncryptor, error) {
	return newEncryptor(key, KeyScopeClinic, DefaultKeyID, opts...)
}

// NewPatientEncryptor stamps patient scope and DefaultKeyID.
func NewPatientEncryptor(key []byte, opts ...EncryptorOption) (*AEADEncryptor, error) {
	return newEncryptor(key, KeyScopePatient, DefaultKeyID, opts...)
}

func newEncryptor(key []byte, scope, kid byte, opts ...EncryptorOption) (*AEADEncryptor, error) {
	if len(key) < 32 {
		return nil, ErrWeakKey
	}
	if !validDataKeyScope(scope) {
		return nil, ErrInvalidKeyScope
	}
	if !validKeyID(kid) {
		return nil, ErrInvalidKeyID
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
		scope:   scope,
		kid:     kid,
	}, nil
}

// NewPatientEncryptorFromBase64 decodes a base64 key and calls NewPatientEncryptor.
func NewPatientEncryptorFromBase64(keyB64 string, opts ...EncryptorOption) (*AEADEncryptor, error) {
	raw, err := decodeKeyBase64(keyB64)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(raw)
	return NewPatientEncryptor(raw, opts...)
}

func decodeKeyBase64(keyB64 string) ([]byte, error) {
	if keyB64 == "" {
		return nil, ErrWeakKey
	}
	raw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: invalid base64 key")
	}
	return raw, nil
}

// Version returns the default Magic Byte version.
func (e *AEADEncryptor) Version() byte {
	return e.version
}

// KeyScope returns the key hierarchy tier stamped into ciphertext.
func (e *AEADEncryptor) KeyScope() byte {
	return e.scope
}

// KeyID returns the key generation stamped into ciphertext.
func (e *AEADEncryptor) KeyID() byte {
	return e.kid
}

// Encrypt encrypts plaintext with AAD, injecting magic, key scope, and key id at the front.
func (e *AEADEncryptor) Encrypt(plaintext, aad []byte) ([]byte, error) {
	spec, ok := supportedCiphers[e.version]
	if !ok {
		return nil, ErrUnsupportedVersion
	}

	aead, err := NewAEADCipherByVersion(e.version, e.key)
	if err != nil {
		return nil, err
	}

	nonce, err := RandomBytes(spec.NonceSize)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: generate nonce")
	}

	header := []byte{e.version, e.scope, e.kid}
	out := make([]byte, CiphertextHeaderSize+len(nonce), CiphertextHeaderSize+len(nonce)+len(plaintext)+aead.Overhead())
	copy(out, header)
	copy(out[CiphertextHeaderSize:], nonce)
	out = aead.Seal(out, nonce, plaintext, appendEnvelopeAAD(header, aad))
	return out, nil
}

// Decrypt inspects the Magic Byte, key scope, and key id, then decrypts the payload.
func (e *AEADEncryptor) Decrypt(ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < CiphertextHeaderSize {
		return nil, ErrCiphertextTooShort
	}

	magicByte := ciphertext[0]
	spec, ok := supportedCiphers[magicByte]
	if !ok {
		return nil, ErrUnsupportedVersion
	}

	scope := ciphertext[1]
	if !validDataKeyScope(scope) {
		return nil, ErrInvalidKeyScope
	}
	if scope != e.scope {
		return nil, ErrKeyScopeMismatch
	}
	kid := ciphertext[2]
	if !validKeyID(kid) {
		return nil, ErrInvalidKeyID
	}
	if kid != e.kid {
		return nil, ErrKeyIDMismatch
	}

	minSize := CiphertextHeaderSize + spec.NonceSize + spec.TagSize
	if len(ciphertext) < minSize {
		return nil, ErrCiphertextTooShort
	}

	header := ciphertext[:CiphertextHeaderSize]
	nonce := ciphertext[CiphertextHeaderSize : CiphertextHeaderSize+spec.NonceSize]
	payload := ciphertext[CiphertextHeaderSize+spec.NonceSize:]

	aead, err := NewAEADCipherByVersion(magicByte, e.key)
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, payload, appendEnvelopeAAD(header, aad))
	if err != nil {
		return nil, errors.Wrapf(ErrDecryptionFailed, "%v", err)
	}
	return plaintext, nil
}

// EncryptStruct serializes a Go struct to JSON and encrypts it.
func (e *AEADEncryptor) EncryptStruct(value any, aad []byte) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: marshal struct")
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
		return errors.Wrap(err, "crypto: unmarshal payload")
	}
	return nil
}

// Cipher returns the canonical cipher name configured for this Encryptor.
func (e *AEADEncryptor) Cipher() string {
	return e.cipher
}

// IsCiphertext reports whether the provided data is a recognized ciphertext payload.
func (e *AEADEncryptor) IsCiphertext(data []byte) bool {
	return IsCiphertext(data)
}
