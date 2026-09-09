package acme

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
)

func TestProvideCertStoreFile(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
		ACME: config.ACMEConfig{
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     filepath.Join(dataDir, "acme"),
			},
		},
	}

	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()

	store, err := ProvideCertStore(cfg, lc, logger)
	require.NoError(t, err)
	assert.IsType(t, &FileCertStore{}, store)
}

func TestProvideCertStoreKeystoreBBolt(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
		Keystore: config.KVConfig{
			Backend: config.BackendBBolt,
			BBolt: config.BBoltConfig{
				Path: filepath.Join(dataDir, "keystore.db"),
			},
		},
		ACME: config.ACMEConfig{
			Storage: config.ACMEStorageConfig{
				Backend: "keystore",
			},
		},
	}

	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()

	store, err := ProvideCertStore(cfg, lc, logger)
	require.NoError(t, err)
	assert.IsType(t, &KVCertStore{}, store)

	// Verify acme.db was created in DataDir
	acmeDBPath := filepath.Join(dataDir, "acme.db")
	_, err = os.Stat(acmeDBPath)
	require.NoError(t, err, "acme.db should exist")

	// Save and load key to ensure it works
	ctx := context.Background()
	key, err := GeneratePrivateKey()
	require.NoError(t, err)
	err = store.SaveAccountKey(ctx, key)
	require.NoError(t, err)

	loadedKey, err := store.LoadAccountKey(ctx)
	require.NoError(t, err)
	assert.NotNil(t, loadedKey)

	// Stop lifecycle to verify close hook runs cleanly
	require.NoError(t, lc.Stop(ctx))
}

func TestProvideCertStoreDefaultIsKeystore(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
		Keystore: config.KVConfig{
			Backend: config.BackendBBolt,
			BBolt: config.BBoltConfig{
				Path: filepath.Join(dataDir, "keystore.db"),
			},
		},
		ACME: config.ACMEConfig{
			Storage: config.ACMEStorageConfig{
				Backend: "", // omitted / default
			},
		},
	}

	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()

	store, err := ProvideCertStore(cfg, lc, logger)
	require.NoError(t, err)
	assert.IsType(t, &KVCertStore{}, store)

	acmeDBPath := filepath.Join(dataDir, "acme.db")
	_, err = os.Stat(acmeDBPath)
	require.NoError(t, err, "acme.db should be created by default")
	require.NoError(t, lc.Stop(context.Background()))
}

func TestProvideCertStoreKeystoreInvalidBackend(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
		Keystore: config.KVConfig{
			Backend: "nonexistent",
		},
		ACME: config.ACMEConfig{
			Storage: config.ACMEStorageConfig{
				Backend: "keystore",
			},
		},
	}

	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()

	_, err := ProvideCertStore(cfg, lc, logger)
	assert.Error(t, err)
}
