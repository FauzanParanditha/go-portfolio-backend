package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// postJSON membuat request POST dengan body JSON + header Content-Type.
func postJSON(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestContactCreateValid memastikan POST /contact yang valid membalas 201 dengan
// envelope {data:{id, message}} dan memanggil repo.Create.
func TestContactCreateValid(t *testing.T) {
	var created *models.ContactMessage
	repo := &fakeContactRepo{
		createFn: func(_ context.Context, m *models.ContactMessage) error {
			m.ID = uuid.New() // simulasikan DB mengisi PK
			created = m
			return nil
		},
	}

	app := newTestApp()
	app.Post("/contact", handlers.NewContactHandler(repo).Create)

	payload := `{"name":"Budi","email":"budi@example.com","subject":"Halo","message":"Pesan uji"}`
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/contact", payload))

	if status != http.StatusCreated {
		t.Fatalf("status = %d, mau 201; body=%s", status, string(body))
	}
	if created == nil || created.Email != "budi@example.com" {
		t.Fatalf("repo.Create tidak dipanggil dengan data benar: %+v", created)
	}
	data := decode(t, body)["data"].(map[string]any)
	if data["message"] != "message received" {
		t.Errorf("data.message = %v, mau 'message received'", data["message"])
	}
	if data["id"] == "" || data["id"] == nil {
		t.Errorf("data.id kosong")
	}
}

// TestContactCreateValidationFails memastikan payload tak lengkap membalas 422
// dengan envelope {error:{code:VALIDATION_ERROR, details:{...}}} tanpa menyentuh repo.
func TestContactCreateValidationFails(t *testing.T) {
	repoCalled := false
	repo := &fakeContactRepo{
		createFn: func(_ context.Context, _ *models.ContactMessage) error {
			repoCalled = true
			return nil
		},
	}

	app := newTestApp()
	app.Post("/contact", handlers.NewContactHandler(repo).Create)

	// email tidak valid + subject/message kosong
	payload := `{"name":"Budi","email":"bukan-email","subject":"","message":""}`
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/contact", payload))

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
	details, ok := errObj["details"].(map[string]any)
	if !ok || len(details) == 0 {
		t.Errorf("details harus berisi field error, dapat: %v", errObj["details"])
	}
}

// TestContactCreateInvalidJSON memastikan body JSON rusak membalas 400.
func TestContactCreateInvalidJSON(t *testing.T) {
	repo := &fakeContactRepo{
		createFn: func(_ context.Context, _ *models.ContactMessage) error { return nil },
	}

	app := newTestApp()
	app.Post("/contact", handlers.NewContactHandler(repo).Create)

	status, _ := doJSON(t, app, postJSON(http.MethodPost, "/contact", `{"name":`))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", status)
	}
}

// TestContactCreateRepoErrorReturns500 memastikan eror repo dipetakan ke 500.
func TestContactCreateRepoErrorReturns500(t *testing.T) {
	repo := &fakeContactRepo{
		createFn: func(_ context.Context, _ *models.ContactMessage) error {
			return gorm.ErrInvalidDB
		},
	}

	app := newTestApp()
	app.Post("/contact", handlers.NewContactHandler(repo).Create)

	payload := `{"name":"Budi","email":"budi@example.com","subject":"Halo","message":"Pesan uji"}`
	status, body := doJSON(t, app, postJSON(http.MethodPost, "/contact", payload))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500; body=%s", status, string(body))
	}
}
