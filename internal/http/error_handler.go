package http

import (
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func NewErrorHandler() fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		msg := "internal server error"
		errorCode := "INTERNAL_ERROR"
		var details any = nil

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			msg = e.Message
		}

		// Kalau ada validation_errors di context → masukkan ke details
		if v := c.Locals("validation_errors"); v != nil {
			if m, ok := v.(map[string]string); ok {
				details = m
				errorCode = "VALIDATION_ERROR"
			}
		}

		rid := helpers.GetRequestID(c)

		log.Error().
			Err(err).
			Int("status", code).
			Str("code", errorCode).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("request_id", rid).
			Interface("details", details).
			Msg("request error")

		return c.Status(code).JSON(fiber.Map{
			"error": fiber.Map{
				"message": msg,
				"code":    errorCode,
				"details": details,
			},
			"requestId": rid,
		})
	}
}
