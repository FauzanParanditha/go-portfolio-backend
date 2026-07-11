package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	apphttp "github.com/FauzanParanditha/portfolio-backend/internal/http"
	"github.com/gofiber/fiber/v2"
)

// newTestApp membuat fiber app dengan ErrorHandler produksi terpasang, sehingga
// envelope error yang diuji (`{"error": {message, code, details}}`) sama persis
// dengan yang diterima client. DB-free: tidak ada koneksi database.
func newTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: apphttp.NewErrorHandler(),
	})
}

// doJSON mengeksekusi request lewat app.Test dan mengembalikan status code plus
// body mentah. Membaca & menutup body agar test ringkas.
func doJSON(t *testing.T, app *fiber.App, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("gagal baca body: %v", err)
	}
	return resp.StatusCode, body
}

// decode mengurai body JSON ke map generik untuk memeriksa bentuk envelope.
func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("gagal decode JSON (%s): %v", string(body), err)
	}
	return m
}
