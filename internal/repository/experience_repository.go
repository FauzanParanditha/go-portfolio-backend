package repository

import (
	"context"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"gorm.io/gorm"
)

type ExperienceRepository interface {
	ListPublic(ctx context.Context) ([]models.Experience, error)
}

type experienceRepository struct {
	db *gorm.DB
}

func NewExperienceRepository(db *gorm.DB) ExperienceRepository {
	return &experienceRepository{db: db}
}

func (r *experienceRepository) ListPublic(ctx context.Context) ([]models.Experience, error) {
	var exps []models.Experience

	err := r.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		Order("experiences.sort_order ASC").
		Order("experiences.start_date DESC").
		Find(&exps).Error

	if err != nil {
		return nil, err
	}

	return exps, nil
}
