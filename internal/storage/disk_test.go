package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FauzanParanditha/portfolio-backend/internal/storage"
)

// pngBytes adalah PNG 1x1 yang valid — cukup untuk membuat http.DetectContentType
// mengenalinya sebagai image/png.
var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
	0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

func newDisk(t *testing.T, maxBytes int64) (*storage.Disk, string) {
	t.Helper()
	dir := t.TempDir()
	d, err := storage.NewDisk(dir, "https://api.example.com", maxBytes)
	if err != nil {
		t.Fatalf("NewDisk error: %v", err)
	}
	return d, dir
}

func TestSavePNG(t *testing.T) {
	d, dir := newDisk(t, 1<<20)

	saved, err := d.Save(pngBytes, "foto.png")
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	if saved.ContentType != "image/png" {
		t.Errorf("ContentType = %q, mau image/png", saved.ContentType)
	}
	if saved.Size != int64(len(pngBytes)) {
		t.Errorf("Size = %d, mau %d", saved.Size, len(pngBytes))
	}
	if !strings.HasPrefix(saved.URL, "https://api.example.com/uploads/") {
		t.Errorf("URL = %q, mau berawalan base publik + /uploads/", saved.URL)
	}
	if _, err := os.Stat(filepath.Join(dir, saved.Filename)); err != nil {
		t.Errorf("berkas tidak ada di disk: %v", err)
	}
}

// Nama berkas HARUS dibuat server. Nama kiriman klien tidak boleh ikut
// menentukan path — kalau ikut, "../" bisa menulis di luar folder unggahan.
func TestSaveMengabaikanNamaKiriman(t *testing.T) {
	d, dir := newDisk(t, 1<<20)

	saved, err := d.Save(pngBytes, "../../../etc/passwd.png")
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	if strings.Contains(saved.Filename, "..") || strings.ContainsAny(saved.Filename, `/\`) {
		t.Fatalf("Filename %q memuat komponen path", saved.Filename)
	}
	if !strings.HasSuffix(saved.Filename, ".png") {
		t.Errorf("Filename %q tidak berakhiran .png", saved.Filename)
	}

	// Berkas harus mendarat DI DALAM folder unggahan.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != saved.Filename {
		t.Fatalf("isi folder = %v, mau tepat satu berkas bernama %s", entries, saved.Filename)
	}
}

// Ekstensi ditentukan dari ISI berkas, bukan dari nama kiriman. Berkas PNG yang
// diberi nama .pdf tetap tersimpan sebagai .png.
func TestSaveEkstensiDariIsiBukanNama(t *testing.T) {
	d, _ := newDisk(t, 1<<20)

	saved, err := d.Save(pngBytes, "menyesatkan.pdf")
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if !strings.HasSuffix(saved.Filename, ".png") {
		t.Errorf("Filename = %q, mau berakhiran .png sesuai isinya", saved.Filename)
	}
}

// SVG bisa memuat <script> dan dilayani dari domain yang sama dengan API,
// jadi harus ditolak meski ia "gambar".
func TestSaveMenolakSVGdanHTML(t *testing.T) {
	d, dir := newDisk(t, 1<<20)

	cases := map[string][]byte{
		"svg":  []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html": []byte("<!DOCTYPE html><html><body><script>alert(1)</script></body></html>"),
	}

	for nama, isi := range cases {
		t.Run(nama, func(t *testing.T) {
			_, err := d.Save(isi, "payload."+nama)
			if !errors.Is(err, storage.ErrTypeNotAllowed) {
				t.Fatalf("err = %v, mau ErrTypeNotAllowed", err)
			}
		})
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("ada %d berkas tertulis padahal semuanya ditolak", len(entries))
	}
}

func TestSaveMenolakTerlaluBesar(t *testing.T) {
	d, dir := newDisk(t, 32) // batas sengaja lebih kecil dari PNG uji

	_, err := d.Save(pngBytes, "besar.png")
	if !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("err = %v, mau ErrTooLarge", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("berkas tertulis padahal melebihi batas")
	}
}

func TestSaveMenolakBerkasKosong(t *testing.T) {
	d, _ := newDisk(t, 1<<20)

	if _, err := d.Save(nil, "kosong.png"); err == nil {
		t.Fatal("berkas kosong seharusnya ditolak")
	}
}

// Dua unggahan dengan isi identik tidak boleh saling menimpa.
func TestSaveNamaSelaluUnik(t *testing.T) {
	d, _ := newDisk(t, 1<<20)

	a, err := d.Save(pngBytes, "sama.png")
	if err != nil {
		t.Fatalf("Save pertama error: %v", err)
	}
	b, err := d.Save(pngBytes, "sama.png")
	if err != nil {
		t.Fatalf("Save kedua error: %v", err)
	}
	if a.Filename == b.Filename {
		t.Fatalf("dua unggahan memakai nama sama (%s) — yang lama tertimpa", a.Filename)
	}
}
