// Package config loads LibreVita configuration.
//
// Sources are merged by Koanf in this order: defaults, file, environment/.env,
// and flags.
package config

import (
	"fmt"
	"net/url"
	"strings"
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

	defaultMode           = "production"
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

	// KV backends.
	BackendBBolt = "bbolt"
	BackendNATS  = "nats"
	BackendEtcd  = "etcd"
	BackendVault = "vault"
)

// Config is the application configuration root.
type Config struct {
	// ConfigFile is the file that was loaded, if any.
	ConfigFile string `koanf:"config_file"`

	// Mode is the runtime mode: "production" (default) or "development".
	// Every value other than "development" is treated as a persistent
	// deployment (secrets required, Secure cookies).
	Mode string `koanf:"mode"`

	// HTTPBind is the address the HTTP server binds to, e.g. "0.0.0.0"
	// (all interfaces) or "127.0.0.1" (loopback only).
	HTTPBind string `koanf:"http_bind"`

	// HTTPPort is the TCP port the HTTP server listens on.
	HTTPPort int `koanf:"http_port"`

	// BaseDomain is the DNS suffix used to resolve clinics from Host
	// (`{slug}.{base_domain}`). Apex is this value or `www.` plus this
	// value. Required in production; defaults to lv.test in
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

	// Keystore holds wrapped Clinic and Patient DEKs (bbolt, nats, etcd, or vault).
	Keystore KVConfig `koanf:"keystore"`

	// Meta is installation key-value metadata (bbolt, nats, or etcd).
	Meta KVConfig `koanf:"meta"`

	// Sessions is the PASETO revocation index (bbolt, nats, or etcd).
	Sessions KVConfig `koanf:"sessions"`

	// PasetoKey is the base64-encoded 32-byte key for PASETO v4.local
	// session tokens. Required outside development; generated at startup
	// in development if omitted.
	PasetoKey string `koanf:"paseto_key"`

	// MasterKey is the base64-encoded 32-byte master key for field-level
	// encryption and blind indexes of patient identifiers. Required
	// outside development; an ephemeral key is generated at startup
	// in development if omitted (previously encrypted values become
	// undecryptable on restart).
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

// KVConfig is one independent key-value store (keystore, meta, or sessions).
type KVConfig struct {
	// Backend is BackendBBolt (default), BackendNATS, BackendEtcd, or
	// BackendVault. Vault is valid only on the keystore block.
	Backend string `koanf:"backend"`

	BBolt BBoltConfig        `koanf:"bbolt"`
	NATS  NATSConfig         `koanf:"nats"`
	Etcd  EtcdConfig         `koanf:"etcd"`
	Vault VaultBackendConfig `koanf:"vault"`
}

// BBoltConfig configures an embedded bbolt file.
type BBoltConfig struct {
	Path string `koanf:"path"`
}

// NATSConfig configures a NATS JetStream KeyValue bucket.
type NATSConfig struct {
	URL    string `koanf:"url"`
	Bucket string `koanf:"bucket"`
}

// EtcdConfig configures an etcd v3 key prefix.
type EtcdConfig struct {
	Endpoints string `koanf:"endpoints"`
	Prefix    string `koanf:"prefix"`
}

// VaultBackendConfig configures HashiCorp Vault / OpenBao KV v2.
type VaultBackendConfig struct {
	Address string `koanf:"address"`
	Token   string `koanf:"token"`
	Mount   string `koanf:"mount"`
	Prefix  string `koanf:"prefix"`
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
