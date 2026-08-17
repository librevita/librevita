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
	t.Setenv("LIBREVITA_DB_PATH", "env.db")

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := []byte("mode: development\nhttp_bind: '127.0.0.1'\nhttp_port: 9000\ndatabase:\n  driver: sqlite\n  path: file.db\n  dqlite_addrs: node1:9001,node2:9001\n  dqlite_database: lv\n")
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
	if cfg.Database.Path != "env.db" {
		t.Errorf("Database.Path = %q, want env.db", cfg.Database.Path)
	}
	if cfg.Database.DqliteAddrs != "node1:9001,node2:9001" {
		t.Errorf("Database.DqliteAddrs = %q, want node1:9001,node2:9001", cfg.Database.DqliteAddrs)
	}
	if cfg.Database.DqliteDatabase != "lv" {
		t.Errorf("Database.DqliteDatabase = %q, want lv", cfg.Database.DqliteDatabase)
	}
	if cfg.Logging.Mode != LogModeConsole {
		t.Errorf("Logging.Mode = %q, want console", cfg.Logging.Mode)
	}
	if cfg.Logging.MaxSizeMB != defaultLogSizeMB {
		t.Errorf("Logging.MaxSizeMB = %d, want %d", cfg.Logging.MaxSizeMB, defaultLogSizeMB)
	}
	if !cfg.Logging.Compress {
		t.Error("Logging.Compress = false, want true by default")
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
				Database: DatabaseConfig{Driver: DriverDqlite, DqliteAddrs: tc.addrs, DqliteDiscoverySRV: tc.srv, DqliteDatabase: "lv"},
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
		"--dqlite-addrs", "node1:9001,node2:9001",
		"--dqlite-discovery-srv", "_dqlite._tcp.librevita.svc.cluster.local",
		"--dqlite-database", "lv",
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
	if cfg.Database.DqliteAddrs != "node1:9001,node2:9001" {
		t.Errorf("DqliteAddrs = %q", cfg.Database.DqliteAddrs)
	}
	if cfg.Database.DqliteDiscoverySRV != "_dqlite._tcp.librevita.svc.cluster.local" {
		t.Errorf("DqliteDiscoverySRV = %q", cfg.Database.DqliteDiscoverySRV)
	}
	if cfg.Database.DqliteDatabase != "lv" {
		t.Errorf("DqliteDatabase = %q", cfg.Database.DqliteDatabase)
	}
}

func TestDqliteEnvKeysMapToNestedKeys(t *testing.T) {
	if got := mapEnvironmentKey("database_dqlite_discovery_srv"); got != "database.dqlite_discovery_srv" {
		t.Errorf("database_dqlite_discovery_srv -> %q", got)
	}
	if got := mapEnvironmentKey("dqlite_discovery_srv"); got != "database.dqlite_discovery_srv" {
		t.Errorf("dqlite_discovery_srv -> %q", got)
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
