# LibreVita

Self-hosted medical clinic management software built in Go. The module path is
`librevita.org`.

## Requirements

- Earthly 0.8 or newer
- Podman or Docker for the Earthly BuildKit daemon

There is no Makefile. Earthly is the build and development interface.

## Earthly Targets

```sh
earthly +generate
earthly +build-dev
earthly +build
earthly +image --IMAGE_TAG=librevita:latest
earthly +test
earthly +vet
earthly +tidy
earthly +build-cross --GOOS=linux --GOARCH=riscv64
earthly +build-cross --GOOS=linux --GOARCH=loong64
earthly +build-cross --GOOS=linux --GOARCH=mips64
```

`+build-dev` writes the fast, unoptimized `bin/librevita-dev` binary. `+build`
writes the optimized production binary to `bin/librevita`. Cross builds write
files such as `bin/librevita-linux-riscv64`.

SQLC and templ output is not committed. `+generate` writes generated files to
the workspace when needed for editor support; build, test, and vet targets
generate them inside Earthly.

## Container Image

`+image` builds the production binary and packages it in a `scratch` OCI image.
The image runs as non-root UID/GID `65532:65532` and sets
`LIBREVITA_DATA_DIR=/data/librevita`. Go creates that directory and its files
at startup.

Docker and Podman use the same commands:

```sh
earthly +image --IMAGE_TAG=librevita:latest
podman run --rm \
  -p 8080:8080 \
  -v librevita-data:/data \
  -e LIBREVITA_ENV=production \
  librevita:latest
```

Replace `podman` with `docker` when using Docker. Timezone data is embedded by
Go through `time/tzdata`, and the CA bundle is embedded through `rootcerts`.
The image has no shell, package manager, or pre-created data directory. The
mounted `/data` volume must be writable by UID/GID `65532:65532`.

The Earthfile uses `golang:1.26.5-alpine3.24` and sets `CGO_ENABLED=0` for
every Go build. This keeps the binaries statically linkable across supported
architectures.

The build cache is split into three layers:

- Go module downloads
- `templ` and `sqlc` installation
- Project source files

Changing application source does not invalidate the dependency or tool layers.

If a rootful Earthly BuildKit daemon already occupies ports `8371` and `8372`,
configure the client to reuse it:

```sh
earthly config global.buildkit_host 'tcp://localhost.:8372'
```

The trailing dot prevents Earthly from treating the existing TLS endpoint as a
local daemon that it should manage itself.

## Configuration

Configuration is loaded by Koanf with this precedence, from lowest to highest:

1. Built-in defaults
2. `config.yaml`, `config.yml`, `config.json`, or the file passed with `--config`
3. `.env` and `LIBREVITA_*` environment variables
4. Command-line flags

Example `config.yaml`:

```yaml
env: production
http_addr: ":8080"
data_dir: ./data
database:
  driver: sqlite
  path: ./librevita.db
  rqlite_addr: http://localhost:4001
logging:
  mode: console
  path: ./librevita.log
  max_size_mb: 100
  max_backups: 3
  max_age_days: 28
  compress: true
```

The main flags are:

| Flag | Environment variable | Purpose |
| --- | --- | --- |
| `--config` | `LIBREVITA_CONFIG_FILE` | Configuration file path |
| `--env` | `LIBREVITA_ENV` | Runtime environment |
| `--http-addr` | `LIBREVITA_HTTP_ADDR` | HTTP bind address |
| `--data-dir` | `LIBREVITA_DATA_DIR` | Base directory for default database and logs |
| `--db-driver` | `LIBREVITA_DB_DRIVER` | `sqlite` or `rqlite` |
| `--db-path` | `LIBREVITA_DB_PATH` | SQLite file path |
| `--rqlite-addr` | `LIBREVITA_RQLITE_ADDR` | rqlite node URL |
| `--log-mode` | `LIBREVITA_LOG_MODE` | `console`, `file`, or `rotating` |
| `--log-path` | `LIBREVITA_LOG_PATH` | File destination |
| `--log-max-size` | `LIBREVITA_LOG_MAX_SIZE_MB` | Rotating file size in MB |
| `--log-max-backups` | `LIBREVITA_LOG_MAX_BACKUPS` | Number of rotated files |
| `--log-max-age` | `LIBREVITA_LOG_MAX_AGE_DAYS` | Maximum rotated file age |
| `--log-compress` | `LIBREVITA_LOG_COMPRESS` | Compress rotated files |
| `--paseto-key` | `LIBREVITA_PASETO_KEY` | Session key (base64, 32 bytes; required outside development) |
| `--auth-max-concurrent-hashes` | `LIBREVITA_AUTH_MAX_CONCURRENT_HASHES` | Bound on concurrent Argon2id operations |

`LIBREVITA_DATABASE_*` names are also accepted for database settings.

## Logging

The application uses `log/slog` with Zap and `zapslog`. Fx and Goose use the
same logger.

Development output is human-readable text with one record per line. Records
are emitted as columns, not JSON. The source path is shortened to
`file.go:line`, and lines are truncated to the terminal width. Set `COLUMNS` to
override the detected width; the fallback is 120 columns.

Production output is always JSON. The selected mode controls the destination:

- `console` writes JSON to stderr
- `file` appends JSON to the configured file
- `rotating` uses `lumberjack` with the configured size, backup count, age, and compression settings

Example production commands:

```sh
LIBREVITA_ENV=production \
LIBREVITA_LOG_MODE=file \
LIBREVITA_LOG_PATH=./librevita.log \
./bin/librevita

LIBREVITA_ENV=production \
LIBREVITA_LOG_MODE=rotating \
LIBREVITA_LOG_PATH=./librevita.log \
LIBREVITA_LOG_MAX_SIZE_MB=100 \
LIBREVITA_LOG_MAX_BACKUPS=3 \
LIBREVITA_LOG_MAX_AGE_DAYS=28 \
LIBREVITA_LOG_COMPRESS=true \
./bin/librevita
```

## Database

SQLite uses `modernc.org/sqlite`, so it does not require CGO. The connection
factory enables WAL mode, a busy timeout, foreign keys, and synchronous mode.
The SQL pool is limited to one open connection because SQLite has a single
writer.

Primary keys are UUIDv7 (`TEXT`), generated by the application through
`github.com/google/uuid` and stored in canonical lowercase form. UUIDv7 is
temporally sortable, non-enumerable (a patient id never reveals "patient
#42"), and stable across independent databases, which matters for importing
or merging clinical records. IDs are generated in the application, never by
SQLite defaults, so the code never depends on `last_insert_rowid`; the
`id` column is passed explicitly to inserts. Display identifiers such as an
MRN remain separate columns.

Go creates `data_dir` at startup. If database or log paths are not set
explicitly, they are created as `data_dir/librevita.db` and
`data_dir/librevita.log`.

The default driver is embedded SQLite. Set `LIBREVITA_DB_DRIVER=rqlite` to use
the `gorqlite` client for a distributed deployment. Goose migrations run on
the embedded SQLite backend; rqlite schema management remains a cluster-level
operation.

## Migrations

Migration files live in `db/migrations` and are embedded into the binary. The
Fx database lifecycle applies pending Goose migrations before the HTTP server
starts. Goose logs use the same structured logger as the rest of the process.

Generated repository code is not a source artifact. Edit the SQL under
`db/schema` or `db/query`, then run `earthly +generate` when local generated
files are needed.

## HTTP Server

Echo is created and managed by Fx. The foundation currently exposes:

- `GET /healthz`
- `GET /setup`, `POST /setup` (initial onboarding)
- `GET /auth/login`, `POST /auth/login`
- `GET /auth/register`, `POST /auth/register`
- `POST /auth/logout`
- `GET /` (authenticated dashboard)
- `GET /admin` (admin role only)

The endpoint returns `{"status":"ok"}`. Business routes are not registered yet.
HTTP errors use RFC 7807 `application/problem+json` responses.

## Onboarding

A fresh installation has no accounts. Any navigation while the system is
not onboarded (login, dashboard, register) is redirected to `GET /setup`,
which creates the initial `admin` account and the clinic profile (name, tax
id, contact, and address) in a single atomic transaction. Setup runs exactly
once: the transaction also persists a `setup_completed` marker in the `meta`
table, so the system stays onboarded even if every account and the clinic
are later removed. Onboarded systems redirect setup requests to the login
page, and concurrent setup attempts never produce more than one admin:
exactly one wins, the rest receive the redirect. Setup is rate-limited to 5
attempts per minute per IP.

After onboarding, account creation is never public: `RequireAuth` plus the
`users.register` policy guard the registration routes. The default policy
restricts registration to the `admin` role; an operator can tighten it to a
single user (`principal.email == 'hr@example.org'`) or close it entirely
(`false`). The created accounts are `patient` by default; role assignment is
an admin responsibility.

## Authentication and Authorization

Authentication lives in `internal/core/auth` (transport-agnostic) with HTTP
adapters in `internal/core/server`:

- Passwords are hashed with Argon2id (`golang.org/x/crypto/argon2`)
- Sessions are PASETO v4.local tokens (`aidanwoods.dev/go-paseto`): the
  payload is encrypted with XChaCha20-Poly1305 under a single server key and
  validated cryptographically on every request. The `sessions` table holds
  only the token id (SHA-256) for revocation, logout, and account
  deactivation checks. The cookie is `HttpOnly` and `SameSite=Lax`, with the
  `Secure` flag enabled in production
- The session key is `LIBREVITA_PASETO_KEY` (base64, 32 bytes). Every
  environment except the explicit `development` requires the key and sets
  the `Secure` flag on cookies; only `development` falls back to an
  ephemeral key (sessions reset on restart). Deployments labeled
  `staging`, `prod`, or any other value are treated as persistent
- Concurrent Argon2id operations are bounded by
  `--auth-max-concurrent-hashes` (default 4, ~64 MiB each) to protect the
  process from memory exhaustion under abusive login traffic
- CSRF uses the double-submit cookie pattern. Forms embed the token in the
  `_csrf` field; HTMX and fetch requests send it in the `X-CSRF-Token` header
- Authorization is policy-based and lives in `internal/core/policy`. Roles
  (`admin`, `physician`, `receptionist`, `patient`) are principal attributes;
  permissions are CEL expressions compiled once at startup and evaluated per
  request. `RequireAuth` redirects anonymous browsers to the login page, and
  `RequirePolicy(name)` returns an RFC 7807 `403` when the policy denies

CEL (`github.com/google/cel-go`) is a non-Turing-complete expression
language: it has no loops, recursion, or side effects, so authorization
rules are bounded, safe to evaluate, and auditable. Policies receive two
variables:

- `principal` — `id`, `email`, `name`, `role`
- `request` — `method`, `path`

Default policies are seeded into the `policies` table on startup. The stored
expression always wins, and the admin panel (`/admin/policies`) edits them at
runtime: every change is validated before activation (the expression must
compile and evaluate to a boolean; a broken policy is rejected and the
previous one stays active), takes effect immediately, and is written to the
audit trail. Each change is also versioned in `policy_versions` with the
acting user and timestamp; the panel shows the latest versions per policy.

Critical policies (`admin.view`) are protected against self-lockout: a
change that would deny the admin role is rejected, because the admin panel
is the only place that could restore it.

| Policy | Expression |
| --- | --- |
| `dashboard.view` | `principal.role in ['admin', 'physician', 'receptionist', 'patient']` |
| `admin.view` | `principal.role == 'admin'` |
| `users.register` | `principal.role == 'admin'` |

Abuse controls:

- Login is rate-limited to 10 attempts per minute per IP (`429` beyond that)
- The request body is limited to 1 MiB, and input fields have explicit
  length limits
- Login runs an Argon2id verification even for unknown or deactivated
  accounts, so response timing does not reveal whether an email exists
- The HTTP server enforces read timeouts

All authentication and authorization outcomes (register, login, logout,
policy denials) are written to the durable `audit_log` table by
`internal/core/audit`. The trail records actor, action, resource, result,
IP, request id, and a detail message; passwords, tokens, and CSRF values are
never stored. Recording is best-effort and never breaks the audited
operation.

Sessions require the SQLite backend; rqlite deployments must provide their
own authentication layer.
