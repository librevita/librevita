// Package config loads LibreVita configuration.
//
// Sources are merged by Koanf in this order: defaults, file, environment/.env,
// and flags.
package config

import (
	"errors"
	"fmt"
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
	DriverSQLite = "sqlite" // Embedded SQLite for the monolith and edge deployments.
	DriverDqlite = "dqlite" // dqlite cluster for distributed deployments.
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

	defaultEnv            = "development"
	defaultHTTPAddr       = ":8080"
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

	// Env is the runtime environment, such as "development" or "production".
	Env string `koanf:"env"`

	// HTTPAddr is the Echo bind address, for example ":8080".
	HTTPAddr string `koanf:"http_addr"`

	// TrustedProxies is a comma-separated list of proxy addresses whose
	// X-Forwarded-For header is trusted for rate limiting and audit IPs.
	// Empty means the app is not behind a proxy and the remote address is
	// used directly.
	TrustedProxies string `koanf:"trusted_proxies"`

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

// AuthConfig controls authentication behavior.
type AuthConfig struct {
	// MaxConcurrentHashes bounds concurrent Argon2id operations.
	MaxConcurrentHashes int `koanf:"max_concurrent_hashes"`
}

// DatabaseConfig defines the active persistence backend.
type DatabaseConfig struct {
	// Driver is DriverSQLite or DriverDqlite.
	Driver string `koanf:"driver"`

	// Path is the SQLite database path.
	Path string `koanf:"path"`

	// DqliteAddrs is the comma-separated list of dqlite node addresses
	// (the wire protocol), e.g. "node1:9001,node2:9001,node3:9001".
	DqliteAddrs string `koanf:"dqlite_addrs"`

	// DqliteDatabase is the database name on the dqlite cluster.
	DqliteDatabase string `koanf:"dqlite_database"`
}

// LoggingConfig controls production logging.
type LoggingConfig struct {
	Mode       string `koanf:"mode"`
	Path       string `koanf:"path"`
	MaxSizeMB  int    `koanf:"max_size_mb"`
	MaxBackups int    `koanf:"max_backups"`
	MaxAgeDays int    `koanf:"max_age_days"`
	Compress   bool   `koanf:"compress"`
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
	stringFlag(fs, "env", defaultEnv, "runtime environment")
	stringFlag(fs, "http-addr", defaultHTTPAddr, "HTTP bind address")
	stringFlag(fs, "trusted-proxies", "", "comma-separated proxy IPs allowed to set X-Forwarded-For")
	stringFlag(fs, "data-dir", defaultDataDir, "base directory for database and logs")
	stringFlag(fs, "db-driver", DriverSQLite, "database backend: sqlite or dqlite")
	stringFlag(fs, "db-path", "", "SQLite database path")
	stringFlag(fs, "dqlite-addrs", "", "comma-separated dqlite node addresses (wire protocol)")
	stringFlag(fs, "dqlite-database", defaultDqliteDatabase, "dqlite database name")
	stringFlag(fs, "log-mode", defaultLogMode, "production log mode: console, file, or rotating")
	stringFlag(fs, "log-path", "", "log file path")
	intFlag(fs, "log-max-size", defaultLogSizeMB, "rotating log maximum size in MB")
	intFlag(fs, "log-max-backups", defaultLogBackups, "rotating log backup count")
	intFlag(fs, "log-max-age", defaultLogAgeDays, "rotating log maximum age in days")
	boolFlag(fs, "log-compress", true, "compress rotated log files")
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
}

// IsProduction reports whether the application runs in production.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// IsDevelopment reports whether the application runs in the explicit
// development environment. Every other environment is treated as a
// persistent deployment: the PASETO key is required and cookies use the
// Secure flag, so a deployment labeled "staging" or "prod" never falls
// back to ephemeral keys or insecure cookies.
func (c *Config) IsDevelopment() bool {
	return strings.EqualFold(c.Env, "development")
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
	c.Env = strings.TrimSpace(c.Env)
	if c.Env == "" {
		c.Env = defaultEnv
	}

	c.HTTPAddr = strings.TrimSpace(c.HTTPAddr)
	if c.HTTPAddr == "" {
		c.HTTPAddr = defaultHTTPAddr
	}

	c.DataDir = strings.TrimSpace(c.DataDir)
	if c.DataDir == "" {
		c.DataDir = defaultDataDir
	}

	c.Database.Driver = strings.ToLower(strings.TrimSpace(c.Database.Driver))
	if c.Database.Driver == "" {
		c.Database.Driver = DriverSQLite
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		c.Database.Path = filepath.Join(c.DataDir, "librevita.db")
	}
	if strings.TrimSpace(c.Database.DqliteDatabase) == "" {
		c.Database.DqliteDatabase = defaultDqliteDatabase
	}

	c.Logging.Mode = strings.ToLower(strings.TrimSpace(c.Logging.Mode))
	if c.Logging.Mode == "" {
		c.Logging.Mode = defaultLogMode
	}
	if strings.TrimSpace(c.Logging.Path) == "" {
		c.Logging.Path = filepath.Join(c.DataDir, "librevita.log")
	}
	if c.Logging.MaxSizeMB <= 0 {
		c.Logging.MaxSizeMB = defaultLogSizeMB
	}
	if c.Logging.MaxBackups < 0 {
		c.Logging.MaxBackups = defaultLogBackups
	}
	if c.Logging.MaxAgeDays < 0 {
		c.Logging.MaxAgeDays = defaultLogAgeDays
	}

	if c.Auth.MaxConcurrentHashes <= 0 {
		c.Auth.MaxConcurrentHashes = defaultMaxConcurrentHashes
	}
}

func (c *Config) validate() error {
	switch c.Database.Driver {
	case DriverSQLite, DriverDqlite:
	default:
		return fmt.Errorf("config: invalid database.driver %q (use %q or %q)",
			c.Database.Driver, DriverSQLite, DriverDqlite)
	}
	if c.Database.Driver == DriverDqlite {
		addresses := 0
		for _, addr := range strings.Split(c.Database.DqliteAddrs, ",") {
			if strings.TrimSpace(addr) != "" {
				addresses++
			}
		}
		if addresses == 0 {
			return fmt.Errorf("config: database.dqlite_addrs requires at least one node address (e.g. \"node1:9001,node2:9001,node3:9001\")")
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
	case "env":
		return "env"
	case "http-addr", "http_addr":
		return "http_addr"
	case "data-dir", "data_dir":
		return "data_dir"
	case "db-driver", "db_driver":
		return "database.driver"
	case "db-path", "db_path":
		return "database.path"
	case "dqlite-addrs", "dqlite_addrs":
		return "database.dqlite_addrs"
	case "dqlite-database", "dqlite_database":
		return "database.dqlite_database"
	case "log-mode", "log_mode":
		return "logging.mode"
	case "log-path", "log_path":
		return "logging.path"
	case "log-max-size", "log_max_size":
		return "logging.max_size_mb"
	case "log-max-backups", "log_max_backups":
		return "logging.max_backups"
	case "log-max-age", "log_max_age":
		return "logging.max_age_days"
	case "log-compress", "log_compress":
		return "logging.compress"
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
	default:
		return ""
	}
}

func mapEnvironmentKey(key string) string {
	key = strings.ToLower(strings.TrimPrefix(key, envPrefix))
	switch key {
	case "config", "config_file":
		return keyConfigFile
	case "env":
		return "env"
	case "http_addr":
		return "http_addr"
	case "data_dir":
		return "data_dir"
	case "db_driver", "database_driver":
		return "database.driver"
	case "db_path", "database_path":
		return "database.path"
	case "dqlite_addrs", "database_dqlite_addrs":
		return "database.dqlite_addrs"
	case "dqlite_database", "database_dqlite_database":
		return "database.dqlite_database"
	case "log_mode", "logging_mode":
		return "logging.mode"
	case "log_path", "logging_path":
		return "logging.path"
	case "log_max_size_mb", "logging_max_size_mb":
		return "logging.max_size_mb"
	case "log_max_backups", "logging_max_backups":
		return "logging.max_backups"
	case "log_max_age_days", "logging_max_age_days":
		return "logging.max_age_days"
	case "log_compress", "logging_compress":
		return "logging.compress"
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
