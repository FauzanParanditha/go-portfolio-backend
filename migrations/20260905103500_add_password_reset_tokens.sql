-- Tabel token reset password. Token MENTAH tidak pernah disimpan: yang masuk
-- kolom `token_hash` adalah SHA-256 dari token acak yang dikirim lewat email.
-- Dengan begitu bocornya isi tabel tidak cukup untuk mengambil alih akun.
CREATE TABLE password_reset_tokens (
  id         uuid        PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text        NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Lookup saat menerbitkan token baru (membatalkan token lama milik user yang sama).
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens (user_id);

-- Mempercepat janitor pembersih token kedaluwarsa
-- (DELETE FROM password_reset_tokens WHERE expires_at < now()).
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens (expires_at);
