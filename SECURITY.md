# Security

Dokumen ini merangkum kontrol keamanan yang berlaku di Portfolio Backend, **invarian yang tidak boleh diregres**, dan panduan pelaporan kerentanan. Konteks keputusan ada di [`docs/DECISIONS.md`](docs/DECISIONS.md) (ADR-003, ADR-007, ADR-010).

## Melaporkan Kerentanan

Jangan buka issue publik untuk kerentanan. Laporkan secara privat ke **teknis@pandi.id** dengan langkah reproduksi, dampak, dan versi/commit yang terdampak.

---

## Invarian Keamanan (JANGAN DIREGRES)

Aturan berikut sudah ditegakkan dan diverifikasi. Setiap perubahan yang melanggarnya harus dianggap regresi:

1. **Otorisasi admin** — setiap grup route `/api/v1/admin/*` **wajib** memasang `AuthJWT` lalu `RequireRole("admin")`. `AuthJWT` saja tidak cukup: ia hanya memverifikasi token valid, bukan role. Saat menambah resource admin baru, salin pola di `internal/http/router.go` persis.
2. **JWT secret kuat** — `JWT_SECRET` ≥ 32 karakter dan bukan nilai default. `config.Validate()` menolaknya saat startup (fatal di `production`).
3. **Rate limit login** — `POST /auth/login` dibatasi 10 req/menit (anti brute-force).
4. **Password hashing** — bcrypt untuk simpan & verifikasi. Field `Password` model `User` adalah `json:"-"` dan tidak boleh terekspos di response mana pun.
5. **Query parameterized** — semua akses DB lewat GORM dengan placeholder (`Where("col = ?", v)` / `ILIKE ?`). Dilarang string-concat ke query (anti SQL-injection).
6. **Security headers** — `helmet` terpasang di middleware global.
7. **Body limit** — Fiber `BodyLimit` 1 MB.
8. **Swagger** — UI hanya di-mount saat `APP_ENV` ≠ `production`.
9. **CORS** — origin dari env; `*` dilarang berbarengan dengan credentials (ditolak `config.Validate()`).
10. **Secrets** — `.env` di-gitignore dan tidak boleh di-commit; gunakan `.env.example` sebagai template.
11. **Cookie auth HttpOnly** — kredensial browser disimpan di cookie `access_token` dengan flag **`HttpOnly`** (tidak terbaca JS) dan `SameSite=Lax`; flag `Secure` wajib aktif di produksi (otomatis via `cfg.IsProduction()`). Jangan men-set cookie token tanpa `HttpOnly`, dan jangan mengekspos JWT ke storage JS-accessible.

---

## Model Ancaman & Mitigasi

| Ancaman | Mitigasi |
| ------- | -------- |
| Brute-force / credential stuffing login | Rate limiter 10/menit; bcrypt (lambat by design) |
| Forge JWT (alg confusion / `alg=none`) | Middleware memaksa signing method HMAC; secret kuat (HS256) |
| Pencurian token via XSS | Kredensial browser di cookie `HttpOnly` (tidak terbaca JS); `Secure` di produksi. API client tetap boleh kirim via header `Authorization: Bearer` |
| Privilege escalation ke endpoint admin | `RequireRole("admin")` pada semua route admin |
| SQL injection | GORM parameterized di seluruh repository/handler |
| User enumeration via login | Pesan error generik `invalid credentials` untuk semua kegagalan |
| Clickjacking / MIME sniffing | Header dari `helmet` (`X-Frame-Options`, `X-Content-Type-Options`, dll) |
| Resource exhaustion (payload besar) | `BodyLimit` 1 MB |
| Kebocoran permukaan API | Swagger dimatikan di produksi |
| Misconfiguration saat deploy | `config.Validate()` fatal di produksi |
| Kebocoran data sensitif di response | `Password` `json:"-"`; DTO response eksplisit |

---

## Komponen Keamanan (peta kode)

| Area | Lokasi |
| ---- | ------ |
| Verifikasi JWT (sumber ganda: header Authorization **atau** cookie `access_token`) | `internal/http/middleware/auth_jwt.go` |
| Cek role | `internal/http/middleware/require_role.go` |
| Middleware global (helmet, CORS, recover, logger) | `internal/http/middleware/middleware.go` |
| Rate limit login | `internal/http/router.go` (`registerAuthRoutes`) |
| Validasi config & fail-fast | `internal/config/config.go` (`Validate`, `IsProduction`) |
| Hashing, login, logout & set/clear cookie HttpOnly | `internal/http/handlers/auth_handler.go` (`setAuthCookie`/`clearAuthCookie`, `Logout`) |
| Error handler terpusat | `internal/http/error_handler.go` |

Test verifikasi otorisasi: `internal/http/middleware/require_role_test.go` (memastikan non-admin → 403, token salah-secret → 401). Jalankan:

```sh
go test ./internal/http/middleware/ -run TestAdminRoute -v
```

---

## Konfigurasi Aman per Environment

`config.Validate()` dipanggil di `cmd/api/main.go`:
- **production** (`APP_ENV=production`): konfigurasi tidak aman → **aplikasi gagal start** (fatal).
- **non-production**: hanya log warning (developer experience tetap lancar).

Yang divalidasi: `JWT_SECRET` (tidak kosong, bukan default, ≥32 char) dan kombinasi CORS `*` + credentials.

---

## Checklist Pengerasan Produksi

- [ ] `APP_ENV=production`.
- [ ] `JWT_SECRET` acak ≥32 char (`openssl rand -base64 48`), disimpan di secret manager — bukan di repo.
- [ ] Ganti `SEED_ADMIN_PASSWORD` default dan kredensial DB default (`postgres:postgres` / `postgres:root`).
- [ ] `DB_DSN` memakai `sslmode=require`.
- [ ] `CORS_ALLOWED_ORIGINS` di-set ke domain frontend spesifik (bukan `*`); set `CORS_ALLOW_CREDENTIALS` sesuai kebutuhan.
- [ ] Pastikan Swagger tidak ter-mount (otomatis off di produksi).
- [ ] Jalankan di belakang reverse proxy/TLS (HSTS efektif hanya via HTTPS).

---

## Hutang Keamanan / Peningkatan ke Depan

Belum ditangani, kandidat perbaikan berikutnya:

- **Refresh token & revocation** — saat ini logout tidak instan; token valid sampai `exp`. Pertimbangkan refresh token + denylist, atau pindah ke RS256 (lihat ADR-003).
- **Seeder mencetak password admin ke log** (`cmd/seed/main.go`) — hanya untuk dev; hilangkan/lindungi di produksi.
- **Account lockout** — rate limit saat ini per-IP global pada login; belum ada lockout per-akun.
- **Audit dependency** — jadwalkan `govulncheck ./...` secara berkala (belum terpasang di CI).
