package main

import (
	"os"

	"github.com/FauzanParanditha/portfolio-backend/config"
	"github.com/FauzanParanditha/portfolio-backend/routes"
	"github.com/FauzanParanditha/portfolio-backend/utils"
	"github.com/joho/godotenv"
)

func main() {
	utils.InitLogger()

	err := godotenv.Load()
	if err != nil {
		utils.Logger.Fatal("Error loading .env")
	}

	utils.Logger.Info("Starting servzer...")

	config.InitDB()

	utils.RunMigration()

	// seed.SeedAdmin()

	r := routes.SetupRouter()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	utils.Logger.Infof("Server running at http://localhost:%s", port)
	r.Run(":" + port)
}
