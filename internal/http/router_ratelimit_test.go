package http_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	apphttp "github.com/FauzanParanditha/portfolio-backend/internal/http"
	"github.com/gofiber/fiber/v2"
)

// Test di berkas ini memakai router ASLI (NewRouter), bukan rakitan ulang, agar
// yang diuji benar-benar wiring limiter & konfigurasi proxy di produksi.
//
// DB-free: DB sengaja nil. Body request dibuat gagal validasi ({}), sehingga
// handler contact berhenti di 422 sebelum menyentuh repository. Limiter
// dipasang SEBELUM handler, jadi request yang lolos limiter terlihat sebagai
// 422 dan yang ditahan sebagai 429.

const contactLimit = 5

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		AppEnv:             "test",
		JWTSecret:          strings.Repeat("s", 32),
		CORSAllowedOrigins: "http://localhost:3000",
		CookieSameSite:     "Lax",
		UploadDir:          t.TempDir(),
		UploadMaxBytes:     1024 * 1024,
		AppPublicURL:       "http://localhost:8080",
	}
}

func newRouter(t *testing.T, cfg *config.Config) *fiber.App {
	t.Helper()
	return apphttp.NewRouter(apphttp.AppDeps{
		DB:     nil,
		Config: cfg,
		// Mailer & Storage dibiarkan nil: NewRouter membuat default-nya
		// sendiri (mailer log tanpa SMTP, disk di UploadDir sementara).
	})
}

// postContact mengirim body tidak valid ke path contact, dengan header
// X-Real-IP opsional, lalu mengembalikan status dan body.
func postContact(t *testing.T, app *fiber.App, path, realIP string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if realIP != "" {
		req.Header.Set("X-Real-IP", realIP)
	}

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("gagal baca body: %v", err)
	}
	return resp.StatusCode, body
}

// habiskanJatah mengirim tepat `contactLimit` request yang semuanya harus lolos
// limiter (422 dari validasi).
func habiskanJatah(t *testing.T, app *fiber.App, realIP string) {
	t.Helper()
	for i := 1; i <= contactLimit; i++ {
		status, body := postContact(t, app, "/api/v1/contact", realIP)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("request ke-%d (ip=%q): status = %d, mau 422 (lolos limiter); body=%s",
				i, realIP, status, string(body))
		}
	}
}

func TestContactRateLimitMengembalikan429DenganEnvelope(t *testing.T) {
	app := newRouter(t, newTestConfig(t))

	habiskanJatah(t, app, "")

	status, body := postContact(t, app, "/api/v1/contact", "")
	if status != http.StatusTooManyRequests {
		t.Fatalf("request ke-%d: status = %d, mau 429; body=%s", contactLimit+1, status, string(body))
	}

	// Harus lewat error handler pusat: {"error":{message,code,details},"requestId"}.
	var env struct {
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		RequestID *string `json:"requestId"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body bukan JSON (%s): %v", string(body), err)
	}
	if env.Error == nil {
		t.Fatalf("envelope error tidak ada: %s", string(body))
	}
	if env.Error.Code != "TOO_MANY_REQUESTS" {
		t.Errorf("error.code = %q, mau TOO_MANY_REQUESTS", env.Error.Code)
	}
	if env.Error.Message == "" {
		t.Error("error.message kosong")
	}
	if env.RequestID == nil || *env.RequestID == "" {
		t.Errorf("requestId tidak ada di envelope: %s", string(body))
	}
}

// Alias /contact-messages memakai instance limiter yang sama: berganti-ganti
// path tidak boleh menggandakan kuota.
func TestContactRateLimitBerbagiJatahDenganAlias(t *testing.T) {
	app := newRouter(t, newTestConfig(t))

	for i := 1; i <= contactLimit; i++ {
		path := "/api/v1/contact"
		if i%2 == 0 {
			path = "/api/v1/contact-messages"
		}
		if status, body := postContact(t, app, path, ""); status != http.StatusUnprocessableEntity {
			t.Fatalf("request ke-%d ke %s: status = %d, mau 422; body=%s", i, path, status, string(body))
		}
	}

	if status, _ := postContact(t, app, "/api/v1/contact-messages", ""); status != http.StatusTooManyRequests {
		t.Fatalf("alias setelah jatah habis: status = %d, mau 429", status)
	}
}

// Dengan proxy tepercaya, X-Real-IP dipakai sebagai IP klien: dua klien di
// balik proxy yang sama punya jatah terpisah.
//
// app.Test memakai koneksi palsu dengan remote addr 0.0.0.0, jadi alamat itu
// yang didaftarkan sebagai proxy tepercaya.
func TestTrustedProxyMemisahkanJatahPerIPKlien(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"0.0.0.0"}
	app := newRouter(t, cfg)

	habiskanJatah(t, app, "203.0.113.10")

	if status, _ := postContact(t, app, "/api/v1/contact", "203.0.113.10"); status != http.StatusTooManyRequests {
		t.Fatalf("klien A setelah jatah habis: status = %d, mau 429", status)
	}

	// Klien B masih punya jatah penuh.
	if status, body := postContact(t, app, "/api/v1/contact", "203.0.113.20"); status != http.StatusUnprocessableEntity {
		t.Fatalf("klien B: status = %d, mau 422 (jatah terpisah); body=%s", status, string(body))
	}
}

// Header yang bukan IP valid tidak boleh menjadi kunci limiter (kalau boleh,
// penyerang cukup mengganti string acak tiap request). EnableIPValidation
// membuatnya jatuh ke remote addr, sehingga semua nilai sampah berbagi jatah.
func TestTrustedProxyHeaderSampahJatuhKeRemoteAddr(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"0.0.0.0/32"}
	app := newRouter(t, cfg)

	for i := 1; i <= contactLimit; i++ {
		if status, _ := postContact(t, app, "/api/v1/contact", "bukan-ip-"+string(rune('a'+i))); status != http.StatusUnprocessableEntity {
			t.Fatalf("request ke-%d: status = %d, mau 422", i, status)
		}
	}
	if status, _ := postContact(t, app, "/api/v1/contact", "bukan-ip-lain"); status != http.StatusTooManyRequests {
		t.Fatalf("header sampah seharusnya berbagi jatah remote addr: status = %d, mau 429", status)
	}
}

// Tanpa TRUSTED_PROXIES, X-Real-IP diabaikan: klien tidak bisa mengakali limit
// dengan memalsukan header.
func TestTanpaTrustedProxyHeaderDiabaikan(t *testing.T) {
	app := newRouter(t, newTestConfig(t))

	habiskanJatah(t, app, "203.0.113.10")

	if status, _ := postContact(t, app, "/api/v1/contact", "203.0.113.20"); status != http.StatusTooManyRequests {
		t.Fatalf("header palsu seharusnya diabaikan: status = %d, mau 429", status)
	}
}

// Bila koneksi BUKAN dari proxy yang terdaftar, header juga diabaikan walau
// TRUSTED_PROXIES diisi — inilah yang mencegah spoofing dari klien langsung.
func TestHeaderDariProxyTakTerdaftarDiabaikan(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"10.0.0.1", "192.168.0.0/16"}
	app := newRouter(t, cfg)

	habiskanJatah(t, app, "203.0.113.10")

	if status, _ := postContact(t, app, "/api/v1/contact", "203.0.113.20"); status != http.StatusTooManyRequests {
		t.Fatalf("header dari peer tak tepercaya seharusnya diabaikan: status = %d, mau 429", status)
	}
}

// Entri IP tunggal dinormalkan sebelum diberikan ke Fiber. Fiber mencocokkan
// string persis dengan RemoteIP().String() ("0.0.0.0" pada app.Test), jadi
// tanpa normalisasi "::ffff:0.0.0.0" tidak pernah cocok dan X-Real-IP diabaikan.
func TestTrustedProxyIPv4MappedDinormalkan(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"::ffff:0.0.0.0"}
	app := newRouter(t, cfg)

	habiskanJatah(t, app, "203.0.113.10")

	if status, body := postContact(t, app, "/api/v1/contact", "203.0.113.20"); status != http.StatusUnprocessableEntity {
		t.Fatalf("proxy IPv4-mapped seharusnya dipercaya (jatah per X-Real-IP): status = %d, mau 422; body=%s",
			status, string(body))
	}
}

// postJSONVia mengirim body `{}` (selalu gagal validasi → 422 bila lolos
// limiter) ke path mana pun dengan X-Real-IP tertentu.
func postJSONVia(t *testing.T, app *fiber.App, path, realIP string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", realIP)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// SEMUA limiter (login, forgot, reset, contact) memakai kunci bersama: IPv6
// dikelompokkan per /64. Dua alamat berbeda dalam /64 yang sama berbagi jatah;
// /64 lain tetap punya jatah sendiri. Tanpa ini satu pelanggan IPv6 bisa memutar
// ~2^64 alamat untuk mengakali limit login.
func TestSemuaLimiterMengunciIPv6PerSlash64(t *testing.T) {
	cases := []struct {
		path string
		max  int
	}{
		{"/api/v1/auth/login", 10},
		{"/api/v1/auth/forgot-password", 5},
		{"/api/v1/auth/reset-password", 10},
		{"/api/v1/contact", 5},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.TrustedProxies = []string{"0.0.0.0"}
			app := newRouter(t, cfg)

			// Habiskan jatah dengan alamat yang BERBEDA-BEDA di /64 yang sama.
			for i := 1; i <= tc.max; i++ {
				ip := fmt.Sprintf("2001:db8:1:2::%x", i)
				if status := postJSONVia(t, app, tc.path, ip); status != http.StatusUnprocessableEntity {
					t.Fatalf("request ke-%d (%s): status = %d, mau 422 (lolos limiter)", i, ip, status)
				}
			}

			if status := postJSONVia(t, app, tc.path, "2001:db8:1:2:ffff:ffff:ffff:ffff"); status != http.StatusTooManyRequests {
				t.Fatalf("alamat lain di /64 yang sama: status = %d, mau 429 (jatah bersama)", status)
			}

			if status := postJSONVia(t, app, tc.path, "2001:db8:1:3::1"); status != http.StatusUnprocessableEntity {
				t.Fatalf("/64 berbeda: status = %d, mau 422 (jatah terpisah)", status)
			}

			// IPv4 tetap dikunci per alamat: klien IPv4 lain tidak terdampak.
			if status := postJSONVia(t, app, tc.path, "203.0.113.30"); status != http.StatusUnprocessableEntity {
				t.Fatalf("klien IPv4: status = %d, mau 422", status)
			}
		})
	}
}

// Frontend membaca Retry-After dari respons 429 untuk menampilkan "coba lagi
// dalam N detik". Header itu bukan CORS-safelisted, jadi harus tercantum di
// Access-Control-Expose-Headers, termasuk pada respons 429 yang dirender error
// handler pusat (bukan hanya respons sukses).
func TestCORSMengeksposRetryAfterPada429(t *testing.T) {
	app := newRouter(t, newTestConfig(t))
	const origin = "http://localhost:3000" // diizinkan oleh newTestConfig

	kirim := func() *http.Response {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/contact", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("app.Test error: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	for i := 1; i <= contactLimit; i++ {
		if resp := kirim(); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("request ke-%d: status = %d, mau 422", i, resp.StatusCode)
		}
	}

	resp := kirim()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, mau %q", got, origin)
	}
	if got := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(strings.ToLower(got), "retry-after") {
		t.Errorf("Access-Control-Expose-Headers = %q, harus memuat Retry-After", got)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("respons 429 seharusnya membawa header Retry-After")
	}
}

// Regresi N3(a): entri TRUSTED_PROXIES yang bermasalah dibuang di SEMUA
// environment. Dulu di luar produksi (mis. staging) "0.0.0.0/0" tetap dipasang
// ke Fiber dan hanya memicu peringatan, sehingga siapa pun bisa memalsukan
// X-Real-IP. Sekarang entri itu diabaikan: header palsu tidak berpengaruh dan
// semua request berbagi jatah alamat koneksi.
func TestTrustedProxyBermasalahDibuangDiLuarProduksi(t *testing.T) {
	for _, entry := range []string{"0.0.0.0/0", "::ffff:0:0/96", "0.0.0.0/1", "::/0"} {
		t.Run(entry, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.AppEnv = "staging"
			cfg.TrustedProxies = []string{entry}
			app := newRouter(t, cfg)

			habiskanJatah(t, app, "203.0.113.10")

			if status, _ := postContact(t, app, "/api/v1/contact", "203.0.113.20"); status != http.StatusTooManyRequests {
				t.Fatalf("%s seharusnya diabaikan (X-Real-IP palsu tak berpengaruh): status = %d, mau 429", entry, status)
			}
		})
	}
}

// Regresi N2 lewat router asli: X-Real-IP dengan grup hex bernol berlebih lolos
// validasi IP Fiber tetapi ditolak net.ParseIP. Semua variasinya harus berbagi
// satu jatah (jatuh ke alamat koneksi), bukan mendapat jatah baru masing-masing.
func TestVariasiPaddingIPv6BerbagiJatahLewatRouter(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"0.0.0.0"}
	app := newRouter(t, cfg)

	lolos := 0
	const total = 20
	for i := 1; i <= total; i++ {
		ip := fmt.Sprintf("2001:db8:1:2::%05x", i) // mis. "2001:db8:1:2::00001"
		switch status, body := postContact(t, app, "/api/v1/contact", ip); status {
		case http.StatusUnprocessableEntity:
			lolos++
		case http.StatusTooManyRequests:
		default:
			t.Fatalf("status tak terduga %d untuk %q; body=%s", status, ip, string(body))
		}
	}
	if lolos != contactLimit {
		t.Fatalf("variasi padding harus berbagi satu jatah: %d/%d lolos, mau %d", lolos, total, contactLimit)
	}
}
