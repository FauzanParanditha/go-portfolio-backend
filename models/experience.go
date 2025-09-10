package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Experience struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Title       string     `json:"title" validate:"required"`
	Company     string     `json:"company" validate:"required"`
	Location    string     `json:"location" validate:"required"`
	StartDate   time.Time  `json:"startDate" validate:"required"`
	EndDate     *time.Time `json:"endDate"`
	Description []string   `json:"description"`
	TechUsed    []string   `json:"techUsed"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (e *Experience) BeforeCreate(tx *gorm.DB) (err error) {
	e.ID = uuid.New()
	return
}
