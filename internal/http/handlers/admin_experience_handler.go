package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/helpers"
	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type AdminExperienceHandler struct {
	db *gorm.DB
}

func NewAdminExperienceHandler(db *gorm.DB) *AdminExperienceHandler {
	return &AdminExperienceHandler{db: db}
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
	offset := (page - 1) * limit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	qb := h.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		Model(&models.Experience{})

	if q != "" {
		like := "%" + q + "%"
		qb = qb.Where(
			h.db.Where("experiences.title ILIKE ?", like).
				Or("experiences.company ILIKE ?", like),
		)
	}

	var total int64
	if err := qb.Count(&total).Error; err != nil {
		log.Error().Err(err).Msg("failed to count experiences (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch experiences")
	}

	var exps []models.Experience
	if err := qb.
		Order("experiences.sort_order ASC").
		Order("experiences.start_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&exps).Error; err != nil {

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
			Page:     page,
			Limit:    limit,
			Total:    total,
			HasMore:  hasMore,
			Query:    q,
			Featured: false, // nggak relevan di experiences, tapi field-nya ada
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

	var exp models.Experience
	if err := h.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		First(&exp, "id = ?", id).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(http.StatusNotFound, "experience not found")
		}

		log.Error().Err(err).Str("id", idStr).Msg("failed to get experience (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch experience")
	}

	return c.JSON(fiber.Map{
		"data": experienceToResponse(exp),
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

	tx := h.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	exp := models.Experience{
		Title:       req.Title,
		Company:     req.Company,
		Location:    req.Location,
		StartDate:   startDate,
		EndDate:     endDate,
		IsCurrent:   req.IsCurrent,
		Description: req.Description,
		SortOrder:   req.SortOrder,
	}

	// Tags
	if len(tagUUIDs) > 0 {
		var tags []models.Tag
		if err := tx.Where("id IN ?", tagUUIDs).Find(&tags).Error; err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to load tags for experience create")
			return fiber.NewError(http.StatusInternalServerError, "failed to load tags")
		}
		exp.Tags = tags
	}

	if err := tx.Create(&exp).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to create experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to create experience")
	}

	// Highlights
	if len(req.Highlights) > 0 {
		highs := make([]models.ExperienceHighlight, 0, len(req.Highlights))
		for i, text := range req.Highlights {
			if text == "" {
				continue
			}
			highs = append(highs, models.ExperienceHighlight{
				ExperienceID: exp.ID,
				Text:         text,
				SortOrder:    i,
			})
		}
		if len(highs) > 0 {
			if err := tx.Create(&highs).Error; err != nil {
				tx.Rollback()
				log.Error().Err(err).Msg("failed to create experience highlights")
				return fiber.NewError(http.StatusInternalServerError, "failed to create experience highlights")
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Error().Err(err).Msg("failed to commit experience create")
		return fiber.NewError(http.StatusInternalServerError, "failed to create experience")
	}

	// reload
	if err := h.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		First(&exp, "id = ?", exp.ID).Error; err != nil {

		log.Error().Err(err).Msg("failed to reload created experience")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"data": experienceToResponse(exp),
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

	tx := h.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var exp models.Experience
	if err := tx.First(&exp, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			tx.Rollback()
			return fiber.NewError(http.StatusNotFound, "experience not found")
		}
		tx.Rollback()
		log.Error().Err(err).Str("id", idStr).Msg("failed to load experience for update")
		return fiber.NewError(http.StatusInternalServerError, "failed to update experience")
	}

	// update fields
	exp.Title = req.Title
	exp.Company = req.Company
	exp.Location = req.Location
	exp.StartDate = startDate
	exp.EndDate = endDate
	exp.IsCurrent = req.IsCurrent
	exp.Description = req.Description
	exp.SortOrder = req.SortOrder

	if err := tx.Save(&exp).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to update experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to update experience")
	}

	// update tags
	if len(tagUUIDs) > 0 {
		var tags []models.Tag
		if err := tx.Where("id IN ?", tagUUIDs).Find(&tags).Error; err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to load tags for experience update")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
		if err := tx.Model(&exp).Association("Tags").Replace(&tags); err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to update experience tags")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
	} else {
		if err := tx.Model(&exp).Association("Tags").Clear(); err != nil {
			tx.Rollback()
			log.Error().Err(err).Msg("failed to clear experience tags")
			return fiber.NewError(http.StatusInternalServerError, "failed to update tags")
		}
	}

	// update highlights: delete + reinsert
	if err := tx.Where("experience_id = ?", exp.ID).Delete(&models.ExperienceHighlight{}).Error; err != nil {
		tx.Rollback()
		log.Error().Err(err).Msg("failed to delete old experience highlights")
		return fiber.NewError(http.StatusInternalServerError, "failed to update highlights")
	}

	if len(req.Highlights) > 0 {
		highs := make([]models.ExperienceHighlight, 0, len(req.Highlights))
		for i, text := range req.Highlights {
			if text == "" {
				continue
			}
			highs = append(highs, models.ExperienceHighlight{
				ExperienceID: exp.ID,
				Text:         text,
				SortOrder:    i,
			})
		}
		if len(highs) > 0 {
			if err := tx.Create(&highs).Error; err != nil {
				tx.Rollback()
				log.Error().Err(err).Msg("failed to create new experience highlights")
				return fiber.NewError(http.StatusInternalServerError, "failed to update highlights")
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Error().Err(err).Msg("failed to commit experience update")
		return fiber.NewError(http.StatusInternalServerError, "failed to update experience")
	}

	// reload
	if err := h.db.WithContext(ctx).
		Preload("Highlights", func(db *gorm.DB) *gorm.DB {
			return db.Order("experience_highlights.sort_order ASC")
		}).
		Preload("Tags").
		First(&exp, "id = ?", exp.ID).Error; err != nil {

		log.Error().Err(err).Msg("failed to reload updated experience")
	}

	return c.JSON(fiber.Map{
		"data": experienceToResponse(exp),
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

	if err := h.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Experience{}).Error; err != nil {

		log.Error().Err(err).Str("id", idStr).Msg("failed to delete experience")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete experience")
	}

	return c.SendStatus(http.StatusNoContent)
}
