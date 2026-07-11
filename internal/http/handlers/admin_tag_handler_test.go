package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Catatan: test ini menguji perilaku handler admin tag secara langsung (tanpa
// middleware auth). Invarian keamanan /admin/* (AuthJWT + RequireRole) sudah
// diuji terpisah di internal/http/middleware/require_role_test.go.

// TestAdminTagListReturnsEnvelope memastikan GET list membalas 200 dengan
// {data:[...], meta:{...}}.
func TestAdminTagListReturnsEnvelope(t *testing.T) {
	repo := &fakeTagRepo{
		listFn: func(_ context.Context, _ repository.TagListParams) ([]models.Tag, int64, error) {
			return []models.Tag{
				{ID: uuid.New(), Name: "Go", Type: "language"},
				{ID: uuid.New(), Name: "Fiber", Type: "framework"},
			}, 2, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/tags", handlers.NewAdminTagHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/tags", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	env := decode(t, body)
	if len(env["data"].([]any)) != 2 {
		t.Errorf("data harus 2 item")
	}
	if _, ok := env["meta"].(map[string]any); !ok {
		t.Errorf("meta hilang: %v", env["meta"])
	}
}

// TestAdminTagGetByIDFound memastikan GET detail 200 dengan {data:{...}}.
func TestAdminTagGetByIDFound(t *testing.T) {
	id := uuid.New()
	repo := &fakeTagRepo{
		getByIDFn: func(_ context.Context, gotID string) (*models.Tag, error) {
			if gotID != id.String() {
				t.Errorf("id diteruskan = %q, mau %q", gotID, id.String())
			}
			return &models.Tag{ID: id, Name: "Go", Type: "language"}, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/tags/:id", handlers.NewAdminTagHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/tags/"+id.String(), nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["name"] != "Go" {
		t.Errorf("data.name = %v, mau Go", data["name"])
	}
}

// TestAdminTagGetByIDNotFound memastikan eror repo dipetakan ke 404.
func TestAdminTagGetByIDNotFound(t *testing.T) {
	repo := &fakeTagRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.Tag, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Get("/admin/tags/:id", handlers.NewAdminTagHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/tags/"+uuid.New().String(), nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("error.code = %v, mau NOT_FOUND", errObj["code"])
	}
}

// TestAdminTagCreateValid memastikan POST valid membalas 201 + {data:{...}}.
func TestAdminTagCreateValid(t *testing.T) {
	var created *models.Tag
	repo := &fakeTagRepo{
		createFn: func(_ context.Context, tag *models.Tag) error {
			tag.ID = uuid.New()
			created = tag
			return nil
		},
	}

	app := newTestApp()
	app.Post("/admin/tags", handlers.NewAdminTagHandler(repo).Create)

	status, body := doJSON(t, app, postJSON(http.MethodPost, "/admin/tags", `{"name":"Go","type":"language"}`))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, mau 201; body=%s", status, string(body))
	}
	if created == nil || created.Name != "Go" {
		t.Fatalf("repo.Create tidak dipanggil benar: %+v", created)
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["type"] != "language" {
		t.Errorf("data.type = %v, mau language", data["type"])
	}
}

// TestAdminTagCreateValidationFails memastikan field kosong membalas 422 dengan details.
func TestAdminTagCreateValidationFails(t *testing.T) {
	repoCalled := false
	repo := &fakeTagRepo{
		createFn: func(_ context.Context, _ *models.Tag) error {
			repoCalled = true
			return nil
		},
	}

	app := newTestApp()
	app.Post("/admin/tags", handlers.NewAdminTagHandler(repo).Create)

	status, body := doJSON(t, app, postJSON(http.MethodPost, "/admin/tags", `{"name":"","type":""}`))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422; body=%s", status, string(body))
	}
	if repoCalled {
		t.Error("repo.Create tidak boleh dipanggil saat validasi gagal")
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("error.code = %v, mau VALIDATION_ERROR", errObj["code"])
	}
	if details, ok := errObj["details"].(map[string]any); !ok || len(details) == 0 {
		t.Errorf("details harus berisi field error: %v", errObj["details"])
	}
}

// TestAdminTagUpdateValid memastikan PUT valid membalas 200 + {data} dengan
// nilai terbaru; handler load dulu via GetByID lalu Update.
func TestAdminTagUpdateValid(t *testing.T) {
	id := uuid.New()
	updated := false
	repo := &fakeTagRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.Tag, error) {
			return &models.Tag{ID: id, Name: "Lama", Type: "language"}, nil
		},
		updateFn: func(_ context.Context, tag *models.Tag) error {
			updated = true
			if tag.Name != "Baru" {
				t.Errorf("tag.Name saat Update = %q, mau Baru", tag.Name)
			}
			return nil
		},
	}

	app := newTestApp()
	app.Put("/admin/tags/:id", handlers.NewAdminTagHandler(repo).Update)

	status, body := doJSON(t, app, postJSON(http.MethodPut, "/admin/tags/"+id.String(), `{"name":"Baru","type":"tool"}`))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	if !updated {
		t.Error("repo.Update tidak dipanggil")
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["name"] != "Baru" || data["type"] != "tool" {
		t.Errorf("data tidak diperbarui: %v", data)
	}
}

// TestAdminTagUpdateInvalidUUID memastikan ID non-UUID membalas 400 tanpa
// menyentuh repo.
func TestAdminTagUpdateInvalidUUID(t *testing.T) {
	repo := &fakeTagRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.Tag, error) {
			t.Fatal("GetByID tidak boleh dipanggil untuk UUID invalid")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Put("/admin/tags/:id", handlers.NewAdminTagHandler(repo).Update)

	status, _ := doJSON(t, app, postJSON(http.MethodPut, "/admin/tags/bukan-uuid", `{"name":"X","type":"y"}`))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}

// TestAdminTagUpdateNotFound memastikan tag tidak ada membalas 404.
func TestAdminTagUpdateNotFound(t *testing.T) {
	repo := &fakeTagRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.Tag, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Put("/admin/tags/:id", handlers.NewAdminTagHandler(repo).Update)

	status, _ := doJSON(t, app, postJSON(http.MethodPut, "/admin/tags/"+uuid.New().String(), `{"name":"X","type":"y"}`))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", status)
	}
}

// TestAdminTagDeleteValid memastikan DELETE valid membalas 204 (tanpa body).
func TestAdminTagDeleteValid(t *testing.T) {
	id := uuid.New()
	deleted := false
	repo := &fakeTagRepo{
		deleteFn: func(_ context.Context, gotID string) error {
			deleted = true
			if gotID != id.String() {
				t.Errorf("id delete = %q, mau %q", gotID, id.String())
			}
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/tags/:id", handlers.NewAdminTagHandler(repo).Delete)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/tags/"+id.String(), nil))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204; body=%s", status, string(body))
	}
	if !deleted {
		t.Error("repo.Delete tidak dipanggil")
	}
	if len(body) != 0 {
		t.Errorf("204 tidak boleh punya body, dapat: %s", string(body))
	}
}

// TestAdminTagDeleteInvalidUUID memastikan ID non-UUID membalas 400.
func TestAdminTagDeleteInvalidUUID(t *testing.T) {
	repo := &fakeTagRepo{
		deleteFn: func(_ context.Context, _ string) error {
			t.Fatal("Delete tidak boleh dipanggil untuk UUID invalid")
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/tags/:id", handlers.NewAdminTagHandler(repo).Delete)

	status, _ := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/tags/bukan-uuid", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}
