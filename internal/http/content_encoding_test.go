package http

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/FauzanParanditha/portfolio-backend/internal/storage"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Test di file ini sengaja DB-free: rantai middleware global dan router asli
// dijalankan lewat app.Test tanpa koneksi database.

const testJWTSecret = "test-secret-yang-cukup-panjang-untuk-uji-content-encoding"

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		AppEnv:             "test",
		JWTSecret:          testJWTSecret,
		JWTExpiresIn:       1800,
		CORSAllowedOrigins: "http://localhost:3000",
		CORSAllowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		CORSAllowedHeaders: "Origin, Content-Type, Accept, Authorization",
		AppPublicURL:       "http://localhost:8080",
		UploadDir:          t.TempDir(),
		UploadMaxBytes:     5 * 1024 * 1024,
	}
}

// newGuardedApp membangun app dengan error handler pusat dan middleware global
// yang sama persis dengan produksi, plus satu route yang membaca body lewat
// c.Body() (jalur yang mendekompresi otomatis). `calls` menghitung berapa kali
// handler benar-benar terpanggil.
func newGuardedApp(t *testing.T) (*fiber.App, *atomic.Int32) {
	t.Helper()

	app := fiber.New(fiber.Config{ErrorHandler: NewErrorHandler()})
	middleware.RegisterGlobal(app, testConfig(t))

	var calls atomic.Int32
	app.Post("/echo", func(c *fiber.Ctx) error {
		calls.Add(1)
		return c.SendString(strconv.Itoa(len(c.Body())))
	})
	return app, &calls
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gagal gzip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gagal menutup gzip: %v", err)
	}
	return buf.Bytes()
}

func deflateBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("gagal membuat writer deflate: %v", err)
	}
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gagal deflate: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gagal menutup deflate: %v", err)
	}
	return buf.Bytes()
}

// gzipBomb membuat body gzip berisi `size` byte nol. Ditulis bertahap supaya
// memori test tetap kecil; hasil kompresinya hanya ~0,1% dari ukuran asli.
func gzipBomb(t *testing.T, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gagal membuat writer gzip: %v", err)
	}
	chunk := make([]byte, 1<<20)
	for written := 0; written < size; written += len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatalf("gagal menulis bomb: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gagal menutup gzip: %v", err)
	}
	return buf.Bytes()
}

// doRequest menjalankan request lewat app.Test dan mengembalikan status + body.
func doRequest(t *testing.T, app *fiber.App, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := app.Test(req, 30_000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("gagal membaca body respons: %v", err)
	}
	return resp.StatusCode, body
}

// assertUnsupportedEnvelope memastikan respons 415 dibentuk error handler pusat:
// {"error":{"message","code":"UNSUPPORTED_MEDIA_TYPE","details":null},"requestId"}.
func assertUnsupportedEnvelope(t *testing.T, status int, body []byte) {
	t.Helper()

	if status != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, mau 415. body: %s", status, body)
	}

	var env struct {
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
			Details any    `json:"details"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("respons bukan JSON envelope: %v. body: %s", err, body)
	}
	if env.Error == nil {
		t.Fatalf("field `error` tidak ada di envelope: %s", body)
	}
	if env.Error.Code != "UNSUPPORTED_MEDIA_TYPE" {
		t.Errorf("error.code = %q, mau UNSUPPORTED_MEDIA_TYPE", env.Error.Code)
	}
	if env.Error.Message == "" {
		t.Errorf("error.message kosong")
	}
	if env.Error.Details != nil {
		t.Errorf("error.details = %v, mau null", env.Error.Details)
	}
	if env.RequestID == "" {
		t.Errorf("requestId kosong: guard harus berjalan setelah middleware requestid")
	}
}

func TestContentEncodingTerkompresiDitolak415(t *testing.T) {
	payload := []byte(`{"email":"a@b.c","password":"rahasia123"}`)

	cases := []struct {
		name     string
		encoding []string // satu elemen = satu baris header
		body     []byte
	}{
		{"gzip", []string{"gzip"}, gzipBytes(t, payload)},
		{"br", []string{"br"}, payload}, // header saja sudah cukup untuk ditolak
		{"deflate", []string{"deflate"}, deflateBytes(t, payload)},
		{"gzip berantai", []string{"gzip, gzip"}, gzipBytes(t, gzipBytes(t, payload))},
		{"huruf besar", []string{"GZIP"}, gzipBytes(t, payload)},
		{"identity lalu gzip", []string{"identity, gzip"}, gzipBytes(t, payload)},
		{"zstd", []string{"zstd"}, payload},
		{"encoding tak dikenal", []string{"x-custom"}, payload},
		{"dua baris header", []string{"identity", "gzip"}, gzipBytes(t, payload)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, calls := newGuardedApp(t)

			req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			for _, v := range tc.encoding {
				req.Header.Add("Content-Encoding", v)
			}

			status, body := doRequest(t, app, req)
			assertUnsupportedEnvelope(t, status, body)
			if n := calls.Load(); n != 0 {
				t.Errorf("handler terpanggil %d kali, mau 0", n)
			}
		})
	}
}

func TestContentEncodingIdentityAtauKosongLolos(t *testing.T) {
	payload := []byte(`{"email":"a@b.c","password":"rahasia123"}`)

	cases := []struct {
		name     string
		encoding *string
	}{
		{"tanpa header", nil},
		{"identity", new("identity")},
		{"IDENTITY huruf besar", new("IDENTITY")},
		{"nilai kosong", new("")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, calls := newGuardedApp(t)

			req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			if tc.encoding != nil {
				req.Header.Set("Content-Encoding", *tc.encoding)
			}

			status, body := doRequest(t, app, req)
			if status != http.StatusOK {
				t.Fatalf("status = %d, mau 200. body: %s", status, body)
			}
			if string(body) != strconv.Itoa(len(payload)) {
				t.Errorf("handler membaca %s byte, mau %d", body, len(payload))
			}
			if n := calls.Load(); n != 1 {
				t.Errorf("handler terpanggil %d kali, mau 1", n)
			}
		})
	}
}

// Regression decompression bomb: body gzip kecil (lolos guard Content-Length
// 1 MB) yang mengembang jadi 200 MB harus ditolak SEBELUM ada yang membacanya.
// Handler yang memanggil c.Body() tidak boleh terpanggil sama sekali.
func TestGzipBombTidakPernahDidekompresi(t *testing.T) {
	const decompressedSize = 200 << 20 // 200 MB, setara temuan auditor
	bomb := gzipBomb(t, decompressedSize)

	if len(bomb) >= 1<<20 {
		t.Fatalf("ukuran bomb %d byte, harus < 1 MB agar membuktikan guard Content-Length tidak cukup", len(bomb))
	}

	for _, encoding := range []string{"gzip", "gzip, gzip"} {
		t.Run(encoding, func(t *testing.T) {
			app, calls := newGuardedApp(t)

			req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(bomb))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", encoding)

			status, body := doRequest(t, app, req)
			assertUnsupportedEnvelope(t, status, body)
			if n := calls.Load(); n != 0 {
				t.Fatalf("handler terpanggil %d kali: body bomb sempat dibaca/didekompresi", n)
			}
		})
	}
}

// Kontrol: membuktikan bahwa TANPA guard, Fiber memang mendekompresi body tanpa
// batas — jadi regression test di atas benar-benar menguji sesuatu. Bila test
// ini gagal, kemungkinan Fiber/fasthttp sudah mengubah perilakunya; tinjau
// ulang apakah guard masih diperlukan, jangan langsung menghapus test-nya.
func TestKontrolTanpaGuardFiberMendekompresi(t *testing.T) {
	const decompressedSize = 8 << 20 // 8 MB, cukup untuk melampaui guard 1 MB
	bomb := gzipBomb(t, decompressedSize)

	app := fiber.New(fiber.Config{ErrorHandler: NewErrorHandler()})
	app.Post("/echo", func(c *fiber.Ctx) error {
		return c.SendString(strconv.Itoa(len(c.Body())))
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(bomb))
	req.Header.Set("Content-Encoding", "gzip")

	status, body := doRequest(t, app, req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body: %s", status, body)
	}
	if string(body) != strconv.Itoa(decompressedSize) {
		t.Fatalf("len(c.Body()) = %s, mau %d (body %d byte terkompresi)", body, decompressedSize, len(bomb))
	}
}

// --- Wiring di router produksi ---

// fakeStorage mencatat pemanggilan Save tanpa menyentuh disk.
type fakeStorage struct {
	calls atomic.Int32
}

func (f *fakeStorage) Save(data []byte, _ string) (*storage.SavedFile, error) {
	f.calls.Add(1)
	return &storage.SavedFile{
		Filename:    "abc.png",
		URL:         "http://localhost:8080/uploads/abc.png",
		ContentType: "image/png",
		Size:        int64(len(data)),
	}, nil
}

func signAdminToken(t *testing.T) string {
	t.Helper()
	now := time.Now()
	claims := handlers.JWTCustomClaims{
		UserID: "11111111-1111-1111-1111-111111111111",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("gagal sign token: %v", err)
	}
	return signed
}

func uploadRequest(t *testing.T, encoding string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "foto.png")
	if err != nil {
		t.Fatalf("CreateFormFile error: %v", err)
	}
	if _, err := part.Write([]byte("isi-gambar-palsu")); err != nil {
		t.Fatalf("gagal menulis isi berkas: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gagal menutup writer multipart: %v", err)
	}

	body := buf.Bytes()
	if encoding == "gzip" {
		body = gzipBytes(t, body)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/uploads", bytes.NewReader(body))
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+signAdminToken(t))
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	return req
}

// Endpoint publik tanpa auth yang menjadi vektor serangan harus menolak body
// terkompresi di router produksi. DB sengaja nil: bila handler sampai terpanggil,
// test akan gagal (status bukan 415), bukan diam-diam lolos.
func TestRouterMenolakContentEncodingDiEndpointPublik(t *testing.T) {
	app := NewRouter(AppDeps{Config: testConfig(t), Storage: &fakeStorage{}})
	payload := gzipBytes(t, []byte(`{"email":"a@b.c","password":"rahasia123","name":"x","message":"y"}`))

	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/contact",
		"/api/v1/contact-messages",
		"/api/v1/auth/forgot-password",
		"/api/v1/auth/reset-password",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")

			status, body := doRequest(t, app, req)
			assertUnsupportedEnvelope(t, status, body)
		})
	}
}

// Unggahan multipart normal (tanpa Content-Encoding) tetap berfungsi, sedangkan
// multipart ber-gzip — yang akan di-gunzip fasthttp tanpa batas — ditolak.
func TestRouterUploadMultipart(t *testing.T) {
	t.Run("tanpa Content-Encoding lolos", func(t *testing.T) {
		store := &fakeStorage{}
		app := NewRouter(AppDeps{Config: testConfig(t), Storage: store})

		status, body := doRequest(t, app, uploadRequest(t, ""))
		if status != http.StatusCreated {
			t.Fatalf("status = %d, mau 201. body: %s", status, body)
		}
		if n := store.calls.Load(); n != 1 {
			t.Errorf("storage terpanggil %d kali, mau 1", n)
		}
	})

	t.Run("gzip ditolak 415", func(t *testing.T) {
		store := &fakeStorage{}
		app := NewRouter(AppDeps{Config: testConfig(t), Storage: store})

		status, body := doRequest(t, app, uploadRequest(t, "gzip"))
		assertUnsupportedEnvelope(t, status, body)
		if n := store.calls.Load(); n != 0 {
			t.Errorf("storage terpanggil %d kali, mau 0", n)
		}
	})
}
