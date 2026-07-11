package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// testCfg mengembalikan config aman untuk test (secret >= 32 char) agar
// issueToken dapat menandatangani JWT tanpa menyentuh Validate().
func testCfg() *config.Config {
	return &config.Config{
		AppEnv:       "development",
		JWTSecret:    "test-secret-yang-cukup-panjang-3232",
		JWTExpiresIn: 1800,
	}
}

// bcryptHash membuat hash bcrypt dari password uji.
func bcryptHash(t *testing.T, pw string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("gagal membuat hash bcrypt: %v", err)
	}
	return string(b)
}

// TestLoginValidCredentials memastikan kredensial benar membalas 200 dengan
// envelope {data:{token,tokenType,expiresIn}} dan men-set cookie access_token.
func TestLoginValidCredentials(t *testing.T) {
	userID := uuid.New()
	repo := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, email string) (*models.User, error) {
			return &models.User{
				ID:       userID,
				Name:     "Admin",
				Email:    email,
				Password: bcryptHash(t, "rahasia123"),
				Role:     "admin",
			}, nil
		},
	}

	app := newTestApp()
	h := handlers.NewAuthHandler(repo, testCfg(), nil)
	app.Post("/auth/login", h.Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"admin@example.com","password":"rahasia123"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200", resp.StatusCode)
	}

	// Cookie access_token harus di-set.
	if !strings.Contains(strings.ToLower(resp.Header.Get("Set-Cookie")), "access_token=") {
		t.Errorf("cookie access_token tidak di-set: %q", resp.Header.Get("Set-Cookie"))
	}

	body := readAll(t, resp.Body)
	data := decode(t, body)["data"].(map[string]any)
	if data["token"] == "" || data["token"] == nil {
		t.Errorf("token kosong di response: %v", data)
	}
	if data["tokenType"] != "Bearer" {
		t.Errorf("tokenType = %v, mau Bearer", data["tokenType"])
	}
	if data["expiresIn"].(float64) != 1800 {
		t.Errorf("expiresIn = %v, mau 1800", data["expiresIn"])
	}
}

// TestLoginWrongPassword memastikan password salah membalas 401 UNAUTHORIZED
// (hash cocok untuk password lain, jadi CompareHashAndPassword gagal).
func TestLoginWrongPassword(t *testing.T) {
	repo := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, email string) (*models.User, error) {
			return &models.User{
				ID:       uuid.New(),
				Email:    email,
				Password: bcryptHash(t, "password-benar"),
				Role:     "admin",
			}, nil
		},
	}

	app := newTestApp()
	app.Post("/auth/login", handlers.NewAuthHandler(repo, testCfg(), nil).Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"admin@example.com","password":"password-salah"}`))
	req.Header.Set("Content-Type", "application/json")

	status, body := doJSON(t, app, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "UNAUTHORIZED" {
		t.Errorf("error.code = %v, mau UNAUTHORIZED", errObj["code"])
	}
}

// TestLoginEmailNotFound memastikan email tak ada (gorm.ErrRecordNotFound dari
// repo) membalas 401 UNAUTHORIZED tanpa membocorkan detail.
func TestLoginEmailNotFound(t *testing.T) {
	repo := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Post("/auth/login", handlers.NewAuthHandler(repo, testCfg(), nil).Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"tidak-ada@example.com","password":"apapun123"}`))
	req.Header.Set("Content-Type", "application/json")

	status, body := doJSON(t, app, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "UNAUTHORIZED" {
		t.Errorf("error.code = %v, mau UNAUTHORIZED", errObj["code"])
	}
}

// TestLoginInvalidBody memastikan JSON rusak membalas 400 BAD_REQUEST
// sebelum menyentuh repo.
func TestLoginInvalidBody(t *testing.T) {
	repo := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			t.Fatal("repo tidak boleh dipanggil saat body rusak")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Post("/auth/login", handlers.NewAuthHandler(repo, testCfg(), nil).Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":`)) // JSON tidak lengkap
	req.Header.Set("Content-Type", "application/json")

	status, body := doJSON(t, app, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400; body=%s", status, string(body))
	}
}

// TestLoginValidationFails memastikan field kosong (email/password) membalas
// 422 dengan code VALIDATION_ERROR dan map details.
func TestLoginValidationFails(t *testing.T) {
	repo := &fakeUserRepo{
		findByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			t.Fatal("repo tidak boleh dipanggil saat validasi gagal")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Post("/auth/login", handlers.NewAuthHandler(repo, testCfg(), nil).Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"","password":""}`))
	req.Header.Set("Content-Type", "application/json")

	status, body := doJSON(t, app, req)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("error.code = %v, mau VALIDATION_ERROR", errObj["code"])
	}
	if _, ok := errObj["details"].(map[string]any); !ok {
		t.Errorf("error.details bukan map: %v", errObj["details"])
	}
}

// TestRefreshIssuesNewToken memastikan Refresh (dengan Locals user_id/user_role
// yang sudah di-set oleh AuthJWT) membalas 200 + envelope token baru & cookie.
func TestRefreshIssuesNewToken(t *testing.T) {
	app := newTestApp()
	h := handlers.NewAuthHandler(&fakeUserRepo{}, testCfg(), nil)

	// Middleware kecil mensimulasikan AuthJWT: mengisi Locals sebelum handler.
	app.Post("/auth/refresh", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New().String())
		c.Locals("user_role", "admin")
		return h.Refresh(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Set-Cookie")), "access_token=") {
		t.Errorf("cookie access_token tidak di-set ulang: %q", resp.Header.Get("Set-Cookie"))
	}
	data := decode(t, readAll(t, resp.Body))["data"].(map[string]any)
	if data["token"] == "" || data["token"] == nil {
		t.Errorf("token kosong di response refresh: %v", data)
	}
}

// TestRefreshWithoutAuthReturns401 memastikan tanpa Locals user_id Refresh
// membalas 401 UNAUTHORIZED.
func TestRefreshWithoutAuthReturns401(t *testing.T) {
	app := newTestApp()
	h := handlers.NewAuthHandler(&fakeUserRepo{}, testCfg(), nil)
	app.Post("/auth/refresh", h.Refresh)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodPost, "/auth/refresh", nil))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "UNAUTHORIZED" {
		t.Errorf("error.code = %v, mau UNAUTHORIZED", errObj["code"])
	}
}
