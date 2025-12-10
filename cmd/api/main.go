package main

import (
	"fmt"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/db"
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

	log.Info().
		Str("env", cfg.AppEnv).
		Msg("starting ppnd-backend")

	gormDB := db.New(cfg)

	app := httprouter.NewRouter(httprouter.AppDeps{
		DB:     gormDB,
		Config: cfg,
	})

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Info().Str("addr", addr).Msg("server listening")

	if err := app.Listen(addr); err != nil {
		log.Fatal().Err(err).Msg("fiber server error")
	}
}
