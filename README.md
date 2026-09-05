# Portfolio Backend API

REST API untuk website portfolio pribadi — mengelola **projects**, **experiences**, **contact messages**, dan **admin dashboard**. Dibangun dengan Go + Fiber, PostgreSQL (GORM), migrasi via Atlas, dan autentikasi JWT.

## Tech Stack

| Komponen        | Teknologi                          |
| --------------- | ---------------------------------- |
| Bahasa          | Go 1.26                            |
| Web framework   | [Fiber v2](https://gofiber.io) (fasthttp) |
| Database        | PostgreSQL 14                       |
| ORM             | [GORM](https://gorm.io)            |
| Migrasi         | [Atlas](https://atlasgo.io)        |
| Auth            | JWT HS256 (`golang-jwt/v5`)        |
| Validasi        | `go-playground/validator/v10`      |
| Logging         | `zerolog`                          |
| API docs        | Swagger (`swaggo`)                 |
| Live reload     | [air](https://github.com/air-verse/air) |

## Prasyarat

- Go **1.26+**
- PostgreSQL **14+** berjalan secara lokal
- [Atlas CLI](https://atlasgo.io/getting-started#installation) (untuk migrasi)
- [air](https://github.com/air-verse/air) (opsional, untuk live reload)

## Quick Start

```sh
# 1. Clone & masuk ke direktori proyek
git clone <repo-url>
cd go-portfolio-backend

# 2. Salin contoh environment, lalu isi nilainya
cp .env.example .env
#    Generate JWT_SECRET yang kuat:
openssl rand -base64 48      # tempel hasilnya ke JWT_SECRET di .env

# 3. Pastikan database 'portfolio' sudah dibuat di PostgreSQL
createdb portfolio

# 4. Jalankan migrasi skema
make migrate-up

# 5. Seed user admin (dan data contoh opsional)
make seed

# 6. Jalankan server
make run
```

Server berjalan di `http://localhost:8080` (sesuai `APP_PORT`). Cek `GET /healthz` untuk memastikan hidup.

## Konfigurasi (.env)

Lihat [`.env.example`](.env.example) untuk daftar lengkap. Variabel penting:

| Variabel                  | Default                          | Keterangan |
| ------------------------- | -------------------------------- | ---------- |
| `APP_ENV`                 | `development`                    | `production` mengaktifkan mode ketat (lihat Keamanan) |
| `APP_PORT`                | `8080`                           | Port HTTP |
| `DB_DSN`                  | `postgres://...localhost.../portfolio?sslmode=disable` | Koneksi PostgreSQL |
| `JWT_SECRET`              | —                                | **Wajib diisi**, minimal 32 karakter |
| `JWT_EXPIRES_IN`          | `3600`                           | Masa berlaku token (detik) |
| `CORS_ALLOWED_ORIGINS`    | `http://localhost:3000`          | Jangan `*` jika credentials aktif |
| `CORS_ALLOW_CREDENTIALS`  | `false`                          | |
| `APP_FRONTEND_URL`        | `http://localhost:3000`          | Basis tautan di email (reset password) |
| `COOKIE_SAMESITE`         | `Lax`                            | `Lax`/`Strict`/`None`. `None` untuk deploy lintas-domain |
| `COOKIE_SECURE`           | `true` di produksi               | Wajib `true` bila `COOKIE_SAMESITE=None` |
| `COOKIE_DOMAIN`           | kosong                           | Kosong = host-only. Lihat `docs/DEPLOYMENT.md` |
| `SMTP_HOST` / `SMTP_FROM` | —                                | Wajib di produksi agar reset password aktif |
| `PASSWORD_RESET_TTL`      | `3600`                           | Masa berlaku token reset (detik) |
| `SEED_ADMIN_EMAIL`        | `admin@example.com`              | Dipakai oleh seeder |
| `SEED_ADMIN_PASSWORD`     | `password-admin`                 | **Ganti dengan password kuat** |
| `SEED_SAMPLE_DATA`        | `false`                          | `true` untuk membuat tag & project contoh |

> `.env` di-gitignore dan tidak boleh di-commit. Gunakan `.env.example` sebagai template.

## Perintah Umum (Makefile)

```sh
make run             # jalankan API server
make dev             # live reload via air
make seed            # buat user admin + data contoh (idempotent)
make build           # build binary ke bin/portfolio-backend
make fmt             # go fmt
make vet             # go vet
make swagger         # regenerate dokumentasi Swagger
```

### Migrasi Database (Atlas)

Skema dikelola dengan **versioned SQL migrations** di `migrations/` — bukan GORM AutoMigrate.

```sh
make migrate-new name=add_something   # buat migrasi baru
make migrate-up                       # apply migrasi (env "dev")
make migrate-status                   # cek status
make migrate-down                     # rollback 1 langkah
make migrate-down-to version=xxxx     # rollback ke versi tertentu
```

Konfigurasi environment migrasi ada di [`atlas.hcl`](atlas.hcl) (`dev`, `staging`, `prod`, `docker-dev`).

### Testing

```sh
go test ./...                                              # semua test
go test ./internal/http/middleware/ -run TestAdminRoute -v # satu test
```

## Struktur Proyek

```
cmd/
  api/        # entry point HTTP server
  seed/       # seeder user admin & data contoh
internal/
  config/     # load & validasi konfigurasi env
  db/         # inisialisasi koneksi GORM
  http/
    router.go        # registrasi semua route
    middleware/      # AuthJWT, RequireRole, CORS, helmet, logger
    handlers/        # layer HTTP (parse, validasi, panggil repo)
    response/        # helper envelope respons
    error_handler.go # error handler terpusat
  repository/ # akses database (GORM) per resource
  models/     # struct GORM
  validation/ # wrapper validator
  helpers/    # util kecil (env, request id)
migrations/   # SQL migrasi Atlas
docs/         # output Swagger (generated)
```

Alur request: **handler** parse body → validasi → panggil **repository** (semua query DB di sini) → kembalikan JSON dalam envelope `{"data": ...}` atau `{"error": ...}`.

## API

Base path: `/api/v1`

### Publik

| Method | Endpoint            | Keterangan |
| ------ | ------------------- | ---------- |
| GET    | `/healthz`          | Liveness |
| GET    | `/readyz`           | Readiness (ping DB) |
| POST   | `/auth/login`       | Login admin (rate-limited 10/menit) |
| GET    | `/projects`         | List project |
| GET    | `/projects/:slug`   | Detail project |
| GET    | `/experiences`      | List experience |
| POST   | `/auth/forgot-password` | Kirim tautan reset password (rate-limited 5/menit) |
| POST   | `/auth/reset-password`  | Setel password baru dengan token dari email |
| POST   | `/contact`          | Kirim pesan kontak |

### Terautentikasi (JWT)

| Method | Endpoint            | Keterangan |
| ------ | ------------------- | ---------- |
| GET    | `/me`               | Info user saat ini |

### Admin (JWT + role `admin`)

Semua di bawah `/admin` — butuh `Authorization: Bearer <token>` dengan role `admin`:

- `/admin/dashboard/overview`
- `/admin/projects` (CRUD)
- `/admin/tags` (CRUD)
- `/admin/experiences` (CRUD)
- `/admin/contact-messages` (list, detail, mark-read, delete)

### Autentikasi

```sh
# Login → dapatkan token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"<password>"}'

# Pakai token di endpoint admin
curl http://localhost:8080/api/v1/admin/projects \
  -H "Authorization: Bearer <token>"
```

### Swagger

Saat `APP_ENV` ≠ `production`, UI Swagger tersedia di `http://localhost:8080/swagger/index.html`. Regenerate dengan `make swagger`.

## Keamanan

Beberapa kontrol yang sudah diterapkan:

- Route `/admin/*` wajib JWT **dan** role `admin` (`AuthJWT` + `RequireRole`).
- Password di-hash dengan **bcrypt**; field password tidak pernah dikembalikan ke JSON.
- Validasi konfigurasi saat startup — di `production`, `JWT_SECRET` lemah/default atau CORS `*`+credentials akan **menghentikan** aplikasi.
- Rate limiting pada endpoint login (anti brute-force) dan reset password (5/menit).
- Token reset password acak 256-bit, disimpan sebagai **hash**, sekali pakai, dan kedaluwarsa; `forgot-password` membalas seragam agar tidak bisa dipakai mengenumerasi email terdaftar.
- Security headers via `helmet`, batas body 1MB, query parameterized (anti SQL-injection).
- Swagger UI hanya aktif di luar produksi.

### Checklist Produksi

1. Set `APP_ENV=production`.
2. `JWT_SECRET` acak & kuat (`openssl rand -base64 48`).
3. Ganti `SEED_ADMIN_PASSWORD` dan kredensial database default.
4. `DB_DSN` pakai `sslmode=require`.
5. `CORS_ALLOWED_ORIGINS` di-set ke domain frontend yang spesifik.
6. Atribut cookie sesi disesuaikan dengan topologi deploy — lihat **[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)**.
7. SMTP diisi bila fitur reset password dipakai; tanpa itu endpointnya membalas `503` di produksi.
