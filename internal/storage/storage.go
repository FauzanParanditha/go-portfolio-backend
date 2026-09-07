// Package storage menyimpan berkas unggahan (gambar proyek, CV) ke tempat yang
// bisa dilayani kembali lewat HTTP.
//
// Dipisahkan sebagai paket sendiri — bukan repository — karena tidak menyentuh
// database sama sekali. Polanya mengikuti `denylist` dan `mailer`: satu
// interface kecil dengan implementasi yang bisa ditukar, sehingga handler bisa
// diuji tanpa menyentuh disk.
package storage

import "errors"

// ErrTypeNotAllowed dikembalikan bila jenis berkas tidak ada dalam daftar izin.
var ErrTypeNotAllowed = errors.New("jenis berkas tidak diizinkan")

// ErrTooLarge dikembalikan bila ukuran berkas melebihi batas.
var ErrTooLarge = errors.New("ukuran berkas melebihi batas")

// SavedFile menggambarkan berkas yang berhasil disimpan.
type SavedFile struct {
	// Filename adalah nama yang BENAR-BENAR dipakai di disk. Selalu dibuat
	// server (acak + ekstensi hasil deteksi), tidak pernah dari nama kiriman
	// klien — nama dari klien bisa memuat "../" atau ekstensi menyesatkan.
	Filename string
	// URL adalah alamat publik berkas tersebut.
	URL string
	// ContentType hasil DETEKSI isi berkas, bukan header dari klien.
	ContentType string
	// Size dalam byte.
	Size int64
}

// Storage menyimpan berkas dan mengembalikan alamat publiknya.
type Storage interface {
	// Save membaca seluruh isi `data`, memvalidasi jenis dan ukurannya, lalu
	// menyimpannya. `originalName` hanya dipakai sebagai petunjuk dan TIDAK
	// dipercaya untuk menentukan nama maupun ekstensi berkas.
	Save(data []byte, originalName string) (*SavedFile, error)
}
