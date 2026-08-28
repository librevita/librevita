package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module provides cryptographic contracts (Hasher, Encryptor, Engine)
// configured via application configuration and injected KeyVault.
var Module = fx.Module("crypto",
	fx.Provide(
		NewHasherFromConfig,
		NewEncryptorFromConfig,
		NewFromConfig,
	),
)

// NewHasherFromConfig provides a Hasher instance configured from application settings.
func NewHasherFromConfig(cfg *config.Config, log *slog.Logger) (Hasher, error) {
	rawMasterKey, err := resolveMasterKey(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("crypto: hasher init: %w", err)
	}
	defer ZeroBytes(rawMasterKey)

	blindKey := hkdfExpand(rawMasterKey, InfoBlindIndex)
	defer ZeroBytes(blindKey)

	algo := cfg.Crypto.HashAlgorithm
	if algo == "" {
		algo = DefaultHashAlgorithm
	}

	return NewHasher(blindKey, WithHashAlgorithm(algo))
}

// NewEncryptorFromConfig provides an Encryptor instance configured from application settings.
func NewEncryptorFromConfig(cfg *config.Config, log *slog.Logger) (Encryptor, error) {
	rawMasterKey, err := resolveMasterKey(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("crypto: encryptor init: %w", err)
	}
	defer ZeroBytes(rawMasterKey)

	kek := hkdfExpand(rawMasterKey, InfoKEK)
	defer ZeroBytes(kek)

	cipher := cfg.Crypto.EncryptionCipher
	if cipher == "" {
		cipher = DefaultEncryptionCipher
	}

	return NewEncryptor(kek, WithEncryptionCipher(cipher))
}

// NewFromConfig is the Fx provider for Engine/MasterKey with KeyVault.
func NewFromConfig(cfg *config.Config, vault KeyVault, log *slog.Logger) (*Engine, error) {
	if vault == nil {
		return nil, errors.New("crypto: key vault is required")
	}

	rawMasterKey, err := resolveMasterKey(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("crypto: engine init: %w", err)
	}
	defer ZeroBytes(rawMasterKey)

	algo := cfg.Crypto.HashAlgorithm
	if algo == "" {
		algo = DefaultHashAlgorithm
	}

	cipher := cfg.Crypto.EncryptionCipher
	if cipher == "" {
		cipher = DefaultEncryptionCipher
	}

	return deriveEngine(rawMasterKey, vault, WithEngineHashAlgorithm(algo), WithEngineEncryptionCipher(cipher))
}

func resolveMasterKey(cfg *config.Config, log *slog.Logger) ([]byte, error) {
	masterKey := cfg.MasterKey
	if masterKey == "" {
		if !cfg.IsDevelopment() {
			return nil, errors.New("master key is required outside development (LIBREVITA_MASTER_KEY)")
		}
		log.Warn("no master key configured; using an ephemeral key (encrypted values reset on restart)")
		raw, err := RandomBytes(SizeDEK)
		if err != nil {
			return nil, fmt.Errorf("generate ephemeral master key: %w", err)
		}
		return raw, nil
	}

	raw, err := base64.StdEncoding.DecodeString(masterKey)
	if err != nil {
		return nil, fmt.Errorf("master key is not valid base64: %w", err)
	}
	if len(raw) != SizeDEK {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(raw))
	}
	return raw, nil
}
