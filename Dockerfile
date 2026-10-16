# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27

# The build stage runs on the builder's platform and cross-compiles for the
# target, so multi-arch images need no Go emulation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
# Pure Go build (modernc SQLite): a static binary without CGO.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/fusen ./cmd/fusen

# Distroless static: CA certificates and a non-root user, no shell; a
# multi-arch base, and only COPY runs in this stage. The timezone database is
# embedded in the binary.
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
LABEL org.opencontainers.image.title="fusen" \
      org.opencontainers.image.description="Personal kanban boards in Telegram" \
      org.opencontainers.image.source="https://github.com/Mimic890/fusen" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/fusen /usr/local/bin/fusen
ENV DB_PATH=/data/fusen.db
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/fusen"]
