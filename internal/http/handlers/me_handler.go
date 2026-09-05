package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type MeHandler struct {
	userRepo repository.UserRepository
}

func NewMeHandler(userRepo repository.UserRepository) *MeHandler {
	return &MeHandler{userRepo: userRepo}
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

	// AuthJWT hanya menaruh `user_id` + `user_role` di Locals (klaim JWT tidak
	// memuat name/email), jadi di alur normal cabang di bawah SELALU jatuh ke
	// query DB. Pembacaan Locals ini dipertahankan sebagai jalur cepat bila
	// suatu saat middleware ikut mengisi name/email.
	name, _ := c.Locals("user_name").(string)
	email, _ := c.Locals("user_email").(string)
	role, _ := c.Locals("user_role").(string)

	// Name/email tidak tersedia dari Locals → ambil dari DB.
	if email == "" || name == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		user, err := h.userRepo.FindByID(ctx, userID)
		if err != nil {
			log.Error().Err(err).Str("user_id", userID).Msg("failed to load current user")
			return fiber.NewError(http.StatusInternalServerError, "failed to fetch user")
		}

		return c.JSON(fiber.Map{
			"data": MeResponse{
				ID:    user.ID.String(),
				Name:  user.Name,
				Email: user.Email,
				Role:  user.Role,
			},
		})
	}

	return c.JSON(fiber.Map{
		"data": MeResponse{
			ID:    userID,
			Name:  name,
			Email: email,
			Role:  role,
		},
	})
}
