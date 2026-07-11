package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/response"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type AdminExperienceHandler struct {
	repo repository.ExperienceRepository
}

func NewAdminExperienceHandler(repo repository.ExperienceRepository) *AdminExperienceHandler {
	return &AdminExperienceHandler{repo: repo}
}

// GET /api/v1/admin/experiences
// Admin List Experiences godoc
// @Summary      List all experiences
// @Tags         admin-experiences
// @Security     BearerAuth
// @Param        q     query string false "Search"
// @Param        page  query int    false "Page"
// @Param        limit query int    false "Limit"
// @Success      200  {array} ExperienceResponse
// @Router       /admin/experiences [get]
func (h *AdminExperienceHandler) List(c *fiber.Ctx) error {
	q := c.Query("q")

	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.Query("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exps, total, err := h.repo.ListAdmin(ctx, repository.ExperienceAdminListParams{
		Query: q,
		Page:  page,
		Limit: limit,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list experiences (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch experiences")
	}

	resp := make([]ExperienceResponse, 0, len(exps))
	for _, e := range exps {
		resp = append(resp, experienceToResponse(e))
	}

	hasMore := int64(page*limit) < total

	return c.JSON(fiber.Map{
		"data": resp,
		"meta": PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: response.TotalPages(total, limit),
			HasMore:    hasMore,
			Query:      q,
			Featured:   false, // nggak relevan di experiences, tapi field-nya ada
		},
	})
}

// GET /api/v1/admin/experiences/:id
// Admin Get Experience godoc
// @Summary      Get experience detail
// @Tags         admin-experiences
// @Security     BearerAuth
// @Param        id  path string true "ID"
// @Success      200 {object} ExperienceResponse
// @Router       /admin/experiences/{id} [get]
func (h *AdminExperienceHandler) GetByID(c *fiber.Ctx) error {
	idStr := c.Params("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid experience ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exp, err := h.repo.GetByIDAdmin(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "experience not found")
		}

		log.Error().Err(err).Str("id", idStr).Msg("failed to get experience (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch experience")
	}

	return c.JSON(fiber.Map{
		"data": experienceToResponse(*exp),
	})
}

// POST /api/v1/admin/experiences
// Admin Create Experience godoc
// @Summary      Create new experience
// @Tags         admin-experiences
// @Security     BearerAuth
// @Param        payload  body  ExperienceCreateRequest true "Experience payload"
// @Success      201      {object}  ExperienceResponse
// @Router       /admin/experiences [post]
func (h *AdminExperienceHandler) Create(c *fiber.Ctx) error {
	var req ExperienceCreateRequest

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

	startDate, err := helpers.ParseDateStr(req.StartDate)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid startDate format, expected YYYY-MM-DD")
	}

	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		t, err := helpers.ParseDateStr(*req.EndDate)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid endDate format, expected YYYY-MM-DD")
		}
		endDate = &t
	}

	tagUUIDs, err := parseTagIDs(req.TagIDs)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid tagIds")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := repository.ExperienceWriteInput{
		Experience: models.Experience{
			Title:       req.Title,
			Company:     req.Company,
			Location:    req.Location,
			StartDate:   startDate,
			EndDate:     endDate,
			IsCurrent:   req.IsCurrent,
			Description: req.Description,
			SortOrder:   req.SortOrder,
		},
		TagIDs:     tagUUIDs,
		Highlights: req.Highlights,
	}

	exp, err := h.repo.Create(ctx, in)
	if err != nil {
		log.Error().Err(err).Msg("failed to create experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to create experience")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": experienceToResponse(*exp),
	})
}

// PUT /api/v1/admin/experiences/:id
// Admin Update Experience godoc
// @Summary      Update experience
// @Tags         admin-experiences
// @Security     BearerAuth
// @Param        id      path string true "Experience ID"
// @Param        payload body ExperienceUpdateRequest true "Update payload"
// @Success      200     {object} ExperienceResponse
// @Router       /admin/experiences/{id} [put]
func (h *AdminExperienceHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid experience ID")
	}

	var req ExperienceUpdateRequest

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

	startDate, err := helpers.ParseDateStr(req.StartDate)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid startDate format, expected YYYY-MM-DD")
	}

	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		t, err := helpers.ParseDateStr(*req.EndDate)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid endDate format, expected YYYY-MM-DD")
		}
		endDate = &t
	}

	tagUUIDs, err := parseTagIDs(req.TagIDs)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid tagIds")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := repository.ExperienceWriteInput{
		Experience: models.Experience{
			Title:       req.Title,
			Company:     req.Company,
			Location:    req.Location,
			StartDate:   startDate,
			EndDate:     endDate,
			IsCurrent:   req.IsCurrent,
			Description: req.Description,
			SortOrder:   req.SortOrder,
		},
		TagIDs:     tagUUIDs,
		Highlights: req.Highlights,
	}

	exp, err := h.repo.Update(ctx, id, in)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "experience not found")
		}
		log.Error().Err(err).Msg("failed to update experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to update experience")
	}

	return c.JSON(fiber.Map{
		"data": experienceToResponse(*exp),
	})
}

// DELETE /api/v1/admin/experiences/:id
// Admin Delete Experience godoc
// @Summary      Delete experience
// @Tags         admin-experiences
// @Security     BearerAuth
// @Param        id   path string true "Experience ID"
// @Success      204  "No Content"
// @Router       /admin/experiences/{id} [delete]
func (h *AdminExperienceHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid experience ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.Delete(ctx, id); err != nil {
		log.Error().Err(err).Str("id", idStr).Msg("failed to delete experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete experience")
	}

	return c.SendStatus(http.StatusNoContent)
}
