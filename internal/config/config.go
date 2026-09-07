package config

import (
	"errors"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
)

// defaultInsecureJWTSecret adalah nilai placeholder yang TIDAK boleh dipakai di produksi.
const defaultInsecureJWTSecret = "super-secret-ganti-sendiri"

type Config struct {
	AppEnv string

	AppPort string
	DBDSN   string

	// AppFrontendURL adalah base URL frontend (tanpa trailing slash). Dipakai
	// untuk menyusun tautan yang dikirim lewat email, mis. tautan reset password.
	AppFrontendURL string

	JWTSecret    string
	JWTExpiresIn int

	CORSAllowedOrigins   string
	CORSAllowedMethods   string
	CORSAllowedHeaders   string
	CORSAllowCredentials bool

	// --- Cookie sesi (`access_token`) ---
	// Tiga knob ini yang membuat deploy lintas-domain (FE dan BE beda domain)
	// mungkin dilakukan tanpa mengubah kode. Lihat docs/DEPLOYMENT.md.
	CookieDomain   string // kosong = host-only (default, cocok untuk dev & same-site)
	CookieSameSite string // "Lax" (default) | "Strict" | "None"
	CookieSecure   bool   // default: true di produksi, false di luar produksi

	// --- SMTP (pengiriman email, mis. reset password) ---
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string

	// PasswordResetTTL adalah masa berlaku token reset password dalam detik.
	PasswordResetTTL int

	// --- Unggahan berkas ---
	// AppPublicURL adalah base URL backend ini dari sisi browser. Dipakai untuk
	// menyusun URL berkas unggahan yang disimpan ke database, sehingga harus
	// alamat yang benar-benar bisa dibuka klien (bukan "localhost" di produksi).
	AppPublicURL string
	// UploadDir adalah folder tempat berkas unggahan disimpan.
	UploadDir string
	// UploadMaxBytes adalah batas ukuran satu berkas unggahan.
	UploadMaxBytes int
}

func Load() *Config {
	appEnv := helpers.GetEnv("APP_ENV", "development")
	isProd := strings.EqualFold(appEnv, "production")

	return &Config{
		AppEnv: appEnv,

		AppPort:        helpers.GetEnv("APP_PORT", "8080"),
		DBDSN:          helpers.GetEnv("DB_DSN", "postgres://postgres:postgres@localhost:5432/ppnd?sslmode=disable"),
		AppFrontendURL: strings.TrimRight(helpers.GetEnv("APP_FRONTEND_URL", "http://localhost:3000"), "/"),

		JWTSecret:    helpers.GetEnv("JWT_SECRET", defaultInsecureJWTSecret),
		JWTExpiresIn: helpers.GetEnvInt("JWT_EXPIRES_IN", 1800),

		CORSAllowedOrigins:   helpers.GetEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		CORSAllowedMethods:   helpers.GetEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		CORSAllowedHeaders:   helpers.GetEnv("CORS_ALLOWED_HEADERS", "Origin, Content-Type, Accept, Authorization"),
		CORSAllowCredentials: helpers.GetEnvBool("CORS_ALLOW_CREDENTIALS", false),

		CookieDomain:   helpers.GetEnv("COOKIE_DOMAIN", ""),
		CookieSameSite: helpers.GetEnv("COOKIE_SAMESITE", "Lax"),
		// Secure default mengikuti environment: dev di http://localhost tidak
		// boleh Secure (cookie akan ditolak browser), produksi wajib Secure.
		CookieSecure: helpers.GetEnvBool("COOKIE_SECURE", isProd),

		SMTPHost:     helpers.GetEnv("SMTP_HOST", ""),
		SMTPPort:     helpers.GetEnvInt("SMTP_PORT", 587),
		SMTPUsername: helpers.GetEnv("SMTP_USERNAME", ""),
		SMTPPassword: helpers.GetEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     helpers.GetEnv("SMTP_FROM", ""),
		SMTPFromName: helpers.GetEnv("SMTP_FROM_NAME", "Portfolio"),

		PasswordResetTTL: helpers.GetEnvInt("PASSWORD_RESET_TTL", 3600),

		AppPublicURL: strings.TrimRight(
			helpers.GetEnv("APP_PUBLIC_URL", "http://localhost:"+helpers.GetEnv("APP_PORT", "8080")),
			"/",
		),
		UploadDir:      helpers.GetEnv("UPLOAD_DIR", "./uploads"),
		UploadMaxBytes: helpers.GetEnvInt("UPLOAD_MAX_BYTES", 5*1024*1024), // 5 MB
	}
}

// IsProduction mengembalikan true jika aplikasi berjalan di environment produksi.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// NormalizedSameSite mengembalikan nilai SameSite yang sudah dinormalkan ke
// kapitalisasi yang dipahami Fiber ("Lax", "Strict", "None"). Nilai tak dikenal
// jatuh ke "Lax" — pilihan paling aman.
func (c *Config) NormalizedSameSite() string {
	switch strings.ToLower(strings.TrimSpace(c.CookieSameSite)) {
	case "none":
		return "None"
	case "strict":
		return "Strict"
	default:
		return "Lax"
	}
}

// IsCrossSiteCookie true bila cookie dikonfigurasi untuk dikirim lintas-site
// (SameSite=None) — mode yang dibutuhkan saat FE dan BE berada di domain
// berbeda (mis. app.example.com ↔ api.example.com bukan satu registrable
// domain, atau dua domain yang sama sekali berlainan).
func (c *Config) IsCrossSiteCookie() bool {
	return c.NormalizedSameSite() == "None"
}

// MailerConfigured true bila kredensial SMTP minimal sudah diisi sehingga
// pengiriman email (mis. reset password) bisa dilakukan.
func (c *Config) MailerConfigured() bool {
	return c.SMTPHost != "" && c.SMTPFrom != ""
}

// Validate memeriksa konfigurasi yang berbahaya secara keamanan.
// Di produksi konfigurasi yang tidak aman akan menghentikan aplikasi.
func (c *Config) Validate() error {
	var problems []string

	// JWT secret tidak boleh kosong, default, atau terlalu pendek.
	switch {
	case c.JWTSecret == "":
		problems = append(problems, "JWT_SECRET kosong")
	case c.JWTSecret == defaultInsecureJWTSecret:
		problems = append(problems, "JWT_SECRET masih memakai nilai default yang tidak aman")
	case len(c.JWTSecret) < 32:
		problems = append(problems, "JWT_SECRET terlalu pendek (minimal 32 karakter)")
	}

	// CORS wildcard + credentials adalah kombinasi yang tidak valid/berbahaya.
	if c.CORSAllowCredentials && strings.Contains(c.CORSAllowedOrigins, "*") {
		problems = append(problems, "CORS_ALLOW_CREDENTIALS=true tidak boleh dikombinasikan dengan origin '*'")
	}

	// SameSite=None WAJIB disertai Secure — spesifikasi cookie mengharuskannya
	// dan browser modern membuang cookie yang melanggar. Tanpa penjagaan ini,
	// deploy lintas-domain gagal senyap: login "berhasil" tapi sesi tak pernah
	// menempel karena cookie-nya dibuang browser.
	if c.IsCrossSiteCookie() && !c.CookieSecure {
		problems = append(problems, "COOKIE_SAMESITE=None wajib disertai COOKIE_SECURE=true (browser membuang cookie SameSite=None tanpa Secure)")
	}

	// Cookie lintas-site hanya berguna bila CORS juga mengizinkan credentials.
	if c.IsCrossSiteCookie() && !c.CORSAllowCredentials {
		problems = append(problems, "COOKIE_SAMESITE=None tanpa CORS_ALLOW_CREDENTIALS=true: browser tidak akan mengirim cookie pada request lintas-origin")
	}

	// Di produksi, cookie sesi tanpa Secure berarti token bisa bocor lewat HTTP.
	if c.IsProduction() && !c.CookieSecure {
		problems = append(problems, "COOKIE_SECURE=false di produksi (cookie sesi akan terkirim lewat HTTP polos)")
	}

	// URL berkas unggahan ikut TERSIMPAN ke database (mis. coverImageUrl), jadi
	// salah isi di produksi berarti gambar rusak permanen di baris-baris lama.
	if c.IsProduction() && strings.Contains(c.AppPublicURL, "localhost") {
		problems = append(problems, "APP_PUBLIC_URL masih menunjuk localhost di produksi (URL ini tersimpan ke database bersama berkas unggahan)")
	}

	if len(problems) == 0 {
		return nil
	}

	return errors.New("konfigurasi tidak aman: " + strings.Join(problems, "; "))
}
