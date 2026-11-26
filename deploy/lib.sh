# shellcheck shell=bash
# ==========================================================================
#  deploy/lib.sh — bagian khusus portfolio-backend di atas lib-dasar.sh.
#  Disertakan install.sh, update.sh, status.sh, dan backup.sh:
#     AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#     . "$AKAR/deploy/lib.sh"
#
#  lib-dasar.sh adalah salinan APA ADANYA dari skill project-standard
#  (scripts/lib-dasar.sh). Jangan disunting di sini: perbaikan dilakukan di
#  sumbernya lalu disalin ulang, supaya `diff` langsung menunjukkan pergeseran.
# ==========================================================================

: "${AKAR:?AKAR harus di-set sebelum menyertakan deploy/lib.sh}"
# Path repo ikut tertulis di baris cron dan unit systemd, yang tidak mengenal
# kutip: spasi, %, $, atau kutip di sana menghasilkan berkas rusak.
[[ "$AKAR" =~ ^/[A-Za-z0-9_./-]+$ ]] || {
  echo "GAGAL: path repo \"$AKAR\" memuat karakter selain huruf, angka, _ . / - — pindahkan ke path sederhana (mis. /opt/portfolio-backend)" >&2
  exit 3
}
PROYEK="portfolio-backend"
ENV_FILE="$AKAR/deploy/.env"
ENV_CONTOH="$AKAR/deploy/env.example"
COMPOSE_UTAMA="$AKAR/deploy/docker-compose.yml"
VERSI_FILE="$AKAR/deploy/.versi"
CABANG_FILE="$AKAR/deploy/.cabang"
# shellcheck source=deploy/lib-dasar.sh
. "$AKAR/deploy/lib-dasar.sh"

# Layanan aplikasi (yang punya image bertanda versi dan ikut di-update).
LAYANAN="api"

# Skrip ini menulis ke deploy/.env sebagai root. Bila berkas itu symlink
# (folder repo biasanya milik pengguna biasa), tulisan root mengenai targetnya.
[[ ! -L "$ENV_FILE" ]] || gagal "$ENV_FILE adalah symlink — hapus, lalu jalankan: sudo ./install.sh"

butuh_env() { [[ -f "$ENV_FILE" ]] || gagal "$ENV_FILE belum ada — jalankan dulu: sudo ./install.sh"; }

# Satu operasi pada satu waktu (install, update, pemulihan). Kunci di /run
# (milik root), bukan /var/lock yang bisa ditulisi semua pengguna.
kunci_operasi() {
  install -d -m 700 "/run/$PROYEK"
  kunci "/run/$PROYEK/operasi.lock" || gagal "operasi lain (install/update/pemulihan) sedang berjalan"
}

# PostgreSQL ikut dijalankan stack ini?
db_lokal() { [[ ",$(env_get COMPOSE_PROFILES)," == *",db,"* ]]; }

# compose dengan versi image tertentu (build, migrasi, dan pergantian versi).
# Di luar itu pakai dc_aktif, yang membaca versi aktif dari berkas override.
dcv() { local v="$1"; shift; VERSI="$v" dc "$@"; }

# DB_DSN untuk PostgreSQL di stack ini. Sandi dibuat `rahasia` (alfanumerik),
# jadi aman ditaruh di URL tanpa di-encode.
susun_dsn() {
  printf 'postgres://%s:%s@db:5432/%s?sslmode=disable' \
    "$(env_atau POSTGRES_USER portfolio)" "$(env_get POSTGRES_PASSWORD)" "$(env_atau POSTGRES_DB portfolio)"
}

# Berkas yang di-bind-mount ke kontainer non-root harus terbaca "other": clone
# di server ber-umask ketat (027/077) menghasilkan berkas 600/640, dan
# `git pull` bisa menulis ulang dengan izin ketat lagi. Hanya berkas biasa:
# `chmod` mengikuti symlink, dan symlink di folder ini bisa menunjuk ke mana saja.
izinkan_baca_mount() {
  [[ -d "$AKAR/migrations" && ! -L "$AKAR/migrations" ]] || gagal "$AKAR/migrations bukan folder biasa"
  chmod a+rx "$AKAR/migrations"
  find "$AKAR/migrations" -maxdepth 1 -type f -exec chmod a+r {} +
  [[ -f "$AKAR/deploy/atlas.hcl" && ! -L "$AKAR/deploy/atlas.hcl" ]] || gagal "$AKAR/deploy/atlas.hcl bukan berkas biasa"
  chmod a+r "$AKAR/deploy/atlas.hcl"
}

bangun() { # versi
  langkah "Membangun image $PROYEK/api:$1 di server ini"
  docker buildx version >/dev/null 2>&1 \
    || gagal "docker buildx tidak ada (Dockerfile butuh BuildKit) — pasang docker-buildx-plugin"
  dcv "$1" build api
}

# ------------------------------------------------------------------ image dari GitHub
# Image dibangun GitHub Actions (.github/workflows/image.yml) tiap ada commit di
# master dan diterbitkan ke registry bertanda SHA commit. Server cukup
# menariknya — tidak perlu membangun sendiri.
IMAGE_JAUH_BAWAAN="ghcr.io/fauzanparanditha/go-portfolio-backend"
image_jauh() { env_atau IMAGE_JAUH "$IMAGE_JAUH_BAWAAN"; }
# github (bawaan) | lokal. `--bangun-lokal` pada install.sh/update.sh memaksa
# lokal untuk satu kali jalan.
sumber_image() {
  local s; s="$(env_atau SUMBER_IMAGE github)"
  [[ "${PAKSA_LOKAL:-0}" == 1 ]] && s=lokal
  case "$s" in github|lokal) printf '%s' "$s" ;; *) gagal "SUMBER_IMAGE harus \"github\" atau \"lokal\" (sekarang: $s)" ;; esac
}

# tarik <versi>: ambil image untuk commit yang ter-checkout, lalu beri nama lokal
# $PROYEK/api:<versi>. Sesudah itu ia diperlakukan persis seperti image hasil
# build lokal (ganti versi, rollback, pemangkasan).
tarik() {
  local v="$1" sha jauh rev galat
  sha="$(git_repo rev-parse HEAD 2>/dev/null)" || gagal "tidak bisa membaca commit yang ter-checkout"
  jauh="$(image_jauh):$sha"
  langkah "Menarik image $jauh"
  if ! galat="$(docker pull -q "$jauh" 2>&1 >/dev/null)"; then
    printf '%s\n' "$galat" | tail -n 3 >&2
    merah "Image untuk commit ${sha:0:12} tidak bisa ditarik. Kemungkinan sebabnya:" >&2
    merah "  · build di GitHub belum selesai atau gagal — lihat tab Actions (workflow \"Image\"), lalu ulangi;" >&2
    merah "  · commit ini tidak pernah dibangun (hanya ujung tiap push ke master yang dibangun);" >&2
    merah "  · galat \"unauthorized\"/\"denied\": package di ghcr.io masih privat — ubah menjadi Public;" >&2
    merah "  · GitHub tidak terjangkau dari server ini." >&2
    merah "Versi yang sedang berjalan tidak tersentuh. Jalan lain: ulangi dengan --bangun-lokal." >&2
    return 1
  fi
  # Tag bisa ditulis ulang; label revisi ditanam saat build. Keduanya harus
  # menunjuk commit yang sama dengan kode (dan migrasi) di folder ini.
  rev="$(docker image inspect -f '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$jauh" 2>/dev/null || true)"
  if [[ "$rev" != "$sha" ]]; then
    docker image rm "$jauh" >/dev/null 2>&1 || true
    merah "Image $jauh berlabel revisi \"$rev\", bukan commit $sha — ditolak." >&2
    return 1
  fi
  docker tag "$jauh" "$PROYEK/api:$v"
  # Hanya melepas nama jauhnya; isi image tetap ada lewat nama lokal.
  docker image rm "$jauh" >/dev/null 2>&1 || true
  hijau "image siap: $PROYEK/api:$v"
}

# siapkan_image <versi>: tarik dari registry, atau bangun di sini.
siapkan_image() {
  local s; s="$(sumber_image)" || return 1
  if [[ "$s" == lokal ]]; then bangun "$1"; else tarik "$1"; fi
}

# Migrasi dijalankan dari folder migrations/ di kode yang sedang ter-checkout,
# SEBELUM versi baru API dinyalakan. Gagal = versi lama tetap berjalan.
migrasi() {
  langkah "Menjalankan migrasi database"
  izinkan_baca_mount
  dc --profile alat run --rm migrate
}

# Alamat untuk memeriksa API dari server ini sendiri.
url_lokal() {
  local a; a="$(env_atau ALAMAT_IKAT 127.0.0.1)"
  case "$a" in 0.0.0.0|"") a=127.0.0.1 ;; esac
  printf 'http://%s:%s' "$a" "$(env_atau PORT_API 8080)"
}

volume_unggahan() { printf '%s_unggahan' "$PROYEK"; }

# Image kecil ber-`tar` untuk membaca/menulis volume unggahan: image layanan db
# (dibaca dari compose, jadi versinya satu tempat). Diambil dengan profil db
# dipaksa aktif supaya juga dikenal saat database berada di luar stack ini.
image_alat() { dc --profile db config --images 2>/dev/null | grep -E '^postgres:' | head -n1; }

# Cabang yang diikuti server ini. Dicatat tiap kali kode ditarik dari sebuah
# cabang, supaya `update.sh` sesudah `--ref=<tag>` (HEAD lepas) kembali ke
# cabang yang sama — bukan ke cabang bawaan origin.
catat_cabang() {
  local c; c="$(git_repo symbolic-ref -q --short HEAD 2>/dev/null || true)"
  [[ -z "$c" ]] || printf '%s\n' "$c" >"$CABANG_FILE"
}
cabang_ikut() {
  local c=""
  [[ ! -r "$CABANG_FILE" ]] || c="$(sed -n 1p "$CABANG_FILE")"
  [[ -n "$c" ]] || c="$(git_repo symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's#^origin/##' || true)"
  printf '%s' "${c:-master}"
}

# Pemilik folder repo bukan root: akun itu SETARA ROOT, karena cron dan unit
# systemd menjalankan skrip dari folder ini sebagai root.
peringatkan_pemilik_repo() {
  local p; p="$(stat -c %U "$AKAR" 2>/dev/null || echo root)"
  [[ "$p" == root ]] || kuning "Folder repo milik \"$p\": akun itu setara root di server ini (cron dan systemd menjalankan skrip dari sini sebagai root). Lebih aman: clone sebagai root di /opt/$PROYEK."
}

# Compose >= 2.20 (dibutuhkan `depends_on ... required: false`).
butuh_compose() {
  local v; v="$(docker compose version --short 2>/dev/null || true)"; v="${v#v}"
  [[ -n "$v" ]] || gagal "plugin docker compose tidak ada — pasang docker-compose-plugin"
  [[ "$(printf '%s\n' 2.20 "$v" | sort -V | head -n1)" == 2.20 ]] \
    || gagal "docker compose $v terlalu lama (butuh >= 2.20) — perbarui docker-compose-plugin"
}
