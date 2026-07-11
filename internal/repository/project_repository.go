package repository

import (
	"context"
	"strings"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProjectListParams struct {
	FeaturedOnly bool
	Query        string
	Page         int
	Limit        int
}

// ProjectAdminListParams adalah filter/pagination untuk list admin project.
type ProjectAdminListParams struct {
	Query        string
	FeaturedOnly bool
	Page         int
	Limit        int
}

// ProjectWriteInput adalah agregat data tulis project. Scalar sudah dipetakan
// handler (termasuk TechnicalDetails datatypes.JSON), relasi dikirim mentah.
type ProjectWriteInput struct {
	Project     models.Project // scalar saja
	TagIDs      []uuid.UUID
	Features    []string // index = sort_order; string kosong di-skip di repo
	Screenshots []string // index = sort_order; string kosong di-skip di repo
}

type ProjectRepository interface {
	ListPublic(ctx context.Context, params ProjectListParams) ([]models.Project, int64, error)
	GetBySlug(ctx context.Context, slug string) (*models.Project, error)

	ListAdmin(ctx context.Context, params ProjectAdminListParams) ([]models.Project, int64, error)
	GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Project, error)
	Create(ctx context.Context, in ProjectWriteInput) (*models.Project, error)
	Update(ctx context.Context, id uuid.UUID, in ProjectWriteInput) (*models.Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
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

// reloadProject memuat ulang project beserta relasinya (di luar transaksi).
func (r *projectRepository) reloadProject(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	if err := r.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		Preload("Screenshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_screenshots.sort_order ASC")
		}).
		First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *projectRepository) ListAdmin(ctx context.Context, params ProjectAdminListParams) ([]models.Project, int64, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	q := r.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		Preload("Screenshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_screenshots.sort_order ASC")
		}).
		Model(&models.Project{})

	if params.Query != "" {
		like := "%" + params.Query + "%"
		q = q.Where(
			r.db.Where("projects.title ILIKE ?", like).
				Or("projects.short_desc ILIKE ?", like).
				Or("projects.long_desc ILIKE ?", like).
				Or("projects.category ILIKE ?", like),
		)
	}

	if params.FeaturedOnly {
		q = q.Where("projects.is_featured = ?", true)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var projects []models.Project
	if err := q.
		Order("projects.sort_order ASC").
		Order("projects.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

func (r *projectRepository) GetByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	if err := r.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		Preload("Screenshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_screenshots.sort_order ASC")
		}).
		First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *projectRepository) Create(ctx context.Context, in ProjectWriteInput) (*models.Project, error) {
	project := in.Project

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Tags many-to-many: resolve by id (parameterized).
		if len(in.TagIDs) > 0 {
			var tags []models.Tag
			if err := tx.Where("id IN ?", in.TagIDs).Find(&tags).Error; err != nil {
				return err
			}
			project.Tags = tags
		}

		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		if err := insertProjectFeatures(tx, project.ID, in.Features); err != nil {
			return err
		}
		if err := insertProjectScreenshots(tx, project.ID, in.Screenshots); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.reloadProject(ctx, project.ID)
}

func (r *projectRepository) Update(ctx context.Context, id uuid.UUID, in ProjectWriteInput) (*models.Project, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First di dalam tx agar ErrRecordNotFound mengalir keluar ke handler → 404.
		var project models.Project
		if err := tx.First(&project, "id = ?", id).Error; err != nil {
			return err
		}

		// Overwrite scalar dari input.
		project.Title = in.Project.Title
		project.Slug = in.Project.Slug
		project.ShortDesc = in.Project.ShortDesc
		project.LongDesc = in.Project.LongDesc
		project.CoverImageURL = in.Project.CoverImageURL

		project.Category = in.Project.Category
		project.Timeline = in.Project.Timeline
		project.Role = in.Project.Role

		project.Challenge = in.Project.Challenge
		project.Solution = in.Project.Solution

		project.Results = in.Project.Results

		// TechnicalDetails hanya ditimpa bila non-nil (mempertahankan perilaku:
		// body tanpa technicalDetails tidak menghapus JSON lama).
		if in.Project.TechnicalDetails != nil {
			project.TechnicalDetails = in.Project.TechnicalDetails
		}

		project.DemoURL = in.Project.DemoURL
		project.RepoURL = in.Project.RepoURL
		project.IsFeatured = in.Project.IsFeatured
		project.SortOrder = in.Project.SortOrder

		if err := tx.Save(&project).Error; err != nil {
			return err
		}

		// Tags: Replace bila ada, Clear bila kosong.
		if len(in.TagIDs) > 0 {
			var tags []models.Tag
			if err := tx.Where("id IN ?", in.TagIDs).Find(&tags).Error; err != nil {
				return err
			}
			if err := tx.Model(&project).Association("Tags").Replace(&tags); err != nil {
				return err
			}
		} else {
			if err := tx.Model(&project).Association("Tags").Clear(); err != nil {
				return err
			}
		}

		// Features: hapus lama lalu insert baru.
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectFeature{}).Error; err != nil {
			return err
		}
		if err := insertProjectFeatures(tx, project.ID, in.Features); err != nil {
			return err
		}

		// Screenshots: hapus lama lalu insert baru.
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectScreenshot{}).Error; err != nil {
			return err
		}
		if err := insertProjectScreenshots(tx, project.ID, in.Screenshots); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.reloadProject(ctx, id)
}

func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Perilaku dipertahankan: sukses walau 0 baris terhapus.
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Project{}).Error
}

// insertProjectFeatures menyisipkan features dari []string (skip kosong,
// SortOrder = index). Tidak melakukan apa-apa bila kosong.
func insertProjectFeatures(tx *gorm.DB, projectID uuid.UUID, features []string) error {
	if len(features) == 0 {
		return nil
	}
	rows := make([]models.ProjectFeature, 0, len(features))
	for i, text := range features {
		if text == "" {
			continue
		}
		rows = append(rows, models.ProjectFeature{
			ProjectID: projectID,
			Text:      text,
			SortOrder: i,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// insertProjectScreenshots menyisipkan screenshots dari []string URL (skip
// kosong, SortOrder = index). Tidak melakukan apa-apa bila kosong.
func insertProjectScreenshots(tx *gorm.DB, projectID uuid.UUID, screenshots []string) error {
	if len(screenshots) == 0 {
		return nil
	}
	rows := make([]models.ProjectScreenshot, 0, len(screenshots))
	for i, url := range screenshots {
		if url == "" {
			continue
		}
		rows = append(rows, models.ProjectScreenshot{
			ProjectID: projectID,
			ImageURL:  url,
			SortOrder: i,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}
