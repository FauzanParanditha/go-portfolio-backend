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

## ADR-009 | Unified response envelope + /auth/refresh + /contact-messages alias | Accepted | 2026-06

**Konteks.** Envelope belum seragam: `POST /auth/login` & `GET /me` mengembalikan bare object, sementara endpoint lain `{ data, meta? }`. Frontend butuh pembacaan konsisten, paginasi yang gampang (`totalPages`), endpoint refresh token, dan penyelarasan path submit kontak. Sudah dikoordinasikan dengan agent frontend (lihat `docs/NOTES-FOR-*`).

**Keputusan.**
- **Envelope diseragamkan**: semua response sukses dibungkus `{ "data": <payload> }`; list menambahkan `meta`. `login` & `/me` ikut di-wrap (breaking change terkoordinasi). Envelope error tidak berubah.
- **`totalPages = ceil(total/limit)`** ditambahkan ke `meta` semua endpoint list (guard `limit<=0` → 0); `hasMore` tetap ada.
- **`POST /auth/refresh`** (baru): butuh `AuthJWT` (validasi token saat ini), terbitkan token baru (iat/exp segar, userId+role sama), bentuk response sama dengan login. **Stateless** — tanpa DB/refresh-token store, tanpa rate limiter login.
- **`POST /contact-messages`** ditambahkan sebagai **alias publik** dari `POST /contact` (handler sama).

**Konsekuensi.**
- Auth tetap **bearer JWT stateless** (menggeser usulan Opsi A sebagian). **HttpOnly-cookie auth ditunda** sebagai future enhancement — saat ini frontend tetap mengirim token via header `Authorization: Bearer`. Refresh tidak punya revocation/rotation; token lama valid sampai `exp`.
  - **Update:** "HttpOnly-cookie ditunda" ini **dicabut oleh ADR-010** — cookie auth kini diimplementasikan (sumber ganda: header + cookie).
- Frontend menyesuaikan pembacaan ke `res.data.data.*` untuk `login`/`me` serempak dengan rilis ini.

---

## ADR-010 | Auth via HttpOnly cookie (Set-Cookie) + /auth/logout | Accepted | 2026-06

**Konteks.** ADR-009 menunda "HttpOnly-cookie auth" (Opsi A) dan menyimpan token JWT di sisi browser via header `Authorization: Bearer` — artinya token harus disimpan di JS-accessible storage (mis. localStorage), yang rentan pencurian via XSS. Frontend ingin kredensial yang lebih aman tanpa membuang dukungan API client berbasis header. Dikoordinasikan dengan agent frontend (kontrak cookie identik diimplementasikan paralel).

**Keputusan.** Mengangkat Opsi A: **autentikasi via cookie HttpOnly `access_token`** dengan **sumber ganda** di middleware `AuthJWT`.
- **Middleware dual-source** (`internal/http/middleware/auth_jwt.go`): ambil token dari header `Authorization: Bearer` bila ada (prioritas, untuk API client & test), **ELSE** dari cookie `access_token` (`c.Cookies(...)`). Tidak ada keduanya → `401 "missing credentials"`. Logika validasi (HS256, enforce HMAC, secret & claims) tidak berubah.
- **Cookie** di-set pada `POST /auth/login` dan `POST /auth/refresh` (selain tetap mengembalikan token di body untuk klien non-browser): `HttpOnly; Path=/; SameSite=Lax; Max-Age=JWT_EXPIRES_IN`. Flag **`Secure` hanya di produksi** (`cfg.IsProduction()`) supaya tetap berfungsi di `http://localhost` saat dev.
- **`POST /auth/logout`** (baru, **publik/tanpa auth** agar user dengan token kedaluwarsa tetap bisa membersihkan cookie): set cookie kedaluwarsa (nilai kosong, `Max-Age` negatif / `Expires` di masa lalu). Response `200 { "data": { "message": "logged out" } }`.
- Helper kecil `setAuthCookie` / `clearAuthCookie` menjaga atribut konsisten.

**Konsekuensi.**
- **Mencabut** catatan "HttpOnly-cookie ditunda" di ADR-009. ADR-003 (JWT HS256 stateless) tetap berlaku — ini hanya menambah jalur kredensial cookie, bukan mengganti algoritma.
- **Mitigasi XSS**: token tidak lagi perlu disimpan di JS-accessible storage untuk browser; cookie HttpOnly tidak terbaca skrip.
- **Tidak ada revocation server-side**: logout hanya menghapus cookie di browser. Token JWT lama **tetap valid sampai `exp`** (siapa pun yang sempat menyalinnya masih bisa memakainya). Mitigasi tetap masa berlaku pendek (`JWT_EXPIRES_IN`).
- **CORS dengan credentials**: cross-origin cookie butuh `CORS_ALLOW_CREDENTIALS=true` + origin eksplisit (bukan `*`); `config.Validate()` melarang `*`+credentials. Browser harus pakai `withCredentials`.
- **Caveat SameSite=Lax**: cocok untuk dev (FE `localhost:3000` ↔ BE `localhost:8080` dianggap same-site oleh sebagian besar browser) dan deployment same-site. Untuk deployment **benar-benar cross-site** di produksi (FE & BE beda registrable domain), cookie perlu `SameSite=None; Secure` agar terkirim pada request cross-site — sesuaikan bila topologi berubah.

---

## ADR-011 | Token revocation saat logout via denylist jti (in-memory) | Accepted | 2026-06

**Konteks.** ADR-010 menambahkan `POST /auth/logout` tetapi hanya menghapus cookie browser; token JWT lama tetap valid sampai `exp` (tidak ada revocation server-side). Untuk logout yang benar-benar mematikan sesi, token aktif harus bisa dibatalkan sebelum kedaluwarsa.

**Keputusan.** Revocation berbasis **jti** + **denylist**.
- Setiap token yang diterbitkan (`issueToken` → login & refresh) kini menyertakan **`jti`** unik (`RegisteredClaims.ID`, UUID).
- Paket `internal/denylist` mendefinisikan interface `Denylist` (`Revoke(jti, exp)`, `IsRevoked(jti)`) dengan implementasi default **`Memory`** (in-memory, aman-konkuren, janitor pembersih entri lewat-exp tiap 10 menit). TTL entri = `exp` token.
- `AuthJWT` menerima `Denylist` dan **menolak `401 "token revoked"`** bila `jti` token ada di denylist. Satu instance dibuat di `NewRouter` dan dibagikan ke seluruh middleware + handler auth.
- `POST /auth/logout` mem-parse token saat ini (header/cookie, validasi penuh) lalu `Revoke(jti, exp)` — token langsung tidak berlaku. Logout tetap publik & idempotent (token tidak ada/kedaluwarsa cukup diabaikan).

**Konsekuensi.**
- **Logout kini instan** untuk single-instance: token yang dicabut langsung `401`, tidak menunggu `exp`. ADR-009/010 yang menyatakan "tidak ada revocation" diperbarui oleh ADR ini.
- **Keterbatasan in-memory (sadar)**: denylist hilang saat proses **restart** (token tercabut bisa valid lagi sampai exp-nya) dan **tidak dibagi antar-instance**. Memadai untuk deployment single-instance (portfolio). 
- **Jalur upgrade**: karena `Denylist` adalah interface, implementasi berbasis **DB/Redis** (durable + multi-instance) dapat di-drop-in tanpa mengubah middleware/handler — kandidat bila aplikasi diskalakan horizontal.
- Token lama (pra-perubahan) tanpa `jti` → `IsRevoked("")` selalu false → tetap valid sampai exp (transisi mulus).

---

## ADR-012 | Denylist token dipersisten ke PostgreSQL | Accepted | 2026-07

**Konteks.** ADR-011 memakai denylist in-memory dan sudah mencatat keterbatasannya: pencabutan hilang saat proses restart dan tidak dibagi antar-instance. Logout yang "batal" setelah deploy ulang bukan perilaku yang bisa diterima untuk sesi admin.

**Keputusan.** Menambah implementasi `denylist.Postgres` di belakang interface `Denylist` yang sudah ada (jalur upgrade yang memang diantisipasi ADR-011).
- Tabel `revoked_tokens (jti PK, exp, created_at)` lewat migration Atlas; `Revoke` memakai upsert sehingga idempoten.
- Janitor periodik menghapus baris yang sudah lewat `exp`.
- `IsRevoked` **fail-open** saat DB error/timeout: cek ini berjalan di jalur panas setiap request terautentikasi, sehingga fail-closed akan mengubah satu gangguan DB menjadi penolakan seluruh request auth (DoS). Timeout singkat dipasang agar DB yang hang tidak menahan request.

**Konsekuensi.**
- Pencabutan token bertahan lintas restart dan berlaku untuk semua instance yang berbagi database.
- Trade-off fail-open disengaja: saat DB bermasalah, token yang sudah dicabut bisa sesaat kembali diterima. Ketersediaan dipilih di atas kekakuan revocation.
- Catatan ADR-011 dan dokumen yang menyebut denylist "in-memory / hilang saat restart" diperbarui oleh ADR ini.

*(Entri ini didokumentasikan menyusul untuk implementasi yang sudah ada di kode.)*

---

## ADR-013 | Atribut cookie sesi dikonfigurasi lewat env (dukungan lintas-domain) | Accepted | 2026-09

**Konteks.** ADR-010 memasang cookie `access_token` dengan `SameSite=Lax` dan `Secure` yang dipatok ke `IsProduction()` — nilai tetap di kode. Caveat ADR-010 sendiri sudah menyebut deployment cross-site butuh `SameSite=None; Secure`, tapi tidak ada cara mengubahnya tanpa menyentuh kode. Rencana memisahkan FE dan BE ke domain berbeda membuat batas ini menghalangi.

**Keputusan.** Tiga atribut cookie diangkat ke konfigurasi: `COOKIE_DOMAIN`, `COOKIE_SAMESITE`, `COOKIE_SECURE`.
- Default menjaga perilaku lama: `SameSite=Lax`, domain kosong (host-only), `Secure` mengikuti `APP_ENV`.
- `setAuthCookie` dan `clearAuthCookie` memakai nilai yang **identik** — browser hanya menimpa cookie bila Domain/Path/SameSite/Secure cocok, jadi beda sedikit saja membuat logout gagal menghapus cookie.
- `cfg.Validate()` menolak kombinasi yang pasti gagal: `SameSite=None` tanpa `Secure`, `SameSite=None` tanpa `CORS_ALLOW_CREDENTIALS`, dan produksi tanpa `Secure`.
- Topologi deploy beserta matriks env didokumentasikan di `docs/DEPLOYMENT.md`.

**Konsekuensi.**
- Deployment same-site (termasuk **subdomain satu domain induk**, yang menurut browser tetap same-site) dan reverse-proxy satu-origin berjalan tanpa perubahan konfigurasi.
- Salah konfigurasi lintas-domain gagal saat **startup** dengan pesan jelas, bukan sebagai "login yang tidak menempel" di browser.
- Gate presence-check di frontend (`src/proxy.ts`) hanya bisa melihat cookie yang terkirim ke domain frontend. Untuk topologi di mana cookie tidak sampai ke sana, gate dimatikan lewat `ADMIN_PROXY_GATE=off`; keamanan tetap ditegakkan `RequireRole` di backend.
- `SameSite=None` bergantung pada cookie pihak ketiga yang diblokir Safari dan dipartisi Firefox — karena itu topologi beda-domain-induk didokumentasikan sebagai **tidak disarankan**, bukan sekadar didukung.

---

## ADR-014 | Reset password lewat token sekali pakai + email | Accepted | 2026-09

**Konteks.** Halaman login sudah menautkan "Forgot Password?" padahal tidak ada halaman maupun endpointnya. Satu-satunya cara memulihkan akun adalah menjalankan ulang seeder.

**Keputusan.** Alur dua endpoint publik: `POST /auth/forgot-password` dan `POST /auth/reset-password`.
- Token acak 256-bit dikirim lewat tautan email; **hanya SHA-256-nya** yang disimpan di tabel `password_reset_tokens`. SHA-256 (bukan bcrypt) memadai karena tokennya sendiri sudah tak bisa ditebak — key stretching tidak menambah apa pun.
- Sekali pakai + kedaluwarsa (`PASSWORD_RESET_TTL`, default 1 jam). Menerbitkan token baru membatalkan token lama milik user yang sama, dan reset sukses membatalkan sisanya.
- `forgot-password` **selalu** membalas `200` dengan pesan identik, termasuk saat email tak terdaftar atau pengiriman email gagal — membedakannya akan menjadikan endpoint ini oracle enumerasi user. Kegagalan kirim terlihat operator lewat log.
- Paket `internal/mailer` di belakang interface: SMTP (STARTTLS, atau TLS implisit di port 465) bila dikonfigurasi; di luar produksi jatuh ke penulisan isi email ke log; di produksi tanpa SMTP pengiriman **dinonaktifkan** dan endpoint membalas `503`.
- Rate limit lebih ketat dari login (5/menit) karena tiap request yang lolos mengirim email ke alamat pihak ketiga.

**Konsekuensi.**
- Pemulihan akun tidak lagi butuh akses shell ke server.
- **Reset tidak mencabut sesi lama**: denylist bekerja per-`jti`, sehingga JWT yang terbit sebelum reset tetap valid sampai `exp`. Mencabutnya butuh pelacakan per-user (mis. `password_changed_at` dibandingkan klaim `iat`) — belum dikerjakan dan dicatat di `docs/DEPLOYMENT.md`.
- Menolak pengiriman tanpa TLS berarti server SMTP tanpa STARTTLS tidak didukung; ini disengaja karena tautan reset setara kredensial sementara.
- Di produksi tanpa SMTP, fitur mati terang-terangan alih-alih menulis tautan reset ke log produksi.

---

## Template entri baru

```
## ADR-NNN | Judul | Proposed | YYYY-MM

**Konteks.** Masalah/kebutuhan yang melatarbelakangi.
**Keputusan.** Apa yang diputuskan.
**Konsekuensi.** Trade-off, dampak positif/negatif, hal yang harus dijaga.
```
