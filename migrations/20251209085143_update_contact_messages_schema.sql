-- 1. Ubah subject menjadi NOT NULL (kalau ada null, isi dulu)
UPDATE contact_messages
SET subject = 'No Subject'
WHERE subject IS NULL;

ALTER TABLE contact_messages
  ALTER COLUMN subject SET NOT NULL;

-- 2. Tambah kolom is_read boolean
ALTER TABLE contact_messages
  ADD COLUMN IF NOT EXISTS is_read boolean NOT NULL DEFAULT false;

-- 3. Tambah kolom updated_at
ALTER TABLE contact_messages
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

-- 4. Hapus kolom status
ALTER TABLE contact_messages
  DROP COLUMN IF EXISTS status;
