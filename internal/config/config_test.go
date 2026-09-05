package config

import (
	"strings"
	"testing"
)

// baseCfg mengembalikan config yang lolos Validate, sebagai titik awal agar tiap
// test hanya mengubah satu hal dan kegagalannya jelas penyebabnya.
func baseCfg() *Config {
	return &Config{
		AppEnv:               "development",
		JWTSecret:            strings.Repeat("a", 32),
		CORSAllowedOrigins:   "http://localhost:3000",
		CORSAllowCredentials: true,
		CookieSameSite:       "Lax",
		CookieSecure:         false,
	}
}

func TestValidateConfigAman(t *testing.T) {
	if err := baseCfg().Validate(); err != nil {
		t.Fatalf("config dasar seharusnya lolos, dapat: %v", err)
	}
}

func TestNormalizedSameSite(t *testing.T) {
	cases := map[string]string{
		"none":    "None",
		"NONE":    "None",
		" None ":  "None",
		"strict":  "Strict",
		"lax":     "Lax",
		"":        "Lax",
		"ngawur":  "Lax", // nilai tak dikenal jatuh ke default paling aman
		"Lax   ":  "Lax",
		"sTrIcT":  "Strict",
		"none   ": "None",
	}

	for in, want := range cases {
		c := &Config{CookieSameSite: in}
		if got := c.NormalizedSameSite(); got != want {
			t.Errorf("NormalizedSameSite(%q) = %q, mau %q", in, got, want)
		}
	}
}

// SameSite=None tanpa Secure dibuang browser — konfigurasi ini harus ditolak
// supaya kegagalannya muncul saat startup, bukan sebagai "login yang tidak
// menempel" di produksi.
func TestValidateSameSiteNoneWajibSecure(t *testing.T) {
	c := baseCfg()
	c.CookieSameSite = "None"
	c.CookieSecure = false

	err := c.Validate()
	if err == nil {
		t.Fatal("SameSite=None tanpa Secure seharusnya ditolak")
	}
	if !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("pesan error tidak menyebut COOKIE_SECURE: %v", err)
	}
}

// Cookie lintas-site percuma bila CORS tidak mengizinkan credentials.
func TestValidateSameSiteNoneButuhCORSCredentials(t *testing.T) {
	c := baseCfg()
	c.CookieSameSite = "None"
	c.CookieSecure = true
	c.CORSAllowCredentials = false

	err := c.Validate()
	if err == nil {
		t.Fatal("SameSite=None tanpa CORS credentials seharusnya ditolak")
	}
	if !strings.Contains(err.Error(), "CORS_ALLOW_CREDENTIALS") {
		t.Fatalf("pesan error tidak menyebut CORS_ALLOW_CREDENTIALS: %v", err)
	}
}

// Konfigurasi lintas-domain yang benar harus lolos apa adanya.
func TestValidateLintasDomainValid(t *testing.T) {
	c := baseCfg()
	c.AppEnv = "production"
	c.CookieSameSite = "None"
	c.CookieSecure = true
	c.CORSAllowedOrigins = "https://app.example.com"
	c.CORSAllowCredentials = true

	if err := c.Validate(); err != nil {
		t.Fatalf("konfigurasi lintas-domain yang benar seharusnya lolos, dapat: %v", err)
	}
	if !c.IsCrossSiteCookie() {
		t.Error("IsCrossSiteCookie() = false, mau true untuk SameSite=None")
	}
}

func TestValidateProduksiWajibCookieSecure(t *testing.T) {
	c := baseCfg()
	c.AppEnv = "production"
	c.CookieSecure = false

	err := c.Validate()
	if err == nil {
		t.Fatal("produksi dengan COOKIE_SECURE=false seharusnya ditolak")
	}
	if !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("pesan error tidak menyebut COOKIE_SECURE: %v", err)
	}
}

func TestValidateJWTSecret(t *testing.T) {
	cases := []struct {
		nama   string
		secret string
	}{
		{"kosong", ""},
		{"default tidak aman", defaultInsecureJWTSecret},
		{"terlalu pendek", "pendek"},
	}

	for _, tc := range cases {
		t.Run(tc.nama, func(t *testing.T) {
			c := baseCfg()
			c.JWTSecret = tc.secret
			if err := c.Validate(); err == nil {
				t.Fatalf("JWT_SECRET %q seharusnya ditolak", tc.secret)
			}
		})
	}
}

func TestValidateCORSWildcardDenganCredentials(t *testing.T) {
	c := baseCfg()
	c.CORSAllowedOrigins = "*"
	c.CORSAllowCredentials = true

	if err := c.Validate(); err == nil {
		t.Fatal("origin '*' + credentials seharusnya ditolak")
	}
}

func TestMailerConfigured(t *testing.T) {
	cases := []struct {
		nama string
		host string
		from string
		want bool
	}{
		{"lengkap", "smtp.example.com", "no-reply@example.com", true},
		{"tanpa host", "", "no-reply@example.com", false},
		{"tanpa from", "smtp.example.com", "", false},
		{"kosong", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.nama, func(t *testing.T) {
			c := &Config{SMTPHost: tc.host, SMTPFrom: tc.from}
			if got := c.MailerConfigured(); got != tc.want {
				t.Fatalf("MailerConfigured() = %v, mau %v", got, tc.want)
			}
		})
	}
}
