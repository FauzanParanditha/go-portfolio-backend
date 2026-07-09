# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Backend API for a personal portfolio site (projects, experiences, contact messages, admin dashboard). Go 1.25 + Fiber v2 (fasthttp) + GORM (PostgreSQL) + Atlas migrations. Auth is JWT (HS256). Komentar kode dan pesan log/error di repo ini ditulis dalam Bahasa Indonesia — ikuti gaya itu saat menambah kode.

## Commands

All day-to-day tasks go through the `Makefile`:

```sh
make run            # go run ./cmd/api  — start the API server (port from APP_PORT, default 8080)
make dev            # live reload via air (config in .air.toml)
make seed           # go run ./cmd/seed — idempotent: creates admin user, optional sample data
make build          # build binary to bin/portfolio-backend
make vet            # go vet ./...
make fmt            # go fmt ./...
make swagger        # regenerate docs/ from handler annotations (swag init -g cmd/api/main.go)
```

Tests use the standard toolchain (no `make test` target despite `.PHONY` listing it):

```sh
go test ./...                                              # all tests
go test ./internal/http/middleware/ -run TestAdminRoute -v # single test by name
```

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

- Public routes: `/healthz`, `/readyz`, `/api/v1/projects`, `/api/v1/experiences`, `POST /api/v1/contact`,
  `POST /api/v1/auth/login`.
- `/api/v1/me` requires only `AuthJWT` (any authenticated user reads their own info).
- **Every `/api/v1/admin/*` group MUST chain `AuthJWT` then `RequireRole("admin")`.** Each
  `registerAdmin*Routes` function repeats this pair locally (groups don't inherit). When adding a new admin
  resource, copy that exact pattern — `AuthJWT` alone is not enough (it only proves the token is valid, not
  that the role is admin).

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
  stable `code` from the status via `errorCodeForStatus` (e.g. 401→`UNAUTHORIZED`, 403→`FORBIDDEN`). Don't
  build error JSON by hand.
- Validation failures return `422` with a `details` map built by `validation.ToFieldErrors`.
- Success envelope: `{"data": ...}` via `internal/http/response` helpers (`OK`, `Created`), or `c.JSON` for
  custom shapes. Responses are wrapped (`data`/`error` + `requestId`), not bare models.

### Models & DTOs

Models (`internal/models/`) use `uuid.UUID` PKs with `default:uuid_generate_v4()` and back GORM relations
(Preload-heavy reads in repositories). `User.Password` is `json:"-"` — never expose it. Each handler package
has `*_dto.go` files defining request structs with `validate:` tags; map DTO→model in the handler.

## Security invariants (recently hardened — do not regress)

- `/admin/*` = `AuthJWT` + `RequireRole("admin")`. New admin routes must include both.
- `JWT_SECRET` must be strong (≥32 chars, non-default); enforced by `cfg.Validate()`.
- Login (`POST /auth/login`) is rate-limited (10/min) in `registerAuthRoutes`.
- Global middleware order in `middleware/RegisterGlobal`: recover → helmet (security headers) → requestid →
  logger → CORS. CORS origins come from env; never pair `*` with credentials.
- Fiber `BodyLimit` is 1MB (set in `NewRouter`).
- `.env` is gitignored and must stay untracked; `.env.example` documents the keys.

## API docs

Swagger annotations live in handler comments; `make swagger` regenerates `docs/`. UI is served at `/swagger/*`
only when not in production. Keep annotations in sync when changing endpoints.

## Tim Agent & Orchestrasi

Sesi utama berperan sebagai **PM/orchestrator**: pecah tugas, tentukan urutan, delegasikan ke subagent
(`.claude/agents/`), lalu rakit hasilnya. Subagent tidak memanggil subagent lain — koordinasi terjadi di
sesi utama.

Ringkasan stack untuk konteks delegasi:
- **Backend (repo ini):** Go 1.25 + Fiber v2 (fasthttp) + GORM (PostgreSQL) + Atlas migrations, auth JWT HS256.
- **Frontend (repo terpisah `../portfolio-frontend`):** Next.js 16 App Router + React 19 + TypeScript + Tailwind, pnpm.
- **Testing:** toolchain Go (`go test ./...`); utamakan test DB-free (`app.Test` + JWT). **Lint/format:** `make vet`, `make fmt`.
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
- [ ] Lulus lint & format (`make vet`, `make fmt`).
- [ ] Test relevan ada dan hijau (`go test ./...`).
- [ ] Tidak ada rahasia ter-hardcode; `.env` tetap untracked.
- [ ] Sudah di-review (code-reviewer) dan, bila sensitif, di-audit (security-auditor).
- [ ] Dokumentasi/README/Swagger diperbarui bila perlu.

> Catatan: `security-auditor` & `code-reviewer` sengaja **read-only** — mereka melaporkan temuan; perbaikan
> dikerjakan engineer terkait.
