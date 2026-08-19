package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

func TestLoadPrecedence(t *testing.T) {
	t.Setenv("LIBREVITA_MODE", "production")
	t.Setenv("LIBREVITA_DATABASE_SQLITE_PATH", "env.db")

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := []byte("mode: development\nhttp_bind: '127.0.0.1'\nhttp_port: 9000\ndatabase:\n  driver: sqlite\n  sqlite:\n    path: file.db\n  dqlite:\n    addrs: node1:9001,node2:9001\n    database: lv\n")
	if err := os.WriteFile(configFile, configYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	flags := pflag.NewFlagSet("config-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Parse([]string{"--config", configFile, "--http-bind", "0.0.0.0", "--http-port", "9100"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := load(flags)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ConfigFile != configFile {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, configFile)
	}
	if cfg.Mode != "production" {
		t.Errorf("Mode = %q, want production", cfg.Mode)
	}
	if cfg.HTTPBind != "0.0.0.0" {
		t.Errorf("HTTPBind = %q, want 0.0.0.0", cfg.HTTPBind)
	}
	if cfg.HTTPPort != 9100 {
		t.Errorf("HTTPPort = %d, want 9100", cfg.HTTPPort)
	}
	if cfg.Database.SQLite.Path != "env.db" {
		t.Errorf("Database.SQLite.Path = %q, want env.db", cfg.Database.SQLite.Path)
	}
	if cfg.Database.Dqlite.Addrs != "node1:9001,node2:9001" {
		t.Errorf("Database.Dqlite.Addrs = %q, want node1:9001,node2:9001", cfg.Database.Dqlite.Addrs)
	}
	if cfg.Database.Dqlite.Database != "lv" {
		t.Errorf("Database.Dqlite.Database = %q, want lv", cfg.Database.Dqlite.Database)
	}
	if cfg.Logging.Mode != LogModeConsole {
		t.Errorf("Logging.Mode = %q, want console", cfg.Logging.Mode)
	}
	if cfg.Logging.Rotating.MaxSizeMB != defaultLogSizeMB {
		t.Errorf("Logging.Rotating.MaxSizeMB = %d, want %d", cfg.Logging.Rotating.MaxSizeMB, defaultLogSizeMB)
	}
	if !cfg.Logging.Rotating.Compress {
		t.Error("Logging.Rotating.Compress = false, want true by default")
	}
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
			if tc.wantErr && err == nil {
				t.Fatalf("validate(%q/%q) = nil, want error", tc.addrs, tc.srv)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate(%q/%q) = %v, want nil", tc.addrs, tc.srv, err)
			}
		})
	}
}

func TestStorageFlagsMapToNestedKeys(t *testing.T) {
	flags := pflag.NewFlagSet("storage-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Parse([]string{
		"--storage-backend", "s3",
		"--storage-local-dir", "/tmp/files",
		"--storage-s3-endpoint", "minio:9000",
		"--storage-s3-bucket", "lv",
		"--storage-s3-access-key", "ak",
		"--storage-s3-secret-key", "sk",
		"--storage-s3-region", "us-east-1",
		"--storage-s3-secure=false",
		"--storage-s3-path-style=false",
	}); err != nil {
		t.Fatal(err)
	}
	k := koanf.New(".")
	if err := k.Load(posflag.ProviderWithFlag(flags, ".", k, func(f *pflag.Flag) (string, any) {
		return mapFlagKey(f.Name), posflag.FlagVal(flags, f)
	}), nil); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Backend != "s3" || cfg.Storage.Local.Dir != "/tmp/files" {
		t.Errorf("local: backend=%q dir=%q", cfg.Storage.Backend, cfg.Storage.Local.Dir)
	}
	if cfg.Storage.S3.Endpoint != "minio:9000" || cfg.Storage.S3.Bucket != "lv" ||
		cfg.Storage.S3.AccessKey != "ak" || cfg.Storage.S3.SecretKey != "sk" ||
		cfg.Storage.S3.Region != "us-east-1" || cfg.Storage.S3.Secure || cfg.Storage.S3.PathStyle {
		t.Errorf("s3 = %+v", cfg.Storage.S3)
	}
}

func TestDqliteFlagsMapToNestedKeys(t *testing.T) {
	flags := pflag.NewFlagSet("dqlite-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Parse([]string{
		"--db-dqlite-addrs", "node1:9001,node2:9001",
		"--db-dqlite-discovery-srv", "_dqlite._tcp.librevita.svc.cluster.local",
		"--db-dqlite-database", "lv",
		"--db-sqlite-path", "/tmp/foo.db",
	}); err != nil {
		t.Fatal(err)
	}
	k := koanf.New(".")
	if err := k.Load(posflag.ProviderWithFlag(flags, ".", k, func(f *pflag.Flag) (string, any) {
		return mapFlagKey(f.Name), posflag.FlagVal(flags, f)
	}), nil); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Dqlite.Addrs != "node1:9001,node2:9001" {
		t.Errorf("Dqlite.Addrs = %q", cfg.Database.Dqlite.Addrs)
	}
	if cfg.Database.Dqlite.DiscoverySRV != "_dqlite._tcp.librevita.svc.cluster.local" {
		t.Errorf("Dqlite.DiscoverySRV = %q", cfg.Database.Dqlite.DiscoverySRV)
	}
	if cfg.Database.Dqlite.Database != "lv" {
		t.Errorf("Dqlite.Database = %q", cfg.Database.Dqlite.Database)
	}
	if cfg.Database.SQLite.Path != "/tmp/foo.db" {
		t.Errorf("SQLite.Path = %q", cfg.Database.SQLite.Path)
	}
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
		if got := mapEnvironmentKey(tc.in); got != tc.want {
			t.Errorf("%s -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDqliteEnvKeysRejectAliases(t *testing.T) {
	for _, alias := range []string{"db_path", "db_sqlite_path", "database_path", "db_dqlite_addrs", "dqlite_addrs", "db_dqlite_discovery_srv", "dqlite_discovery_srv", "db_dqlite_database", "dqlite_database", "db_driver"} {
		if got := mapEnvironmentKey(alias); got != "" {
			t.Errorf("%s -> %q, want empty (alias removed)", alias, got)
		}
	}
}

func TestLoggingEnvKeysRejectAliases(t *testing.T) {
	for _, alias := range []string{"log_mode", "log_path", "logging_path", "log_max_size_mb", "logging_max_size_mb", "log_max_backups", "logging_max_backups", "log_max_age_days", "logging_max_age_days", "log_compress", "logging_compress"} {
		if got := mapEnvironmentKey(alias); got != "" {
			t.Errorf("%s -> %q, want empty (alias removed)", alias, got)
		}
	}
	for _, canonical := range []string{"logging_mode", "logging_file_path", "logging_rotating_path", "logging_rotating_max_size_mb", "logging_rotating_max_backups", "logging_rotating_max_age_days", "logging_rotating_compress"} {
		if got := mapEnvironmentKey(canonical); got == "" {
			t.Errorf("%s -> empty, want a nested key", canonical)
		}
	}
}

func TestConfigFileEnvKeyPairing(t *testing.T) {
	if got := mapEnvironmentKey("config"); got != keyConfigFile {
		t.Errorf("config -> %q, want %q", got, keyConfigFile)
	}
	if got := mapEnvironmentKey("config_file"); got != "" {
		t.Errorf("config_file -> %q, want empty (removed; the paired variable is LIBREVITA_CONFIG)", got)
	}
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
			if tc.wantErr && err == nil {
				t.Fatalf("validate(%q) = nil, want error", tc.proxies)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate(%q) = %v, want nil", tc.proxies, err)
			}
		})
	}
}

func TestValidateHTTPPort(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{Driver: DriverSQLite}, Logging: LoggingConfig{Mode: LogModeConsole}}
	cfg.normalize()
	if cfg.HTTPPort != defaultHTTPPort {
		t.Errorf("HTTPPort = %d, want default %d", cfg.HTTPPort, defaultHTTPPort)
	}
	cfg.HTTPPort = 70000
	if err := cfg.validate(); err == nil {
		t.Fatal("validate(70000) = nil, want error")
	}
	cfg.HTTPPort = 443
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate(443) = %v, want nil", err)
	}
}

func TestVaultConfigDefaultsAndValidation(t *testing.T) {
	cfg := &Config{
		DataDir:  "/tmp/librevita",
		Database: DatabaseConfig{Driver: DriverSQLite},
		Logging:  LoggingConfig{Mode: LogModeConsole},
	}
	cfg.normalize()

	if cfg.Vault.Backend != "bbolt" {
		t.Errorf("Vault.Backend = %q, want bbolt", cfg.Vault.Backend)
	}
	wantPath := filepath.Join("/tmp/librevita", "keys.db")
	if cfg.Vault.BBolt.Path != wantPath {
		t.Errorf("Vault.BBolt.Path = %q, want %q", cfg.Vault.BBolt.Path, wantPath)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() = %v, want nil", err)
	}

	cfg.Vault.Backend = "invalid"
	if err := cfg.validate(); err == nil {
		t.Fatal("validate(invalid) = nil, want error")
	}
}
