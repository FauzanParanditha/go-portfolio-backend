# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Backend API for a personal portfolio site (projects, experiences, contact messages, admin dashboard). Go 1.26 + Fiber v2 (fasthttp) + GORM (PostgreSQL) + Atlas migrations. Auth is JWT (HS256). Komentar kode dan pesan log/error di repo ini ditulis dalam Bahasa Indonesia — ikuti gaya itu saat menambah kode.

## Commands

All day-to-day tasks go through the `Makefile`:

```sh
make run            # go run ./cmd/api  — start the API server (port from APP_PORT, default 8080)
make dev            # live reload via air (config in .air.toml)
make seed           # go run ./cmd/seed — idempotent: creates admin user, optional sample data
make build          # build binary to bin/portfolio-backend
make vet            # go vet ./...
make fmt            # go fmt ./...
make test           # go test -race ./...
make cek            # go vet + gofmt -l (read-only: fails on unformatted files)
make periksa        # release gate: bash tools/periksa.sh (CI runs the same script)
make swagger        # regenerate docs/ from handler annotations (swag init -g cmd/api/main.go)
```

Single test by name:

```sh
go test ./internal/http/middleware/ -run TestAdminRoute -v
```

`make periksa` needs `govulncheck` on PATH (`go install golang.org/x/vuln/cmd/govulncheck@latest`); it
fails rather than skipping the audit when the tool is missing.

Tests are sparse; the existing ones (`internal/http/middleware/require_role_test.go`) exercise the real
middleware chain with `app.Test(...)` and signed JWTs — **no database required**. Prefer this pattern for
auth/middleware behavior so tests stay DB-free.

### Database migrations (Atlas, not GORM AutoMigrate)

Schema is managed by Atlas with versioned SQL in `migrations/`. GORM is used only as a query/ORM layer at
runtime — **do not rely on AutoMigrate**; add a migration instead.

```sh
make migrate-new name=add_something   # create a new migration
make migrate-up                       # hash + apply (env "dev" from atlas.hcl)
make migrate-status
make migrate-down                     # rollback one step
```

Atlas env config is in `atlas.hcl` (default `ATLAS_ENV=dev`). Note: `atlas.hcl` dev URL uses password `root`
while the app's `.env` `DB_DSN` uses `postgres` — keep your local Postgres consistent with whichever you run.

## Architecture

Clean layered structure under `internal/`, wired together in `cmd/api/main.go`:

```
cmd/api/main.go        → load .env, config.Load(), cfg.Validate(), db.New(), build router, Listen
  └─ internal/http/router.go   → NewRouter(): one register*Routes() func per resource
       ├─ middleware/           → global chain + AuthJWT + RequireRole
       ├─ handlers/             → HTTP layer: parse, validate, call repo, shape response
       ├─ repository/           → all GORM/DB access lives here (interface + impl per resource)
       └─ models/               → GORM structs (uuid PKs, json tags)
```

Request flow: **handler** parses body (`c.BodyParser`) → validates (`validation.ValidateStruct`) → calls a
**repository** method (takes `ctx`, returns models) → returns JSON. Handlers never write raw SQL; repositories
own all queries and use GORM parameterized `Where("col = ?", val)` (this is the SQL-injection boundary — keep
it that way, never string-concat into queries).

### Routing & auth (internal/http/router.go)

- Public routes: `/healthz`, `/readyz` (root, for monitoring — no envelope, no rate limit),
  `/api/v1/healthz`, `/api/v1/readyz` (public status for the frontend: `{data}` envelope, one shared
  60/min limiter, `Cache-Control: no-store`, generic 503 that never names the failing component),
  `/api/v1/projects`, `/api/v1/experiences`, `POST /api/v1/contact`, `POST /api/v1/auth/login`, and a few
  more — the authoritative, test-enforced list is `routePublik` in `internal/http/router_penjaga_test.go`.
- `/api/v1/me` requires only `AuthJWT` (any authenticated user reads their own info).
- **Every `/api/v1/admin/*` group MUST chain `AuthJWT` then `RequireRole("admin")`.** Each
  `registerAdmin*Routes` function repeats this pair locally. When adding a new admin resource, copy that
  exact pattern — `AuthJWT` alone is not enough (it only proves the token is valid, not that the role is
  admin). Fiber detail: `group.Use(...)` applies to the group's path *prefix* in registration order, so the
  pair in the first-registered admin group already covers later ones, while a route registered *before* any
  pair is unguarded. Repeating the pair keeps each function safe regardless of order — don't rely on another
  group's `Use`.

`AuthJWT` (`middleware/auth_jwt.go`) validates the HS256 signature, enforces the signing method, and stores
`user_id` + `user_role` in `c.Locals`. `RequireRole` (`middleware/require_role.go`) reads `user_role` from
Locals — so it only works *after* `AuthJWT`.

### Config & startup safety (internal/config/config.go)

`config.Load()` reads env via `helpers.GetEnv*`. `cfg.Validate()` is called in `main.go` and rejects insecure
config (default/short `JWT_SECRET`, `CORS *` + credentials). In `production` (`APP_ENV=production`) a violation
is **fatal**; otherwise it logs a warning. `cfg.IsProduction()` also gates Swagger (only mounted off-prod).

### Error & response conventions

- Errors: return `fiber.NewError(status, msg)` from handlers/middleware. The central handler
  (`internal/http/error_handler.go`, set as Fiber `ErrorHandler`) renders the JSON envelope and derives a
  stable `code` from the status via `errorCodeForStatus` (e.g. 401→`UNAUTHORIZED`, 403→`FORBIDDEN`,
  503→`SERVICE_UNAVAILABLE`, other 5xx→`SERVER_ERROR`). Don't build error JSON by hand.
- Validation failures return `422` with a `details` map built by `validation.ToFieldErrors`.
- Success envelope: `{"data": ...}` via `internal/http/response` helpers (`OK`, `Created`), or `c.JSON` for
  custom shapes. Responses are wrapped (`data`/`error` + `requestId`), not bare models.

### Models & DTOs

Models (`internal/models/`) use `uuid.UUID` PKs with `default:uuid_generate_v4()` and back GORM relations
(Preload-heavy reads in repositories). `User.Password` is `json:"-"` — never expose it. Each handler package
has `*_dto.go` files defining request structs with `validate:` tags; map DTO→model in the handler.

## Security invariants (recently hardened — do not regress)

- `/admin/*` = `AuthJWT` + `RequireRole("admin")`. New admin routes must include both.
  `TestSemuaRouteBerpenjaga` (`internal/http/router_penjaga_test.go`) calls every handler route of the real
  router: no credentials → 401, `/api/v1/admin/*` with a non-admin token → 403. A route that is meant to be
  public must be added to `routePublik` in that file, with its reason — that edit is the review signal.
  Its limits: handlers mounted with `Use`/`Static` cannot be called, so only their *paths* are pinned
  (`jalurUse`); and the role check covers the `/api/v1/admin` prefix only — an admin-only endpoint MUST live
  under that prefix, and route paths must use plain `:param` / `*` segments (constraints fail the test).
- `JWT_SECRET` must be strong (≥32 chars, non-default); enforced by `cfg.Validate()`.
- Login (`POST /auth/login`) is rate-limited (10/min) in `registerAuthRoutes`.
- `/api/v1/healthz` + `/api/v1/readyz` share one 60/min limiter (`registerPublicStatusRoutes`) that runs
  before the DB ping; the ping has a 2s timeout and its result is memoized for 5s, so anonymous traffic
  cannot turn these routes into a DB load generator. Their 503 body must stay generic.
- Global middleware order in `middleware/RegisterGlobal`: recover → body-size guard (413, via
  `Content-Length`) → helmet (security headers) → requestid → `RejectContentEncoding` → logger → CORS.
  CORS origins come from env; never pair `*` with credentials.
- Requests with any `Content-Encoding` other than empty/`identity` are rejected with 415
  (`RejectContentEncoding`) before anything reads the body — Fiber/fasthttp would otherwise decompress
  without limit (decompression bomb). Nothing before it may call `c.Body()`/`BodyParser`/`FormFile`.
- Request bodies are capped at 1MB by the body-size guard in `middleware.RegisterGlobal`; only the upload
  route is allowed more. Fiber's own `BodyLimit` (set in `NewRouter`) is `UPLOAD_MAX_BYTES` + 1MB, because
  it applies to the whole app.
- `.env` is gitignored and must stay untracked; `.env.example` documents the keys.

## Deploy & operational scripts

Production runs as a Docker image under Docker Compose (`deploy/Dockerfile`, `deploy/docker-compose.yml`,
`deploy/env.example`), driven by `install.sh`, `update.sh`, `status.sh`, `backup.sh` at the repo root. See
`docs/PEMELIHARAAN.md` (commands, traps, incident checklist) and ADR-015 in `docs/DECISIONS.md`.

- `deploy/lib-dasar.sh` is a **verbatim copy** from the project-standard skill — never edit it here; project
  functions go in `deploy/lib.sh`.
- The runtime image is distroless: no shell. The container healthcheck is the `cmd/healthcheck` binary.
- Images are built by GitHub Actions (`.github/workflows/image.yml`) on every push to `master`, scanned by
  `tools/periksa-image.sh` (every layer — the repo and package are public, and a published image cannot be
  recalled), then pushed to `ghcr.io` tagged with the full commit SHA. `update.sh` pulls the image for the
  checked-out commit; `--bangun-lokal` builds on the server instead (ADR-016). Never add a `COPY` that could
  bring `.env`, `uploads/` or `.git` into the image, and keep `.dockerignore` excluding them.
- A new env key the app needs in production must be added to `deploy/env.example` (`update.sh` copies new
  keys into the server's `deploy/.env`) **and** to the `environment:` list of the `api` service in
  `deploy/docker-compose.yml` — the container only receives keys listed there, not the whole `deploy/.env`
  (`internal/config/deploy_env_test.go` fails when one is missing).
- Every migration must be committed together with an up-to-date `migrations/atlas.sum`, and be additive:
  `update.sh --rollback` does not undo migrations.
- The scripts were tested on a local Linux VM (install, re-install, update, failed update, rollback, backup,
  restore). Re-run that kind of test after changing them; they run as root on the server.

## API docs

Swagger annotations live in handler comments; `make swagger` regenerates `docs/`. UI is served at `/swagger/*`
only when not in production. Keep annotations in sync when changing endpoints.

## Tim Agent & Orchestrasi

Sesi utama berperan sebagai **PM/orchestrator**: pecah tugas, tentukan urutan, delegasikan ke subagent
(`.claude/agents/`), lalu rakit hasilnya. Subagent tidak memanggil subagent lain — koordinasi terjadi di
sesi utama.

Ringkasan stack untuk konteks delegasi:
- **Backend (repo ini):** Go 1.26 + Fiber v2 (fasthttp) + GORM (PostgreSQL) + Atlas migrations, auth JWT HS256.
- **Frontend (repo terpisah `../portfolio-frontend`):** Next.js 16 App Router + React 19 + TypeScript + Tailwind, pnpm.
- **Testing:** `make test` (`go test -race ./...`); utamakan test DB-free (`app.Test` + JWT). **Lint/format:** `make cek`, `make fmt`. **Gerbang rilis:** `make periksa`.
- **Konvensi:** komentar & pesan log/error Bahasa Indonesia; commit konvensional (`feat`, `fix(security)`, dst).

Panduan delegasi:
- **architect** — dipanggil DULUAN untuk fitur besar/lintas-layer: rancang struktur & kontrak sebelum implementasi.
- **backend-engineer** — implementasi API, logika server, database (handler/repository/model/migration).
- **frontend-engineer** — pekerjaan UI/klien (di repo FE terpisah; koordinasi lintas-repo lewat `docs/`).
- **qa-engineer** — setelah implementasi: tulis/jalankan test, reproduksi & verifikasi bug.
- **code-reviewer** — sebelum merge: tinjau kualitas kode (read-only, melapor).
- **security-auditor** — sebelum rilis / saat menyentuh auth/data sensitif: audit kerentanan (read-only, melapor).

### Alur tipikal sebuah fitur
1. **architect** merancang & memecah tugas + mendefinisikan kontrak API.
2. **backend-engineer** (& **frontend-engineer** bila lintas-repo) implementasi.
3. **qa-engineer** menulis & menjalankan test.
4. **code-reviewer** meninjau kualitas.
5. **security-auditor** mengaudit bila fitur sensitif.
6. Sesi utama merangkum, pastikan Definition of Done terpenuhi, lalu siapkan untuk merge.

### Definition of Done (global)
- [ ] Lulus lint & format (`make cek`; perbaiki format dengan `make fmt`).
- [ ] Test relevan ada dan hijau (`make test`).
- [ ] Gerbang rilis hijau (`make periksa`).
- [ ] Tidak ada rahasia ter-hardcode; `.env` tetap untracked.
- [ ] Sudah di-review (code-reviewer) dan, bila sensitif, di-audit (security-auditor).
- [ ] Dokumentasi/README/Swagger diperbarui bila perlu.

> Catatan: `security-auditor` & `code-reviewer` sengaja **read-only** — mereka melaporkan temuan; perbaikan
> dikerjakan engineer terkait.
