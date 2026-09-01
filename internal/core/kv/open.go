package kv

import (
	"strings"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/config"
)

type openOptions struct {
	allowVault bool
}

// Option configures Open.
type Option func(*openOptions)

// AllowVault permits backend "vault" (HashiCorp Vault / OpenBao). Only the
// keystore store may set this.
func AllowVault() Option {
	return func(o *openOptions) {
		o.allowVault = true
	}
}

// Open constructs a Store from a keystore, meta, or sessions config block.
func Open(cfg config.KVConfig, opts ...Option) (Store, error) {
	o := openOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Backend))
	switch backend {
	case "", config.BackendBBolt:
		return OpenBBolt(cfg.BBolt.Path)
	case config.BackendNATS:
		return OpenNATS(cfg.NATS.URL, cfg.NATS.Bucket)
	case config.BackendEtcd:
		return OpenEtcd(cfg.Etcd.Endpoints, cfg.Etcd.Prefix)
	case config.BackendVault:
		if !o.allowVault {
			return nil, errors.New("kv: backend \"vault\" is only supported for the keystore")
		}
		return OpenVault(cfg.Vault.Address, cfg.Vault.Token, cfg.Vault.Mount, cfg.Vault.Prefix)
	default:
		return nil, errors.Newf("kv: unsupported backend %q (use %q, %q, %q, or %q)",
			cfg.Backend, config.BackendBBolt, config.BackendNATS, config.BackendEtcd, config.BackendVault)
	}
}
