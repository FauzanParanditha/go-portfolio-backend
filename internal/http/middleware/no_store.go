package middleware

import "github.com/gofiber/fiber/v2"

// NoStore memasang `Cache-Control: no-store` agar respons tidak disimpan oleh
// browser maupun proxy/CDN di depan API.
//
// Pasang SEBELUM rate limiter dan handler: header diset sebelum c.Next(),
// sehingga ikut terbawa pada respons galat yang dirender error handler pusat
// (429 dari limiter, 503 dari handler), bukan hanya pada respons sukses.
func NoStore() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Next()
	}
}
