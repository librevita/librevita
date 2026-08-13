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
	t.Setenv("LIBREVITA_ENV", "production")
	t.Setenv("LIBREVITA_DB_PATH", "env.db")

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := []byte("env: development\nhttp_addr: ':9000'\ndatabase:\n  driver: sqlite\n  path: file.db\n  dqlite_addrs: node1:9001,node2:9001\n  dqlite_database: lv\n")
	if err := os.WriteFile(configFile, configYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	flags := pflag.NewFlagSet("config-test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Parse([]string{"--config", configFile, "--http-addr", ":9100"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := load(flags)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ConfigFile != configFile {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, configFile)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want production", cfg.Env)
	}
	if cfg.HTTPAddr != ":9100" {
		t.Errorf("HTTPAddr = %q, want :9100", cfg.HTTPAddr)
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
		wantErr bool
	}{
		{"valid", "node1:9001, node2:9001", false},
		{"single", "node1:9001", false},
		{"missing", "", true},
		{"only separators", ",, ,", true},
		{"whitespace", "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Database: DatabaseConfig{Driver: DriverDqlite, DqliteAddrs: tc.addrs, DqliteDatabase: "lv"},
				Logging:  LoggingConfig{Mode: LogModeConsole},
			}
			err := cfg.validate()
			if tc.wantErr && err == nil {
				t.Fatalf("validate(%q) = nil, want error", tc.addrs)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate(%q) = %v, want nil", tc.addrs, err)
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
