package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Catatan: AdminContactHandler kini hanya menerima repository (semua akses DB
// lewat repo), jadi test cukup memakai fake repo tanpa database.
// Invarian auth /admin/* diuji terpisah di require_role_test.go.

// TestAdminContactListReturnsEnvelope memastikan GET list 200 dengan
// {data:[...], meta:{...}} dan meneruskan filter isRead ke repo.
func TestAdminContactListReturnsEnvelope(t *testing.T) {
	var gotParams repository.ContactListParams
	repo := &fakeContactRepo{
		listFn: func(_ context.Context, params repository.ContactListParams) ([]models.ContactMessage, int64, error) {
			gotParams = params
			return []models.ContactMessage{
				{ID: uuid.New(), Name: "Budi", Email: "budi@example.com", Subject: "Halo", CreatedAt: time.Now()},
			}, 1, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/contact-messages", handlers.NewAdminContactHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/contact-messages?isRead=true&q=budi", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	env := decode(t, body)
	if len(env["data"].([]any)) != 1 {
		t.Errorf("data harus 1 item")
	}
	if _, ok := env["meta"].(map[string]any); !ok {
		t.Errorf("meta hilang")
	}
	// Filter isRead=true harus diteruskan sebagai *bool bernilai true.
	if gotParams.IsRead == nil || *gotParams.IsRead != true {
		t.Errorf("IsRead diteruskan salah: %v", gotParams.IsRead)
	}
	if gotParams.Query != "budi" {
		t.Errorf("Query diteruskan = %q, mau budi", gotParams.Query)
	}
}

// TestAdminContactGetByIDFound memastikan GET detail 200 dengan {data:{...}}.
func TestAdminContactGetByIDFound(t *testing.T) {
	id := uuid.New()
	repo := &fakeContactRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.ContactMessage, error) {
			return &models.ContactMessage{ID: id, Name: "Budi", Email: "budi@example.com", CreatedAt: time.Now()}, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/contact-messages/:id", handlers.NewAdminContactHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/contact-messages/"+id.String(), nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["email"] != "budi@example.com" {
		t.Errorf("data.email = %v", data["email"])
	}
}

// TestAdminContactGetByIDInvalidUUID memastikan ID non-UUID membalas 400.
func TestAdminContactGetByIDInvalidUUID(t *testing.T) {
	repo := &fakeContactRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.ContactMessage, error) {
			t.Fatal("GetByID tidak boleh dipanggil untuk UUID invalid")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/contact-messages/:id", handlers.NewAdminContactHandler(repo).GetByID)

	status, _ := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/contact-messages/bukan-uuid", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}

// TestAdminContactGetByIDNotFound memastikan eror repo dipetakan ke 404.
func TestAdminContactGetByIDNotFound(t *testing.T) {
	repo := &fakeContactRepo{
		getByIDFn: func(_ context.Context, _ string) (*models.ContactMessage, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Get("/admin/contact-messages/:id", handlers.NewAdminContactHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/contact-messages/"+uuid.New().String(), nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("error.code = %v, mau NOT_FOUND", errObj["code"])
	}
}

// TestAdminContactMarkReadValid memastikan PATCH read valid membalas 204 dan
// meneruskan flag isRead ke repo.
func TestAdminContactMarkReadValid(t *testing.T) {
	id := uuid.New()
	var gotIsRead bool
	called := false
	repo := &fakeContactRepo{
		markReadFn: func(_ context.Context, _ string, isRead bool) error {
			called = true
			gotIsRead = isRead
			return nil
		},
	}

	app := newTestApp()
	app.Patch("/admin/contact-messages/:id/read", handlers.NewAdminContactHandler(repo).MarkRead)

	status, body := doJSON(t, app, postJSON(http.MethodPatch, "/admin/contact-messages/"+id.String()+"/read", `{"isRead":true}`))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204; body=%s", status, string(body))
	}
	if !called || !gotIsRead {
		t.Errorf("MarkRead(called=%v, isRead=%v), mau true/true", called, gotIsRead)
	}
}

// TestAdminContactMarkReadInvalidUUID memastikan ID non-UUID membalas 400.
func TestAdminContactMarkReadInvalidUUID(t *testing.T) {
	repo := &fakeContactRepo{
		markReadFn: func(_ context.Context, _ string, _ bool) error {
			t.Fatal("MarkRead tidak boleh dipanggil untuk UUID invalid")
			return nil
		},
	}

	app := newTestApp()
	app.Patch("/admin/contact-messages/:id/read", handlers.NewAdminContactHandler(repo).MarkRead)

	status, _ := doJSON(t, app, postJSON(http.MethodPatch, "/admin/contact-messages/bukan-uuid/read", `{"isRead":true}`))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}

// TestAdminContactDeleteValid memastikan DELETE valid membalas 204 tanpa body.
func TestAdminContactDeleteValid(t *testing.T) {
	id := uuid.New()
	deleted := false
	repo := &fakeContactRepo{
		deleteFn: func(_ context.Context, gotID string) error {
			deleted = true
			if gotID != id.String() {
				t.Errorf("id delete = %q, mau %q", gotID, id.String())
			}
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/contact-messages/:id", handlers.NewAdminContactHandler(repo).Delete)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/contact-messages/"+id.String(), nil))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204; body=%s", status, string(body))
	}
	if !deleted {
		t.Error("repo.Delete tidak dipanggil")
	}
	if len(body) != 0 {
		t.Errorf("204 tidak boleh punya body: %s", string(body))
	}
}

// TestAdminContactDeleteInvalidUUID memastikan ID non-UUID membalas 400.
func TestAdminContactDeleteInvalidUUID(t *testing.T) {
	repo := &fakeContactRepo{
		deleteFn: func(_ context.Context, _ string) error {
			t.Fatal("Delete tidak boleh dipanggil untuk UUID invalid")
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/contact-messages/:id", handlers.NewAdminContactHandler(repo).Delete)

	status, _ := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/contact-messages/bukan-uuid", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}
