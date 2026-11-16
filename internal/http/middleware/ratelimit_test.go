package middleware

import "testing"

// Kunci limiter: IPv4 apa adanya, IPv6 dinormalkan ke prefix /64 supaya satu
// pelanggan tidak bisa memutar alamat di dalam subnet-nya untuk mendapat jatah
// baru.
func TestRateLimitKeyForIP(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// IPv4 tidak berubah.
		{"203.0.113.10", "203.0.113.10"},
		{"10.0.0.1", "10.0.0.1"},
		{"0.0.0.0", "0.0.0.0"},
		// Bentuk IPv4-mapped berbagi kunci dengan bentuk IPv4-nya.
		{"::ffff:203.0.113.10", "203.0.113.10"},

		// IPv6 → /64.
		{"2001:db8:1:2::a", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:ffff:ffff:ffff:ffff", "2001:db8:1:2::/64"},
		{"2001:DB8:1:2:0:0:0:1", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::a", "2001:db8:1:3::/64"},
		{"::1", "::/64"},

		// Bukan IP: dikembalikan apa adanya (tidak panic).
		{"bukan-ip", "bukan-ip"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := rateLimitKeyForIP(tc.in); got != tc.want {
			t.Errorf("rateLimitKeyForIP(%q) = %q, mau %q", tc.in, got, tc.want)
		}
	}
}

func TestRateLimitKeySatuSlash64BerbagiKunci(t *testing.T) {
	a := rateLimitKeyForIP("2001:db8:aa:bb:1::1")
	b := rateLimitKeyForIP("2001:db8:aa:bb:dead:beef:cafe:42")
	if a != b {
		t.Errorf("dua alamat dalam /64 yang sama harus berbagi kunci: %q vs %q", a, b)
	}
	if c := rateLimitKeyForIP("2001:db8:aa:bc::1"); c == a {
		t.Errorf("/64 berbeda tidak boleh berbagi kunci: %q", c)
	}
}
