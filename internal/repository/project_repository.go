package repository

import (
	"context"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"gorm.io/gorm"
)

type ProjectListParams struct {
	FeaturedOnly bool
	Query        string
	Page         int
	Limit        int
}

type ProjectRepository interface {
	ListPublic(ctx context.Context, params ProjectListParams) ([]models.Project, int64, error)
	GetBySlug(ctx context.Context, slug string) (*models.Project, error)
}

type projectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepository{db: db}
}

// baseQuery: preload semua relasi yang dibutuhkan untuk public response
func (r *projectRepository) baseQuery() *gorm.DB {
	return r.db.
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		Preload("Screenshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_screenshots.sort_order ASC")
		}).
		Order("projects.sort_order ASC").
		Order("projects.created_at DESC")
}

func (r *projectRepository) ListPublic(ctx context.Context, params ProjectListParams) ([]models.Project, int64, error) {
	var (
		projects []models.Project
		total    int64
	)

	q := r.baseQuery().Model(&models.Project{})

	if params.FeaturedOnly {
		q = q.Where("projects.is_featured = ?", true)
	}

	if params.Query != "" {
		like := "%" + strings.ToLower(params.Query) + "%"
		q = q.Where(
			r.db.
				Where("LOWER(projects.title) LIKE ?", like).
				Or("LOWER(projects.short_desc) LIKE ?", like).
				Or("LOWER(projects.long_desc) LIKE ?", like).
				Or("LOWER(projects.category) LIKE ?", like),
		)
	}

	// hitung total (untuk pagination)
	if err := q.WithContext(ctx).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// pagination
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit <= 0 {
		params.Limit = 12
	}

	offset := (params.Page - 1) * params.Limit

	if err := q.WithContext(ctx).
		Limit(params.Limit).
		Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

func (r *projectRepository) GetBySlug(ctx context.Context, slug string) (*models.Project, error) {
	var p models.Project

	if err := r.baseQuery().
		WithContext(ctx).
		Where("projects.slug = ?", slug).
		First(&p).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, err
	}

	return &p, nil
}
