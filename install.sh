#!/usr/bin/env bash
# ==========================================================================
#  install.sh — memasang backend di server ini (Ubuntu/Debian + Docker).
#
#     sudo ./install.sh                    bertanya untuk nilai yang belum diisi
#     sudo ./install.sh --non-interactive  memakai deploy/.env yang sudah diisi
#     sudo ./install.sh --bangun-lokal     bangun image di server ini, alih-alih
#                                          menarik yang dibangun GitHub
#
#  Aman diulang: tiap langkah melewati dirinya bila sudah selesai, jadi
#  menjalankannya lagi adalah cara mengubah konfigurasi. Menjalankan ulang
#  TIDAK mengganti versi kode yang sedang berjalan dan TIDAK menjalankan
#  migrasi — keduanya tugas ./update.sh (yang mencadangkan dulu dan punya jalan
#  mundur).
#
#  Pemasangan pertama: prasyarat → deploy/.env + rahasia → jaringan Docker →
#  image → database → migrasi → admin pertama → API → menyala saat boot →
#  cadangan harian → ./status.sh
# ==========================================================================
set -Eeuo pipefail
AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy/lib.sh
. "$AKAR/deploy/lib.sh"

INTERAKTIF=1; PAKSA_LOKAL=0
for a in "$@"; do
  case "$a" in
    --non-interactive) INTERAKTIF=0 ;;
    --bangun-lokal) PAKSA_LOKAL=1 ;;
    -h|--help) sed -n '2,19p' "$0" | sed 's/^# \{0,2\}//'; exit 0 ;;
    *) gagal "argumen tidak dikenal: $a (lihat --help)" ;;
  esac
done
[[ -t 0 ]] || INTERAKTIF=0

butuh_root
kunci_operasi
trap 'merah "install.sh berhenti di baris $LINENO. Perbaiki sebabnya lalu jalankan lagi — aman diulang."' ERR

# tanya KUNCI "pertanyaan": nilai tersimpan menjadi bawaan (Enter = pakai itu);
# "-" mengosongkan; placeholder <...> dianggap belum diisi.
tanya() {
  local k="$1" t="$2" lama jawab
  lama="$(env_get "$k")"; [[ "$lama" == \<*\> ]] && lama=""
  if (( INTERAKTIF )); then
    read -r -p "$t${lama:+ [$lama]}: " jawab || true
    if [[ "$jawab" == "-" ]]; then env_set "$k" ""
    else
      jawab="${jawab:-$lama}"
      [[ -z "$jawab" ]] || env_set "$k" "$jawab"
    fi
  fi
}

# ---------------------------------------------------------------- 1. prasyarat
langkah "1/10 Prasyarat"
peringatkan_pemilik_repo
ram="$(awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
bebas="$(df -Pm "$AKAR" | awk 'NR==2{print $4}')"
echo "RAM ${ram} MB · disk bebas ${bebas} MB"
(( ram >= 900 )) || kuning "RAM di bawah 1 GB: build image bisa gagal kehabisan memori."
(( bebas >= 3000 )) || kuning "Disk bebas di bawah 3 GB: image, database, dan cadangan butuh ruang."

if ! command -v docker >/dev/null; then
  command -v apt-get >/dev/null || gagal "Docker belum terpasang dan ini bukan sistem apt — pasang Docker Engine + compose + buildx secara manual"
  langkah "Memasang Docker dari repo resmi Docker"
  # shellcheck disable=SC1091
  . /etc/os-release
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL "https://download.docker.com/linux/$ID/gpg" -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/$ID ${VERSION_CODENAME} stable" \
    >/etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  # Pengaturan daemon HANYA untuk Docker yang baru saja dipasang skrip ini.
  # Docker yang sudah ada mungkin melayani layanan lain: no-new-privileges dan
  # restart daemon tidak boleh dipaksakan kepada mereka.
  if [[ ! -e /etc/docker/daemon.json ]]; then
    mkdir -p /etc/docker
    printf '%s\n' '{' '  "log-driver": "json-file",' '  "log-opts": { "max-size": "20m", "max-file": "5" },' \
      '  "live-restore": true,' '  "no-new-privileges": true' '}' >/etc/docker/daemon.json
    systemctl restart docker
  fi
fi
butuh_perintah docker git curl openssl flock
butuh_compose
hijau "Docker $(docker version --format '{{.Server.Version}}'), compose $(docker compose version --short)"

# ---------------------------------------------------------------- 2. .env
langkah "2/10 Konfigurasi (deploy/.env)"
if [[ ! -f "$ENV_FILE" ]]; then
  install -m 600 -o root -g root "$ENV_CONTOH" "$ENV_FILE"
  hijau "dibuat dari deploy/env.example (milik root, izin 600)"
else
  # Berkas yang disiapkan pengguna biasa sebelum pemasangan: jadikan milik root.
  chown root:root "$ENV_FILE"; chmod 600 "$ENV_FILE"
  env_lengkapi_dari "$ENV_CONTOH"
fi
env_rapikan_ganda
env_periksa_ganda

tanya APP_PUBLIC_URL   "Alamat backend dari sisi browser (mis. https://api.contoh.id)"
tanya APP_FRONTEND_URL "Alamat frontend (mis. https://contoh.id)"
# Origin CORS bawaan = alamat frontend.
cors="$(env_get CORS_ALLOWED_ORIGINS)"
[[ -n "$cors" && "$cors" != \<*\> ]] || env_set CORS_ALLOWED_ORIGINS "$(env_get APP_FRONTEND_URL)"
tanya CORS_ALLOWED_ORIGINS "Origin yang diizinkan CORS (pisahkan dengan koma)"
tanya SEED_ADMIN_EMAIL "Email admin pertama"
tanya ALAMAT_IKAT      "Alamat ikat port API (127.0.0.1 = hanya server ini, 0.0.0.0 = terbuka)"
tanya PORT_API         "Port API di server ini"

rahasia_sekali JWT_SECRET 64
if db_lokal; then
  rahasia_sekali POSTGRES_PASSWORD 32
  [[ -n "$(env_get DB_DSN)" ]] || env_set DB_DSN "$(susun_dsn)"
  wajib POSTGRES_PASSWORD
fi
for k in APP_PUBLIC_URL APP_FRONTEND_URL CORS_ALLOWED_ORIGINS SEED_ADMIN_EMAIL JWT_SECRET DB_DSN; do wajib "$k"; done

IKAT="$(env_atau ALAMAT_IKAT 127.0.0.1)"
PORT="$(env_atau PORT_API 8080)"
case "$(env_get APP_PUBLIC_URL)" in
  https://*) ;;
  *)
    kuning "APP_PUBLIC_URL bukan https: lewat HTTP polos, password dan token admin terkirim TANPA ENKRIPSI,"
    kuning "dan sesi admin di browser tidak bertahan (cookie-nya bertanda Secure). Situs publik tetap jalan."
    if [[ "$IKAT" == 0.0.0.0 && "$(env_get IZINKAN_HTTP_TERBUKA)" != ya ]]; then
      gagal "ALAMAT_IKAT=0.0.0.0 dengan APP_PUBLIC_URL non-https membuka login admin ke internet tanpa enkripsi. Pasang reverse proxy ber-TLS (docs/DEPLOYMENT.md), atau — bila ini keputusan sadar dan sementara — isi IZINKAN_HTTP_TERBUKA=ya di deploy/.env."
    fi ;;
esac
[[ "$IKAT" != 0.0.0.0 ]] \
  || kuning "ALAMAT_IKAT=0.0.0.0: port API terbuka ke internet dan MELEWATI UFW. /healthz dan /readyz akar ikut terbuka."

# ---------------------------------------------------------------- 3. jaringan
langkah "3/10 Jaringan Docker"
atur_jaringan
echo "depan $(env_get DOCKER_SUBNET) · dalam $(env_get DOCKER_SUBNET_DALAM)"

# ---------------------------------------------------------------- 4. image
langkah "4/10 Image"
catat_cabang
VERSI_PAKAI="$(versi_aktif api)"
if [[ -n "$VERSI_PAKAI" ]] && docker image inspect "$PROYEK/api:$VERSI_PAKAI" >/dev/null 2>&1; then
  echo "versi $VERSI_PAKAI sudah terpasang — tidak dibangun ulang (pakai ./update.sh untuk versi baru)"
  PERTAMA=0
else
  VERSI_PAKAI="$(versi_baru)"
  siapkan_image "$VERSI_PAKAI" || gagal "image tidak tersedia — lihat pesan di atas"
  PERTAMA=1
fi

# ---------------------------------------------------------------- 5. database
langkah "5/10 Database"
if db_lokal; then
  dc up -d db
  tunggu_sehat db 120 || { dc logs --tail 30 db || true; gagal "database tidak healthy"; }
  hijau "PostgreSQL healthy"
else
  echo "COMPOSE_PROFILES tidak memuat \"db\": memakai database di luar stack ini (DB_DSN)"
fi

# ---------------------------------------------------------------- 6. migrasi
langkah "6/10 Migrasi"
if (( PERTAMA )); then
  migrasi
else
  # Kode yang ter-checkout bisa lebih baru daripada versi yang berjalan (mis.
  # update.sh sudah menarik kode lalu gagal membangun). Menerapkan migrasinya
  # di sini berarti mengubah skema tanpa cadangan dan tanpa versi pasangannya.
  echo "dilewati — migrasi dijalankan ./update.sh bersama versi barunya"
fi

# ---------------------------------------------------------------- 7. admin pertama
langkah "7/10 Admin pertama"
CATATAN="/root/$PROYEK-admin-awal.txt"
if (( PERTAMA )); then
  # Password dibuat di sini, diteruskan ke seeder lewat ENVIRONMENT (bukan
  # argumen perintah, bukan deploy/.env), dan tidak pernah masuk kontainer API.
  # Seeder idempoten: admin yang sudah ada tidak diubah — dan untuk kasus itu
  # tidak ada catatan yang ditulis, supaya tidak ada password palsu beredar.
  SANDI_ADMIN="$(rahasia 24)"
  HASIL_SEED="$(SEED_ADMIN_NAME="$(env_atau SEED_ADMIN_NAME Admin)" SEED_ADMIN_EMAIL="$(env_get SEED_ADMIN_EMAIL)" \
    SEED_ADMIN_PASSWORD="$SANDI_ADMIN" \
    dcv "$VERSI_PAKAI" run --rm --no-deps -e SEED_ADMIN_NAME -e SEED_ADMIN_EMAIL -e SEED_ADMIN_PASSWORD \
      -e SEED_SAMPLE_DATA=false --entrypoint /app/seed api 2>&1)" \
    || { printf '%s\n' "$HASIL_SEED" | tail -15; gagal "seeder gagal"; }
  if grep -q 'admin user created' <<<"$HASIL_SEED"; then
    ( umask 077
      printf 'Admin pertama %s\nemail   : %s\npassword: %s\n\nSimpan di pengelola sandi, lalu HAPUS berkas ini.\nPassword hanya bisa diganti lewat "lupa password" (butuh SMTP di deploy/.env).\n' \
        "$PROYEK" "$(env_get SEED_ADMIN_EMAIL)" "$SANDI_ADMIN" >"$CATATAN" )
    hijau "admin dibuat; kredensialnya ditulis ke $CATATAN (hanya terbaca root)"
  else
    echo "admin dengan email itu sudah ada di database — tidak diubah, tidak ada password baru"
  fi
  unset SANDI_ADMIN HASIL_SEED
else
  echo "dilewati — admin dibuat sekali, saat pemasangan pertama"
fi

# ---------------------------------------------------------------- 8. API
langkah "8/10 API"
# Selalu diperiksa (kontainer proyek ini sendiri dikecualikan): alamat/port yang
# salah harus ketahuan SEBELUM compose membuang kontainer yang sedang berjalan.
periksa_port "$IKAT" "$PORT" PORT_API || gagal "port API tidak bisa dipakai"
dcv "$VERSI_PAKAI" up -d api
tunggu_sehat api 90 || { dc logs --tail 40 api || true; gagal "API tidak healthy — lihat log di atas"; }
if (( PERTAMA )); then catat_versi api "$VERSI_PAKAI" ""; fi
tulis_override $LAYANAN
hijau "API healthy, versi $(versi_aktif api)"

# ---------------------------------------------------------------- 9. boot
langkah "9/10 Menyala saat boot"
pasang_systemd
# Unit baru hanya "enabled": tanpa di-start, systemd menganggapnya belum aktif
# dan `systemctl stop` tidak menghentikan apa pun. Start di sini tidak membuat
# ulang kontainer yang sudah berjalan.
if [[ -d /run/systemd/system ]]; then systemctl start "$PROYEK.service"; fi

# ---------------------------------------------------------------- 10. cadangan harian
langkah "10/10 Cadangan harian"
if [[ -d /etc/cron.d ]]; then
  printf '%s\n' "# Dibuat install.sh $PROYEK — cadangan database + unggahan tiap hari 02:30." \
    "30 2 * * * root $AKAR/backup.sh >>/var/log/$PROYEK-backup.log 2>&1" >"/etc/cron.d/$PROYEK"
  chmod 644 "/etc/cron.d/$PROYEK"
  hijau "cron: /etc/cron.d/$PROYEK (log: /var/log/$PROYEK-backup.log)"
else
  kuning "/etc/cron.d tidak ada — jadwalkan sendiri: sudo $AKAR/backup.sh"
fi
# Cadangan pertama sekarang juga: membuktikan cadangan bisa dibuat di server
# ini sebelum ada data yang berharga, bukan pada malam pertama.
DIR_CADANGAN="$(env_atau BACKUP_DIR "/var/backups/$PROYEK")"
if [[ -z "$(find "$DIR_CADANGAN" -maxdepth 1 -type f \( -name '*.dump' -o -name 'unggahan-*.tar' \) 2>/dev/null | head -n1)" ]]; then
  "$AKAR/backup.sh" --label=pasang
fi

trap - ERR
langkah "Selesai — keadaan sekarang"
"$AKAR/status.sh" || true
