package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/denylist"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// signTokenWithJTI menandatangani token HS256 dengan jti & exp tertentu.
func signTokenWithJTI(t *testing.T, secret, jti string) string {
	t.Helper()
	now := time.Now()
	claims := handlers.JWTCustomClaims{
		UserID: "11111111-1111-1111-1111-111111111111",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   "11111111-1111-1111-1111-111111111111",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("gagal sign token: %v", err)
	}
	return signed
}

func newRevokeApp(cfg *config.Config, dl denylist.Denylist) *fiber.App {
	app := fiber.New()
	g := app.Group("/protected")
	g.Use(middleware.AuthJWT(cfg, dl))
	g.Get("/", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app
}

// TestAuthJWTRejectsRevokedToken memastikan token yang jti-nya sudah dicabut
// (mis. setelah logout) ditolak 401, sementara jti lain tetap lolos.
func TestAuthJWTRejectsRevokedToken(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}
	dl := denylist.NewMemory()
	app := newRevokeApp(cfg, dl)

	revokedJTI := "revoked-jti-123"
	revokedToken := signTokenWithJTI(t, cfg.JWTSecret, revokedJTI)
	liveToken := signTokenWithJTI(t, cfg.JWTSecret, "still-valid-jti-456")

	// Sebelum dicabut: token lolos.
	if code := doGet(t, app, revokedToken); code != http.StatusOK {
		t.Fatalf("sebelum revoke: status = %d, mau 200", code)
	}

	// Cabut jti-nya.
	dl.Revoke(revokedJTI, time.Now().Add(time.Hour))

	// Setelah dicabut: token yang sama -> 401.
	if code := doGet(t, app, revokedToken); code != http.StatusUnauthorized {
		t.Errorf("setelah revoke: status = %d, mau 401", code)
	}

	// Token dengan jti lain tetap lolos.
	if code := doGet(t, app, liveToken); code != http.StatusOK {
		t.Errorf("token jti lain: status = %d, mau 200", code)
	}
}

func doGet(t *testing.T, app *fiber.App, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
