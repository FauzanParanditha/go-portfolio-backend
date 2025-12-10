package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type AdminContactHandler struct {
	repo repository.ContactMessageRepository
	db   *gorm.DB // opsional, tapi aku keep konsisten dengan handler lain yang pakai db
}

func NewAdminContactHandler(db *gorm.DB, repo repository.ContactMessageRepository) *AdminContactHandler {
	return &AdminContactHandler{
		repo: repo,
		db:   db,
	}
}

// GET /api/v1/admin/contact-messages
// Admin List Contact Messages godoc
// @Summary      View inbox messages
// @Tags         admin-contact
// @Security     BearerAuth
// @Param        q      query string false "Search"
// @Param        isRead query bool   false "Filter read/unread"
// @Param        page   query int    false "Page"
// @Param        limit  query int    false "Limit"
// @Success      200  {array} ContactMessageResponse
// @Router       /admin/contact-messages [get]
func (h *AdminContactHandler) List(c *fiber.Ctx) error {
	q := c.Query("q")
	isReadStr := c.Query("isRead") // "true", "false", atau kosong

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

	var isRead *bool
	if isReadStr == "true" {
		v := true
		isRead = &v
	} else if isReadStr == "false" {
		v := false
		isRead = &v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := repository.ContactListParams{
		Query:  q,
		IsRead: isRead,
		Page:   page,
		Limit:  limit,
	}

	msgs, total, err := h.repo.List(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("failed to list contact messages (admin)")
		return fiber.NewError(http.StatusInternalServerError, "failed to fetch contact messages")
	}

	resp := make([]ContactMessageResponse, 0, len(msgs))
	for _, m := range msgs {
		resp = append(resp, contactToResponse(m))
	}

	hasMore := int64(page*limit) < total

	return c.JSON(fiber.Map{
		"data": resp,
		"meta": fiber.Map{
			"page":    page,
			"limit":   limit,
			"total":   total,
			"hasMore": hasMore,
			"q":       q,
			"isRead":  isReadStr,
		},
	})
}

// GET /api/v1/admin/contact-messages/:id
// Admin Get Message godoc
// @Summary      Get message detail
// @Tags         admin-contact
// @Security     BearerAuth
// @Param        id   path string true "Message ID"
// @Success      200  {object} ContactMessageResponse
// @Router       /admin/contact-messages/{id} [get]
func (h *AdminContactHandler) GetByID(c *fiber.Ctx) error {
	idStr := c.Params("id")

	if _, err := uuid.Parse(idStr); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid message ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := h.repo.GetByID(ctx, idStr)
	if err != nil {
		log.Error().Err(err).Str("id", idStr).Msg("failed to get contact message")
		return fiber.NewError(http.StatusNotFound, "message not found")
	}

	return c.JSON(fiber.Map{
		"data": contactToResponse(*msg),
	})
}

// PATCH /api/v1/admin/contact-messages/:id/read
// Admin Mark Message Read godoc
// @Summary      Mark contact message read/unread
// @Tags         admin-contact
// @Security     BearerAuth
// @Param        id      path string                 true "Message ID"
// @Param        payload body  map[string]bool true  "isRead flag"
// @Success      204     "No Content"
// @Router       /admin/contact-messages/{id}/read [patch]
func (h *AdminContactHandler) MarkRead(c *fiber.Ctx) error {
	idStr := c.Params("id")
	if _, err := uuid.Parse(idStr); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid message ID")
	}

	var payload struct {
		IsRead bool `json:"isRead" validate:"required"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := validation.ValidateStruct(&payload); err != nil {
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

	if err := h.repo.MarkRead(ctx, idStr, payload.IsRead); err != nil {
		log.Error().Err(err).Str("id", idStr).Msg("failed to mark message read/unread")
		return fiber.NewError(http.StatusInternalServerError, "failed to update message status")
	}

	return c.SendStatus(http.StatusNoContent)
}

// DELETE /api/v1/admin/contact-messages/:id
// Admin Delete Contact Message godoc
// @Summary      Delete message
// @Tags         admin-contact
// @Security     BearerAuth
// @Param        id   path string true "Message ID"
// @Success      204 "No Content"
// @Router       /admin/contact-messages/{id} [delete]
func (h *AdminContactHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	if _, err := uuid.Parse(idStr); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid message ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.Delete(ctx, idStr); err != nil {
		log.Error().Err(err).Str("id", idStr).Msg("failed to delete contact message")
		return fiber.NewError(http.StatusInternalServerError, "failed to delete message")
	}

	return c.SendStatus(http.StatusNoContent)
}
