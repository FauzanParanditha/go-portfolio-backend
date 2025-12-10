package models

import (
	"time"

	"github.com/google/uuid"
)

type Experience struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	StartDate   time.Time `json:"startDate"`
	EndDate     *time.Time `json:"endDate"`
	IsCurrent   bool      `json:"isCurrent"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sortOrder"`

	Highlights []ExperienceHighlight `gorm:"foreignKey:ExperienceID" json:"highlights"`
	Tags       []Tag                 `gorm:"many2many:experience_tags;" json:"tags"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ExperienceHighlight struct {
	ID           uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ExperienceID uuid.UUID `gorm:"type:uuid" json:"experienceId"`
	Text         string    `json:"text"`
	SortOrder    int       `json:"sortOrder"`
}
