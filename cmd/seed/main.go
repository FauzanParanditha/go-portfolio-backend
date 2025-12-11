package main

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/db"
	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	database := db.New(cfg)

	log.Info().Msg("running seeder...")

	if err := seedAdminUser(database); err != nil {
		log.Fatal().Err(err).Msg("failed to seed admin user")
	}

	// optional: sample tags + project
	if os.Getenv("SEED_SAMPLE_DATA") == "true" {
		if err := seedSampleTagsAndProject(database); err != nil {
			log.Fatal().Err(err).Msg("failed to seed sample data")
		}
	}

	log.Info().Msg("seeding completed")
}

// ---------------------------------------------------------
// Seeder: Admin User
// ---------------------------------------------------------

func seedAdminUser(db *gorm.DB) error {
	name := helpers.GetEnv("SEED_ADMIN_NAME", "Admin")
	email := helpers.GetEnv("SEED_ADMIN_EMAIL", "admin@example.com")
	password := helpers.GetEnv("SEED_ADMIN_PASSWORD", "password-admin")

	// cek apakah sudah ada user dengan email ini
	var existing models.User
	if err := db.Where("email = ?", email).First(&existing).Error; err == nil {
		log.Info().
			Str("email", email).
			Msg("admin user already exists, skip creating")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	u := models.User{
		Name:     name,
		Email:    email,
		Password: string(hash),
		Role:     "admin",
	}

	if err := db.Create(&u).Error; err != nil {
		return err
	}

	log.Info().
		Str("email", email).
		Str("password", password).
		Msg("admin user created (PLEASE change password in production)")

	return nil
}

// ---------------------------------------------------------
// Seeder: Sample Tags & Project (opsional)
// ---------------------------------------------------------

func seedSampleTagsAndProject(db *gorm.DB) error {
	// kalau sudah ada project, skip
	var count int64
	if err := db.Model(&models.Project{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		log.Info().Msg("projects already exist, skip sample project seeding")
		return nil
	}

	// Seed tags kalau belum ada
	var tagCount int64
	if err := db.Model(&models.Tag{}).Count(&tagCount).Error; err != nil {
		return err
	}

	var tags []models.Tag
	if tagCount == 0 {
		tags = []models.Tag{
			{Name: "Go", Type: "backend"},
			{Name: "PostgreSQL", Type: "database"},
			{Name: "Fiber", Type: "framework"},
			{Name: "Docker", Type: "devops"},
		}
		if err := db.Create(&tags).Error; err != nil {
			return err
		}
		log.Info().Int("count", len(tags)).Msg("sample tags created")
	} else {
		log.Info().Msg("tags already exist, not creating sample tags")
		// load beberapa tag random untuk dipakai di project
		if err := db.Limit(3).Find(&tags).Error; err != nil {
			return err
		}
	}

	// Sample projectx
	p := models.Project{
		Title:         "Personal Portfolio",
		Slug:          "personal-portfolio",
		ShortDesc:     "My personal portfolio website built with Go Fiber, PostgreSQL, and Next.js.",
		CoverImageURL: "https://example.com/cover/portfolio.png",
		IsFeatured:    true,
		SortOrder:     1,
		Tags:          tags,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := db.Create(&p).Error; err != nil {
		return err
	}

	features := []models.ProjectFeature{
		{ProjectID: p.ID, Text: "Responsive layout with dark mode", SortOrder: 1},
		{ProjectID: p.ID, Text: "Admin dashboard with JWT auth", SortOrder: 2},
		{ProjectID: p.ID, Text: "Image optimization and SEO-friendly routing", SortOrder: 3},
	}

	if err := db.Create(&features).Error; err != nil {
		return err
	}

	log.Info().
		Str("slug", p.Slug).
		Msg("sample project created")

	return nil
}
