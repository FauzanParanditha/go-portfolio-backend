# Konfigurasi Atlas untuk layanan `migrate` di deploy/docker-compose.yml.
# URL database dibaca dari environment, bukan dari argumen perintah: argumen
# proses terbaca semua pengguna lokal lewat `ps`.
env "deploy" {
  url = getenv("DB_DSN")

  migration {
    dir = "file:///migrations"
  }
}
