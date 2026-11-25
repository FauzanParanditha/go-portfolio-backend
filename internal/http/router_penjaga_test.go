package http_test

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Invarian struktural: SETIAP route yang didaftarkan router asli harus
// berpenjaga, kecuali yang tertulis di routePublik di bawah.
//
// Test lain (middleware/require_role_test.go) membuktikan rantai AuthJWT +
// RequireRole bekerja pada app rakitan. Test ini membuktikan hal yang berbeda:
// bahwa router ASLI benar-benar memasang rantai itu di tiap route. Tanpa ini,
// grup admin baru yang lupa `RequireRole("admin")` lolos semua test lain.
//
// Yang diuji PERILAKU (status yang benar-benar dikembalikan), bukan teks
// router.go. Di Fiber, `admin.Use(...)` pada grup berlaku untuk AWALAN path-nya,
// bukan untuk objek grup itu saja: penjaga yang dipasang grup admin pertama ikut
// menjaga grup admin yang didaftarkan sesudahnya, dan sebaliknya route yang
// didaftarkan SEBELUM penjaga mana pun tidak terjaga. Grep "tiap fungsi punya
// RequireRole" buta terhadap urutan itu; memanggil route-nya tidak.
//
// DB-free: request tanpa kredensial berhenti di AuthJWT (401) dan token
// non-admin berhenti di RequireRole (403) — keduanya sebelum handler, jadi
// repository (DB nil) tidak pernah tersentuh.

// routePublik adalah daftar route yang MEMANG boleh diakses tanpa kredensial.
// Menambah baris di sini adalah keputusan sadar yang terlihat saat review —
// itulah gunanya daftar ini. Kunci: "METHOD path" persis seperti didaftarkan
// (HEAD mengikuti GET-nya, tidak perlu ditulis).
var routePublik = map[string]string{
	"GET /swagger/*":                    "dokumentasi API; hanya terpasang di luar produksi",
	"GET /healthz":                      "pemantauan (akar, tanpa envelope)",
	"GET /readyz":                       "pemantauan (akar, tanpa envelope)",
	"GET /api/v1/healthz":               "status publik untuk frontend, dibatasi 60/menit",
	"GET /api/v1/readyz":                "status publik untuk frontend, dibatasi 60/menit",
	"GET /api/v1/projects/":             "daftar proyek di situs publik",
	"GET /api/v1/projects/:slug":        "detail proyek di situs publik",
	"GET /api/v1/tags":                  "daftar tag di situs publik",
	"GET /api/v1/experiences/":          "daftar pengalaman di situs publik",
	"POST /api/v1/auth/login":           "pintu masuk; dibatasi 10/menit",
	"POST /api/v1/auth/logout":          "menghapus cookie; aman dipanggil tanpa sesi",
	"POST /api/v1/auth/forgot-password": "pengguna yang lupa password tidak punya sesi; dibatasi 5/menit",
	"POST /api/v1/auth/reset-password":  "dijaga token sekali pakai, bukan sesi; dibatasi 10/menit",
	"POST /api/v1/contact":              "form kontak publik; dibatasi 5/menit",
	"POST /api/v1/contact-messages":     "alias form kontak; berbagi jatah yang sama",
}

const awalanAdmin = "/api/v1/admin/"

// tokenPeran membuat JWT HS256 sah (tanda tangan & masa berlaku benar) untuk
// role tertentu, supaya penolakan yang diamati benar-benar berasal dari
// pemeriksaan role, bukan dari token yang rusak.
func tokenPeran(t *testing.T, secret, role string) string {
	t.Helper()

	now := time.Now()
	claims := handlers.JWTCustomClaims{
		UserID: "11111111-1111-1111-1111-111111111111",
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "jti-test-penjaga",
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("gagal menandatangani token: %v", err)
	}
	return signed
}

// isiParameter mengganti segmen ":nama" dan "*" dengan nilai konkret agar path
// yang didaftarkan bisa dipanggil.
func isiParameter(path string) string {
	segmen := strings.Split(path, "/")
	for i, s := range segmen {
		switch {
		case strings.HasPrefix(s, ":"):
			segmen[i] = "11111111-1111-1111-1111-111111111111"
		case s == "*":
			segmen[i] = "x"
		}
	}
	return strings.Join(segmen, "/")
}

func statusRoute(t *testing.T, app *fiber.App, method, path, token string) int {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: app.Test gagal: %v", method, path, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestSemuaRouteBerpenjaga(t *testing.T) {
	cfg := newTestConfig(t)
	app := newRouter(t, cfg)
	tokenBukanAdmin := tokenPeran(t, cfg.JWTSecret, "user")

	// GetRoutes(true) membuang route yang dipasang lewat Use/Static. Artinya
	// penyajian berkas /uploads (app.Static, publik secara sengaja) tidak
	// termasuk di sini; yang diperiksa adalah semua endpoint ber-handler.
	routes := app.GetRoutes(true)

	terlihat := map[string]bool{}
	var berpenjaga, admin int

	for _, r := range routes {
		kunci := r.Method + " " + r.Path
		// HEAD didaftarkan otomatis oleh Fiber untuk tiap GET dengan rantai
		// handler yang sama, jadi status publiknya mengikuti GET.
		kunciPublik := kunci
		if r.Method == http.MethodHead {
			kunciPublik = http.MethodGet + " " + r.Path
		}
		if _, publik := routePublik[kunciPublik]; publik {
			terlihat[kunciPublik] = true
			if strings.HasPrefix(r.Path, awalanAdmin) {
				t.Errorf("%s: route admin tidak boleh ada di routePublik", kunci)
			}
			continue
		}

		path := isiParameter(r.Path)

		if got := statusRoute(t, app, r.Method, path, ""); got != http.StatusUnauthorized {
			t.Errorf("%s tanpa kredensial: status %d, seharusnya 401 — "+
				"pasang AuthJWT, atau daftarkan di routePublik bila memang publik", kunci, got)
		}
		berpenjaga++

		if strings.HasPrefix(r.Path, awalanAdmin) {
			if got := statusRoute(t, app, r.Method, path, tokenBukanAdmin); got != http.StatusForbidden {
				t.Errorf("%s dengan token role \"user\": status %d, seharusnya 403 — "+
					"grup admin wajib AuthJWT lalu RequireRole(\"admin\")", kunci, got)
			}
			admin++
		}
	}

	// Jaga agar test tidak lulus kosong (mis. GetRoutes berubah perilaku).
	if admin == 0 {
		t.Fatal("tidak ada route admin yang terperiksa — test ini tidak menguji apa pun")
	}
	if berpenjaga == admin {
		t.Fatal("tidak ada route berpenjaga di luar admin (mis. /me) yang terperiksa")
	}

	// Baris routePublik yang tidak lagi cocok dengan route mana pun adalah izin
	// menggantung: bila path itu kelak dipakai lagi, ia langsung publik tanpa
	// ada yang memutuskannya.
	var basi []string
	for kunci := range routePublik {
		if !terlihat[kunci] {
			basi = append(basi, kunci)
		}
	}
	sort.Strings(basi)
	for _, kunci := range basi {
		t.Errorf("routePublik memuat %q yang tidak terdaftar di router — hapus barisnya", kunci)
	}
}
