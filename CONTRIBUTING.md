# Contributing to LibreVita

LibreVita is free software (AGPL-3.0). All contributions are welcome: bug
fixes, features, documentation, and tests.

## Prerequisites

- [Earthly](https://earthly.dev/get-earthly) (0.8) — the build and development
  interface (there is no Makefile)
- Docker or Podman for the Earthly BuildKit daemon
- Go and Node are only needed for editor tooling: Earthly builds inside
  containers with pinned toolchains

## First-time setup

```sh
earthly +generate   # writes generated sources (templ, sqlc, assets) for editors
earthly +all        # runs tests, vet, lint, builds the binary and the image
```

`+all` is the full validation gate; the CI runs exactly the same targets.

## Development loop

```sh
earthly +build --dev=true   # fast unoptimized binary (bin/librevita-dev)
earthly +test               # Go test suite
earthly +test-js            # frontend unit tests (node --test)
earthly +vet                # go vet
earthly +lint               # golangci-lint (config: .golangci.yaml)
earthly +tidy               # sync go.mod/go.sum
earthly +schema             # regenerate db/schema/schema.sql from migrations
```

Cache discipline: the build is layered, so local edits invalidate only the
layers they touch. Images and generators are pinned by digest in the Earthfile;
when bumping a toolchain, resolve the new digest first (see the comment above
the pinned versions).

## Style and conventions

- Code, logs, comments, and commit messages are in English
- Conventional commit messages (`fix:`, `feat:`, `refactor:`, `docs:`, ...)
- `gofmt` and `golangci-lint` must pass (`earthly +lint`)
- Go code must build with `CGO_ENABLED=0` (the production image is scratch)
- The compatibility floor is XP-era browsers; frontend changes must keep
  `npm run check` and `npm test` green
- Tests: domain use cases are unit-tested; handlers have HTTP tests; the audit
  chain, storage saga, and reconciler have dedicated suites

## Schema changes

Migrations are the single source of truth. Edit `db/migrations/`, then:

```sh
earthly +generate   # regenerates db/schema/schema.sql and the sqlc repositories
```

The consolidated schema is derived, not versioned.

## Submitting a pull request

1. Fork the repository and create a branch from `master`
2. Make your change; run `earthly +all` locally before pushing
3. Open the PR with the template filled in
4. The CI runs the same `+all` gate on the PR; from forks it runs without the
   shared build cache (read-only token), so expect a slower first run

Maintainers: enable branch protection on `master` with the CI check required.

## Reporting issues

Include the LibreVita version or commit, the environment (OS, Earthly version),
and the full error output.
