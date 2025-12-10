package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type ProjectUpdateRequest = ProjectCreateRequest

// Response meta untuk list admin
type PaginationMeta struct {
	Page     int    `json:"page"`
	Limit    int    `json:"limit"`
	Total    int64  `json:"total"`
	HasMore  bool   `json:"hasMore"`
	Query    string `json:"q,omitempty"`
	Featured bool   `json:"featured"`
}

type AdminProjectHandler struct {
	db *gorm.DB
}

func NewAdminProjectHandler(db *gorm.DB) *AdminProjectHandler {
	return &AdminProjectHandler{db: db}
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
	offset := (page - 1) * limit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := h.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		Model(&models.Project{})

	if searchQ != "" {
		like := "%" + searchQ + "%"
		q = q.Where(
			h.db.Where("projects.title ILIKE ?", like).
				Or("projects.short_desc ILIKE ?", like),
		)
	}

	if featured {
		q = q.Where("projects.is_featured = ?", true)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		log.Error().Err(err).Msg("failed to count projects (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch projects")
	}

	var projects []models.Project
	if err := q.
		Order("projects.sort_order ASC").
		Order("projects.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&projects).Error; err != nil {

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
			Page:     page,
			Limit:    limit,
			Total:    total,
			HasMore:  hasMore,
			Query:    searchQ,
			Featured: featured,
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

	var project models.Project
	if err := h.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		First(&project, "id = ?", id).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "project not found")
		}

		log.Error().Err(err).Str("id", idStr).Msg("failed to get project (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch project")
	}

	return c.JSON(fiber.Map{
		"data": projectToResponse(project),
	})
}

// POST /api/v1/admin/projects
// Admin Create Project godoc
// @Summary      Create new project
// @Tags         admin-projects
// @Security     BearerAuth
// @Accept       json
// @Produce      json
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

	tx := h.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	project := models.Project{
		Title:         req.Title,
		Slug:          req.Slug,
		ShortDesc:     req.ShortDesc,
		CoverImageURL: req.CoverImageURL,
		LiveURL:       req.LiveURL,
		SourceURL:     req.SourceURL,
		IsFeatured:    req.IsFeatured,
		SortOrder:     req.SortOrder,
	}

	// Handle tags (many-to-many)
	if len(tagUUIDs) > 0 {
		var tags []models.Tag
		if err := tx.Where("id IN ?", tagUUIDs).Find(&tags).Error; err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to load tags for project create")
			return fiber.NewError(http.StatusInternalServerError, "failed to load tags")
		}
		project.Tags = tags
	}

	if err := tx.Create(&project).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to create project")
		return fiber.NewError(http.StatusInternalServerError, "failed to create project")
	}

	// Features
	if len(req.Features) > 0 {
		features := make([]models.ProjectFeature, 0, len(req.Features))
		for i, text := range req.Features {
			if text == "" {
				continue
			}
			features = append(features, models.ProjectFeature{
				ProjectID: project.ID,
				Text:      text,
				SortOrder: i,
			})
		}
		if len(features) > 0 {
			if err := tx.Create(&features).Error; err != nil {
				tx.Rollback()
				log.Error().Err(err).Msg("failed to create project features")
				return fiber.NewError(http.StatusInternalServerError, "failed to create project features")
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Error().Err(err).Msg("failed to commit project create")
		return fiber.NewError(http.StatusInternalServerError, "failed to create project")
	}

	// reload with relations
	if err := h.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		First(&project, "id = ?", project.ID).Error; err != nil {

		log.Error().Err(err).Msg("failed to reload created project")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": projectToResponse(project),
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

	tx := h.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var project models.Project
	if err := tx.First(&project, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			tx.Rollback()
			return fiber.NewError(http.StatusNotFound, "project not found")
		}
		tx.Rollback()
		log.Error().Err(err).Str("id", idStr).Msg("failed to load project for update")
		return fiber.NewError(http.StatusInternalServerError, "failed to update project")
	}

	// Update scalar fields
	project.Title = req.Title
	project.Slug = req.Slug
	project.ShortDesc = req.ShortDesc
	project.CoverImageURL = req.CoverImageURL
	project.LiveURL = req.LiveURL
	project.SourceURL = req.SourceURL
	project.IsFeatured = req.IsFeatured
	project.SortOrder = req.SortOrder

	if err := tx.Save(&project).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to update project")
		return fiber.NewError(http.StatusInternalServerError, "failed to update project")
	}

	// Update tags
	if len(tagUUIDs) > 0 {
		var tags []models.Tag
		if err := tx.Where("id IN ?", tagUUIDs).Find(&tags).Error; err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to load tags for project update")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
		if err := tx.Model(&project).Association("Tags").Replace(&tags); err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to update project tags")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
	} else {
		// kalau tagIds kosong → kosongkan relasi
		if err := tx.Model(&project).Association("Tags").Clear(); err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to clear project tags")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
	}

	// Update features: hapus dulu, lalu insert baru
	if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectFeature{}).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to delete old project features")
		return fiber.NewError(http.StatusInternalServerError, "failed to update features")
	}

	if len(req.Features) > 0 {
		features := make([]models.ProjectFeature, 0, len(req.Features))
		for i, text := range req.Features {
			if text == "" {
				continue
			}
			features = append(features, models.ProjectFeature{
				ProjectID: project.ID,
				Text:      text,
				SortOrder: i,
			})
		}
		if len(features) > 0 {
			if err := tx.Create(&features).Error; err != nil {
				tx.Rollback()
				log.Error().Err(err).Msg("failed to create new project features")
				return fiber.NewError(http.StatusInternalServerError, "failed to update features")
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Error().Err(err).Msg("failed to commit project update")
		return fiber.NewError(http.StatusInternalServerError, "failed to update project")
	}

	// reload
	if err := h.db.WithContext(ctx).
		Preload("Features", func(db *gorm.DB) *gorm.DB {
			return db.Order("project_features.sort_order ASC")
		}).
		Preload("Tags").
		First(&project, "id = ?", project.ID).Error; err != nil {

		log.Error().Err(err).Msg("failed to reload updated project")
	}

	return c.JSON(fiber.Map{
		"data": projectToResponse(project),
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

	if err := h.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Project{}).Error; err != nil {

		log.Error().Err(err).Str("id", idStr).Msg("failed to delete project")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete project")
	}

	return c.SendStatus(http.StatusNoContent)
}
