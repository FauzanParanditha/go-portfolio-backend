-- audit-project-urls.sql
--
-- Audit READ-ONLY: mencari URL proyek yang tersimpan di database tetapi
-- melanggar aturan validasi baru (validation.IsHTTPURL / validation.IsImageURL).
-- Script ini HANYA berisi SELECT dan dijalankan di dalam transaksi READ ONLY,
-- jadi tidak mengubah data apa pun.
--
-- Kenapa perlu: data lama tetap terbaca, tetapi saat proyeknya diedit di panel
-- admin (PUT /api/v1/admin/projects/:id) seluruh field dikirim ulang dan
-- divalidasi, sehingga proyek dengan URL tidak valid akan kena 422 sampai
-- URL-nya diperbaiki. Jalankan SEBELUM deploy versi ini (lihat
-- docs/DEPLOYMENT.md), lalu perbaiki baris yang muncul secara manual.
--
-- Cara pakai:
--   psql "$DB_DSN" -f scripts/audit-project-urls.sql
--
-- Aturan yang diperiksa (sumber: internal/validation/validator.go):
--   projects.demo_url, projects.repo_url  → hanya http/https dengan host
--                                            (NULL = tidak ada, dilewati)
--   projects.cover_image_url              → http/https dengan host ATAU path
--                                            diawali "/" (bukan "//", "/\");
--                                            kosong juga dilaporkan karena
--                                            field ini WAJIB di API
--   project_screenshots.image_url         → sama dengan cover; "" dilewati
--                                            (API melewati item kosong)
--   Semua: spasi di tepi ditolak (tidak dipangkas).
--
-- Catatan: ini pendekatan regex terhadap url.Parse milik Go. Hasilnya cukup
-- untuk menemukan pelanggaran nyata (skema berbahaya, //host, spasi, host
-- kosong), tetapi validator Go tetap penentu akhir.

BEGIN TRANSACTION READ ONLY;

-- 1) Daftar baris yang melanggar, satu baris per nilai bermasalah.
WITH nilai AS (
    SELECT 'projects'::text AS tabel, p.id AS baris_id, p.slug, p.title,
           'demo_url'::text AS kolom, p.demo_url AS nilai, 'http'::text AS jenis
    FROM projects p
    WHERE p.demo_url IS NOT NULL

    UNION ALL
    SELECT 'projects', p.id, p.slug, p.title, 'repo_url', p.repo_url, 'http'
    FROM projects p
    WHERE p.repo_url IS NOT NULL

    UNION ALL
    SELECT 'projects', p.id, p.slug, p.title, 'cover_image_url',
           COALESCE(p.cover_image_url, ''), 'gambar_wajib'
    FROM projects p

    UNION ALL
    SELECT 'project_screenshots', s.id, p.slug, p.title, 'image_url', s.image_url, 'gambar'
    FROM project_screenshots s
    JOIN projects p ON p.id = s.project_id
    WHERE s.image_url <> ''
),
dinilai AS (
    SELECT n.*,
        CASE
            WHEN n.nilai = '' AND n.jenis = 'http'
                THEN 'string kosong (API menolak ""; simpan NULL)'
            WHEN n.nilai = '' AND n.jenis = 'gambar_wajib'
                THEN 'kosong (coverImageUrl wajib diisi saat edit)'
            WHEN n.nilai ~ '[[:cntrl:]]'
                THEN 'mengandung karakter kontrol'
            WHEN n.nilai ~ '^[[:space:]]|[[:space:]]$'
                THEN 'spasi di tepi (tidak dipangkas otomatis)'
            WHEN n.nilai ~ '%([^0-9A-Fa-f]|[0-9A-Fa-f]([^0-9A-Fa-f]|$)|$)'
                THEN 'escape % tidak valid'
            -- Path relatif (hanya untuk kolom gambar).
            WHEN n.jenis <> 'http' AND left(n.nilai, 2) = '//'
                THEN 'URL protocol-relative //host (menunjuk host lain)'
            WHEN n.jenis <> 'http' AND left(n.nilai, 2) = '/' || chr(92)
                THEN 'URL /\host (menunjuk host lain)'
            WHEN n.jenis <> 'http' AND left(n.nilai, 1) = '/'
                THEN NULL -- path "/..." yang sah
            -- Selain itu harus URL http/https absolut.
            WHEN n.nilai !~* '^https?:'
                THEN CASE
                    WHEN n.nilai ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
                        THEN 'skema bukan http/https (mis. javascript:, data:)'
                    WHEN n.jenis = 'http'
                        THEN 'bukan URL http(s) absolut'
                    ELSE 'bukan URL http(s) maupun path diawali "/"'
                END
            WHEN n.nilai !~* '^https?://([^/?#@]*@)?(\[[0-9A-Fa-f:.]+(%[^]]*)?\]|[^][/?#:@[:space:]]+)(:[0-9]*)?([/?#]|$)'
                THEN 'tanpa host atau host/port tidak valid'
            ELSE NULL
        END AS alasan
    FROM nilai n
)
SELECT tabel, baris_id, slug, title, kolom, nilai, alasan
FROM dinilai
WHERE alasan IS NOT NULL
ORDER BY slug, tabel, kolom, baris_id;

-- 2) Ringkasan: jumlah pelanggaran per kolom dan alasan, plus jumlah proyek
--    yang akan kena 422 saat diedit.
WITH nilai AS (
    SELECT p.id AS project_id, 'demo_url'::text AS kolom, p.demo_url AS nilai, 'http'::text AS jenis
    FROM projects p WHERE p.demo_url IS NOT NULL
    UNION ALL
    SELECT p.id, 'repo_url', p.repo_url, 'http' FROM projects p WHERE p.repo_url IS NOT NULL
    UNION ALL
    SELECT p.id, 'cover_image_url', COALESCE(p.cover_image_url, ''), 'gambar_wajib' FROM projects p
    UNION ALL
    SELECT s.project_id, 'screenshots.image_url', s.image_url, 'gambar'
    FROM project_screenshots s WHERE s.image_url <> ''
),
dinilai AS (
    SELECT n.project_id, n.kolom,
        CASE
            WHEN n.nilai = '' AND n.jenis = 'http' THEN 'string kosong'
            WHEN n.nilai = '' AND n.jenis = 'gambar_wajib' THEN 'kosong (wajib)'
            WHEN n.nilai ~ '[[:cntrl:]]' THEN 'karakter kontrol'
            WHEN n.nilai ~ '^[[:space:]]|[[:space:]]$' THEN 'spasi di tepi'
            WHEN n.nilai ~ '%([^0-9A-Fa-f]|[0-9A-Fa-f]([^0-9A-Fa-f]|$)|$)' THEN 'escape % tidak valid'
            WHEN n.jenis <> 'http' AND left(n.nilai, 2) = '//' THEN 'protocol-relative'
            WHEN n.jenis <> 'http' AND left(n.nilai, 2) = '/' || chr(92) THEN 'protocol-relative'
            WHEN n.jenis <> 'http' AND left(n.nilai, 1) = '/' THEN NULL
            WHEN n.nilai !~* '^https?:' THEN
                CASE WHEN n.nilai ~ '^[A-Za-z][A-Za-z0-9+.-]*:' THEN 'skema bukan http/https'
                     ELSE 'bukan URL http(s)/path' END
            WHEN n.nilai !~* '^https?://([^/?#@]*@)?(\[[0-9A-Fa-f:.]+(%[^]]*)?\]|[^][/?#:@[:space:]]+)(:[0-9]*)?([/?#]|$)'
                THEN 'tanpa host/host tidak valid'
            ELSE NULL
        END AS alasan
    FROM nilai n
)
SELECT kolom, alasan, count(*) AS jumlah_nilai, count(DISTINCT project_id) AS jumlah_proyek
FROM dinilai
WHERE alasan IS NOT NULL
GROUP BY ROLLUP (kolom, alasan)
ORDER BY kolom NULLS LAST, alasan NULLS LAST;

ROLLBACK;
