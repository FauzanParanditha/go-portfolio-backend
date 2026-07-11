package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestExperienceListReturnsData memastikan GET /experiences membalas 200 dengan
// envelope {data:[...]} dan memformat tanggal ke "2006-01-02".
func TestExperienceListReturnsData(t *testing.T) {
	start := time.Date(2022, 1, 15, 0, 0, 0, 0, time.UTC)
	repo := &fakeExperienceRepo{
		listFn: func(_ context.Context) ([]models.Experience, error) {
			return []models.Experience{
				{ID: uuid.New(), Title: "Backend Engineer", Company: "PANDI", StartDate: start, IsCurrent: true},
			}, nil
		},
	}

	app := newTestApp()
	app.Get("/experiences", handlers.NewExperienceHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/experiences", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}

	data := decode(t, body)["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("jumlah data = %d, mau 1", len(data))
	}
	item := data[0].(map[string]any)
	if item["company"] != "PANDI" {
		t.Errorf("company = %v, mau PANDI", item["company"])
	}
	if item["startDate"] != "2022-01-15" {
		t.Errorf("startDate = %v, mau 2022-01-15", item["startDate"])
	}
}

// TestExperienceListEmptyReturnsEmptyArray memastikan hasil kosong tetap array
// kosong (bukan null).
func TestExperienceListEmptyReturnsEmptyArray(t *testing.T) {
	repo := &fakeExperienceRepo{
		listFn: func(_ context.Context) ([]models.Experience, error) {
			return []models.Experience{}, nil
		},
	}

	app := newTestApp()
	app.Get("/experiences", handlers.NewExperienceHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/experiences", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200", status)
	}
	data, ok := decode(t, body)["data"].([]any)
	if !ok || len(data) != 0 {
		t.Errorf("data harus array kosong, dapat %v", decode(t, body)["data"])
	}
}

// TestExperienceListRepoErrorReturns500 memastikan eror repo dipetakan ke 500.
func TestExperienceListRepoErrorReturns500(t *testing.T) {
	repo := &fakeExperienceRepo{
		listFn: func(_ context.Context) ([]models.Experience, error) {
			return nil, gorm.ErrInvalidDB
		},
	}

	app := newTestApp()
	app.Get("/experiences", handlers.NewExperienceHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/experiences", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500", status)
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "SERVER_ERROR" {
		t.Errorf("error.code = %v, mau SERVER_ERROR", errObj["code"])
	}
}
