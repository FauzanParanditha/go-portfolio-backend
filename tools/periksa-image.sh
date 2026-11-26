#!/usr/bin/env bash
# Memindai ISI sebuah image sebelum dipublikasikan:
#     bash tools/periksa-image.sh <image>
#
# Image di registry publik bisa dibongkar siapa saja, LAPIS DEMI LAPIS: berkas
# yang ditambahkan di satu lapisan lalu dihapus di lapisan berikutnya tetap ada
# di lapisan pertama. Karena itu nama berkas diperiksa di SEMUA lapisan, bukan
# hanya di sistem berkas akhirnya. Sekali terbit, isi image tidak bisa ditarik.
#
# Dipanggil .github/workflows/image.yml sesudah build dan SEBELUM push; bisa
# juga dijalankan di laptop terhadap image hasil build lokal. Butuh docker.
set -euo pipefail

IMG="${1:?pakai: bash tools/periksa-image.sh <image>}"

# ─────────────────────────── KONFIGURASI ───────────────────────────
# Tempat berkas milik proyek di dalam image (regex atas path tanpa "/" depan).
# Pemeriksaan ekstensi kunci hanya di sini: di luar itu ada sertifikat CA
# sistem yang memang berekstensi .pem.
JALUR_PROYEK='^(app|data)/'
# Folder yang isinya dipindai (grep) dari sistem berkas akhir.
ISI_PINDAI=(app data)
# Yang tidak boleh ada di dalam berkas proyek. Hanya header kunci privat: NAMA
# env rahasia (JWT_SECRET, DB_DSN, …) memang ada di binary Go — kode membacanya
# lewat os.Getenv — jadi tidak bisa dijadikan pola.
RAHASIA='-----BEGIN [A-Z ]*PRIVATE KEY-----'
# Berkas unggahan pengguna (gambar proyek, CV) tidak boleh ikut ke image.
TERLARANG_PROYEK='^data/uploads/.+'
# ───────────────────────── AKHIR KONFIGURASI ─────────────────────────

gagal=0
ok()    { printf '✓ %s\n' "$*"; }
salah() { printf '✗ %s\n' "$*"; gagal=1; }

docker image inspect "$IMG" >/dev/null 2>&1 || { echo "image tidak ditemukan: $IMG" >&2; exit 2; }
# Berkas hasil bongkar mewarisi izin dari image (sering hanya-baca, termasuk
# foldernya): izin tulis dikembalikan dulu supaya folder sementara bisa dihapus.
SP="$(mktemp -d)"; trap 'chmod -R u+w "$SP" 2>/dev/null || true; rm -rf -- "$SP"' EXIT

# ---- 1. nama berkas di semua lapisan
docker save "$IMG" -o "$SP/image.tar"
mkdir "$SP/bongkar"; tar -xf "$SP/image.tar" -C "$SP/bongkar"
: >"$SP/nama.txt"; lapis=0
while IFS= read -r f; do
  # Lapisan adalah arsip tar; manifest dan konfigurasi (JSON) bukan, dan dilewati.
  if tar -tf "$f" >"$SP/satu.txt" 2>/dev/null && [[ -s "$SP/satu.txt" ]]; then
    sed 's#^\./##; s#^/##' "$SP/satu.txt" >>"$SP/nama.txt"; lapis=$((lapis + 1))
  fi
done < <(find "$SP/bongkar" -type f ! -name '*.json' ! -name 'oci-layout' ! -name 'repositories')
(( lapis > 0 )) || { echo "tidak ada lapisan yang terbaca dari $IMG — format docker save berubah?" >&2; exit 2; }

temu="$(grep -E '(^|/)\.env(\.[^/]*)?$' "$SP/nama.txt" | grep -vE '\.(example|sample|template|defaults?)$' | sort -u || true)"
[[ -z "$temu" ]] && ok "tidak ada berkas .env di $lapis lapisan" || { echo "$temu"; salah "berkas .env ada di dalam image"; }

temu="$(grep -E '(^|/)\.git/' "$SP/nama.txt" | sed 's#\(\.git\)/.*#\1/#' | sort -u || true)"
[[ -z "$temu" ]] && ok "tidak ada folder .git" || { echo "$temu"; salah "folder .git (riwayat dan konfigurasi repo) ada di dalam image"; }

temu="$(grep -E "$JALUR_PROYEK" "$SP/nama.txt" | grep -Ei '(\.(pem|key|p12|pfx|jks)|(^|/)id_(rsa|dsa|ecdsa|ed25519))$' | sort -u || true)"
[[ -z "$temu" ]] && ok "tidak ada berkas kunci di berkas proyek" || { echo "$temu"; salah "berkas kunci ada di dalam image"; }

if [[ -n "$TERLARANG_PROYEK" ]]; then
  temu="$(grep -E "$TERLARANG_PROYEK" "$SP/nama.txt" | sort -u | sed -n '1,10p' || true)"
  [[ -z "$temu" ]] && ok "tidak ada berkas terlarang ($TERLARANG_PROYEK)" || { echo "$temu"; salah "berkas yang tidak boleh dikirim ada di dalam image"; }
fi

# ---- 2. isi berkas proyek di sistem berkas akhir
id="$(docker create "$IMG")"
mkdir "$SP/fs"
# Tidak semua folder di ISI_PINDAI ada di tiap image: yang tidak ada dilewati.
docker export "$id" | tar -x -C "$SP/fs" "${ISI_PINDAI[@]}" 2>/dev/null || true
docker rm "$id" >/dev/null
if [[ -z "$(find "$SP/fs" -type f 2>/dev/null | sed -n 1p)" ]]; then
  salah "tidak ada berkas proyek yang terbaca dari ${ISI_PINDAI[*]} — ISI_PINDAI salah?"
else
  temu="$(grep -rlaE -- "$RAHASIA" "$SP/fs" 2>/dev/null | sed "s#^$SP/fs/##" | sort -u || true)"
  [[ -z "$temu" ]] && ok "tidak ada rahasia di isi berkas proyek ($(find "$SP/fs" -type f | wc -l | tr -d ' ') berkas)" \
    || { echo "$temu"; salah "rahasia ditemukan di isi berkas proyek"; }
fi

printf '\n'
if (( gagal )); then echo "ADA YANG GAGAL — image TIDAK boleh dipublikasikan"; else echo "SEMUA LULUS: $IMG"; fi
exit $gagal
