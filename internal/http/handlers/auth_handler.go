package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	userRepo repository.UserRepository
	cfg      *config.Config
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		userRepo: repository.NewUserRepository(db),
		cfg:      cfg,
	}
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"tokenType"`
	ExpiresIn int    `json:"expiresIn"` // detik
}

type JWTCustomClaims struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// POST /api/v1/auth/login
// Login godoc
// @Summary      Login admin
// @Description  Authenticate admin and return JWT token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      LoginRequest  true  "Login data"
// @Success      200      {object}  LoginResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		fieldErrors := validation.ToFieldErrors(err)
		return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "validation failed",
				"code":    "VALIDATION_ERROR",
				"details": fieldErrors,
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		log.Warn().Err(err).Str("email", req.Email).Msg("login failed: user not found")
		// jangan bocorkan info yang terlalu detail
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		log.Warn().Err(err).Str("email", req.Email).Msg("login failed: wrong password")
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	// Generate JWT
	now := time.Now()
	exp := now.Add(time.Duration(h.cfg.JWTExpiresIn) * time.Second)

	claims := JWTCustomClaims{
		UserID: user.ID.String(),
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		log.Error().Err(err).Msg("failed to sign JWT")
		return fiber.NewError(http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(LoginResponse{
		Token:     signed,
		TokenType: "Bearer",
		ExpiresIn: h.cfg.JWTExpiresIn,
	})
}
