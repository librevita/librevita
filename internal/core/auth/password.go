package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. The memory cost must stay a power of two.
const (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

var errInvalidHash = errors.New("auth: malformed password hash")

// HashPassword derives an Argon2id PHC string from plain.
func HashPassword(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: password salt: %w", err)
	}

	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks plain against a stored Argon2id PHC string.
// A malformed stored hash is an error, not a mismatch.
func VerifyPassword(hash, plain string) (bool, error) {
	memory, time, threads, keyLen, salt, key, err := decodeHash(hash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(plain), salt, time, memory, threads, keyLen)
	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

func decodeHash(hash string) (memory, time uint32, threads uint8, keyLen uint32, salt, key []byte, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}

	var params [3]uint64
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params[0], &params[1], &params[2]); err != nil {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(salt) == 0 || len(key) == 0 {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}

	// Reject hashes derived with stronger parameters than this binary.
	if params[0] != uint64(argonMemory) || params[1] != uint64(argonTime) || params[2] != uint64(argonThreads) {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}

	return uint32(params[0]), uint32(params[1]), uint8(params[2]), uint32(len(key)), salt, key, nil
}
