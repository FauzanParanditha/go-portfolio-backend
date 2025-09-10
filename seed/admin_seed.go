package seed

import (
	"fmt"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/config"
	"github.com/FauzanParanditha/portfolio-backend/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func SeedAdmin() {
	db := config.DB

	// Cek apakah admin dengan email ini sudah ada
	var existing models.Admin
	email := "admin@pandi.id"
	if err := db.Where("email = ?", email).First(&existing).Error; err == nil {
		fmt.Println("[Seeder] Admin already exists, skipping seeding.")
		return
	}

	// Hash password
	password := "Pandi@123#"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic("Failed to hash password: " + err.Error())
	}

	admin := models.Admin{
		ID:        uuid.New(),
		FullName:  "Super Admin",
		Email:     email,
		Password:  string(hashedPassword),
		Role:      "superadmin",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(&admin).Error; err != nil {
		panic("Failed to seed admin: " + err.Error())
	}

	fmt.Println("[Seeder] Admin user seeded successfully!")
}
