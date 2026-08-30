package config

import "strings"

// configKeys is the canonical list of Koanf configuration properties.
var configKeys = []string{
	// Top-level
	"mode",
	"http_bind",
	"http_port",
	"base_domain",
	"trusted_proxies",
	"hsts_max_age",
	"data_dir",
	"paseto_key",
	"master_key",

	// Auth
	"auth.max_concurrent_hashes",

	// Database (flags use db- prefix)
	"database.driver",
	"database.sqlite.path",
	"database.postgres.url",
	"database.postgres.host",
	"database.postgres.port",
	"database.postgres.user",
	"database.postgres.password",
	"database.postgres.database",
	"database.postgres.sslmode",
	"database.postgres.max_open_conns",
	"database.postgres.max_idle_conns",
	"database.dqlite.addrs",
	"database.dqlite.discovery_srv",
	"database.dqlite.database",

	// Logging (flags use log- prefix)
	"logging.mode",
	"logging.level",
	"logging.file.path",
	"logging.rotating.path",
	"logging.rotating.max_size_mb",
	"logging.rotating.max_backups",
	"logging.rotating.max_age_days",
	"logging.rotating.compress",

	// Storage
	"storage.backend",
	"storage.local.dir",
	"storage.s3.endpoint",
	"storage.s3.bucket",
	"storage.s3.access_key",
	"storage.s3.secret_key",
	"storage.s3.region",
	"storage.s3.secure",
	"storage.s3.path_style",

	// Vault
	"vault.backend",
	"vault.bbolt.path",
	"vault.nats.url",
	"vault.nats.bucket",
	"vault.etcd.endpoints",
	"vault.etcd.prefix",
	"vault.hashicorp.address",
	"vault.hashicorp.token",
	"vault.hashicorp.mount",

	// Crypto
	"crypto.hash_algorithm",
	"crypto.encryption_cipher",
}

// Specific CLI flag aliases.
var flagAliases = map[string]string{ // #nosec G101 -- Koanf field paths, not credentials.
	"config":                keyConfigFile,
	"config-file":           keyConfigFile,
	"log-rotating-max-size": "logging.rotating.max_size_mb",
	"log-rotating-max-age":  "logging.rotating.max_age_days",
}

// Specific environment variable aliases.
var envAliases = map[string]string{ // #nosec G101 -- Koanf field paths, not credentials.
	"config":    keyConfigFile,
	"http_addr": "http_bind",
}

var (
	flagKeys = make(map[string]string)
	envKeys  = make(map[string]string)
)

func init() {
	for _, k := range configKeys {
		flagKeys[koanfToFlag(k)] = k
		envKeys[koanfToEnv(k)] = k
	}
	for alias, k := range flagAliases {
		flagKeys[strings.ReplaceAll(alias, "_", "-")] = k
	}
	for alias, k := range envAliases {
		envKeys[alias] = k
	}
}

func koanfToFlag(k string) string {
	s := strings.ReplaceAll(strings.ReplaceAll(k, ".", "-"), "_", "-")
	if strings.HasPrefix(s, "database-") {
		return "db-" + strings.TrimPrefix(s, "database-")
	}
	if strings.HasPrefix(s, "logging-") {
		return "log-" + strings.TrimPrefix(s, "logging-")
	}
	return s
}

func koanfToEnv(k string) string {
	return strings.ReplaceAll(k, ".", "_")
}

func mapFlagKey(name string) string {
	return flagKeys[strings.ToLower(strings.ReplaceAll(name, "_", "-"))]
}

func mapEnvironmentKey(key string) string {
	return envKeys[strings.ToLower(strings.TrimPrefix(key, envPrefix))]
}
