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
> - **Topologi B** (reverse proxy) — cookie jadi first-party di domain FE, gate berfungsi penuh. Paling rapi.
> - Set `COOKIE_DOMAIN=.example.com` supaya cookie terkirim juga ke domain FE.
> - Nonaktifkan gate proxy dan andalkan `AdminGuard` (client-side) + penegakan backend. Aman secara keamanan — `RequireRole("admin")` tetap berjalan — hanya menambah kedip UI sesaat.
>
> Gate dimatikan dengan `ADMIN_PROXY_GATE=off` di repo frontend. Proxy Next.js
> berjalan di Edge runtime, jadi nilai env ini **di-inline saat build** — set
> sebelum `pnpm build`, bukan hanya saat runtime.

---

## Topologi B — satu origin lewat reverse proxy (PALING TAHAN BANTING)

```
https://example.com          → frontend
https://example.com/api/*    → di-proxy ke backend
```

FE dan BE tampil sebagai **satu origin**. Tidak ada CORS, tidak ada isu
`SameSite`, tidak ada cookie pihak ketiga, dan gate `src/proxy.ts` berfungsi
penuh karena cookie-nya first-party.

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
2. **Jalankan migration:** `make migrate-up` (Atlas: hash lalu apply).
3. **Seed admin pertama:** `make seed` — idempoten, aman diulang.
4. **Build & jalankan:** `make build` lalu jalankan `bin/portfolio-backend`.
5. **Verifikasi** dengan langkah di bawah.

Swagger UI **tidak** dipasang saat `APP_ENV=production` — itu disengaja.

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
