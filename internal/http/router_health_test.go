package http_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	apphttp "github.com/FauzanParanditha/portfolio-backend/internal/http"
	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Test di berkas ini memakai router ASLI (NewRouter) supaya yang diuji adalah
// wiring produksi: middleware global, NoStore, limiter bersama, dan error
// handler pusat. DB-free: Pinger diganti tiruan lewat AppDeps.Pinger, atau
// *gorm.DB dirakit di atas driver database/sql tiruan.

const statusLimit = 60

// pingerStub menghitung pemanggilan dan mengembalikan err yang ditentukan.
type pingerStub struct {
	calls atomic.Int32
	err   error
}

func (p *pingerStub) PingContext(context.Context) error {
	p.calls.Add(1)
	return p.err
}

func newStatusRouter(t *testing.T, cfg *config.Config, pinger *pingerStub) *fiber.App {
	t.Helper()
	return apphttp.NewRouter(apphttp.AppDeps{Config: cfg, Pinger: pinger})
}

// get menjalankan GET lewat app.Test; header opsional berpasangan kunci, nilai.
func get(t *testing.T, app *fiber.App, path string, headers ...string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("GET %s: app.Test error: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: gagal baca body: %v", path, err)
	}
	return resp, body
}

type errorEnvelope struct {
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Details any    `json:"details"`
	} `json:"error"`
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}

// assertErrorEnvelope memastikan body dibentuk error handler pusat:
// {"error":{"message","code","details":null},"requestId"} tanpa field `data`.
func assertErrorEnvelope(t *testing.T, body []byte, wantCode string) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body bukan JSON (%s): %v", body, err)
	}
	if env.Error == nil {
		t.Fatalf("envelope error tidak ada: %s", body)
	}
	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, mau %q", env.Error.Code, wantCode)
	}
	if env.Error.Message == "" {
		t.Error("error.message kosong")
	}
	if env.Error.Details != nil {
		t.Errorf("error.details = %v, mau null", env.Error.Details)
	}
	if env.Data != nil {
		t.Errorf("respons galat tidak boleh membawa `data`: %s", body)
	}
	if env.RequestID == "" {
		t.Errorf("requestId tidak ada di envelope: %s", body)
	}
	return env
}

func assertNoStore(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, mau \"no-store\" (status %d)", got, resp.StatusCode)
	}
}

func TestStatusHealthz200Envelope(t *testing.T) {
	pinger := &pingerStub{}
	app := newStatusRouter(t, newTestConfig(t), pinger)

	resp, body := get(t, app, "/api/v1/healthz")
	t.Logf("GET /api/v1/healthz → %d %s", resp.StatusCode, body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", resp.StatusCode, body)
	}
	if string(body) != `{"data":{"status":"ok"}}` {
		t.Errorf("body = %s, mau persis {\"data\":{\"status\":\"ok\"}}", body)
	}
	assertNoStore(t, resp)
	if n := pinger.calls.Load(); n != 0 {
		t.Errorf("liveness menyentuh Pinger %d kali, mau 0", n)
	}
}

func TestStatusReadyz200Envelope(t *testing.T) {
	pinger := &pingerStub{}
	app := newStatusRouter(t, newTestConfig(t), pinger)

	resp, body := get(t, app, "/api/v1/readyz")
	t.Logf("GET /api/v1/readyz → %d %s", resp.StatusCode, body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", resp.StatusCode, body)
	}
	if string(body) != `{"data":{"status":"ready"}}` {
		t.Errorf("body = %s, mau persis {\"data\":{\"status\":\"ready\"}}", body)
	}
	assertNoStore(t, resp)
	if n := pinger.calls.Load(); n != 1 {
		t.Errorf("Pinger dipanggil %d kali, mau 1", n)
	}
}

// Pinger gagal → 503 lewat error handler pusat dengan pesan generik. Teks galat
// asli (yang di dunia nyata memuat host, port, nama driver) tidak boleh ikut.
func TestStatusReadyz503TidakMembocorkanDetail(t *testing.T) {
	const rahasia = "pq-RAHASIA-7f3a: dial tcp 10.1.2.3:5432 connect: connection refused (database=portfolio)"
	pinger := &pingerStub{err: errors.New(rahasia)}
	app := newStatusRouter(t, newTestConfig(t), pinger)

	resp, body := get(t, app, "/api/v1/readyz")
	t.Logf("GET /api/v1/readyz (pinger gagal) → %d %s", resp.StatusCode, body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503; body=%s", resp.StatusCode, body)
	}
	env := assertErrorEnvelope(t, body, "SERVICE_UNAVAILABLE")
	if env.Error.Message != "layanan sedang terganggu" {
		t.Errorf("error.message = %q, mau \"layanan sedang terganggu\"", env.Error.Message)
	}
	assertNoStore(t, resp)

	// Tidak ada potongan galat asli maupun kata yang menunjuk ke database, baik
	// di body maupun di header respons.
	var headers strings.Builder
	for k, vs := range resp.Header {
		headers.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	full := strings.ToLower(string(body) + "\n" + headers.String())
	// requestId acak (UUID) dikeluarkan dari pemindaian: digit heksanya bisa
	// kebetulan memuat "5432"/"7f3a" dan membuat test ini flaky.
	full = strings.ReplaceAll(full, strings.ToLower(env.RequestID), "<request-id>")
	for _, terlarang := range []string{
		"rahasia", "7f3a", "10.1.2.3", "5432", "dial", "refused", "portfolio",
		"database", "db_", "postgres", "pq", "sql", "ping", "driver",
	} {
		if strings.Contains(full, terlarang) {
			t.Errorf("respons 503 memuat %q:\n%s", terlarang, full)
		}
	}

	// Liveness tetap 200 walau readiness gagal.
	if resp, body := get(t, app, "/api/v1/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("healthz saat readiness gagal: status = %d, mau 200; body=%s", resp.StatusCode, body)
	}
}

// Tanpa Pinger dan tanpa DB (seperti test router lain yang DB-nya nil), route
// baru tidak boleh panic: readiness 503 generik, liveness 200.
func TestStatusTanpaDBTidakPanic(t *testing.T) {
	app := newRouter(t, newTestConfig(t))

	resp, body := get(t, app, "/api/v1/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz: status = %d, mau 503; body=%s", resp.StatusCode, body)
	}
	env := assertErrorEnvelope(t, body, "SERVICE_UNAVAILABLE")
	if env.Error.Message != "layanan sedang terganggu" {
		t.Errorf("error.message = %q, mau pesan generik", env.Error.Message)
	}

	if resp, body := get(t, app, "/api/v1/healthz"); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: status = %d, mau 200; body=%s", resp.StatusCode, body)
	}
}

// Request ke-61 dalam semenit → 429 dengan envelope; /healthz dan /readyz
// berbagi satu jatah (berganti path tidak menggandakan kuota).
func TestStatusRateLimitJatahBersama(t *testing.T) {
	pinger := &pingerStub{}
	app := newStatusRouter(t, newTestConfig(t), pinger)

	for i := 1; i <= statusLimit; i++ {
		path := "/api/v1/healthz"
		if i%2 == 0 {
			path = "/api/v1/readyz"
		}
		if resp, body := get(t, app, path); resp.StatusCode != http.StatusOK {
			t.Fatalf("request ke-%d ke %s: status = %d, mau 200; body=%s", i, path, resp.StatusCode, body)
		}
	}

	for _, path := range []string{"/api/v1/healthz", "/api/v1/readyz"} {
		resp, body := get(t, app, path)
		t.Logf("GET %s (jatah habis) → %d %s", path, resp.StatusCode, body)

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("%s setelah %d request: status = %d, mau 429; body=%s", path, statusLimit, resp.StatusCode, body)
		}
		assertErrorEnvelope(t, body, "TOO_MANY_REQUESTS")
		assertNoStore(t, resp)
		if resp.Header.Get("Retry-After") == "" {
			t.Errorf("%s: respons 429 seharusnya membawa Retry-After", path)
		}
	}

	// Limiter berada di depan handler: 30 request readyz yang lolos dijawab
	// dari satu ping (memo), dan yang ditolak 429 tidak menambahnya.
	if n := pinger.calls.Load(); n != 1 {
		t.Errorf("Pinger dipanggil %d kali, mau 1", n)
	}

	// Route root tidak ikut jatah ini (monitoring tidak boleh ikut terblokir).
	if resp, body := get(t, app, "/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz root setelah jatah /api/v1 habis: status = %d, mau 200; body=%s", resp.StatusCode, body)
	}
}

// Jatah dihitung per klien: dengan proxy tepercaya, klien lain di balik proxy
// yang sama tidak ikut terblokir (limiter memakai kunci bersama rateLimitKey).
func TestStatusRateLimitPerKlien(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TrustedProxies = []string{"0.0.0.0"}
	app := newStatusRouter(t, cfg, &pingerStub{})

	for i := 1; i <= statusLimit; i++ {
		if resp, _ := get(t, app, "/api/v1/healthz", "X-Real-IP", "203.0.113.10"); resp.StatusCode != http.StatusOK {
			t.Fatalf("klien A request ke-%d: status = %d, mau 200", i, resp.StatusCode)
		}
	}
	if resp, _ := get(t, app, "/api/v1/healthz", "X-Real-IP", "203.0.113.10"); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("klien A setelah jatah habis: status = %d, mau 429", resp.StatusCode)
	}
	if resp, body := get(t, app, "/api/v1/healthz", "X-Real-IP", "203.0.113.20"); resp.StatusCode != http.StatusOK {
		t.Fatalf("klien B: status = %d, mau 200 (jatah terpisah); body=%s", resp.StatusCode, body)
	}
}

// Limiter status harus memakai kunci bersama middleware.NewRateLimiter: IPv6
// dikelompokkan per /64. Dua alamat berbeda dalam /64 yang sama berbagi jatah
// (kalau tidak, satu pelanggan IPv6 bisa memutar ~2^64 alamat dan tidak pernah
// kena limit); /64 lain dan klien IPv4 punya jatah sendiri. Test IPv4 di atas
// saja tidak menangkap limiter yang dipasang dengan kunci bawaan Fiber.
func TestStatusRateLimitMengunciIPv6PerSlash64(t *testing.T) {
	for _, path := range []string{"/api/v1/healthz", "/api/v1/readyz"} {
		t.Run(path, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.TrustedProxies = []string{"0.0.0.0"}
			app := newStatusRouter(t, cfg, &pingerStub{})

			// Habiskan jatah dengan alamat yang BERBEDA-BEDA di /64 yang sama.
			for i := 1; i <= statusLimit; i++ {
				ip := fmt.Sprintf("2001:db8:1:2::%x", i)
				if resp, body := get(t, app, path, "X-Real-IP", ip); resp.StatusCode != http.StatusOK {
					t.Fatalf("request ke-%d (%s): status = %d, mau 200; body=%s", i, ip, resp.StatusCode, body)
				}
			}

			resp, body := get(t, app, path, "X-Real-IP", "2001:db8:1:2:ffff:ffff:ffff:ffff")
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("alamat lain di /64 yang sama: status = %d, mau 429 (jatah bersama); body=%s", resp.StatusCode, body)
			}
			assertErrorEnvelope(t, body, "TOO_MANY_REQUESTS")

			if resp, body := get(t, app, path, "X-Real-IP", "2001:db8:1:3::1"); resp.StatusCode != http.StatusOK {
				t.Fatalf("/64 berbeda: status = %d, mau 200 (jatah terpisah); body=%s", resp.StatusCode, body)
			}
			if resp, body := get(t, app, path, "X-Real-IP", "203.0.113.30"); resp.StatusCode != http.StatusOK {
				t.Fatalf("klien IPv4: status = %d, mau 200; body=%s", resp.StatusCode, body)
			}
		})
	}
}

// Middleware global tetap berlaku untuk route baru: Content-Encoding selain
// identity ditolak 415 sebelum handler (dan sebelum Pinger) tersentuh.
func TestStatusMenolakContentEncoding(t *testing.T) {
	pinger := &pingerStub{}
	app := newStatusRouter(t, newTestConfig(t), pinger)

	for _, path := range []string{"/api/v1/healthz", "/api/v1/readyz"} {
		resp, body := get(t, app, path, "Content-Encoding", "gzip")
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("%s: status = %d, mau 415; body=%s", path, resp.StatusCode, body)
		}
		assertErrorEnvelope(t, body, "UNSUPPORTED_MEDIA_TYPE")
	}
	if n := pinger.calls.Load(); n != 0 {
		t.Errorf("Pinger dipanggil %d kali padahal request ditolak 415, mau 0", n)
	}
}

// Header keamanan (helmet), requestid, dan CORS dari rantai global ikut
// terpasang. CORS diperiksa juga pada 503: browser di origin lain harus bisa
// membaca respons galat, kalau tidak terminal status hanya melihat "network
// error" dan tidak bisa membedakannya dari API yang mati.
func TestStatusMembawaHeaderGlobal(t *testing.T) {
	const origin = "http://localhost:3000" // diizinkan oleh newTestConfig
	app := newStatusRouter(t, newTestConfig(t), &pingerStub{err: errors.New("gagal")})

	resp, _ := get(t, app, "/api/v1/healthz", "Origin", origin)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: status = %d, mau 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, mau nosniff (helmet)", got)
	}
	if resp.Header.Get("X-Request-Id") == "" {
		t.Error("X-Request-Id tidak ada (middleware requestid)")
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("healthz: Access-Control-Allow-Origin = %q, mau %q", got, origin)
	}

	resp, _ = get(t, app, "/api/v1/readyz", "Origin", origin)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz: status = %d, mau 503", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("readyz 503: Access-Control-Allow-Origin = %q, mau %q", got, origin)
	}
}

// --- Driver database/sql tiruan: *gorm.DB sungguhan tanpa PostgreSQL ---

type konektorTiruan struct{ err error }

func (k konektorTiruan) Connect(context.Context) (driver.Conn, error) {
	if k.err != nil {
		return nil, k.err
	}
	return koneksiTiruan{}, nil
}

func (konektorTiruan) Driver() driver.Driver { return driverTiruan{} }

type driverTiruan struct{}

func (driverTiruan) Open(string) (driver.Conn, error) {
	return nil, errors.New("driver tiruan: Open tidak dipakai")
}

type koneksiTiruan struct{}

func (koneksiTiruan) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("driver tiruan: query tidak didukung")
}
func (koneksiTiruan) Close() error { return nil }
func (koneksiTiruan) Begin() (driver.Tx, error) {
	return nil, errors.New("driver tiruan: transaksi tidak didukung")
}
func (koneksiTiruan) Ping(context.Context) error { return nil }

// newGormTiruan merakit *gorm.DB di atas driver tiruan. connectErr != nil
// membuat setiap ping gagal dengan galat itu.
func newGormTiruan(t *testing.T, connectErr error) *gorm.DB {
	t.Helper()
	sqlDB := sql.OpenDB(konektorTiruan{err: connectErr})
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gagal merakit gorm di atas driver tiruan: %v", err)
	}
	return gdb
}

// Route root /healthz dan /readyz dipakai monitoring/orkestrasi dan TIDAK boleh
// berubah: tetap di luar envelope, tetap tanpa Cache-Control, dan /readyz
// tetap melaporkan "db_unreachable". Sekaligus menguji wiring produksi route
// baru: tanpa AppDeps.Pinger, readiness memakai *sql.DB di balik deps.DB.
func TestRouteRootTidakBerubahDanWiringDariDB(t *testing.T) {
	t.Run("database terjangkau", func(t *testing.T) {
		app := apphttp.NewRouter(apphttp.AppDeps{Config: newTestConfig(t), DB: newGormTiruan(t, nil)})

		resp, body := get(t, app, "/healthz")
		if resp.StatusCode != http.StatusOK || string(body) != `{"status":"ok"}` {
			t.Errorf("/healthz: %d %s, mau 200 {\"status\":\"ok\"}", resp.StatusCode, body)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "" {
			t.Errorf("/healthz root kini membawa Cache-Control %q; route root tidak boleh berubah", cc)
		}

		resp, body = get(t, app, "/readyz")
		if resp.StatusCode != http.StatusOK || string(body) != `{"status":"ready"}` {
			t.Errorf("/readyz: %d %s, mau 200 {\"status\":\"ready\"}", resp.StatusCode, body)
		}

		resp, body = get(t, app, "/api/v1/readyz")
		if resp.StatusCode != http.StatusOK || string(body) != `{"data":{"status":"ready"}}` {
			t.Errorf("/api/v1/readyz: %d %s, mau 200 {\"data\":{\"status\":\"ready\"}}", resp.StatusCode, body)
		}
	})

	t.Run("database tidak terjangkau", func(t *testing.T) {
		const rahasia = "konektor-RAHASIA-91c2: host db.internal tidak terjangkau"
		app := apphttp.NewRouter(apphttp.AppDeps{Config: newTestConfig(t), DB: newGormTiruan(t, errors.New(rahasia))})

		resp, body := get(t, app, "/healthz")
		if resp.StatusCode != http.StatusOK || string(body) != `{"status":"ok"}` {
			t.Errorf("/healthz: %d %s, mau 200 {\"status\":\"ok\"}", resp.StatusCode, body)
		}

		// Perilaku lama route root dipertahankan apa adanya.
		resp, body = get(t, app, "/readyz")
		if resp.StatusCode != http.StatusServiceUnavailable || string(body) != `{"status":"db_unreachable"}` {
			t.Errorf("/readyz: %d %s, mau 503 {\"status\":\"db_unreachable\"}", resp.StatusCode, body)
		}

		// Route baru: 503 generik lewat error handler pusat.
		resp, body = get(t, app, "/api/v1/readyz")
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("/api/v1/readyz: status = %d, mau 503; body=%s", resp.StatusCode, body)
		}
		assertErrorEnvelope(t, body, "SERVICE_UNAVAILABLE")
		for _, terlarang := range []string{"RAHASIA", "91c2", "db.internal", "db_unreachable", "konektor"} {
			if strings.Contains(string(body), terlarang) {
				t.Errorf("/api/v1/readyz membocorkan %q: %s", terlarang, body)
			}
		}
	})
}
