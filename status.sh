#!/usr/bin/env bash
# ==========================================================================
#  status.sh — "apakah backend berjalan, dan kalau tidak, salahnya di mana?"
#
#     ./status.sh             untuk manusia: tiap komponen + sebab + saran
#     ./status.sh --ringkas   satu baris per komponen
#     ./status.sh --json      untuk mesin (pemantau, cron)
#
#  Kode keluar: 0 normal · 1 terganggu (sebagian berfungsi) · 2 mati
#               3 tidak bisa memeriksa (argumen salah, .env/Docker tidak terbaca)
#
#  Tidak mengubah apa pun. Jalankan dengan sudo: deploy/.env dan folder
#  cadangan hanya terbaca root.
# ==========================================================================
set -uo pipefail
AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy/lib.sh
. "$AKAR/deploy/lib.sh"

# "Tidak bisa memeriksa" bukan "terganggu": pemantau harus bisa membedakannya.
gagal() { merah "GAGAL: $*" >&2; exit 3; }

MODE=penuh
for a in "$@"; do
  case "$a" in
    --ringkas) MODE=ringkas ;;
    --json) MODE=json ;;
    -h|--help) sed -n '2,13p' "$0" | sed 's/^# \{0,2\}//'; exit 0 ;;
    *) gagal "argumen tidak dikenal: $a (lihat --help)" ;;
  esac
done

butuh_env
[[ -r "$ENV_FILE" ]] || gagal "$ENV_FILE tidak terbaca — jalankan: sudo $0 $*"
butuh_perintah docker curl
# Compose yang tidak bisa membaca konfigurasinya membuat semua kontainer tampak
# "absen" — itu kegagalan memeriksa, bukan layanan yang mati.
dc config -q 2>/dev/null || gagal "docker compose tidak bisa membaca konfigurasi — lihat: docker compose -f $COMPOSE_UTAMA --env-file $ENV_FILE config -q"

# Versi yang BENAR-BENAR berjalan (tag image kontainer), bukan yang tercatat.
versi_jalan() {
  local id; id="$(dc ps -q "$1" 2>/dev/null | head -n1 || true)"
  [[ -n "$id" ]] || return 0
  docker inspect -f '{{.Config.Image}}' "$id" 2>/dev/null | sed 's/^.*://'
}

# Seperti keadaan_kontainer, tetapi kontainer yang TIDAK berjalan dilaporkan
# dengan statusnya (exited, restarting, …): Docker tetap menyimpan hasil
# healthcheck terakhir, sehingga kontainer yang dihentikan tampak "unhealthy"
# dan sarannya jadi salah arah.
keadaan() {
  local id; id="$(dc ps -a -q "$1" 2>/dev/null | head -n1 || true)"
  [[ -n "$id" ]] || { echo absen; return; }
  docker inspect -f '{{if ne .State.Status "running"}}{{.State.Status}}{{else if .State.Health}}{{.State.Health.Status}}{{else}}running{{end}}' "$id" 2>/dev/null || echo absen
}

# Tiap butir: id|nama|keadaan(ok/terganggu/mati)|kritis(1/0)|pesan|saran
BUTIR=()
# "|" adalah pemisah kolom dan karakter kendali merusak JSON: keduanya dibuang
# dari pesan (bisa berasal dari nilai deploy/.env).
bersih() { printf '%s' "$1" | tr '|' '/' | tr -d '\000-\037'; }
catat() { BUTIR+=("$1|$2|$3|$4|$(bersih "$5")|$(bersih "${6:-}")"); }

# ---- kontainer API
st="$(keadaan api)"
case "$st" in
  healthy) catat api_kontainer "Kontainer API" ok 1 "healthy, versi $(versi_jalan api)" ;;
  starting) catat api_kontainer "Kontainer API" terganggu 1 "baru menyala, belum healthy" "tunggu ±30 detik lalu periksa lagi" ;;
  absen|exited) catat api_kontainer "Kontainer API" mati 1 "$st" "sudo systemctl restart $PROYEK   (belum pernah dipasang: sudo ./install.sh)" ;;
  restarting) catat api_kontainer "Kontainer API" mati 1 "terus mati lalu menyala lagi" "lihat sebabnya: docker logs --tail 50 ${PROYEK}-api-1 — biasanya konfigurasi di deploy/.env ditolak saat start" ;;
  *) catat api_kontainer "Kontainer API" mati 1 "$st" "lihat sebabnya: docker logs --tail 50 ${PROYEK}-api-1" ;;
esac

# ---- versi yang berjalan vs yang tercatat. Berbeda = update terputus di
#      tengah: boot berikutnya akan menyalakan versi yang TERCATAT.
jalan="$(versi_jalan api)"; catat_v="$(versi_aktif api)"
if [[ -n "$jalan" && -n "$catat_v" && "$jalan" != "$catat_v" ]]; then
  catat versi "Versi" terganggu 0 "berjalan $jalan, tercatat $catat_v" "samakan dengan menjalankan: sudo ./update.sh --no-pull   (atau --rollback)"
fi

# ---- API menjawab (dari server ini, lewat port yang dipublikasikan)
URL="$(url_lokal)"
kode="$(curl -s -o /dev/null -m 5 -w '%{http_code}' "$URL/healthz" 2>/dev/null)" || kode=000
if [[ "$kode" == 200 ]]; then
  catat api_hidup "API menjawab" ok 1 "$URL/healthz → 200"
else
  catat api_hidup "API menjawab" mati 1 "$URL/healthz → $kode" "periksa ALAMAT_IKAT/PORT_API di deploy/.env dan log kontainer API"
fi

# ---- database, dilihat DARI DALAM aplikasi. Inilah yang membedakan
#      "kontainer sehat tetapi database mati": me-restart API tidak menolong.
if [[ "$kode" == 200 ]]; then
  kode_db="$(curl -s -o /dev/null -m 6 -w '%{http_code}' "$URL/readyz" 2>/dev/null)" || kode_db=000
  if [[ "$kode_db" == 200 ]]; then
    catat db_dari_api "Database (dari API)" ok 1 "terjangkau"
  elif db_lokal; then
    catat db_dari_api "Database (dari API)" mati 1 "/readyz → $kode_db" "masalahnya di database, BUKAN di API — jangan restart API; lihat butir kontainer database"
  else
    catat db_dari_api "Database (dari API)" mati 1 "/readyz → $kode_db" "database di luar server ini tidak terjangkau: periksa DB_DSN dan jaringan ke sana; restart API tidak menolong"
  fi
fi

# ---- kontainer database (hanya bila ikut dijalankan di sini)
if db_lokal; then
  st="$(keadaan db)"
  case "$st" in
    healthy) catat db_kontainer "Kontainer database" ok 1 "healthy" ;;
    starting) catat db_kontainer "Kontainer database" terganggu 1 "baru menyala" "tunggu ±30 detik" ;;
    absen|exited) catat db_kontainer "Kontainer database" mati 1 "$st" "sudo systemctl restart $PROYEK" ;;
    *) catat db_kontainer "Kontainer database" mati 1 "$st" "docker logs --tail 50 ${PROYEK}-db-1 — sering karena disk penuh" ;;
  esac
fi

# ---- cadangan terbaru
dir="$(env_atau BACKUP_DIR "/var/backups/$PROYEK")"
apa="Cadangan"
if [[ -d "$dir" && ! -r "$dir" ]]; then
  catat cadangan "$apa" terganggu 0 "folder $dir tidak terbaca" "jalankan dengan sudo"
else
  # Yang dicari: dump database bila database dijalankan di sini, selain itu
  # arsip unggahan (satu-satunya yang dicadangkan stack ini).
  if db_lokal; then pola='*.dump'; apa="Cadangan database"; else pola='unggahan-*.tar'; apa="Cadangan unggahan"; fi
  baru="$(find "$dir" -maxdepth 1 -name "$pola" -type f -printf '%T@ %f\n' 2>/dev/null | sort -rn | sed -n 1p)"
  if [[ -z "$baru" ]]; then
    catat cadangan "$apa" terganggu 0 "belum ada cadangan di $dir" "sudo ./backup.sh"
  else
    umur=$(( ( $(date +%s) - ${baru%%.*} ) / 3600 ))
    if (( umur > 48 )); then
      catat cadangan "$apa" terganggu 0 "terbaru ${baru#* } berumur ${umur} jam" "periksa cron /etc/cron.d/$PROYEK dan jalankan: sudo ./backup.sh"
    else
      catat cadangan "$apa" ok 0 "terbaru ${baru#* } (${umur} jam lalu)"
    fi
  fi
fi

# ---- disk (database berhenti menulis saat penuh)
# Yang diukur: sistem berkas terpenuh di antara folder repo dan tempat Docker
# menyimpan image, volume, dan log.
pakai="$(df -P "$AKAR" "$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || echo /var/lib/docker)" 2>/dev/null \
  | awk 'NR>1{gsub("%","",$5); if ($5+0 > m) m=$5+0} END{if (NR>1) print m}')"
if [[ -n "$pakai" ]]; then
  if (( pakai >= 90 )); then
    catat disk "Disk" terganggu 0 "terpakai ${pakai}%" "docker builder prune -f; hapus cadangan lama; periksa: docker system df"
  else
    catat disk "Disk" ok 0 "terpakai ${pakai}%"
  fi
fi

# ---- keadaan keseluruhan: komponen kritis mati = mati; selain itu yang tidak ok = terganggu
KELUAR=0
for b in "${BUTIR[@]}"; do
  IFS='|' read -r _ _ k kritis _ _ <<<"$b"
  [[ "$k" == ok ]] && continue
  if [[ "$k" == mati && "$kritis" == 1 ]]; then KELUAR=2; elif (( KELUAR < 1 )); then KELUAR=1; fi
done
case $KELUAR in 0) SEMUA=normal ;; 1) SEMUA=terganggu ;; *) SEMUA=mati ;; esac

json_teks() { local s="$1"; s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; printf '"%s"' "$s"; }

case "$MODE" in
  json)
    printf '{"proyek":%s,"keadaan":%s,"waktu":%s,"komponen":[' "$(json_teks "$PROYEK")" "$(json_teks "$SEMUA")" "$(json_teks "$(date -Is)")"
    pisah=""
    for b in "${BUTIR[@]}"; do
      IFS='|' read -r id nama k kritis pesan saran <<<"$b"
      printf '%s{"id":%s,"nama":%s,"keadaan":%s,"kritis":%s,"pesan":%s,"saran":%s}' "$pisah" \
        "$(json_teks "$id")" "$(json_teks "$nama")" "$(json_teks "$k")" \
        "$([[ $kritis == 1 ]] && echo true || echo false)" "$(json_teks "$pesan")" "$(json_teks "$saran")"
      pisah=","
    done
    printf ']}\n'
    ;;
  ringkas)
    for b in "${BUTIR[@]}"; do
      IFS='|' read -r _ nama k _ pesan _ <<<"$b"
      printf '%-10s %s: %s\n' "$k" "$nama" "$pesan"
    done
    printf '%s: %s\n' "$PROYEK" "$SEMUA"
    ;;
  *)
    printf '%s — %s\n\n' "$PROYEK" "$(date '+%Y-%m-%d %H:%M:%S')"
    for b in "${BUTIR[@]}"; do
      IFS='|' read -r _ nama k _ pesan saran <<<"$b"
      case "$k" in ok) hijau "  ✓ $nama: $pesan" ;; terganggu) kuning "  ! $nama: $pesan" ;; *) merah "  ✗ $nama: $pesan" ;; esac
      [[ -z "$saran" ]] || printf '      → %s\n' "$saran"
    done
    printf '\n'
    case $KELUAR in 0) hijau "Keadaan: normal" ;; 1) kuning "Keadaan: terganggu" ;; *) merah "Keadaan: mati" ;; esac
    ;;
esac
exit $KELUAR
