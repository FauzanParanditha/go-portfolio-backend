package http

import (
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// errorCodeForStatus memetakan HTTP status ke kode error yang stabil & mudah
// dibaca client. Fallback ke pola umum (4xx -> CLIENT_ERROR, 5xx -> SERVER_ERROR).
func errorCodeForStatus(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "BAD_REQUEST"
	case fiber.StatusUnauthorized:
		return "UNAUTHORIZED"
	case fiber.StatusForbidden:
		return "FORBIDDEN"
	case fiber.StatusNotFound:
		return "NOT_FOUND"
	case fiber.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case fiber.StatusConflict:
		return "CONFLICT"
	case fiber.StatusUnprocessableEntity:
		return "VALIDATION_ERROR"
	case fiber.StatusTooManyRequests:
		return "TOO_MANY_REQUESTS"
	}

	switch {
	case status >= 500:
		return "SERVER_ERROR"
	case status >= 400:
		return "CLIENT_ERROR"
	default:
		return "INTERNAL_ERROR"
	}
}

func NewErrorHandler() fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		msg := "internal server error"
		errorCode := "INTERNAL_ERROR"
		var details any = nil

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			msg = e.Message
			errorCode = errorCodeForStatus(code)
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
