#!/usr/bin/env bash
# ==========================================================================
#  backup.sh — cadangan database dan berkas unggahan.
#
#     sudo ./backup.sh                          cadangkan sekarang (juga dipanggil cron)
#     sudo ./backup.sh --daftar                 daftar cadangan
#     sudo ./backup.sh --pulihkan=<berkas> --ya pulihkan (MENIMPA data saat ini)
#
#  Dua jenis berkas di BACKUP_DIR (bawaan /var/backups/portfolio-backend):
#     <db>-<waktu>-<label>.dump       pg_dump format custom
#     unggahan-<waktu>-<label>.tar    isi volume berkas unggahan
#  Cadangan lebih tua dari BACKUP_KEEP_DAYS (bawaan 14) dihapus.
#
#  Bila database berada di luar stack ini (COMPOSE_PROFILES tanpa "db"), hanya
#  berkas unggahan yang dicadangkan dan dipulihkan di sini.
#
#  Cadangan ini berada di DISK YANG SAMA dengan datanya: ia menyelamatkan dari
#  migrasi yang salah, bukan dari server yang mati. Menyalinnya ke luar server
#  belum diatur — lihat docs/PEMELIHARAAN.md.
# ==========================================================================
set -Eeuo pipefail
AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy/lib.sh
. "$AKAR/deploy/lib.sh"

AKSI=cadangkan; BERKAS=""; YA=0; LABEL=harian
for a in "$@"; do
  case "$a" in
    --daftar) AKSI=daftar ;;
    --pulihkan=*) AKSI=pulihkan; BERKAS="${a#*=}" ;;
    --ya) YA=1 ;;
    --label=*) LABEL="${a#*=}" ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,2\}//'; exit 0 ;;
    *) gagal "argumen tidak dikenal: $a (lihat --help)" ;;
  esac
done
[[ "$LABEL" =~ ^[a-z0-9-]+$ ]] || gagal "label hanya boleh huruf kecil, angka, dan tanda hubung"

butuh_root
butuh_env
butuh_perintah docker

DIR="$(env_atau BACKUP_DIR "/var/backups/$PROYEK")"
SIMPAN="$(env_atau BACKUP_KEEP_DAYS 14)"
U="$(env_atau POSTGRES_USER portfolio)"
DB="$(env_atau POSTGRES_DB portfolio)"
[[ "$SIMPAN" =~ ^[0-9]+$ ]] || gagal "BACKUP_KEEP_DAYS harus angka"
# Nama ini ikut ke perintah SQL dan nama berkas.
[[ "$U" =~ ^[a-z_][a-z0-9_]*$ && "$DB" =~ ^[a-z_][a-z0-9_]*$ ]] \
  || gagal "POSTGRES_USER/POSTGRES_DB hanya boleh huruf kecil, angka, dan garis bawah"
# Folder cadangan di-chmod 700 dan ditulisi root: harus path mutlak yang
# sederhana, bukan symlink, dan bukan folder milik pengguna lain (di sana nama
# berkas sementara bisa dijebak dengan symlink).
[[ "$DIR" =~ ^/[A-Za-z0-9_./-]+$ && "$DIR" != */../* && "$DIR" != / && "$DIR" != /tmp && "$DIR" != /var/tmp ]] \
  || gagal "BACKUP_DIR tidak sah: $DIR (pakai path mutlak khusus, mis. /var/backups/$PROYEK)"
[[ ! -L "$DIR" ]] || gagal "BACKUP_DIR adalah symlink: $DIR"
[[ ! -e "$DIR" || "$(stat -c %U "$DIR")" == root ]] || gagal "BACKUP_DIR bukan milik root: $DIR"

daftar() {
  [[ -d "$DIR" ]] || { echo "belum ada cadangan di $DIR"; return 0; }
  find "$DIR" -maxdepth 1 -type f \( -name '*.dump' -o -name 'unggahan-*.tar' \) -printf '%TY-%Tm-%Td %TH:%TM  %10s  %f\n' | sort -r
}

# Kontainer sekali jalan untuk membaca/menulis volume unggahan, tanpa jaringan.
alat() { docker run --rm -i --network none "$@"; }

cadangkan_unggahan() { # label → path berkas
  local img f="$DIR/unggahan-$(date +%Y%m%d-%H%M%S)-$1.tar"
  img="$(image_alat)"; [[ -n "$img" ]] || gagal "image alat (layanan db di compose) tidak dikenali"
  ( umask 077
    alat -v "$(volume_unggahan)":/data:ro "$img" tar -C /data -cf - . >"$f.tmp"
  ) || { rm -f "$f.tmp"; return 1; }
  mv "$f.tmp" "$f"
  printf '%s' "$f"
}

cadangkan_semua() { # label
  local f g
  mkdir -p "$DIR"; chmod 700 "$DIR"
  if db_lokal; then
    [[ "$(keadaan_kontainer db)" == healthy ]] || gagal "kontainer database tidak healthy — cadangan dibatalkan (lihat ./status.sh)"
    f="$(cadangkan_postgres db "$U" "$DB" "$1")" || gagal "pg_dump gagal — tidak ada cadangan baru yang dibuat"
    hijau "database : $f ($(du -h "$f" | cut -f1))"
  else
    kuning "COMPOSE_PROFILES tidak memuat \"db\": database berada di luar stack ini dan TIDAK dicadangkan di sini."
  fi
  g="$(cadangkan_unggahan "$1")" || gagal "cadangan berkas unggahan gagal"
  hijau "unggahan : $g ($(du -h "$g" | cut -f1))"
}

psql_admin() { dc exec -T db psql -v ON_ERROR_STOP=1 -U "$U" -d postgres -Atqc "$1"; }

case "$AKSI" in
  daftar)
    daftar
    ;;

  cadangkan)
    # Cadangan yang gagal diam-diam baru ketahuan pada hari ia dibutuhkan.
    # Trap EXIT (bukan ERR): `gagal` keluar lewat exit, yang tidak memicu ERR.
    trap 's=$?; (( s == 0 )) || kirim_alarm "[$PROYEK] cadangan GAGAL di $(hostname) — jalankan: sudo ./backup.sh"' EXIT
    cadangkan_semua "$LABEL"
    # Hanya berkas berpola nama cadangan yang dihapus, tidak pernah isi folder lain.
    find "$DIR" -maxdepth 1 -type f \( -name "$DB-*.dump" -o -name 'unggahan-*.tar' \) -mtime +"$SIMPAN" -delete
    ;;

  pulihkan)
    [[ -f "$BERKAS" && ! -L "$BERKAS" ]] || gagal "berkas tidak ditemukan: $BERKAS (lihat: sudo ./backup.sh --daftar)"
    case "$BERKAS" in
      *.dump) JENIS=database; db_lokal || gagal "database tidak dijalankan stack ini; pulihkan di server databasenya" ;;
      *.tar) JENIS=unggahan ;;
      *) gagal "jenis berkas tidak dikenal (harus .dump atau .tar)" ;;
    esac
    if (( ! YA )); then
      merah "Ini akan MENIMPA $JENIS yang sekarang dengan isi: $BERKAS"
      echo "Data saat ini dicadangkan dulu (label pra-pulih). Ulangi dengan --ya untuk melanjutkan."
      exit 1
    fi
    kunci_operasi
    IMG="$(image_alat)"; [[ -n "$IMG" ]] || gagal "image alat (layanan db di compose) tidak dikenali"

    # Semua yang bisa diperiksa, diperiksa SEBELUM ada yang dihentikan atau
    # dihapus: arsip yang rusak tidak boleh sampai mematikan layanan.
    langkah "Memeriksa $BERKAS"
    if [[ "$JENIS" == database ]]; then
      [[ "$(keadaan_kontainer db)" == healthy ]] || gagal "kontainer database tidak healthy"
      dc exec -T db pg_restore --list >/dev/null <"$BERKAS" || gagal "bukan dump pg_dump yang utuh — tidak ada yang diubah"
    else
      alat "$IMG" tar -tf - >/dev/null <"$BERKAS" || gagal "bukan arsip tar yang utuh — tidak ada yang diubah"
    fi

    langkah "Mencadangkan keadaan sekarang sebelum menimpa"
    cadangkan_semua pra-pulih

    # Dari sini API bisa dalam keadaan berhenti: apa pun yang terjadi, nyalakan
    # lagi dan beri tahu.
    API_BERHENTI=0
    trap 's=$?
      if (( s != 0 )); then
        merah "Pemulihan $JENIS GAGAL (kode $s)."
        (( ! API_BERHENTI )) || { kuning "Menyalakan API kembali."; dc_aktif up -d api || true; }
        kirim_alarm "[$PROYEK] pemulihan $JENIS GAGAL di $(hostname) — periksa: sudo ./status.sh"
      fi' EXIT

    if [[ "$JENIS" == database ]]; then
      # Dipulihkan ke database BARU lalu ditukar namanya. `pg_restore --clean`
      # di tempat hanya membuang objek yang ada di dump: tabel buatan migrasi
      # yang lebih baru akan tertinggal sementara catatan revisi Atlas kembali
      # ke keadaan lama, dan update berikutnya gagal "relation already exists".
      # Dengan cara ini, pemulihan yang gagal juga tidak menyentuh data sekarang.
      BARU="${DB}_pulih"; LAMA="${DB}_lama"
      langkah "Memulihkan ke database sementara $BARU"
      psql_admin "DROP DATABASE IF EXISTS \"$BARU\""
      psql_admin "CREATE DATABASE \"$BARU\" OWNER \"$U\""
      dc exec -T db pg_restore -U "$U" -d "$BARU" --no-owner --exit-on-error <"$BERKAS" \
        || { psql_admin "DROP DATABASE IF EXISTS \"$BARU\"" || true; gagal "pg_restore gagal — database yang sekarang TIDAK berubah"; }

      langkah "Menghentikan API dan menukar database"
      API_BERHENTI=1
      dc_aktif stop api
      psql_admin "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('$DB') AND pid <> pg_backend_pid()" >/dev/null
      psql_admin "DROP DATABASE IF EXISTS \"$LAMA\""
      psql_admin "ALTER DATABASE \"$DB\" RENAME TO \"$LAMA\""
      psql_admin "ALTER DATABASE \"$BARU\" RENAME TO \"$DB\"" \
        || { psql_admin "ALTER DATABASE \"$LAMA\" RENAME TO \"$DB\""; gagal "penukaran database gagal — database lama dikembalikan"; }
    else
      langkah "Menghentikan API dan memulihkan berkas unggahan"
      API_BERHENTI=1
      dc_aktif stop api
      # Folder dikosongkan lalu diisi dari arsip (sudah diperiksa utuh di atas);
      # pemilik = pengguna kontainer API.
      alat -v "$(volume_unggahan)":/data "$IMG" \
        sh -c 'find /data -mindepth 1 -delete && tar -C /data -xf - && chown -R 65532:65532 /data' <"$BERKAS" \
        || gagal "pemulihan unggahan gagal di tengah jalan — isi folder unggahan tidak lengkap; pulihkan dari cadangan pra-pulih: sudo ./backup.sh --daftar"
    fi

    langkah "Menyalakan API"
    dc_aktif up -d api
    tunggu_sehat api 90 || gagal "API tidak healthy sesudah pemulihan — lihat: sudo ./status.sh"
    API_BERHENTI=0
    if [[ "$JENIS" == database ]]; then
      psql_admin "DROP DATABASE IF EXISTS \"$LAMA\"" || kuning "database lama \"$LAMA\" tidak terhapus — hapus manual bila sudah yakin"
    fi
    hijau "Selesai: $JENIS dipulihkan dari $BERKAS"
    kirim_alarm "[$PROYEK] $JENIS dipulihkan dari $(basename "$BERKAS") di $(hostname)"
    ;;
esac
