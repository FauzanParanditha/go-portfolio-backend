package middleware

import (
	"net"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// ipv6RateLimitPrefix adalah panjang prefix IPv6 yang dianggap "satu klien"
// oleh rate limiter. /64 adalah ukuran subnet terkecil yang lazim diberikan ISP
// ke satu pelanggan, sehingga satu penyerang bisa memutar ~2^64 alamat di
// dalamnya. Mengunci per alamat penuh berarti limiter tidak berguna untuk IPv6.
const ipv6RateLimitPrefix = 64

var ipv6RateLimitMask = net.CIDRMask(ipv6RateLimitPrefix, 128)

// RateLimitKey adalah KeyGenerator bersama untuk SEMUA rate limiter (login,
// forgot/reset password, contact). Dasarnya c.IP(), jadi otomatis mengikuti
// konfigurasi TRUSTED_PROXIES; lihat rateLimitKeyForIP untuk normalisasinya.
func RateLimitKey(c *fiber.Ctx) string {
	return rateLimitKeyForIP(c.IP())
}

// rateLimitKeyForIP menormalkan IP menjadi kunci limiter:
//   - IPv4 (termasuk bentuk IPv4-mapped "::ffff:a.b.c.d") → alamat IPv4 apa
//     adanya, sehingga kedua penulisan berbagi satu jatah.
//   - IPv6 → prefix /64, mis. "2001:db8:1:2::/64".
//   - Bukan IP valid → string asli (tidak terjadi bila c.IP() berasal dari
//     remote addr atau EnableIPValidation aktif, tapi jangan sampai panic).
func rateLimitKeyForIP(raw string) string {
	ip := net.ParseIP(raw)
	if ip == nil {
		return raw
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(ipv6RateLimitMask).String() + "/64"
}

// NewRateLimiter membuat limiter in-memory per klien (lihat RateLimitKey).
// Saat jatah habis, galat 429 dikembalikan lewat fiber.NewError agar dirender
// oleh error handler pusat dengan envelope standar.
//
// Pakai helper ini untuk SETIAP limiter baru, supaya tidak ada limiter yang
// lupa memasang KeyGenerator dan kembali mengunci per alamat IPv6 penuh.
func NewRateLimiter(max int, expiration time.Duration, message string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   expiration,
		KeyGenerator: RateLimitKey,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, message)
		},
	})
}
