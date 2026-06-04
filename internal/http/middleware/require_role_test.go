package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// signToken membuat JWT HS256 valid untuk role tertentu, ditandatangani dengan
// secret yang sama seperti yang dipakai middleware.
func signToken(t *testing.T, secret, role string) string {
	t.Helper()

	now := time.Now()
	claims := handlers.JWTCustomClaims{
		UserID: "11111111-1111-1111-1111-111111111111",
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("gagal sign token: %v", err)
	}
	return signed
}

// newAdminApp membangun app dengan rantai middleware yang sama persis dengan
// route admin di router.go: AuthJWT lalu RequireRole("admin").
func newAdminApp(cfg *config.Config) *fiber.App {
	app := fiber.New()
	admin := app.Group("/admin")
	admin.Use(middleware.AuthJWT(cfg))
	admin.Use(middleware.RequireRole("admin"))
	admin.Get("/secret", func(c *fiber.Ctx) error {
		return c.SendString("handler reached")
	})
	return app
}

func TestAdminRouteRejectsNonAdmin(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}
	app := newAdminApp(cfg)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "tanpa token -> 401",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token role=user -> 403 (ditolak)",
			authHeader: "Bearer " + signToken(t, cfg.JWTSecret, "user"),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "token role=editor -> 403 (ditolak)",
			authHeader: "Bearer " + signToken(t, cfg.JWTSecret, "editor"),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "token role=admin -> 200 (lolos)",
			authHeader: "Bearer " + signToken(t, cfg.JWTSecret, "admin"),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/secret", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("app.Test error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d, mau %d; body=%s", resp.StatusCode, tc.wantStatus, string(body))
			}
		})
	}
}

// TestAdminRouteRejectsTokenSignedWithWrongSecret memastikan token yang
// ditandatangani dengan secret lain (mis. secret default yang bocor) ditolak.
func TestAdminRouteRejectsTokenSignedWithWrongSecret(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret-asli-server-yang-kuat-dan-panjang"}
	app := newAdminApp(cfg)

	forged := signToken(t, "super-secret-ganti-sendiri", "admin")

	req := httptest.NewRequest(http.MethodGet, "/admin/secret", nil)
	req.Header.Set("Authorization", "Bearer "+forged)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("token dengan secret salah harusnya 401, dapat %d", resp.StatusCode)
	}
}
