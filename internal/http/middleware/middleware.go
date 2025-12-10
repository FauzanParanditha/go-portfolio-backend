package middleware

import (
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog/log"
)

// RegisterGlobal mendaftarkan semua middleware level-aplikasi.
func RegisterGlobal(app *fiber.App, cfg *config.Config) {
	// Panic safety
	app.Use(recover.New())

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
		AllowOrigins: cfg.CORSAllowedOrigins,
		AllowMethods: cfg.CORSAllowedMethods,
		AllowHeaders: cfg.CORSAllowedHeaders,
	}))
}
