package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type ExperienceHandler struct {
	repo repository.ExperienceRepository
}

func NewExperienceHandler(repo repository.ExperienceRepository) *ExperienceHandler {
	return &ExperienceHandler{repo: repo}
}

// GET /api/v1/experiences
// Public List Experiences godoc
// @Summary      Get experiences
// @Tags         experiences
// @Accept       json
// @Produce      json
// @Success      200  {array}  ExperienceResponse
// @Router       /experiences [get]
func (h *ExperienceHandler) List(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exps, err := h.repo.ListPublic(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list experiences (public)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch experiences")
	}

	resp := make([]ExperienceResponse, 0, len(exps))
	for _, e := range exps {
		resp = append(resp, experienceToResponse(e))
	}

	return c.JSON(fiber.Map{
		"data": resp,
	})
}
