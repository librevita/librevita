package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"

	"librevita.org/internal/core/crypto"
)

// Argon2id parameters. The memory cost must stay a power of two.
const (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16

	// defaultMaxConcurrentHashes bounds concurrent Argon2 operations
	// (~64 MiB and multiple threads each) to protect the process from
	// memory exhaustion under concurrent login attempts.
	defaultMaxConcurrentHashes = 4
)

var errInvalidHash = errors.New("auth: malformed password hash")

var (
	hashSemMu sync.RWMutex
	hashSem   = make(chan struct{}, defaultMaxConcurrentHashes)
)

// SetMaxConcurrentHashes bounds concurrent Argon2 work. Values below one
// are clamped to one. Call it once at startup from configuration.
func SetMaxConcurrentHashes(n int) {
	if n < 1 {
		n = 1
	}
	hashSemMu.Lock()
	hashSem = make(chan struct{}, n)
	hashSemMu.Unlock()
}

// acquireHashSlot blocks until an Argon2 slot is free and returns the
// release function. Argon2 is not cancelable, so this bounds peak memory
// instead of relying on request timeouts.
func acquireHashSlot() func() {
	hashSemMu.RLock()
	sem := hashSem
	hashSemMu.RUnlock()
	sem <- struct{}{}
	return func() { <-sem }
}

// HashPassword derives an Argon2id PHC string from plain.
func HashPassword(plain string) (string, error) {
	release := acquireHashSlot()
	defer release()

	salt, err := crypto.RandomBytes(argonSaltLen)
	if err != nil {
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
	release := acquireHashSlot()
	defer release()

	memory, time, threads, keyLen, salt, key, err := decodeHash(hash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(plain), salt, time, memory, threads, keyLen)
	return crypto.ConstantTimeCompareBytes(candidate, key), nil
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

	// #nosec G115 -- the equality check above pins the params to the
	// binary's constants, so the narrowing casts cannot overflow.
	return uint32(params[0]), uint32(params[1]), uint8(params[2]), uint32(len(key)), salt, key, nil
}
