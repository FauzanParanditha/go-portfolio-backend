package repository

import (
	"context"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PasswordResetRepository mengelola siklus hidup token reset password.
type PasswordResetRepository interface {
	// Create menyimpan token baru (sudah dalam bentuk hash).
	Create(ctx context.Context, t *models.PasswordResetToken) error
	// FindValidByHash mengambil token yang belum dipakai dan belum kedaluwarsa.
	// Meneruskan gorm.ErrRecordNotFound apa adanya bila tidak ada.
	FindValidByHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error)
	// MarkUsed menandai token terpakai agar tidak bisa dipakai dua kali.
	MarkUsed(ctx context.Context, id uuid.UUID) error
	// InvalidateAllForUser menandai semua token aktif milik user sebagai
	// terpakai. Dipanggil saat menerbitkan token baru dan setelah reset sukses,
	// sehingga hanya ada maksimal satu token hidup per user.
	InvalidateAllForUser(ctx context.Context, userID uuid.UUID) error
	// DeleteExpired membersihkan token yang sudah lewat masa berlaku.
	DeleteExpired(ctx context.Context) error
}

type passwordResetRepository struct {
	db *gorm.DB
}

func NewPasswordResetRepository(db *gorm.DB) PasswordResetRepository {
	return &passwordResetRepository{db: db}
}

func (r *passwordResetRepository) Create(ctx context.Context, t *models.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *passwordResetRepository) FindValidByHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error) {
	var t models.PasswordResetToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *passwordResetRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.PasswordResetToken{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", time.Now()).Error
}

func (r *passwordResetRepository) InvalidateAllForUser(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.PasswordResetToken{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Update("used_at", time.Now()).Error
}

func (r *passwordResetRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&models.PasswordResetToken{}).Error
}
