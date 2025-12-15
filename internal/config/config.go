package config

import (
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
)

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

		CORSAllowedOrigins: helpers.GetEnv("CORS_ALLOWED_ORIGINS", "*"),
		CORSAllowedMethods: helpers.GetEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		CORSAllowedHeaders: helpers.GetEnv("CORS_ALLOWED_HEADERS", "Origin, Content-Type, Accept, Authorization"),
		CORSAllowCredentials: helpers.GetEnvBool("CORS_ALLOW_CREDENTIALS", false),
	}
}
