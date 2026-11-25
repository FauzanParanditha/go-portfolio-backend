#!/usr/bin/env bash
# Gerbang rilis — dijalankan sebelum mengirim perubahan (`task periksa` /
# `npm run periksa`). Tidak bergantung pada runner: CI dan server polos bisa
# menjalankannya langsung (bash tools/periksa.sh).
#
# Delapan pemeriksaan, masing-masing menolak satu kelas kesalahan. Tidak berhenti
# pada kegagalan pertama: semuanya dijalankan, lalu dilaporkan sekaligus,
# supaya satu putaran cukup untuk tahu semua yang salah.
#
# Pemeriksaannya sama di semua stack; perintahnya dipilih otomatis dari berkas
# di akar repo (package.json, go.mod/go.work, Cargo.toml, pyproject.toml).
# Monorepo Go + web (go.work + package.json, tata-letak.md) dikenali sebagai
# STACK=go+node: Go diperiksa per modul di go.work, web lewat workspace npm.
# Nilai di blok KONFIGURASI menimpa tebakan otomatis.
#
# Asal: skill project-standard (scripts/periksa.sh). Perubahan lokal, semuanya
# bertanda [portfolio]:
#   - KECUALI: pengecualian sempit untuk pemeriksaan 3 (pola berbahaya);
#   - PERINTAH_PENJAGA: pemeriksaan 5 boleh berupa perintah (mis. test yang
#     menelusuri router asli), bukan hanya grep per berkas;
#   - ROUTE_ALASAN: alasan tertulis bila pemeriksaan 5 memang tidak berlaku.

set -uo pipefail
# Akar repo: lewat git bila ada, kalau tidak dari letak skrip (tools/periksa.sh).
AKAR="$(git rev-parse --show-toplevel 2>/dev/null)" || AKAR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$AKAR" || exit 1

# ─────────────────────────── KONFIGURASI ───────────────────────────
# Kosongkan (atau biarkan) untuk tebakan otomatis.
STACK="go"               # node | go | go+node | rust | python
SUMBER=()                # folder yang dipindai pola berbahaya
PERINTAH_CEK=()          # pemeriksaan statis
PERINTAH_BUILD=()
# Tanpa bungkus bash -c supaya alat yang belum terpasang dilaporkan dengan
# namanya. Pasang: go install golang.org/x/vuln/cmd/govulncheck@latest
PERINTAH_AUDIT=(govulncheck ./...)
PERINTAH_UJI=()          # uji asap; kosong = belum ada
POLA=""                  # pola berbahaya (regex grep -E)
EKSTENSI=()              # berkas yang dipindai pola, mis. (--include='*.go' --include='*.ts'); kosong = per stack
LEWATI_DIR=(tmp uploads) # tmp = keluaran air (make dev); uploads = berkas unggahan, bukan sumber
# [portfolio] Temuan pola berbahaya yang SAH: satu regex (grep -E) per baris,
# dicocokkan ke "berkas:isi baris" (tanpa nomor baris, spasi awal dibuang).
# Tulis sesempit mungkin dan beri alasan di komentar — jangan melebarkan POLA.
KECUALI=()

# Yang DIKIRIM: folder bundel peramban dan/atau binary/folder hasil build
# layanan — SEMUANYA (monorepo: bin DAN apps/web/dist). Inilah yang dipindai
# rahasia. Kosong = tebakan per stack; tidak ada yang ditemukan = dilewati.
KIRIM=(bin)              # tiga binary hasil build: api, seed, import

# Nama rahasia dan alamat internal yang TIDAK BOLEH ada di yang dikirim.
# Tambahkan nama env rahasia proyek ini, dan alamat backend internal yang
# PERSIS — bukan awalan rentang. Awalan seperti '10\.10\.' akan mencocoki
# placeholder di form ("10.10.10.10") dan menghasilkan alarm palsu; putaran
# pertama memang akan ramai, dan menyetelnya justru bagian yang berguna.
# Header PEM ditulis lengkap: "PRIVATE KEY" saja cocok dengan pesan galat
# crypto/tls di pustaka standar Go (alarm palsu di setiap binary Go).
RAHASIA='SESSION_SECRET|DB_PASSWORD|-----BEGIN [A-Z ]*PRIVATE KEY-----|createCipheriv'
# Contoh menambah host internal yang persis:
#   RAHASIA="$RAHASIA|10\.10\.13\.175|PANDI-SECURITY-0\.1"

# Invarian: tiap route/handler harus melewati salah satu penjaga ini.
# ROUTE_DIR kosong = dilewati. Untuk router (Go/Rust/Python), arahkan ke folder
# handler dan isi PENJAGA dengan nama middleware/fungsi auth.
ROUTE_DIR=""
ROUTE_FILE=""            # pola nama berkas, mis. route.ts | *_handler.go | *.py
PENJAGA='getAuth|requireSession|izin\(|butuhSesi'
# Jalur yang memang publik — satu baris per awalan path. Menambah baris di
# sini adalah keputusan sadar, dan itulah gunanya daftar ini.
PUBLIK=()
# [portfolio] Bila diisi, pemeriksaan 5 menjalankan perintah ini (dan ROUTE_DIR
# diabaikan). ROUTE_ALASAN: alasan pemeriksaan 5 tidak berlaku di repo ini.
# Test ini menelusuri SEMUA route router asli: tanpa kredensial harus 401,
# /admin/* dengan token non-admin harus 403, kecuali daftar publik eksplisit di
# internal/http/router_penjaga_test.go (routePublik).
PERINTAH_PENJAGA=(go test ./internal/http -run '^TestSemuaRouteBerpenjaga$' -count=1)
ROUTE_ALASAN=""
# ───────────────────────── AKHIR KONFIGURASI ─────────────────────────

# ---- tebakan per stack (hanya mengisi yang masih kosong) ----
if [ -z "$STACK" ]; then
  if { [ -f go.work ] || [ -f go.mod ]; } && [ -f package.json ]; then STACK=go+node
  elif [ -f go.work ] || [ -f go.mod ]; then STACK=go
  elif [ -f Cargo.toml ]; then STACK=rust
  elif [ -f pyproject.toml ] || [ -f requirements.txt ]; then STACK=python
  elif [ -f package.json ]; then STACK=node
  fi
fi
ada() { command -v "$1" >/dev/null 2>&1; }
EKSTENSI_ATUR=${#EKSTENSI[@]}   # >0 = diisi di KONFIGURASI, jangan ditimpa tebakan
CALON_KIRIM=()   # dicari SESUDAH build — artefaknya baru ada setelah langkah 2
case "$STACK" in
  node)
    [ ${#SUMBER[@]} -eq 0 ] && SUMBER=(app components lib hooks src apps packages)
    [ ${#PERINTAH_CEK[@]} -eq 0 ] && PERINTAH_CEK=(npx tsc --noEmit)
    [ ${#PERINTAH_BUILD[@]} -eq 0 ] && PERINTAH_BUILD=(npm run build)
    [ ${#PERINTAH_AUDIT[@]} -eq 0 ] && PERINTAH_AUDIT=(npm audit --omit=dev --audit-level=moderate)
    [ -z "$POLA" ] && POLA='dangerouslySetInnerHTML|\.innerHTML\s*=|\beval\s*\(|new Function\s*\(|document\.write|child_process'
    CALON_KIRIM=(.next/static dist/client build/static dist)
    [ "$EKSTENSI_ATUR" -gt 0 ] || EKSTENSI=(--include='*.ts' --include='*.tsx' --include='*.js' --include='*.jsx' --include='*.mjs')
    ;;
  go|go+node)
    # Per modul: dengan go.work, `./...` di akar tidak mencakup modul apps/api.
    PER_MODUL='for d in $(go list -m -f "{{.Dir}}"); do (cd "$d" && eval "$1") || exit 1; done'
    GO_CEK=(bash -c "$PER_MODUL" _ 'go vet ./... && test -z "$(gofmt -l . | grep -v /vendor/)"')
    GO_BUILD=(bash -c "mkdir -p bin && b=\$PWD/bin; $PER_MODUL" _ 'go build -o "$b/" ./...')
    GO_AUDIT=(bash -c "$PER_MODUL" _ 'govulncheck ./...')
    if [ "$STACK" = go+node ]; then
      [ ${#SUMBER[@]} -eq 0 ] && SUMBER=(apps packages cmd internal)
      [ ${#PERINTAH_CEK[@]} -eq 0 ] && PERINTAH_CEK=(bash -c '"$@" && npm run typecheck --workspaces --if-present' _ "${GO_CEK[@]}")
      [ ${#PERINTAH_BUILD[@]} -eq 0 ] && PERINTAH_BUILD=(bash -c '"$@" && npm run build --workspaces --if-present' _ "${GO_BUILD[@]}")
      [ ${#PERINTAH_AUDIT[@]} -eq 0 ] && PERINTAH_AUDIT=(bash -c '"$@" && npm audit --omit=dev --audit-level=moderate' _ "${GO_AUDIT[@]}")
      [ -z "$POLA" ] && POLA='InsecureSkipVerify:\s*true|exec\.Command\(|"unsafe"|Sprintf\("(SELECT|INSERT|UPDATE|DELETE)|"math/rand"|dangerouslySetInnerHTML|\.innerHTML\s*=|\beval\s*\(|new Function\s*\(|document\.write'
      CALON_KIRIM=(bin apps/web/dist)
      [ "$EKSTENSI_ATUR" -gt 0 ] || EKSTENSI=(--include='*.go' --include='*.ts' --include='*.tsx')
      ROUTE_FILE="${ROUTE_FILE:-*.go}"
    else
      [ ${#SUMBER[@]} -eq 0 ] && SUMBER=(.)
      [ ${#PERINTAH_CEK[@]} -eq 0 ] && PERINTAH_CEK=("${GO_CEK[@]}")
      [ ${#PERINTAH_BUILD[@]} -eq 0 ] && PERINTAH_BUILD=("${GO_BUILD[@]}")
      [ ${#PERINTAH_AUDIT[@]} -eq 0 ] && PERINTAH_AUDIT=("${GO_AUDIT[@]}")
      [ -z "$POLA" ] && POLA='InsecureSkipVerify:\s*true|exec\.Command\(|"unsafe"|Sprintf\("(SELECT|INSERT|UPDATE|DELETE)|math/rand'
      CALON_KIRIM=(bin)
      [ "$EKSTENSI_ATUR" -gt 0 ] || EKSTENSI=(--include='*.go')
    fi
    ;;
  rust)
    [ ${#SUMBER[@]} -eq 0 ] && SUMBER=(src crates)
    [ ${#PERINTAH_CEK[@]} -eq 0 ] && PERINTAH_CEK=(bash -c 'cargo clippy --all-targets -- -D warnings && cargo fmt --check')
    [ ${#PERINTAH_BUILD[@]} -eq 0 ] && PERINTAH_BUILD=(cargo build --release)
    [ ${#PERINTAH_AUDIT[@]} -eq 0 ] && PERINTAH_AUDIT=(cargo audit)
    [ -z "$POLA" ] && POLA='\bunsafe\s*\{|danger_accept_invalid_certs\(true\)|Command::new\('
    CALON_KIRIM=(target/release)
    [ "$EKSTENSI_ATUR" -gt 0 ] || EKSTENSI=(--include='*.rs')
    ;;
  python)
    [ ${#SUMBER[@]} -eq 0 ] && SUMBER=(src app .)
    [ ${#PERINTAH_CEK[@]} -eq 0 ] && PERINTAH_CEK=(bash -c 'ruff check . && { ! command -v mypy >/dev/null || mypy .; }')
    [ ${#PERINTAH_BUILD[@]} -eq 0 ] && PERINTAH_BUILD=(python -m build)
    [ ${#PERINTAH_AUDIT[@]} -eq 0 ] && PERINTAH_AUDIT=(pip-audit)
    [ -z "$POLA" ] && POLA='\beval\(|\bexec\(|shell\s*=\s*True|pickle\.loads|\byaml\.load\(|verify\s*=\s*False'
    CALON_KIRIM=(dist)
    [ "$EKSTENSI_ATUR" -gt 0 ] || EKSTENSI=(--include='*.py')
    ;;
esac

gagal=0
SP="${TMPDIR:-/tmp}/periksa-$$"; mkdir -p "$SP"
trap 'rm -rf -- "$SP"' EXIT

judul() { printf '\n\033[1;34m== %s\033[0m\n' "$*"; }
ok()    { printf '\033[32m✓ %s\033[0m\n' "$*"; }
salah() { printf '\033[31m✗ %s\033[0m\n' "$*"; gagal=1; }
lewat() { printf '\033[33m– %s\033[0m\n' "$*"; }

# jalankan <nama> <log> perintah… — gagal bila alatnya tidak terpasang (bukan lulus diam-diam)
jalankan() {
  local nama="$1" log="$2"; shift 2
  if [ $# -eq 0 ]; then lewat "$nama: tidak dikonfigurasi"; return; fi
  if ! ada "$1"; then salah "$nama: alat '$1' tidak terpasang"; return; fi
  if "$@" > "$SP/$log" 2>&1; then ok "$nama"; else tail -25 "$SP/$log"; salah "$nama"; fi
}

printf 'Stack: %s\n' "${STACK:-tidak dikenali — isi STACK di KONFIGURASI}"

ada_sumber=()
for d in "${SUMBER[@]}"; do [ -d "$d" ] && ada_sumber+=("$d"); done

judul "1. pemeriksaan statis"
jalankan "pemeriksaan statis" cek.log "${PERINTAH_CEK[@]}"

judul "2. build"
jalankan "build" build.log "${PERINTAH_BUILD[@]}"

judul "3. pola berbahaya di sumber"
if [ ${#ada_sumber[@]} -eq 0 ] || [ -z "$POLA" ]; then lewat "tidak ada folder sumber/pola yang cocok"
else
  lewati=(--exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=target --exclude-dir=.git
          --exclude-dir=dist --exclude-dir=.next --exclude-dir=.venv --exclude-dir=.output --exclude-dir=bin)
  for d in ${LEWATI_DIR[@]+"${LEWATI_DIR[@]}"}; do lewati+=(--exclude-dir="$d"); done
  temu=$(grep -rnE "$POLA" "${ada_sumber[@]}" ${EKSTENSI[@]+"${EKSTENSI[@]}"} "${lewati[@]}" 2>/dev/null)
  # [portfolio] buang temuan yang cocok dengan salah satu KECUALI
  if [ -n "$temu" ] && [ ${#KECUALI[@]} -gt 0 ]; then
    sisa=""
    while IFS= read -r baris; do
      kunci=$(printf '%s' "$baris" | sed -E 's/^([^:]+):[0-9]+:[[:space:]]*/\1:/')
      sah=0
      for k in "${KECUALI[@]}"; do printf '%s' "$kunci" | grep -qE -- "$k" && sah=1; done
      [ $sah -eq 1 ] || sisa+="$baris"$'\n'
    done <<< "$temu"
    temu="${sisa%$'\n'}"
  fi
  if [ -n "$temu" ]; then echo "$temu"; salah "pola berbahaya ditemukan (beri pengecualian sempit beralasan bila memang sah)"
  else ok "tidak ada"; fi
fi

# Inilah pemeriksaan yang paling sering menyelamatkan: langkah build meng-inline
# nilai environment, dan isi .env cenderung tersalin ke konfigurasi deployment
# dan image. Satu grep menutup seluruh kelas itu.
judul "4. rahasia di yang dikirim"
# KIRIM diisi = semuanya wajib ada (artefak hilang = gagal, bukan lewat).
# Kosong = semua CALON_KIRIM yang ada (monorepo: binary DAN bundel web).
if [ ${#KIRIM[@]} -eq 0 ]; then WAJIB_KIRIM=0; for d in ${CALON_KIRIM[@]+"${CALON_KIRIM[@]}"}; do [ -e "$d" ] && KIRIM+=("$d"); done; else WAJIB_KIRIM=1; fi
ada_kirim=(); for d in ${KIRIM[@]+"${KIRIM[@]}"}; do [ -e "$d" ] && ada_kirim+=("$d"); done
if [ ${#ada_kirim[@]} -eq 0 ] || [ ${#ada_kirim[@]} -ne ${#KIRIM[@]} ]; then
  if [ "$WAJIB_KIRIM" -eq 1 ]; then salah "artefak yang dikirim tidak lengkap (${KIRIM[*]}) — build gagal?"
  else lewat "tidak ada artefak untuk dipindai (isi KIRIM, atau build gagal?)"; fi
else
  bocor=$(grep -rlaE "$RAHASIA" "${ada_kirim[@]}" 2>/dev/null)
  env_ikut=$(find "${ada_kirim[@]}" \( -name '.env' -o -name '*.pem' -o -name '*.key' \) 2>/dev/null)
  if [ -n "$bocor$env_ikut" ]; then printf '%s\n%s\n' "$bocor" "$env_ikut"; salah "rahasia, alamat internal, atau berkas kunci ada di yang dikirim"
  else ok "bersih: $(for d in "${ada_kirim[@]}"; do printf '%s %s  ' "$d" "$(du -sh "$d" 2>/dev/null | cut -f1)"; done)"; fi
fi

judul "5. tiap route punya penjaga"
# [portfolio] perintah (test router) didahulukan; alasan tertulis bila tidak berlaku
if [ ${#PERINTAH_PENJAGA[@]} -gt 0 ]; then jalankan "tiap route berpenjaga" penjaga.log "${PERINTAH_PENJAGA[@]}"
elif [ -n "$ROUTE_ALASAN" ]; then lewat "tidak berlaku: $ROUTE_ALASAN"
elif [ -z "$ROUTE_DIR" ] || [ ! -d "$ROUTE_DIR" ]; then lewat "ROUTE_DIR belum diisi — isi agar invarian ini dijaga"
else
  tanpa=""
  while IFS= read -r f; do
    publik=0
    for p in "${PUBLIK[@]}"; do case "$f" in "$p"*) publik=1;; esac; done
    [ $publik -eq 1 ] && continue
    grep -qE "$PENJAGA" "$f" || tanpa+="$f"$'\n'
  done < <(find "$ROUTE_DIR" -name "${ROUTE_FILE:-*}" -type f | sort)
  if [ -n "$tanpa" ]; then printf '%s' "$tanpa"; salah "route tanpa penjaga (tambahkan penjaga, atau daftarkan sebagai publik bila memang publik)"
  else ok "semua route berpenjaga"; fi
fi

judul "6. audit dependensi produksi"
jalankan "audit dependensi" audit.log "${PERINTAH_AUDIT[@]}"

judul "7. uji asap"
if [ ${#PERINTAH_UJI[@]} -gt 0 ]; then jalankan "uji asap" uji.log "${PERINTAH_UJI[@]}"
else lewat "belum ada — isi PERINTAH_UJI setelah uji asap dibuat"; fi

judul "8. aturan kontainer"
# Aturan Docker yang murah diperiksa statis (references/docker.md). Tidak
# menggantikan build & jalankan image; menolak kesalahan yang paling sering:
# image :latest, kontainer root, rahasia ikut ke konteks build, port DB
# dipublikasikan ke semua antarmuka (Docker melewati UFW/CSF).
_lewati='\( -name node_modules -o -name .git -o -name vendor -o -name target -o -name .venv -o -name dist \) -prune'
DF_DAFTAR=$(eval "find . $_lewati -o -type f \( -name Dockerfile -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) -print" 2>/dev/null | sort)
DC_DAFTAR=$(eval "find . $_lewati -o -type f \( -name '*compose*.yml' -o -name '*compose*.yaml' \) -print" 2>/dev/null | sort)
if [ -z "$DF_DAFTAR$DC_DAFTAR" ]; then lewat "tidak ada Dockerfile/compose"
else
  temuan=""
  _tag_sah() { # image → kosong bila bertag persis, atau alasan
    local n="${1##*/}"
    case "$n" in
      *@sha256:*) ;;
      *:latest) echo "memakai :latest" ;;
      *:*) ;;
      *) echo "tanpa tag versi" ;;
    esac
  }
  while IFS= read -r df; do
    [ -n "$df" ] || continue
    tahap=$(grep -E '^FROM[[:space:]]' "$df" | awk 'tolower($(NF-1))=="as"{print $NF}')
    # Image dasar: baris FROM dan ARG *IMAGE=… (pola ARG NODE_IMAGE=node:24.x).
    while IFS= read -r img; do
      case "$img" in ''|scratch|*'$'*) continue ;; esac
      printf '%s\n' "$tahap" | grep -qxF "$img" && continue
      a=$(_tag_sah "$img"); [ -z "$a" ] || temuan+="$df: image dasar $img $a"$'\n'
    done < <( { grep -E '^FROM[[:space:]]' "$df" | sed -E 's/^FROM[[:space:]]+(--[^[:space:]]+[[:space:]]+)*//' | awk '{print $1}'
                grep -E '^ARG[[:space:]]+[A-Za-z_]*IMAGE=' "$df" | sed -E 's/^ARG[[:space:]]+[^=]+=//; s/["'\'']//g'; } )
    # Tahap akhir non-root: USER sesudah FROM terakhir, atau image dasar yang memang non-root.
    akhir=$(grep -E '^FROM[[:space:]]' "$df" | tail -1 | sed -E 's/^FROM[[:space:]]+(--[^[:space:]]+[[:space:]]+)*//' | awk '{print $1}')
    case "$akhir" in *'$'*)
      v=$(printf '%s' "$akhir" | sed -E 's/^\$\{?([A-Za-z_][A-Za-z0-9_]*).*/\1/')
      akhir=$(grep -E "^ARG[[:space:]]+$v=" "$df" | head -1 | sed -E 's/^ARG[[:space:]]+[^=]+=//') ;;
    esac
    pengguna=$(awk '/^FROM[[:space:]]/{u=""} /^USER[[:space:]]/{u=$2} END{print u}' "$df")
    case "$pengguna" in
      ''|root|0|root:*|0:*)
        printf '%s' "$akhir" | grep -qE 'nonroot|unprivileged|rootless' \
          || temuan+="$df: tahap akhir berjalan sebagai root (tambahkan USER non-root)"$'\n' ;;
    esac
    # HEALTHCHECK di image; pengecualian sempit: baris "# periksa: tanpa-healthcheck <alasan>".
    grep -qE '^HEALTHCHECK[[:space:]]' "$df" || grep -qE '^#[[:space:]]*periksa:[[:space:]]*tanpa-healthcheck[[:space:]]+[^[:space:]]' "$df" \
      || temuan+="$df: tidak ada HEALTHCHECK (atau tulis '# periksa: tanpa-healthcheck <alasan>')"$'\n'
    # .dockerignore yang mengecualikan .env, di akar repo atau di samping Dockerfile.
    di=""
    for c in "$df.dockerignore" "$(dirname "$df")/.dockerignore" ./.dockerignore; do [ -f "$c" ] && { di="$c"; break; }; done
    if [ -z "$di" ]; then temuan+="$df: tidak ada .dockerignore — COPY bisa membawa .env dan kunci ke image"$'\n'
    elif ! grep -qE '(^|/)\.env([.*]|[[:space:]]*$)' "$di"; then temuan+="$di: tidak mengecualikan .env"$'\n'; fi
  done <<< "$DF_DAFTAR"
  while IFS= read -r dc; do
    [ -n "$dc" ] || continue
    temuan+=$(awk '
      function ind(s) { match(s, /^ */); return RLENGTH }
      /^[[:space:]]*#/ { next }
      /^[[:space:]]*image:[[:space:]]*/ {
        s = $0; sub(/^[[:space:]]*image:[[:space:]]*/, "", s); gsub(/["\047]/, "", s); sub(/[[:space:]]+#.*$/, "", s)
        if (s !~ /\$/) {
          n = s; sub(/.*\//, "", n)
          if (n ~ /:latest$/) print FILENAME ":" FNR ": image " s " memakai :latest"
          else if (n !~ /[:@]/) print FILENAME ":" FNR ": image " s " tanpa tag versi"
        }
      }
      /^[[:space:]]*privileged:[[:space:]]*true/ { print FILENAME ":" FNR ": privileged: true" }
      /^[[:space:]]*network_mode:[[:space:]]*["\047]?host/ { print FILENAME ":" FNR ": network_mode: host (melewati isolasi jaringan)" }
      /^[[:space:]]*ports:[[:space:]]*$/ { dlm = 1; pi = ind($0); next }
      dlm {
        if ($0 ~ /^[[:space:]]*$/) next
        if (ind($0) <= pi) dlm = 0
        else if ($0 ~ /^[[:space:]]*-/) {
          s = $0; sub(/^[[:space:]]*-[[:space:]]*/, "", s); gsub(/["\047]/, "", s); sub(/[[:space:]]+#.*$/, "", s)
          gsub(/\$\{[^}]*\}/, "X", s)
          if (s !~ /^[a-z_]+:/ && gsub(/:/, ":", s) < 2)
            print FILENAME ":" FNR ": port tanpa alamat ikat (terbuka di semua antarmuka, melewati UFW) — tulis 127.0.0.1: atau 0.0.0.0: secara eksplisit"
          next
        }
      }' "$dc")$'\n'
  done <<< "$DC_DAFTAR"
  temuan=$(printf '%s' "$temuan" | awk 'NF && !s[$0]++')
  if [ -n "$temuan" ]; then printf '%s\n' "$temuan"; salah "aturan kontainer dilanggar (references/docker.md)"
  else ok "$(printf '%s\n' "$DF_DAFTAR" | grep -c . | tr -d ' ') Dockerfile, $(printf '%s\n' "$DC_DAFTAR" | grep -c . | tr -d ' ') compose sesuai aturan"; fi
fi

printf '\n'
if [ $gagal -eq 0 ]; then printf '\033[1;32mSEMUA LULUS\033[0m\n'; else printf '\033[1;31mADA YANG GAGAL — lihat tanda ✗ di atas\033[0m\n'; fi
exit $gagal
