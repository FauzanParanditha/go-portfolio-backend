package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type ProjectHandler struct {
	repo repository.ProjectRepository
}

func NewProjectHandler(repo repository.ProjectRepository) *ProjectHandler {
	return &ProjectHandler{repo: repo}
}

// GET /api/v1/projects?featured=true&q=...&page=1&limit=12
// List Public Projects godoc
// @Summary      Get public projects
// @Description  List projects visible publicly with search & pagination
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        q         query    string false "Search keyword"
// @Param        featured  query    bool   false "Filter featured"
// @Param        page      query    int    false "Page number"
// @Param        limit     query    int    false "Items per page"
// @Success      200  {object}  ProjectsListResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /projects [get]
func (h *ProjectHandler) List(c *fiber.Ctx) error {
	featured := c.Query("featured") == "true"
	searchQ := c.Query("q")

	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.Query("limit", "12"))
	if err != nil || limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := repository.ProjectListParams{
		FeaturedOnly: featured,
		Query:        searchQ,
		Page:         page,
		Limit:        limit,
	}

	projects, total, err := h.repo.ListPublic(ctx, params)
	if err != nil {
		log.Error().
			Err(err).
			Bool("featured", featured).
			Str("q", searchQ).
			Int("page", page).
			Int("limit", limit).
			Msg("failed to list projects (public)")

		return fiber.NewError(http.StatusInternalServerError, "failed to fetch projects")
	}

	resp := make([]ProjectResponse, 0, len(projects))
	for _, p := range projects {
		resp = append(resp, projectToResponse(p))
	}

	hasMore := int64(page*limit) < total

	return c.JSON(fiber.Map{
		"data": resp,
		"meta": fiber.Map{
			"page":     page,
			"limit":    limit,
			"total":    total,
			"hasMore":  hasMore,
			"q":        searchQ,
			"featured": featured,
		},
	})
}

// GET /api/v1/projects/:slug
// Get Project By Slug godoc
// @Summary      Get project detail
// @Description  Get single public project by slug
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        slug   path  string  true  "Project slug"
// @Success      200    {object}  ProjectResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /projects/{slug} [get]
func (h *ProjectHandler) DetailBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	project, err := h.repo.GetBySlug(ctx, slug)
	if err != nil {
		if err.Error() == "record not found" {
			return fiber.NewError(http.StatusNotFound, "project not found")
		}

		log.Error().
			Err(err).
			Str("slug", slug).
			Msg("failed to get project by slug (public)")

		return fiber.NewError(http.StatusInternalServerError, "failed to fetch project")
	}

	return c.JSON(fiber.Map{
		"data": projectToResponse(*project),
	})
}
