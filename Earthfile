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

# Generated files exist only inside Earthly build containers.
go-generated:
    FROM +go-source-tools
    RUN templ generate
    RUN sqlc generate

# Export generated sources for local editor support.
generate:
    FROM +go-generated
    SAVE ARTIFACT internal/domain AS LOCAL ./internal/domain
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
