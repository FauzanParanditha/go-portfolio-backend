package helpers

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

func GetRequestID(c *fiber.Ctx) string {
	// cek dari request header
	rid := c.Get(fiber.HeaderXRequestID)
	if rid != "" {
		return rid
	}

	// cek dari response header (Fiber menambahkannya di sini)
	if b := c.Response().Header.Peek(fiber.HeaderXRequestID); b != nil {
		return string(b)
	}

	return ""
}

func GetEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func GetEnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func GetEnvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func ParseDateStr(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
