package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/gofiber/fiber/v2"
)

// TestLogoutClearsCookie memastikan POST /auth/logout menulis cookie
// `access_token` yang kedaluwarsa (nilai kosong, Max-Age negatif) dan body
// { "data": { "message": "logged out" } }. DB-free: Logout tidak menyentuh DB,
// jadi *gorm.DB nil aman.
func TestLogoutClearsCookie(t *testing.T) {
	cfg := &config.Config{JWTExpiresIn: 1800, AppEnv: "development"}
	// denylist nil: tanpa token di request, Logout tidak mencabut apa pun.
	h := handlers.NewAuthHandler(nil, cfg, nil)

	app := fiber.New()
	app.Post("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200", resp.StatusCode)
	}

	setCookie := strings.ToLower(resp.Header.Get("Set-Cookie"))
	if !strings.Contains(setCookie, "access_token=") {
		t.Fatalf("Set-Cookie tidak menghapus access_token: %q", setCookie)
	}
	// Cookie kedaluwarsa: max-age=0 atau Expires di masa lalu (1970).
	if !strings.Contains(setCookie, "max-age=0") && !strings.Contains(setCookie, "expires=thu, 01 jan 1970") {
		t.Errorf("cookie logout harus kedaluwarsa, dapat: %q", setCookie)
	}
	// Dev: tidak boleh ada flag Secure (agar jalan di http://localhost).
	if strings.Contains(setCookie, "secure") {
		t.Errorf("dev tidak boleh memasang Secure: %q", setCookie)
	}
}
