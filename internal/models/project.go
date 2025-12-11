package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
)

type Project struct {
	ID    uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Title string    `json:"title"`
	Slug  string    `gorm:"uniqueIndex" json:"slug"`

	ShortDesc string `json:"shortDesc"`
	LongDesc  string `json:"longDescription"`

	CoverImageURL string `json:"coverImageUrl"`

	Category string `json:"category"`
	Timeline string `json:"timeline"`
	Role     string `json:"role"`

	Challenge string `json:"challenge"`
	Solution  string `json:"solution"`

	Results          pq.StringArray `gorm:"type:text[]" json:"results"`
	TechnicalDetails datatypes.JSON `json:"technicalDetails"`

	DemoURL *string `json:"demoUrl"`
	RepoURL *string `json:"repoUrl"`

	// relationships
	Screenshots []ProjectScreenshot `gorm:"foreignKey:ProjectID" json:"screenshots"`

	Features []ProjectFeature `gorm:"foreignKey:ProjectID" json:"features"`
	Tags     []Tag            `gorm:"many2many:project_tags;" json:"tags"`

	IsFeatured bool      `json:"isFeatured"`
	SortOrder  int       `json:"sortOrder"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type ProjectScreenshot struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid" json:"projectId"`
	ImageURL  string    `json:"imageUrl"`
	SortOrder int       `json:"sortOrder"`
}

type ProjectFeature struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid" json:"projectId"`
	Text      string    `json:"text"`
	SortOrder int       `json:"sortOrder"`
}
