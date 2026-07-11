package repository

import (
	"context"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"gorm.io/gorm"
)

// DashboardCounts adalah hasil agregasi mentah untuk dashboard overview,
// netral terhadap bentuk response (handler yang menyusunnya ke DTO).
type DashboardCounts struct {
	ProjectsTotal    int64
	ProjectsFeatured int64
	ProjectsRecent   int64

	ExperiencesTotal   int64
	ExperiencesCurrent int64
	ExperiencesRecent  int64

	ContactTotal  int64
	ContactUnread int64
	ContactRecent int64
}

type DashboardRepository interface {
	// Overview menghitung seluruh agregat sejak `since` (batas "recent").
	Overview(ctx context.Context, since time.Time) (DashboardCounts, error)
}

type dashboardRepository struct {
	db *gorm.DB
}

func NewDashboardRepository(db *gorm.DB) DashboardRepository {
	return &dashboardRepository{db: db}
}

func (r *dashboardRepository) Overview(ctx context.Context, since time.Time) (DashboardCounts, error) {
	var counts DashboardCounts

	db := r.db.WithContext(ctx)

	// ===== Projects =====
	if err := db.Model(&models.Project{}).Count(&counts.ProjectsTotal).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.Project{}).Where("is_featured = ?", true).Count(&counts.ProjectsFeatured).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.Project{}).Where("created_at >= ?", since).Count(&counts.ProjectsRecent).Error; err != nil {
		return counts, err
	}

	// ===== Experiences =====
	if err := db.Model(&models.Experience{}).Count(&counts.ExperiencesTotal).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.Experience{}).Where("is_current = ?", true).Count(&counts.ExperiencesCurrent).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.Experience{}).Where("created_at >= ?", since).Count(&counts.ExperiencesRecent).Error; err != nil {
		return counts, err
	}

	// ===== Contact Messages =====
	if err := db.Model(&models.ContactMessage{}).Count(&counts.ContactTotal).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.ContactMessage{}).Where("is_read = ?", false).Count(&counts.ContactUnread).Error; err != nil {
		return counts, err
	}
	if err := db.Model(&models.ContactMessage{}).Where("created_at >= ?", since).Count(&counts.ContactRecent).Error; err != nil {
		return counts, err
	}

	return counts, nil
}
