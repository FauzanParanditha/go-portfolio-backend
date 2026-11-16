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

// invalidIPRateLimitKey adalah bucket tetap untuk request yang IP-nya tidak
// bisa dinormalkan sama sekali (baik c.IP() maupun alamat koneksi). Semua
// request semacam itu berbagi SATU jatah, sehingga variasi string tidak pernah
// menghasilkan jatah baru.
const invalidIPRateLimitKey = "invalid"

// rateLimitKey adalah KeyGenerator bersama untuk SEMUA rate limiter (login,
// forgot/reset password, contact). Dasarnya c.IP(), jadi otomatis mengikuti
// konfigurasi TRUSTED_PROXIES; lihat rateLimitKeyFrom untuk fallback-nya.
func rateLimitKey(c *fiber.Ctx) string {
	return rateLimitKeyFrom(c.IP(), c.Context().RemoteIP())
}

// rateLimitKeyFrom memilih kunci limiter dari IP klien (c.IP()) dengan
// fallback ke alamat koneksi.
//
// c.IP() bisa berisi string yang lolos validasi Fiber (utils.IsIPv6) tetapi
// ditolak net.ParseIP, mis. grup hex dengan nol berlebih "2001:db8:1:2::00001".
// String mentah seperti itu TIDAK boleh menjadi kunci: tiap variasi padding
// akan mendapat jatah baru dan limiter bisa diakali dari header proxy. Jadi:
//  1. c.IP() valid → kunci ternormalisasi dari c.IP().
//  2. Tidak valid → kunci ternormalisasi dari alamat koneksi (remote).
//  3. Keduanya gagal → satu bucket tetap invalidIPRateLimitKey.
func rateLimitKeyFrom(clientIP string, remote net.IP) string {
	if key, ok := rateLimitKeyForNetIP(net.ParseIP(clientIP)); ok {
		return key
	}
	if key, ok := rateLimitKeyForNetIP(remote); ok {
		return key
	}
	return invalidIPRateLimitKey
}

// rateLimitKeyForIP menormalkan IP dalam bentuk string menjadi kunci limiter;
// ok=false bila string bukan IP valid menurut net.ParseIP.
func rateLimitKeyForIP(raw string) (string, bool) {
	return rateLimitKeyForNetIP(net.ParseIP(raw))
}

// rateLimitKeyForNetIP menormalkan IP menjadi kunci limiter:
//   - IPv4 (termasuk bentuk IPv4-mapped "::ffff:a.b.c.d") → alamat IPv4 apa
//     adanya, sehingga kedua penulisan berbagi satu jatah.
//   - IPv6 → prefix /64, mis. "2001:db8:1:2::/64".
//   - nil atau panjang tidak sah → ok=false (pemanggil memilih fallback).
func rateLimitKeyForNetIP(ip net.IP) (string, bool) {
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), true
	}
	if len(ip) != net.IPv6len {
		return "", false
	}
	return ip.Mask(ipv6RateLimitMask).String() + "/64", true
}

// NewRateLimiter membuat limiter in-memory per klien (lihat rateLimitKey).
// Saat jatah habis, galat 429 dikembalikan lewat fiber.NewError agar dirender
// oleh error handler pusat dengan envelope standar.
//
// Pakai helper ini untuk SETIAP limiter baru, supaya tidak ada limiter yang
// lupa memasang KeyGenerator dan kembali mengunci per alamat IPv6 penuh.
func NewRateLimiter(limit int, expiration time.Duration, message string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          limit,
		Expiration:   expiration,
		KeyGenerator: rateLimitKey,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, message)
		},
	})
}
