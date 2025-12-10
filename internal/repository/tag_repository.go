package repository

import (
	"context"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"gorm.io/gorm"
)

type TagListParams struct {
	Query string
	Page  int
	Limit int
}

type TagRepository interface {
	List(ctx context.Context, params TagListParams) ([]models.Tag, int64, error)
	GetByID(ctx context.Context, id string) (*models.Tag, error)
	Create(ctx context.Context, tag *models.Tag) error
	Update(ctx context.Context, tag *models.Tag) error
	Delete(ctx context.Context, id string) error
}

type tagRepository struct {
	db *gorm.DB
}

func NewTagRepository(db *gorm.DB) TagRepository {
	return &tagRepository{db: db}
}

func (r *tagRepository) List(ctx context.Context, params TagListParams) ([]models.Tag, int64, error) {
	var tags []models.Tag
	var total int64

	q := r.db.WithContext(ctx).Model(&models.Tag{})

	if params.Query != "" {
		like := "%" + strings.ToLower(params.Query) + "%"
		q = q.Where("LOWER(name) LIKE ?", like)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit

	err := q.
		Order("created_at DESC").
		Limit(params.Limit).
		Offset(offset).
		Find(&tags).Error

	if err != nil {
		return nil, 0, err
	}

	return tags, total, nil
}

func (r *tagRepository) GetByID(ctx context.Context, id string) (*models.Tag, error) {
	var tag models.Tag
	if err := r.db.WithContext(ctx).First(&tag, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *tagRepository) Create(ctx context.Context, tag *models.Tag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *tagRepository) Update(ctx context.Context, tag *models.Tag) error {
	return r.db.WithContext(ctx).Save(tag).Error
}

func (r *tagRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Tag{}, "id = ?", id).Error
}
