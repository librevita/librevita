# LibreVita

[![CI](https://github.com/librevita/librevita/actions/workflows/ci.yaml/badge.svg)](https://github.com/librevita/librevita/actions/workflows/ci.yaml)

Self-hosted medical clinic management software built in Go. The module path is `librevita.org`.

## Requirements

- Earthly 0.8 or newer
- Podman or Docker for the Earthly BuildKit daemon

There is no Makefile. Earthly is the build and development interface.

## Earthly Targets

```sh
earthly +go-gen-templ +go-gen-sqlc +go-gen-schema
earthly +build --dev=true
earthly +build
earthly +image --IMAGE_TAG=librevita:latest
earthly +go-test
earthly +go-vet
earthly +go-lint
earthly +go-vuln-source
earthly +go-tidy
earthly +build --os=linux --arch=riscv64
earthly +build --os=linux --arch=loong64
earthly +build --os=linux --arch=mips64
```

`+build` writes the optimized production binary to `bin/librevita`. `--dev=true` writes the fast, unoptimized
`bin/librevita-dev` binary. Cross builds write files such as `bin/librevita-linux-riscv64`. `--name=myapp` renames the
exported binaries (the `name` global arg, default `librevita`).

SQLC and templ output is not committed. The generation stages (`+go-gen-templ`, `+go-gen-sqlc`, `+go-gen-schema`)
export their outputs to the workspace for editor support; build, test, and vet targets generate them inside Earthly.

## Frontend

The UI follows the GOTH stack: Go + templ + HTMX, server-driven and progressive. Assets are embedded in the binary
(`internal/ui`, served under `/static`) — there is no CDN, npm, or Node at runtime:

- **HTMX 1.9.12** for hypermedia interactions (`allowEval=false`, `allowScriptTags=false`), with the SSE extension for
  agenda live updates
- **First-party TypeScript** modules (bundled by esbuild) for the ephemeral client state (menus, tabs, theme); clinical
  state always lives on the server. The strict Content-Security-Policy (`script-src 'self'`, no `unsafe-eval`) is never
  relaxed
- **Tailwind CSS 3.4.17** compiled at build time with a hex palette override (the v3.4 default `oklch` colors are
  unparseable by XP-era browsers)
- The frontend build is driven by **Node** (`node:26.7-alpine3.24`, pinned by digest in the Earthfile): `package.json`
  declares the dependencies and scripts, and `package-lock.json` pins them with integrity hashes — nothing is versioned
  inside the Earthfile. esbuild bundles the TypeScript source to the XP floor (`target=firefox58`, verified by
  `assertXpFloor` after the build) and the PostCSS pipeline compiles Tailwind from `internal/ui/input.css`; no one
  writes ES5
- **TypeScript** in `internal/ui/ts` with strict checking (`tsc --noEmit`, `lib: ES2017+DOM` aligned to the XP floor, so
  APIs missing from Firefox 52 are compile errors)

The runtime assets (HTMX and its SSE extension) come from npm packages pinned in `package-lock.json` and are documented
in `VENDOR.md`. The compatibility floor is Windows XP-era browsers (Pale Moon 28.8, Basilisk 55, K-Meleon 74G/76,
Firefox 52 ESR); newer engines are covered automatically. The server applies a strict CSP, `X-Content-Type-Options:
nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, and `Cache-Control: no-store` on all non-static
responses; the application is same-origin and no CORS is configured.

## Container Image

`+image` builds the production binary and packages it in a `scratch` OCI image. The image runs as non-root UID/GID
`65532:65532` and sets `LIBREVITA_DATA_DIR=/data/librevita`. Go creates that directory and its files at startup.

Docker and Podman use the same commands:

```sh
earthly +image --IMAGE_TAG=librevita:latest
podman run --rm \
  -p 8080:8080 \
  -v librevita-data:/data \
  -e LIBREVITA_ENV=production \
  librevita:latest
```

Replace `podman` with `docker` when using Docker. Timezone data is embedded by Go through `time/tzdata`, and the CA
bundle is embedded through `rootcerts`. The image has no shell, package manager, or pre-created data directory. The
mounted `/data` volume must be writable by UID/GID `65532:65532`.

The Earthfile pins the toolchain images (`golang:1.26.5-alpine3.24`, `node:26.7-alpine3.24`) by digest, so the
BuildKit resolves them from its local content store and builds never query the registry — no Docker Hub rate limits,
and the exact images are fixed by the Earthfile. It sets `CGO_ENABLED=0` for every Go build, keeping the binaries
statically linkable across supported architectures.

Build caching is layered: each target consumes only the inputs it needs, so a change invalidates the minimal subtree:

- `+go-deps` — Go module downloads, rebuilt only when `go.mod`/`go.sum` change
- `+go-templ`/`+go-sqlc` — code generators installed from the bare Go toolchain, independent of the application modules
- `+node-deps`/`+node-check`/`+node-css`/`+node-bundle` — npm install plus the frontend stages: template changes
  re-run only the Tailwind JIT, TS changes only the type-check and the bundle
- `+schema` — consolidated DDL for sqlc, built from only the packages on the schemagen import chain
- `+go-generated` — sources plus generated code (templ, sqlc, compiled assets)

If a rootful Earthly BuildKit daemon already occupies ports `8371` and `8372`, configure the client to reuse it:

```sh
earthly config global.buildkit_host 'tcp://localhost.:8372'
```

The trailing dot prevents Earthly from treating the existing TLS endpoint as a local daemon that it should manage
itself.

## Configuration

Configuration is loaded by Koanf with this precedence, from lowest to highest:

1. Built-in defaults 2. `config.yaml`, `config.yml`, `config.json`, or the file passed with `--config` 3. `.env` and
   `LIBREVITA_*` environment variables 4. Command-line flags

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
storage:
  backend: local # local or s3
  local:
    dir: ./data/files # default: <data_dir>/files
  s3:
    endpoint: minio.example.org:9000
    bucket: librevita
    access_key: ...
    secret_key: ...
    region: "" # may be empty outside AWS
    secure: false # HTTPS for the S3 endpoint
    path_style: true # path-style addressing for S3-compatible APIs
```

The main flags are:

| Flag                           | Environment variable                   | Purpose                                                      |
| ------------------------------ | -------------------------------------- | ------------------------------------------------------------ |
| `--config`                     | `LIBREVITA_CONFIG_FILE`                | Configuration file path                                      |
| `--env`                        | `LIBREVITA_ENV`                        | Runtime environment                                          |
| `--http-addr`                  | `LIBREVITA_HTTP_ADDR`                  | HTTP bind address                                            |
| `--data-dir`                   | `LIBREVITA_DATA_DIR`                   | Base directory for default database and logs                 |
| `--db-driver`                  | `LIBREVITA_DB_DRIVER`                  | `sqlite` or `rqlite`                                         |
| `--db-path`                    | `LIBREVITA_DB_PATH`                    | SQLite file path                                             |
| `--rqlite-addr`                | `LIBREVITA_RQLITE_ADDR`                | rqlite node URL                                              |
| `--log-mode`                   | `LIBREVITA_LOG_MODE`                   | `console`, `file`, or `rotating`                             |
| `--log-path`                   | `LIBREVITA_LOG_PATH`                   | File destination                                             |
| `--log-max-size`               | `LIBREVITA_LOG_MAX_SIZE_MB`            | Rotating file size in MB                                     |
| `--log-max-backups`            | `LIBREVITA_LOG_MAX_BACKUPS`            | Number of rotated files                                      |
| `--log-max-age`                | `LIBREVITA_LOG_MAX_AGE_DAYS`           | Maximum rotated file age                                     |
| `--log-compress`               | `LIBREVITA_LOG_COMPRESS`               | Compress rotated files                                       |
| `--paseto-key`                 | `LIBREVITA_PASETO_KEY`                 | Session key (base64, 32 bytes; required outside development) |
| `--auth-max-concurrent-hashes` | `LIBREVITA_AUTH_MAX_CONCURRENT_HASHES` | Bound on concurrent Argon2id operations                      |
| `--storage-backend`            | `LIBREVITA_STORAGE_BACKEND`            | File storage backend: `local` or `s3`                        |
| `--storage-dir`                | `LIBREVITA_STORAGE_LOCAL_DIR`          | Local file storage directory (default `<data-dir>/files`)    |
| `--s3-endpoint`                | `LIBREVITA_STORAGE_S3_ENDPOINT`        | S3-compatible API endpoint                                   |
| `--s3-bucket`                  | `LIBREVITA_STORAGE_S3_BUCKET`          | S3 bucket for stored files                                   |
| `--s3-access-key`              | `LIBREVITA_STORAGE_S3_ACCESS_KEY`      | S3 access key                                                |
| `--s3-secret-key`              | `LIBREVITA_STORAGE_S3_SECRET_KEY`      | S3 secret key                                                |
| `--s3-region`                  | `LIBREVITA_STORAGE_S3_REGION`          | S3 region (may be empty outside AWS)                         |
| `--s3-secure`                  | `LIBREVITA_STORAGE_S3_SECURE`          | Use HTTPS for the S3 endpoint                                |
| `--s3-path-style`              | `LIBREVITA_STORAGE_S3_PATH_STYLE`      | Use path-style S3 addressing                                 |
| `--trusted-proxies`            | `LIBREVITA_TRUSTED_PROXIES`            | Comma-separated proxy IPs allowed to set `X-Forwarded-For`   |

`LIBREVITA_DATABASE_*` names are also accepted for database settings.

## File Storage

File storage lives in `internal/core/storage` behind the `Store` port (`Put`/`Get`/`Delete`/`Stat`/`List` over
key-addressed objects; keys such as `patients/<id>/prescription.pdf` are owned by the domain layer). Two backends
implement it, selected with `storage.backend`:

- **`local`** — a directory on the server (default `<data-dir>/files`). Writes are atomic (temp file + rename), each
  object has a sidecar metadata file (content type, ETag) under `.meta/`, and keys are validated so path traversal is
  impossible.
- **`s3`** — any S3-compatible API (MinIO, Garage, Ceph, ...), not necessarily AWS: endpoint, credentials, region, and
  path-style addressing are configurable. The bucket is verified at startup so a misconfigured backend fails fast.

The backend is wired through the Fx module and injected as the `Store` interface, so domains never depend on the
concrete implementation.

## Logging

The application uses `log/slog` with Zap and `zapslog`. Fx and Goose use the same logger.

Development output is human-readable text with one record per line. Records are emitted as columns, not JSON. The source
path is shortened to `file.go:line`, and lines are truncated to the terminal width. Set `COLUMNS` to override the
detected width; the fallback is 120 columns.

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

SQLite uses `modernc.org/sqlite`, so it does not require CGO. The connection factory enables WAL mode, a busy timeout,
foreign keys, and synchronous mode. The SQL pool is limited to one open connection because SQLite has a single writer.

Primary keys are UUIDv7 (`TEXT`), generated by the application through `github.com/google/uuid` and stored in canonical
lowercase form. UUIDv7 is temporally sortable, non-enumerable (a patient id never reveals "patient
#42"), and stable across independent databases, which matters for importing
or merging clinical records. IDs are generated in the application, never by SQLite defaults, so the code never depends
on `last_insert_rowid`; the `id` column is passed explicitly to inserts. Display identifiers such as an MRN remain
separate columns.

Go creates `data_dir` at startup. If database or log paths are not set explicitly, they are created as
`data_dir/librevita.db` and `data_dir/librevita.log`.

The default driver is embedded SQLite. Set `LIBREVITA_DB_DRIVER=rqlite` to use the `gorqlite` client for a distributed
deployment. Goose migrations run on the embedded SQLite backend; rqlite schema management remains a cluster-level
operation.

Tables are `STRICT` and every closed value set is enforced twice: by a `CHECK` constraint in SQLite and by a typed enum
in `internal/types` (`AuditResult`, `PatientStatus`, `Sex`, `StaffRequestStatus`, `PolicyOrigin`, `UITheme`). Timestamp
columns (`created_at`, `updated_at`, `expires_at`, `decided_at`) map to `types.DateTime` — an ISO-8601 UTC millis string
that parses the database `strftime` and legacy RFC3339Nano forms — and the generated repositories type `0/1` flags as
`bool` and UUID columns as `uuid.UUID`, so id mixing and magic-number comparisons are compile errors.

Migrations are organized by domain (`db/migrations/00001`..`00009`) and are the single source of truth for the
schema: `cmd/schemagen` applies them to an in-memory SQLite and exports the consolidated DDL that sqlc consumes
(`db/schema/schema.sql`). The schema file is a build artifact, not versioned; it is regenerated by the generation stages
after any migration change.

## Migrations

Migration files live in `db/migrations` and are embedded into the binary. The Fx database lifecycle applies pending
Goose migrations before the HTTP server starts. Goose logs use the same structured logger as the rest of the process.

Generated repository code and the consolidated schema are not source artifacts. Edit the migrations under
`db/migrations` and the queries under `db/query`, then run the generation stages when local generated files are needed.

## HTTP Server

Echo is created and managed by Fx. Routes:

- `GET /healthz` — liveness probe, returns `{"status":"ok"}`
- `GET /setup`, `POST /setup` — initial onboarding (admin account + clinic profile)
- `GET /auth/login`, `POST /auth/login`, `GET /auth/register`, `POST /auth/register`, `POST /auth/logout`
- `GET /`, `GET /activity/recent` — authenticated dashboard
- `GET /profile`, `POST /profile` — self-service preferences (UI theme, personal timezone); `GET /profile/avatar`,
  `POST /profile/avatar`, `POST /profile/avatar/remove` — profile picture; `GET /users/:id/avatar` — avatar of any
  user
- `GET /patients`, `GET /patients/new`, `POST /patients`, `GET /patients/:id`, `GET /patients/:id/edit`, `POST
/patients/:id`, `POST /patients/:id/archive`, `POST /patients/:id/restore`, `POST /patients/bulk-archive`, `POST
/patients/check-document`
- `GET /users`, `GET /users/new`, `POST /users`, `GET /users/:id/edit`, `POST /users/:id`, `POST /users/:id/status` —
  staff account management
- `GET /specialties`, `POST /specialties`, `POST /specialties/:id/delete` — clinic specialty catalog
- `GET /roles`, `POST /roles`, `POST /roles/:id/rename`, `POST /roles/:id/clinical`, `POST /roles/:id/delete` — dynamic
  role management
- `GET /policies`, `POST /policies`, `POST /policies/reset` — policy editor
- `GET /staff`, `GET /staff/new`, `POST /staff`, `GET /staff/:id/edit`, `POST /staff/:id`, `POST /staff/:id/request`,
  `GET /staff/my-requests`, `GET /staff/requests`, `POST /staff/requests/:id/approve`, `POST /staff/requests/:id/reject`
  — physician directory and the change-approval workflow
- `GET /audit/integrity` — verifies the append-only audit hash chain

HTTP errors use RFC 7807 `application/problem+json` responses.

## Domains

The clinical and administrative features are organized in `internal/domain`:

- **Patients** — full registry CRUD with whole-word search (debounced server-side), status (active/archived), bulk
  archive, duplicate document checks, and an audit-backed change history on the detail page. Editing is governed by the
  resource-level `patient.edit` policy: physicians edit only the patients they registered, admins edit everything.
- **Staff & specialties** — the clinic specialty catalog and the physician directory. Receptionists propose profile
  changes (name, email, specialties) that an administrator approves or rejects; the request snapshots the previous
  profile so the diff stays readable, and the whole flow (list, history, filters, pagination) is audited.
- **Users** — account management with relational roles: create staff accounts, change roles and status, and manage
  dynamic roles (rename, mark as clinical, delete when unused). The anti-lockout rules refuse to demote or deactivate
  the last active admin, enforced atomically in a single SQL statement.
- **Preferences** — every user stores their own UI theme (`system`/`light`/`dark`, mirrored by `types.UITheme`) and
  personal timezone (empty inherits the clinic timezone); the shell renders the theme server-side and the display clock
  resolves the user's zone with a clinic fallback.

## Onboarding

A fresh installation has no accounts. Any navigation while the system is not onboarded (login, dashboard, register) is
redirected to `GET /setup`, which creates the initial `admin` account and the clinic profile (name, tax id, contact, and
address) in a single atomic transaction. Setup runs exactly once: the transaction also persists a `setup_completed`
marker in the `meta` table, so the system stays onboarded even if every account and the clinic are later removed.
Onboarded systems redirect setup requests to the login page, and concurrent setup attempts never produce more than one
admin: exactly one wins, the rest receive the redirect. Setup is rate-limited to 5 attempts per minute per IP.

After onboarding, account creation is never public: `RequireAuth` plus the `users.register` policy guard the
registration routes. The default policy restricts registration to the `admin` role; an operator can tighten it to a
single user (`principal.email == 'hr@example.org'`) or close it entirely (`false`). The created accounts are `patient`
by default; role assignment is an admin responsibility.

## Authentication and Authorization

Authentication lives in `internal/core/auth` (transport-agnostic) with HTTP adapters in `internal/core/server`:

- Passwords are hashed with Argon2id (`golang.org/x/crypto/argon2`)
- Sessions are PASETO v4.local tokens (`aidanwoods.dev/go-paseto`): the payload is encrypted with XChaCha20-Poly1305
  under a single server key and validated cryptographically on every request. The `sessions` table holds only the token
  id (SHA-256) for revocation, logout, and account deactivation checks. The authenticated principal is loaded fresh on
  every request and carries the user's timezone and UI-theme preferences. The cookie is `HttpOnly` and `SameSite=Lax`,
  with the `Secure` flag enabled in production
- The session key is `LIBREVITA_PASETO_KEY` (base64, 32 bytes). Every environment except the explicit `development`
  requires the key and sets the `Secure` flag on cookies; only `development` falls back to an ephemeral key (sessions
  reset on restart). Deployments labeled `staging`, `prod`, or any other value are treated as persistent
- Concurrent Argon2id operations are bounded by `--auth-max-concurrent-hashes` (default 4, ~64 MiB each) to protect the
  process from memory exhaustion under abusive login traffic
- CSRF uses the double-submit cookie pattern. Forms embed the token in the `_csrf` field; HTMX and fetch requests send
  it in the `X-CSRF-Token` header
- Authorization is policy-based and lives in `internal/core/policy`. Roles are relational rows in the `roles` table: the
  four system roles (`admin`, `physician`, `receptionist`, `patient`) are seeded at onboarding, and an administrator can
  add custom roles or mark roles as clinical (they then join the physician directory and the staff change workflow).
  Permissions are CEL expressions compiled once at startup and evaluated per request. `RequireAuth` redirects anonymous
  browsers to the login page, and `RequirePolicy(name)` returns an RFC 7807 `403` when the policy denies. Resource-level
  policies receive a third variable, `resource`, and are enforced inside the use cases (see `patient.edit`).

CEL (`github.com/google/cel-go`) is a non-Turing-complete expression language: it has no loops, recursion, or side
effects, so authorization rules are bounded, safe to evaluate, and auditable. Policies receive two variables:

- `principal` — `id`, `email`, `name`, `role`
- `request` — `method`, `path`
- `resource` — only for resource-level policies (`patient.edit`), with the record attributes (`id`, `created_by`,
  `status`)

Default policies are seeded into the `policies` table on startup. The stored expression always wins, and the policy
editor (`/policies`) edits them at runtime: every change is validated before activation (the expression must compile and
evaluate to a boolean; a broken policy is rejected and the previous one stays active), takes effect immediately, and is
written to the audit trail. Each change is also versioned in `policy_versions` with the acting user and timestamp; the
editor shows the latest versions per policy. Renaming a role that a policy references by name is rejected, because the
policies would silently change meaning.

Critical policies (`admin.view`) are protected against self-lockout: a change that would deny the admin role is
rejected, because the policy editor is the only place that could restore it.

| Policy                   | Expression                                                                                              |
| ------------------------ | ------------------------------------------------------------------------------------------------------- |
| `dashboard.view`         | `principal.role in ['admin', 'physician', 'receptionist', 'patient']`                                   |
| `profile.update`         | `principal.role in ['admin', 'physician', 'receptionist', 'patient']`                                   |
| `admin.view`             | `principal.role == 'admin'`                                                                             |
| `users.register`         | `principal.role == 'admin'`                                                                             |
| `users.manage`           | `principal.role == 'admin'`                                                                             |
| `staff.view`             | `principal.role in ['admin', 'physician', 'receptionist']`                                              |
| `staff.edit`             | `principal.role == 'admin'`                                                                             |
| `staff.request`          | `principal.role in ['admin', 'receptionist']`                                                           |
| `staff.approve`          | `principal.role == 'admin'`                                                                             |
| `patient.view`           | `principal.role in ['admin', 'physician', 'receptionist']`                                              |
| `patient.edit`           | `principal.role == 'admin' \|\| (principal.role == 'physician' && resource.created_by == principal.id)` |
| `patient.document.read`  | `principal.role in ['admin', 'physician', 'receptionist']`                                              |
| `patient.document.write` | `principal.role in ['admin', 'physician']`                                                              |

Abuse controls:

- Login is rate-limited to 10 attempts per minute per IP (`429` beyond that)
- The request body is limited to 1 MiB, and input fields have explicit length limits
- Login runs an Argon2id verification even for unknown or deactivated accounts, so response timing does not reveal
  whether an email exists
- The HTTP server enforces read timeouts

All security-relevant and clinical events (register, login, logout, policy denials, patient create/update/archive, staff
approvals, preference changes) are written to the `audit_log` table by `internal/core/audit`. The trail records actor,
action, resource, result, IP, request id, and a detail message; passwords, tokens, and CSRF values are never stored.
Each row carries a BLAKE2b signature chained to the previous entry, so modifying or reordering any entry breaks the
chain for every following row; database triggers make the table append-only. `GET /audit/integrity` recomputes the chain
and reports the first broken entry. Rows are self-contained snapshots (Event Sourcing): the actor name, role, user
agent, and resource name are denormalized onto every event. Recording is best-effort and never breaks the audited
operation; the per-resource history powers the patient detail page.

Sessions require the SQLite backend; rqlite deployments must provide their own authentication layer.
