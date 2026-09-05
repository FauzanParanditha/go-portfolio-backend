package models

import (
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken adalah baris tabel `password_reset_tokens`.
//
// TokenHash menyimpan SHA-256 dari token mentah; token mentahnya hanya ada di
// email penerima dan tidak pernah dipersistensikan. Struct ini murni internal —
// tidak pernah diserialisasi ke response, karena itu tanpa tag json.
type PasswordResetToken struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index"`
	TokenHash string    `gorm:"uniqueIndex"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// TableName memaksa nama tabel ke `password_reset_tokens`.
func (PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}
