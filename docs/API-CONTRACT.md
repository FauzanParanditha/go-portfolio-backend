# API Contract

> ⚠️ **File kontrak bersama (Backend ⇄ Frontend).** Jangan diubah sepihak oleh satu agent. Usulkan perubahan dulu lewat `docs/NOTES-FOR-FRONTEND.md` / `docs/NOTES-FOR-BACKEND.md`, sepakati, baru update di sini (manual oleh developer yang menjaga). Jika schema ikut berubah, regenerate migration setelahnya.

Kontrak tingkat tinggi untuk Portfolio Backend API. Untuk skema request/response per-field yang detail, lihat **Swagger** (`/swagger/index.html` saat non-produksi; sumber di `docs/swagger.yaml`). Dokumen ini menjelaskan **konvensi lintas-endpoint** yang tidak terekam jelas di Swagger.

- **Base URL:** `http://localhost:8080`
- **Base path:** `/api/v1`
- **Content-Type:** `application/json`

---

## Format Response

### Sukses

Envelope **diseragamkan**: **setiap** response sukses dibungkus `{ "data": <payload> }` — termasuk `POST /auth/login` dan `GET /me` (sebelumnya bare object, kini di-wrap).

Objek tunggal atau aksi:

```json
{ "data": { "...": "..." } }
```

List berpaginasi menambahkan `meta`:

```json
{
  "data": [ { "...": "..." } ],
  "meta": {
    "page": 1,
    "limit": 12,
    "total": 42,
    "totalPages": 4,
    "hasMore": true,
    "q": "keyword",
    "featured": false
  }
}
```

> `totalPages` = `ceil(total / limit)` (0 jika `limit <= 0`). Ada di **semua** endpoint list (projects publik & admin, admin experiences, admin tags, admin contact-messages). `hasMore` tetap dipertahankan.

### Error

```json
{
  "error": {
    "message": "human readable message",
    "code": "MACHINE_CODE",
    "details": null
  },
  "requestId": "uuid-untuk-korelasi-log"
}
```

> `requestId` selalu disertakan pada error (dari header `X-Request-Id` atau di-generate). Sertakan saat melaporkan bug.

#### Kode error (`error.code`)

Diturunkan dari HTTP status secara terpusat:

| HTTP | `code`              |
| ---- | ------------------- |
| 400  | `BAD_REQUEST`       |
| 401  | `UNAUTHORIZED`      |
| 403  | `FORBIDDEN`         |
| 404  | `NOT_FOUND`         |
| 405  | `METHOD_NOT_ALLOWED`|
| 409  | `CONFLICT`          |
| 422  | `VALIDATION_ERROR`  |
| 429  | `TOO_MANY_REQUESTS` |
| 4xx lain | `CLIENT_ERROR`  |
| 5xx  | `SERVER_ERROR`      |

#### Error validasi (422)

`details` berisi map `field → pesan`:

```json
{
  "error": {
    "message": "validation failed",
    "code": "VALIDATION_ERROR",
    "details": { "email": "format email tidak valid", "password": "wajib diisi" }
  }
}
```

---

## Autentikasi

Skema **JWT (HS256)** dengan **dua sumber kredensial** — middleware `AuthJWT` menerima token dari salah satu, dengan urutan prioritas:

1. **Header** `Authorization: Bearer <token>` (untuk API client non-browser & test).
2. **Cookie HttpOnly** `access_token` (untuk klien browser) — dipakai jika header tidak ada.

Jika keduanya tidak ada → `401` (`missing credentials`).

```
Authorization: Bearer <token>
```

| Hal             | Nilai |
| --------------- | ----- |
| Algoritma       | HS256 |
| Masa berlaku    | `JWT_EXPIRES_IN` detik (default 3600) |
| Claims          | `userId`, `role`, `sub`, `exp`, `iat` |
| Token type      | `Bearer` |

### Cookie HttpOnly (klien browser)

`POST /auth/login` dan `POST /auth/refresh` selain mengembalikan token di body **juga men-set** cookie `access_token`:

| Atribut    | Nilai |
| ---------- | ----- |
| Name       | `access_token` |
| Value      | JWT yang ditandatangani |
| HttpOnly   | ya (tidak bisa dibaca JS → mitigasi pencurian token via XSS) |
| Path       | `/` |
| SameSite   | `Lax` |
| Max-Age    | `JWT_EXPIRES_IN` detik |
| Secure     | **hanya di produksi** (`APP_ENV=production`) — agar tetap jalan di `http://localhost` saat dev |

- **Browser**: pakai cookie (mengabaikan token di body). Wajib `withCredentials: true` (axios) / `credentials: 'include'` (fetch) di semua request agar cookie ikut terkirim. CORS server harus `AllowCredentials=true` dengan origin eksplisit (bukan `*`).
- **API client non-browser**: abaikan cookie, simpan `data.token`, kirim via header `Authorization: Bearer`.

**Tingkat akses:**
- **Publik** — tanpa token.
- **Authenticated** — token valid (role apa pun). Contoh: `GET /me`.
- **Admin** — token valid **dan** `role == "admin"`. Semua `/admin/*`.

Respons auth yang relevan:
- `401 UNAUTHORIZED` — header hilang/format salah, token invalid/kedaluwarsa/signature salah.
- `403 FORBIDDEN` — token valid tetapi role tidak mencukupi.

---

## Pagination & Query

Endpoint list menerima query params:

| Param      | Tipe   | Default | Catatan |
| ---------- | ------ | ------- | ------- |
| `page`     | int    | `1`     | < 1 dinormalisasi ke 1 |
| `limit`    | int    | `12`    | maksimum **100** (di-clamp) |
| `q`        | string | —       | pencarian (case-insensitive, `ILIKE`) |
| `featured` | bool   | `false` | filter `featured` (projects) |

---

## Endpoints

### Health

| Method | Path       | Auth | Keterangan |
| ------ | ---------- | ---- | ---------- |
| GET    | `/healthz` | —    | Liveness, selalu `{"status":"ok"}` |
| GET    | `/readyz`  | —    | Readiness, ping DB (`503` jika DB unreachable) |

> Health endpoints berada di root, **bukan** di bawah `/api/v1`.

### Auth

| Method | Path            | Auth | Keterangan |
| ------ | --------------- | ---- | ---------- |
| POST   | `/auth/login`   | —    | Login admin. **Rate-limited 10 req/menit** (`429` jika terlampaui). Set cookie `access_token` + body token |
| POST   | `/auth/refresh` | JWT (header **atau** cookie) | Terbitkan token baru dari token valid yang sedang dipakai; set ulang cookie `access_token`. **Tidak** rate-limited. Stateless (tanpa DB/refresh-token store) |
| POST   | `/auth/logout`  | —    | **Publik.** Hapus cookie `access_token` DAN **cabut token saat ini** (via `jti` → denylist) sehingga langsung tidak berlaku. Kirim token saat ini (header/cookie) agar tercabut |
| GET    | `/me`           | JWT (header **atau** cookie) | Info user saat ini |

`POST /auth/login`
```json
// request
{ "email": "admin@example.com", "password": "secret" }
// 200 — payload di-wrap dalam "data"; server JUGA mengirim header
//   Set-Cookie: access_token=<jwt>; Path=/; HttpOnly; SameSite=Lax; Max-Age=3600  (+ Secure di produksi)
{ "data": { "token": "<jwt>", "tokenType": "Bearer", "expiresIn": 3600 } }
// 401 → { "error": { "message": "invalid credentials", ... } }
```

`POST /auth/logout`
```json
// request — tanpa body; sertakan token saat ini (header Authorization atau cookie)
//   agar token tsb DICABUT (jti masuk denylist), bukan sekadar hapus cookie
// 200 — Set-Cookie mengosongkan access_token (Max-Age=0); token saat ini langsung tidak berlaku
{ "data": { "message": "logged out" } }
```

> Revocation memakai `jti` + denylist in-memory (ADR-011): setelah logout, token yang sama → `401 "token revoked"`. Catatan: denylist hilang saat server restart & tidak dibagi antar-instance (single-instance OK; multi-instance perlu denylist DB/Redis).

> Pesan error login sengaja generik (`invalid credentials`) untuk email salah maupun password salah — mencegah enumerasi user.

`POST /auth/refresh`
```json
// request — tanpa body; kirim token saat ini via header Authorization: Bearer <token>
//   ATAU cookie access_token (browser otomatis mengirimnya bila withCredentials)
// 200 — bentuk SAMA dengan login (token baru: iat/exp segar, userId+role sama); set ulang cookie access_token
{ "data": { "token": "<jwt>", "tokenType": "Bearer", "expiresIn": 3600 } }
// 401 → token hilang/invalid/kedaluwarsa
```

> Refresh memakai middleware `AuthJWT` yang sama (validasi HS256, enforce signing method, secret yang sama) dan menerbitkan token baru dengan `jti` baru. Catatan: refresh **tidak** otomatis mencabut token lama — token sebelumnya tetap valid sampai `exp` atau sampai `logout` mencabutnya.

### Projects (publik)

| Method | Path               | Keterangan |
| ------ | ------------------ | ---------- |
| GET    | `/projects`        | List (mendukung `page`, `limit`, `q`, `featured`) |
| GET    | `/projects/:slug`  | Detail by slug (`404` jika tidak ada) |

### Experiences (publik)

| Method | Path            | Keterangan |
| ------ | --------------- | ---------- |
| GET    | `/experiences`  | List experience |

### Contact (publik)

| Method | Path                | Keterangan |
| ------ | ------------------- | ---------- |
| POST   | `/contact`          | Kirim pesan kontak |
| POST   | `/contact-messages` | **Alias** dari `/contact` (handler sama) untuk konsistensi penamaan dengan `admin/contact-messages` |

`POST /contact` (dan alias `/contact-messages`)
```json
// request — semua field required, email tervalidasi
{ "name": "Jane", "email": "jane@x.com", "subject": "Halo", "message": "..." }
// 201
{ "data": { "id": "<uuid>", "message": "message received" } }
```

### Admin (JWT + role `admin`)

Semua di bawah `/admin`. Memerlukan header Bearer dengan role `admin`.

| Resource          | Endpoints |
| ----------------- | --------- |
| Dashboard         | `GET /admin/dashboard/overview` |
| Projects          | `GET /admin/projects` · `GET /admin/projects/:id` · `POST /admin/projects` · `PUT /admin/projects/:id` · `DELETE /admin/projects/:id` |
| Tags              | `GET /admin/tags` · `GET /admin/tags/:id` · `POST /admin/tags` · `PUT /admin/tags/:id` · `DELETE /admin/tags/:id` |
| Experiences       | `GET /admin/experiences` · `GET /admin/experiences/:id` · `POST /admin/experiences` · `PUT /admin/experiences/:id` · `DELETE /admin/experiences/:id` |
| Contact messages  | `GET /admin/contact-messages` · `GET /admin/contact-messages/:id` · `PATCH /admin/contact-messages/:id/read` · `DELETE /admin/contact-messages/:id` |

---

## Konvensi tipe data

- **ID**: UUID string (mis. `27fc6c5d-bad6-4df9-bf9c-c77db351b5d8`).
- **Timestamp**: RFC 3339 / ISO 8601 (mis. `2026-06-04T10:22:15+07:00`).
- **Field JSON**: `camelCase` (mis. `createdAt`, `isRead`, `coverImageUrl`).
- **Password**: tidak pernah dikembalikan dalam response apa pun.

---

## CORS

Dikontrol via env (`CORS_ALLOWED_ORIGINS`, `CORS_ALLOWED_METHODS`, `CORS_ALLOWED_HEADERS`, `CORS_ALLOW_CREDENTIALS`). Default origin `http://localhost:3000`. Origin `*` **tidak boleh** dikombinasikan dengan credentials (ditolak saat startup).

## Batas & Rate Limit

- **Body limit:** 1 MB per request (`413` jika melebihi).
- **Login:** 10 request/menit per IP (`429 TOO_MANY_REQUESTS`).
