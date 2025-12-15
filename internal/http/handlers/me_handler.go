package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type MeHandler struct {
	db *gorm.DB
}

func NewMeHandler(db *gorm.DB) *MeHandler {
	return &MeHandler{db: db}
}

type MeResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// Me godoc
// @Summary      Get current user
// @Description  Return authenticated admin user info from JWT
// @Tags         auth
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  MeResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /me [get]
func (h *MeHandler) Me(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	// kamu bisa langsung return tanpa query DB.
	name, _ := c.Locals("user_name").(string)
	email, _ := c.Locals("user_email").(string)
	role, _ := c.Locals("user_role").(string)

	// Jika name/email kosong (middleware belum set lengkap), fallback ke DB
	if email == "" || name == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var user models.User
		if err := h.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
			log.Error().Err(err).Str("user_id", userID).Msg("failed to load current user")
			return fiber.NewError(http.StatusInternalServerError, "failed to fetch user")
		}

		return c.JSON(MeResponse{
			ID:    user.ID.String(),
			Name:  user.Name,
			Email: user.Email,
			Role:  user.Role, // kalau tidak ada, hapus field ini
		})
	}

	return c.JSON(MeResponse{
		ID:    userID,
		Name:  name,
		Email: email,
		Role:  role,
	})
}
