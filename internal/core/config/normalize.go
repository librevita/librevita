package config

import (
	"net"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

func (c *Config) normalize() {
	c.normalizeHTTP()
	c.normalizeDatabase()
	c.normalizeLogging()
	c.normalizeVaultCrypto()
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

func (c *Config) normalizeVaultCrypto() {
	c.Vault.Backend = strings.ToLower(strings.TrimSpace(c.Vault.Backend))
	if c.Vault.Backend == "" {
		c.Vault.Backend = "bbolt"
	}
	if strings.TrimSpace(c.Vault.BBolt.Path) == "" {
		c.Vault.BBolt.Path = filepath.Join(c.DataDir, "keys.db")
	}
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
	if err := c.validateCrypto(); err != nil {
		return err
	}
	if err := c.validateVault(); err != nil {
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
	return c.validateLogging()
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

func (c *Config) validateVault() error {
	switch c.Vault.Backend {
	case "", "bbolt", "nats", "etcd", "hashicorp", "hashicorp_vault", "openbao":
		return nil
	default:
		return errors.Newf("config: invalid vault.backend %q (use \"bbolt\", \"nats\", \"etcd\", \"hashicorp\", \"hashicorp_vault\", or \"openbao\")", c.Vault.Backend)
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
