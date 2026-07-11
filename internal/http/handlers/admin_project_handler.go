package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/response"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ProjectUpdateRequest = ProjectCreateRequest

// Response meta untuk list admin
type PaginationMeta struct {
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	Total      int64  `json:"total"`
	TotalPages int64  `json:"totalPages"`
	HasMore    bool   `json:"hasMore"`
	Query      string `json:"q,omitempty"`
	Featured   bool   `json:"featured"`
}

type AdminProjectHandler struct {
	repo repository.ProjectRepository
}

func NewAdminProjectHandler(repo repository.ProjectRepository) *AdminProjectHandler {
	return &AdminProjectHandler{repo: repo}
}

// Helper: kirim response error validasi
func sendValidationError(c *fiber.Ctx, fieldErrors map[string]string) error {
	return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
		"error": fiber.Map{
			"message": "validation failed",
			"code":    "VALIDATION_ERROR",
			"details": fieldErrors,
		},
	})
}

// Helper: parse tag IDs string → []uuid.UUID
func parseTagIDs(ids []string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, 0, len(ids))
	for _, s := range ids {
		if s == "" {
			continue
		}
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, nil
}

// GET /api/v1/admin/projects
// Admin List Projects godoc
// @Summary      List all projects (admin)
// @Description  Admin-only list with search, pagination, featured filter
// @Tags         admin-projects
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        q         query  string false "Search keyword"
// @Param        featured  query  bool   false "Filter featured"
// @Param        page      query  int    false "Page"
// @Param        limit     query  int    false "Limit"
// @Success      200  {object}  ProjectsListResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /admin/projects [get]
func (h *AdminProjectHandler) List(c *fiber.Ctx) error {
	searchQ := c.Query("q")
	featured := c.Query("featured") == "true"

	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.Query("limit", "12"))
	if err != nil || limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	projects, total, err := h.repo.ListAdmin(ctx, repository.ProjectAdminListParams{
		Query:        searchQ,
		FeaturedOnly: featured,
		Page:         page,
		Limit:        limit,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list projects (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch projects")
	}

	resp := make([]ProjectResponse, 0, len(projects))
	for _, p := range projects {
		resp = append(resp, projectToResponse(p))
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
			Query:      searchQ,
			Featured:   featured,
		},
	})
}

// GET /api/v1/admin/projects/:id
// Admin Get Project godoc
// @Summary      Get project by ID
// @Tags         admin-projects
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path  string  true  "Project ID"
// @Success      200  {object}  ProjectResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /admin/projects/{id} [get]
func (h *AdminProjectHandler) GetByID(c *fiber.Ctx) error {
	idStr := c.Params("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid project ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	project, err := h.repo.GetByIDAdmin(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "project not found")
		}

		log.Error().Err(err).Str("id", idStr).Msg("failed to get project (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch project")
	}

	return c.JSON(fiber.Map{
		"data": projectToResponse(*project),
	})
}

// POST /api/v1/admin/projects
// Admin Create Project godoc
// @Summary      Create new project
// @Tags         admin-projects
// @Security     BearerAuth
// @Accept       json
// @Produce       json
// @Param        payload  body  ProjectCreateRequest  true  "Project payload"
// @Success      201      {object}  ProjectResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      422      {object}  ErrorResponse
// @Router       /admin/projects [post]
func (h *AdminProjectHandler) Create(c *fiber.Ctx) error {
	var req ProjectCreateRequest

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		fieldErrors := validation.ToFieldErrors(err)
		return sendValidationError(c, fieldErrors)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tagUUIDs, err := parseTagIDs(req.TagIDs)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid tagIds")
	}

	// marshal technicalDetails (map -> JSON)
	var technicalDetails datatypes.JSON
	if req.TechnicalDetails != nil {
		if b, err := json.Marshal(req.TechnicalDetails); err == nil {
			technicalDetails = datatypes.JSON(b)
		} else {
			log.Warn().Err(err).Msg("failed to marshal technicalDetails, ignoring")
		}
	}

	// Scalar dipetakan di handler; relasi (tags/features/screenshots) dikirim
	// mentah ke repository yang menjalankan transaksi.
	in := repository.ProjectWriteInput{
		Project: models.Project{
			Title:         req.Title,
			Slug:          req.Slug,
			ShortDesc:     req.ShortDesc,
			LongDesc:      req.LongDesc,
			CoverImageURL: req.CoverImageURL,

			Category: req.Category,
			Timeline: req.Timeline,
			Role:     req.Role,

			Challenge: req.Challenge,
			Solution:  req.Solution,

			Results: req.Results,

			TechnicalDetails: technicalDetails,

			DemoURL: req.DemoURL,
			RepoURL: req.RepoURL,

			IsFeatured: req.IsFeatured,
			SortOrder:  req.SortOrder,
		},
		TagIDs:      tagUUIDs,
		Features:    req.Features,
		Screenshots: req.Screenshots,
	}

	project, err := h.repo.Create(ctx, in)
	if err != nil {
		log.Error().Err(err).Msg("failed to create project")
		return fiber.NewError(http.StatusInternalServerError, "failed to create project")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": projectToResponse(*project),
	})
}

// PUT /api/v1/admin/projects/:id
// Admin Update Project godoc
// @Summary      Update project by ID
// @Tags         admin-projects
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path  string                true "Project ID"
// @Param        payload  body  ProjectUpdateRequest  true "Update payload"
// @Success      200      {object}  ProjectResponse
// @Failure      400      {object}  ErrorResponse
// @Router       /admin/projects/{id} [put]
func (h *AdminProjectHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid project ID")
	}

	var req ProjectUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&req); err != nil {
		fieldErrors := validation.ToFieldErrors(err)
		return sendValidationError(c, fieldErrors)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tagUUIDs, err := parseTagIDs(req.TagIDs)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid tagIds")
	}

	// marshal technicalDetails (map -> JSON)
	var technicalDetails datatypes.JSON
	if req.TechnicalDetails != nil {
		if b, err := json.Marshal(req.TechnicalDetails); err == nil {
			technicalDetails = datatypes.JSON(b)
		} else {
			log.Warn().Err(err).Msg("failed to marshal technicalDetails, ignoring")
		}
	}

	// TechnicalDetails hanya dikirim non-nil bila body memuatnya; repository
	// hanya menimpa kolom itu saat non-nil (mempertahankan JSON lama).
	in := repository.ProjectWriteInput{
		Project: models.Project{
			Title:         req.Title,
			Slug:          req.Slug,
			ShortDesc:     req.ShortDesc,
			LongDesc:      req.LongDesc,
			CoverImageURL: req.CoverImageURL,

			Category: req.Category,
			Timeline: req.Timeline,
			Role:     req.Role,

			Challenge: req.Challenge,
			Solution:  req.Solution,

			Results: req.Results,

			TechnicalDetails: technicalDetails,

			DemoURL: req.DemoURL,
			RepoURL: req.RepoURL,

			IsFeatured: req.IsFeatured,
			SortOrder:  req.SortOrder,
		},
		TagIDs:      tagUUIDs,
		Features:    req.Features,
		Screenshots: req.Screenshots,
	}

	project, err := h.repo.Update(ctx, id, in)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "project not found")
		}
		log.Error().Err(err).Str("id", idStr).Msg("failed to update project")
		return fiber.NewError(http.StatusInternalServerError, "failed to update project")
	}

	return c.JSON(fiber.Map{
		"data": projectToResponse(*project),
	})
}

// DELETE /api/v1/admin/projects/:id
// Admin Delete Project godoc
// @Summary      Delete project
// @Tags         admin-projects
// @Security     BearerAuth
// @Param        id   path  string  true "Project ID"
// @Success      204  "No Content"
// @Failure      404  {object}  ErrorResponse
// @Router       /admin/projects/{id} [delete]
func (h *AdminProjectHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid project ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.Delete(ctx, id); err != nil {
		log.Error().Err(err).Str("id", idStr).Msg("failed to delete project")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete project")
	}

	return c.SendStatus(http.StatusNoContent)
}
