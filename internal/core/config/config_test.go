package config

import (
	"os"
	"path/filepath"
	"testing"

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
