package response

import "github.com/gofiber/fiber/v2"

func OK(c *fiber.Ctx, data any) error {
	return c.JSON(fiber.Map{
		"data": data,
	})
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": data,
	})
}

// TotalPages menghitung jumlah halaman = ceil(total/limit).
// Guard: jika limit <= 0 kembalikan 0 untuk mencegah pembagian nol.
func TotalPages(total int64, limit int) int64 {
	if limit <= 0 {
		return 0
	}
	return (total + int64(limit) - 1) / int64(limit)
}
