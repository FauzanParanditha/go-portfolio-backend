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

// TestProjectListReturnsEnvelopeWithData memastikan GET /projects membalas 200
// dengan envelope {data:[...], meta:{...}} dan meneruskan param pagination ke repo.
func TestProjectListReturnsEnvelopeWithData(t *testing.T) {
	var gotParams repository.ProjectListParams
	repo := &fakeProjectRepo{
		listFn: func(_ context.Context, params repository.ProjectListParams) ([]models.Project, int64, error) {
			gotParams = params
			return []models.Project{
				{ID: uuid.New(), Title: "Portfolio", Slug: "portfolio"},
				{ID: uuid.New(), Title: "Blog", Slug: "blog"},
			}, 2, nil
		},
	}

	app := newTestApp()
	h := handlers.NewProjectHandler(repo)
	app.Get("/projects", h.List)

	req := httptest.NewRequest(http.MethodGet, "/projects?page=2&limit=5&q=go&featured=true", nil)
	status, body := doJSON(t, app, req)

	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}

	env := decode(t, body)
	data, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("field data bukan array: %v", env["data"])
	}
	if len(data) != 2 {
		t.Fatalf("jumlah data = %d, mau 2", len(data))
	}

	meta, ok := env["meta"].(map[string]any)
	if !ok {
		t.Fatalf("field meta hilang/bukan objek: %v", env["meta"])
	}
	if meta["total"].(float64) != 2 {
		t.Errorf("meta.total = %v, mau 2", meta["total"])
	}

	// Handler harus meneruskan query ke repo dengan benar.
	if gotParams.Page != 2 || gotParams.Limit != 5 || gotParams.Query != "go" || !gotParams.FeaturedOnly {
		t.Errorf("params diteruskan salah: %+v", gotParams)
	}
}

// TestProjectListEmptyReturnsEmptyArray memastikan list kosong tetap 200 dengan
// data berupa array kosong (bukan null), penting untuk klien.
func TestProjectListEmptyReturnsEmptyArray(t *testing.T) {
	repo := &fakeProjectRepo{
		listFn: func(_ context.Context, _ repository.ProjectListParams) ([]models.Project, int64, error) {
			return []models.Project{}, 0, nil
		},
	}

	app := newTestApp()
	app.Get("/projects", handlers.NewProjectHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/projects", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200", status)
	}
	env := decode(t, body)
	data, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data bukan array: %v", env["data"])
	}
	if len(data) != 0 {
		t.Errorf("data harus kosong, dapat %d item", len(data))
	}
}

// TestProjectListRepoErrorReturns500 memastikan eror dari repo dipetakan ke 500
// dengan envelope error {error:{code:SERVER_ERROR}}.
func TestProjectListRepoErrorReturns500(t *testing.T) {
	repo := &fakeProjectRepo{
		listFn: func(_ context.Context, _ repository.ProjectListParams) ([]models.Project, int64, error) {
			return nil, 0, gorm.ErrInvalidDB
		},
	}

	app := newTestApp()
	app.Get("/projects", handlers.NewProjectHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/projects", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "SERVER_ERROR" {
		t.Errorf("error.code = %v, mau SERVER_ERROR", errObj["code"])
	}
}

// TestProjectDetailBySlugFound memastikan detail 200 dengan envelope {data:{...}}
// dan slug yang benar diteruskan ke repo.
func TestProjectDetailBySlugFound(t *testing.T) {
	var gotSlug string
	repo := &fakeProjectRepo{
		getBySlugFn: func(_ context.Context, slug string) (*models.Project, error) {
			gotSlug = slug
			return &models.Project{ID: uuid.New(), Title: "Portfolio", Slug: slug}, nil
		},
	}

	app := newTestApp()
	app.Get("/projects/:slug", handlers.NewProjectHandler(repo).DetailBySlug)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/projects/portfolio", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	if gotSlug != "portfolio" {
		t.Errorf("slug diteruskan = %q, mau portfolio", gotSlug)
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["slug"] != "portfolio" {
		t.Errorf("data.slug = %v, mau portfolio", data["slug"])
	}
}

// TestProjectDetailBySlugNotFound memastikan gorm.ErrRecordNotFound dipetakan
// ke 404 NOT_FOUND (handler mencocokkan pesan "record not found").
func TestProjectDetailBySlugNotFound(t *testing.T) {
	repo := &fakeProjectRepo{
		getBySlugFn: func(_ context.Context, _ string) (*models.Project, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Get("/projects/:slug", handlers.NewProjectHandler(repo).DetailBySlug)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/projects/tidak-ada", nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("error.code = %v, mau NOT_FOUND", errObj["code"])
	}
}
