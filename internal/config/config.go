package config

import (
	"errors"
	"net"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
)

// DefaultProxyHeader adalah header IP klien yang dipakai bila TRUSTED_PROXIES
// diisi tapi PROXY_HEADER tidak. Dipilih X-Real-IP (bukan X-Forwarded-For)
// karena proxy umumnya MENIMPA X-Real-IP dengan alamat peer-nya, sedangkan
// X-Forwarded-For sering DITAMBAH (append) — entri paling kiri, yang diambil
// Fiber, tetap berasal dari klien dan bisa dipalsukan.
const DefaultProxyHeader = "X-Real-IP"

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

	// --- Reverse proxy (IP klien untuk rate limit & log) ---
	// TrustedProxies adalah daftar IP/CIDR proxy yang header IP-nya dipercaya
	// (env TRUSTED_PROXIES, dipisah koma). Kosong = header proxy DIABAIKAN dan
	// IP klien diambil dari alamat koneksi (perilaku aman bila tanpa proxy).
	TrustedProxies []string
	// ProxyHeader adalah nilai mentah env PROXY_HEADER (boleh kosong). Pakai
	// ResolvedProxyHeader() untuk nilai efektifnya.
	ProxyHeader string
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

		TrustedProxies: splitList(helpers.GetEnv("TRUSTED_PROXIES", "")),
		ProxyHeader:    strings.TrimSpace(helpers.GetEnv("PROXY_HEADER", "")),
	}
}

// splitList memecah daftar dipisah koma, membuang spasi & entri kosong.
func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// BehindTrustedProxy true bila TRUSTED_PROXIES diisi, artinya header IP dari
// proxy tersebut boleh dipakai sebagai IP klien.
func (c *Config) BehindTrustedProxy() bool {
	return len(c.TrustedProxies) > 0
}

// ResolvedProxyHeader mengembalikan header IP klien yang efektif: PROXY_HEADER
// bila diisi, selain itu DefaultProxyHeader.
func (c *Config) ResolvedProxyHeader() string {
	if c.ProxyHeader != "" {
		return c.ProxyHeader
	}
	return DefaultProxyHeader
}

// trustedProxyProblems memeriksa tiap entri TRUSTED_PROXIES. Entri yang tidak
// bisa di-parse akan diabaikan Fiber tanpa suara, jadi harus terlihat di sini.
func (c *Config) trustedProxyProblems() []string {
	var problems []string
	for _, entry := range c.TrustedProxies {
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				problems = append(problems, "TRUSTED_PROXIES berisi CIDR tidak valid: "+entry)
				continue
			}
			// /0 = mempercayai SEMUA alamat: siapa pun bisa memalsukan header
			// IP dan mengakali rate limit (termasuk login).
			if ones, _ := ipNet.Mask.Size(); ones == 0 {
				problems = append(problems, "TRUSTED_PROXIES berisi "+entry+" yang mempercayai semua alamat (header IP klien bisa dipalsukan siapa pun)")
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			problems = append(problems, "TRUSTED_PROXIES berisi IP tidak valid: "+entry)
		}
	}
	return problems
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

	// PROXY_HEADER tanpa TRUSTED_PROXIES TIDAK berpengaruh: header diabaikan dan
	// semua klien di balik proxy berbagi satu jatah rate limit (IP proxy).
	// Operator yang mengisinya jelas mengira header itu dipakai — lebih baik
	// gagal terang-terangan daripada login limiter diam-diam menjadi global.
	if c.ProxyHeader != "" && !c.BehindTrustedProxy() {
		problems = append(problems, "PROXY_HEADER diisi tanpa TRUSTED_PROXIES: header diabaikan dan semua klien di balik proxy berbagi satu jatah rate limit")
	}
	problems = append(problems, c.trustedProxyProblems()...)

	if len(problems) == 0 {
		return nil
	}

	return errors.New("konfigurasi tidak aman: " + strings.Join(problems, "; "))
}
