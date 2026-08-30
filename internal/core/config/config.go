// Package config loads LibreVita configuration.
//
// Sources are merged by Koanf in this order: defaults, file, environment/.env,
// and flags.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// Supported persistence drivers.
const (
	DriverSQLite   = "sqlite"   // Embedded SQLite for the monolith and edge deployments.
	DriverPostgres = "postgres" // PostgreSQL for scalable cloud deployments.
	DriverDqlite   = "dqlite"   // dqlite cluster for distributed deployments.
)

// Supported production log destinations.
const (
	LogModeConsole  = "console"
	LogModeFile     = "file"
	LogModeRotating = "rotating"

	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

const (
	envPrefix = "LIBREVITA_"

	keyConfigFile = "config_file"

	defaultMode           = "development"
	defaultHTTPBind       = "0.0.0.0"
	defaultHTTPPort       = 8080
	defaultDataDir        = "./data"
	defaultDqliteDatabase = "librevita"
	defaultLogMode        = LogModeConsole
	defaultLogSizeMB      = 100
	defaultLogBackups     = 3
	defaultLogAgeDays     = 28

	defaultMaxConcurrentHashes = 4
	defaultBaseDomain          = "lv.test"
)

// Config is the application configuration root.
type Config struct {
	// ConfigFile is the file that was loaded, if any.
	ConfigFile string `koanf:"config_file"`

	// Mode is the runtime mode: "development" or "production". Every
	// value other than "development" is treated as a persistent
	// deployment (secrets required, Secure cookies).
	Mode string `koanf:"mode"`

	// HTTPBind is the address the HTTP server binds to, e.g. "0.0.0.0"
	// (all interfaces) or "127.0.0.1" (loopback only).
	HTTPBind string `koanf:"http_bind"`

	// HTTPPort is the TCP port the HTTP server listens on.
	HTTPPort int `koanf:"http_port"`

	// BaseDomain is the DNS suffix used to resolve clinics from Host
	// (`{slug}.{base_domain}`). Apex is this value or `www.` plus this
	// value. Required outside development; defaults to lv.test in
	// development.
	BaseDomain string `koanf:"base_domain"`

	// TrustedProxies is a comma-separated list of proxy addresses whose
	// X-Forwarded-For header is trusted for rate limiting and audit IPs.
	// Empty means the app is not behind a proxy and the remote address is
	// used directly.
	TrustedProxies string `koanf:"trusted_proxies"`

	// HSTSMaxAge enables the Strict-Transport-Security header with this
	// max-age in seconds when > 0. Only meaningful over HTTPS; keep it 0
	// (the default) on plain-HTTP deployments, where the header would
	// make the site unreachable for the whole window.
	HSTSMaxAge int `koanf:"hsts_max_age"`

	// DataDir is the base directory for default database and log files.
	DataDir string `koanf:"data_dir"`

	// Database selects the persistence backend.
	Database DatabaseConfig `koanf:"database"`

	// Logging controls the production log destination and rotation policy.
	Logging LoggingConfig `koanf:"logging"`

	// Auth tunes authentication behavior.
	Auth AuthConfig `koanf:"auth"`

	// Storage selects the file storage backend (local directory or
	// S3-compatible API).
	Storage StorageConfig `koanf:"storage"`

	// Vault selects the key vault storage backend (embedded bbolt or memory).
	Vault VaultConfig `koanf:"vault"`

	// PasetoKey is the base64-encoded 32-byte key for PASETO v4.local
	// session tokens. Required outside development; generated at startup
	// otherwise.
	PasetoKey string `koanf:"paseto_key"`

	// MasterKey is the base64-encoded 32-byte master key for field-level
	// encryption and blind indexes of patient identifiers. Required
	// outside development; an ephemeral key is generated at startup
	// otherwise (previously encrypted values become undecryptable on
	// restart).
	MasterKey string `koanf:"master_key"`

	// Crypto configures cryptographic agility defaults (hashing algorithm and encryption version).
	Crypto CryptoConfig `koanf:"crypto"`
}

// CryptoConfig defines defaults for cryptographic hashing and symmetric AEAD encryption.
type CryptoConfig struct {
	// HashAlgorithm is the default hash engine: "blake2s" (default) or "blake2b".
	HashAlgorithm string `koanf:"hash_algorithm"`

	// EncryptionCipher is the default symmetric encryption cipher: "xchacha20-poly1305" (default).
	EncryptionCipher string `koanf:"encryption_cipher"`
}

// VaultConfig defines the active key vault backend. The backend-specific
// settings live in their own section (bbolt), mirroring DatabaseConfig
// and StorageConfig.
type VaultConfig struct {
	// Backend is "bbolt" (default), "nats", "etcd", or "hashicorp" ("vault").
	Backend string `koanf:"backend"`

	// BBolt configures the embedded bbolt key vault.
	BBolt BBoltConfig `koanf:"bbolt"`

	// NATS configures the NATS JetStream key vault.
	NATS NATSVaultConfig `koanf:"nats"`

	// Etcd configures the etcd v3 key vault.
	Etcd EtcdVaultConfig `koanf:"etcd"`

	// HashiCorp configures the HashiCorp Vault / OpenBao key vault.
	HashiCorp HashiCorpVaultConfig `koanf:"hashicorp"`
}

// BBoltConfig configures the embedded bbolt key vault.
type BBoltConfig struct {
	// Path is the bbolt key vault database file path.
	Path string `koanf:"path"`
}

// NATSVaultConfig configures the NATS JetStream KeyValue vault.
type NATSVaultConfig struct {
	URL    string `koanf:"url"`
	Bucket string `koanf:"bucket"`
}

// EtcdVaultConfig configures the etcd v3 KeyValue vault.
type EtcdVaultConfig struct {
	Endpoints string `koanf:"endpoints"`
	Prefix    string `koanf:"prefix"`
}

// HashiCorpVaultConfig configures the HashiCorp Vault / OpenBao key vault.
type HashiCorpVaultConfig struct {
	Address string `koanf:"address"`
	Token   string `koanf:"token"`
	Mount   string `koanf:"mount"`
}

// AuthConfig controls authentication behavior.
type AuthConfig struct {
	// MaxConcurrentHashes bounds concurrent Argon2id operations.
	MaxConcurrentHashes int `koanf:"max_concurrent_hashes"`
}

// DatabaseConfig defines the active persistence backend. The
// backend-specific settings live in their own section (sqlite,
// postgres, or dqlite), mirroring the storage configuration.
type DatabaseConfig struct {
	// Driver is DriverSQLite, DriverPostgres, or DriverDqlite.
	Driver string `koanf:"driver"`

	// SQLite configures the embedded SQLite backend.
	SQLite SQLiteConfig `koanf:"sqlite"`

	// Postgres configures the PostgreSQL backend.
	Postgres PostgresConfig `koanf:"postgres"`

	// Dqlite configures the dqlite cluster backend.
	Dqlite DqliteConfig `koanf:"dqlite"`
}

// SQLiteConfig configures the embedded SQLite backend.
type SQLiteConfig struct {
	// Path is the SQLite database file path.
	Path string `koanf:"path"`
}

// PostgresConfig configures the PostgreSQL backend.
type PostgresConfig struct {
	// URL is the full connection string / DSN (e.g. "postgres://user:pass@host:5432/dbname?sslmode=disable").
	URL string `koanf:"url"`
	// Host is the PostgreSQL server host.
	Host string `koanf:"host"`
	// Port is the PostgreSQL server port (default 5432).
	Port int `koanf:"port"`
	// User is the PostgreSQL username.
	User string `koanf:"user"`
	// Password is the PostgreSQL password.
	Password string `koanf:"password"`
	// Database is the PostgreSQL database name.
	Database string `koanf:"database"`
	// SSLMode is the PostgreSQL SSL mode (disable, require, verify-ca, verify-full; default "disable").
	SSLMode string `koanf:"sslmode"`
	// MaxOpenConns is the maximum number of open connections (default 25).
	MaxOpenConns int `koanf:"max_open_conns"`
	// MaxIdleConns is the maximum number of idle connections (default 5).
	MaxIdleConns int `koanf:"max_idle_conns"`
}

// DSN returns the PostgreSQL connection string.
func (p *PostgresConfig) DSN() string {
	if strings.TrimSpace(p.URL) != "" {
		return strings.TrimSpace(p.URL)
	}
	host := p.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := p.Port
	if port <= 0 {
		port = 5432
	}
	ssl := p.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	user := p.User
	if user == "" {
		user = "postgres"
	}
	db := p.Database
	if db == "" {
		db = "librevita"
	}
	if p.Password != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(user), url.QueryEscape(p.Password), host, port, db, ssl)
	}
	return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(user), host, port, db, ssl)
}

// DqliteConfig configures the dqlite cluster backend.
type DqliteConfig struct {
	// Addrs is the comma-separated list of dqlite node addresses (the
	// wire protocol), e.g. "node1:9001,node2:9001,node3:9001". It is
	// the bootstrap candidate list; either Addrs or DiscoverySRV is
	// required.
	Addrs string `koanf:"addrs"`

	// DiscoverySRV optionally names a DNS SRV record (e.g.
	// "_dqlite._tcp.librevita.svc.cluster.local") whose targets seed
	// the candidate node list in place of static Addrs. The record is
	// queried on every reconnect, so cluster membership can change
	// without restarting the application.
	DiscoverySRV string `koanf:"discovery_srv"`

	// Database is the database name on the dqlite cluster.
	Database string `koanf:"database"`
}

// LoggingConfig controls production logging. The destination-specific
// settings live in their own section (console, file or rotating,
// selected by Mode), mirroring the storage and database configuration.
type LoggingConfig struct {
	// Mode is LogModeConsole, LogModeFile or LogModeRotating.
	Mode string `koanf:"mode"`

	// Level is LogLevelDebug, LogLevelInfo, LogLevelWarn or LogLevelError.
	// Empty means debug in development and info in production.
	Level string `koanf:"level"`

	// Console configures stderr output (no settings today).
	Console ConsoleLogConfig `koanf:"console"`

	// File configures the plain append-only file output.
	File FileLogConfig `koanf:"file"`

	// Rotating configures the lumberjack rotating file output.
	Rotating RotatingLogConfig `koanf:"rotating"`
}

// ConsoleLogConfig configures stderr output. Kept as a section so the
// per-mode layout stays uniform; there are no knobs yet.
type ConsoleLogConfig struct{}

// FileLogConfig configures the plain, append-only file output.
type FileLogConfig struct {
	// Path is the file destination.
	Path string `koanf:"path"`
}

// RotatingLogConfig configures the lumberjack rotating file output.
type RotatingLogConfig struct {
	// Path is the rotating file destination.
	Path string `koanf:"path"`

	// MaxSizeMB is the size at which the file rotates, in megabytes.
	MaxSizeMB int `koanf:"max_size_mb"`

	// MaxBackups is the number of rotated files kept.
	MaxBackups int `koanf:"max_backups"`

	// MaxAgeDays is the maximum age of rotated files, in days.
	MaxAgeDays int `koanf:"max_age_days"`

	// Compress compresses rotated files.
	Compress bool `koanf:"compress"`
}

// StorageConfig selects the file storage backend.
type StorageConfig struct {
	// Backend is "local" (default) or "s3".
	Backend string      `koanf:"backend"`
	Local   LocalConfig `koanf:"local"`
	S3      S3Config    `koanf:"s3"`
}

// LocalConfig configures the directory backend.
type LocalConfig struct {
	// Dir is the storage root; empty defaults to <data_dir>/files.
	Dir string `koanf:"dir"`
}

// S3Config configures an S3-compatible API (MinIO, Garage, ...).
type S3Config struct {
	// Endpoint is the API base, e.g. "minio.example.org:9000".
	Endpoint string `koanf:"endpoint"`
	// Bucket stores every object.
	Bucket string `koanf:"bucket"`
	// AccessKey and SecretKey are the API credentials.
	AccessKey string `koanf:"access_key"`
	SecretKey string `koanf:"secret_key"`
	// Region is used for signature calculation; may be empty outside
	// AWS.
	Region string `koanf:"region"`
	// Secure selects HTTPS.
	Secure bool `koanf:"secure"`
	// PathStyle forces path-style addressing; on by default for
	// S3-compatible servers.
	PathStyle bool `koanf:"path_style"`
}

// RegisterFlags registers the application flags. It is safe to call more
// than once.
func RegisterFlags(fs *pflag.FlagSet) {
	stringFlag(fs, "config", "", "configuration file (.yaml, .yml, or .json)")
	stringFlag(fs, "mode", defaultMode, "runtime mode: development or production")
	stringFlag(fs, "http-bind", defaultHTTPBind, "HTTP bind address (0.0.0.0, 127.0.0.1, ...)")
	intFlag(fs, "http-port", defaultHTTPPort, "HTTP listen port")
	stringFlag(fs, "base-domain", "", "DNS suffix for clinic hosts ({slug}.{base-domain}; default lv.test in development)")
	stringFlag(fs, "trusted-proxies", "", "comma-separated proxy IPs allowed to set X-Forwarded-For")
	intFlag(fs, "hsts-max-age", 0, "Strict-Transport-Security max-age in seconds (0 disables; HTTPS deployments only)")
	stringFlag(fs, "data-dir", defaultDataDir, "base directory for database and logs")
	stringFlag(fs, "db-driver", DriverSQLite, "database backend: sqlite, postgres, or dqlite")
	stringFlag(fs, "db-sqlite-path", "", "SQLite database path")
	stringFlag(fs, "db-postgres-url", "", "PostgreSQL connection string (DSN)")
	stringFlag(fs, "db-postgres-host", "", "PostgreSQL host")
	intFlag(fs, "db-postgres-port", 5432, "PostgreSQL port")
	stringFlag(fs, "db-postgres-user", "", "PostgreSQL user")
	stringFlag(fs, "db-postgres-password", "", "PostgreSQL password")
	stringFlag(fs, "db-postgres-database", "", "PostgreSQL database name")
	stringFlag(fs, "db-postgres-sslmode", "disable", "PostgreSQL SSL mode (disable, require, verify-ca, verify-full)")
	intFlag(fs, "db-postgres-max-open-conns", 25, "PostgreSQL max open connections")
	intFlag(fs, "db-postgres-max-idle-conns", 5, "PostgreSQL max idle connections")
	stringFlag(fs, "db-dqlite-addrs", "", "comma-separated dqlite node addresses (wire protocol)")
	stringFlag(fs, "db-dqlite-discovery-srv", "", "DNS SRV record seeding the dqlite node candidates, e.g. _dqlite._tcp.librevita.svc.cluster.local")
	stringFlag(fs, "db-dqlite-database", defaultDqliteDatabase, "dqlite database name")
	stringFlag(fs, "log-mode", defaultLogMode, "production log mode: console, file, or rotating")
	stringFlag(fs, "log-level", "", "log level: debug, info, warn, or error")
	stringFlag(fs, "log-file-path", "", "log file path (file mode)")
	stringFlag(fs, "log-rotating-path", "", "rotating log file path")
	intFlag(fs, "log-rotating-max-size", defaultLogSizeMB, "rotating log maximum size in MB")
	intFlag(fs, "log-rotating-max-backups", defaultLogBackups, "rotating log backup count")
	intFlag(fs, "log-rotating-max-age", defaultLogAgeDays, "rotating log maximum age in days")
	boolFlag(fs, "log-rotating-compress", true, "compress rotated log files")
	stringFlag(fs, "paseto-key", "", "PASETO v4.local session key (base64, 32 bytes)")
	stringFlag(fs, "master-key", "", "field-level encryption master key (base64, 32 bytes)")
	intFlag(fs, "auth-max-concurrent-hashes", defaultMaxConcurrentHashes, "bound on concurrent Argon2id operations")
	stringFlag(fs, "storage-backend", "local", "file storage backend: local or s3")
	stringFlag(fs, "storage-local-dir", "", "local file storage directory (default <data-dir>/files)")
	stringFlag(fs, "storage-s3-endpoint", "", "S3-compatible API endpoint, e.g. minio.example.org:9000")
	stringFlag(fs, "storage-s3-bucket", "", "S3 bucket for stored files")
	stringFlag(fs, "storage-s3-access-key", "", "S3 access key")
	stringFlag(fs, "storage-s3-secret-key", "", "S3 secret key")
	stringFlag(fs, "storage-s3-region", "", "S3 region (may be empty outside AWS)")
	boolFlag(fs, "storage-s3-secure", true, "use HTTPS for the S3 endpoint")
	boolFlag(fs, "storage-s3-path-style", true, "use path-style S3 addressing")
	stringFlag(fs, "vault-backend", "bbolt", "key vault storage backend: bbolt, nats, etcd, or hashicorp")
	stringFlag(fs, "vault-bbolt-path", "", "bbolt key vault path (default <data-dir>/keys.db)")
	stringFlag(fs, "vault-nats-url", "", "NATS server URL (e.g. nats://localhost:4222)")
	stringFlag(fs, "vault-nats-bucket", "patient_deks", "NATS JetStream KeyValue bucket name")
	stringFlag(fs, "vault-etcd-endpoints", "", "comma-separated etcd v3 endpoints (e.g. localhost:2379)")
	stringFlag(fs, "vault-etcd-prefix", "/librevita/keys/", "etcd key prefix")
	stringFlag(fs, "vault-hashicorp-address", "", "HashiCorp Vault / OpenBao address (e.g. http://localhost:8200)")
	stringFlag(fs, "vault-hashicorp-token", "", "HashiCorp Vault / OpenBao authentication token")
	stringFlag(fs, "vault-hashicorp-mount", "secret", "HashiCorp Vault KV v2 mount path")
	stringFlag(fs, "crypto-hash-algorithm", "blake2s", "Default cryptographic hash engine (blake2s, blake2b)")
	stringFlag(fs, "crypto-encryption-cipher", "xchacha20-poly1305", "Default symmetric encryption cipher (xchacha20-poly1305)")
}

// IsProduction reports whether the application runs in production.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Mode, "production")
}

// IsDevelopment reports whether the application runs in the explicit
// development environment. Every other environment is treated as a
// persistent deployment: the PASETO key is required and cookies use the
// Secure flag, so a deployment labeled "staging" or "prod" never falls
// back to ephemeral keys or insecure cookies.
func (c *Config) IsDevelopment() bool {
	return strings.EqualFold(c.Mode, "development")
}

// New is the Fx configuration provider.
func New() (*Config, error) {
	RegisterFlags(pflag.CommandLine)

	// Keep support for .env files. Existing process variables take precedence.
	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrap(err, "config: failed to read .env")
	}

	cfg, err := load(pflag.CommandLine)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, errors.Wrapf(err, "config: failed to create data directory %q", cfg.DataDir)
	}
	return cfg, nil
}

func load(fs *pflag.FlagSet) (*Config, error) {
	// Resolve the configuration file before loading the remaining sources.
	bootstrap := koanf.New(".")
	if err := loadEnvironment(bootstrap); err != nil {
		return nil, errors.Wrap(err, "config: environment")
	}
	if err := loadFlags(bootstrap, fs); err != nil {
		return nil, errors.Wrap(err, "config: flags")
	}

	configFile := bootstrap.String(keyConfigFile)
	if configFile == "" {
		configFile = discoverConfigFile()
	}

	k := koanf.New(".")
	if configFile != "" {
		parser, err := parserFor(configFile)
		if err != nil {
			return nil, err
		}
		if err := k.Load(file.Provider(configFile), parser); err != nil {
			return nil, errors.Wrapf(err, "config: failed to read %q", configFile)
		}
	}
	if err := loadEnvironment(k); err != nil {
		return nil, errors.Wrap(err, "config: environment")
	}
	if err := loadFlags(k, fs); err != nil {
		return nil, errors.Wrap(err, "config: flags")
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, errors.Wrap(err, "config: decode")
	}

	cfg.ConfigFile = configFile
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadEnvironment(k *koanf.Koanf) error {
	return k.Load(env.Provider(envPrefix, ".", mapEnvironmentKey), nil)
}

func loadFlags(k *koanf.Koanf, fs *pflag.FlagSet) error {
	return k.Load(posflag.ProviderWithFlag(fs, ".", k, func(f *pflag.Flag) (string, any) {
		key := mapFlagKey(f.Name)
		if key == "" {
			return "", nil
		}
		return key, posflag.FlagVal(fs, f)
	}), nil)
}

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

func stringFlag(fs *pflag.FlagSet, name, value, usage string) {
	if fs.Lookup(name) == nil {
		fs.String(name, value, usage)
	}
}

func intFlag(fs *pflag.FlagSet, name string, value int, usage string) {
	if fs.Lookup(name) == nil {
		fs.Int(name, value, usage)
	}
}

func boolFlag(fs *pflag.FlagSet, name string, value bool, usage string) {
	if fs.Lookup(name) == nil {
		fs.Bool(name, value, usage)
	}
}

// flagKeys maps CLI flag names (hyphen or underscore) onto Koanf keys.
var flagKeys = map[string]string{ // #nosec G101 -- Koanf field paths, not credentials.
	"config":                     keyConfigFile,
	"config-file":                keyConfigFile,
	"config_file":                keyConfigFile,
	"mode":                       "mode",
	"http-bind":                  "http_bind",
	"http_bind":                  "http_bind",
	"http-port":                  "http_port",
	"http_port":                  "http_port",
	"base-domain":                "base_domain",
	"base_domain":                "base_domain",
	"data-dir":                   "data_dir",
	"data_dir":                   "data_dir",
	"db-driver":                  "database.driver",
	"db_driver":                  "database.driver",
	"db-sqlite-path":             "database.sqlite.path",
	"db_sqlite_path":             "database.sqlite.path",
	"db-postgres-url":            "database.postgres.url",
	"db_postgres_url":            "database.postgres.url",
	"db-postgres-host":           "database.postgres.host",
	"db_postgres_host":           "database.postgres.host",
	"db-postgres-port":           "database.postgres.port",
	"db_postgres_port":           "database.postgres.port",
	"db-postgres-user":           "database.postgres.user",
	"db_postgres_user":           "database.postgres.user",
	"db-postgres-password":       "database.postgres.password",
	"db_postgres_password":       "database.postgres.password",
	"db-postgres-database":       "database.postgres.database",
	"db_postgres_database":       "database.postgres.database",
	"db-postgres-sslmode":        "database.postgres.sslmode",
	"db_postgres_sslmode":        "database.postgres.sslmode",
	"db-postgres-max-open-conns": "database.postgres.max_open_conns",
	"db_postgres_max_open_conns": "database.postgres.max_open_conns",
	"db-postgres-max-idle-conns": "database.postgres.max_idle_conns",
	"db_postgres_max_idle_conns": "database.postgres.max_idle_conns",
	"db-dqlite-addrs":            "database.dqlite.addrs",
	"db_dqlite_addrs":            "database.dqlite.addrs",
	"db-dqlite-discovery-srv":    "database.dqlite.discovery_srv",
	"db_dqlite_discovery_srv":    "database.dqlite.discovery_srv",
	"db-dqlite-database":         "database.dqlite.database",
	"db_dqlite_database":         "database.dqlite.database",
	"log-mode":                   "logging.mode",
	"log_mode":                   "logging.mode",
	"log-level":                  "logging.level",
	"log_level":                  "logging.level",
	"log-file-path":              "logging.file.path",
	"log_file_path":              "logging.file.path",
	"log-rotating-path":          "logging.rotating.path",
	"log_rotating_path":          "logging.rotating.path",
	"log-rotating-max-size":      "logging.rotating.max_size_mb",
	"log_rotating_max_size":      "logging.rotating.max_size_mb",
	"log-rotating-max-backups":   "logging.rotating.max_backups",
	"log_rotating_max_backups":   "logging.rotating.max_backups",
	"log-rotating-max-age":       "logging.rotating.max_age_days",
	"log_rotating_max_age":       "logging.rotating.max_age_days",
	"log-rotating-compress":      "logging.rotating.compress",
	"log_rotating_compress":      "logging.rotating.compress",
	"paseto-key":                 "paseto_key",
	"paseto_key":                 "paseto_key",
	"master-key":                 "master_key",
	"master_key":                 "master_key",
	"auth-max-concurrent-hashes": "auth.max_concurrent_hashes",
	"auth_max_concurrent_hashes": "auth.max_concurrent_hashes",
	"storage-backend":            "storage.backend",
	"storage_backend":            "storage.backend",
	"storage-local-dir":          "storage.local.dir",
	"storage_local_dir":          "storage.local.dir",
	"storage-s3-endpoint":        "storage.s3.endpoint",
	"storage_s3_endpoint":        "storage.s3.endpoint",
	"storage-s3-bucket":          "storage.s3.bucket",
	"storage_s3_bucket":          "storage.s3.bucket",
	"storage-s3-access-key":      "storage.s3.access_key",
	"storage_s3_access_key":      "storage.s3.access_key",
	"storage-s3-secret-key":      "storage.s3.secret_key",
	"storage_s3_secret_key":      "storage.s3.secret_key",
	"storage-s3-region":          "storage.s3.region",
	"storage_s3_region":          "storage.s3.region",
	"storage-s3-secure":          "storage.s3.secure",
	"storage_s3_secure":          "storage.s3.secure",
	"storage-s3-path-style":      "storage.s3.path_style",
	"storage_s3_path_style":      "storage.s3.path_style",
	"vault-backend":              "vault.backend",
	"vault_backend":              "vault.backend",
	"vault-bbolt-path":           "vault.bbolt.path",
	"vault_bbolt_path":           "vault.bbolt.path",
	"vault-nats-url":             "vault.nats.url",
	"vault_nats_url":             "vault.nats.url",
	"vault-nats-bucket":          "vault.nats.bucket",
	"vault_nats_bucket":          "vault.nats.bucket",
	"vault-etcd-endpoints":       "vault.etcd.endpoints",
	"vault_etcd_endpoints":       "vault.etcd.endpoints",
	"vault-etcd-prefix":          "vault.etcd.prefix",
	"vault_etcd_prefix":          "vault.etcd.prefix",
	"vault-hashicorp-address":    "vault.hashicorp.address",
	"vault_hashicorp_address":    "vault.hashicorp.address",
	"vault-hashicorp-token":      "vault.hashicorp.token",
	"vault_hashicorp_token":      "vault.hashicorp.token",
	"vault-hashicorp-mount":      "vault.hashicorp.mount",
	"vault_hashicorp_mount":      "vault.hashicorp.mount",
}

// envKeys maps LIBREVITA_* suffixes (after the prefix is stripped) onto Koanf keys.
var envKeys = map[string]string{ // #nosec G101 -- Koanf field paths, not credentials.
	"config":                           keyConfigFile,
	"mode":                             "mode",
	"http_bind":                        "http_bind",
	"http_addr":                        "http_bind",
	"http_port":                        "http_port",
	"base_domain":                      "base_domain",
	"data_dir":                         "data_dir",
	"database_driver":                  "database.driver",
	"database_sqlite_path":             "database.sqlite.path",
	"database_postgres_url":            "database.postgres.url",
	"database_postgres_host":           "database.postgres.host",
	"database_postgres_port":           "database.postgres.port",
	"database_postgres_user":           "database.postgres.user",
	"database_postgres_password":       "database.postgres.password",
	"database_postgres_database":       "database.postgres.database",
	"database_postgres_sslmode":        "database.postgres.sslmode",
	"database_postgres_max_open_conns": "database.postgres.max_open_conns",
	"database_postgres_max_idle_conns": "database.postgres.max_idle_conns",
	"database_dqlite_addrs":            "database.dqlite.addrs",
	"database_dqlite_discovery_srv":    "database.dqlite.discovery_srv",
	"database_dqlite_database":         "database.dqlite.database",
	"logging_mode":                     "logging.mode",
	"logging_level":                    "logging.level",
	"logging_file_path":                "logging.file.path",
	"logging_rotating_path":            "logging.rotating.path",
	"logging_rotating_max_size_mb":     "logging.rotating.max_size_mb",
	"logging_rotating_max_backups":     "logging.rotating.max_backups",
	"logging_rotating_max_age_days":    "logging.rotating.max_age_days",
	"logging_rotating_compress":        "logging.rotating.compress",
	"paseto_key":                       "paseto_key",
	"master_key":                       "master_key",
	"auth_max_concurrent_hashes":       "auth.max_concurrent_hashes",
	"storage_backend":                  "storage.backend",
	"storage_local_dir":                "storage.local.dir",
	"storage_s3_endpoint":              "storage.s3.endpoint",
	"storage_s3_bucket":                "storage.s3.bucket",
	"storage_s3_access_key":            "storage.s3.access_key",
	"storage_s3_secret_key":            "storage.s3.secret_key",
	"storage_s3_region":                "storage.s3.region",
	"storage_s3_secure":                "storage.s3.secure",
	"storage_s3_path_style":            "storage.s3.path_style",
	"vault_backend":                    "vault.backend",
	"vault_bbolt_path":                 "vault.bbolt.path",
	"vault_nats_url":                   "vault.nats.url",
	"vault_nats_bucket":                "vault.nats.bucket",
	"vault_etcd_endpoints":             "vault.etcd.endpoints",
	"vault_etcd_prefix":                "vault.etcd.prefix",
	"vault_hashicorp_address":          "vault.hashicorp.address",
	"vault_hashicorp_token":            "vault.hashicorp.token",
	"vault_hashicorp_mount":            "vault.hashicorp.mount",
	"crypto_hash_algorithm":            "crypto.hash_algorithm",
	"crypto_encryption_cipher":         "crypto.encryption_cipher",
}

func mapFlagKey(name string) string {
	return flagKeys[strings.ToLower(name)]
}

func mapEnvironmentKey(key string) string {
	return envKeys[strings.ToLower(strings.TrimPrefix(key, envPrefix))]
}

func discoverConfigFile() string {
	for _, path := range []string{"config.yaml", "config.yml", "config.json"} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func parserFor(path string) (koanf.Parser, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return yaml.Parser(), nil
	case ".json":
		return json.Parser(), nil
	default:
		return nil, errors.Newf("config: unsupported extension in %q (use .yaml, .yml, or .json)", path)
	}
}
