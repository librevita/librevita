# ==========================================================================
# LibreVita — reproducible build pipeline (Earthly).
#
# Strategy: layered caching.
#
#   Public targets (top)      run by humans and CI: +all, +build (with
#                             --dev/--os/--arch), +image, +generate,
#                             +test, +vet, +tidy.
#   Internal targets (middle) cache-friendly dependency layers:
#                             +go-deps (modules only), +go-templ/+go-sqlc
#                             (generators), +node-deps/+node-check/
#                             +node-css/+node-bundle (npm assets),
#                             +schema (DDL for sqlc), +frontend (exported
#                             assets), +go-generated (sources plus
#                             generated code).
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
# code generator versions. Overridable from the CLI:
# earthly +build --GO_IMAGE=golang:1.27.0-alpine3.24
# --------------------------------------------------------------------------

ARG --global GO_IMAGE=golang:1.26.5-alpine3.24
ARG --global NODE_IMAGE=node:26.7-alpine3.24
ARG --global TEMPL_VERSION=v0.3.1020
ARG --global SQLC_VERSION=v1.31.1

# Base name of the binaries exported by +build.
ARG --global name=librevita

# --------------------------------------------------------------------------
# Public targets
# --------------------------------------------------------------------------

# Validate code, analysis, and the production image together.
all:
    BUILD +test
    BUILD +vet
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

# Export generated sources to the local tree for editor support.
generate:
    FROM +go-generated
    SAVE ARTIFACT internal/domain AS LOCAL ./internal/domain
    SAVE ARTIFACT internal/core/audit/repository AS LOCAL ./internal/core/audit/repository
    SAVE ARTIFACT internal/core/auth/repository AS LOCAL ./internal/core/auth/repository
    SAVE ARTIFACT internal/core/policy/repository AS LOCAL ./internal/core/policy/repository
    SAVE ARTIFACT internal/ui AS LOCAL ./internal/ui
    SAVE ARTIFACT pkg AS LOCAL ./pkg

# Run the complete test suite against generated sources.
test:
    FROM +go-generated
    DO +GO_TEST

# Run static analysis against generated sources.
vet:
    FROM +go-generated
    DO +GO_VET

# Synchronize module metadata locally. Runs against generated sources so
# imports of templ and sqlc output resolve.
tidy:
    FROM +go-generated
    DO +GO_MOD --args=tidy
    SAVE ARTIFACT go.mod AS LOCAL ./go.mod
    SAVE ARTIFACT go.sum AS LOCAL ./go.sum

# --------------------------------------------------------------------------
# Internal targets
# --------------------------------------------------------------------------

# Module dependencies. Rebuilt only when go.mod or go.sum change, so every
# downstream target reuses the downloaded module cache across code edits.
go-deps:
    FROM $GO_IMAGE
    WORKDIR /src
    ENV CGO_ENABLED=0
    COPY go.mod go.sum ./
    DO +GO_MOD

# Code generation tools, pinned by version and built from the bare Go
# toolchain: go install pkg@version resolves the tool's own module graph,
# so neither application go.mod changes nor the other tool rebuilds ever
# invalidate them.
go-templ:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=github.com/a-h/templ/cmd/templ --version=$TEMPL_VERSION
    SAVE ARTIFACT /go/bin/templ

go-sqlc:
    FROM $GO_IMAGE
    ENV CGO_ENABLED=0
    DO +GO_INSTALL --package=github.com/sqlc-dev/sqlc/cmd/sqlc --version=$SQLC_VERSION
    SAVE ARTIFACT /go/bin/sqlc

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
frontend:
    FROM +node-check
    COPY +node-css/app.css /out/app.css
    COPY +node-bundle/static-js /static-js
    SAVE ARTIFACT /out/app.css AS LOCAL ./internal/ui/static/css/app.css
    SAVE ARTIFACT /static-js /static-js
    SAVE ARTIFACT /static-js AS LOCAL ./internal/ui/static/js

# Consolidated schema for sqlc, derived from the Goose migrations (the
# single source of truth). The exported file is an artifact for local editor
# support and is not versioned. Only the packages on the schemagen import
# chain are copied, so changes elsewhere do not invalidate the schema.
schema:
    FROM +go-deps
    COPY cmd/schemagen ./cmd/schemagen
    COPY internal/core/database ./internal/core/database
    COPY internal/core/config ./internal/core/config
    COPY db/embed.go ./db/embed.go
    COPY db/migrations ./db/migrations
    DO +GO_CMD --args="run ./cmd/schemagen --out /out/schema.sql"
    SAVE ARTIFACT /out/schema.sql AS LOCAL ./db/schema/schema.sql

# Sources plus generated code: compiled assets, the consolidated schema,
# then templ and sqlc output. Everything generated lives only inside
# Earthly containers. COPY . . sends only what .earthlyignore does not
# exclude, so local outputs never invalidate the layers.
go-generated:
    FROM +go-templ
    WORKDIR /src
    COPY +go-sqlc/sqlc /go/bin/sqlc
    COPY . .
    COPY +frontend/app.css ./internal/ui/static/css/app.css
    COPY +frontend/static-js ./internal/ui/static/js
    COPY +schema/schema.sql ./db/schema/schema.sql
    RUN templ generate
    RUN sqlc generate

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
