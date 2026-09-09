package acme

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/log"
)

// Module provides the ACME Manager and lifecycle hooks.
var Module = fx.Module("acme",
	fx.Provide(
		ProvideCertStore,
		ProvideDNSProvider,
		NewManager,
	),
	fx.Invoke(registerLifecycle),
)

// ProvideCertStore creates a CertStore based on configuration.
func ProvideCertStore(cfg *config.Config, lc fx.Lifecycle, logger log.Logger) (CertStore, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.ACME.Storage.Backend))
	if backend == "file" {
		return NewFileCertStore(cfg.ACME.Storage.Dir)
	}

	// Default is "keystore"
	keystoreBackend := strings.ToLower(strings.TrimSpace(cfg.Keystore.Backend))
	if keystoreBackend == "" || keystoreBackend == config.BackendBBolt {
		return openDedicatedBBoltCertStore(cfg, lc, logger)
	}
	return openKeystoreKVCertStore(cfg, lc, logger, keystoreBackend)
}

func openDedicatedBBoltCertStore(cfg *config.Config, lc fx.Lifecycle, logger log.Logger) (CertStore, error) {
	bboltPath := filepath.Join(cfg.DataDir, "acme.db")
	logger.Info("initializing acme dedicated bbolt store", log.String("path", bboltPath))
	store, err := kv.OpenBBolt(bboltPath)
	if err != nil {
		return nil, errors.Wrap(err, "acme: open dedicated bbolt store")
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			logger.Info("closing acme dedicated bbolt store")
			return store.Close()
		},
	})
	return NewKVCertStore(store), nil
}

func openKeystoreKVCertStore(cfg *config.Config, lc fx.Lifecycle, logger log.Logger, backend string) (CertStore, error) {
	logger.Info("initializing acme store from keystore backend", log.String("backend", backend))
	store, err := kv.Open(cfg.Keystore, kv.AllowVault())
	if err != nil {
		return nil, errors.Wrap(err, "acme: open keystore kv store")
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			logger.Info("closing acme kv store")
			return store.Close()
		},
	})
	return NewKVCertStore(store), nil
}

// ProvideDNSProvider creates the appropriate DNSProvider for DNS-01 challenges.
func ProvideDNSProvider(cfg *config.Config) DNSProvider {
	if !cfg.ACME.Enabled || cfg.ACME.Challenge != config.ACMEChallengeDNS01 {
		return nil
	}
	switch cfg.ACME.DNS.Provider {
	case config.ACMEDNSProviderCloudflare:
		return NewCloudflareProvider(cfg.ACME.DNS.CloudflareAPIToken, cfg.ACME.DNS.CloudflareZoneID)
	case config.ACMEDNSProviderRFC2136:
		return NewRFC2136Provider(RFC2136Config{
			Nameserver:    cfg.ACME.DNS.RFC2136Nameserver,
			Zone:          cfg.ACME.DNS.RFC2136Zone,
			TSIGKeyName:   cfg.ACME.DNS.RFC2136TSIGKeyName,
			TSIGSecret:    cfg.ACME.DNS.RFC2136TSIGSecret,
			TSIGAlgorithm: cfg.ACME.DNS.RFC2136TSIGAlgorithm,
		})
	case config.ACMEDNSProviderExec:
		return NewExecProvider(cfg.ACME.DNS.ExecScript)
	case config.ACMEDNSProviderMock:
		return NewMockDNSProvider()
	default:
		return nil
	}
}

func registerLifecycle(lc fx.Lifecycle, m *Manager, cfg *config.Config, logger log.Logger) {
	if !cfg.ACME.Enabled {
		return
	}
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() { // #nosec G118 -- Background manager startup outlives Fx start hook.
				startCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				if err := m.Start(startCtx); err != nil {
					logger.Error("acme manager failed to start", log.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			m.Stop()
			return nil
		},
	})
}
