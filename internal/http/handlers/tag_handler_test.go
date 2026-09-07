package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/google/uuid"
)

func TestPublicTagListSukses(t *testing.T) {
	var gotParams repository.TagListParams
	repo := &fakeTagRepo{
		listFn: func(_ context.Context, p repository.TagListParams) ([]models.Tag, int64, error) {
			gotParams = p
			return []models.Tag{
				{ID: uuid.New(), Name: "Go", Type: "backend"},
				{ID: uuid.New(), Name: "PostgreSQL", Type: "database"},
			}, 2, nil
		},
	}

	app := newTestApp()
	app.Get("/tags", handlers.NewTagHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/tags", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200. body: %s", status, body)
	}

	m := decode(t, body)
	data, ok := m["data"].([]any)
	if !ok {
		t.Fatalf("envelope data bukan array: %s", body)
	}
	if len(data) != 2 {
		t.Fatalf("jumlah tag = %d, mau 2", len(data))
	}

	first, _ := data[0].(map[string]any)
	if first["name"] != "Go" || first["type"] != "backend" {
		t.Errorf("tag pertama = %v, mau Go/backend", first)
	}

	// Endpoint publik tidak boleh memakai default paginasi kecil yang membuat
	// sebagian keahlian hilang diam-diam dari halaman About.
	if gotParams.Limit < 100 {
		t.Errorf("Limit = %d, terlalu kecil untuk daftar keahlian publik", gotParams.Limit)
	}
}

// Daftar kosong tetap harus berupa array, bukan null — supaya klien bisa
// langsung memetakannya tanpa penjagaan tambahan.
func TestPublicTagListKosong(t *testing.T) {
	repo := &fakeTagRepo{
		listFn: func(context.Context, repository.TagListParams) ([]models.Tag, int64, error) {
			return nil, 0, nil
		},
	}

	app := newTestApp()
	app.Get("/tags", handlers.NewTagHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/tags", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200", status)
	}

	m := decode(t, body)
	if _, ok := m["data"].([]any); !ok {
		t.Fatalf("data bukan array pada daftar kosong: %s", body)
	}
}

func TestPublicTagListError(t *testing.T) {
	repo := &fakeTagRepo{
		listFn: func(context.Context, repository.TagListParams) ([]models.Tag, int64, error) {
			return nil, 0, errors.New("db mati")
		},
	}

	app := newTestApp()
	app.Get("/tags", handlers.NewTagHandler(repo).List)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/tags", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500. body: %s", status, body)
	}
}
