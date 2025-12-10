package repository

import (
	"context"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"gorm.io/gorm"
)

type ContactListParams struct {
	Query  string
	IsRead *bool
	Page   int
	Limit  int
}

type ContactMessageRepository interface {
	Create(ctx context.Context, m *models.ContactMessage) error
	List(ctx context.Context, params ContactListParams) ([]models.ContactMessage, int64, error)
	GetByID(ctx context.Context, id string) (*models.ContactMessage, error)
	MarkRead(ctx context.Context, id string, isRead bool) error
	Delete(ctx context.Context, id string) error
}

type contactMessageRepository struct {
	db *gorm.DB
}

func NewContactMessageRepository(db *gorm.DB) ContactMessageRepository {
	return &contactMessageRepository{db: db}
}

func (r *contactMessageRepository) Create(ctx context.Context, m *models.ContactMessage) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *contactMessageRepository) List(ctx context.Context, params ContactListParams) ([]models.ContactMessage, int64, error) {
	var msgs []models.ContactMessage
	var total int64

	q := r.db.WithContext(ctx).Model(&models.ContactMessage{})

	if params.Query != "" {
		like := "%" + strings.ToLower(params.Query) + "%"
		q = q.Where(
			r.db.Where("LOWER(name) LIKE ?", like).
				Or("LOWER(email) LIKE ?", like).
				Or("LOWER(subject) LIKE ?", like),
		)
	}

	if params.IsRead != nil {
		q = q.Where("is_read = ?", *params.IsRead)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit

	if err := q.
		Order("created_at DESC").
		Limit(params.Limit).
		Offset(offset).
		Find(&msgs).Error; err != nil {
		return nil, 0, err
	}

	return msgs, total, nil
}

func (r *contactMessageRepository) GetByID(ctx context.Context, id string) (*models.ContactMessage, error) {
	var m models.ContactMessage
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *contactMessageRepository) MarkRead(ctx context.Context, id string, isRead bool) error {
	return r.db.WithContext(ctx).
		Model(&models.ContactMessage{}).
		Where("id = ?", id).
		Update("is_read", isRead).Error
}

func (r *contactMessageRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Delete(&models.ContactMessage{}, "id = ?", id).Error
}
