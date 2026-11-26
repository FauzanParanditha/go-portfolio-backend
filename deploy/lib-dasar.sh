# shellcheck shell=bash
# ==========================================================================
#  lib-dasar.sh — fungsi bersama skrip operasional (install/update/status/
#  pantau/backup). Diambil dari deploy/lib.sh console-old-registry dan
#  pg-registry, dibuang bagian yang khusus proyek itu. Setiap fungsi di sini pernah menutup satu kejadian
#  nyata; alasannya ditulis di komentar supaya tidak "dirapikan" kembali.
#
#  Salin ke deploy/lib.sh di proyek, lalu di setiap skrip:
#     AKAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#     . "$AKAR/deploy/lib.sh"
#
#  Variabel yang boleh di-set SEBELUM disertakan:
#     PROYEK        nama proyek compose & awalan image   (bawaan: nama folder)
#     ENV_FILE      lokasi .env                           (bawaan: $AKAR/.env)
#     COMPOSE_UTAMA berkas compose utama                  (bawaan: $AKAR/docker-compose.yml)
# ==========================================================================

: "${AKAR:?AKAR harus di-set sebelum menyertakan lib-dasar.sh}"
PROYEK="${PROYEK:-$(basename "$AKAR")}"
ENV_FILE="${ENV_FILE:-$AKAR/.env}"
COMPOSE_UTAMA="${COMPOSE_UTAMA:-$AKAR/docker-compose.yml}"
LIB_DASAR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

# Server dengan umask ketat (027/077) membuat berkas yang di-bind-mount ke
# kontainer tidak terbaca proses non-root di dalamnya (nginx uid 101):
# "permission denied" hanya di server tertentu. Berkas rahasia diberi izin
# eksplisit (install -m 600, chmod 600, subshell umask 077), bukan lewat umask.
umask 022

# ------------------------------------------------------------------ keluaran
# Warna hanya ke terminal: keluaran yang masuk log/cron/Telegram tetap bersih.
if [[ -t 1 ]]; then
  merah()   { printf '\033[31m%s\033[0m\n' "$*"; }
  hijau()   { printf '\033[32m%s\033[0m\n' "$*"; }
  kuning()  { printf '\033[33m%s\033[0m\n' "$*"; }
  langkah() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
else
  merah()   { printf '%s\n' "$*"; }
  hijau()   { printf '%s\n' "$*"; }
  kuning()  { printf '%s\n' "$*"; }
  langkah() { printf '\n==> %s\n' "$*"; }
fi
gagal() { merah "GAGAL: $*" >&2; exit 1; }
butuh_root() { [[ $EUID -eq 0 ]] || gagal "jalankan sebagai root: sudo $0"; }
butuh_perintah() { local c; for c in "$@"; do command -v "$c" >/dev/null || gagal "$c tidak ditemukan"; done; }

# Satu proses saja (dua update bersamaan = dua build berebut versi).
# Pakai: kunci /var/lock/<proyek>-update.lock || gagal "update lain sedang berjalan"
kunci() { exec 9>"$1"; flock -n 9; }

# ------------------------------------------------------------------ .env
# Baca/tulis .env TANPA mengeksekusinya (jangan `source .env`). docker compose
# MENAFSIRKAN nilai tanpa kutip: $VAR/${…} diinterpolasi dan " #…" dibuang —
# sandi `Db$ecret #2024` pernah sampai ke kontainer sebagai "Db". Nilai
# berkarakter khusus ditulis dalam kutip tunggal (literal bagi compose).
env_get() {
  local v; v="$(grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -n1 | cut -d= -f2-)" || true
  [[ ${#v} -ge 2 && "$v" == \'*\' ]] && v="${v:1:${#v}-2}"
  printf '%s' "$v"
}
env_atau() { local v; v="$(env_get "$1")"; printf '%s' "${v:-$2}"; }
env_set() {
  local k="$1" v="$2" tmp
  # Kutip tunggal compose tidak dapat memuat ' dan tidak boleh diakhiri \.
  [[ "$v" != *"'"* && "$v" != *\\ && "$v" != *$'\n'* ]] \
    || gagal "nilai $k tidak boleh memuat kutip tunggal ('), baris baru, atau diakhiri \\"
  case "$v" in *[[:space:]\$\#\"\\]*) v="'$v'" ;; esac
  tmp="$(mktemp)"
  if grep -qE "^$k=" "$ENV_FILE" 2>/dev/null; then
    # Nilai lewat ENVIRON, bukan `awk -v` (yang memproses escape \t, \\ …).
    BARIS_ENV="$k=$v" awk -v k="$k" 'BEGIN{FS=OFS="="} $1==k{print ENVIRON["BARIS_ENV"]; next} {print}' "$ENV_FILE" >"$tmp"
  else
    cat "$ENV_FILE" >"$tmp" 2>/dev/null || true; printf '%s=%s\n' "$k" "$v" >>"$tmp"
  fi
  # `cat >` (bukan mv): izin 600 & pemilik berkas asli tetap.
  cat "$tmp" >"$ENV_FILE"; rm -f "$tmp"
}
# Nilai wajib: kosong atau masih placeholder <...> = berhenti dengan nama kuncinya.
wajib() { local v; v="$(env_get "$1")"; [[ -n "$v" && "$v" != \<*\> ]] || gagal "$1 belum diisi di $ENV_FILE"; }
# Rahasia acak alfanumerik (aman untuk .env, URL, dan compose).
rahasia() { openssl rand -base64 96 | tr -dc 'A-Za-z0-9' | head -c "${1:-48}"; }
# Rahasia dibuat SEKALI dan tidak pernah ditimpa (menimpa = semua sesi putus).
rahasia_sekali() { [[ -n "$(env_get "$1")" ]] || env_set "$1" "$(rahasia "${2:-48}")"; }

# Kunci baru di env.example ditambahkan ke .env tanpa menimpa nilai lama.
# `#NAMA=` (sengaja dimatikan) dianggap ada. Placeholder <...> tidak disalin:
# dulu user MySQL "<user_db>" terbaca sebagai nilai sungguhan.
env_lengkapi_dari() { # env.example
  local baris k v
  while IFS= read -r baris; do
    [[ "$baris" =~ ^([A-Z_][A-Z0-9_]*)=(.*)$ ]] || continue
    k="${BASH_REMATCH[1]}"; v="${BASH_REMATCH[2]}"
    grep -qE "^[[:space:]]*#?[[:space:]]*$k=" "$ENV_FILE" && continue
    [[ "$v" == \<*\> ]] && v=""
    printf '%s=%s\n' "$k" "$v" >>"$ENV_FILE"
    kuning "Variabel baru ditambahkan ke .env: $k${v:+=$v}"
  done <"$1"
}
# Kunci ganda: compose memakai baris TERAKHIR — sebab umum "nilai tidak terbaca".
# Sebab umumnya: env_lengkapi_dari menambah kunci KOSONG di bawah .env, lalu
# admin mengetik nilainya sebagai baris baru di tengah — baris kosong di bawah
# yang menang. Yang maksudnya jelas dirapikan otomatis (salinan .env disimpan):
#   · satu nilai terisi (baris lain kosong/sama) → baris terisi dipertahankan;
#   · semua kosong → satu baris dipertahankan.
# Dua nilai BERBEDA tidak dipilihkan — env_periksa_ganda memperingatkannya.
# Urutan di update.sh: env_lengkapi_dari → env_rapikan_ganda → env_periksa_ganda.
env_rapikan_ganda() {
  local rapi k cadangan
  [[ -n "$(grep -oE '^[A-Z_][A-Z0-9_]*=' "$ENV_FILE" 2>/dev/null | sort | uniq -d)" ]] || return 0
  rapi="$(mktemp)"
  awk '
    function kunci(b) { return match(b, /^[A-Z_][A-Z0-9_]*=/) ? substr(b, 1, RLENGTH - 1) : "" }
    function isi(b,  v) { v = substr(b, index(b, "=") + 1); return (v == "" || v == "\047\047" || v == "\"\"") ? "" : v }
    NR == FNR {
      k = kunci($0); if (k == "") next
      n[k]++; akhir[k] = FNR; v = isi($0)
      if (v != "") { if (!(k in nilai)) nilai[k] = v; else if (nilai[k] != v) beda[k] = 1; terisi[k] = FNR }
      next
    }
    {
      k = kunci($0)
      if (k != "" && n[k] > 1 && !(k in beda)) {
        simpan = (k in terisi) ? terisi[k] : akhir[k]
        if (FNR != simpan) { if (!(k in lapor)) { lapor[k] = 1; print k > "/dev/stderr" } next }
      }
      print
    }' "$ENV_FILE" "$ENV_FILE" >"$rapi" 2>"$rapi.k"
  if [[ -s "$rapi.k" ]]; then
    cadangan="$ENV_FILE.sebelum-rapi-$(date +%Y%m%d%H%M%S)"
    install -m 600 "$ENV_FILE" "$cadangan"
    cat "$rapi" >"$ENV_FILE"
    while IFS= read -r k; do kuning "Variabel ganda dirapikan: $k (baris berisi nilai dipertahankan; salinan lama: $cadangan)"; done <"$rapi.k"
  fi
  rm -f "$rapi" "$rapi.k"
}
env_periksa_ganda() {
  local k
  for k in $(grep -oE '^[A-Z_][A-Z0-9_]*=' "$ENV_FILE" | sort | uniq -d | tr -d '='); do
    kuning "PERHATIAN: $k muncul lebih dari sekali di .env dengan nilai BERBEDA (baris $(grep -nE "^$k=" "$ENV_FILE" | cut -d: -f1 | paste -sd, -)) — yang dipakai baris terakhir. Hapus yang salah."
  done
}

# ------------------------------------------------------------------ git
# git atas repo ini. Lewat sudo dan repo milik pengguna biasa, git dijalankan
# sebagai PEMILIK folder: root ditolak git ("dubious ownership"), tidak
# memegang kunci SSH GitHub pemilik, dan objek .git milik root mematahkan
# `git pull` pemilik berikutnya.
git_repo() {
  local p; p="$(stat -c %U "$AKAR" 2>/dev/null || echo root)"
  if [[ $EUID -eq 0 && "$p" != root ]]; then sudo -u "$p" -H git -C "$AKAR" "$@"
  else git -C "$AKAR" "$@"; fi
}
# Git yang pernah dijalankan sebagai root (`sudo git pull`) meninggalkan berkas
# milik root di .git dan pohon kerja; di server ber-umask ketat (077) berkas
# itu 600, sehingga git pemilik folder gagal "Permission denied" (.git/index)
# dan versi tercatat "manual-…". Dipanggil root sebelum git_repo: kembalikan
# .git, berkas terlacak, dan foldernya ke pemilik folder repo.
#
# Fungsi ini berjalan sebagai ROOT atas folder milik pengguna biasa, jadi
# pemilik folder tidak boleh bisa mengarahkannya. Tiga hal yang dulu bisa:
#  · git TIDAK dijalankan sebagai root. .git/config milik pemilik folder dapat
#    menyuruh git menjalankan program (core.fsmonitor, core.pager, …); dulu
#    `git ls-files` di sini mengeksekusinya sebagai root. Sekarang .git
#    dikembalikan dulu dengan chown (tanpa git), baru daftar berkas terlacak
#    ditanyakan ke git SEBAGAI PEMILIK (git_repo).
#  · Yang di-chown hanya path yang terlacak menurut git DAN ditemukan `find`
#    bukan milik pemilik — dan `find` tidak menyeberangi symlink. Dulu daftar
#    dari git dipakai langsung: folder terlacak yang diganti symlink ke luar
#    repo membuat root menyerahkan berkas di sana (mis. /etc/shadow) kepada
#    pemilik folder.
#  · Daftar dipisah NUL. Nama berkas boleh memuat baris baru, dan dengan
#    pemisah baris satu nama bisa menyamar sebagai dua path.
# Berkas biasa ber-hardlink lebih dari satu dilewati (nama kedua bisa berada di
# luar repo). Sisa yang diketahui: jeda sesaat antara `find` dan `chown` —
# menutupnya butuh chown lewat deskriptor berkas, yang tidak ada di bash.
# TIDAK BOLEH menghentikan pemanggil (set -e + pipefail): galat apa pun
# dianggap "tidak ada yang perlu dirapikan".
rapikan_pemilik_repo() {
  local p sp f d asing=""
  p="$(stat -c %U "$AKAR" 2>/dev/null || echo root)"
  [[ $EUID -eq 0 && "$p" != root && -d "$AKAR/.git" && ! -L "$AKAR/.git" ]] || return 0
  sp="$(mktemp -d 2>/dev/null)" || return 0   # milik root, 700

  # 1. .git lebih dulu, tanpa git. `chown -R -h` tidak mengikuti symlink.
  asing="$(find "$AKAR/.git" ! -user "$p" -print -quit 2>/dev/null || true)"
  [[ -z "$asing" ]] || chown -R -h "$p": "$AKAR/.git" 2>/dev/null || true

  # 2. Kandidat: isi pohon kerja yang bukan milik pemilik, sebagaimana DILIHAT
  #    `find` (tidak pernah masuk lewat symlink).
  find "$AKAR" -mindepth 1 -xdev -path "$AKAR/.git" -prune -o \
       ! -user "$p" \( -type d -o -links 1 \) -printf '%P\0' 2>/dev/null \
    | LC_ALL=C sort -zu >"$sp/kandidat" || true

  if [[ -s "$sp/kandidat" ]]; then
    # 3. Yang boleh disentuh: berkas terlacak dan folder-folder induknya.
    { git_repo ls-files -z 2>/dev/null || true; } |
      while IFS= read -r -d '' f; do
        printf '%s\0' "$f"; d="$f"
        while [[ "$d" == */* ]]; do d="${d%/*}"; printf '%s\0' "$d"; done
      done | LC_ALL=C sort -zu >"$sp/boleh" || true
    LC_ALL=C comm -z -12 "$sp/boleh" "$sp/kandidat" >"$sp/irisan" 2>/dev/null || true
    if [[ -s "$sp/irisan" ]]; then
      [[ -n "$asing" ]] || asing="$AKAR/$(tr '\0' '\n' <"$sp/irisan" | sed -n 1p)"
      ( cd "$AKAR" && xargs -0 -r chown -h "$p": -- <"$sp/irisan" ) 2>/dev/null || true
    fi
  fi
  rm -rf -- "$sp"

  [[ -z "$asing" ]] \
    || kuning "berkas repo milik selain $p (mis. $asing) — git pernah dijalankan dengan sudo? Dikembalikan ke $p. Jangan 'sudo git'; update.sh yang menarik kode." >&2
  return 0
}
versi_baru() { printf '%s-%s' "$(git_repo rev-parse --short HEAD 2>/dev/null || echo manual)" "$(date +%Y%m%d%H%M)"; }

# ------------------------------------------------------------------ docker
# compose HANYA dengan berkas utama; versi image dipilih lewat override.
dc() { docker compose -p "$PROYEK" -f "$COMPOSE_UTAMA" --env-file "$ENV_FILE" "$@"; }
# Versi AKTIF: berkas utama + override (tulis_override) bila ada. Dipakai unit
# systemd, pantau.sh, dan perintah admin — BUKAN di tengah update, karena
# override akan menimpa VERSI baru yang sedang diuji.
dc_aktif() {
  local o; o="$(dirname "$COMPOSE_UTAMA")/docker-compose.override.yml"
  if [[ -f "$o" ]]; then docker compose -p "$PROYEK" -f "$COMPOSE_UTAMA" -f "$o" --env-file "$ENV_FILE" "$@"
  else dc "$@"; fi
}

# healthy | unhealthy | starting | running | exited | restarting | absen
keadaan_kontainer() {
  local id; id="$(dc ps -a -q "$1" 2>/dev/null | head -n1 || true)"
  [[ -n "$id" ]] || { echo absen; return; }
  docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id" 2>/dev/null || echo absen
}
# Layanan tanpa healthcheck dianggap siap begitu "running"; yang punya
# healthcheck HARUS healthy. unhealthy/exited = berhenti menunggu lebih awal.
tunggu_sehat() { # layanan [detik]
  local s="$1" batas="${2:-120}" t=0 st
  while (( t < batas )); do
    st="$(keadaan_kontainer "$s")"
    case "$st" in
      healthy) return 0 ;;
      running) docker inspect -f '{{if .State.Health}}ada{{end}}' "$(dc ps -q "$s")" 2>/dev/null | grep -q ada || return 0 ;;
      unhealthy|exited) return 1 ;;
    esac
    sleep 3; t=$((t + 3))
  done
  return 1
}

# ------------------------------------------------------------------ versi & rollback
# Baris "<layanan> <versi-aktif> <versi-sebelumnya>". Image bertanda versi
# lama TIDAK dihapus sampai ada SIMPAN versi lebih baru — itu yang membuat
# rollback cukup satu perintah.
VERSI_FILE="${VERSI_FILE:-$AKAR/deploy/.versi}"
# Berkas belum ada (pemasangan pertama) = kosong, bukan galat: awk keluar 2
# untuk berkas yang hilang dan `set -e` menghentikan skrip pemanggil.
versi_aktif() { [[ -f "$VERSI_FILE" ]] || return 0; awk -v a="$1" '$1==a{print $2}' "$VERSI_FILE"; }
versi_lama()  { [[ -f "$VERSI_FILE" ]] || return 0; awk -v a="$1" '$1==a{print $3}' "$VERSI_FILE"; }
catat_versi() { # layanan aktif sebelumnya
  local tmp; tmp="$(mktemp)"
  { awk -v a="$1" '$1!=a' "$VERSI_FILE" 2>/dev/null || true; echo "$1 $2 $3"; } >"$tmp"
  cat "$tmp" >"$VERSI_FILE"; rm -f "$tmp"; chmod 600 "$VERSI_FILE"
}
# `docker compose up` polos oleh admin tetap memakai versi aktif.
tulis_override() { # layanan...
  local s v
  {
    echo "# Dibuat oleh skrip operasional — versi image aktif. Jangan disunting."
    echo "services:"
    for s in "$@"; do
      v="$(versi_aktif "$s")"; [[ -n "$v" ]] || continue
      echo "  $s:"; echo "    image: $PROYEK/$s:$v"
    done
  } >"$(dirname "$COMPOSE_UTAMA")/docker-compose.override.yml"
}
# Hapus image lama, sisakan N terbaru + yang aktif + yang sebelumnya.
pangkas_image() { # layanan [simpan]
  local s="$1" n="${2:-3}" a l tag
  a="$(versi_aktif "$s")"; l="$(versi_lama "$s")"
  docker image ls "$PROYEK/$s" --format '{{.CreatedAt}}\t{{.Tag}}' | sort -r | cut -f2 | tail -n +$((n + 1)) |
    while read -r tag; do
      [[ "$tag" == "$a" || "$tag" == "$l" || "$tag" == "<none>" ]] && continue
      docker image rm "$PROYEK/$s:$tag" >/dev/null 2>&1 || true
    done
}

# ------------------------------------------------------------------ jaringan docker
# Subnet tetap di compose (172.28.0.0/24 …) bentrok dengan jaringan kantor/VPN
# yang kebetulan memakai rentang sama: server tidak lagi dapat menjawab klien
# di rentang itu. Pilih /24 yang belum dipakai jaringan Docker lain maupun rute
# host, SEKALI, lalu simpan di .env (compose: subnet: ${DOCKER_SUBNET}).
# Pemeriksaan per /16: rute yang lebih luas (mis. 10.0.0.0/8) tidak terdeteksi —
# di server seperti itu isi ketiga kunci secara manual.
pilih_subnet() { # → "subnet_depan ip_proxy subnet_dalam"
  local terpakai c n
  terpakai="$( {
      for n in $(docker network ls -q 2>/dev/null || true); do
        docker network inspect "$n" --format '{{range .IPAM.Config}}{{.Subnet}} {{end}}' 2>/dev/null || true
      done
      { ip -4 route 2>/dev/null || true; } | awk '{print $1}'
    } | tr ' ' '\n' | grep -E '^[0-9]+\.' | cut -d. -f1,2 | sort -u || true)"
  for c in 172.29 172.30 172.31 10.213 10.214; do
    grep -qx "$c" <<<"$terpakai" && continue
    echo "$c.77.0/24 $c.77.10 $c.78.0/24"; return 0
  done
  return 1
}
# Kunci .env: DOCKER_SUBNET, DOCKER_SUBNET_DALAM, PROXY_IP — IP tetap reverse
# proxy di jaringan "depan", satu-satunya yang dipercaya app untuk
# X-Forwarded-For (BUKAN gateway bridge: klien IPv6/localhost masuk lewat
# docker-proxy dan tampak berasal dari gateway).
atur_jaringan() {
  local sn ip dl hasil
  [[ -n "$(env_get DOCKER_SUBNET)" && -n "$(env_get PROXY_IP)" && -n "$(env_get DOCKER_SUBNET_DALAM)" ]] && return 0
  hasil="$(pilih_subnet)" || gagal "tidak ada subnet bebas; isi DOCKER_SUBNET, PROXY_IP, DOCKER_SUBNET_DALAM di $ENV_FILE"
  read -r sn ip dl <<<"$hasil"
  env_set DOCKER_SUBNET "$sn"; env_set PROXY_IP "$ip"; env_set DOCKER_SUBNET_DALAM "$dl"
  hijau "jaringan Docker: depan $sn (proxy $ip), dalam $dl"
}

# ------------------------------------------------------------------ menyala saat boot
# Unit systemd oneshot: stack naik sesudah docker & jaringan, dan dapat
# dikendalikan `systemctl start|stop <proyek>`. `restart: unless-stopped` saja
# tidak cukup — kontainer yang di-stop manual sebelum reboot tidak kembali.
# Path proyek tidak boleh memuat kutip tunggal.
pasang_systemd() {
  [[ -d /run/systemd/system ]] || { kuning "systemd tidak ada, unit $PROYEK.service dilewati (kontainer tetap restart: unless-stopped)"; return 0; }
  local muat="AKAR='$AKAR' PROYEK='$PROYEK' ENV_FILE='$ENV_FILE' COMPOSE_UTAMA='$COMPOSE_UTAMA'; . '$LIB_DASAR'"
  cat >"/etc/systemd/system/$PROYEK.service" <<UNIT
[Unit]
Description=$PROYEK (docker compose)
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=$AKAR
ExecStart=/bin/bash -c "$muat; dc_aktif up -d --remove-orphans"
ExecStop=/bin/bash -c "$muat; dc_aktif stop"
TimeoutStartSec=300

[Install]
WantedBy=multi-user.target
UNIT
  chmod 644 "/etc/systemd/system/$PROYEK.service"
  systemctl daemon-reload
  systemctl enable docker "$PROYEK.service" >/dev/null 2>&1
  hijau "menyala saat boot: docker.service dan $PROYEK.service"
}

# ------------------------------------------------------------------ cadangan database
# pg_dump format custom lewat kontainer DB. Ditulis ke .tmp lalu di-mv: dump
# yang terputus tidak pernah tampak sebagai cadangan sah. Folder 700, berkas
# 600 sejak dibuat (umask 077 di subshell). Kunci .env: BACKUP_DIR.
cadangkan_postgres() { # layanan user db [label] → path berkas
  local svc="$1" u="$2" db="$3" label="${4:-harian}" dir f
  dir="$(env_atau BACKUP_DIR "/var/backups/$PROYEK")"
  mkdir -p "$dir"; chmod 700 "$dir"
  f="$dir/$db-$(date +%Y%m%d-%H%M%S)-$label.dump"
  ( umask 077; dc exec -T "$svc" pg_dump -U "$u" -d "$db" -Fc --no-owner >"$f.tmp" ) || { rm -f "$f.tmp"; return 1; }
  mv "$f.tmp" "$f"
  printf '%s' "$f"
}

# ------------------------------------------------------------------ alamat & port
# Docker hanya dapat mengikat port di alamat yang terpasang di antarmuka server
# ini. Dulu APP_BIND diisi IP NAT/IP proxy depan dan `compose up` berhenti
# dengan "cannot assign requested address" — DI TENGAH update.
ip_server() { ip -o addr show 2>/dev/null | awk '{split($4,a,"/"); print a[1]}'; }
daftar_ip_server() { ip_server | grep -vE '^(127\.|::1$|fe80:)' | paste -sd' ' -; }
alamat_ikat_sah() { # alamat → 0 bila Docker dapat mengikat port di situ
  case "$1" in ""|0.0.0.0|::|127.*|::1) return 0 ;; esac
  command -v ip >/dev/null || return 0
  ip_server | grep -qxF "$1"
}
port_sah() { [[ "$1" =~ ^[0-9]+$ ]] && (( $1 >= 1 && $1 <= 65535 )); }
# Siapa yang memakai port host (selain kontainer proyek ini). Kosong = bebas.
pemakai_port() { # alamat port
  local alamat="$1" port="$2" c baris
  c="$(docker ps --filter "publish=$port" --format '{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null \
    | awk -F'|' -v p="$PROYEK" '$2!=p{print $1; exit}')"
  [[ -z "$c" ]] || { printf 'kontainer %s' "$c"; return 0; }
  docker ps -q --filter "publish=$port" 2>/dev/null | grep -q . && return 0
  baris="$(ss -Htlnp 2>/dev/null | awk -v p="$port" -v a="$alamat" '{
      n=split($4,x,":"); if (x[n]!=p) next
      la=substr($4,1,length($4)-length(p)-1); gsub(/^\[|\]$/,"",la)
      if (la=="*"||la=="0.0.0.0"||la=="::"||a=="0.0.0.0"||a=="::"||la==a) {print; exit}
    }')"
  [[ -n "$baris" ]] || return 0
  c="$(grep -o 'users:(("[^"]*' <<<"$baris" | cut -d'"' -f2)"
  printf 'proses %s di %s' "${c:-tidak dikenal}" "$(awk '{print $4}' <<<"$baris")"
}
# Panggil SEBELUM `compose up`: berhenti lebih awal dengan pesan yang jelas.
periksa_port() { # alamat port [nama]
  local siapa
  alamat_ikat_sah "$1" || { merah "Alamat $1 tidak terpasang di server ini. Alamat server: $(daftar_ip_server)"; return 1; }
  port_sah "$2" || { merah "Port '${3:-$2}' tidak sah: $2"; return 1; }
  siapa="$(pemakai_port "$1" "$2")"
  [[ -z "$siapa" ]] || { merah "Port $1:$2${3:+ ($3)} sudah dipakai $siapa."; return 1; }
}

# Port SSH yang BENAR-BENAR dipakai (sshd_config / ssh.socket Ubuntu 24.04).
# Profil "OpenSSH" UFW hanya membuka 22 — server dengan SSH di port lain
# pernah terkunci begitu firewall diaktifkan.
port_ssh() {
  { sshd -T 2>/dev/null | awk '$1=="port"{print $2}'
    systemctl show -p Listen ssh.socket 2>/dev/null | grep -oE ':[0-9]+ \(Stream\)' | tr -dc '0-9\n'; } \
    | awk '/^[0-9]+$/ && $1>=1 && $1<=65535' | sort -un | xargs
}

# ------------------------------------------------------------------ rahasia di baris perintah
# Token/URL rahasia untuk curl lewat berkas konfigurasi (-K <(…)), BUKAN
# argumen: argumen proses terbaca semua pengguna lokal lewat `ps`.
#   curl -s -K <(curl_header_bearer "$TOKEN") "$URL"
curl_header_bearer() { printf 'header = "Authorization: Bearer %s"\n' "$1"; }
curl_url_rahasia()   { printf 'url = "%s"\n' "$1"; }

# Kirim alarm ke webhook (Slack/Mattermost/Google Chat) dan/atau Telegram.
# Kunci .env: ALARM_WEBHOOK_URL, ALARM_TELEGRAM_BOT_TOKEN, ALARM_TELEGRAM_CHAT_ID
kirim_alarm() { # teks
  local teks="$1" url tok chat
  url="$(env_get ALARM_WEBHOOK_URL)"
  tok="$(env_get ALARM_TELEGRAM_BOT_TOKEN)"; chat="$(env_get ALARM_TELEGRAM_CHAT_ID)"
  if [[ -n "$url" ]]; then
    jq -n --arg t "$teks" '{text:$t, content:$t}' |
      curl -s -m 10 -o /dev/null -w '%{http_code}' -H 'content-type: application/json' --data-binary @- -K <(curl_url_rahasia "$url") |
      grep -qE '^2' || kuning "PERINGATAN: webhook alarm gagal dikirim" >&2
  fi
  if [[ -n "$tok" && -n "$chat" ]]; then
    curl -s -m 10 -o /dev/null -K <(curl_url_rahasia "https://api.telegram.org/bot$tok/sendMessage") \
      --data-urlencode "chat_id=$chat" --data-urlencode "text=$teks" || kuning "PERINGATAN: alarm Telegram gagal dikirim" >&2
  fi
}

# ------------------------------------------------------------------ proses lokal (dev/demo)
# Hentikan HANYA proses yang MENDENGARKAN port. Tanpa -sTCP:LISTEN, lsof ikut
# mengembalikan klien yang terhubung (peramban, Playwright, curl) dan ikut
# dibunuh. `pkill -f "PORT=…"` juga tidak pernah cocok: env meng-exec node.
# Penerus port VM Docker di macOS (colima: ssh/limactl; Docker Desktop:
# com.docker.*, vpnkit, gvproxy) TIDAK pernah dibunuh: dulu itu memutus socket
# Docker sampai colima di-restart. Port kontainer: `docker compose stop` dulu.
# Lebih aman lagi: simpan $! proses yang dijalankan sendiri dan hentikan itu.
henti_port() {
  local p pid nama
  for p in "$@"; do
    for pid in $(lsof -ti "tcp:$p" -sTCP:LISTEN 2>/dev/null || true); do
      nama="$(ps -p "$pid" -o comm= 2>/dev/null || true)"
      case "${nama##*/}" in
        ssh|limactl|lima*|colima|com.docker*|vpnkit*|gvproxy|docker-proxy|qemu*)
          kuning "port $p dipegang $nama (penerus Docker) — tidak dihentikan; hentikan kontainernya" >&2 ;;
        *) kill "$pid" 2>/dev/null || true ;;
      esac
    done
  done
}
