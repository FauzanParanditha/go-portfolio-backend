package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Project struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Title         string    `json:"title" validate:"required"`
	Description   string    `json:"description" validate:"required"`
	Features      []string  `json:"features"`
	TechStack     []string  `json:"techStack"`
	LiveDemoURL   string    `json:"liveDemoURL" validate:"omitempty,url"`
	SourceCodeURL string    `json:"sourceCodeURL" validate:"omitempty,url"`
	ThumbnailURL  string    `json:"thumbnailURL" validate:"omitempty,url"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Auto-create UUID on insert
func (p *Project) BeforeCreate(tx *gorm.DB) (err error) {
	p.ID = uuid.New()
	return
}
