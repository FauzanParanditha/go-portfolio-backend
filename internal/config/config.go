package config

import (
	"errors"
	"fmt"
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

// Batas terlebar rentang proxy yang masih masuk akal. Proxy tepercaya adalah
// segelintir mesin milik sendiri; rentang selebar ini (apalagi lebih) hampir
// pasti salah ketik dan membuat header IP bisa dipalsukan dari luar.
const (
	minTrustedPrefixIPv4 = 8  // /8 = 16 juta alamat
	minTrustedPrefixIPv6 = 32 // /32 = alokasi tipikal satu ISP
)

// Batas terlebar rentang proxy yang MEMUAT alamat publik sebelum memicu
// peringatan. Rentang publik selebar ini berisi mesin milik orang lain (mis.
// seluruh blok cloud/ISP), dan siapa pun di dalamnya bisa memalsukan header IP.
// Hanya peringatan karena masih sah (mis. blok IP milik sendiri).
const (
	publicWarnPrefixIPv4 = 24
	publicWarnPrefixIPv6 = 64
)

// trustedProxyEntry adalah hasil parse satu entri TRUSTED_PROXIES.
type trustedProxyEntry struct {
	raw string
	// canonical adalah bentuk yang diberikan ke Fiber: IP tunggal dalam bentuk
	// kanonik net.IP.String(), atau CIDR apa adanya (Fiber mem-parse sendiri).
	// SELALU kosong bila problem terisi: entri bermasalah tidak pernah sampai
	// ke Fiber, di environment mana pun.
	canonical string
	// problem diisi bila entri harus ditolak (fatal di produksi, dibuang dari
	// FiberTrustedProxies di semua environment).
	problem string
	// warnings diisi bila entri sah tapi kemungkinan besar salah maksud.
	warnings []string
}

// parseTrustedProxy memeriksa satu entri TRUSTED_PROXIES.
//
// IP tunggal dinormalkan karena Fiber mencocokkannya sebagai string PERSIS
// dengan RemoteIP().String() (app.handleTrustedProxy, Ctx.IsProxyTrusted).
// Tanpa normalisasi, "::ffff:10.0.0.5" (tercetak "10.0.0.5") atau "FD00::1"
// (tercetak "fd00::1") tidak pernah cocok dan proxy diam-diam tidak dipercaya.
func parseTrustedProxy(entry string) trustedProxyEntry {
	e := trustedProxyEntry{raw: entry}

	if !strings.Contains(entry, "/") {
		ip := net.ParseIP(entry)
		if ip == nil {
			e.problem = "TRUSTED_PROXIES berisi IP tidak valid: " + entry
			return e
		}
		e.canonical = ip.String()
		return e
	}

	ip, ipNet, err := net.ParseCIDR(entry)
	if err != nil {
		e.problem = "TRUSTED_PROXIES berisi CIDR tidak valid: " + entry
		return e
	}

	// Prefix efektif. CIDR IPv4-mapped (mis. "::ffff:0:0/96") ditulis sebagai
	// IPv6 tetapi net.IPNet.Contains mencocokkannya dengan alamat IPv4, jadi
	// lebarnya harus dihitung di ruang IPv4: ones-96. Tanpa ini "::ffff:0:0/96"
	// terlihat seperti /96 yang sempit padahal mempercayai SEMUA IPv4.
	ones, bits := ipNet.Mask.Size()
	isIPv4 := bits == 32
	if bits == 128 && ipNet.IP.To4() != nil {
		isIPv4 = true
		ones -= 96
	}

	switch {
	case ones == 0:
		// /0 = mempercayai SEMUA alamat: siapa pun bisa memalsukan header IP
		// dan mengakali rate limit (termasuk login).
		e.problem = "TRUSTED_PROXIES berisi " + entry + " yang mempercayai semua alamat (header IP klien bisa dipalsukan siapa pun)"
	case isIPv4 && ones < minTrustedPrefixIPv4:
		e.problem = fmt.Sprintf("TRUSTED_PROXIES berisi %s yang terlalu lebar (prefix IPv4 efektif /%d, minimal /%d)", entry, ones, minTrustedPrefixIPv4)
	case !isIPv4 && ones < minTrustedPrefixIPv6:
		e.problem = fmt.Sprintf("TRUSTED_PROXIES berisi %s yang terlalu lebar (prefix IPv6 /%d, minimal /%d)", entry, ones, minTrustedPrefixIPv6)
	}
	if e.problem != "" {
		// Fail-safe: canonical dibiarkan kosong agar entri ini TIDAK dipasang
		// ke Fiber. Dulu canonical diisi sebelum cek lebar, sehingga di luar
		// produksi (mis. staging) "0.0.0.0/0" tetap aktif dan hanya memicu
		// peringatan.
		return e
	}
	e.canonical = entry

	// Host bit terisi (mis. "10.0.0.5/24"): Fiber diam-diam memakai jaringan
	// 10.0.0.0/24, padahal operator mungkin hanya bermaksud satu IP proxy.
	if !ip.Equal(ipNet.IP) {
		e.warnings = append(e.warnings, "TRUSTED_PROXIES berisi "+entry+" dengan host bit terisi: yang dipercaya adalah seluruh jaringan "+
			ipNet.String()+" (tulis "+ipNet.String()+" bila memang itu maksudnya, atau IP tunggal tanpa /prefix)")
	}

	// Rentang lebar yang memuat alamat publik (mis. "8.0.0.0/8", "2600::/32"):
	// lolos batas minimal, tapi berisi mesin milik pihak lain yang semuanya
	// ikut bisa memalsukan header IP.
	limit, family := publicWarnPrefixIPv6, "IPv6"
	if isIPv4 {
		limit, family = publicWarnPrefixIPv4, "IPv4"
	}
	if ones < limit && cidrHasPublicAddress(ipNet) {
		e.warnings = append(e.warnings, fmt.Sprintf(
			"TRUSTED_PROXIES berisi %s yang memuat alamat publik dengan prefix %s /%d (lebih lebar dari /%d): siapa pun di rentang itu bisa memalsukan header IP klien; tulis IP spesifik proxy",
			entry, family, ones, limit))
	}
	return e
}

// cidrHasPublicAddress true bila rentang memuat setidaknya satu alamat yang
// bukan privat (RFC 1918 / ULA fc00::/7), loopback, maupun link-local.
//
// Cukup memeriksa alamat pertama dan terakhir: blok non-publik tersebut adalah
// CIDR yang saling lepas, dan dua CIDR selalu saling memuat atau tidak beririsan.
// Jadi bila ujung awal dan akhir sama-sama non-publik, keduanya berada di blok
// yang sama dan seluruh rentang ada di dalamnya (rentang yang lolos batas
// minimal /8 atau /32 terlalu sempit untuk merentang dua blok berbeda).
func cidrHasPublicAddress(n *net.IPNet) bool {
	first := n.IP
	last := make(net.IP, len(first))
	for i := range first {
		last[i] = first[i] | ^n.Mask[i]
	}
	return isPublicIP(first) || isPublicIP(last)
}

// isPublicIP true bila ip bukan alamat privat, loopback, atau link-local.
func isPublicIP(ip net.IP) bool {
	return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

// trustedProxyProblems memeriksa tiap entri TRUSTED_PROXIES. Entri yang tidak
// bisa di-parse dan rentang yang terlalu lebar dibuang dari Fiber (lihat
// FiberTrustedProxies), tetapi harus tetap terlihat di sini agar operator tahu
// proxy-nya tidak dipercaya.
func (c *Config) trustedProxyProblems() []string {
	var problems []string
	for _, entry := range c.TrustedProxies {
		if p := parseTrustedProxy(entry).problem; p != "" {
			problems = append(problems, p+" (entri ini diabaikan)")
		}
	}
	return problems
}

// FiberTrustedProxies mengembalikan TRUSTED_PROXIES dalam bentuk yang siap
// diberikan ke fiber.Config.TrustedProxies: IP tunggal dinormalkan ke bentuk
// kanonik (lihat parseTrustedProxy). Entri yang punya problem (tidak bisa
// di-parse ATAU terlalu lebar) DIBUANG di semua environment, bukan hanya fatal
// di produksi — di luar produksi Validate cuma memperingatkan, jadi tanpa
// pembuangan ini "0.0.0.0/0" di staging tetap aktif. Bila semua entri dibuang,
// tidak ada proxy yang dipercaya dan IP klien diambil dari alamat koneksi.
func (c *Config) FiberTrustedProxies() []string {
	out := make([]string, 0, len(c.TrustedProxies))
	for _, entry := range c.TrustedProxies {
		if e := parseTrustedProxy(entry); e.problem == "" && e.canonical != "" {
			out = append(out, e.canonical)
		}
	}
	return out
}

// Warnings mengembalikan konfigurasi yang sah tetapi kemungkinan salah maksud.
// Berbeda dengan Validate, ini TIDAK menghentikan aplikasi di produksi; cukup
// dicatat sebagai peringatan saat start.
func (c *Config) Warnings() []string {
	var warnings []string
	for _, entry := range c.TrustedProxies {
		warnings = append(warnings, parseTrustedProxy(entry).warnings...)
	}
	return warnings
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
