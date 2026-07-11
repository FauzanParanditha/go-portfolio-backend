package repository

import (
	"context"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExperienceAdminListParams adalah filter/pagination untuk list admin experience.
type ExperienceAdminListParams struct {
	Query string
	Page  int
	Limit int
}

// ExperienceWriteInput adalah agregat data tulis experience. Scalar sudah
// dipetakan handler (termasuk StartDate/EndDate yang sudah jadi time.Time),
// relasi dikirim sebagai data mentah.
type ExperienceWriteInput struct {
	Experience models.Experience // scalar saja
	TagIDs     []uuid.UUID
	Highlights []string // index = sort_order; string kosong di-skip di repo
}

type ExperienceRepository interface {
	ListPublic(ctx context.Context) ([]models.Experience, error)

	ListAdmin(ctx context.Context, params ExperienceAdminListParams) ([]models.Experience, int64, error)
	GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Experience, error)
	Create(ctx context.Context, in ExperienceWriteInput) (*models.Experience, error)
	Update(ctx context.Context, id uuid.UUID, in ExperienceWriteInput) (*models.Experience, error)
	Delete(ctx context.Context, id uuid.UUID) error
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

// reloadExperience memuat ulang experience beserta relasinya (di luar transaksi).
func (r *experienceRepository) reloadExperience(ctx context.Context, id uuid.UUID) (*models.Experience, error) {
	var exp models.Experience
	if err := r.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		First(&exp, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &exp, nil
}

func (r *experienceRepository) ListAdmin(ctx context.Context, params ExperienceAdminListParams) ([]models.Experience, int64, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	qb := r.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		Model(&models.Experience{})

	if params.Query != "" {
		like := "%" + params.Query + "%"
		qb = qb.Where(
			r.db.Where("experiences.title ILIKE ?", like).
				Or("experiences.company ILIKE ?", like),
		)
	}

	var total int64
	if err := qb.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var exps []models.Experience
	if err := qb.
		Order("experiences.sort_order ASC").
		Order("experiences.start_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&exps).Error; err != nil {
		return nil, 0, err
	}

	return exps, total, nil
}

func (r *experienceRepository) GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Experience, error) {
	var exp models.Experience
	if err := r.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		First(&exp, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &exp, nil
}

func (r *experienceRepository) Create(ctx context.Context, in ExperienceWriteInput) (*models.Experience, error) {
	exp := in.Experience

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Tags: resolve by id (parameterized), transaksi otomatis rollback bila error.
		if len(in.TagIDs) > 0 {
			var tags []models.Tag
			if err := tx.Where("id IN ?", in.TagIDs).Find(&tags).Error; err != nil {
				return err
			}
			exp.Tags = tags
		}

		if err := tx.Create(&exp).Error; err != nil {
			return err
		}

		// Highlights: skip string kosong, index = sort_order.
		if err := insertExperienceHighlights(tx, exp.ID, in.Highlights); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.reloadExperience(ctx, exp.ID)
}

func (r *experienceRepository) Update(ctx context.Context, id uuid.UUID, in ExperienceWriteInput) (*models.Experience, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First di dalam tx agar ErrRecordNotFound mengalir keluar ke handler → 404.
		var exp models.Experience
		if err := tx.First(&exp, "id = ?", id).Error; err != nil {
			return err
		}

		// Overwrite scalar dari input.
		exp.Title = in.Experience.Title
		exp.Company = in.Experience.Company
		exp.Location = in.Experience.Location
		exp.StartDate = in.Experience.StartDate
		exp.EndDate = in.Experience.EndDate
		exp.IsCurrent = in.Experience.IsCurrent
		exp.Description = in.Experience.Description
		exp.SortOrder = in.Experience.SortOrder

		if err := tx.Save(&exp).Error; err != nil {
			return err
		}

		// Tags: Replace bila ada, Clear bila kosong.
		if len(in.TagIDs) > 0 {
			var tags []models.Tag
			if err := tx.Where("id IN ?", in.TagIDs).Find(&tags).Error; err != nil {
				return err
			}
			if err := tx.Model(&exp).Association("Tags").Replace(&tags); err != nil {
				return err
			}
		} else {
			if err := tx.Model(&exp).Association("Tags").Clear(); err != nil {
				return err
			}
		}

		// Highlights: hapus lama lalu insert baru.
		if err := tx.Where("experience_id = ?", exp.ID).Delete(&models.ExperienceHighlight{}).Error; err != nil {
			return err
		}
		if err := insertExperienceHighlights(tx, exp.ID, in.Highlights); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.reloadExperience(ctx, id)
}

func (r *experienceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Perilaku dipertahankan: sukses walau 0 baris terhapus.
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Experience{}).Error
}

// insertExperienceHighlights membangun & menyisipkan highlights dari []string
// (skip string kosong, SortOrder = index). Tidak melakukan apa-apa bila kosong.
func insertExperienceHighlights(tx *gorm.DB, expID uuid.UUID, highlights []string) error {
	if len(highlights) == 0 {
		return nil
	}
	highs := make([]models.ExperienceHighlight, 0, len(highlights))
	for i, text := range highlights {
		if text == "" {
			continue
		}
		highs = append(highs, models.ExperienceHighlight{
			ExperienceID: expID,
			Text:         text,
			SortOrder:    i,
		})
	}
	if len(highs) == 0 {
		return nil
	}
	return tx.Create(&highs).Error
}
