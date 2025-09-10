package utils

import (
	"github.com/FauzanParanditha/portfolio-backend/config"
	"github.com/joho/godotenv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigration() {
	err := godotenv.Load()
	if err != nil {
		Logger.Fatal("Error loading .env")
	}

	db := config.DB
	sqlDB, err := db.DB()
	if err != nil {
		Logger.Fatal("Failed to get raw DB:", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		Logger.Fatal("Failed to create migration driver:", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver,
	)
	if err != nil {
		Logger.Fatal("Migration instance failed:", err)
	}

	Logger.Info("Running database migration...")

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		Logger.Fatal("Migration failed:", err)
	}

	Logger.Info("Migration completed successfully.")
}
