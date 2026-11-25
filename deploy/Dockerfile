# syntax=docker/dockerfile:1.7
# Image produksi backend. Konteks build = akar repo:
#   docker compose -f deploy/docker-compose.yml build api
# Butuh BuildKit (docker-buildx-plugin) karena memakai RUN --mount=type=cache.

# Satu tempat untuk menaikkan versi; jangan pakai `latest`.
ARG GO_IMAGE=golang:1.26.11-alpine3.24
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM ${GO_IMAGE} AS build
WORKDIR /src
# Binary statis dan dapat diulang; GOTOOLCHAIN=local supaya build tidak
# mengunduh toolchain lain diam-diam.
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-trimpath

# Manifest dulu: lapisan dependensi tidak diulang saat hanya kode yang berubah.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
# Paket docs (Swagger) di-import cmd/api; berkas .md-nya dibuang .dockerignore.
COPY docs ./docs

ARG VERSI=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags "-s -w" -o /out/app/ ./cmd/... \
 && printf '%s\n' "$VERSI" > /out/app/VERSI \
 && chmod -R a+rX,a-w /out/app \
 && mkdir -p /out/uploads

FROM ${RUNTIME_IMAGE}
ARG VERSI=dev
LABEL org.opencontainers.image.title="portfolio-backend" \
      org.opencontainers.image.version="${VERSI}"

# api (server), seed (admin pertama), import (impor proyek), healthcheck.
COPY --from=build /out/app /app
# Folder unggahan milik pengguna non-root: volume bernama yang dipasang di sini
# mewarisi pemiliknya saat pertama dibuat.
COPY --from=build --chown=65532:65532 /out/uploads /data/uploads

ENV APP_ENV=production \
    APP_PORT=8080 \
    UPLOAD_DIR=/data/uploads

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/app/healthcheck"]
ENTRYPOINT ["/app/api"]
