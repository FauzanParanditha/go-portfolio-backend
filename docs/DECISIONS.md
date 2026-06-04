# Architecture Decision Records (ADR)

Catatan keputusan arsitektur & teknis untuk Portfolio Backend. Setiap entri menjelaskan **konteks**, **keputusan**, dan **konsekuensi** — supaya keputusan sadar tidak "diperbaiki" tanpa sengaja di kemudian hari.

Format: `ADR-NNN | Judul | Status | Tanggal`. Status: `Accepted`, `Superseded`, `Deprecated`.

---

## ADR-001 | Fiber sebagai web framework | Accepted | 2025-11

**Konteks.** Butuh HTTP framework Go yang ringan, cepat, dan ergonomis untuk REST API.

**Keputusan.** Pakai [Fiber v2](https://gofiber.io) (di atas fasthttp).

**Konsekuensi.**
- Performa tinggi, API mirip Express.
- fasthttp **bukan** `net/http` — beberapa library berbasis `net/http` tidak kompatibel langsung; gunakan middleware dari ekosistem Fiber.
- Context request adalah `*fiber.Ctx` (di-reuse antar request — jangan simpan referensinya melewati lifecycle handler).

---

## ADR-002 | GORM untuk runtime, Atlas untuk migrasi | Accepted | 2025-11

**Konteks.** Butuh akses DB yang produktif sekaligus skema yang terversioning dan auditable.

**Keputusan.** GORM dipakai **hanya** sebagai query/ORM layer saat runtime. Skema dikelola oleh **Atlas** lewat versioned SQL di `migrations/`. **GORM AutoMigrate tidak dipakai.**

**Konsekuensi.**
- Perubahan skema **wajib** lewat `make migrate-new` → SQL eksplisit, bukan inferensi dari struct.
- Struct model dan migrasi harus dijaga konsisten secara manual.
- Konfigurasi environment migrasi ada di `atlas.hcl` (`dev`/`staging`/`prod`/`docker-dev`).

---

## ADR-003 | JWT HS256 stateless untuk autentikasi | Accepted | 2025-11

**Konteks.** Aplikasi single-admin/portfolio; butuh auth sederhana tanpa infrastruktur session.

**Keputusan.** JWT **HS256** (symmetric, satu `JWT_SECRET`). Claims menyimpan `userId` dan `role`. Tidak ada refresh token maupun token revocation.

**Konsekuensi.**
- Sederhana, stateless, tanpa store session.
- Karena symmetric, **siapa pun yang tahu `JWT_SECRET` bisa memalsukan token** → secret wajib kuat & rahasia (lihat ADR-007).
- Logout/revocation tidak instan: token valid sampai `exp`. Mitigasi: masa berlaku pendek (`JWT_EXPIRES_IN`, default 1 jam).
- Middleware memaksa signing method HMAC untuk mencegah serangan `alg=none`/algoritma campur.
- **Jika nanti perlu multi-service atau rotasi kunci**, pertimbangkan migrasi ke RS256 (asymmetric) — itu akan men-*supersede* ADR ini.

---

## ADR-004 | Arsitektur berlapis: handler → repository → models | Accepted | 2025-11

**Konteks.** Memisahkan logika HTTP, akses data, dan domain agar mudah diuji dan dipelihara.

**Keputusan.** Tiga lapis:
- `handlers/` — parsing, validasi, shaping response. Tidak menyentuh SQL langsung.
- `repository/` — **semua** akses DB (interface + implementasi GORM per resource), menerima `context.Context`.
- `models/` — struct GORM.

**Konsekuensi.**
- Repository adalah **satu-satunya batas SQL** → titik tunggal untuk jaminan query parameterized (anti SQL-injection).
- Catatan: sebagian handler admin (mis. `admin_project_handler.go`) masih query GORM langsung, bukan lewat repository. Ini hutang teknis yang diterima sementara; idealnya dipindah ke repository agar konsisten.

---

## ADR-005 | Envelope response & error code terpusat | Accepted | 2025-12

**Konteks.** Konsistensi format respons untuk konsumsi frontend.

**Keputusan.**
- Sukses: `{"data": ...}` (plus `"meta"` untuk list berpaginasi).
- Error: `{"error": {"message", "code", "details"}, "requestId"}` dirender oleh error handler terpusat (`internal/http/error_handler.go`).
- `code` diturunkan dari HTTP status via `errorCodeForStatus` (mis. 401→`UNAUTHORIZED`, 422→`VALIDATION_ERROR`).

**Konsekuensi.**
- Handler cukup `return fiber.NewError(status, msg)`; jangan menyusun JSON error manual.
- Field `requestId` (dari middleware `requestid`) memudahkan korelasi log ↔ respons.

---

## ADR-006 | Validasi via struct tags + DTO terpisah | Accepted | 2025-11

**Konteks.** Input eksternal harus tervalidasi sebelum menyentuh domain/DB.

**Keputusan.** Pakai `go-playground/validator/v10`. Tiap handler punya DTO request (`*_dto.go`) dengan tag `validate:`; handler memanggil `validation.ValidateStruct`, lalu memetakan DTO → model.

**Konsekuensi.**
- Pemisahan jelas antara payload wire dan model DB.
- Error validasi otomatis menjadi `422` dengan map `details` per field (`validation.ToFieldErrors`).

---

## ADR-007 | Hardening keamanan & fail-fast config | Accepted | 2026-06

**Konteks.** Audit menemukan: role admin tidak diperiksa, `JWT_SECRET` default lemah, tidak ada rate limit/headers, Swagger terbuka di produksi.

**Keputusan.**
- Route `/admin/*` = `AuthJWT` + `RequireRole("admin")`.
- `cfg.Validate()` dipanggil saat startup; di `production` config tidak aman (secret lemah/default, CORS `*`+credentials) bersifat **fatal**, di non-produksi hanya warning.
- Rate limit login (10/menit), security headers (`helmet`), `BodyLimit` 1MB, Swagger hanya non-produksi.

**Konsekuensi.**
- Deploy produksi yang misconfigured akan gagal start (sengaja) — mencegah rilis rentan.
- Detail lengkap & invarian ada di [`SECURITY.md`](../SECURITY.md). Jangan regres tanpa meng-update ADR ini.

---

## ADR-008 | Logging terstruktur dengan zerolog | Accepted | 2025-11

**Konteks.** Butuh log yang machine-parseable dan punya korelasi request.

**Keputusan.** Pakai `zerolog`. Middleware mencatat tiap request (method, path, status, latency, ip, request_id).

**Konsekuensi.**
- Log JSON-terstruktur siap dikirim ke agregator.
- **Hindari mencatat data sensitif** (password, token penuh). Catatan: seeder saat ini mencetak password admin awal ke log — hanya untuk kemudahan dev, jangan diandalkan di produksi.

---

## Template entri baru

```
## ADR-NNN | Judul | Proposed | YYYY-MM

**Konteks.** Masalah/kebutuhan yang melatarbelakangi.
**Keputusan.** Apa yang diputuskan.
**Konsekuensi.** Trade-off, dampak positif/negatif, hal yang harus dijaga.
```
