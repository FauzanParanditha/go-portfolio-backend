package middleware

import (
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/denylist"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

// AuthJWT mengembalikan middleware Fiber untuk verifikasi JWT.
// dl boleh nil (revocation dilewati); jika di-set, token yang jti-nya sudah
// dicabut (mis. setelah logout) akan ditolak 401.
func AuthJWT(cfg *config.Config, dl denylist.Denylist) fiber.Handler {
	secret := []byte(cfg.JWTSecret)

	return func(c *fiber.Ctx) error {
		// Dual-source: token diambil dari header Authorization (prioritas, untuk
		// API client & test berbasis header), ELSE dari cookie HttpOnly
		// `access_token` (dipakai browser via withCredentials).
		var tokenStr string

		authHeader := c.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid Authorization header format")
			}
			tokenStr = parts[1]
		} else if cookie := c.Cookies("access_token"); cookie != "" {
			tokenStr = cookie
		} else {
			return fiber.NewError(fiber.StatusUnauthorized, "missing credentials")
		}

		token, err := jwt.ParseWithClaims(tokenStr, &handlers.JWTCustomClaims{}, func(t *jwt.Token) (interface{}, error) {
			// pastikan method HS256
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid token signing method")
			}
			return secret, nil
		})

		if err != nil {
			log.Warn().Err(err).Msg("JWT parse error")
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		claims, ok := token.Claims.(*handlers.JWTCustomClaims)
		if !ok || !token.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token claims")
		}

		// Tolak token yang sudah dicabut (mis. setelah logout).
		if dl != nil && dl.IsRevoked(claims.ID) {
			return fiber.NewError(fiber.StatusUnauthorized, "token revoked")
		}
		// log.Info().
		// 	Interface("claims", claims).
		// 	Msg("jwt claims")

		// simpan ke context, bisa dipakai handler admin
		c.Locals("user_id", claims.UserID)
		c.Locals("user_role", claims.Role)

		return c.Next()
	}
}
