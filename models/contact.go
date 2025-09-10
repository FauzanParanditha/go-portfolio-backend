package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Contact struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Name      string    `json:"name" validate:"required,min=3"`
	Email     string    `json:"email" validate:"required,email"`
	Subject   string    `json:"subject" validate:"required"`
	Message   string    `json:"message" validate:"required,min=10"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *Contact) BeforeCreate(tx *gorm.DB) (err error) {
	c.ID = uuid.New()
	return
}
