package repository

import (
	"context"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	// FindByID mengambil user berdasarkan id (tanpa preload). Meneruskan
	// gorm.ErrRecordNotFound apa adanya agar handler bisa memetakannya.
	FindByID(ctx context.Context, id string) (*models.User, error)
	// UpdatePassword mengganti hash password user. Nilai yang dikirim WAJIB
	// sudah berupa hash bcrypt — repository tidak melakukan hashing.
	UpdatePassword(ctx context.Context, id uuid.UUID, hashedPassword string) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) UpdatePassword(ctx context.Context, id uuid.UUID, hashedPassword string) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Update("password", hashedPassword).Error
}
