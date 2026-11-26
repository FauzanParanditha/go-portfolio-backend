# Pemeliharaan backend

Dokumen ini menjawab: **bagaimana memasang, memperbarui, memeriksa, dan
memulihkan backend di server — dan apa yang harus dilakukan saat ia mati?**

Topologi domain, cookie, dan reverse proxy ada di [DEPLOYMENT.md](DEPLOYMENT.md).

## Ringkasan perintah

Semua dijalankan dari akar repo di server.

| Keperluan | Perintah |
|---|---|
| Pasang pertama kali, atau ubah konfigurasi | `sudo ./install.sh` |
| Apakah berjalan? Kalau tidak, salahnya di mana? | `sudo ./status.sh` |
| Pasang versi terbaru (image ditarik dari GitHub) | `sudo ./update.sh` |
| Sama, tetapi image dibangun di server ini | `sudo ./update.sh --bangun-lokal` |
| Pasang tag/cabang/commit tertentu | `sudo ./update.sh --ref=<ref>` |
| Kembali ke versi sebelumnya | `sudo ./update.sh --rollback` |
| Versi aktif dan sebelumnya | `sudo ./update.sh --status` |
| Cadangkan sekarang | `sudo ./backup.sh` |
| Daftar cadangan | `sudo ./backup.sh --daftar` |
| Pulihkan (menimpa data) | `sudo ./backup.sh --pulihkan=<berkas> --ya` |
| Hentikan / nyalakan seluruh stack | `sudo systemctl stop\|start portfolio-backend` |
| Log API | `docker logs --tail 100 -f portfolio-backend-api-1` |
| Impor proyek dari JSON | lihat [IMPOR-PROYEK](IMPORT-PROYEK.md); di server, binary-nya ada di image: `/app/import` |

`status.sh` keluar dengan `0` normal, `1` terganggu, `2` mati, `3` tidak bisa
memeriksa; `--ringkas` dan `--json` untuk cron dan pemantau.

## 1. Arsitektur singkat

| Komponen | Isi | Terjangkau dari |
|---|---|---|
| `api` | image `portfolio-backend/api:<versi>` (distroless, non-root, akar hanya-baca) | `ALAMAT_IKAT:PORT_API` di server |
| `db` | PostgreSQL 17, hanya bila `COMPOSE_PROFILES` memuat `db` | jaringan Docker internal saja — tidak dipublikasikan |
| `migrate` | Atlas, sekali jalan; membaca `migrations/` dari repo | — |
| volume `data_db` | isi database | — |
| volume `unggahan` | gambar proyek dan CV yang diunggah admin | — |

Berkas: `deploy/docker-compose.yml` (stack), `deploy/.env` (konfigurasi, izin
600, tidak masuk git), `deploy/.versi` dan `deploy/docker-compose.override.yml`
(versi aktif — ditulis skrip, jangan disunting).

Jangan menjalankan `docker compose up` polos di `deploy/`, dan jangan
`sudo git pull`: skrip yang memilih versi image dan menarik kode sebagai
pemilik folder.

**Pemilik folder repo setara root.** Cron dan unit systemd menjalankan skrip
dari folder ini sebagai root. Siapa pun yang bisa menulis ke folder ini (atau
mem-push ke cabang yang diikuti server) bisa menjadi root di server. Karena
itu: clone
sebagai root di `/opt/portfolio-backend`, atau perlakukan akun pemiliknya
seperti akun root. `install.sh` memperingatkan bila pemiliknya bukan root.

### Jebakan yang perlu diketahui sebelum memasang

- **Port yang dipublikasikan Docker melewati UFW/CSF.** `ALAMAT_IKAT=0.0.0.0`
  membuka port API ke internet apa pun aturan firewall-nya — termasuk
  `/healthz` dan `/readyz` akar yang tanpa rate limit. Bawaan `127.0.0.1`.
- **Jangan login admin lewat HTTP polos.** Tanpa TLS, password dan token admin
  terkirim tanpa enkripsi dan terbaca siapa pun di jalur jaringan. Di browser
  sesinya juga tidak bertahan (cookie bertanda `Secure`, wajib di produksi),
  tetapi API-nya sendiri tetap menerima login dan token `Bearer`. Situs publik
  tidak terpengaruh. Karena itu `install.sh` MENOLAK `ALAMAT_IKAT=0.0.0.0` dengan
  `APP_PUBLIC_URL` non-https, kecuali `IZINKAN_HTTP_TERBUKA=ya` diisi sebagai
  keputusan sadar dan sementara. Jalan yang benar: reverse proxy ber-TLS di depan.
- **Frontend di server yang sama tidak bisa menjangkau `127.0.0.1`.** Server
  Next.js mengambil data dari DALAM kontainernya. Supaya terjangkau tanpa
  membuka port ke internet, ikat API ke alamat jembatan Docker:
  `ALAMAT_IKAT=<hasil ip -4 addr show docker0, biasanya 172.17.0.1>`, lalu di
  frontend `API_INTERNAL_URL=http://host.docker.internal:<PORT_API>/api/v1`.
  Reverse proxy di host yang sama nanti menunjuk ke alamat itu juga. Alamat itu
  tidak dirutekan dari internet, tetapi terjangkau semua kontainer lain di
  server ini.
- **`migrations/atlas.sum` harus ikut di-commit** setiap menambah migrasi
  (`make migrate-up` menghitungnya). Bila tertinggal, migrasi di server gagal
  dengan `checksum mismatch` dan update dibatalkan — versi lama tetap berjalan.

## 2. Log

| Apa | Di mana | Disimpan |
|---|---|---|
| API (JSON, satu baris per kejadian) | `docker logs portfolio-backend-api-1` | 5 berkas × 20 MB |
| Database | `docker logs portfolio-backend-db-1` | 5 berkas × 20 MB |
| Cadangan harian | `/var/log/portfolio-backend-backup.log` | tidak dirotasi (beberapa baris per hari) |

## 3. Pembaruan dan jalan mundur

**Image dibangun di GitHub, bukan di server.** Tiap commit di `master`
dibangun workflow `Image` (`.github/workflows/image.yml`), dipindai isinya, lalu
diterbitkan ke `ghcr.io/fauzanparanditha/go-portfolio-backend` bertanda SHA
commit. Server hanya menariknya.

Alurnya: gabungkan PR ke `master` → tunggu workflow `Image` hijau (±2–3 menit)
→ di server `sudo ./update.sh`.

`sudo ./update.sh` mengerjakan, berurutan: tarik kode → lengkapi `deploy/.env`
dengan kunci baru → tarik image untuk commit itu → cadangkan database (label
`pra-update`) → migrasi → ganti API → tunggu healthy.

- Gagal **sebelum** "ganti API" (image belum ada, cadangan, migrasi): update
  dibatalkan, versi lama tidak pernah berhenti.
- Image belum ada di registry (build belum selesai atau gagal): skrip berhenti
  dengan daftar kemungkinan sebabnya. Ulangi sesudah workflow hijau, atau
  pakai `--bangun-lokal` (build Go ringan, aman dijalankan di server).
- Hanya **ujung tiap push ke `master`** yang dibangun. `--ref=<commit>` ke
  commit lain butuh `--bangun-lokal`.
- API versi baru **tidak healthy**: versi lama dinyalakan lagi secara otomatis.
- `--rollback` kembali ke versi sebelumnya. **Migrasi tidak dibatalkan**: tulis
  migrasi secara aditif (tambah kolom, isi, baru hapus di rilis berikutnya)
  supaya versi lama tetap jalan di skema baru. Bila tidak, pulihkan cadangan
  `pra-update`.

Update menolak berjalan bila pohon kerja git di server tidak bersih.

### Satu kali, sesudah image pertama terbit

Package baru di `ghcr.io` bisa terbuat **privat** walaupun repo-nya publik, dan
server lalu gagal menariknya (`unauthorized`). Sesudah workflow `Image` pertama
selesai: profil GitHub → **Packages** → `go-portfolio-backend` → **Package
settings** → **Change visibility** → **Public**. Cukup sekali.

Image publik bisa dibongkar siapa saja, lapis demi lapis, dan tidak bisa
ditarik kembali. Karena itu workflow menjalankan `tools/periksa-image.sh`
sebelum menerbitkan: tidak boleh ada `.env`, `.git`, berkas kunci, atau berkas
unggahan di lapisan mana pun.

## 4. Mengubah konfigurasi

Sunting `deploy/.env` (dengan sudo; berkasnya milik root), lalu
`sudo ./install.sh --non-interactive`: kontainer dibuat ulang hanya bila perlu;
versi kode tidak berubah dan migrasi tidak dijalankan. Kunci dan artinya ada di
`deploy/env.example`.

Kontainer API hanya menerima kunci yang tertulis di `environment:` layanan `api`
pada `deploy/docker-compose.yml` — bukan seluruh `deploy/.env`. Kunci baru yang
dibaca aplikasi harus ditambahkan di sana (ada test yang menjaganya).

## 5. Rahasia

| Rahasia | Dibuat | Mengganti |
|---|---|---|
| `JWT_SECRET` | `install.sh`, sekali | ganti nilainya lalu jalankan `install.sh`; semua sesi admin putus |
| `POSTGRES_PASSWORD` | `install.sh`, sekali | ubah di PostgreSQL (`ALTER USER`) **dan** di `POSTGRES_PASSWORD` serta `DB_DSN`, lalu `install.sh` |
| Password admin pertama | `install.sh`, saat pemasangan pertama; hanya tertulis di `/root/portfolio-backend-admin-awal.txt` (tidak di `deploy/.env`, tidak di kontainer) | pindahkan ke pengelola sandi lalu hapus berkas itu. **Belum ada fitur ganti password**: satu-satunya jalan adalah "lupa password", yang butuh SMTP |

## 6. Cadangan dan pemulihan

Cron (`/etc/cron.d/portfolio-backend`) menjalankan `backup.sh` tiap hari 02:30.
Hasilnya di `BACKUP_DIR` (bawaan `/var/backups/portfolio-backend`), disimpan
`BACKUP_KEEP_DAYS` hari (bawaan 14):

- `portfolio-<waktu>-<label>.dump` — database (`pg_dump` format custom);
- `unggahan-<waktu>-<label>.tar` — berkas unggahan.

Memulihkan menimpa data saat ini. Urutannya: arsip diperiksa utuh dulu (yang
rusak ditolak sebelum apa pun dihentikan) → keadaan sekarang dicadangkan (label
`pra-pulih`) → database dipulihkan ke database sementara → API dihentikan,
database ditukar, API dinyalakan lagi. Pemulihan yang gagal tidak mengubah
database yang sedang dipakai, dan API selalu dinyalakan kembali.

```sh
sudo ./backup.sh --daftar
sudo ./backup.sh --pulihkan=/var/backups/portfolio-backend/<berkas> --ya
```

Bila database berada di luar stack ini (`COMPOSE_PROFILES` tanpa `db`), hanya
berkas unggahan yang dicadangkan dan dipulihkan di sini; cadangan database
menjadi urusan server database itu.

**Terakhir diuji pulih:** 2026-11-25, di VM Linux lokal (database, unggahan,
arsip rusak, dan mode database eksternal). Belum pernah diuji di server produksi.

**Cadangan berada di disk yang sama dengan datanya.** Ia menyelamatkan dari
migrasi atau penghapusan yang salah, bukan dari server yang mati. Menyalin
cadangan ke luar server belum diatur.

## 7. Penanganan masalah

Mulai selalu dari `sudo ./status.sh` — tiap baris yang tidak hijau membawa saran.

| Gejala | Sebab yang paling mungkin | Tindakan |
|---|---|---|
| `Database (dari API)` mati, kontainer API sehat | database mati atau tidak terjangkau | **jangan restart API**; lihat `docker logs portfolio-backend-db-1`; periksa disk |
| Kontainer API `restarting` | `deploy/.env` ditolak saat start (mis. `APP_PUBLIC_URL` memuat `localhost`, `JWT_SECRET` pendek) | baca pesan di `docker logs portfolio-backend-api-1`, perbaiki `.env`, `install.sh` |
| `API menjawab` mati, kontainer sehat | `ALAMAT_IKAT`/`PORT_API` tidak cocok, atau port dipakai proses lain | periksa `deploy/.env`; `ss -tlnp \| grep <port>` |
| Login admin selalu kembali ke halaman login | situs dibuka lewat HTTP, cookie `Secure` dibuang | pasang reverse proxy ber-TLS (DEPLOYMENT.md) |
| Semua pengunjung kena `429` bersamaan | di balik proxy tanpa `TRUSTED_PROXIES` | DEPLOYMENT.md, bagian reverse proxy |
| Update berhenti: image tidak bisa ditarik | build GitHub belum selesai/gagal, package masih privat, atau GitHub tidak terjangkau | tab **Actions** → workflow `Image`; periksa visibilitas package; atau `--bangun-lokal` |
| Update berhenti: `checksum mismatch` | `atlas.sum` tidak ikut di-commit | commit `atlas.sum` yang benar, `update.sh` lagi |
| Disk hampir penuh | image lama, cache build, cadangan | `docker builder prune -f`; `docker system df` |

## 8. Daftar periksa insiden

1. `sudo ./status.sh` — catat baris yang merah.
2. Baru saja update? `sudo ./update.sh --rollback`.
3. Yang merah database, bukan API: jangan restart API; periksa disk dan log database.
4. Kontainer API `restarting`: baca log-nya — hampir selalu konfigurasi.
5. Kontainer berhenti (`exited`): `sudo systemctl restart portfolio-backend`
   (`start` tidak berbuat apa-apa bila unit-nya masih tercatat aktif).
6. Data rusak: `sudo ./backup.sh --daftar`, pulihkan cadangan terakhir yang baik.
7. Sesudah pulih: `sudo ./status.sh` harus kembali `normal`.

## Yang belum ada

- Pemantau berkala dengan alarm saat keadaan berubah (`pantau.sh`); saat ini
  hanya hasil update dan cadangan yang gagal yang bisa dikirim ke webhook/Telegram
  (`ALARM_*` di `deploy/.env`).
- Salinan cadangan di luar server.
- Fitur ganti password admin (selain "lupa password" lewat email).
- Role database terpisah untuk aplikasi: saat ini API memakai pengguna
  PostgreSQL yang dibuat image (superuser di database itu).
- Skrip firewall untuk membatasi port yang dipublikasikan ke alamat tertentu.
- Uji asap terhadap stack yang berjalan (pemeriksaan 7 di `tools/periksa.sh`).
- Jalur pemasangan Docker otomatis di `install.sh` belum pernah dijalankan:
  semua pengujian dilakukan di mesin yang sudah punya Docker.
