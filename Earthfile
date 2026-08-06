VERSION 0.8

# Reproducible Earthly build pipeline.
# CGO is disabled for every Go build.

# Dependency layer. It changes only when the Go module files change.
go-base:
    FROM golang:1.26.5-alpine3.24
    WORKDIR /src
    ENV CGO_ENABLED=0
    COPY go.mod go.sum ./
    RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Code generation tools are isolated from the application source layer.
tools:
    FROM +go-base
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go install github.com/a-h/templ/cmd/templ@v0.3.1020
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# Source layers keep dependency and tool caches reusable across code changes.
go-source:
    FROM +go-base
    COPY . .

go-source-tools:
    FROM +tools
    COPY . .

# Frontend toolchain, split into independently cached tool layers.
# Each target downloads a pinned binary, verifies its SHA-256, and exports
# it; the frontend target consumes both. No Node or npm anywhere.

esbuild-tool:
    ARG ESBUILD_VERSION=0.28.1
    ARG ESBUILD_SHA256=0c6588b092a2c291a72bab90659f3c9e0e25e0fe59c9ac12b4dae4d945e5548c
    FROM debian:bookworm-slim
    RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tar \
        && rm -rf /var/lib/apt/lists/*
    RUN curl -fsSL -o /tmp/esbuild.tgz \
        https://registry.npmjs.org/@esbuild/linux-x64/-/linux-x64-${ESBUILD_VERSION}.tgz \
        && tar -xzf /tmp/esbuild.tgz -C /tmp \
        && cp /tmp/package/bin/esbuild /usr/local/bin/esbuild \
        && echo "${ESBUILD_SHA256}  /usr/local/bin/esbuild" | sha256sum -c - \
        && chmod +x /usr/local/bin/esbuild
    SAVE ARTIFACT /usr/local/bin/esbuild

tailwind-tool:
    ARG TAILWIND_VERSION=3.4.17
    ARG TAILWIND_SHA256=7d24f7fa191d2193b78cd5f5a42a6093e14409521908529f42d80b11fde1f1d4
    FROM debian:bookworm-slim
    RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
        && rm -rf /var/lib/apt/lists/*
    RUN curl -fsSL -o /usr/local/bin/tailwindcss \
        https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-x64 \
        && echo "${TAILWIND_SHA256}  /usr/local/bin/tailwindcss" | sha256sum -c - \
        && chmod +x /usr/local/bin/tailwindcss
    SAVE ARTIFACT /usr/local/bin/tailwindcss

# Download the pinned runtime assets (no CDN, no committed vendor files).
# Each file is verified against its recorded SHA-256; see
# internal/ui/static/js/VENDOR.md for the manifest.
frontend-vendor:
    ARG HTMX_SHA256=449317ade7881e949510db614991e195c3a099c4c791c24dacec55f9f4a2a452
    ARG SSE_SHA256=be05b2e2265279f035271adbea0b72a356f20ce4dfa5870481bfe9c51b822fc1
    ARG ALPINE_SHA256=566167134bb2347110904e2ced6e816d2e8d837200c158f98b72372b3bb0b9a6
    FROM debian:bookworm-slim
    RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
        && rm -rf /var/lib/apt/lists/*
    RUN mkdir -p /out
    RUN curl -fsSL -o /out/htmx-1.9.12.min.js https://unpkg.com/htmx.org@1.9.12/dist/htmx.min.js \
        && echo "${HTMX_SHA256}  /out/htmx-1.9.12.min.js" | sha256sum -c -
    RUN curl -fsSL -o /out/htmx-sse-1.9.12.js https://unpkg.com/htmx.org@1.9.12/dist/ext/sse.js \
        && echo "${SSE_SHA256}  /out/htmx-sse-1.9.12.js" | sha256sum -c -
    RUN curl -fsSL -o /out/alpine-csp-3.15.12.min.js https://unpkg.com/@alpinejs/csp@3.15.12/dist/cdn.min.js \
        && echo "${ALPINE_SHA256}  /out/alpine-csp-3.15.12.min.js" | sha256sum -c -
    SAVE ARTIFACT /out

# Build the frontend assets with the pinned toolchain (glibc binaries).
# Tailwind generates the stylesheet; esbuild bundles the modern JavaScript
# source (frontend/src) to the XP floor (target=firefox52) with the
# compatibility polyfills. Runtime assets come from +frontend-vendor. No
# Node or Tailwind ever reaches the runtime image. Outputs are also
# exported locally for editor support.
frontend:
    FROM debian:bookworm-slim
    COPY +esbuild-tool/esbuild /usr/local/bin/esbuild
    COPY +tailwind-tool/tailwindcss /usr/local/bin/tailwindcss
    COPY +frontend-vendor/out/htmx-1.9.12.min.js /out/vendor/htmx-1.9.12.min.js
    COPY +frontend-vendor/out/htmx-sse-1.9.12.js /out/vendor/htmx-sse-1.9.12.js
    COPY +frontend-vendor/out/alpine-csp-3.15.12.min.js /out/vendor/alpine-csp-3.15.12.min.js
    WORKDIR /src
    COPY . .
    RUN tailwindcss -i frontend/input.css -o /out/app.css --minify
    RUN esbuild frontend/src/ui.js --bundle --platform=browser --format=iife --target=firefox52 --minify --outfile=/out/ui.js
    RUN mkdir -p internal/ui/static/js \
        && cp /out/vendor/* internal/ui/static/js/ \
        && cp /out/ui.js internal/ui/static/js/
    SAVE ARTIFACT /out/app.css AS LOCAL ./internal/ui/static/css/app.css
    SAVE ARTIFACT internal/ui/static/js /static-js
    SAVE ARTIFACT internal/ui/static/js AS LOCAL ./internal/ui/static/js

# Generated files exist only inside Earthly build containers.
go-generated:
    FROM +go-source-tools
    COPY +frontend/app.css ./internal/ui/static/css/app.css
    COPY +frontend/static-js ./internal/ui/static/js
    RUN templ generate
    RUN sqlc generate

# Export generated sources for local editor support.
generate:
    FROM +go-generated
    SAVE ARTIFACT internal/domain AS LOCAL ./internal/domain
    SAVE ARTIFACT internal/core/audit/repository AS LOCAL ./internal/core/audit/repository
    SAVE ARTIFACT internal/core/auth/repository AS LOCAL ./internal/core/auth/repository
    SAVE ARTIFACT internal/core/policy/repository AS LOCAL ./internal/core/policy/repository
    SAVE ARTIFACT internal/ui AS LOCAL ./internal/ui
    SAVE ARTIFACT pkg AS LOCAL ./pkg

# Fast, unoptimized developer binary.
build-dev:
    FROM +go-generated
    RUN mkdir -p /out
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -o /out/librevita-dev ./cmd/web
    SAVE ARTIFACT /out/librevita-dev AS LOCAL ./bin/librevita-dev

# Stripped production binary.
build:
    FROM +go-generated
    RUN mkdir -p /out
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -trimpath -ldflags="-s -w -buildid=" -o /out/librevita ./cmd/web
    SAVE ARTIFACT /out/librevita AS LOCAL ./bin/librevita

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

# Cross-compile a static binary without building a container image.
# Usage: earthly +build-cross --GOARCH=riscv64
build-cross:
    ARG GOOS=linux
    ARG GOARCH=amd64
    FROM +go-generated
    RUN mkdir -p /out
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags="-s -w -buildid=" -o /out/librevita-$GOOS-$GOARCH ./cmd/web
    SAVE ARTIFACT /out/librevita-$GOOS-$GOARCH AS LOCAL ./bin/librevita-$GOOS-$GOARCH

# Run the complete test suite against generated sources.
test:
    FROM +go-generated
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go test ./...

# Run static analysis against generated sources.
vet:
    FROM +go-generated
    RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go vet ./...

# Synchronize module metadata locally.
tidy:
    FROM +go-source
    RUN --mount=type=cache,target=/go/pkg/mod go mod tidy
    SAVE ARTIFACT go.mod AS LOCAL ./go.mod
    SAVE ARTIFACT go.sum AS LOCAL ./go.sum

# Validate code, analysis, and the production image together.
all:
    BUILD +test
    BUILD +vet
    BUILD +build
    BUILD +image
