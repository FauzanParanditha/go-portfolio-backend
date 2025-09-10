package models

import (
	"time"

	"github.com/google/uuid"
)

type Admin struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	FullName  string    `json:"fullName" validate:"required"`
	Email     string    `json:"email" validate:"required,email" gorm:"unique"`
	Password  string    `json:"password" validate:"required"` // hashed password
	Role      string    `json:"role" validate:"required"`     // e.g., "admin", "superadmin"
	CreatedAt time.Time
	UpdatedAt time.Time
}
