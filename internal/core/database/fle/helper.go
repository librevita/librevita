package fle

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"entgo.io/ent/schema/field"
	"librevita.org/internal/core/crypto"
)

// EncryptedValueScanner implements field.TypeValueScanner[string] for transparent field encryption.
// In this stateless architecture, the scanner acts as a pass-through format converter,
// while actual cryptographic operations with dynamic context keys are handled by Ent Hooks and Interceptors.
type EncryptedValueScanner struct {
	Domain string
}

// EncryptedString returns an Ent TypeValueScanner for encrypted string fields.
func EncryptedString(domain ...string) EncryptedValueScanner {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return EncryptedValueScanner{Domain: d}
}

// Value converts string to driver.Value.
func (s EncryptedValueScanner) Value(v string) (driver.Value, error) {
	return v, nil
}

// ScanValue returns the destination scanner for database scanning.
func (s EncryptedValueScanner) ScanValue() field.ValueScanner {
	return &sql.NullString{}
}

// FromValue extracts string from scanned database driver.Value.
func (s EncryptedValueScanner) FromValue(v driver.Value) (string, error) {
	if v == nil {
		return "", nil
	}
	nullStr, ok := v.(*sql.NullString)
	if !ok || !nullStr.Valid {
		return "", nil
	}
	return nullStr.String, nil
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

// Value converts *string to driver.Value.
func (s EncryptedPtrValueScanner) Value(v *string) (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	return *v, nil
}

// ScanValue returns the destination scanner for database scanning.
func (s EncryptedPtrValueScanner) ScanValue() field.ValueScanner {
	return &sql.NullString{}
}

// FromValue extracts *string from scanned database driver.Value.
func (s EncryptedPtrValueScanner) FromValue(v driver.Value) (*string, error) {
	if v == nil {
		return nil, nil
	}
	nullStr, ok := v.(*sql.NullString)
	if !ok || !nullStr.Valid {
		return nil, nil
	}
	str := nullStr.String
	return &str, nil
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
			return nil, nil, fmt.Errorf("entfle: marshal payload: %w", err)
		}
	}
	defer crypto.ZeroBytes(data)

	ciphertext, err = encryptor.Encrypt(data, aad)
	if err != nil {
		return nil, nil, fmt.Errorf("entfle: encrypt payload: %w", err)
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
		return fmt.Errorf("entfle: decrypt payload: %w", err)
	}
	defer crypto.ZeroBytes(plaintext)

	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("entfle: unmarshal payload: %w", err)
	}
	return nil
}

// EncryptString encrypts a string via Encryptor and returns the self-contained ciphertext bytes.
func EncryptString(encryptor crypto.Encryptor, text string, aad []byte) ([]byte, error) {
	if text == "" {
		return nil, nil
	}
	if crypto.IsCiphertextString(text) {
		return []byte(text), nil
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
	if !crypto.IsCiphertext(ciphertext) {
		return string(ciphertext), nil
	}
	plaintext, err := encryptor.Decrypt(ciphertext, aad)
	if err != nil {
		legacy := crypto.LegacyAAD()
		if len(aad) == 0 || !bytes.Equal(aad, legacy) {
			plaintext, err = encryptor.Decrypt(ciphertext, legacy)
		}
		if err != nil {
			return "", err
		}
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
