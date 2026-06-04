package config

import (
	"errors"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
)

// defaultInsecureJWTSecret adalah nilai placeholder yang TIDAK boleh dipakai di produksi.
const defaultInsecureJWTSecret = "super-secret-ganti-sendiri"

type Config struct {
	AppEnv string

	AppPort string
	DBDSN   string

	JWTSecret    string
	JWTExpiresIn int

	CORSAllowedOrigins string
	CORSAllowedMethods string
	CORSAllowedHeaders string
	CORSAllowCredentials bool
}

func Load() *Config {
	return &Config{
		AppEnv: helpers.GetEnv("APP_ENV", "development"),

		AppPort: helpers.GetEnv("APP_PORT", "8080"),
		DBDSN:   helpers.GetEnv("DB_DSN", "postgres://postgres:postgres@localhost:5432/ppnd?sslmode=disable"),

		JWTSecret:    helpers.GetEnv("JWT_SECRET", "super-secret-ganti-sendiri"),
		JWTExpiresIn: helpers.GetEnvInt("JWT_EXPIRES_IN", 1800),

		CORSAllowedOrigins: helpers.GetEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		CORSAllowedMethods: helpers.GetEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		CORSAllowedHeaders: helpers.GetEnv("CORS_ALLOWED_HEADERS", "Origin, Content-Type, Accept, Authorization"),
		CORSAllowCredentials: helpers.GetEnvBool("CORS_ALLOW_CREDENTIALS", false),
	}
}

// IsProduction mengembalikan true jika aplikasi berjalan di environment produksi.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// Validate memeriksa konfigurasi yang berbahaya secara keamanan.
// Di produksi konfigurasi yang tidak aman akan menghentikan aplikasi.
func (c *Config) Validate() error {
	var problems []string

	// JWT secret tidak boleh kosong, default, atau terlalu pendek.
	switch {
	case c.JWTSecret == "":
		problems = append(problems, "JWT_SECRET kosong")
	case c.JWTSecret == defaultInsecureJWTSecret:
		problems = append(problems, "JWT_SECRET masih memakai nilai default yang tidak aman")
	case len(c.JWTSecret) < 32:
		problems = append(problems, "JWT_SECRET terlalu pendek (minimal 32 karakter)")
	}

	// CORS wildcard + credentials adalah kombinasi yang tidak valid/berbahaya.
	if c.CORSAllowCredentials && strings.Contains(c.CORSAllowedOrigins, "*") {
		problems = append(problems, "CORS_ALLOW_CREDENTIALS=true tidak boleh dikombinasikan dengan origin '*'")
	}

	if len(problems) == 0 {
		return nil
	}

	return errors.New("konfigurasi tidak aman: " + strings.Join(problems, "; "))
}
