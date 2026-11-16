package middleware

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// RejectContentEncoding menolak request yang body-nya dikirim terkompresi
// (header `Content-Encoding` selain kosong atau `identity`) dengan 415
// Unsupported Media Type.
//
// Alasannya: Fiber v2 mendekompresi body secara otomatis TANPA batas ukuran.
// `c.Body()` (dipakai `BodyParser` untuk JSON/XML) memanggil
// BodyGunzip/BodyUnbrotli/BodyInflate milik fasthttp untuk setiap encoding di
// header, termasuk yang berantai (`gzip, gzip`), dan `c.FormFile`/
// `c.MultipartForm` men-gunzip body multipart lewat
// `Request.MultipartForm()` tanpa limit. Guard Content-Length 1 MB maupun
// `BodyLimit` hanya melihat ukuran yang MASIH terkompresi, sehingga ~200 KB gzip
// bisa mengembang jadi ratusan MB di memori (decompression bomb) — cukup untuk
// membuat server OOM lewat endpoint publik tanpa auth (login, contact,
// forgot-password).
//
// Tidak ada klien sah API ini yang mengirim body terkompresi (browser maupun
// fetch tidak melakukannya secara default), jadi menolak seluruhnya lebih aman
// daripada mencoba membatasi hasil dekompresi.
//
// Semua kemunculan header diperiksa (bisa lebih dari satu baris), dan setiap
// nilai dipecah per koma sesuai RFC 9110 §8.4, sehingga `identity, gzip` pun
// ditolak.
func RejectContentEncoding() fiber.Handler {
	headerName := []byte(fiber.HeaderContentEncoding)

	return func(c *fiber.Ctx) error {
		for key, value := range c.Request().Header.All() {
			if !bytes.EqualFold(key, headerName) {
				continue
			}
			if !isIdentityEncoding(string(value)) {
				return fiber.NewError(
					http.StatusUnsupportedMediaType,
					"Content-Encoding pada request tidak didukung; kirim body tanpa kompresi",
				)
			}
		}
		return c.Next()
	}
}

// isIdentityEncoding bernilai true bila nilai header Content-Encoding hanya
// berisi token kosong atau `identity` (tanpa kompresi).
func isIdentityEncoding(value string) bool {
	for token := range strings.SplitSeq(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" || strings.EqualFold(token, "identity") {
			continue
		}
		return false
	}
	return true
}
