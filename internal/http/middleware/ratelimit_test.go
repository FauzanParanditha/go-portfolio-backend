package middleware

import (
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Kunci limiter: IPv4 apa adanya, IPv6 dinormalkan ke prefix /64 supaya satu
// pelanggan tidak bisa memutar alamat di dalam subnet-nya untuk mendapat jatah
// baru.
func TestRateLimitKeyForIP(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		// IPv4 tidak berubah.
		{"203.0.113.10", "203.0.113.10", true},
		{"10.0.0.1", "10.0.0.1", true},
		{"0.0.0.0", "0.0.0.0", true},
		// Bentuk IPv4-mapped berbagi kunci dengan bentuk IPv4-nya.
		{"::ffff:203.0.113.10", "203.0.113.10", true},

		// IPv6 → /64.
		{"2001:db8:1:2::a", "2001:db8:1:2::/64", true},
		{"2001:db8:1:2:ffff:ffff:ffff:ffff", "2001:db8:1:2::/64", true},
		{"2001:DB8:1:2:0:0:0:1", "2001:db8:1:2::/64", true},
		{"2001:db8:1:3::a", "2001:db8:1:3::/64", true},
		{"::1", "::/64", true},

		// Bukan IP menurut net.ParseIP: ok=false (tidak panic, tidak
		// mengembalikan string mentah).
		{"bukan-ip", "", false},
		{"", "", false},
		{"2001:db8:1:2::00001", "", false}, // lolos utils.IsIPv6 Fiber
	}

	for _, tc := range cases {
		got, ok := rateLimitKeyForIP(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("rateLimitKeyForIP(%q) = (%q, %v), mau (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestRateLimitKeySatuSlash64BerbagiKunci(t *testing.T) {
	a, _ := rateLimitKeyForIP("2001:db8:aa:bb:1::1")
	b, _ := rateLimitKeyForIP("2001:db8:aa:bb:dead:beef:cafe:42")
	if a != b {
		t.Errorf("dua alamat dalam /64 yang sama harus berbagi kunci: %q vs %q", a, b)
	}
	if c, _ := rateLimitKeyForIP("2001:db8:aa:bc::1"); c == a {
		t.Errorf("/64 berbeda tidak boleh berbagi kunci: %q", c)
	}
}

// IP klien yang tidak bisa di-parse jatuh ke alamat koneksi, lalu ke bucket
// tetap — tidak pernah ke string mentah.
func TestRateLimitKeyFromFallback(t *testing.T) {
	remote4 := net.ParseIP("198.51.100.7")
	remote6 := net.ParseIP("2001:db8:9:9::1")

	cases := []struct {
		name     string
		clientIP string
		remote   net.IP
		want     string
	}{
		{"IP klien valid dipakai", "203.0.113.10", remote4, "203.0.113.10"},
		{"IP klien IPv6 valid → /64", "2001:db8:1:2::a", remote4, "2001:db8:1:2::/64"},
		{"padding → remote IPv4", "2001:db8:1:2::00001", remote4, "198.51.100.7"},
		{"padding → remote IPv6 /64", "2001:db8:1:2::00001", remote6, "2001:db8:9:9::/64"},
		{"sampah → remote", "bukan-ip", remote4, "198.51.100.7"},
		{"keduanya gagal → bucket tetap", "2001:db8:1:2::00001", nil, invalidIPRateLimitKey},
		{"remote panjang tak sah → bucket tetap", "", net.IP{1, 2, 3}, invalidIPRateLimitKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rateLimitKeyFrom(tc.clientIP, tc.remote); got != tc.want {
				t.Errorf("rateLimitKeyFrom(%q, %v) = %q, mau %q", tc.clientIP, tc.remote, got, tc.want)
			}
		})
	}
}

// Regresi N2: Fiber (EnableIPValidation) meloloskan grup hex dengan nol
// berlebih, sehingga dulu tiap variasi padding mendapat jatah baru. Sekarang
// semua variasi harus berbagi satu jatah.
func TestRateLimiterVariasiPaddingBerbagiJatah(t *testing.T) {
	const limit = 5

	// Konfigurasi sama dengan applyProxyConfig di router. Remote addr koneksi
	// app.Test selalu 0.0.0.0, jadi itulah "proxy" tepercayanya.
	app := fiber.New(fiber.Config{
		ProxyHeader:             "X-Real-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"0.0.0.0"},
		EnableIPValidation:      true,
	})
	app.Get("/", NewRateLimiter(limit, time.Minute, "terlalu banyak request"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	// Header ini lolos validasi Fiber tetapi ditolak net.ParseIP.
	variasi := []string{
		"2001:db8:1:2::00001",
		"2001:db8:1:2::000001",
		"2001:db8:1:2::0000001",
		"2001:db8:1:2::00000001",
		"2001:db8:1:2::00002",
		"2001:db8:1:2::00003",
		"02001:db8:1:2::1",
		"002001:db8:1:2::1",
		"2001:0db80:1:2::1",
		"2001:db8:00001:2::1",
		"2001:db8:1:00002::1",
		"2001:db8:1:2::0000a",
		"2001:db8:1:2::0000b",
		"2001:db8:1:2::0000c",
		"2001:db8:1:2::0000d",
		"2001:db8:1:2::0000e",
		"2001:db8:1:2::0000f",
		"2001:db8:1:2::00010",
		"2001:db8:1:2::00011",
		"2001:db8:1:2::00012",
	}

	lolos := 0
	for _, v := range variasi {
		req := httptest.NewRequest(fiber.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", v)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		switch resp.StatusCode {
		case fiber.StatusOK:
			lolos++
		case fiber.StatusTooManyRequests:
		default:
			t.Fatalf("status tak terduga %d untuk %q", resp.StatusCode, v)
		}
	}

	if lolos != limit {
		t.Fatalf("variasi padding harus berbagi satu jatah: %d/%d lolos, mau %d", lolos, len(variasi), limit)
	}
}

// Kontrol: IP valid yang berbeda /64 tetap mendapat jatah masing-masing,
// memastikan test di atas tidak lulus hanya karena header diabaikan.
func TestRateLimiterIPValidBedaJatah(t *testing.T) {
	app := fiber.New(fiber.Config{
		ProxyHeader:             "X-Real-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"0.0.0.0"},
		EnableIPValidation:      true,
	})
	app.Get("/", NewRateLimiter(1, time.Minute, "terlalu banyak request"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	for _, ip := range []string{"2001:db8:1:1::1", "2001:db8:1:2::1", "203.0.113.10"} {
		req := httptest.NewRequest(fiber.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", ip)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("IP %q seharusnya punya jatah sendiri, dapat %d", ip, resp.StatusCode)
		}
	}
}
