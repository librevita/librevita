# LibreVita

[![CI](https://img.shields.io/github/actions/workflow/status/librevita/librevita/ci.yaml?style=flat-square&logo=github)](https://github.com/librevita/librevita/actions/workflows/ci.yaml)
[![Go version](https://img.shields.io/github/go-mod/go-version/librevita/librevita?style=flat-square)](https://go.dev/dl/)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square)](LICENSE)
[![Repo size](https://img.shields.io/github/repo-size/librevita/librevita?style=flat-square)]()

Self-hosted medical clinic management software built in Go. The module path is `librevita.org`.

## Requirements

- [Task](https://taskfile.dev/install) 3.x — the build and development interface
- Go (the `go.mod` floor; `go` auto-downloads the pinned `GO_VERSION` toolchain
  from the Taskfile when needed)
- Node 24 LTS (see `.nvmrc`) for the frontend pipeline
- Podman or Docker for the image task only (`task image`); nothing else needs
  containers

## Task Targets

```sh
task gen                    # regenerate schema, templ views and sqlc repositories
task dev                    # fast unoptimized binary (bin/librevita-dev)
task build                  # optimized production binary (bin/librevita)
task image                  # OCI image (podman by default, task image -- IMG=docker)
task test                   # Go test suite + frontend unit tests
task vet                    # go vet
task lint                   # golangci-lint
task audit                  # govulncheck (source + binary) and npm audit
task tidy                   # sync go.mod/go.sum
task cross -- os=linux arch=riscv64
task cross -- os=linux arch=loong64
task cross -- os=linux arch=mips64
```

`task build` writes the optimized production binary to `bin/librevita`; `task dev` writes the fast, unoptimized
`bin/librevita-dev`. Cross builds write files such as `bin/librevita-linux-riscv64`. Every Go command runs on the
pinned `GO_VERSION` toolchain (Taskfile `vars`) with `CGO_ENABLED=0`, so the binaries are static.

SQLC and templ output is not committed. The generation task (`task gen`) writes them to the workspace for editor
support; build, test, and vet tasks generate them as dependencies. Incremental behaviour comes from the Go build
cache, the npm cache and the Taskfile `sources`/`generates` gates: a task only re-runs when its inputs changed.

## Frontend

The UI follows the GOTH stack: Go + templ + HTMX, server-driven and progressive. Assets are embedded in the binary
(`internal/ui`, served under `/static`) — there is no CDN, npm, or Node at runtime:

- **HTMX 1.9.12** for hypermedia interactions (`allowEval=false`, `allowScriptTags=false`), with the bundled SSE
  extension (`dist/ext/sse.js`) configured for live updates
  (`hx-ext="sse"` + `sse-connect`/`sse-swap`), pending the calendar feature — both bundled into the single application script
- **First-party TypeScript** modules (bundled by esbuild) for the ephemeral client state (menus, tabs, theme); clinical
  state always lives on the server. The strict Content-Security-Policy (`script-src 'self'`, no
  `unsafe-eval`/`unsafe-inline`) is never relaxed
- **Tailwind CSS 3.4.17** compiled at build time with a hex palette override (the v3.4 default `oklch` colors are
  unparseable by legacy browsers)
- The frontend build is driven by **Node** (24 LTS, see `.nvmrc`; the same version
  is pinned in the Taskfile): `package.json`
  declares the dependencies and scripts, and `package-lock.json` pins them with integrity hashes — nothing is versioned
  inside the Taskfile. esbuild bundles the TypeScript source to the legacy floor (`target=firefox58`, verified by
  `assertLegacyFloor` after the build) and the PostCSS pipeline compiles Tailwind from `internal/ui/input.css`; no one
  writes ES5. The output is a single stylesheet and a single script with content-addressed names
  (`app-<hash>.css`, `app-<hash>.js`) carrying the HTMX runtime, its SSE extension and the application code,
  loaded with `defer`; the templates render them with Subresource Integrity hashes, so the assets can be cached
  immutably. The theme bootstrap is a small inline head script (allowed by its CSP content hash), so the dark
  class exists before first paint while the bundle loads deferred
- **TypeScript** in `internal/ui/ts` with strict checking (`tsc --noEmit`, `lib: ES2017+DOM` aligned to the legacy floor, so
  APIs missing from Firefox 52 are compile errors)

The runtime assets (HTMX and its SSE extension) come from npm packages pinned in `package-lock.json`, bundled into the
single application script, and are documented in `VENDOR.md`. The compatibility floor is legacy browsers (Pale
Moon 28.8, Basilisk 55, K-Meleon 74G/76, Firefox 52 ESR); newer engines are covered automatically — see
[Supported Browsers](#supported-browsers). The server applies a
strict CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`,
`Cross-Origin-Resource-Policy: same-origin`, `Permissions-Policy` (camera/microphone/geolocation denied), and
`Cache-Control: no-store` on all non-static responses; `Strict-Transport-Security` is opt-in via `--hsts-max-age` for
HTTPS deployments. The application is same-origin and no CORS is configured.

### Supported Browsers

The reference hardware is a PowerPC Mac (iBook G4, 1024×768), and the UI must remain fast and fully functional on
20+ year old machines — reception desks with legacy PCs or cheap SBCs are first-class targets, not an afterthought.
Layout uses flexbox only (CSS Grid is Firefox 52+, `gap` in flex is Firefox 63+), fonts are self-hosted Inter
(woff2+woff), and the PostCSS `legacyFallbacks` pipeline rewrites every modern syntax the floor cannot parse
(`:where()`, `:is(.dark *)`, `:host`, space-separated `rgb()`, 4-digit hex, `calc()` multiplication in the
`space-x/y` utilities) into equivalent classic CSS, verified in the build. The single JS bundle is compiled by
esbuild with the legacy floor asserted after the build (`assertLegacyFloor` rejects `?.`, `??`, lookbehind, named groups and
unicode property escapes), so a tool or dependency upgrade can never silently break the floor.

| Browser                                                                | Status                                                                                       |
| ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| AquaFox / TenFourFox 45 (Firefox 45-era, PPC Mac)                      | **Primary floor** — the reference browser on the iBook; fully supported                      |
| Firefox 52 ESR / Goanna (Pale Moon 28.8, Basilisk 55, K-Meleon 74G/76) | Fully supported — the JS floor (`esbuild` `firefox58` target + assertions)                   |
| Firefox ESR / Chrome / Edge / modern Safari                            | Fully supported — all features, automatic                                                    |
| NetSurf 3.x                                                            | Not supported — no `XMLHttpRequest`/`DOMParser`/`MutationObserver` bindings; htmx cannot run |

The floor is defined twice and enforced independently: CSS syntax in the `legacyFallbacks` PostCSS plugin
(`internal/ui/build.ts`) and JavaScript syntax in the esbuild target plus `assertLegacyFloor`, which also runs in CI.
Browserslist (`package.json`) drives autoprefixer only, aligned to `Firefox >= 45`.

## Container Image

`task image` packages the production binary in a `scratch` OCI image. The image runs as non-root UID/GID
`65532:65532` and sets `LIBREVITA_DATA_DIR=/data/librevita`. Go creates that directory and its files at startup.

Podman and Docker use the same commands:

```sh
task image                       # podman by default
task image -- IMG=docker         # or docker
podman run --rm \
  -p 8080:8080 \
  -v librevita-data:/data \
  -e LIBREVITA_MODE=production \
  librevita:latest
```

Replace `podman` with `docker` when using Docker. Timezone data is embedded by Go through `time/tzdata`, and the CA
bundle is embedded through `rootcerts`. The image has no shell, package manager, or pre-created data directory. The
mounted `/data` volume must be writable by UID/GID `65532:65532`.

The Taskfile pins the toolchains: every Go command runs on `GO_VERSION` (`GOTOOLCHAIN=go1.26.6`, auto-downloaded
when the local Go is older) and the frontend on Node 24 LTS. It sets `CGO_ENABLED=0` for every Go build, keeping the
binaries statically linkable across supported architectures.

Build caching is incremental: the Go build cache, the npm cache and the Taskfile gates mean a change re-runs only the
affected tasks:

- `task gen` — schema (from the migrations), templ views and sqlc repositories, regenerated only when their inputs change
- `task frontend` — npm `ci`, type-check, Tailwind CSS and the esbuild bundle, each gated on its own inputs
- `task tools` — the pinned generators and analyzers (templ, sqlc, golangci-lint, govulncheck) installed into
  `.tools/bin` from the bare Go toolchain, independent of the application modules
- `task build`/`task test`/`task vet`/`task lint`/`task audit` — the Go gate, on the pinned toolchain

## Configuration

Configuration is loaded by Koanf with this precedence, from lowest to highest:

1. Built-in defaults
2. `config.yaml`, `config.yml`, `config.json`, or the file passed with `--config`
3. `.env` and `LIBREVITA_*` environment variables
4. Command-line flags

Example `config.yaml`:

```yaml
mode: production
trusted_proxies: 10.0.0.0/8 # proxies allowed to set X-Forwarded-For
http_bind: "0.0.0.0"
http_port: 8080
data_dir: ./data
database:
  driver: sqlite # sqlite or dqlite
  sqlite:
    path: ./librevita.db
  # For driver: dqlite, give the node candidates as static addresses
  # and/or a discovery SRV record whose targets seed the cluster.
  dqlite:
    addrs: node1:9001,node2:9001,node3:9001
    discovery_srv: _dqlite._tcp.librevita.svc.cluster.local # optional
    database: librevita
auth:
  max_concurrent_hashes: 4
paseto_key: ... # base64, 32 bytes; required outside development
master_key: ... # base64, 32 bytes; required outside development
logging:
  mode: console # console, file or rotating
  file: # used when mode: file
    path: ./librevita.log
  rotating: # used when mode: rotating
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

All configuration flags are:

| Flag                           | Environment variable                      | Purpose                                                                                                                                                      |
| ------------------------------ | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--config`                     | `LIBREVITA_CONFIG`                        | Configuration file path                                                                                                                                      |
| `--mode`                       | `LIBREVITA_MODE`                          | Runtime mode: `development` or `production`                                                                                                                  |
| `--http-bind`                  | `LIBREVITA_HTTP_BIND`                     | HTTP bind address (`0.0.0.0`, `127.0.0.1`, ...)                                                                                                              |
| `--http-port`                  | `LIBREVITA_HTTP_PORT`                     | HTTP listen port (default `8080`)                                                                                                                            |
| `--trusted-proxies`            | `LIBREVITA_TRUSTED_PROXIES`               | Comma-separated proxy IPs allowed to set `X-Forwarded-For`                                                                                                   |
| `--hsts-max-age`               | `LIBREVITA_HSTS_MAX_AGE`                  | `Strict-Transport-Security` max-age in seconds (0 disables; HTTPS only)                                                                                      |
| `--data-dir`                   | `LIBREVITA_DATA_DIR`                      | Base directory for default database and logs                                                                                                                 |
| `--db-driver`                  | `LIBREVITA_DATABASE_DRIVER`               | Database backend: `sqlite` or `dqlite`                                                                                                                       |
| `--db-sqlite-path`             | `LIBREVITA_DATABASE_SQLITE_PATH`          | SQLite file path                                                                                                                                             |
| `--db-dqlite-addrs`            | `LIBREVITA_DATABASE_DQLITE_ADDRS`         | Comma-separated dqlite node addresses (wire protocol)                                                                                                        |
| `--db-dqlite-discovery-srv`    | `LIBREVITA_DATABASE_DQLITE_DISCOVERY_SRV` | DNS SRV record seeding the dqlite node candidates (e.g. `_dqlite._tcp.librevita.svc.cluster.local`); at least one of this or `--db-dqlite-addrs` is required |
| `--db-dqlite-database`         | `LIBREVITA_DATABASE_DQLITE_DATABASE`      | dqlite database name (default `librevita`)                                                                                                                   |
| `--log-mode`                   | `LIBREVITA_LOGGING_MODE`                  | `console`, `file`, or `rotating`                                                                                                                             |
| `--log-file-path`              | `LIBREVITA_LOGGING_FILE_PATH`             | File destination (file mode)                                                                                                                                 |
| `--log-rotating-path`          | `LIBREVITA_LOGGING_ROTATING_PATH`         | Rotating log file destination                                                                                                                                |
| `--log-rotating-max-size`      | `LIBREVITA_LOGGING_ROTATING_MAX_SIZE_MB`  | Rotating file size in MB                                                                                                                                     |
| `--log-rotating-max-backups`   | `LIBREVITA_LOGGING_ROTATING_MAX_BACKUPS`  | Number of rotated files                                                                                                                                      |
| `--log-rotating-max-age`       | `LIBREVITA_LOGGING_ROTATING_MAX_AGE_DAYS` | Maximum rotated file age                                                                                                                                     |
| `--log-rotating-compress`      | `LIBREVITA_LOGGING_ROTATING_COMPRESS`     | Compress rotated files                                                                                                                                       |
| `--paseto-key`                 | `LIBREVITA_PASETO_KEY`                    | Session key (base64, 32 bytes; required outside development)                                                                                                 |
| `--master-key`                 | `LIBREVITA_MASTER_KEY`                    | Field-encryption key (base64, 32 bytes; required outside development)                                                                                        |
| `--auth-max-concurrent-hashes` | `LIBREVITA_AUTH_MAX_CONCURRENT_HASHES`    | Bound on concurrent Argon2id operations                                                                                                                      |
| `--storage-backend`            | `LIBREVITA_STORAGE_BACKEND`               | File storage backend: `local` or `s3`                                                                                                                        |
| `--storage-local-dir`          | `LIBREVITA_STORAGE_LOCAL_DIR`             | Local file storage directory (default `<data-dir>/files`)                                                                                                    |
| `--storage-s3-endpoint`        | `LIBREVITA_STORAGE_S3_ENDPOINT`           | S3-compatible API endpoint                                                                                                                                   |
| `--storage-s3-bucket`          | `LIBREVITA_STORAGE_S3_BUCKET`             | S3 bucket for stored files                                                                                                                                   |
| `--storage-s3-access-key`      | `LIBREVITA_STORAGE_S3_ACCESS_KEY`         | S3 access key                                                                                                                                                |
| `--storage-s3-secret-key`      | `LIBREVITA_STORAGE_S3_SECRET_KEY`         | S3 secret key                                                                                                                                                |
| `--storage-s3-region`          | `LIBREVITA_STORAGE_S3_REGION`             | S3 region (may be empty outside AWS)                                                                                                                         |
| `--storage-s3-secure`          | `LIBREVITA_STORAGE_S3_SECURE`             | Use HTTPS for the S3 endpoint                                                                                                                                |
| `--storage-s3-path-style`      | `LIBREVITA_STORAGE_S3_PATH_STYLE`         | Use path-style S3 addressing                                                                                                                                 |

Environment variables are the config keys with `_` separators, always in the full section form (`LIBREVITA_DATABASE_*`, `LIBREVITA_LOGGING_*`, `LIBREVITA_STORAGE_*`); no short aliases are accepted.

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

Production output is always JSON. The `logging.mode` switch selects the destination, and each mode has its own
subsection (`logging.file.*`, `logging.rotating.*`), mirroring the storage layout:

- `console` writes JSON to stderr (no knobs — the `console` section exists for layout symmetry)
- `file` appends JSON to `logging.file.path`
- `rotating` uses `lumberjack` with `logging.rotating.path` plus the size, backup count, age, and compression settings

Example production commands:

```sh
LIBREVITA_MODE=production \
LIBREVITA_LOGGING_MODE=file \
LIBREVITA_LOGGING_FILE_PATH=./librevita.log \
./bin/librevita

LIBREVITA_MODE=production \
LIBREVITA_LOGGING_MODE=rotating \
LIBREVITA_LOGGING_ROTATING_PATH=./librevita.log \
LIBREVITA_LOGGING_ROTATING_MAX_SIZE_MB=100 \
LIBREVITA_LOGGING_ROTATING_MAX_BACKUPS=3 \
LIBREVITA_LOGGING_ROTATING_MAX_AGE_DAYS=28 \
LIBREVITA_LOGGING_ROTATING_COMPRESS=true \
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

The `database` section mirrors the `storage` layout: a `driver` switch plus a per-backend `sqlite`/`dqlite` subsection.
The default driver is embedded SQLite (`database.sqlite.path`). Set `LIBREVITA_DATABASE_DRIVER=dqlite` to use the pure-Go wire
protocol client
(`github.com/canonical/go-dqlite/v3`) against a dqlite cluster: real transactions (BEGIN/COMMIT replicated through
Raft), prepared statements, and strong consistency, with the same embedded Goose migrations. The cluster itself is
operated as a separate server process (a dqlite node binary built with CGO, outside the CGO-disabled application
build); the integration test behind the `dqlite` build tag (`go test -tags dqlite ./internal/core/database/`) skips
when no cluster is reachable.

Node candidates come from `database.dqlite.addrs` (static) and/or `database.dqlite.discovery_srv` (a DNS SRV record,
resolved live on every attempt); at least one is required. The driver only needs the candidates to find the cluster
leader — once connected, the cluster syncs the full membership — so SRV discovery just bootstraps the list and tracks
membership changes without restarts; static addresses remain the fallback when a record is empty or the lookup fails.

Tables are `STRICT` and every closed value set is enforced twice: by a `CHECK` constraint in SQLite and by a typed enum
in `internal/types` (`AuditResult`, `PatientStatus`, `Sex`, `StaffRequestStatus`, `PolicyOrigin`, `UITheme`). Timestamp
columns (`created_at`, `updated_at`, `expires_at`, `decided_at`) map to `types.DateTime` — an ISO-8601 UTC millis string
that parses the database `strftime` and legacy RFC3339Nano forms — and the generated repositories type `0/1` flags as
`bool` and UUID columns as `uuid.UUID`, so id mixing and magic-number comparisons are compile errors.

Migrations are organized by domain (`db/migrations/00001`..`00012`) and are the single source of truth for the
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

| Method | Route                                            | Purpose                                                          |
| ------ | ------------------------------------------------ | ---------------------------------------------------------------- |
| GET    | `/healthz`                                       | Liveness probe                                                   |
| GET    | `/setup`                                         | Onboarding page                                                  |
| POST   | `/setup`                                         | Onboarding: admin account + clinic profile (rate-limited)        |
| GET    | `/auth/login`                                    | Login page                                                       |
| POST   | `/auth/login`                                    | Authenticate (rate-limited)                                      |
| GET    | `/auth/register`                                 | Registration page                                                |
| POST   | `/auth/register`                                 | Create account (rate-limited, `users.register`)                  |
| POST   | `/auth/logout`                                   | End session                                                      |
| GET    | `/`                                              | Dashboard                                                        |
| GET    | `/activity/recent`                               | Recent activity (dashboard panel)                                |
| GET    | `/profile`                                       | Preferences page (UI theme, personal timezone)                   |
| POST   | `/profile`                                       | Save preferences                                                 |
| GET    | `/profile/avatar`                                | Profile picture                                                  |
| POST   | `/profile/avatar`                                | Upload profile picture (2 MiB limit)                             |
| POST   | `/profile/avatar/remove`                         | Remove profile picture                                           |
| GET    | `/users/:id/avatar`                              | Avatar of any user                                               |
| GET    | `/patients/lookup`                               | Exact identification-document lookup (blind index, rate-limited) |
| GET    | `/patients`                                      | Patient registry (search, filter, pager)                         |
| GET    | `/patients/new`                                  | Registration form                                                |
| POST   | `/patients`                                      | Register a patient (optionally with an identification document)  |
| GET    | `/patients/:id`                                  | Patient detail (documents, identifiers, history)                 |
| GET    | `/patients/:id/edit`                             | Edit form                                                        |
| POST   | `/patients/:id`                                  | Save edits (optionally adds an identification document)          |
| POST   | `/patients/:id/archive`                          | Archive a patient                                                |
| POST   | `/patients/:id/restore`                          | Restore an archived patient                                      |
| POST   | `/patients/bulk-archive`                         | Archive selected patients (up to 50)                             |
| POST   | `/patients/:id/identifiers`                      | Add an encrypted identification document                         |
| POST   | `/patients/:id/identifiers/:identifierID/remove` | Remove an identification document                                |
| GET    | `/identifier-systems`                            | Administrator catalog of document systems                        |
| POST   | `/identifier-systems`                            | Create a document system                                         |
| POST   | `/identifier-systems/:id`                        | Update a document system (URN immutable)                         |
| POST   | `/identifier-systems/:id/active`                 | Activate/deactivate a document system                            |
| GET    | `/identifier-systems/check-fields`               | Conditional check-digit fields of the system form                |
| POST   | `/patients/:id/documents`                        | Upload a clinical attachment (25 MiB limit)                      |
| GET    | `/patients/:id/documents/:fileID`                | Download a clinical attachment (audited)                         |
| GET    | `/users`                                         | Staff account list                                               |
| GET    | `/users/new`                                     | Account creation form                                            |
| POST   | `/users`                                         | Create an account                                                |
| GET    | `/users/:id/edit`                                | Account edit form                                                |
| POST   | `/users/:id`                                     | Update an account (role, name, email, status)                    |
| POST   | `/users/:id/status`                              | Activate/deactivate an account                                   |
| GET    | `/specialties`                                   | Clinic specialty catalog                                         |
| POST   | `/specialties`                                   | Create a specialty                                               |
| POST   | `/specialties/:id/delete`                        | Delete a specialty                                               |
| GET    | `/roles`                                         | Role catalog                                                     |
| POST   | `/roles`                                         | Create a role                                                    |
| POST   | `/roles/:id/rename`                              | Rename a role                                                    |
| POST   | `/roles/:id/clinical`                            | Toggle the clinical flag of a role                               |
| POST   | `/roles/:id/delete`                              | Delete a role                                                    |
| GET    | `/policies`                                      | Access policy editor                                             |
| POST   | `/policies`                                      | Save a policy expression                                         |
| POST   | `/policies/reset`                                | Restore the default policies                                     |
| GET    | `/staff`                                         | Physician directory                                              |
| GET    | `/staff/new`                                     | Physician creation form                                          |
| POST   | `/staff`                                         | Create a physician                                               |
| GET    | `/staff/:id/edit`                                | Physician edit form                                              |
| POST   | `/staff/:id`                                     | Admin direct edit of a physician                                 |
| POST   | `/staff/:id/request`                             | Receptionist proposes a physician change                         |
| GET    | `/staff/my-requests`                             | The user's own change requests                                   |
| GET    | `/staff/requests`                                | Pending change requests (admin)                                  |
| POST   | `/staff/requests/:id/approve`                    | Approve a change request                                         |
| POST   | `/staff/requests/:id/reject`                     | Reject a change request with a note                              |
| GET    | `/audit/integrity`                               | Verify the append-only audit hash chain                          |

HTTP errors use RFC 7807 `application/problem+json` responses.

## Domains

The clinical and administrative features are organized in `internal/domain`:

- **Patients** — full registry CRUD with whole-word search (debounced server-side), status (active/archived), bulk
  archive, an audit-backed change history on the detail page, clinical attachments (uploads are checksummed into the
  audit chain, downloads are audited), and FHIR-style identification documents (system + value) stored encrypted at
  field level with a keyed blind index for exact lookup; duplicates are rejected deployment-wide. The document
  systems themselves are administered at runtime (pattern, transform, check digit), so a deployment registers its
  jurisdictions' documents without a code change. Editing is governed by the
  resource-level `patient.edit` policy: physicians edit only the patients they registered, admins edit everything.
- **Clinic** — the installation profile (name, tax id, contact, timezone), created once by onboarding and resolved
  per request through the clock provider. The tenant model is single-clinic per installation (ADR-0001,
  `docs/adr/0001-single-clinic-tenant.md`): `clinic_id` on clinical tables is future-proofing, with the scope
  convention enforced by a test guard over the sqlc queries.
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

**Threat model of the chain:** the trail is _tamper-evidence_, not tamper-proof. The signature is unkeyed BLAKE2b over
the previous signature and the row payload, so anyone with write access to the database (or the underlying files) can
recompute the whole chain — the guarantee is that such an alteration is _detectable_ by running `GET /audit/integrity`,
not that it is impossible. Detection depends on someone actually verifying; deployments that need stronger guarantees
should schedule periodic chain verification (a cron hitting the endpoint, or a replica) and eventually anchor the chain
head in an external append-only store (e.g. a public transparency log or a second cluster).

Sessions require a database/sql backend; the dqlite driver qualifies, so session revocation works on both backends.
