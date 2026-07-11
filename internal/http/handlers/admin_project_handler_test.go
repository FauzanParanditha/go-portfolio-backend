package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// jsonReq membuat request POST/PUT JSON dengan Content-Type yang benar.
func jsonReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestAdminProjectListReturnsDataAndMeta memastikan ListAdmin membalas 200 dengan
// {data,meta} dan meneruskan param q/featured/page/limit ke repo.
func TestAdminProjectListReturnsDataAndMeta(t *testing.T) {
	var gotParams repository.ProjectAdminListParams
	repo := &fakeProjectRepo{
		listAdminFn: func(_ context.Context, params repository.ProjectAdminListParams) ([]models.Project, int64, error) {
			gotParams = params
			return []models.Project{
				{ID: uuid.New(), Title: "A", Slug: "a"},
			}, 1, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/projects", handlers.NewAdminProjectHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/projects?q=go&featured=true&page=3&limit=5", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	env := decode(t, body)
	if data, ok := env["data"].([]any); !ok || len(data) != 1 {
		t.Fatalf("data tidak sesuai: %v", env["data"])
	}
	meta, ok := env["meta"].(map[string]any)
	if !ok || meta["total"].(float64) != 1 {
		t.Fatalf("meta tidak sesuai: %v", env["meta"])
	}
	if gotParams.Query != "go" || !gotParams.FeaturedOnly || gotParams.Page != 3 || gotParams.Limit != 5 {
		t.Errorf("params diteruskan salah: %+v", gotParams)
	}
}

// TestAdminProjectGetByIDFound memastikan GetByID membalas 200 + {data} dan
// meneruskan UUID yang benar ke repo.
func TestAdminProjectGetByIDFound(t *testing.T) {
	id := uuid.New()
	var gotID uuid.UUID
	repo := &fakeProjectRepo{
		getByIDAdminFn: func(_ context.Context, pid uuid.UUID) (*models.Project, error) {
			gotID = pid
			return &models.Project{ID: id, Title: "A", Slug: "a"}, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/projects/"+id.String(), nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	if gotID != id {
		t.Errorf("id diteruskan = %v, mau %v", gotID, id)
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["id"] != id.String() {
		t.Errorf("data.id = %v, mau %v", data["id"], id.String())
	}
}

// TestAdminProjectGetByIDNotFound memastikan gorm.ErrRecordNotFound → 404 NOT_FOUND.
func TestAdminProjectGetByIDNotFound(t *testing.T) {
	repo := &fakeProjectRepo{
		getByIDAdminFn: func(_ context.Context, _ uuid.UUID) (*models.Project, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Get("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/projects/"+uuid.New().String(), nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(body))
	}
	if decode(t, body)["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("error.code mau NOT_FOUND")
	}
}

// TestAdminProjectGetByIDInvalidUUID memastikan id non-UUID → 400 BAD_REQUEST.
func TestAdminProjectGetByIDInvalidUUID(t *testing.T) {
	repo := &fakeProjectRepo{
		getByIDAdminFn: func(_ context.Context, _ uuid.UUID) (*models.Project, error) {
			t.Fatal("repo tidak boleh dipanggil untuk UUID invalid")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/projects/bukan-uuid", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400; body=%s", status, string(body))
	}
}

// TestAdminProjectCreateSuccess memastikan Create membalas 201 + {data} dan
// meneruskan TagIDs & Features ke repo (fake merekam input).
func TestAdminProjectCreateSuccess(t *testing.T) {
	tagID := uuid.New()
	var gotIn repository.ProjectWriteInput
	repo := &fakeProjectRepo{
		createFn: func(_ context.Context, in repository.ProjectWriteInput) (*models.Project, error) {
			gotIn = in
			return &models.Project{ID: uuid.New(), Title: in.Project.Title, Slug: in.Project.Slug}, nil
		},
	}

	app := newTestApp()
	app.Post("/admin/projects", handlers.NewAdminProjectHandler(repo).Create)

	body := `{
		"title":"Portfolio","slug":"portfolio","shortDesc":"desc","coverImageUrl":"https://x/c.png",
		"tagIds":["` + tagID.String() + `"],"features":["cepat","aman"]
	}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPost, "/admin/projects", body))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, mau 201; body=%s", status, string(respBody))
	}
	if decode(t, respBody)["data"] == nil {
		t.Errorf("data kosong di response create")
	}
	// Assert relasi diteruskan apa adanya ke repository.
	if len(gotIn.TagIDs) != 1 || gotIn.TagIDs[0] != tagID {
		t.Errorf("TagIDs diteruskan salah: %v", gotIn.TagIDs)
	}
	if len(gotIn.Features) != 2 || gotIn.Features[0] != "cepat" {
		t.Errorf("Features diteruskan salah: %v", gotIn.Features)
	}
}

// TestAdminProjectCreateValidationFails memastikan field wajib kosong → 422
// VALIDATION_ERROR tanpa memanggil repo.
func TestAdminProjectCreateValidationFails(t *testing.T) {
	repo := &fakeProjectRepo{
		createFn: func(_ context.Context, _ repository.ProjectWriteInput) (*models.Project, error) {
			t.Fatal("repo tidak boleh dipanggil saat validasi gagal")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Post("/admin/projects", handlers.NewAdminProjectHandler(repo).Create)

	// title/slug/shortDesc/coverImageUrl kosong → gagal validasi required.
	status, body := doJSON(t, app, jsonReq(http.MethodPost, "/admin/projects", `{"title":""}`))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422; body=%s", status, string(body))
	}
	if decode(t, body)["error"].(map[string]any)["code"] != "VALIDATION_ERROR" {
		t.Errorf("error.code mau VALIDATION_ERROR")
	}
}

// TestAdminProjectUpdateSuccess memastikan Update membalas 200 + {data} dengan
// UUID yang benar diteruskan.
func TestAdminProjectUpdateSuccess(t *testing.T) {
	id := uuid.New()
	var gotID uuid.UUID
	repo := &fakeProjectRepo{
		updateFn: func(_ context.Context, pid uuid.UUID, in repository.ProjectWriteInput) (*models.Project, error) {
			gotID = pid
			return &models.Project{ID: pid, Title: in.Project.Title, Slug: in.Project.Slug}, nil
		},
	}

	app := newTestApp()
	app.Put("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).Update)

	body := `{"title":"Baru","slug":"baru","shortDesc":"d","coverImageUrl":"https://x/c.png"}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPut, "/admin/projects/"+id.String(), body))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(respBody))
	}
	if gotID != id {
		t.Errorf("id diteruskan = %v, mau %v", gotID, id)
	}
}

// TestAdminProjectUpdateNotFound memastikan gorm.ErrRecordNotFound → 404.
func TestAdminProjectUpdateNotFound(t *testing.T) {
	repo := &fakeProjectRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, _ repository.ProjectWriteInput) (*models.Project, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Put("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).Update)

	body := `{"title":"x","slug":"x","shortDesc":"d","coverImageUrl":"https://x/c.png"}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPut, "/admin/projects/"+uuid.New().String(), body))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(respBody))
	}
	if decode(t, respBody)["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("error.code mau NOT_FOUND")
	}
}

// TestAdminProjectDeleteSuccess memastikan Delete membalas 204 tanpa body.
func TestAdminProjectDeleteSuccess(t *testing.T) {
	id := uuid.New()
	var gotID uuid.UUID
	repo := &fakeProjectRepo{
		deleteFn: func(_ context.Context, pid uuid.UUID) error {
			gotID = pid
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/projects/:id", handlers.NewAdminProjectHandler(repo).Delete)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/projects/"+id.String(), nil))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204; body=%s", status, string(body))
	}
	if gotID != id {
		t.Errorf("id diteruskan = %v, mau %v", gotID, id)
	}
	if len(body) != 0 {
		t.Errorf("204 seharusnya tanpa body, dapat: %s", string(body))
	}
}
