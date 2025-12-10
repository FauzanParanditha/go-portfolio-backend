package models

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Title         string    `json:"title"`
	Slug          string    `gorm:"uniqueIndex" json:"slug"`
	ShortDesc     string    `json:"shortDesc"`
	CoverImageURL string    `json:"coverImageUrl"`
	LiveURL       *string   `json:"liveUrl"`
	SourceURL     *string   `json:"sourceUrl"`
	IsFeatured    bool      `json:"isFeatured"`
	SortOrder     int       `json:"sortOrder"`

	// Relations
	Features []ProjectFeature `gorm:"foreignKey:ProjectID" json:"features"`
	Tags     []Tag            `gorm:"many2many:project_tags;" json:"tags"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectFeature struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid" json:"projectId"`
	Text      string    `json:"text"`
	SortOrder int       `json:"sortOrder"`
}
