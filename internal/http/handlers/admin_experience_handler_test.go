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

// TestAdminExperienceListReturnsDataAndMeta memastikan ListAdmin membalas 200
// dengan {data,meta} dan meneruskan q/page/limit ke repo.
func TestAdminExperienceListReturnsDataAndMeta(t *testing.T) {
	var gotParams repository.ExperienceAdminListParams
	repo := &fakeExperienceRepo{
		listAdminFn: func(_ context.Context, params repository.ExperienceAdminListParams) ([]models.Experience, int64, error) {
			gotParams = params
			return []models.Experience{
				{ID: uuid.New(), Title: "Engineer", Company: "PANDI", StartDate: time.Now()},
			}, 1, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/experiences", handlers.NewAdminExperienceHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/experiences?q=eng&page=2&limit=7", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	env := decode(t, body)
	if data, ok := env["data"].([]any); !ok || len(data) != 1 {
		t.Fatalf("data tidak sesuai: %v", env["data"])
	}
	if _, ok := env["meta"].(map[string]any); !ok {
		t.Fatalf("meta hilang: %v", env["meta"])
	}
	if gotParams.Query != "eng" || gotParams.Page != 2 || gotParams.Limit != 7 {
		t.Errorf("params diteruskan salah: %+v", gotParams)
	}
}

// TestAdminExperienceGetByIDFound memastikan GetByID membalas 200 + {data}.
func TestAdminExperienceGetByIDFound(t *testing.T) {
	id := uuid.New()
	repo := &fakeExperienceRepo{
		getByIDAdminFn: func(_ context.Context, _ uuid.UUID) (*models.Experience, error) {
			return &models.Experience{ID: id, Title: "Engineer", Company: "PANDI", StartDate: time.Now()}, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/experiences/:id", handlers.NewAdminExperienceHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/experiences/"+id.String(), nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}
	if decode(t, body)["data"].(map[string]any)["id"] != id.String() {
		t.Errorf("data.id tidak sesuai")
	}
}

// TestAdminExperienceGetByIDNotFound memastikan gorm.ErrRecordNotFound → 404.
func TestAdminExperienceGetByIDNotFound(t *testing.T) {
	repo := &fakeExperienceRepo{
		getByIDAdminFn: func(_ context.Context, _ uuid.UUID) (*models.Experience, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Get("/admin/experiences/:id", handlers.NewAdminExperienceHandler(repo).GetByID)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/experiences/"+uuid.New().String(), nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(body))
	}
	if decode(t, body)["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("error.code mau NOT_FOUND")
	}
}

// TestAdminExperienceCreateSuccess memastikan Create membalas 201 + {data} dan
// meneruskan TagIDs, Highlights, serta StartDate ter-parse ke repo.
func TestAdminExperienceCreateSuccess(t *testing.T) {
	tagID := uuid.New()
	var gotIn repository.ExperienceWriteInput
	repo := &fakeExperienceRepo{
		createFn: func(_ context.Context, in repository.ExperienceWriteInput) (*models.Experience, error) {
			gotIn = in
			return &models.Experience{ID: uuid.New(), Title: in.Experience.Title, Company: in.Experience.Company, StartDate: in.Experience.StartDate}, nil
		},
	}

	app := newTestApp()
	app.Post("/admin/experiences", handlers.NewAdminExperienceHandler(repo).Create)

	body := `{
		"title":"Engineer","company":"PANDI","startDate":"2023-01-15",
		"tagIds":["` + tagID.String() + `"],"highlights":["memimpin tim","rilis fitur"]
	}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPost, "/admin/experiences", body))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, mau 201; body=%s", status, string(respBody))
	}
	if len(gotIn.TagIDs) != 1 || gotIn.TagIDs[0] != tagID {
		t.Errorf("TagIDs diteruskan salah: %v", gotIn.TagIDs)
	}
	if len(gotIn.Highlights) != 2 || gotIn.Highlights[0] != "memimpin tim" {
		t.Errorf("Highlights diteruskan salah: %v", gotIn.Highlights)
	}
	if gotIn.Experience.StartDate.Format("2006-01-02") != "2023-01-15" {
		t.Errorf("StartDate ter-parse salah: %v", gotIn.Experience.StartDate)
	}
}

// TestAdminExperienceCreateValidationFails memastikan field wajib kosong → 422.
func TestAdminExperienceCreateValidationFails(t *testing.T) {
	repo := &fakeExperienceRepo{
		createFn: func(_ context.Context, _ repository.ExperienceWriteInput) (*models.Experience, error) {
			t.Fatal("repo tidak boleh dipanggil saat validasi gagal")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Post("/admin/experiences", handlers.NewAdminExperienceHandler(repo).Create)

	// title/company/startDate kosong → gagal required.
	status, body := doJSON(t, app, jsonReq(http.MethodPost, "/admin/experiences", `{"title":""}`))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, mau 422; body=%s", status, string(body))
	}
	if decode(t, body)["error"].(map[string]any)["code"] != "VALIDATION_ERROR" {
		t.Errorf("error.code mau VALIDATION_ERROR")
	}
}

// TestAdminExperienceCreateInvalidDate memastikan startDate format salah → 400.
func TestAdminExperienceCreateInvalidDate(t *testing.T) {
	repo := &fakeExperienceRepo{
		createFn: func(_ context.Context, _ repository.ExperienceWriteInput) (*models.Experience, error) {
			t.Fatal("repo tidak boleh dipanggil saat tanggal invalid")
			return nil, nil
		},
	}

	app := newTestApp()
	app.Post("/admin/experiences", handlers.NewAdminExperienceHandler(repo).Create)

	body := `{"title":"E","company":"C","startDate":"15-01-2023"}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPost, "/admin/experiences", body))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400; body=%s", status, string(respBody))
	}
}

// TestAdminExperienceUpdateNotFound memastikan gorm.ErrRecordNotFound → 404.
func TestAdminExperienceUpdateNotFound(t *testing.T) {
	repo := &fakeExperienceRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, _ repository.ExperienceWriteInput) (*models.Experience, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	app := newTestApp()
	app.Put("/admin/experiences/:id", handlers.NewAdminExperienceHandler(repo).Update)

	body := `{"title":"E","company":"C","startDate":"2023-01-15"}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPut, "/admin/experiences/"+uuid.New().String(), body))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404; body=%s", status, string(respBody))
	}
	if decode(t, respBody)["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("error.code mau NOT_FOUND")
	}
}

// TestAdminExperienceUpdateSuccess memastikan Update membalas 200 + {data}.
func TestAdminExperienceUpdateSuccess(t *testing.T) {
	id := uuid.New()
	var gotID uuid.UUID
	repo := &fakeExperienceRepo{
		updateFn: func(_ context.Context, eid uuid.UUID, in repository.ExperienceWriteInput) (*models.Experience, error) {
			gotID = eid
			return &models.Experience{ID: eid, Title: in.Experience.Title, Company: in.Experience.Company, StartDate: in.Experience.StartDate}, nil
		},
	}

	app := newTestApp()
	app.Put("/admin/experiences/:id", handlers.NewAdminExperienceHandler(repo).Update)

	body := `{"title":"Baru","company":"C","startDate":"2023-01-15"}`
	status, respBody := doJSON(t, app, jsonReq(http.MethodPut, "/admin/experiences/"+id.String(), body))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(respBody))
	}
	if gotID != id {
		t.Errorf("id diteruskan = %v, mau %v", gotID, id)
	}
}

// TestAdminExperienceDeleteSuccess memastikan Delete membalas 204.
func TestAdminExperienceDeleteSuccess(t *testing.T) {
	id := uuid.New()
	var gotID uuid.UUID
	repo := &fakeExperienceRepo{
		deleteFn: func(_ context.Context, eid uuid.UUID) error {
			gotID = eid
			return nil
		},
	}

	app := newTestApp()
	app.Delete("/admin/experiences/:id", handlers.NewAdminExperienceHandler(repo).Delete)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodDelete, "/admin/experiences/"+id.String(), nil))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, mau 204; body=%s", status, string(body))
	}
	if gotID != id {
		t.Errorf("id diteruskan = %v, mau %v", gotID, id)
	}
}
