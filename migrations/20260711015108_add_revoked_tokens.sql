-- Tabel denylist token JWT yang dicabut (mis. saat logout) agar revocation
-- tetap persisten walau proses aplikasi restart. Kunci utama adalah `jti`
-- (JWT ID) sehingga upsert saat revoke aman dipanggil berulang.
CREATE TABLE revoked_tokens (
  jti        text        PRIMARY KEY,
  exp        timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Index pada `exp` untuk mempercepat janitor pembersih entri kedaluwarsa
-- (DELETE FROM revoked_tokens WHERE exp < now()).
CREATE INDEX idx_revoked_tokens_exp ON revoked_tokens (exp);
