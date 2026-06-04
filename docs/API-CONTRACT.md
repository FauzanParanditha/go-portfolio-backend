# API Contract

> ⚠️ **File kontrak bersama (Backend ⇄ Frontend).** Jangan diubah sepihak oleh satu agent. Usulkan perubahan dulu lewat `docs/NOTES-FOR-FRONTEND.md` / `docs/NOTES-FOR-BACKEND.md`, sepakati, baru update di sini (manual oleh developer yang menjaga). Jika schema ikut berubah, regenerate migration setelahnya.

Kontrak tingkat tinggi untuk Portfolio Backend API. Untuk skema request/response per-field yang detail, lihat **Swagger** (`/swagger/index.html` saat non-produksi; sumber di `docs/swagger.yaml`). Dokumen ini menjelaskan **konvensi lintas-endpoint** yang tidak terekam jelas di Swagger.

- **Base URL:** `http://localhost:8080`
- **Base path:** `/api/v1`
- **Content-Type:** `application/json`

---

## Format Response

### Sukses

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
    "hasMore": true,
    "q": "keyword",
    "featured": false
  }
}
```

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

Skema **Bearer JWT**. Dapatkan token via login, lalu kirim di header:

```
Authorization: Bearer <token>
```

| Hal             | Nilai |
| --------------- | ----- |
| Algoritma       | HS256 |
| Masa berlaku    | `JWT_EXPIRES_IN` detik (default 3600) |
| Claims          | `userId`, `role`, `sub`, `exp`, `iat` |
| Token type      | `Bearer` |

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

| Method | Path          | Auth | Keterangan |
| ------ | ------------- | ---- | ---------- |
| POST   | `/auth/login` | —    | Login admin. **Rate-limited 10 req/menit** (`429` jika terlampaui) |
| GET    | `/me`         | JWT  | Info user saat ini |

`POST /auth/login`
```json
// request
{ "email": "admin@example.com", "password": "secret" }
// 200
{ "token": "<jwt>", "tokenType": "Bearer", "expiresIn": 3600 }
// 401 → { "error": { "message": "invalid credentials", ... } }
```

> Pesan error login sengaja generik (`invalid credentials`) untuk email salah maupun password salah — mencegah enumerasi user.

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

| Method | Path       | Keterangan |
| ------ | ---------- | ---------- |
| POST   | `/contact` | Kirim pesan kontak |

`POST /contact`
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
