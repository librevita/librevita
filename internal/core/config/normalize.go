package config

import (
	"encoding/base64"
	"net"
	"path/filepath"
	"strings"

	"librevita.org/pkg/errors"
)

func (c *Config) normalize() {
	c.normalizeHTTP()
	c.normalizeDatabase()
	c.normalizeLogging()
	c.normalizeKV()
	c.normalizeCrypto()
	c.normalizeTLS()
	c.normalizeACME()
}

func (c *Config) normalizeHTTP() {
	c.Mode = strings.TrimSpace(c.Mode)
	if c.Mode == "" {
		c.Mode = defaultMode
	}
	c.HTTPBind = strings.TrimSpace(c.HTTPBind)
	if c.HTTPBind == "" {
		c.HTTPBind = defaultHTTPBind
	}
	if c.HTTPPort <= 0 {
		c.HTTPPort = defaultHTTPPort
	}
	c.BaseDomain = strings.ToLower(strings.TrimSpace(c.BaseDomain))
	if c.BaseDomain == "" && !c.IsProduction() {
		c.BaseDomain = defaultBaseDomain
	}
	c.DataDir = strings.TrimSpace(c.DataDir)
	if c.DataDir == "" {
		c.DataDir = defaultDataDir
	}
}

func (c *Config) normalizeDatabase() {
	c.Database.Driver = strings.ToLower(strings.TrimSpace(c.Database.Driver))
	if c.Database.Driver == "" {
		c.Database.Driver = DriverSQLite
	}
	if strings.TrimSpace(c.Database.SQLite.Path) == "" {
		c.Database.SQLite.Path = filepath.Join(c.DataDir, "librevita.db")
	}
	if strings.TrimSpace(c.Database.Dqlite.Database) == "" {
		c.Database.Dqlite.Database = defaultDqliteDatabase
	}
	c.Database.Dqlite.Addrs = strings.TrimSpace(c.Database.Dqlite.Addrs)
	c.Database.Dqlite.DiscoverySRV = strings.TrimSpace(c.Database.Dqlite.DiscoverySRV)

	c.Database.Postgres.URL = strings.TrimSpace(c.Database.Postgres.URL)
	c.Database.Postgres.Host = strings.TrimSpace(c.Database.Postgres.Host)
	c.Database.Postgres.User = strings.TrimSpace(c.Database.Postgres.User)
	c.Database.Postgres.Password = strings.TrimSpace(c.Database.Postgres.Password)
	c.Database.Postgres.Database = strings.TrimSpace(c.Database.Postgres.Database)
	c.Database.Postgres.SSLMode = strings.TrimSpace(c.Database.Postgres.SSLMode)
	if c.Database.Postgres.Port <= 0 {
		c.Database.Postgres.Port = 5432
	}
	if c.Database.Postgres.MaxOpenConns <= 0 {
		c.Database.Postgres.MaxOpenConns = 25
	}
	if c.Database.Postgres.MaxIdleConns <= 0 {
		c.Database.Postgres.MaxIdleConns = 5
	}
}

func (c *Config) normalizeLogging() {
	c.Logging.Mode = strings.ToLower(strings.TrimSpace(c.Logging.Mode))
	if c.Logging.Mode == "" {
		c.Logging.Mode = defaultLogMode
	}
	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))
	if c.Logging.Level == "" {
		if c.IsProduction() {
			c.Logging.Level = LogLevelInfo
		} else {
			c.Logging.Level = LogLevelDebug
		}
	}
	if strings.TrimSpace(c.Logging.File.Path) == "" {
		c.Logging.File.Path = filepath.Join(c.DataDir, "librevita.log")
	}
	if strings.TrimSpace(c.Logging.Rotating.Path) == "" {
		c.Logging.Rotating.Path = filepath.Join(c.DataDir, "librevita.log")
	}
	if c.Logging.Rotating.MaxSizeMB <= 0 {
		c.Logging.Rotating.MaxSizeMB = defaultLogSizeMB
	}
	if c.Logging.Rotating.MaxBackups < 0 {
		c.Logging.Rotating.MaxBackups = defaultLogBackups
	}
	if c.Logging.Rotating.MaxAgeDays < 0 {
		c.Logging.Rotating.MaxAgeDays = defaultLogAgeDays
	}
	if c.Auth.MaxConcurrentHashes <= 0 {
		c.Auth.MaxConcurrentHashes = defaultMaxConcurrentHashes
	}
}

func (c *Config) normalizeKV() {
	c.normalizeKVBlock(&c.Keystore, "keystore.db", "keystore", "/librevita/keystore/")
	c.normalizeKVBlock(&c.Meta, "meta.db", "meta", "/librevita/meta/")
	c.normalizeKVBlock(&c.Sessions, "sessions.db", "sessions", "/librevita/sessions/")
	if strings.TrimSpace(c.Keystore.Vault.Mount) == "" {
		c.Keystore.Vault.Mount = "secret"
	}
	if strings.TrimSpace(c.Keystore.Vault.Prefix) == "" {
		c.Keystore.Vault.Prefix = "librevita/keystore/"
	}
}

func (c *Config) normalizeKVBlock(block *KVConfig, bboltFile, natsBucket, etcdPrefix string) {
	block.Backend = strings.ToLower(strings.TrimSpace(block.Backend))
	if block.Backend == "" {
		block.Backend = BackendBBolt
	}
	if strings.TrimSpace(block.BBolt.Path) == "" {
		block.BBolt.Path = filepath.Join(c.DataDir, bboltFile)
	}
	if strings.TrimSpace(block.NATS.Bucket) == "" {
		block.NATS.Bucket = natsBucket
	}
	if strings.TrimSpace(block.Etcd.Prefix) == "" {
		block.Etcd.Prefix = etcdPrefix
	}
}

func (c *Config) normalizeCrypto() {
	c.Crypto.HashAlgorithm = strings.ToLower(strings.TrimSpace(c.Crypto.HashAlgorithm))
	if c.Crypto.HashAlgorithm == "" {
		c.Crypto.HashAlgorithm = "blake2s"
	}
	c.Crypto.EncryptionCipher = strings.ToLower(strings.TrimSpace(c.Crypto.EncryptionCipher))
	if c.Crypto.EncryptionCipher == "" {
		c.Crypto.EncryptionCipher = "xchacha20-poly1305"
	}
}

func (c *Config) validate() error {
	if c.IsProduction() && strings.TrimSpace(c.BaseDomain) == "" {
		err := errors.New("config: base_domain is required in production (LIBREVITA_BASE_DOMAIN)")
		return errors.WithHint(err, "Configure a variável de ambiente LIBREVITA_BASE_DOMAIN com o domínio público da instalação (ex: app.librevita.org).")
	}
	if err := c.validateRequiredKeys(); err != nil {
		return err
	}
	if err := c.validateCrypto(); err != nil {
		return err
	}
	if err := c.validateKV(); err != nil {
		return err
	}
	if err := c.validateDatabase(); err != nil {
		return err
	}
	if c.HTTPPort > 65535 {
		return errors.Newf("config: invalid http_port %d (max 65535)", c.HTTPPort)
	}
	if err := c.validateTrustedProxies(); err != nil {
		return err
	}
	if err := c.validateTLS(); err != nil {
		return err
	}
	if err := c.validateACME(); err != nil {
		return err
	}
	return c.validateLogging()
}

func (c *Config) validateRequiredKeys() error {
	if !c.IsDevelopment() {
		if strings.TrimSpace(c.PasetoKey) == "" {
			err := errors.New("config: paseto_key is required outside development (LIBREVITA_PASETO_KEY)")
			return errors.WithHint(err, "Gere uma chave simétrica segura de 32 bytes em base64 (ex: openssl rand -base64 32) e configure LIBREVITA_PASETO_KEY.")
		}
		if strings.TrimSpace(c.MasterKey) == "" {
			err := errors.New("config: master_key is required outside development (LIBREVITA_MASTER_KEY)")
			return errors.WithHint(err, "Gere uma chave simétrica segura de 32 bytes em base64 (ex: openssl rand -base64 32) e configure LIBREVITA_MASTER_KEY.")
		}
	}
	if c.PasetoKey != "" {
		raw, err := base64.StdEncoding.DecodeString(c.PasetoKey)
		if err != nil || len(raw) != 32 {
			return errors.New("config: paseto_key must be a valid base64 32-byte string")
		}
	}
	if c.MasterKey != "" {
		raw, err := base64.StdEncoding.DecodeString(c.MasterKey)
		if err != nil || len(raw) != 32 {
			return errors.New("config: master_key must be a valid base64 32-byte string")
		}
	}
	return nil
}

func (c *Config) validateCrypto() error {
	switch c.Crypto.HashAlgorithm {
	case "", "blake2s", "blake2b":
	default:
		return errors.Newf("config: invalid crypto.hash_algorithm %q (active supported: \"blake2s\", \"blake2b\")", c.Crypto.HashAlgorithm)
	}
	switch c.Crypto.EncryptionCipher {
	case "", "xchacha20-poly1305", "xchacha20poly1305":
	default:
		return errors.Newf("config: invalid crypto.encryption_cipher %q (active supported: \"xchacha20-poly1305\")", c.Crypto.EncryptionCipher)
	}
	return nil
}

func (c *Config) validateKV() error {
	if err := validateKVBackend("keystore", c.Keystore.Backend, true); err != nil {
		return err
	}
	if err := validateKVBackend("meta", c.Meta.Backend, false); err != nil {
		return err
	}
	return validateKVBackend("sessions", c.Sessions.Backend, false)
}

func validateKVBackend(name, backend string, allowVault bool) error {
	switch backend {
	case "", BackendBBolt, BackendNATS, BackendEtcd:
		return nil
	case BackendVault:
		if allowVault {
			return nil
		}
		return errors.Newf("config: %s.backend %q is only supported for keystore", name, backend)
	default:
		if allowVault {
			return errors.Newf("config: invalid %s.backend %q (use %q, %q, %q, or %q)",
				name, backend, BackendBBolt, BackendNATS, BackendEtcd, BackendVault)
		}
		return errors.Newf("config: invalid %s.backend %q (use %q, %q, or %q)",
			name, backend, BackendBBolt, BackendNATS, BackendEtcd)
	}
}

func (c *Config) validateDatabase() error {
	switch c.Database.Driver {
	case DriverSQLite, DriverPostgres, DriverDqlite:
	default:
		return errors.Newf("config: invalid database.driver %q (use %q, %q, or %q)",
			c.Database.Driver, DriverSQLite, DriverPostgres, DriverDqlite)
	}
	if c.Database.Driver != DriverDqlite {
		return nil
	}
	addresses := 0
	for _, addr := range strings.Split(c.Database.Dqlite.Addrs, ",") {
		if strings.TrimSpace(addr) != "" {
			addresses++
		}
	}
	if addresses == 0 && c.Database.Dqlite.DiscoverySRV == "" {
		err := errors.New("config: database.dqlite.addrs requires at least one node address (e.g. \"node1:9001,node2:9001,node3:9001\") or database.dqlite.discovery_srv (an SRV record)")
		return errors.WithHint(err, "Defina LIBREVITA_DATABASE_DQLITE_ADDRS com uma lista de nós ou configure LIBREVITA_DATABASE_DQLITE_DISCOVERY_SRV.")
	}
	return nil
}

func (c *Config) validateTrustedProxies() error {
	// A typo here would silently degrade to trusting the remote address
	// (or worse, trusting the client), so the list is validated at boot.
	if strings.TrimSpace(c.TrustedProxies) == "" {
		return nil
	}
	for _, p := range strings.Split(c.TrustedProxies, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(p); err == nil {
			continue
		}
		if net.ParseIP(p) == nil {
			return errors.Newf("config: invalid trusted_proxies entry %q (use CIDR or IP, comma-separated)", p)
		}
	}
	return nil
}

func (c *Config) validateLogging() error {
	switch c.Logging.Mode {
	case LogModeConsole, LogModeFile, LogModeRotating:
	default:
		return errors.Newf("config: invalid logging.mode %q (use %q, %q, or %q)",
			c.Logging.Mode, LogModeConsole, LogModeFile, LogModeRotating)
	}
	switch c.Logging.Level {
	case "", LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return nil
	default:
		return errors.Newf("config: invalid logging.level %q (use %q, %q, %q, or %q)",
			c.Logging.Level, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError)
	}
}

func (c *Config) normalizeTLS() {
	c.TLS.HTTPSBind = strings.TrimSpace(c.TLS.HTTPSBind)
	if c.TLS.HTTPSBind == "" {
		c.TLS.HTTPSBind = defaultHTTPSBind
	}
	if c.TLS.HTTPSPort <= 0 {
		c.TLS.HTTPSPort = defaultHTTPSPort
	}
}

func (c *Config) normalizeACME() {
	if !c.ACME.Enabled {
		return
	}
	c.TLS.Enabled = true

	c.ACME.Directory = strings.TrimSpace(c.ACME.Directory)
	if c.ACME.Directory == "" || strings.EqualFold(c.ACME.Directory, "production") {
		c.ACME.Directory = ACMEDirectoryProduction
	} else if strings.EqualFold(c.ACME.Directory, "staging") {
		c.ACME.Directory = ACMEDirectoryStaging
	}

	c.ACME.Email = strings.TrimSpace(c.ACME.Email)
	c.ACME.Challenge = strings.ToLower(strings.TrimSpace(c.ACME.Challenge))
	if c.ACME.Challenge == "" {
		c.ACME.Challenge = ACMEChallengeDNS01
	}

	if c.ACME.RenewBeforeDays <= 0 {
		c.ACME.RenewBeforeDays = defaultACMERenewBeforeDays
	}

	if c.ACME.OnDemand == nil {
		onDemand := true
		c.ACME.OnDemand = &onDemand
	}

	c.ACME.Storage.Backend = strings.ToLower(strings.TrimSpace(c.ACME.Storage.Backend))
	if c.ACME.Storage.Backend == "" {
		c.ACME.Storage.Backend = "keystore"
	}
	c.ACME.Storage.Dir = strings.TrimSpace(c.ACME.Storage.Dir)
	if c.ACME.Storage.Dir == "" {
		c.ACME.Storage.Dir = filepath.Join(c.DataDir, "acme")
	}

	c.ACME.DNS.Provider = strings.ToLower(strings.TrimSpace(c.ACME.DNS.Provider))
	if c.ACME.DNS.PropagationTimeoutSec <= 0 {
		c.ACME.DNS.PropagationTimeoutSec = defaultACMEDNSPropagationTimeoutSec
	}

	c.normalizeACMEDomains()
}

func (c *Config) normalizeACMEDomains() {
	if len(c.ACME.Domains) > 0 || c.BaseDomain == "" {
		return
	}
	if c.ACME.Challenge == ACMEChallengeDNS01 {
		c.ACME.Domains = []string{c.BaseDomain, "*." + c.BaseDomain}
	} else {
		c.ACME.Domains = []string{c.BaseDomain, "www." + c.BaseDomain}
	}
}

func (c *Config) validateTLS() error {
	if !c.TLS.Enabled {
		return nil
	}
	if c.TLS.HTTPSPort > 65535 {
		return errors.Newf("config: invalid tls.https_port %d (max 65535)", c.TLS.HTTPSPort)
	}
	if c.TLS.CertFile != "" && c.TLS.KeyFile == "" {
		return errors.New("config: tls.key_file is required when tls.cert_file is specified")
	}
	if c.TLS.KeyFile != "" && c.TLS.CertFile == "" {
		return errors.New("config: tls.cert_file is required when tls.key_file is specified")
	}
	return nil
}

func (c *Config) validateACME() error {
	if !c.ACME.Enabled {
		return nil
	}
	if c.IsDevelopment() {
		return nil
	}
	if c.ACME.Email == "" {
		return errors.New("config: acme.email is required when acme is enabled")
	}
	if err := c.validateACMEChallenge(); err != nil {
		return err
	}
	switch c.ACME.Storage.Backend {
	case "file", "keystore":
	default:
		return errors.Newf("config: invalid acme.storage.backend %q (must be \"file\" or \"keystore\")", c.ACME.Storage.Backend)
	}
	return nil
}

func (c *Config) validateACMEChallenge() error {
	switch c.ACME.Challenge {
	case ACMEChallengeDNS01:
		return c.validateACMEDNSProvider()
	case ACMEChallengeHTTP01:
		for _, domain := range c.ACME.Domains {
			if strings.HasPrefix(domain, "*.") {
				return errors.Newf("config: domain %q is a wildcard, which is not supported by acme http-01 challenge (use dns-01)", domain)
			}
		}
		return nil
	default:
		return errors.Newf("config: invalid acme.challenge %q (must be \"http-01\" or \"dns-01\")", c.ACME.Challenge)
	}
}

func (c *Config) validateACMEDNSProvider() error {
	switch c.ACME.DNS.Provider {
	case ACMEDNSProviderCloudflare:
		if c.ACME.DNS.CloudflareAPIToken == "" {
			return errors.New("config: acme.dns.cloudflare_api_token is required for cloudflare dns provider")
		}
	case ACMEDNSProviderRFC2136:
		if c.ACME.DNS.RFC2136Nameserver == "" {
			return errors.New("config: acme.dns.rfc2136_nameserver is required for rfc2136 dns provider")
		}
	case ACMEDNSProviderExec:
		if c.ACME.DNS.ExecScript == "" {
			return errors.New("config: acme.dns.exec_script is required for exec dns provider")
		}
	case ACMEDNSProviderMock:
		// allowed for test
	default:
		return errors.Newf("config: invalid or missing acme.dns.provider %q (must be \"cloudflare\", \"rfc2136\", \"exec\", or \"mock\")", c.ACME.DNS.Provider)
	}
	return nil
}
