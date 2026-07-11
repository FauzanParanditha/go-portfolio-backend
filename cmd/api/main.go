package main

import (
	"fmt"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/db"
	"github.com/FauzanParanditha/portfolio-backend/internal/denylist"
	"github.com/FauzanParanditha/portfolio-backend/internal/logger"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"

	_ "github.com/FauzanParanditha/portfolio-backend/docs"
	httprouter "github.com/FauzanParanditha/portfolio-backend/internal/http"
)

// @title           Portfolio Backend API
// @version         1.0
// @description     API untuk personal portfolio (projects, experiences, contact, dll).
// @contact.name    Fauzan
// @contact.email   fauzan@pandi.id

// @host      localhost:8080
// @BasePath  /api/v1
// @schemes   http
func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	// Init zerolog global logger
	logger.Init(cfg.AppEnv)

	// Validasi konfigurasi keamanan.
	// Di produksi, konfigurasi tidak aman menghentikan aplikasi.
	// Di non-produksi cukup peringatan agar developer experience tetap lancar.
	if err := cfg.Validate(); err != nil {
		if cfg.IsProduction() {
			log.Fatal().Err(err).Msg("refusing to start with insecure configuration")
		}
		log.Warn().Err(err).Msg("insecure configuration detected (allowed in non-production)")
	}

	log.Info().
		Str("env", cfg.AppEnv).
		Msg("starting ppnd-backend")

	gormDB := db.New(cfg)

	// Denylist token persisten berbasis Postgres agar revocation (logout) tetap
	// berlaku setelah proses restart — menggantikan default in-memory.
	dl := denylist.NewPostgres(gormDB)

	app := httprouter.NewRouter(httprouter.AppDeps{
		DB:       gormDB,
		Config:   cfg,
		Denylist: dl,
	})

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Info().Str("addr", addr).Msg("server listening")

	if err := app.Listen(addr); err != nil {
		log.Fatal().Err(err).Msg("fiber server error")
	}
}
