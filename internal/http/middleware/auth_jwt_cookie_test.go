package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/gofiber/fiber/v2"
)

// newAuthApp membangun app dengan HANYA AuthJWT (tanpa RequireRole) agar fokus
// menguji sumber token (header vs cookie). Handler echo role yang lolos.
func newAuthApp(cfg *config.Config) *fiber.App {
	app := fiber.New()
	g := app.Group("/protected")
	g.Use(middleware.AuthJWT(cfg, nil))
	g.Get("/", func(c *fiber.Ctx) error {
		role, _ := c.Locals("user_role").(string)
		return c.SendString("ok:" + role)
	})
	return app
}

// TestAuthJWTAcceptsCookie memastikan token via cookie `access_token` (tanpa
// header Authorization) diterima dan handler tercapai.
func TestAuthJWTAcceptsCookie(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}
	app := newAuthApp(cfg)

	token := signToken(t, cfg.JWTSecret, "admin")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, mau 200; body=%s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok:admin" {
		t.Errorf("body = %q, mau %q", string(body), "ok:admin")
	}
}

// TestAuthJWTHeaderTakesPrecedence memastikan header Authorization diprioritaskan
// di atas cookie: header valid + cookie sampah -> tetap lolos.
func TestAuthJWTHeaderTakesPrecedence(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}
	app := newAuthApp(cfg)

	headerToken := signToken(t, cfg.JWTSecret, "admin")

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	// Cookie sampah; harus diabaikan karena header diutamakan.
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "garbage.token.value"})

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, mau 200 (header diutamakan); body=%s", resp.StatusCode, string(body))
	}
}

// TestAuthJWTMissingCredentials memastikan tanpa header & tanpa cookie -> 401.
func TestAuthJWTMissingCredentials(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret-yang-cukup-panjang-untuk-uji"}
	app := newAuthApp(cfg)

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, mau 401", resp.StatusCode)
	}
}
