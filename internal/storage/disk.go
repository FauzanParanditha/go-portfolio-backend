package storage

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// allowedTypes memetakan MIME type yang diizinkan ke ekstensi berkasnya.
//
// Daftar ini sengaja pendek dan TIDAK memuat SVG maupun HTML: keduanya bisa
// memuat <script>, dan berkas ini dilayani dari domain yang sama dengan API —
// mengunggah SVG berisi skrip sama saja dengan membuka celah XSS.
var allowedTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
}

// Disk menyimpan berkas ke folder lokal dan melayaninya lewat URL statis.
//
// Konsekuensi yang perlu disadari: berkas ikut hilang bila container dibuat
// ulang tanpa volume yang dipasang. Untuk deployment yang bisa di-recreate,
// pasang folder ini sebagai volume persisten.
type Disk struct {
	dir       string
	publicURL string
	maxBytes  int64
}

// NewDisk membuat penyimpanan disk. Folder dibuat bila belum ada.
func NewDisk(dir, publicBaseURL string, maxBytes int64) (*Disk, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("gagal menyiapkan folder unggahan %q: %w", dir, err)
	}

	return &Disk{
		dir:       dir,
		publicURL: strings.TrimRight(publicBaseURL, "/"),
		maxBytes:  maxBytes,
	}, nil
}

func (d *Disk) Save(data []byte, originalName string) (*SavedFile, error) {
	size := int64(len(data))
	if size == 0 {
		return nil, fmt.Errorf("berkas kosong")
	}
	if size > d.maxBytes {
		return nil, ErrTooLarge
	}

	// Jenis berkas ditentukan dari ISI-nya, bukan dari header Content-Type
	// kiriman klien maupun ekstensi nama aslinya — keduanya gampang dipalsukan.
	sniffLen := 512
	if len(data) < sniffLen {
		sniffLen = len(data)
	}
	contentType := http.DetectContentType(data[:sniffLen])
	// DetectContentType bisa mengembalikan "image/png; charset=..." pada
	// beberapa kasus; ambil bagian tipenya saja.
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}

	ext, ok := allowedTypes[contentType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTypeNotAllowed, contentType)
	}

	// Nama berkas dibuat server: UUID + ekstensi hasil deteksi. Nama asli dari
	// klien tidak pernah dipakai untuk membentuk path, sehingga "../" atau
	// nama aneh lain tidak bisa keluar dari folder unggahan.
	filename := uuid.NewString() + ext
	full := filepath.Join(d.dir, filename)

	if err := os.WriteFile(full, data, 0o644); err != nil {
		return nil, fmt.Errorf("gagal menulis berkas: %w", err)
	}

	return &SavedFile{
		Filename:    filename,
		URL:         d.publicURL + path.Join("/uploads", filename),
		ContentType: contentType,
		Size:        size,
	}, nil
}
