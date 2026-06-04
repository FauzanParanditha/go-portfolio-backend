package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// RequireRole memastikan user yang sudah terautentikasi memiliki salah satu
// role yang diizinkan. Harus dipasang SETELAH AuthJWT karena membaca
// "user_role" dari Locals.
func RequireRole(allowed ...string) fiber.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		role, ok := c.Locals("user_role").(string)
		if !ok || role == "" {
			return fiber.NewError(fiber.StatusForbidden, "forbidden: missing role")
		}

		if _, allowed := allowedSet[role]; !allowed {
			return fiber.NewError(fiber.StatusForbidden, "forbidden: insufficient privileges")
		}

		return c.Next()
	}
}
