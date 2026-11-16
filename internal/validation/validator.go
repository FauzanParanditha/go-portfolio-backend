package validation

import (
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = newValidator()

// newValidator membuat instance validator beserta aturan kustom proyek ini.
// Gagal mendaftarkan aturan = bug pemrograman, jadi panic saat init lebih baik
// daripada tag yang diam-diam tidak pernah dijalankan.
func newValidator() *validator.Validate {
	v := validator.New()

	mustRegister(v, "httpurl", func(fl validator.FieldLevel) bool {
		return IsHTTPURL(fl.Field().String())
	})
	mustRegister(v, "imageurl", func(fl validator.FieldLevel) bool {
		return IsImageURL(fl.Field().String())
	})

	return v
}

func mustRegister(v *validator.Validate, tag string, fn validator.Func) {
	if err := v.RegisterValidation(tag, fn); err != nil {
		panic("gagal mendaftarkan validator " + tag + ": " + err.Error())
	}
}

// IsHTTPURL true hanya untuk URL absolut berskema http/https dengan host yang
// tidak kosong.
//
// Tag `url` bawaan go-playground menerima skema APA SAJA — `javascript:alert(1)`
// dan `data:text/html,...` lolos — padahal nilai seperti demoUrl/repoUrl
// dirender frontend sebagai `href`. Aturan ini menutup celah XSS tersebut.
func IsHTTPURL(s string) bool {
	// Nilai berspasi di tepi ditolak (bukan dipangkas) agar yang tersimpan sama
	// persis dengan yang divalidasi. Ini SENGAJA lebih ketat daripada tag `url`
	// lama: tag itu menolak spasi di depan tapi MENERIMA spasi di akhir
	// ("https://x/y ") dan juga bentuk tanpa "//" seperti "http:example.com"
	// (host kosong, ditolak di bawah). Klien diharapkan memangkas spasi sebelum
	// mengirim; backend tidak memangkas diam-diam.
	if s == "" || s != strings.TrimSpace(s) {
		return false
	}

	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	// u.Hostname() membuang port & kurung IPv6; "http://:8080" atau "http:///x"
	// menghasilkan host kosong dan ditolak.
	return u.Hostname() != ""
}

// IsImageURL true untuk sumber gambar yang aman: URL http/https absolut (lihat
// IsHTTPURL) ATAU path relatif terhadap root situs, mis. "/images/cover.webp".
//
// Path relatif tetap diizinkan karena frontend melayani aset dari folder
// `public/` dan merendernya apa adanya. Yang ditolak adalah skema lain
// (`javascript:`, `data:`, `file:`), path tanpa "/" di depan, serta "//host"
// (URL protocol-relative yang sebenarnya menunjuk host lain).
func IsImageURL(s string) bool {
	if IsHTTPURL(s) {
		return true
	}

	if s != strings.TrimSpace(s) || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, `/\`) {
		return false
	}

	// Path relatif tidak boleh membawa skema maupun host setelah di-parse.
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "" && u.Host == ""
}

func ValidateStruct(s any) error {
	return validate.Struct(s)
}

// Ubah ValidationErrors → map[field]message sederhana
func ToFieldErrors(err error) map[string]string {
	res := map[string]string{}

	if verrs, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range verrs {
			field := fe.Field()

			switch fe.Tag() {
			case "required":
				res[field] = "wajib diisi"
			case "email":
				res[field] = "format email tidak valid"
			case "max":
				// Untuk string, `max` di go-playground menghitung karakter (rune).
				res[field] = "maksimal " + fe.Param() + " karakter"
			case "url":
				res[field] = "format URL tidak valid"
			case "httpurl":
				res[field] = "harus berupa URL http:// atau https:// yang valid, mis. https://contoh.com"
			case "imageurl":
				res[field] = "harus berupa URL gambar http:// atau https://, atau path yang diawali \"/\""
			default:
				res[field] = "tidak valid"
			}
		}
	}

	return res
}
