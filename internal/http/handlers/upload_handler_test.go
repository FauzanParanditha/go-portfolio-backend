package handlers_test

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/storage"
)

// multipartRequest menyusun request multipart berisi satu berkas pada field
// `field`. Dipakai untuk menguji handler tanpa menyentuh disk.
func multipartRequest(t *testing.T, target, field, filename string, content []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("gagal menulis isi berkas uji: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gagal menutup writer multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestUploadSukses(t *testing.T) {
	store := &fakeStorage{}
	app := newTestApp()
	app.Post("/admin/uploads", handlers.NewAdminUploadHandler(store).Upload)

	isi := []byte("isi-gambar-palsu")
	req := multipartRequest(t, "/admin/uploads", "file", "foto.png", isi)

	status, body := doJSON(t, app, req)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, mau 201. body: %s", status, body)
	}

	m := decode(t, body)
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope data tidak ditemukan: %s", body)
	}
	if data["url"] != "https://api.example.com/uploads/abc.png" {
		t.Errorf("url = %v", data["url"])
	}
	if data["contentType"] != "image/png" {
		t.Errorf("contentType = %v", data["contentType"])
	}

	if store.calls != 1 {
		t.Fatalf("storage dipanggil %d kali, mau 1", store.calls)
	}
	if !bytes.Equal(store.gotData, isi) {
		t.Errorf("isi yang diteruskan ke storage tidak sama dengan yang diunggah")
	}
	if store.gotName != "foto.png" {
		t.Errorf("nama asli = %q, mau foto.png", store.gotName)
	}
}

func TestUploadTanpaBerkas(t *testing.T) {
	store := &fakeStorage{}
	app := newTestApp()
	app.Post("/admin/uploads", handlers.NewAdminUploadHandler(store).Upload)

	// Field-nya salah nama: handler harus menolak, bukan panik.
	req := multipartRequest(t, "/admin/uploads", "berkas", "foto.png", []byte("x"))

	status, body := doJSON(t, app, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body: %s", status, body)
	}
	if store.calls != 0 {
		t.Errorf("storage tidak boleh dipanggil saat berkas tidak ada")
	}
}

// Jenis berkas yang ditolak storage harus jadi 415, bukan 500 — ini kesalahan
// klien, bukan kegagalan server.
func TestUploadJenisDitolak(t *testing.T) {
	store := &fakeStorage{
		saveFn: func([]byte, string) (*storage.SavedFile, error) {
			return nil, errors.Join(storage.ErrTypeNotAllowed, errors.New("image/svg+xml"))
		},
	}
	app := newTestApp()
	app.Post("/admin/uploads", handlers.NewAdminUploadHandler(store).Upload)

	req := multipartRequest(t, "/admin/uploads", "file", "payload.svg", []byte("<svg/>"))
	status, body := doJSON(t, app, req)

	if status != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, mau 415. body: %s", status, body)
	}
}

// Berkas kebesaran harus jadi 413.
func TestUploadTerlaluBesar(t *testing.T) {
	store := &fakeStorage{
		saveFn: func([]byte, string) (*storage.SavedFile, error) {
			return nil, storage.ErrTooLarge
		},
	}
	app := newTestApp()
	app.Post("/admin/uploads", handlers.NewAdminUploadHandler(store).Upload)

	req := multipartRequest(t, "/admin/uploads", "file", "besar.png", []byte("x"))
	status, body := doJSON(t, app, req)

	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, mau 413. body: %s", status, body)
	}
}
