
# -----------------------------------------------------------------------------
# Environment: Development (Local)
# Dipakai default oleh Makefile kamu (ATLAS_ENV=dev)
# -----------------------------------------------------------------------------
env "dev" {
  # Database utama untuk development (lokal)
  # Pakai yang ini dulu:
  url = "postgres://postgres:root@localhost:5432/portfolio?sslmode=disable"

  # Dev database khusus Atlas (wajib ada)
  dev = "postgres://postgres:root@localhost:5432/portfolio_dev?sslmode=disable"

  # Kalau nanti mau pakai ENV variable, bisa ganti jadi:
  # url     = env("DEV_DATABASE_URL")
  # dev = env("DEV_ATLAS_DEV_URL")

  migration {
    dir = "file://migrations"
  }
}

# -----------------------------------------------------------------------------
# Environment: Staging
# Dipakai untuk database staging (misal di server atau Docker Compose lain)
# -----------------------------------------------------------------------------
env "staging" {
  # Contoh koneksi staging. Ganti sesuai environment kamu.
  # Misal staging di server/dokcer pakai host "staging-db" dan password beda.
  url = "postgres://postgres:staging_password@staging-db:5432/portfolio_staging?sslmode=disable"

  # Dev DB staging untuk Atlas (boleh sama host, beda nama DB)
  dev = "postgres://postgres:staging_password@staging-db:5432/portfolio_staging_dev?sslmode=disable"

  # Atau via ENV (disarankan untuk server):
  # url     = env("STAGING_DATABASE_URL")
  # dev = env("STAGING_ATLAS_DEV_URL")

  migration {
    dir = "file://migrations"
  }
}

# -----------------------------------------------------------------------------
# Environment: Production
# DISARANKAN pakai ENV, jangan hardcode password di file ini kalau sudah di server
# -----------------------------------------------------------------------------
env "prod" {
  # Contoh koneksi. Lebih aman pakai ENV seperti di bawah.
  # url = "postgres://postgres:prod_password@prod-db:5432/portfolio_prod?sslmode=disable"

  # Kuat disarankan:
  url     = env("PROD_DATABASE_URL")
  dev = env("PROD_ATLAS_DEV_URL")

  migration {
    dir = "file://migrations"
  }
}

# -----------------------------------------------------------------------------
# Environment: Docker Dev (opsional)
# Kalau kamu punya Postgres di Docker dengan hostname "postgres"
# -----------------------------------------------------------------------------
env "docker-dev" {
  # Misal di docker-compose:
  #   host: postgres
  #   user: postgres
  #   password: root
  #   db: portfolio
  url = "postgres://postgres:root@postgres:5432/portfolio?sslmode=disable"

  dev_url = "postgres://postgres:root@postgres:5432/portfolio_dev?sslmode=disable"

  migration {
    dir = "file://migrations"
  }
}
