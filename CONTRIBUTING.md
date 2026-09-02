# Contributing to LibreVita

LibreVita is free software (AGPL-3.0). All contributions are welcome: bug
fixes, features, documentation, and tests.

## Prerequisites

- [Task](https://taskfile.dev/install) (3.x) — the build and development
  interface
- Go — the version pinned in the Taskfile (`GO_VERSION`) is downloaded
  automatically by `go` when the local one is older
- Node 24 (LTS, see `.nvmrc`) for the frontend pipeline
- curl, only for `task hadolint` / `task zizmor` (pinned GitHub release binaries)
- Podman or Docker, only if you build the OCI image (`task image`)

## First-time setup

```sh
task gen        # Ent models, ident codecs, mocks, and templ views (needed for the editor)
task all        # tests, vet, lint, audits, production binary, and OCI image
```

`task all` is the full validation gate; the CI runs the same tasks split
across jobs.

## Development loop

```sh
task dev        # fast unoptimized binary (bin/librevita-dev)
task test       # Go test suite + frontend unit tests
task vet        # go vet
task lint       # golangci-lint, hadolint, actionlint, and zizmor
task audit      # govulncheck, npm audit, OSV-Scanner, and gitleaks
task tidy       # sync go.mod/go.sum
task db-diff -- name=describe_the_change  # Goose migrations from the Ent schema
```

Cache discipline: the Go build cache, the npm cache and the Taskfile
`sources`/`generates` gates keep the loop incremental — a task only
re-runs when its inputs changed. Generators and analyzers are pinned by
version in the Taskfile `vars`; when bumping a toolchain, update the
version there (and the same `GO_VERSION` in the CI workflow). Bumping
hadolint or zizmor also requires updating the matching SHA-256 pins for
each release binary or archive.

## Style and conventions

- Code, logs, comments, commit messages, and UI chrome are in English
- Validator catalogs may include `pt-BR` for field errors; that is not a translated UI
- Conventional commit messages (`fix:`, `feat:`, `refactor:`, `docs:`, ...)
- `gofmt`, `golangci-lint`, `hadolint`, `actionlint`, and `zizmor` must pass (`task lint`)
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

The Ent schema in `internal/database/schema` is the model. Versioned SQL
is Goose under `internal/database/migrations/{sqlite,postgres}` (embedded
and applied at process start). To add a change:

1. Edit the Ent schema
2. Generate migrations: `task db-diff -- name=add_patient_model`
3. Run `task gen` so the Ent client, ident codecs, and templ views match

Do not hand-edit generated `ent/` output. There is no sqlc path.

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

