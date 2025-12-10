package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type AdminTagHandler struct {
	repo repository.TagRepository
}

func NewAdminTagHandler(repo repository.TagRepository) *AdminTagHandler {
	return &AdminTagHandler{repo: repo}
}

// GET /api/v1/admin/tags
// Admin List Tags godoc
// @Summary      List tags
// @Tags         admin-tags
// @Security     BearerAuth
// @Param        q     query  string false "Search"
// @Param        page  query  int    false "Page number"
// @Param        limit query  int    false "Page size"
// @Success      200  {array}  TagResponse
// @Failure      401  {object} ErrorResponse
// @Router       /admin/tags [get]
func (h *AdminTagHandler) List(c *fiber.Ctx) error {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := repository.TagListParams{
		Query: q,
		Page:  page,
		Limit: limit,
	}

	tags, total, err := h.repo.List(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tags")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch tags")
	}

	resp := make([]TagResponse, 0)
	for _, t := range tags {
		resp = append(resp, tagToResponse(t))
	}

	return c.JSON(fiber.Map{
		"data": resp,
		"meta": fiber.Map{
			"page":    page,
			"limit":   limit,
			"total":   total,
			"hasMore": int64(page*limit) < total,
			"q":       q,
		},
	})
}

// GET /api/v1/admin/tags/:id
// Admin Get Tag godoc
// @Summary      Get single tag
// @Tags         admin-tags
// @Security     BearerAuth
// @Param        id   path string true "Tag ID"
// @Success      200  {object} TagResponse
// @Failure      404  {object} ErrorResponse
// @Router       /admin/tags/{id} [get]
func (h *AdminTagHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return fiber.NewError(http.StatusNotFound, "tag not found")
	}

	return c.JSON(fiber.Map{
		"data": tagToResponse(*tag),
	})
}

// POST /api/v1/admin/tags
// Admin Create Tag godoc
// @Summary      Create new tag
// @Tags         admin-tags
// @Security     BearerAuth
// @Param        payload  body  TagCreateRequest  true  "Tag payload"
// @Success      201      {object} TagResponse
// @Failure      422      {object} ErrorResponse
// @Router       /admin/tags [post]
func (h *AdminTagHandler) Create(c *fiber.Ctx) error {
	var req TagCreateRequest
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

	tag := models.Tag{
		Name: req.Name,
		Type: req.Type,
	}

	if err := h.repo.Create(ctx, &tag); err != nil {
		log.Error().Err(err).Msg("failed to create tag")
		return fiber.NewError(http.StatusInternalServerError, "failed to create tag")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{"data": tagToResponse(tag)})
}

// PUT /api/v1/admin/tags/:id
// Admin Update Tag godoc
// @Summary      Update tag
// @Tags         admin-tags
// @Security     BearerAuth
// @Param        id      path string            true  "Tag ID"
// @Param        payload  body TagUpdateRequest true  "Tag payload"
// @Success      200     {object} TagResponse
// @Router       /admin/tags/{id} [put]
func (h *AdminTagHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid tag ID")
	}

	var req TagUpdateRequest
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

	tag, err := h.repo.GetByID(ctx, idStr)
	if err != nil {
		return fiber.NewError(http.StatusNotFound, "tag not found")
	}

	tag.Name = req.Name
	tag.Type = req.Type

	if err := h.repo.Update(ctx, tag); err != nil {
		log.Error().Err(err).Msg("failed to update tag")
		return fiber.NewError(http.StatusInternalServerError, "failed to update tag")
	}

	return c.JSON(fiber.Map{"data": tagToResponse(*tag)})
}

// DELETE /api/v1/admin/tags/:id
// Admin Delete Tag godoc
// @Summary      Delete tag
// @Tags         admin-tags
// @Security     BearerAuth
// @Param        id   path string true "Tag ID"
// @Success      204  "No Content"
// @Router       /admin/tags/{id} [delete]
func (h *AdminTagHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")

	_, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid tag ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.Delete(ctx, idStr); err != nil {
		log.Error().Err(err).Msg("failed to delete tag")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete tag")
	}

	return c.SendStatus(http.StatusNoContent)
}
