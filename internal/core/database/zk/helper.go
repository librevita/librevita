package zk

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"

	"entgo.io/ent/schema/field"
	"librevita.org/internal/core/crypto"
)

var (
	globalEncryptorMu sync.RWMutex
	globalEncryptor   crypto.Encryptor
)

// SetGlobalEncryptor configures the global encryptor used by ValueScanners.
func SetGlobalEncryptor(e crypto.Encryptor) {
	globalEncryptorMu.Lock()
	defer globalEncryptorMu.Unlock()
	globalEncryptor = e
}

// GetGlobalEncryptor retrieves the active global encryptor.
func GetGlobalEncryptor() crypto.Encryptor {
	globalEncryptorMu.RLock()
	defer globalEncryptorMu.RUnlock()
	return globalEncryptor
}

// EncryptedValueScanner implements field.TypeValueScanner[string] for transparent field encryption.
type EncryptedValueScanner struct {
	Domain string
}

// EncryptedString returns an Ent TypeValueScanner that transparently encrypts strings on write and decrypts on scan.
func EncryptedString(domain ...string) EncryptedValueScanner {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return EncryptedValueScanner{Domain: d}
}

func (s EncryptedValueScanner) Value(v string) (driver.Value, error) {
	if v == "" {
		return "", nil
	}
	if v[0] == crypto.MagicByteXChaCha20Poly1305 {
		return v, nil
	}
	enc := GetGlobalEncryptor()
	if enc == nil {
		return v, nil
	}
	aad := []byte("urn:librevita")
	if s.Domain != "" {
		aad = []byte("urn:librevita:" + s.Domain)
	}
	return EncryptString(enc, v, aad)
}

func (s EncryptedValueScanner) ScanValue() field.ValueScanner {
	return &sql.NullString{}
}

func (s EncryptedValueScanner) FromValue(v driver.Value) (string, error) {
	if v == nil {
		return "", nil
	}
	nullStr, ok := v.(*sql.NullString)
	if !ok || !nullStr.Valid || nullStr.String == "" {
		return "", nil
	}
	raw := nullStr.String
	if len(raw) > 0 && raw[0] == crypto.MagicByteXChaCha20Poly1305 {
		enc := GetGlobalEncryptor()
		if enc != nil {
			aad := []byte("urn:librevita")
			if s.Domain != "" {
				aad = []byte("urn:librevita:" + s.Domain)
			}
			return DecryptString(enc, []byte(raw), aad)
		}
	}
	return raw, nil
}

// EncryptedPtrValueScanner implements field.TypeValueScanner[*string] for transparent nullable field encryption.
type EncryptedPtrValueScanner struct {
	Domain string
}

// EncryptedStringPtr returns an Ent TypeValueScanner for nullable *string fields.
func EncryptedStringPtr(domain ...string) EncryptedPtrValueScanner {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return EncryptedPtrValueScanner{Domain: d}
}

func (s EncryptedPtrValueScanner) Value(v *string) (driver.Value, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	if (*v)[0] == crypto.MagicByteXChaCha20Poly1305 {
		return *v, nil
	}
	enc := GetGlobalEncryptor()
	if enc == nil {
		return *v, nil
	}
	aad := []byte("urn:librevita")
	if s.Domain != "" {
		aad = []byte("urn:librevita:" + s.Domain)
	}
	return EncryptString(enc, *v, aad)
}

func (s EncryptedPtrValueScanner) ScanValue() field.ValueScanner {
	return &sql.NullString{}
}

func (s EncryptedPtrValueScanner) FromValue(v driver.Value) (*string, error) {
	if v == nil {
		return nil, nil
	}
	nullStr, ok := v.(*sql.NullString)
	if !ok || !nullStr.Valid {
		return nil, nil
	}
	raw := nullStr.String
	if len(raw) > 0 && raw[0] == crypto.MagicByteXChaCha20Poly1305 {
		enc := GetGlobalEncryptor()
		if enc != nil {
			aad := []byte("urn:librevita")
			if s.Domain != "" {
				aad = []byte("urn:librevita:" + s.Domain)
			}
			dec, err := DecryptString(enc, []byte(raw), aad)
			if err != nil {
				return nil, err
			}
			return &dec, nil
		}
	}
	return &raw, nil
}

// EncryptPayload serializes a domain struct to JSON, encrypts it via Encryptor,
// and extracts the 24-byte nonce from the resulting ciphertext.
func EncryptPayload(encryptor crypto.Encryptor, payload any, aad []byte) (ciphertext, nonce []byte, err error) {
	var data []byte

	switch p := payload.(type) {
	case []byte:
		data = p
	case string:
		data = []byte(p)
	default:
		data, err = json.Marshal(p)
		if err != nil {
			return nil, nil, fmt.Errorf("entzk: marshal payload: %w", err)
		}
	}
	defer crypto.ZeroBytes(data)

	ciphertext, err = encryptor.Encrypt(data, aad)
	if err != nil {
		return nil, nil, fmt.Errorf("entzk: encrypt payload: %w", err)
	}

	if len(ciphertext) >= 25 {
		nonce = make([]byte, 24)
		copy(nonce, ciphertext[1:25])
	}

	return ciphertext, nonce, nil
}

// DecryptPayload decrypts ciphertext via Encryptor and unmarshals JSON into target.
func DecryptPayload(encryptor crypto.Encryptor, ciphertext, aad []byte, target any) error {
	plaintext, err := encryptor.Decrypt(ciphertext, aad)
	if err != nil {
		return fmt.Errorf("entzk: decrypt payload: %w", err)
	}
	defer crypto.ZeroBytes(plaintext)

	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("entzk: unmarshal payload: %w", err)
	}
	return nil
}

// EncryptString encrypts a string via Encryptor and returns the self-contained ciphertext bytes.
func EncryptString(encryptor crypto.Encryptor, text string, aad []byte) ([]byte, error) {
	if text == "" {
		return nil, nil
	}
	data := []byte(text)
	defer crypto.ZeroBytes(data)
	return encryptor.Encrypt(data, aad)
}

// DecryptString decrypts ciphertext bytes via Encryptor and returns the cleartext string.
func DecryptString(encryptor crypto.Encryptor, ciphertext, aad []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	plaintext, err := encryptor.Decrypt(ciphertext, aad)
	if err != nil {
		return "", err
	}
	defer crypto.ZeroBytes(plaintext)
	return string(plaintext), nil
}

// EncryptStringPtr encrypts a nullable string pointer.
func EncryptStringPtr(encryptor crypto.Encryptor, text *string, aad []byte) ([]byte, error) {
	if text == nil || *text == "" {
		return nil, nil
	}
	return EncryptString(encryptor, *text, aad)
}

// DecryptStringPtr decrypts nullable ciphertext bytes into a string pointer.
func DecryptStringPtr(encryptor crypto.Encryptor, ciphertext, aad []byte) (*string, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	s, err := DecryptString(encryptor, ciphertext, aad)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
