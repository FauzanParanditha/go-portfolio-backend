package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestMeFromLocals memastikan bila middleware sudah mengisi name/email di Locals,
// handler membalas 200 + {data} tanpa menyentuh repo (DB).
func TestMeFromLocals(t *testing.T) {
	repo := &fakeUserRepo{
		findByIDFn: func(_ context.Context, _ string) (*models.User, error) {
			t.Fatal("repo tidak boleh dipanggil bila name/email sudah ada di Locals")
			return nil, nil
		},
	}

	app := newTestApp()
	h := handlers.NewMeHandler(repo)
	id := uuid.New().String()
	app.Get("/me", func(c *fiber.Ctx) error {
		c.Locals("user_id", id)
		c.Locals("user_name", "Admin")
		c.Locals("user_email", "admin@example.com")
		c.Locals("user_role", "admin")
		return h.Me(c)
	})

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/me", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["email"] != "admin@example.com" || data["name"] != "Admin" || data["role"] != "admin" {
		t.Errorf("data tidak sesuai: %v", data)
	}
	// Password tidak boleh ikut terekspos.
	if _, ok := data["password"]; ok {
		t.Errorf("field password bocor di response: %v", data)
	}
}

// TestMeFallbackToDB memastikan bila name/email Locals kosong, handler
// mengambil user via repo.FindByID lalu membalas 200 + {data}.
func TestMeFallbackToDB(t *testing.T) {
	id := uuid.New()
	var gotID string
	repo := &fakeUserRepo{
		findByIDFn: func(_ context.Context, uid string) (*models.User, error) {
			gotID = uid
			return &models.User{
				ID:       id,
				Name:     "Fauzan",
				Email:    "fauzan@example.com",
				Password: "hash-rahasia",
				Role:     "admin",
			}, nil
		},
	}

	app := newTestApp()
	h := handlers.NewMeHandler(repo)
	app.Get("/me", func(c *fiber.Ctx) error {
		c.Locals("user_id", id.String())
		// name/email sengaja tidak di-set → memicu fallback ke DB.
		return h.Me(c)
	})

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/me", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	if gotID != id.String() {
		t.Errorf("id diteruskan ke repo = %q, mau %q", gotID, id.String())
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["email"] != "fauzan@example.com" || data["name"] != "Fauzan" {
		t.Errorf("data tidak sesuai: %v", data)
	}
	if _, ok := data["password"]; ok {
		t.Errorf("field password bocor di response: %v", data)
	}
}

// TestMeUserNotFound memastikan bila repo.FindByID mengembalikan
// gorm.ErrRecordNotFound (fallback DB), handler membalas 500 SERVER_ERROR.
func TestMeUserNotFound(t *testing.T) {
	repo := &fakeUserRepo{
		findByIDFn: func(_ context.Context, _ string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	h := handlers.NewMeHandler(repo)
	app.Get("/me", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New().String())
		return h.Me(c)
	})

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/me", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "SERVER_ERROR" {
		t.Errorf("error.code = %v, mau SERVER_ERROR", errObj["code"])
	}
}

// TestMeUnauthorized memastikan tanpa Locals user_id, handler membalas 401.
func TestMeUnauthorized(t *testing.T) {
	app := newTestApp()
	h := handlers.NewMeHandler(&fakeUserRepo{})
	app.Get("/me", h.Me)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/me", nil))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "UNAUTHORIZED" {
		t.Errorf("error.code = %v, mau UNAUTHORIZED", errObj["code"])
	}
}
