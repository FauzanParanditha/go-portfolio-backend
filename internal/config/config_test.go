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

func TestLoadTrustedProxiesDanProxyHeader(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 10.0.0.1, ,172.16.0.0/12 ,")
	t.Setenv("PROXY_HEADER", "")

	c := Load()
	if got, want := strings.Join(c.TrustedProxies, "|"), "10.0.0.1|172.16.0.0/12"; got != want {
		t.Errorf("TrustedProxies = %q, mau %q", got, want)
	}
	if !c.BehindTrustedProxy() {
		t.Error("BehindTrustedProxy seharusnya true")
	}
	// PROXY_HEADER kosong → default X-Real-IP, bukan X-Forwarded-For.
	if got := c.ResolvedProxyHeader(); got != "X-Real-IP" {
		t.Errorf("ResolvedProxyHeader = %q, mau X-Real-IP", got)
	}

	t.Setenv("PROXY_HEADER", "CF-Connecting-IP")
	if got := Load().ResolvedProxyHeader(); got != "CF-Connecting-IP" {
		t.Errorf("ResolvedProxyHeader = %q, mau CF-Connecting-IP", got)
	}
}

func TestLoadTanpaTrustedProxies(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	c := Load()
	if c.BehindTrustedProxy() || len(c.TrustedProxies) != 0 {
		t.Errorf("tanpa env, proxy tidak boleh dipercaya: %v", c.TrustedProxies)
	}
}

func TestValidateProxyConfig(t *testing.T) {
	cases := []struct {
		name    string
		proxies []string
		header  string
		wantErr string // kosong = harus lolos
	}{
		{name: "tanpa proxy", wantErr: ""},
		{name: "proxy IP + default header", proxies: []string{"10.0.0.1"}, wantErr: ""},
		{name: "proxy CIDR + header eksplisit", proxies: []string{"172.16.0.0/12", "::1"}, header: "X-Real-IP", wantErr: ""},
		{name: "header tanpa proxy", header: "X-Real-IP", wantErr: "PROXY_HEADER diisi tanpa TRUSTED_PROXIES"},
		{name: "CIDR rusak", proxies: []string{"10.0.0.0/99"}, wantErr: "CIDR tidak valid"},
		{name: "IP rusak", proxies: []string{"nginx"}, wantErr: "IP tidak valid"},
		{name: "percaya semua IPv4", proxies: []string{"0.0.0.0/0"}, wantErr: "mempercayai semua alamat"},
		{name: "percaya semua IPv6", proxies: []string{"::/0"}, wantErr: "mempercayai semua alamat"},
		// IPv4-mapped /96 = seluruh ruang IPv4 (prefix efektif /0).
		{name: "IPv4-mapped /96", proxies: []string{"::ffff:0:0/96"}, wantErr: "mempercayai semua alamat"},
		{name: "IPv4-mapped /100 (efektif /4)", proxies: []string{"::ffff:0:0/100"}, wantErr: "prefix IPv4 efektif /4"},
		{name: "IPv4 /1 bawah", proxies: []string{"0.0.0.0/1"}, wantErr: "terlalu lebar"},
		{name: "IPv4 /1 atas", proxies: []string{"128.0.0.0/1"}, wantErr: "terlalu lebar"},
		{name: "IPv4 /7", proxies: []string{"10.0.0.0/7"}, wantErr: "terlalu lebar"},
		{name: "IPv6 /1", proxies: []string{"::/1"}, wantErr: "terlalu lebar"},
		{name: "IPv6 /31", proxies: []string{"2001:db8::/31"}, wantErr: "prefix IPv6 /31"},
		{name: "rentang lebar di antara entri sah", proxies: []string{"10.0.0.1", "0.0.0.0/1"}, wantErr: "0.0.0.0/1"},
		// Batas yang masih diterima.
		{name: "IPv4 /8 tepat di batas", proxies: []string{"10.0.0.0/8"}, wantErr: ""},
		{name: "IPv4-mapped /104 (efektif /8)", proxies: []string{"::ffff:10.0.0.0/104"}, wantErr: ""},
		{name: "IPv6 /32 tepat di batas", proxies: []string{"2001:db8::/32"}, wantErr: ""},
		{name: "IPv6 ULA /8", proxies: []string{"fd00::/8"}, wantErr: "terlalu lebar"},
		// Host bit hanya peringatan, bukan galat.
		{name: "CIDR dengan host bit", proxies: []string{"10.0.0.5/24"}, wantErr: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseCfg()
			c.TrustedProxies = tc.proxies
			c.ProxyHeader = tc.header

			err := c.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("seharusnya lolos, dapat: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("galat mau memuat %q, dapat: %v", tc.wantErr, err)
			}
		})
	}
}

// Rentang terlalu lebar harus FATAL di produksi (pola problems di Validate).
func TestValidateProxyTerlaluLebarFatalDiProduksi(t *testing.T) {
	c := baseCfg()
	c.AppEnv = "production"
	c.CookieSecure = true
	c.TrustedProxies = []string{"::ffff:0:0/96"}

	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Fatalf("produksi dengan rentang proxy terlalu lebar harus ditolak, dapat: %v", err)
	}
}

// Fiber mencocokkan IP tunggal sebagai string PERSIS dengan
// RemoteIP().String(), jadi entri harus dinormalkan ke bentuk kanonik.
func TestFiberTrustedProxiesNormalisasi(t *testing.T) {
	c := &Config{TrustedProxies: []string{
		"::ffff:10.0.0.5", // IPv4-mapped → dicetak Go sebagai 10.0.0.5
		"FD00::1",         // huruf besar → huruf kecil
		"2001:DB8:0:0:0:0:0:1",
		"10.0.0.1",
		"172.20.0.0/16", // CIDR diteruskan apa adanya (Fiber mem-parse sendiri)
		"bukan-ip",      // tidak valid: dibuang (dilaporkan Validate)
		"10.0.0.0/99",   // CIDR tidak valid: dibuang
	}}

	got := strings.Join(c.FiberTrustedProxies(), "|")
	want := "10.0.0.5|fd00::1|2001:db8::1|10.0.0.1|172.20.0.0/16"
	if got != want {
		t.Errorf("FiberTrustedProxies = %q, mau %q", got, want)
	}
}

func TestWarningsCIDRDenganHostBit(t *testing.T) {
	cases := []struct {
		entry    string
		wantWarn bool
	}{
		{"10.0.0.5/24", true},
		{"10.0.0.0/24", false},
		{"fd00::1/64", true},
		{"fd00::/64", false},
		{"::ffff:10.0.0.5/120", true},
		{"10.0.0.5", false}, // IP tunggal tidak punya host bit
	}

	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			c := baseCfg()
			c.TrustedProxies = []string{tc.entry}
			w := c.Warnings()
			if tc.wantWarn {
				if len(w) != 1 || !strings.Contains(w[0], "host bit") {
					t.Fatalf("mau satu peringatan host bit, dapat: %v", w)
				}
				return
			}
			if len(w) != 0 {
				t.Fatalf("tidak boleh ada peringatan, dapat: %v", w)
			}
		})
	}

	// Peringatan tidak boleh berubah menjadi galat Validate.
	c := baseCfg()
	c.AppEnv = "production"
	c.CookieSecure = true
	c.TrustedProxies = []string{"10.0.0.5/24"}
	if err := c.Validate(); err != nil {
		t.Fatalf("host bit hanya peringatan, Validate seharusnya lolos: %v", err)
	}
}

// N3(a): entri bermasalah tidak pernah sampai ke Fiber, di environment mana
// pun. Validate di luar produksi hanya memperingatkan, jadi pembuangan inilah
// yang menjadi pagar sebenarnya.
func TestFiberTrustedProxiesMembuangEntriBermasalahDiSemuaEnv(t *testing.T) {
	bermasalah := []string{
		"0.0.0.0/0",
		"::/0",
		"::ffff:0:0/96",  // efektif /0 IPv4
		"::ffff:0:0/100", // efektif /4
		"0.0.0.0/1",
		"10.0.0.0/7",
		"2001:db8::/31",
		"fd00::/8",
		"bukan-ip",
		"10.0.0.0/99",
	}
	sah := []string{"10.0.0.1", "10.0.0.0/8", "2001:db8::/32", "8.0.0.0/8"}

	for _, env := range []string{"development", "staging", "test", "production"} {
		t.Run(env, func(t *testing.T) {
			c := baseCfg()
			c.AppEnv = env
			c.TrustedProxies = append(append([]string{}, bermasalah...), sah...)

			got := strings.Join(c.FiberTrustedProxies(), "|")
			want := strings.Join(sah, "|")
			if got != want {
				t.Fatalf("FiberTrustedProxies = %q, mau %q", got, want)
			}
		})
	}

	// Problem tetap dilaporkan Validate (beserta keterangan bahwa entrinya
	// diabaikan) agar operator tahu proxy-nya tidak dipercaya.
	c := baseCfg()
	c.AppEnv = "staging"
	c.TrustedProxies = []string{"0.0.0.0/0"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "entri ini diabaikan") {
		t.Fatalf("Validate harus melaporkan entri yang diabaikan, dapat: %v", err)
	}
}

// N3(b): rentang lebar yang memuat alamat publik → peringatan (bukan fatal).
func TestWarningsRentangPublikLebar(t *testing.T) {
	cases := []struct {
		entry    string
		wantWarn bool
	}{
		// Publik & lebih lebar dari /24 (IPv4) atau /64 (IPv6).
		{"8.0.0.0/8", true},
		{"203.0.112.0/23", true},
		{"2600::/32", true},
		{"2001::/32", true},
		{"2001:db8:1::/48", true},
		{"2001:db8:1:2::/63", true},
		{"::ffff:8.0.0.0/104", true}, // IPv4-mapped, efektif /8
		// Rentang yang melampaui blok privat ke alamat publik: 172.0.0.0/11
		// memuat 172.16.0.0/12 sekaligus 172.0.x.x (publik); 192.168.0.0/15
		// memuat 192.169.x.x (publik).
		{"172.0.0.0/11", true},
		{"192.168.0.0/15", true},

		// Batas: /24 dan /64 tidak kena.
		{"203.0.113.0/24", false},
		{"2001:db8:1:2::/64", false},
		{"203.0.113.10", false}, // IP publik tunggal
		{"2600::1", false},

		// Privat / loopback / link-local: tidak kena peringatan publik.
		{"10.0.0.0/8", false},
		{"172.16.0.0/12", false},
		{"192.168.0.0/16", false},
		{"127.0.0.0/8", false},
		{"169.254.0.0/16", false},
		{"fd00::/32", false},
		{"fe80::/10", false},
		{"::ffff:10.0.0.0/104", false},

		// Bermasalah (dibuang): tidak menambah peringatan, sudah jadi problem.
		{"0.0.0.0/0", false},
		{"2600::/31", false},
	}

	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			c := baseCfg()
			c.TrustedProxies = []string{tc.entry}
			w := c.Warnings()
			if tc.wantWarn {
				if len(w) != 1 || !strings.Contains(w[0], "alamat publik") {
					t.Fatalf("mau satu peringatan alamat publik, dapat: %v", w)
				}
				return
			}
			for _, msg := range w {
				if strings.Contains(msg, "alamat publik") {
					t.Fatalf("tidak boleh ada peringatan alamat publik, dapat: %v", w)
				}
			}
		})
	}

	// Peringatan publik tidak boleh menjadi galat Validate, juga di produksi.
	c := baseCfg()
	c.AppEnv = "production"
	c.CookieSecure = true
	c.AppPublicURL = "https://api.example.com"
	c.TrustedProxies = []string{"8.0.0.0/8", "2600::/32"}
	if err := c.Validate(); err != nil {
		t.Fatalf("rentang publik lebar hanya peringatan, Validate seharusnya lolos: %v", err)
	}
	if got := len(c.Warnings()); got != 2 {
		t.Fatalf("mau 2 peringatan, dapat %d: %v", got, c.Warnings())
	}
}

// Satu entri bisa memicu dua peringatan sekaligus (host bit + rentang publik).
func TestWarningsHostBitDanPublikSekaligus(t *testing.T) {
	c := baseCfg()
	c.TrustedProxies = []string{"8.8.8.8/8"}
	w := strings.Join(c.Warnings(), "\n")
	if !strings.Contains(w, "host bit") || !strings.Contains(w, "alamat publik") {
		t.Fatalf("mau peringatan host bit DAN alamat publik, dapat: %q", w)
	}
}
