package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type ContactHandler struct {
	repo repository.ContactMessageRepository
}

func NewContactHandler(repo repository.ContactMessageRepository) *ContactHandler {
	return &ContactHandler{repo: repo}
}

// POST /api/v1/contact
// Submit Contact Form godoc
// @Summary      Submit contact message
// @Tags         contact
// @Accept       json
// @Produce      json
// @Param        payload body ContactCreateRequest true "Contact form"
// @Success      201    {object} map[string]string
// @Failure      422    {object} ErrorResponse
// @Router       /contact [post]
func (h *ContactHandler) Create(c *fiber.Ctx) error {
	var req ContactCreateRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
			"error": fiber.Map{
				"message": "validation failed",
				"code":    "VALIDATION_ERROR",
				"details": validation.ToFieldErrors(err),
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := models.ContactMessage{
		Name:    req.Name,
		Email:   req.Email,
		Subject: req.Subject,
		Message: req.Message,
		IsRead:  false,
	}

	if err := h.repo.Create(ctx, &msg); err != nil {
		log.Error().Err(err).Msg("failed to create contact message")
		return fiber.NewError(http.StatusInternalServerError, "failed to submit message")
	}

	// Untuk public form, biasanya kita gak perlu balikin semua fields sensitif
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": fiber.Map{
			"id":      msg.ID.String(),
			"message": "message received",
		},
	})
}
