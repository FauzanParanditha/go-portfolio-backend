package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog/log"
)

// defaultMaxBodyBytes adalah batas ukuran body untuk route biasa (1 MB).
//
// `BodyLimit` di fiber.Config berlaku SATU aplikasi penuh, sehingga menaikkannya
// demi unggahan berkas otomatis melonggarkan seluruh endpoint lain. Guard ini
// mengembalikan batas ketat itu untuk semua route kecuali unggahan, jadi
// perlindungan terhadap resource exhaustion tidak ikut hilang.
const defaultMaxBodyBytes = 1 * 1024 * 1024

// uploadPathPrefix adalah satu-satunya route yang boleh melebihi batas di atas.
const uploadPathPrefix = "/api/v1/admin/uploads"

// RegisterGlobal mendaftarkan semua middleware level-aplikasi.
func RegisterGlobal(app *fiber.App, cfg *config.Config) {
	// Panic safety
	app.Use(recover.New())

	// Batas body ketat untuk route non-unggahan (lihat defaultMaxBodyBytes).
	app.Use(func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), uploadPathPrefix) {
			return c.Next()
		}
		if c.Request().Header.ContentLength() > defaultMaxBodyBytes {
			return fiber.NewError(http.StatusRequestEntityTooLarge, "request body too large")
		}
		return c.Next()
	})

	// Security headers (X-Frame-Options, X-Content-Type-Options, HSTS, dll)
	app.Use(helmet.New())

	// Request ID
	app.Use(requestid.New())

	// Custom HTTP request logger dengan zerolog
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		latency := time.Since(start)

		ev := log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("latency", latency).
			Str("ip", c.IP()).
			Str("request_id", helpers.GetRequestID(c))

		if qs := string(c.Context().QueryArgs().QueryString()); qs != "" {
			ev = ev.Str("query", qs)
		}

		ev.Msg("http request")

		return err
	})

	// CORS dari ENV
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowedOrigins,
		AllowMethods:     cfg.CORSAllowedMethods,
		AllowHeaders:     cfg.CORSAllowedHeaders,
		AllowCredentials: cfg.CORSAllowCredentials,
	}))
}
