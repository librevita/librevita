# Contributing to LibreVita

LibreVita is free software (AGPL-3.0). All contributions are welcome: bug
fixes, features, documentation, and tests.

## Prerequisites

- [Task](https://taskfile.dev/install) (3.x) — the build and development
  interface
- Go — the version pinned in the Taskfile (`GO_VERSION`) is downloaded
  automatically by `go` when the local one is older
- Node 24 (LTS, see `.nvmrc`) for the frontend pipeline
- Podman or Docker, only if you build the OCI image (`task image`)

## First-time setup

```sh
task gen        # writes generated sources (templ, sqlc, assets) for editors
task all        # runs tests, vet, lint, builds the binary and the image
```

`task all` is the full validation gate; the CI runs the same tasks split
across jobs.

## Development loop

```sh
task dev        # fast unoptimized binary (bin/librevita-dev)
task test       # Go test suite + frontend unit tests
task vet        # go vet
task lint       # golangci-lint (config: .golangci.yaml)
task audit      # govulncheck (source + binary) and npm audit
task tidy       # sync go.mod/go.sum
task gen-schema # regenerate db/schema/schema.sql from migrations
```

Cache discipline: the Go build cache, the npm cache and the Taskfile
`sources`/`generates` gates keep the loop incremental — a task only
re-runs when its inputs changed. Generators and analyzers are pinned by
version in the Taskfile `vars`; when bumping a toolchain, update the
version there (and the same `GO_VERSION` in the CI workflow).

## Style and conventions

- Code, logs, comments, and commit messages are in English
- Conventional commit messages (`fix:`, `feat:`, `refactor:`, `docs:`, ...)
- `gofmt` and `golangci-lint` must pass (`task lint`)
- Go code must build with `CGO_ENABLED=0` (the production image is scratch)
- The compatibility floor is legacy browsers; frontend changes must keep
  `npm run check` and `npm test` green
- Tests: domain use cases are unit-tested; handlers have HTTP tests; the audit
  chain, storage saga, and reconciler have dedicated suites

## Logging

Application code logs through `librevita.org/pkg/log` with typed Fields
(`log.String`, `log.Error`, ...). Do not import `log/slog` or `go.uber.org/zap`
outside `pkg/log` and `internal/core/telemetry`.

- Name logger parameters `logger`, not `log`, so Field constructors (`log.String`, `log.Error`) stay in scope
- Use the `*Context` methods when a `context.Context` is available so `request_id` is attached
- Log swallowed errors at the call site. HTTP 500s that return `err` to Echo are logged by `ProblemErrorHandler`; do not double-log
- Do not log 4xx as errors; RequestLog already records them at Warn
- Never log PHI (identifier plaintext, SOAP notes, decrypted patient fields)
- Log messages are English

`loggercheck` covers leftover `log/slog` and Zap sugared kv. It cannot check `pkg/log`: its custom rules treat extra arguments as key/value pairs and would reject typed Fields. The compiler is the check (`...Field`). Do not add `pkg/log` to `loggercheck.rules`.

## Schema changes

Migrations are the single source of truth. Edit `db/migrations/`, then:

```sh
task gen   # regenerates db/schema/schema.sql and the sqlc repositories
```

The consolidated schema is derived, not versioned.

## Submitting a pull request

1. Fork the repository and create a branch from `master`
2. Make your change; run `task all` locally before pushing
3. Open the PR with the template filled in
4. The CI runs the same gate on the PR; from forks it runs without the
   shared caches (read-only token), so expect a slower first run

Maintainers: enable branch protection on `master` with the CI check required.

## Reporting issues

Include the LibreVita version or commit, the environment (OS, Go and Node
versions), and the full error output.

## Licensing & Community Pledge

LibreVita is free and open-source software licensed under the **GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later)**.

By contributing to LibreVita:
1. You agree that your contributions are licensed under the **AGPL-3.0-or-later**.
2. We commit that your contributions will remain permanently free software for the global commons, and will never be relicensed under closed-source or restrictive commercial licenses.
3. We do not require proprietary Contributor License Agreements (CLAs). We follow the standard Developer Certificate of Origin (DCO / Inbound=Outbound) principle.

