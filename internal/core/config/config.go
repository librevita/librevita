// Package config loads LibreVita configuration.
//
// Sources are merged by Koanf in this order: defaults, file, environment/.env,
// and flags.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
		return nil, fmt.Errorf("config: failed to read .env: %w", err)
	}

	cfg, err := load(pflag.CommandLine)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("config: failed to create data directory %q: %w", cfg.DataDir, err)
	}
	return cfg, nil
}

func load(fs *pflag.FlagSet) (*Config, error) {
	// Resolve the configuration file before loading the remaining sources.
	bootstrap := koanf.New(".")
	if err := loadEnvironment(bootstrap); err != nil {
		return nil, fmt.Errorf("config: environment: %w", err)
	}
	if err := loadFlags(bootstrap, fs); err != nil {
		return nil, fmt.Errorf("config: flags: %w", err)
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
			return nil, fmt.Errorf("config: failed to read %q: %w", configFile, err)
		}
	}
	if err := loadEnvironment(k); err != nil {
		return nil, fmt.Errorf("config: environment: %w", err)
	}
	if err := loadFlags(k, fs); err != nil {
		return nil, fmt.Errorf("config: flags: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
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

	c.DataDir = strings.TrimSpace(c.DataDir)
	if c.DataDir == "" {
		c.DataDir = defaultDataDir
	}

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

	c.Logging.Mode = strings.ToLower(strings.TrimSpace(c.Logging.Mode))
	if c.Logging.Mode == "" {
		c.Logging.Mode = defaultLogMode
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

	c.Vault.Backend = strings.ToLower(strings.TrimSpace(c.Vault.Backend))
	if c.Vault.Backend == "" {
		c.Vault.Backend = "bbolt"
	}
	if strings.TrimSpace(c.Vault.BBolt.Path) == "" {
		c.Vault.BBolt.Path = filepath.Join(c.DataDir, "keys.db")
	}
}

func (c *Config) validate() error {
	switch c.Vault.Backend {
	case "", "bbolt", "nats", "etcd", "hashicorp", "hashicorp_vault", "openbao":
	default:
		return fmt.Errorf("config: invalid vault.backend %q (use \"bbolt\", \"nats\", \"etcd\", \"hashicorp\", \"hashicorp_vault\", or \"openbao\")", c.Vault.Backend)
	}

	switch c.Database.Driver {
	case DriverSQLite, DriverPostgres, DriverDqlite:
	default:
		return fmt.Errorf("config: invalid database.driver %q (use %q, %q, or %q)",
			c.Database.Driver, DriverSQLite, DriverPostgres, DriverDqlite)
	}
	if c.Database.Driver == DriverDqlite {
		addresses := 0
		for _, addr := range strings.Split(c.Database.Dqlite.Addrs, ",") {
			if strings.TrimSpace(addr) != "" {
				addresses++
			}
		}
		if addresses == 0 && c.Database.Dqlite.DiscoverySRV == "" {
			return fmt.Errorf("config: database.dqlite.addrs requires at least one node address (e.g. \"node1:9001,node2:9001,node3:9001\") or database.dqlite.discovery_srv (an SRV record)")
		}
	}

	if c.HTTPPort > 65535 {
		return fmt.Errorf("config: invalid http_port %d (max 65535)", c.HTTPPort)
	}

	// A typo here would silently degrade to trusting the remote address
	// (or worse, trusting the client), so the list is validated at boot.
	if strings.TrimSpace(c.TrustedProxies) != "" {
		for _, p := range strings.Split(c.TrustedProxies, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, _, err := net.ParseCIDR(p); err == nil {
				continue
			}
			if net.ParseIP(p) == nil {
				return fmt.Errorf("config: invalid trusted_proxies entry %q (use CIDR or IP, comma-separated)", p)
			}
		}
	}

	switch c.Logging.Mode {
	case LogModeConsole, LogModeFile, LogModeRotating:
		return nil
	default:
		return fmt.Errorf("config: invalid logging.mode %q (use %q, %q, or %q)",
			c.Logging.Mode, LogModeConsole, LogModeFile, LogModeRotating)
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

func mapFlagKey(name string) string {
	switch strings.ToLower(name) {
	case "config", "config-file", "config_file":
		return keyConfigFile
	case "mode":
		return "mode"
	case "http-bind", "http_bind":
		return "http_bind"
	case "http-port", "http_port":
		return "http_port"
	case "data-dir", "data_dir":
		return "data_dir"
	case "db-driver", "db_driver":
		return "database.driver"
	case "db-sqlite-path", "db_sqlite_path":
		return "database.sqlite.path"
	case "db-postgres-url", "db_postgres_url":
		return "database.postgres.url"
	case "db-postgres-host", "db_postgres_host":
		return "database.postgres.host"
	case "db-postgres-port", "db_postgres_port":
		return "database.postgres.port"
	case "db-postgres-user", "db_postgres_user":
		return "database.postgres.user"
	case "db-postgres-password", "db_postgres_password":
		return "database.postgres.password"
	case "db-postgres-database", "db_postgres_database":
		return "database.postgres.database"
	case "db-postgres-sslmode", "db_postgres_sslmode":
		return "database.postgres.sslmode"
	case "db-postgres-max-open-conns", "db_postgres_max_open_conns":
		return "database.postgres.max_open_conns"
	case "db-postgres-max-idle-conns", "db_postgres_max_idle_conns":
		return "database.postgres.max_idle_conns"
	case "db-dqlite-addrs", "db_dqlite_addrs":
		return "database.dqlite.addrs"
	case "db-dqlite-discovery-srv", "db_dqlite_discovery_srv":
		return "database.dqlite.discovery_srv"
	case "db-dqlite-database", "db_dqlite_database":
		return "database.dqlite.database"
	case "log-mode", "log_mode":
		return "logging.mode"
	case "log-file-path", "log_file_path":
		return "logging.file.path"
	case "log-rotating-path", "log_rotating_path":
		return "logging.rotating.path"
	case "log-rotating-max-size", "log_rotating_max_size":
		return "logging.rotating.max_size_mb"
	case "log-rotating-max-backups", "log_rotating_max_backups":
		return "logging.rotating.max_backups"
	case "log-rotating-max-age", "log_rotating_max_age":
		return "logging.rotating.max_age_days"
	case "log-rotating-compress", "log_rotating_compress":
		return "logging.rotating.compress"
	case "paseto-key", "paseto_key":
		return "paseto_key"
	case "master-key", "master_key":
		return "master_key"
	case "auth-max-concurrent-hashes", "auth_max_concurrent_hashes":
		return "auth.max_concurrent_hashes"
	case "storage-backend", "storage_backend":
		return "storage.backend"
	case "storage-local-dir", "storage_local_dir":
		return "storage.local.dir"
	case "storage-s3-endpoint", "storage_s3_endpoint":
		return "storage.s3.endpoint"
	case "storage-s3-bucket", "storage_s3_bucket":
		return "storage.s3.bucket"
	case "storage-s3-access-key", "storage_s3_access_key":
		return "storage.s3.access_key"
	case "storage-s3-secret-key", "storage_s3_secret_key":
		return "storage.s3.secret_key"
	case "storage-s3-region", "storage_s3_region":
		return "storage.s3.region"
	case "storage-s3-secure", "storage_s3_secure":
		return "storage.s3.secure"
	case "storage-s3-path-style", "storage_s3_path_style":
		return "storage.s3.path_style"
	case "vault-backend", "vault_backend":
		return "vault.backend"
	case "vault-bbolt-path", "vault_bbolt_path":
		return "vault.bbolt.path"
	case "vault-nats-url", "vault_nats_url":
		return "vault.nats.url"
	case "vault-nats-bucket", "vault_nats_bucket":
		return "vault.nats.bucket"
	case "vault-etcd-endpoints", "vault_etcd_endpoints":
		return "vault.etcd.endpoints"
	case "vault-etcd-prefix", "vault_etcd_prefix":
		return "vault.etcd.prefix"
	case "vault-hashicorp-address", "vault_hashicorp_address":
		return "vault.hashicorp.address"
	case "vault-hashicorp-token", "vault_hashicorp_token":
		return "vault.hashicorp.token"
	case "vault-hashicorp-mount", "vault_hashicorp_mount":
		return "vault.hashicorp.mount"
	default:
		return ""
	}
}

func mapEnvironmentKey(key string) string {
	key = strings.ToLower(strings.TrimPrefix(key, envPrefix))
	switch key {
	case "config":
		return keyConfigFile
	case "mode":
		return "mode"
	case "http_bind", "http_addr":
		return "http_bind"
	case "http_port":
		return "http_port"
	case "data_dir":
		return "data_dir"
	case "database_driver":
		return "database.driver"
	case "database_sqlite_path":
		return "database.sqlite.path"
	case "database_postgres_url":
		return "database.postgres.url"
	case "database_postgres_host":
		return "database.postgres.host"
	case "database_postgres_port":
		return "database.postgres.port"
	case "database_postgres_user":
		return "database.postgres.user"
	case "database_postgres_password":
		return "database.postgres.password"
	case "database_postgres_database":
		return "database.postgres.database"
	case "database_postgres_sslmode":
		return "database.postgres.sslmode"
	case "database_postgres_max_open_conns":
		return "database.postgres.max_open_conns"
	case "database_postgres_max_idle_conns":
		return "database.postgres.max_idle_conns"
	case "database_dqlite_addrs":
		return "database.dqlite.addrs"
	case "database_dqlite_discovery_srv":
		return "database.dqlite.discovery_srv"
	case "database_dqlite_database":
		return "database.dqlite.database"
	case "logging_mode":
		return "logging.mode"
	case "logging_file_path":
		return "logging.file.path"
	case "logging_rotating_path":
		return "logging.rotating.path"
	case "logging_rotating_max_size_mb":
		return "logging.rotating.max_size_mb"
	case "logging_rotating_max_backups":
		return "logging.rotating.max_backups"
	case "logging_rotating_max_age_days":
		return "logging.rotating.max_age_days"
	case "logging_rotating_compress":
		return "logging.rotating.compress"
	case "paseto_key":
		return "paseto_key"
	case "master_key":
		return "master_key"
	case "auth_max_concurrent_hashes":
		return "auth.max_concurrent_hashes"
	case "storage_backend":
		return "storage.backend"
	case "storage_local_dir":
		return "storage.local.dir"
	case "storage_s3_endpoint":
		return "storage.s3.endpoint"
	case "storage_s3_bucket":
		return "storage.s3.bucket"
	case "storage_s3_access_key":
		return "storage.s3.access_key"
	case "storage_s3_secret_key":
		return "storage.s3.secret_key"
	case "storage_s3_region":
		return "storage.s3.region"
	case "storage_s3_secure":
		return "storage.s3.secure"
	case "storage_s3_path_style":
		return "storage.s3.path_style"
	case "vault_backend":
		return "vault.backend"
	case "vault_bbolt_path":
		return "vault.bbolt.path"
	case "vault_nats_url":
		return "vault.nats.url"
	case "vault_nats_bucket":
		return "vault.nats.bucket"
	case "vault_etcd_endpoints":
		return "vault.etcd.endpoints"
	case "vault_etcd_prefix":
		return "vault.etcd.prefix"
	case "vault_hashicorp_address":
		return "vault.hashicorp.address"
	case "vault_hashicorp_token":
		return "vault.hashicorp.token"
	case "vault_hashicorp_mount":
		return "vault.hashicorp.mount"
	default:
		return ""
	}
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
		return nil, fmt.Errorf("config: unsupported extension in %q (use .yaml, .yml, or .json)", path)
	}
}
