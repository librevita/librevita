package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPrecedence(t *testing.T) {
	t.Setenv("LIBREVITA_MODE", "production")
	t.Setenv("LIBREVITA_DATABASE_SQLITE_PATH", "env.db")

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := []byte("mode: development\nhttp_bind: '127.0.0.1'\nhttp_port: 9000\ndatabase:\n  driver: sqlite\n  sqlite:\n    path: file.db\n  dqlite:\n    addrs: node1:9001,node2:9001\n    database: lv\n")
	err := os.WriteFile(configFile, configYAML, 0o600)
	require.NoError(t, err)

	flags := pflag.NewFlagSet("config-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	err = flags.Parse([]string{"--config", configFile, "--http-bind", "0.0.0.0", "--http-port", "9100"})
	require.NoError(t, err)

	cfg, err := load(flags)
	require.NoError(t, err)

	assert.Equal(t, configFile, cfg.ConfigFile)
	assert.Equal(t, "production", cfg.Mode)
	assert.Equal(t, "0.0.0.0", cfg.HTTPBind)
	assert.Equal(t, 9100, cfg.HTTPPort)
	assert.Equal(t, "env.db", cfg.Database.SQLite.Path)
	assert.Equal(t, "node1:9001,node2:9001", cfg.Database.Dqlite.Addrs)
	assert.Equal(t, "lv", cfg.Database.Dqlite.Database)
	assert.Equal(t, LogModeConsole, cfg.Logging.Mode)
	assert.Equal(t, defaultLogSizeMB, cfg.Logging.Rotating.MaxSizeMB)
	assert.True(t, cfg.Logging.Rotating.Compress)
}

func TestValidateDqliteAddrs(t *testing.T) {
	cases := []struct {
		name    string
		addrs   string
		srv     string
		wantErr bool
	}{
		{"valid", "node1:9001, node2:9001", "", false},
		{"single", "node1:9001", "", false},
		{"missing", "", "", true},
		{"only separators", ",, ,", "", true},
		{"whitespace", "   ", "", true},
		{"srv only", "", "_dqlite._tcp.librevita.svc.cluster.local", false},
		{"srv only with empty separators", "  , ", "_dqlite._tcp.librevita.svc.cluster.local", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Database: DatabaseConfig{Driver: DriverDqlite, Dqlite: DqliteConfig{Addrs: tc.addrs, DiscoverySRV: tc.srv, Database: "lv"}},
				Logging:  LoggingConfig{Mode: LogModeConsole},
			}
			err := cfg.validate()
			if tc.wantErr {
				assert.Error(t, err, "validate(%q/%q) should fail", tc.addrs, tc.srv)
			} else {
				assert.NoError(t, err, "validate(%q/%q) should succeed", tc.addrs, tc.srv)
			}
		})
	}
}

func TestStorageFlagsMapToNestedKeys(t *testing.T) {
	flags := pflag.NewFlagSet("storage-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	err := flags.Parse([]string{
		"--storage-backend", "s3",
		"--storage-local-dir", "/tmp/files",
		"--storage-s3-endpoint", "minio:9000",
		"--storage-s3-bucket", "lv",
		"--storage-s3-access-key", "ak",
		"--storage-s3-secret-key", "sk",
		"--storage-s3-region", "us-east-1",
		"--storage-s3-secure=false",
		"--storage-s3-path-style=false",
	})
	require.NoError(t, err)

	k := koanf.New(".")
	err = k.Load(posflag.ProviderWithFlag(flags, ".", k, func(f *pflag.Flag) (string, any) {
		return mapFlagKey(f.Name), posflag.FlagVal(flags, f)
	}), nil)
	require.NoError(t, err)

	var cfg Config
	err = k.Unmarshal("", &cfg)
	require.NoError(t, err)

	assert.Equal(t, "s3", cfg.Storage.Backend)
	assert.Equal(t, "/tmp/files", cfg.Storage.Local.Dir)
	assert.Equal(t, "minio:9000", cfg.Storage.S3.Endpoint)
	assert.Equal(t, "lv", cfg.Storage.S3.Bucket)
	assert.Equal(t, "ak", cfg.Storage.S3.AccessKey)
	assert.Equal(t, "sk", cfg.Storage.S3.SecretKey)
	assert.Equal(t, "us-east-1", cfg.Storage.S3.Region)
	assert.False(t, cfg.Storage.S3.Secure)
	assert.False(t, cfg.Storage.S3.PathStyle)
}

func TestDqliteFlagsMapToNestedKeys(t *testing.T) {
	flags := pflag.NewFlagSet("dqlite-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	err := flags.Parse([]string{
		"--db-dqlite-addrs", "node1:9001,node2:9001",
		"--db-dqlite-discovery-srv", "_dqlite._tcp.librevita.svc.cluster.local",
		"--db-dqlite-database", "lv",
		"--db-sqlite-path", "/tmp/foo.db",
	})
	require.NoError(t, err)

	k := koanf.New(".")
	err = k.Load(posflag.ProviderWithFlag(flags, ".", k, func(f *pflag.Flag) (string, any) {
		return mapFlagKey(f.Name), posflag.FlagVal(flags, f)
	}), nil)
	require.NoError(t, err)

	var cfg Config
	err = k.Unmarshal("", &cfg)
	require.NoError(t, err)

	assert.Equal(t, "node1:9001,node2:9001", cfg.Database.Dqlite.Addrs)
	assert.Equal(t, "_dqlite._tcp.librevita.svc.cluster.local", cfg.Database.Dqlite.DiscoverySRV)
	assert.Equal(t, "lv", cfg.Database.Dqlite.Database)
	assert.Equal(t, "/tmp/foo.db", cfg.Database.SQLite.Path)
}

func TestDqliteEnvKeysMapToNestedKeys(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"database_dqlite_discovery_srv", "database.dqlite.discovery_srv"},
		{"database_dqlite_addrs", "database.dqlite.addrs"},
		{"database_dqlite_database", "database.dqlite.database"},
		{"database_sqlite_path", "database.sqlite.path"},
		{"database_driver", "database.driver"},
	} {
		assert.Equal(t, tc.want, mapEnvironmentKey(tc.in), "key mapping for %s", tc.in)
	}
}

func TestDqliteEnvKeysRejectAliases(t *testing.T) {
	for _, alias := range []string{"db_path", "db_sqlite_path", "database_path", "db_dqlite_addrs", "dqlite_addrs", "db_dqlite_discovery_srv", "dqlite_discovery_srv", "db_dqlite_database", "dqlite_database", "db_driver"} {
		assert.Empty(t, mapEnvironmentKey(alias), "%s should have no alias", alias)
	}
}

func TestLoggingEnvKeysRejectAliases(t *testing.T) {
	for _, alias := range []string{"log_mode", "log_path", "logging_path", "log_max_size_mb", "logging_max_size_mb", "log_max_backups", "logging_max_backups", "log_max_age_days", "logging_max_age_days", "log_compress", "logging_compress"} {
		assert.Empty(t, mapEnvironmentKey(alias), "%s should have no alias", alias)
	}
	for _, canonical := range []string{"logging_mode", "logging_file_path", "logging_rotating_path", "logging_rotating_max_size_mb", "logging_rotating_max_backups", "logging_rotating_max_age_days", "logging_rotating_compress"} {
		assert.NotEmpty(t, mapEnvironmentKey(canonical), "%s should map to a key", canonical)
	}
}

func TestConfigFileEnvKeyPairing(t *testing.T) {
	assert.Equal(t, keyConfigFile, mapEnvironmentKey("config"))
	assert.Empty(t, mapEnvironmentKey("config_file"))
}

func TestValidateTrustedProxies(t *testing.T) {
	cases := []struct {
		name    string
		proxies string
		wantErr bool
	}{
		{"empty", "", false},
		{"cidr", "10.0.0.0/8", false},
		{"ip", "192.168.1.5", false},
		{"mixed", "10.0.0.0/8, 192.168.1.5", false},
		{"trailing comma", "10.0.0.0/8,", false},
		{"typo", "10.0.0.0/33", true},
		{"not an address", "proxy.example.org", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Database:       DatabaseConfig{Driver: DriverSQLite},
				Logging:        LoggingConfig{Mode: LogModeConsole},
				TrustedProxies: tc.proxies,
			}
			err := cfg.validate()
			if tc.wantErr {
				assert.Error(t, err, "validate(%q) should fail", tc.proxies)
			} else {
				assert.NoError(t, err, "validate(%q) should succeed", tc.proxies)
			}
		})
	}
}

func TestValidateHTTPPort(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{Driver: DriverSQLite}, Logging: LoggingConfig{Mode: LogModeConsole}}
	cfg.normalize()
	assert.Equal(t, defaultHTTPPort, cfg.HTTPPort)

	cfg.HTTPPort = 70000
	assert.Error(t, cfg.validate())

	cfg.HTTPPort = 443
	assert.NoError(t, cfg.validate())
}

func TestVaultConfigDefaultsAndValidation(t *testing.T) {
	cfg := &Config{
		DataDir:  "/tmp/librevita",
		Database: DatabaseConfig{Driver: DriverSQLite},
		Logging:  LoggingConfig{Mode: LogModeConsole},
	}
	cfg.normalize()

	assert.Equal(t, "bbolt", cfg.Vault.Backend)
	wantPath := filepath.Join("/tmp/librevita", "keys.db")
	assert.Equal(t, wantPath, cfg.Vault.BBolt.Path)
	assert.NoError(t, cfg.validate())

	cfg.Vault.Backend = "invalid"
	assert.Error(t, cfg.validate())
}

func TestPostgresConfigDefaultsAndValidation(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{
			Driver: DriverPostgres,
			Postgres: PostgresConfig{
				Host:     "db.example.com",
				Port:     5432,
				User:     "app",
				Password: "secret",
				Database: "librevita_prod",
				SSLMode:  "require",
			},
		},
		Logging: LoggingConfig{Mode: LogModeConsole},
	}
	cfg.normalize()

	assert.Equal(t, DriverPostgres, cfg.Database.Driver)
	assert.Equal(t, 25, cfg.Database.Postgres.MaxOpenConns)
	assert.Equal(t, 5, cfg.Database.Postgres.MaxIdleConns)
	assert.NoError(t, cfg.validate())

	dsn := cfg.Database.Postgres.DSN()
	wantDSN := "postgres://app:secret@db.example.com:5432/librevita_prod?sslmode=require"
	assert.Equal(t, wantDSN, dsn)
}

func TestPostgresFlagsAndEnvMapping(t *testing.T) {
	flags := pflag.NewFlagSet("postgres-test", pflag.ContinueOnError)
	RegisterFlags(flags)

	flagNames := []string{
		"db-postgres-url",
		"db-postgres-host",
		"db-postgres-port",
		"db-postgres-user",
		"db-postgres-password",
		"db-postgres-database",
		"db-postgres-sslmode",
		"db-postgres-max-open-conns",
		"db-postgres-max-idle-conns",
	}
	for _, name := range flagNames {
		assert.NotEmpty(t, mapFlagKey(name), "flag %s should map to a key", name)
	}

	envKeys := []string{
		"database_postgres_url",
		"database_postgres_host",
		"database_postgres_port",
		"database_postgres_user",
		"database_postgres_password",
		"database_postgres_database",
		"database_postgres_sslmode",
		"database_postgres_max_open_conns",
		"database_postgres_max_idle_conns",
	}
	for _, k := range envKeys {
		assert.NotEmpty(t, mapEnvironmentKey(k), "env %s should map to a key", k)
	}
}
