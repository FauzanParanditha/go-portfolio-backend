package validation

import (
	"strings"
	"testing"
)

// Struct uji meniru pemakaian nyata di DTO: field wajib, field opsional berupa
// pointer (seperti demoUrl/repoUrl), dan daftar gambar (seperti screenshots).
type wajibURL struct {
	Link string `validate:"required,httpurl"`
}

type opsionalURL struct {
	Link *string `validate:"omitempty,httpurl"`
}

type daftarGambar struct {
	Cover  string   `validate:"required,imageurl"`
	Images []string `validate:"dive,imageurl"`
}

func strPtr(s string) *string { return &s }

func TestIsHTTPURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"http://contoh.com", true},
		{"https://contoh.com/path?q=1#x", true},
		{"HTTPS://Contoh.com", true},
		{"https://localhost:8080/uploads/a.png", true},
		{"http://[::1]:8080/", true},

		{"javascript:alert(1)", false},
		{"JavaScript:alert(1)", false},
		{"javascript://contoh.com/%0aalert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"ftp://contoh.com/file", false},
		{"file:///etc/passwd", false},
		{"mailto:a@b.c", false},
		{"https://", false},      // tanpa host
		{"http:///path", false},  // tanpa host
		{"http://:8080/", false}, // hanya port, host kosong
		{"//contoh.com", false},  // protocol-relative, tanpa skema
		{"contoh.com", false},    // tanpa skema
		{"/relatif/saja", false}, // path relatif bukan link valid
		{"", false},              // kosong
		{"   ", false},
		{" https://contoh.com", false}, // spasi di tepi ditolak, bukan dipangkas                // spasi saja
		{"http://a b.com/%zz", false},  // gagal di-parse
	}

	for _, tc := range cases {
		if got := IsHTTPURL(tc.in); got != tc.want {
			t.Errorf("IsHTTPURL(%q) = %v, mau %v", tc.in, got, tc.want)
		}
	}
}

func TestIsImageURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://cdn.contoh.com/a.webp", true},
		{"http://localhost:8080/uploads/a.png", true},
		{"/images/zns.webp", true},

		{"javascript:alert(1)", false},
		{"data:image/svg+xml;base64,PHN2Zz4=", false},
		{"ftp://contoh.com/a.png", false},
		{"//evil.com/a.png", false},
		{`/\evil.com/a.png`, false},
		{"images/a.png", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := IsImageURL(tc.in); got != tc.want {
			t.Errorf("IsImageURL(%q) = %v, mau %v", tc.in, got, tc.want)
		}
	}
}

func TestHTTPURLTagWajib(t *testing.T) {
	if err := ValidateStruct(&wajibURL{Link: "https://contoh.com"}); err != nil {
		t.Fatalf("https seharusnya lolos: %v", err)
	}

	// String kosong pada field required harus gagal di tag `required`.
	err := ValidateStruct(&wajibURL{Link: ""})
	if err == nil {
		t.Fatal("string kosong seharusnya ditolak")
	}
	if got := ToFieldErrors(err)["Link"]; got != "wajib diisi" {
		t.Errorf("pesan kosong = %q, mau %q", got, "wajib diisi")
	}

	for _, buruk := range []string{"javascript:alert(1)", "data:text/html,x", "ftp://contoh.com", "https://"} {
		err := ValidateStruct(&wajibURL{Link: buruk})
		if err == nil {
			t.Errorf("%q seharusnya ditolak", buruk)
			continue
		}
		msg := ToFieldErrors(err)["Link"]
		if !strings.Contains(msg, "http://") || !strings.Contains(msg, "https://") {
			t.Errorf("pesan 422 untuk %q kurang jelas: %q", buruk, msg)
		}
	}
}

func TestHTTPURLTagOpsional(t *testing.T) {
	// nil (field tidak dikirim / null) lolos.
	for name, v := range map[string]*string{
		"nil":   nil,
		"https": strPtr("https://github.com/x/y"),
	} {
		if err := ValidateStruct(&opsionalURL{Link: v}); err != nil {
			t.Errorf("%s seharusnya lolos: %v", name, err)
		}
	}

	// Pointer ke "" TIDAK dilewati omitempty (pointer-nya tidak nil) dan tetap
	// ditolak — sama persis dengan perilaku tag `url` lama, jadi tidak ada
	// perubahan kontrak bagi klien.
	if err := ValidateStruct(&opsionalURL{Link: strPtr("")}); err == nil {
		t.Error("pointer ke string kosong seharusnya tetap ditolak (paritas dengan tag url lama)")
	}

	if err := ValidateStruct(&opsionalURL{Link: strPtr("javascript:alert(1)")}); err == nil {
		t.Fatal("javascript: seharusnya ditolak walau field opsional")
	}
}

func TestImageURLTag(t *testing.T) {
	ok := daftarGambar{
		Cover:  "/images/cover.webp",
		Images: []string{"https://cdn.contoh.com/1.png", "/images/2.png"},
	}
	if err := ValidateStruct(&ok); err != nil {
		t.Fatalf("seharusnya lolos: %v", err)
	}

	buruk := daftarGambar{
		Cover:  "https://cdn.contoh.com/c.png",
		Images: []string{"https://cdn.contoh.com/1.png", "javascript:alert(1)"},
	}
	err := ValidateStruct(&buruk)
	if err == nil {
		t.Fatal("screenshot javascript: seharusnya ditolak")
	}
	fields := ToFieldErrors(err)
	msg, ada := fields["Images[1]"]
	if !ada {
		t.Fatalf("detail error harus menunjuk elemen yang salah, dapat: %v", fields)
	}
	if !strings.Contains(msg, "/") || !strings.Contains(msg, "https://") {
		t.Errorf("pesan imageurl kurang jelas: %q", msg)
	}
}
