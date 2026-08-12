# ==========================================================================
# LibreVita — reproducible build pipeline (Earthly).
#
# Strategy: layered caching.
#
#   Public targets (top)      run by humans and CI: +all, +build (with
#                             --dev/--os/--arch), +image, +test, +vet,
#                             +lint, +audit. They delegate to the
#                             language stages.
#   Internal targets (middle) cache-friendly dependency layers and the
#                             stages the public targets delegate to:
#                             +go-deps (modules), +go-tool-* (pinned
#                             toolchains), +go-gen-* (generation stages),
#                             +go-test/+go-vet/+go-lint/+go-vuln-source/
#                             +go-vuln-binary/+go-tidy (Go consumers),
#                             +node-deps/+node-check/+node-css/
#                             +node-bundle (npm stages),
#                             +node-test/+node-audit (npm consumers),
#                             +node-frontend (exported assets),
#                             +go-generated (sources plus generated code).
#   Functions (bottom)        shared recipes: every Go command runs through
#                             a function, so the cache mounts are declared
#                             once each (+GO_BUILD, +GO_CMD, +GO_INSTALL,
#                             +GO_MOD, +GO_TEST, +GO_VET).
#
# CGO is disabled for every Go build, so the binaries are static and the
# production image runs on scratch. Files that would invalidate the cache
# (local outputs, generated sources) are excluded via .earthlyignore.
# ==========================================================================

VERSION 0.8

# --------------------------------------------------------------------------
# Pinned versions - the single source of truth for toolchain images and
# code generator versions. Images are pinned by digest: the BuildKit never
# resolves the tag on the registry, so builds do not hit Docker Hub (and
# cannot be rate-limited). To update an image, resolve the new digest and
# bump it here; overridable from the CLI:
# earthly +build --GO_IMAGE=golang:1.26.5-alpine3.24
# --------------------------------------------------------------------------

ARG --global GO_IMAGE=golang:1.26.5-alpine3.24@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2
ARG --global NODE_IMAGE=node:26.7-alpine3.24@sha256:aadf416b2cdce311a8811ba3f0608a61b77dbf997500e2eafe781b51f6a0b019
ARG --global TEMPL_VERSION=v0.3.1020
ARG --global SQLC_VERSION=v1.31.1
ARG --global GOLANGCI_VERSION=v2.12.2
ARG --global GOVULN_VERSION=v1.1.4

# Base name of the binaries exported by +build.
ARG --global name=librevita

# --------------------------------------------------------------------------
# Public targets
# --------------------------------------------------------------------------

# Validate code, analysis, and the production image together.
all:
    BUILD +test
    BUILD +vet
    BUILD +lint
    BUILD +audit
    BUILD +build
    BUILD +image

# Build the application binary. Defaults to a stripped production build;
# pass --dev=true for a fast, unoptimized binary, or --os/--arch to
# cross-compile. Exported to ./bin/NAME[-dev][-$os-$arch] (NAME from the
# global "name" arg, default librevita).
build:
    ARG dev=false
    ARG os=linux
    ARG arch=amd64
    FROM +go-generated
    LET binname = "$name"
    IF [ "$dev" = "true" ]
        SET binname = "$name-dev"
    ELSE IF [ "$os" = "windows" ]
        SET binname = "$name-$os-$arch.exe"
    ELSE IF [ "$os$arch" != "linuxamd64" ]
        SET binname = "$name-$os-$arch"
    END
    DO +GO_BUILD --dev=$dev --os=$os --arch=$arch
    SAVE ARTIFACT /out/librevita AS LOCAL ./bin/$binname

# Minimal non-root production image.
image:
    ARG IMAGE_TAG=librevita:latest
    FROM scratch
    COPY +build/librevita /usr/local/bin/librevita
    ENV LIBREVITA_DATA_DIR=/data/librevita
    VOLUME /data
    EXPOSE 8080
    USER 65532:65532
    ENTRYPOINT ["/usr/local/bin/librevita"]
    SAVE IMAGE $IMAGE_TAG

# Run the complete test suite: Go tests and the frontend unit tests.
test:
    BUILD +go-test
    BUILD +node-test

# Run static analysis.
vet:
    BUILD +go-vet

# Run golangci-lint against generated sources (config: .golangci.yaml).
lint:
    BUILD +go-lint

# Security audits: the Go module scan (govulncheck), the binary scan and
# the npm audit.
audit:
    BUILD +go-vuln-source
    BUILD +go-vuln-binary
    BUILD +node-audit

# --------------------------------------------------------------------------
# Internal targets - Go
# --------------------------------------------------------------------------

# Module dependencies. Rebuilt only when go.mod or go.sum change, so every
# downstream target reuses the downloaded module cache across code edits.
go-deps:
    FROM $GO_IMAGE
    WORKDIR /src
    ENV CGO_ENABLED=0
    COPY go.mod go.sum ./
    DO +GO_MOD

# Build-time tools (generators and analyzers), pinned by version and
# built from the bare Go toolchain: go install pkg@version resolves the
# tool's own module graph, so neither application go.mod changes nor the
# other tool rebuilds ever invalidate them.
go-tool-templ:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=github.com/a-h/templ/cmd/templ --version=$TEMPL_VERSION
    SAVE ARTIFACT /go/bin/templ

go-tool-sqlc:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=github.com/sqlc-dev/sqlc/cmd/sqlc --version=$SQLC_VERSION
    SAVE ARTIFACT /go/bin/sqlc

go-tool-lint:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=github.com/golangci/golangci-lint/v2/cmd/golangci-lint --version=$GOLANGCI_VERSION
    SAVE ARTIFACT /go/bin/golangci-lint

go-tool-vuln:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=golang.org/x/vuln/cmd/govulncheck --version=$GOVULN_VERSION
    SAVE ARTIFACT /go/bin/govulncheck

# Consolidated schema for sqlc, derived from the Goose migrations (the
# single source of truth). The exported file is an artifact for local editor
# support and is not versioned. Only the packages on the schemagen import
# chain are copied, so changes elsewhere do not invalidate the schema.
go-gen-schema:
    FROM +go-deps
    COPY cmd/schemagen ./cmd/schemagen
    COPY internal/core/database ./internal/core/database
    COPY internal/core/config ./internal/core/config
    COPY db/embed.go ./db/embed.go
    COPY db/migrations ./db/migrations
    DO +GO_CMD --args="run ./cmd/schemagen --out /out/schema.sql"
    SAVE ARTIFACT /out/schema.sql AS LOCAL ./db/schema/schema.sql

# Generated templ views. Only the sources the generator reads are
# copied, so a .templ edit re-runs just this stage; the sqlc stage and
# the rest of the graph stay cached. The views are also exported for
# editor support.
go-gen-templ:
    FROM +go-tool-templ
    WORKDIR /src
    COPY go.mod ./
    COPY internal ./internal
    RUN templ generate
    SAVE ARTIFACT internal/domain ./internal/domain
    SAVE ARTIFACT internal/ui ./internal/ui
    SAVE ARTIFACT internal/domain AS LOCAL ./internal/domain
    SAVE ARTIFACT internal/ui AS LOCAL ./internal/ui

# Generated sqlc repositories. Depends on the consolidated schema from
# go-gen-schema: the queries and that DDL are the only inputs, so a
# change elsewhere never re-runs this stage. Every repository tree the
# generator produces is exported for editor support.
go-gen-sqlc:
    FROM +go-tool-sqlc
    WORKDIR /src
    COPY go.mod ./
    COPY sqlc.yaml ./
    COPY db/query ./db/query
    COPY +go-gen-schema/schema.sql ./db/schema/schema.sql
    RUN sqlc generate
    SAVE ARTIFACT internal/core ./internal/core
    SAVE ARTIFACT internal/domain ./internal/domain
    SAVE ARTIFACT internal/core AS LOCAL ./internal/core
    SAVE ARTIFACT internal/domain AS LOCAL ./internal/domain

# Sources plus generated code: compiled assets, the templ views and the
# sqlc repositories. Everything generated lives only inside Earthly
# containers; COPY . . sends only what .earthlyignore does not exclude,
# so local outputs never invalidate the layers.
go-generated:
    FROM +go-deps
    WORKDIR /src
    COPY . .
    COPY +node-frontend/app.css ./internal/ui/static/css/app.css
    COPY +node-frontend/static-js ./internal/ui/static/js
    COPY +go-gen-templ/internal ./internal
    COPY +go-gen-sqlc/internal ./internal

# Consumers delegated by the public targets.
go-test:
    FROM +go-generated
    DO +GO_TEST

go-vet:
    FROM +go-generated
    DO +GO_VET

go-lint:
    FROM +go-generated
    COPY +go-tool-lint/golangci-lint /go/bin/golangci-lint
    RUN golangci-lint run

# Scan the module for known vulnerabilities (Go Vulnerability Database,
# source analysis: reports only symbols the code actually calls).
go-vuln-source:
    FROM +go-generated
    COPY +go-tool-vuln/govulncheck /go/bin/govulncheck
    RUN govulncheck ./...

# Scan the application binary for known vulnerabilities. The scanned
# artifact is compiled here with placeholder assets: govulncheck
# analyzes the Go code, not the embedded files, so the whole frontend
# pipeline is skipped (the scanned binary is never run). The build is
# unsymbolized (--dev=true): with -s -w the call graph is unavailable
# and govulncheck reports every vulnerability in the module graph -
# including GO-2026-5932, the deprecated-by-design x/crypto/openpgp
# that the code never calls, which would keep the gate red forever.
# With symbols, the symbol-level filter applies and only reachable code
# is reported.
go-vuln-binary:
    FROM +go-deps
    WORKDIR /src
    COPY . .
    COPY +go-gen-templ/internal ./internal
    COPY +go-gen-sqlc/internal ./internal
    COPY +go-tool-vuln/govulncheck /go/bin/govulncheck
    RUN mkdir -p internal/ui/static/css internal/ui/static/js && \
        touch internal/ui/static/css/app.css internal/ui/static/js/placeholder.js
    DO +GO_BUILD --dev=true --output=/out/librevita-scan
    RUN govulncheck -mode=binary /out/librevita-scan

# Synchronize module metadata locally. Runs against generated sources so
# imports of templ and sqlc output resolve.
go-tidy:
    FROM +go-generated
    DO +GO_MOD --args=tidy
    SAVE ARTIFACT go.mod AS LOCAL ./go.mod
    SAVE ARTIFACT go.sum AS LOCAL ./go.sum

# --------------------------------------------------------------------------
# Internal targets - Node
# --------------------------------------------------------------------------

# npm dependencies. Rebuilt only when package.json or package-lock.json
# change, so downstream layers reuse the installed tree across edits.
node-deps:
    FROM $NODE_IMAGE
    WORKDIR /src
    COPY package.json package-lock.json ./
    RUN --mount=type=cache,target=/root/.npm npm ci

# Type-check the TypeScript sources (noEmit, validation only).
node-check:
    FROM +node-deps
    COPY tsconfig.json ./
    COPY internal/ui/ts ./internal/ui/ts
    RUN --mount=type=cache,target=/root/.npm npm run check

# CSS pipeline (Tailwind JIT + autoprefixer + cssnano). Tailwind scans
# the templates and TS sources for class names, so they are inputs here
# as well; the domain tree is copied whole to reach the templates.
node-css:
    FROM +node-deps
    COPY postcss.config.ts ./
    COPY tailwind.config.ts ./
    COPY internal/ui ./internal/ui
    COPY internal/domain ./internal/domain
    RUN --mount=type=cache,target=/root/.npm npm run css
    SAVE ARTIFACT /out/app.css

# Bundled TypeScript (esbuild) for the XP browser floor.
node-bundle:
    FROM +node-deps
    COPY tsconfig.json ./
    COPY internal/ui/ts ./internal/ui/ts
    COPY internal/ui/build.ts ./internal/ui/build.ts
    RUN --mount=type=cache,target=/root/.npm npm run bundle
    RUN mkdir -p /static-out/js && cp /out/ui.js /out/theme.js /out/vendor/* /static-out/js/
    SAVE ARTIFACT /static-out/js /static-js

# Compiled assets (CSS, bundled JS) plus the type-check dependency,
# exported for the embed layer and local editor use.
node-frontend:
    FROM +node-check
    COPY +node-css/app.css /out/app.css
    COPY +node-bundle/static-js /static-js
    SAVE ARTIFACT /out/app.css AS LOCAL ./internal/ui/static/css/app.css
    SAVE ARTIFACT /static-js /static-js
    SAVE ARTIFACT /static-js AS LOCAL ./internal/ui/static/js

# Consumers delegated by the public targets.
node-test:
    FROM +node-deps
    COPY internal/ui/ts ./internal/ui/ts
    RUN --mount=type=cache,target=/root/.npm npm test

# npm audit: known vulnerabilities in the frontend dependencies.
node-audit:
    FROM +node-deps
    RUN --mount=type=cache,target=/root/.npm npm audit --audit-level=high

# --------------------------------------------------------------------------
# Functions
# --------------------------------------------------------------------------
#
# Every Go command runs through one of these functions, so the cache mounts
# (module cache /go/pkg/mod, build cache /root/.cache/go-build) live in a
# single RUN inside GO_CMD; the specialized functions compose over it.
# GO_BUILD is the exception: it needs flags GO_CMD cannot carry.

# Run any go subcommand with the shared caches. Args are expanded by word
# splitting only: quotes are not processed, so values containing spaces
# split (use GO_BUILD for anything that needs them), and an environment
# assignment at the start (GOOS=...) would be treated as the command.
GO_CMD:
    FUNCTION
    ARG args
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
        go $args

# Run the complete test suite.
GO_TEST:
    FUNCTION
    DO +GO_CMD --args="test ./..."

# Run static analysis.
GO_VET:
    FUNCTION
    DO +GO_CMD --args="vet ./..."

# Manage module metadata (download by default, tidy via --args=tidy).
GO_MOD:
    FUNCTION
    ARG args=download
    DO +GO_CMD --args="mod $args"

# Install a Go tool pinned at version.
GO_INSTALL:
    FUNCTION
    ARG package
    ARG version
    DO +GO_CMD --args="install $package@$version"

# Compile a Go binary for the target os/arch, set as environment variables.
# This is the one function that cannot compose over GO_CMD: GO_CMD expands
# args by word splitting without quote processing, and the -ldflags value
# contains spaces. Production builds strip the binary; pass --dev=true for
# a fast, unoptimized build.
GO_BUILD:
    FUNCTION
    ARG package=./cmd/web
    ARG output=/out/librevita
    ARG dev=false
    ARG os=linux
    ARG arch=amd64
    ENV GOOS=$os
    ENV GOARCH=$arch
    LET trimpath = "-trimpath"
    LET ldflags = "-s -w -buildid="
    IF [ "$dev" = "true" ]
        SET trimpath = ""
        SET ldflags = ""
    END
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
        go build $trimpath -ldflags="$ldflags" -o "$output" "$package"
