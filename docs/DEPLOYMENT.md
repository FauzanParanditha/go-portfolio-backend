# Deployment

Fokus dokumen ini: **cara men-deploy frontend dan backend di domain terpisah**
tanpa merusak sesi admin, plus konfigurasi environment yang menyertainya.

Backend memakai cookie **HttpOnly `access_token`** sebagai pembawa sesi browser.
Cookie adalah bagian yang paling gampang patah saat FE dan BE dipisah domain,
karena tiga atribut (`SameSite`, `Secure`, `Domain`) harus cocok dengan topologi
deploy. Ketiganya kini dikendalikan lewat env (`COOKIE_SAMESITE`,
`COOKIE_SECURE`, `COOKIE_DOMAIN`) — tidak ada lagi nilai yang dipatok di kode.

---

## Kunci yang sering disalahpahami: "beda domain" ada dua jenis

Browser menentukan *same-site* dari **registrable domain** (eTLD+1), **bukan**
dari origin. Konsekuensinya:

| FE | BE | Menurut browser | Cookie `SameSite=Lax` terkirim? |
| -- | -- | --------------- | ------------------------------- |
| `app.example.com` | `api.example.com` | **same-site** (beda origin) | ✅ ya |
| `example.com` | `api.example.com` | **same-site** | ✅ ya |
| `portfolio.com` | `api-portfolio.dev` | **cross-site** | ❌ tidak |
| `example.com` | `example.net` | **cross-site** | ❌ tidak |

Artinya: memisahkan FE dan BE ke **subdomain dari satu domain induk** sudah
cukup, dan itu **tidak** membutuhkan `SameSite=None`. Ini topologi yang
direkomendasikan — lihat Topologi A.

`SameSite=None` hanya perlu bila kedua domain induknya benar-benar berbeda
(Topologi C), dan itu pilihan yang paling rapuh karena bergantung pada
cookie pihak ketiga.

---

## Topologi A — subdomain satu domain induk (REKOMENDASI)

```
https://example.com          → frontend (Next.js)
https://api.example.com      → backend (Go/Fiber)
```

Same-site, jadi `SameSite=Lax` bekerja dan cookie tetap first-party.

**Backend (`.env`):**

```sh
APP_ENV=production
APP_FRONTEND_URL=https://example.com

CORS_ALLOWED_ORIGINS=https://example.com
CORS_ALLOW_CREDENTIALS=true

COOKIE_DOMAIN=            # kosongkan → host-only untuk api.example.com
COOKIE_SAMESITE=Lax
COOKIE_SECURE=true        # default di produksi; HTTPS wajib
```

**Frontend (`.env`):**

```sh
NEXT_PUBLIC_API_URL=https://api.example.com/api/v1
NEXT_PUBLIC_SITE_URL=https://example.com
```

**Catatan `COOKIE_DOMAIN`.** Biarkan kosong. Mengisinya `.example.com` membuat
cookie sesi ikut terkirim ke **semua** subdomain (termasuk yang tidak
berkepentingan) — memperluas permukaan kebocoran tanpa manfaat, karena
`SameSite=Lax` sudah cukup di topologi ini. Isi hanya jika ada kebutuhan nyata
beberapa subdomain berbagi satu sesi.

> ⚠️ **Batas gate proxy FE.** `src/proxy.ts` di frontend melakukan
> *presence-check* cookie `access_token` untuk mencegah kedip konten admin. Gate
> itu berjalan di server Next.js dan hanya bisa melihat cookie yang dikirim
> browser ke **domain frontend**. Di Topologi A cookie bersifat host-only milik
> `api.example.com`, sehingga **proxy tidak akan melihatnya** dan setiap
> kunjungan ke `/admin` akan dilempar ke halaman login.
>
> Pilih salah satu:
> - **Topologi B** (reverse proxy) — cookie jadi first-party di domain FE, gate berfungsi penuh. Paling rapi untuk cookie, **dengan syarat** nginx/Caddy di depan (lihat Topologi B).
> - Set `COOKIE_DOMAIN=.example.com` supaya cookie terkirim juga ke domain FE.
> - Nonaktifkan gate proxy dan andalkan `AdminGuard` (client-side) + penegakan backend. Aman secara keamanan — `RequireRole("admin")` tetap berjalan — hanya menambah kedip UI sesaat.
>
> Gate dimatikan dengan `ADMIN_PROXY_GATE=off` di repo frontend. Proxy Next.js
> berjalan di Edge runtime, jadi nilai env ini **di-inline saat build** — set
> sebelum `pnpm build`, bukan hanya saat runtime.

---

## Topologi B — satu origin lewat reverse proxy (paling sederhana untuk cookie, butuh nginx/Caddy di depan)

```
https://example.com          → frontend
https://example.com/api/*    → di-proxy ke backend
```

FE dan BE tampil sebagai **satu origin**. Tidak ada CORS, tidak ada isu
`SameSite`, tidak ada cookie pihak ketiga, dan gate `src/proxy.ts` berfungsi
penuh karena cookie-nya first-party.

> ⚠️ **Syarat wajib: nginx/Caddy di depan Next.js yang MENIMPA `X-Real-IP`.**
> Next.js 16 `rewrites` meneruskan header `X-Real-IP` dan `X-Forwarded-For`
> kiriman klien **apa adanya** ke backend. Bila server Next.js dipercaya lewat
> `TRUSTED_PROXIES` tanpa proxy lain di depannya, siapa pun cukup mengirim
> `X-Real-IP: <acak>` untuk mendapat jatah rate limit baru di setiap request
> (termasuk login 10/menit), sehingga limiter tidak berguna.
>
> Pilih salah satu, tidak ada jalan tengah:
>
> 1. **Ada nginx/Caddy di depan** yang menimpa header dengan alamat peer
>    (`proxy_set_header X-Real-IP $remote_addr;` atau Caddy
>    `header_up X-Real-IP {remote_host}`), dan Next.js tidak bisa dijangkau
>    langsung dari internet. Barulah isi `TRUSTED_PROXIES` dengan IP spesifik
>    server Next.js (sebagaimana dilihat backend).
> 2. **Tanpa proxy semacam itu:** biarkan `TRUSTED_PROXIES` **kosong**. Header
>    diabaikan dan backend memakai alamat koneksi, yaitu IP server Next.js.
>    Konsekuensinya diterima secara sadar: semua pengunjung **berbagi satu
>    jatah** untuk setiap limiter per-IP (login 10/menit, forgot 5, reset 10,
>    contact 5). Satu pengirim spam bisa menghabiskan jatah contact atau login
>    untuk semua orang, termasuk admin.
>
> Dalam kedua kasus, port backend **tidak boleh** dipublikasikan ke internet:
> bind ke `127.0.0.1` atau jaringan Docker internal saja (lihat README, bagian
> "Deploy di balik reverse proxy").

**Next.js (`next.config.ts`):**

```ts
async rewrites() {
  return [{ source: "/api/:path*", destination: "http://backend:8080/api/:path*" }];
}
```

**Frontend (`.env`):** `NEXT_PUBLIC_API_URL=/api/v1` — path relatif, bukan URL absolut.

**Backend (`.env`):**

```sh
APP_ENV=production
APP_FRONTEND_URL=https://example.com
CORS_ALLOWED_ORIGINS=https://example.com   # tetap diisi untuk jaga-jaga
CORS_ALLOW_CREDENTIALS=true
COOKIE_SAMESITE=Lax
COOKIE_SECURE=true
```

Pola yang sama berlaku bila reverse proxy-nya Nginx/Caddy/Traefik di depan
keduanya.

**IP klien untuk rate limit.** Di topologi ini backend melihat IP proxy, bukan
IP pengunjung. Rantai yang benar:

```
klien → nginx/Caddy (X-Real-IP = $remote_addr) → Next.js rewrites (meneruskan header) → backend
                                                  ↑ TRUSTED_PROXIES = IP server Next.js
```

Isi `TRUSTED_PROXIES` dengan **IP spesifik** hop terakhir sebelum backend
(server Next.js), bukan CIDR jaringan yang lebar. Detail & contoh: bagian
"Deploy di balik reverse proxy" di `README.md`.

---

## Topologi C — domain induk benar-benar berbeda

```
https://portfolio.com        → frontend
https://api-portfolio.dev    → backend
```

Cross-site. Cookie sesi menjadi **cookie pihak ketiga**, dan itu masalah nyata:

- **Safari (ITP)** memblokir cookie pihak ketiga secara default → sesi admin
  tidak akan pernah menempel.
- **Firefox (Total Cookie Protection)** mempartisinya.
- **Chrome** terus memperketat.

Jadi topologi ini **tidak disarankan**. Bila tetap dipakai, wajib:

```sh
APP_ENV=production
APP_FRONTEND_URL=https://portfolio.com

CORS_ALLOWED_ORIGINS=https://portfolio.com
CORS_ALLOW_CREDENTIALS=true

COOKIE_DOMAIN=
COOKIE_SAMESITE=None
COOKIE_SECURE=true        # WAJIB — browser membuang SameSite=None tanpa Secure
```

`cfg.Validate()` menolak start bila `COOKIE_SAMESITE=None` dipasangkan dengan
`COOKIE_SECURE=false` atau tanpa `CORS_ALLOW_CREDENTIALS=true`, sehingga salah
konfigurasi terlihat saat startup, bukan sebagai "login yang tidak menempel".

Gate `src/proxy.ts` juga tidak akan melihat cookie di topologi ini — matikan
gate-nya dan andalkan `AdminGuard` + penegakan backend.

---

## Matriks env cepat

| Env | Dev | A (subdomain) | B (reverse proxy) | C (beda domain) |
| --- | --- | ------------- | ----------------- | --------------- |
| `APP_ENV` | `development` | `production` | `production` | `production` |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | `https://example.com` | `https://example.com` | `https://portfolio.com` |
| `CORS_ALLOW_CREDENTIALS` | `true` | `true` | `true` | `true` |
| `COOKIE_SAMESITE` | `Lax` | `Lax` | `Lax` | `None` |
| `COOKIE_SECURE` | `false` | `true` | `true` | `true` |
| `COOKIE_DOMAIN` | kosong | kosong atau `.example.com` | kosong | kosong |
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080/api/v1` | `https://api.example.com/api/v1` | `/api/v1` | `https://api-portfolio.dev/api/v1` |

`CORS_ALLOWED_ORIGINS` **tidak boleh** `*` bila credentials aktif — ditolak saat
startup.

---

## Langkah deploy

1. **Siapkan env** sesuai tabel di atas. `JWT_SECRET` minimal 32 karakter:
   `openssl rand -base64 48`.
2. **Audit data URL proyek lama (WAJIB sebelum deploy versi dengan validasi
   URL http/https).** Lihat bagian [Audit URL proyek](#audit-url-proyek-sebelum-deploy).
3. **Jalankan migration:** `make migrate-up` (Atlas: hash lalu apply).
4. **Seed admin pertama:** `make seed` — idempoten, aman diulang.
5. **Build & jalankan:** `make build` lalu jalankan `bin/portfolio-backend`.
6. **Verifikasi** dengan langkah di bawah.

Swagger UI **tidak** dipasang saat `APP_ENV=production` — itu disengaja.

### Audit URL proyek sebelum deploy

Versi ini memperketat validasi URL proyek: `demoUrl`/`repoUrl` hanya
`http`/`https` dengan host; `coverImageUrl` dan screenshot boleh juga path
`/...`; `//host`, `data:`, `javascript:`, dan spasi di tepi ditolak (aturan
lengkap: `docs/IMPORT-PROYEK.md`, bagian "Aturan URL"). Data lama **tetap
tampil**, tetapi proyek yang URL-nya melanggar akan kena **`422` saat diedit**
di panel admin, karena form mengirim ulang semua field.

Jalankan audit read-only terhadap database produksi **sebelum** deploy:

```sh
psql "$DB_DSN" -f scripts/audit-project-urls.sql
```

Script hanya berisi `SELECT` di dalam transaksi `READ ONLY` (diakhiri
`ROLLBACK`), jadi aman dijalankan di produksi. Hasilnya:

1. daftar baris yang melanggar (tabel, id, slug, kolom, nilai, alasan);
2. ringkasan jumlah per kolom dan alasan.

Bila ada baris yang muncul, perbaiki URL-nya (lewat panel admin versi lama
sebelum deploy, atau `UPDATE` manual yang ditinjau) **sebelum** atau segera
setelah deploy. Nilai `javascript:`/`data:` di `demo_url`/`repo_url` adalah
indikasi stored XSS yang perlu diperiksa asal-usulnya, bukan sekadar dirapikan.

---

## Tolak request body terkompresi di reverse proxy

Fiber v2 mendekompresi body request (`Content-Encoding: gzip`/`br`/`deflate`,
termasuk berantai seperti `gzip, gzip`) **tanpa batas**, sementara batas body
1 MB hanya melihat ukuran yang masih terkompresi. ~200 KB gzip bisa mengembang
jadi ~200 MB di memori (*decompression bomb*) lewat endpoint publik seperti
`/auth/login` atau `/contact`.

Backend sudah menolaknya sendiri: setiap request dengan `Content-Encoding`
selain kosong/`identity` dibalas **415 `UNSUPPORTED_MEDIA_TYPE`** sebelum body
dibaca (`middleware.RejectContentEncoding`). Klien normal — browser, `fetch`,
unggahan multipart — tidak pernah mengirim header ini pada request.

Tetap disarankan reverse proxy **juga** menolaknya sebagai lapis kedua, supaya
request semacam itu tidak sampai ke proses Go sama sekali, dan supaya proxy
tidak dikonfigurasi untuk mendekompresi body atas nama backend (mis. modul
`gunzip` request milik proxy — jangan diaktifkan).

**Nginx** (taruh di blok `location /api/` yang SAMA dengan `proxy_set_header X-Real-IP`
dari bagian trusted proxy, jangan membuat blok kedua — nginx menolak `location` ganda):

```nginx
location /api/ {
    if ($http_content_encoding !~* "^(identity)?$") {
        return 415;
    }
    proxy_pass http://backend:8080;
}
```

**Caddy:**

```caddy
@compressedBody {
    header Content-Encoding *
    not header Content-Encoding identity
}
respond @compressedBody 415
```

Ini hanya menyangkut header **request**; kompresi **respons**
(`Accept-Encoding` → `Content-Encoding` pada respons) tidak terpengaruh dan
boleh tetap diaktifkan di proxy.

---

## Verifikasi cookie setelah deploy

```sh
curl -i -X POST https://api.example.com/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -H 'Origin: https://example.com' \
  -d '{"email":"admin@example.com","password":"..."}'
```

Periksa header `Set-Cookie`:

```
Set-Cookie: access_token=...; Path=/; HttpOnly; Secure; SameSite=Lax
```

Yang harus dipastikan:

- `HttpOnly` ada — JS tidak boleh bisa membaca token.
- `Secure` ada di produksi.
- `SameSite` sesuai topologi (`Lax` untuk A/B, `None` untuk C).
- `SameSite=None` **selalu** disertai `Secure`.

Cek juga header CORS-nya:

```
Access-Control-Allow-Origin: https://example.com
Access-Control-Allow-Credentials: true
```

`Allow-Origin` harus origin persis, bukan `*`.

---

## SMTP (reset password)

Fitur reset password mengirim email, jadi SMTP wajib diisi di produksi:

```sh
SMTP_HOST=smtp.example.com
SMTP_PORT=587            # 465 = TLS implisit; selain itu STARTTLS
SMTP_USERNAME=...
SMTP_PASSWORD=...
SMTP_FROM=no-reply@example.com
SMTP_FROM_NAME=Portfolio
APP_FRONTEND_URL=https://example.com   # basis tautan reset di email
```

Bila SMTP kosong:

- **di luar produksi** — isi email ditulis ke log supaya developer bisa menyalin
  tautan reset tanpa menyiapkan SMTP;
- **di produksi** — pengiriman dinonaktifkan dan `POST /auth/forgot-password`
  membalas `503`. Ini disengaja: tautan reset setara kredensial sementara dan
  tidak boleh mendarat di log produksi.

Pengiriman tanpa enkripsi tidak didukung — bila server tidak menawarkan
STARTTLS, pengiriman ditolak.

---

## Yang belum ditangani

- **Reset password tidak mencabut sesi lama.** Denylist token berbasis `jti` per
  token, sehingga JWT yang sudah terbit sebelum reset tetap berlaku sampai
  `exp`. Untuk mencabutnya butuh pelacakan token per user (mis. kolom
  `password_changed_at` yang dibandingkan dengan klaim `iat`).
- **Rate limit bersifat per-instance** (in-memory). Di belakang beberapa replika,
  batas efektifnya berlipat sejumlah replika. Untuk penegakan yang benar,
  limiter perlu backing store bersama (mis. Redis).
