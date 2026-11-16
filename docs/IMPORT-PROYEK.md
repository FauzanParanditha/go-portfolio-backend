# Format Data Impor Proyek

Cara memasukkan proyek ke portfolio **tanpa panel admin** — cocok untuk data yang
digenerate agen lain, lalu dimuat sekali jalan.

```sh
make import file=docs/contoh-import-proyek.json args=-dry   # pratinjau, tidak menulis
make import file=docs/contoh-import-proyek.json             # muat ke database
```

Butuh `.env` terisi (`DB_DSN`) dan migrasi sudah dijalankan (`make migrate-up`).

---

## Tiga sifat yang perlu diketahui

**Upsert berdasarkan `slug`.** Menjalankan perintah yang sama dua kali tidak
menghasilkan duplikat — proyek dengan slug yang sudah ada akan **diperbarui**.
Jadi berkas ini bisa diperlakukan sebagai sumber kebenaran dan dimuat ulang kapan
saja setelah disunting.

**Tag ditulis dengan nama, bukan UUID.** Penulis berkas tidak mungkin tahu UUID.
Perintah ini mencari tag berdasarkan nama (case-insensitive) dan **membuatnya bila
belum ada**.

**Field di luar skema ditolak, bukan diabaikan.** Impor gagal — tanpa menulis
apa pun — bila sebuah entri memuat field yang tidak ada di tabel skema di bawah,
atau bila ada kunci tambahan di akar berkas selain `projects`.

Ini bukan kerewelan. Berkas keluaran agen pernah memuat `"visibility": "internal"`.
Bila field itu diabaikan diam-diam, proyek yang ditandai internal akan **terbit
sebagai proyek publik** — dan tidak ada satu baris keluaran pun yang memberi tahu.
Pesan galatnya menyebut nama field dan entri ke berapa:

```
proyek #1: json: unknown field "visibility"
akar berkas: json: unknown field "_internal"
```

Kalau memang ingin memuatnya, hapus dulu field tersebut — keputusannya di tangan
Anda, bukan di tangan parser.

**Slug ganda dalam satu berkas juga ditolak.** Karena kuncinya `slug`, entri
terakhir akan menimpa entri sebelumnya tanpa jejak. Galatnya menyebut kedua
nomornya.

Validasi dijalankan untuk **seluruh** entri sebelum satu pun ditulis, sehingga
berkas yang cacat tidak masuk separuh.

---

## Skema

Berkas boleh berbentuk `{"projects": [...]}` atau array telanjang.

| Field | Tipe | Wajib | Keterangan |
| ----- | ---- | ----- | ---------- |
| `title` | string | **ya** | Nama proyek |
| `slug` | string | **ya** | URL-safe, huruf kecil, tanda hubung. Jadi kunci upsert dan alamat halaman `/projects/<slug>` |
| `shortDesc` | string | **ya** | 1–2 kalimat. Tampil di kartu daftar proyek |
| `longDescription` | string | tidak | Deskripsi panjang di halaman detail |
| `coverImageUrl` | string | tidak | URL gambar cover: `http(s)` absolut atau path diawali `/` (lihat [Aturan URL](#aturan-url)). Kosongkan bila belum ada — situs memakai gambar cadangan, bukan gambar rusak |
| `category` | string | tidak | mis. `Web App`, `API`, `CLI` |
| `timeline` | string | tidak | mis. `3 months`, `2025 – 2026` |
| `role` | string | tidak | mis. `Fullstack Developer` |
| `challenge` | string | tidak | Bagian studi kasus: masalahnya |
| `solution` | string | tidak | Bagian studi kasus: pendekatannya |
| `results` | string[] | tidak | Poin hasil terukur. **Yang bisa diperiksa jauh lebih kuat** daripada klaim umum |
| `technicalDetails` | object | tidak | Objek bebas. Kunci yang dipakai frontend: `backend`, `frontend`, `database`, `deployment`, `architecture` |
| `demoUrl` | string \| null | tidak | Hanya `http://`/`https://` dengan host bila diisi. `null` atau `""` = tidak ada demo |
| `repoUrl` | string \| null | tidak | Hanya `http://`/`https://` dengan host bila diisi. `null` atau `""` = tidak ada repo |
| `screenshots` | string[] | tidak | Daftar URL gambar (`http(s)` atau path `/...`), urutan = urutan tampil. Item `""` dilewati |
| `features` | string[] | tidak | Daftar fitur, urutan = urutan tampil |
| `tags` | `{name, type}[]` | tidak | Lihat di bawah |
| `isFeatured` | boolean | tidak | `true` = ikut tampil di beranda (beranda mengambil 6 teratas) |
| `sortOrder` | number | tidak | Makin kecil makin dulu |

### Aturan URL

Nilai URL dirender situs sebagai `href`/`src`, jadi importer menolak apa pun
selain `http`/`https`. Aturannya sama dengan panel admin, dan diperiksa untuk
seluruh berkas sebelum ada yang ditulis (juga saat `-dry`).

| Field | Diterima |
| ----- | -------- |
| `demoUrl`, `repoUrl` | URL absolut `http://` atau `https://` dengan host, mis. `https://github.com/x/y` |
| `coverImageUrl`, tiap item `screenshots` | Sama seperti di atas, **atau** path relatif terhadap root situs yang diawali `/`, mis. `/images/cover.webp` |

Yang **ditolak**:

- Skema lain: `javascript:`, `data:`, `file:`, `ftp:`, `mailto:`, dst. (tidak
  peka huruf besar/kecil).
- URL protocol-relative `//host/...` (dan `/\host/...`): sebenarnya menunjuk
  host lain, jadi tidak dianggap path lokal.
- URL tanpa host, mis. `https://` atau `http:contoh.com`.
- Path gambar tanpa `/` di depan, mis. `images/a.png`.
- **Spasi di tepi**, mis. `" https://x.com"` atau `"/images/a.png "`. Nilai
  tidak dipangkas otomatis; rapikan di berkas.

Yang **dilewati** (bukan galat): `demoUrl`/`repoUrl`/`coverImageUrl` yang
kosong, dan item `screenshots` berisi `""`, yang tidak ikut disimpan. Urutan
screenshot lain tidak berubah.

Galat menyebut nomor proyek, judul, dan field-nya, mis.
`proyek #3 ("Portfolio"): URL tidak valid (...): demoUrl, screenshots[1]`.

### `tags`

```json
"tags": [
  { "name": "Go", "type": "backend" },
  { "name": "PostgreSQL", "type": "database" }
]
```

`type` menentukan pengelompokan di grid keahlian section About beranda. **Pakai
tipe yang sudah ada** agar tidak memunculkan kelompok kembar:

`language` · `backend` · `frontend` · `framework` · `database` · `devops` ·
`cloud` · `tools`

Tipe di luar daftar itu tetap tampil, hanya dengan ikon generik — lihat
`ICON_BY_TYPE` di `portfolio-frontend/src/components/AboutSection.tsx` bila ingin
menambah ikonnya. `type` yang dikosongkan menjadi `lainnya`.

---

## Catatan untuk agen yang menggenerate

1. **`slug` harus stabil.** Ia kunci upsert; mengubahnya membuat proyek baru,
   bukan memperbarui yang lama.
2. **Jangan mengarang `coverImageUrl`.** Kosongkan bila tidak ada gambar nyata —
   URL yang tidak bisa diambil akan menghasilkan gambar rusak. Gambar yang
   diunggah lewat panel admin dilayani dari `{APP_PUBLIC_URL}/uploads/...` dan
   otomatis dioptimasi; host lain tetap tampil tapi tanpa optimasi.
3. **`results` harus bisa diperiksa.** "Waktu muat turun dari 3,2 s ke 0,4 s"
   berbobot; "meningkatkan performa" tidak.
4. **Jangan mengisi `id`, `createdAt`, atau `updatedAt`** — semuanya diurus
   database dan akan diabaikan.
5. Nama field di sini **sama** dengan response `GET /api/v1/projects/:slug`,
   jadi keluaran API bisa disunting lalu dimasukkan kembali.

---

## Alternatif: lewat API

Bila lebih suka HTTP, endpoint admin menerima bentuk yang mirip — bedanya
`tags` diganti `tagIds` berisi UUID, sehingga tag harus sudah ada:

```sh
curl -X POST http://localhost:8080/api/v1/admin/projects \
  -H 'Content-Type: application/json' -b cookies.txt \
  -d @proyek.json
```

Untuk data yang digenerate, `make import` lebih cocok: tidak perlu login, tidak
perlu tahu UUID tag, dan idempoten.
