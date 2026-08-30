package config

import (
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

// RegisterFlags registers the application flags. It is safe to call more
// than once.
func RegisterFlags(fs *pflag.FlagSet) {
	stringFlag(fs, "config", "", "configuration file (.yaml, .yml, or .json)")
	stringFlag(fs, "mode", defaultMode, "runtime mode: production or development")
	stringFlag(fs, "http-bind", defaultHTTPBind, "HTTP bind address (0.0.0.0, 127.0.0.1, ...)")
	intFlag(fs, "http-port", defaultHTTPPort, "HTTP listen port")
	stringFlag(fs, "base-domain", "", "DNS suffix for clinic hosts ({slug}.{base-domain}; required in production, default lv.test in development)")
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
