#!/usr/bin/env bash
# ==========================================================================
#  update.sh — memasang versi baru, dengan jalan mundur.
#
#     sudo ./update.sh                 tarik kode terbaru cabang ini lalu pasang
#     sudo ./update.sh --ref=<tag|cabang|commit>
#     sudo ./update.sh --no-pull       pasang kode yang sudah ada di folder ini
#     sudo ./update.sh --bangun-lokal  bangun image di server ini, alih-alih
#                                      menarik yang dibangun GitHub
#     sudo ./update.sh --rollback      kembali ke versi sebelumnya
#     sudo ./update.sh --status        versi aktif dan versi sebelumnya
#
#  Urutan: kunci → tolak pohon kerja kotor → tarik kode → lengkapi .env →
#  tarik image versi baru (dibangun GitHub) → cadangkan database → migrasi → ganti API →
#  tunggu healthy (tidak healthy = versi lama dinyalakan lagi).
#  Selama langkah sebelum "ganti API" gagal, versi lama tetap berjalan.
#
#  --rollback TIDAK membatalkan migrasi. Bila versi lama tidak jalan di skema
#  baru, pulihkan cadangan "pra-update" yang dibuat tepat sebelum migrasi:
#     sudo ./backup.sh --daftar
# ==========================================================================
set -Eeuo pipefail
AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy/lib.sh
. "$AKAR/deploy/lib.sh"

REF=""; TARIK=1; AKSI=update; PAKSA_LOKAL=0; ARG_ASLI=("$@")
for a in "$@"; do
  case "$a" in
    --ref=*) REF="${a#*=}" ;;
    --no-pull) TARIK=0 ;;
    --bangun-lokal) PAKSA_LOKAL=1 ;;
    --rollback) AKSI=rollback ;;
    --status) AKSI=status ;;
    -h|--help) sed -n '2,21p' "$0" | sed 's/^# \{0,2\}//'; exit 0 ;;
    *) gagal "argumen tidak dikenal: $a (lihat --help)" ;;
  esac
done

if [[ "$AKSI" == status ]]; then
  butuh_env
  printf 'aktif     : %s\nsebelumnya: %s\n' "$(versi_aktif api)" "$(versi_lama api)"
  exit 0
fi

butuh_root
butuh_env
butuh_perintah docker git flock
# Diteruskan ke git: tidak boleh diawali "-" (terbaca sebagai opsi).
[[ -z "$REF" || "$REF" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || gagal "--ref tidak sah: $REF"
kunci_operasi

AKTIF="$(versi_aktif api)"
[[ -n "$AKTIF" ]] || gagal "belum ada versi terpasang — jalankan dulu: sudo ./install.sh"

# Nyalakan API pada versi tertentu dan tunggu sampai healthy.
nyalakan() { dcv "$1" up -d --no-deps api && tunggu_sehat api 90; }

if [[ "$AKSI" == rollback ]]; then
  LAMA="$(versi_lama api)"
  [[ -n "$LAMA" ]] || gagal "tidak ada versi sebelumnya yang tercatat"
  docker image inspect "$PROYEK/api:$LAMA" >/dev/null 2>&1 || gagal "image $PROYEK/api:$LAMA sudah tidak ada"
  langkah "Kembali dari $AKTIF ke $LAMA"
  kuning "Migrasi database TIDAK dibatalkan. Bila $LAMA tidak jalan di skema sekarang, pulihkan cadangan pra-update (sudo ./backup.sh --daftar)."
  if nyalakan "$LAMA"; then
    catat_versi api "$LAMA" "$AKTIF"; tulis_override $LAYANAN
    hijau "Versi aktif sekarang: $LAMA"
    kirim_alarm "[$PROYEK] rollback ke $LAMA di $(hostname)"
    exec "$AKAR/status.sh" --ringkas
  fi
  merah "Versi $LAMA tidak healthy — log terakhir:"; dc logs --tail 30 api || true
  kirim_alarm "[$PROYEK] ROLLBACK GAGAL di $(hostname): $LAMA tidak healthy"
  exit 1
fi

trap 'merah "update.sh berhenti di baris $LINENO. Versi aktif tetap $AKTIF; aman dijalankan ulang."' ERR

# ---------------------------------------------------------------- kode
rapikan_pemilik_repo
if (( TARIK )) || [[ -n "$REF" ]]; then
  # Hanya berkas TERLACAK yang diubah: itulah yang membuat tarikan gagal atau
  # diam-diam membangun kode yang bukan dari git. Berkas tak terlacak (catatan
  # skrip ini sendiri, salinan .env) tidak menghalangi.
  [[ -z "$(git_repo status --porcelain --untracked-files=no)" ]] \
    || gagal "ada berkas terlacak yang diubah di server (git status) — simpan atau buang perubahannya dulu, atau pakai --no-pull"
  HASH_AWAL="$(sha256sum "$AKAR/update.sh" "$AKAR/deploy/lib.sh" "$AKAR/deploy/lib-dasar.sh" | sha256sum)"
  langkah "Menarik kode"
  git_repo fetch --prune --tags origin
  if [[ -n "$REF" ]]; then
    # Nama cabang harus berarti origin/<cabang>, bukan cabang lokal yang basi.
    if git_repo rev-parse -q --verify "refs/remotes/origin/$REF" >/dev/null; then
      git_repo checkout -q --detach "origin/$REF"
    else
      git_repo checkout -q --detach "$REF"
    fi
  else
    # HEAD lepas (sisa --ref sebelumnya) membuat `git pull` gagal: kembali ke
    # cabang yang diikuti server ini (dicatat di deploy/.cabang), bukan
    # sembarang cabang bawaan origin.
    if ! git_repo symbolic-ref -q HEAD >/dev/null; then
      git_repo checkout -q "$(cabang_ikut)"
    fi
    git_repo pull -q --ff-only
    catat_cabang
  fi
  # Bash masih mengeksekusi skrip LAMA yang sudah dimuat: perbaikan pada
  # update.sh baru berlaku satu update kemudian. Jalankan ulang yang baru.
  HASH_BARU="$(sha256sum "$AKAR/update.sh" "$AKAR/deploy/lib.sh" "$AKAR/deploy/lib-dasar.sh" | sha256sum)"
  if [[ "$HASH_AWAL" != "$HASH_BARU" ]]; then
    kuning "skrip update berubah — menjalankan ulang versi barunya"
    exec 9>&-
    ARG_BARU=(--no-pull)
    for a in ${ARG_ASLI[@]+"${ARG_ASLI[@]}"}; do
      case "$a" in --ref=*|--no-pull) ;; *) ARG_BARU+=("$a") ;; esac
    done
    exec "$AKAR/update.sh" "${ARG_BARU[@]}"
  fi
fi

# ---------------------------------------------------------------- .env
env_lengkapi_dari "$ENV_CONTOH"
env_rapikan_ganda
env_periksa_ganda

# ---------------------------------------------------------------- image
BARU="$(versi_baru)"
# Tag beresolusi menit: dua update dalam semenit dari commit yang sama akan
# menimpa image versi aktif dan membuat "versi sebelumnya" = dirinya sendiri.
[[ "$BARU" != "$AKTIF" ]] || BARU="$BARU$(date +%S)"
siapkan_image "$BARU" || gagal "image versi baru tidak tersedia — update dibatalkan, versi $AKTIF tetap berjalan"

# ---------------------------------------------------------------- data
if db_lokal; then
  langkah "Mencadangkan database sebelum migrasi"
  "$AKAR/backup.sh" --label=pra-update || gagal "cadangan gagal — update dibatalkan, versi $AKTIF tetap berjalan"
fi
migrasi || gagal "migrasi gagal — update dibatalkan, versi $AKTIF tetap berjalan"

# ---------------------------------------------------------------- ganti versi
langkah "Mengganti API: $AKTIF → $BARU"
if nyalakan "$BARU"; then
  catat_versi api "$BARU" "$AKTIF"; tulis_override $LAYANAN
  pangkas_image api 3
  docker builder prune -f --filter until=168h >/dev/null 2>&1 || true
  hijau "Versi aktif sekarang: $BARU (sebelumnya $AKTIF)"
  kirim_alarm "[$PROYEK] update ke $BARU berhasil di $(hostname)"
  trap - ERR
  exec "$AKAR/status.sh" --ringkas
fi

merah "Versi $BARU tidak healthy — log terakhir:"; dc logs --tail 30 api || true
langkah "Mengembalikan versi $AKTIF"
trap - ERR
if nyalakan "$AKTIF"; then
  kuning "Versi $AKTIF berjalan lagi. Migrasi versi $BARU sudah diterapkan dan tidak dibatalkan."
  kirim_alarm "[$PROYEK] update ke $BARU GAGAL di $(hostname); kembali ke $AKTIF"
else
  merah "Versi $AKTIF juga tidak healthy — periksa: ./status.sh"
  kirim_alarm "[$PROYEK] update ke $BARU GAGAL dan pengembalian ke $AKTIF juga gagal di $(hostname)"
fi
exit 1
