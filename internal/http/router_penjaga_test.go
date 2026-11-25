package http_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Invarian struktural: setiap endpoint ber-handler yang didaftarkan router asli
// harus berpenjaga, kecuali yang tertulis di routePublik di bawah.
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
//
// BATAS yang perlu diketahui (test ini tidak menggantikan review):
//   - Handler yang dipasang lewat `Use`/`Static` tidak punya route sendiri untuk
//     dipanggil. Yang dijaga hanyalah DAFTAR path-nya (jalurUse): path Use baru
//     menggagalkan test sampai seseorang menuliskannya beserta alasannya.
//     Middleware baru di path yang SUDAH terdaftar (mis. "/") tidak terdeteksi.
//   - Pemeriksaan role (403) hanya untuk awalan /api/v1/admin. Route berpenjaga
//     di luar awalan itu (mis. /me) cukup menolak request tanpa kredensial;
//     endpoint khusus admin HARUS berada di bawah /api/v1/admin.

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

// jalurUse adalah daftar path tempat sesuatu dipasang lewat `Use` atau
// `Static`. Pemasangan seperti itu tidak muncul sebagai route yang bisa
// dipanggil test ini, sehingga `app.Use("/internal", handler)` akan menjadi
// endpoint tanpa penjaga yang tak terlihat. Path baru harus ditulis di sini
// beserta alasannya.
var jalurUse = map[string]string{
	"/":             "middleware global (RegisterGlobal): recover, batas body, helmet, requestid, dst.",
	"/uploads":      "app.Static — penyajian berkas unggahan, publik secara sengaja",
	"/api/v1/admin": "pasangan penjaga AuthJWT + RequireRole(\"admin\") tiap grup admin",
}

const akarAdmin = "/api/v1/admin"

func routeAdmin(path string) bool {
	return path == akarAdmin || strings.HasPrefix(path, akarAdmin+"/")
}

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

var (
	segmenParameter = regexp.MustCompile(`^:[A-Za-z_][A-Za-z0-9_]*$`)
	segmenPolos     = regexp.MustCompile(`^[A-Za-z0-9._~-]*$`)
)

// isiParameter mengganti segmen ":nama" dan "*" dengan nilai konkret agar path
// yang didaftarkan bisa dipanggil. Hanya dua bentuk itu yang dipahami. Sintaks
// lain (constraint ":id<int>", opsional ":id?", "+", parameter di tengah
// segmen) ditolak: bila nilai tebakan tidak cocok dengan route-nya, request
// jatuh ke middleware lain dan test lulus tanpa pernah menyentuh route itu.
func isiParameter(path string) (string, bool) {
	segmen := strings.Split(path, "/")
	for i, s := range segmen {
		switch {
		case segmenParameter.MatchString(s):
			segmen[i] = "11111111-1111-1111-1111-111111111111"
		case s == "*":
			segmen[i] = "x"
		case !segmenPolos.MatchString(s):
			return "", false
		}
	}
	return strings.Join(segmen, "/"), true
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

	// Satu request dulu: route milik sub-app (`app.Mount`) baru masuk daftar
	// setelah Fiber menjalankan proses startup-nya, yang dipicu request pertama.
	if got := statusRoute(t, app, http.MethodGet, "/healthz", ""); got != http.StatusOK {
		t.Fatalf("GET /healthz: status %d, seharusnya 200", got)
	}

	// GetRoutes(true) hanya memuat endpoint ber-handler; pemasangan lewat
	// Use/Static dibuang dan diperiksa terpisah di bawah (jalurUse).
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
			if routeAdmin(r.Path) {
				t.Errorf("%s: route admin tidak boleh ada di routePublik", kunci)
			}
			continue
		}

		path, ok := isiParameter(r.Path)
		if !ok {
			t.Errorf("%s: sintaks path tidak dipahami test ini, jadi route-nya tidak bisa "+
				"dipastikan terpanggil — perluas isiParameter dengan nilai contoh yang cocok", kunci)
			continue
		}

		if got := statusRoute(t, app, r.Method, path, ""); got != http.StatusUnauthorized {
			t.Errorf("%s tanpa kredensial: status %d, seharusnya 401 — "+
				"pasang AuthJWT, atau daftarkan di routePublik bila memang publik", kunci, got)
		}
		berpenjaga++

		if routeAdmin(r.Path) {
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

	// Pemasangan lewat Use/Static = selisih daftar lengkap dan daftar
	// ber-handler, dihitung per "METHOD path" agar Use di path yang sama dengan
	// sebuah endpoint tetap terlihat.
	sisa := map[string]int{}
	for _, r := range app.GetRoutes() {
		sisa[r.Method+" "+r.Path]++
	}
	for _, r := range routes {
		sisa[r.Method+" "+r.Path]--
	}
	jalurTerpasang := map[string]bool{}
	for kunci, n := range sisa {
		if n > 0 {
			jalurTerpasang[kunci[strings.Index(kunci, " ")+1:]] = true
		}
	}
	var jalur []string
	for p := range jalurTerpasang {
		jalur = append(jalur, p)
	}
	for p := range jalurUse {
		if !jalurTerpasang[p] {
			jalur = append(jalur, p)
		}
	}
	sort.Strings(jalur)
	for _, p := range jalur {
		_, tercatat := jalurUse[p]
		switch {
		case jalurTerpasang[p] && !tercatat:
			t.Errorf("ada Use/Static di %q yang tidak tercatat di jalurUse — handler yang dipasang "+
				"begitu tidak diperiksa test ini; pastikan ia berpenjaga lalu catat alasannya", p)
		case !jalurTerpasang[p] && tercatat:
			t.Errorf("jalurUse memuat %q yang tidak lagi dipasang di router — hapus barisnya", p)
		}
	}
}
