package handlers

import (
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type AdminDashboardHandler struct {
	db *gorm.DB
}

func NewAdminDashboardHandler(db *gorm.DB) *AdminDashboardHandler {
	return &AdminDashboardHandler{db: db}
}

func (h *AdminDashboardHandler) Overview(c *fiber.Ctx) error {
	recentDays := 30
	since := time.Now().AddDate(0, 0, -recentDays)

	var resp models.DashboardOverviewResponse
	resp.System.ServerTime = time.Now()
	resp.System.RecentDays = recentDays

	// ===== Projects =====
	if err := h.db.Model(&models.Project{}).Count(&resp.Projects.Total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count projects"})
	}
	if err := h.db.Model(&models.Project{}).Where("is_featured = ?", true).Count(&resp.Projects.Featured).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count featured projects"})
	}
	if err := h.db.Model(&models.Project{}).Where("created_at >= ?", since).Count(&resp.Projects.RecentCount).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count recent projects"})
	}

	// ===== Experiences =====
	if err := h.db.Model(&models.Experience{}).Count(&resp.Experiences.Total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count experiences"})
	}
	if err := h.db.Model(&models.Experience{}).Where("is_current = ?", true).Count(&resp.Experiences.Current).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count current experiences"})
	}
	if err := h.db.Model(&models.Experience{}).Where("created_at >= ?", since).Count(&resp.Experiences.RecentCount).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count recent experiences"})
	}

	// ===== Contact Messages =====
	if err := h.db.Model(&models.ContactMessage{}).Count(&resp.ContactMessages.Total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count contact messages"})
	}
	if err := h.db.Model(&models.ContactMessage{}).Where("is_read = ?", false).Count(&resp.ContactMessages.Unread).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count unread messages"})
	}
	if err := h.db.Model(&models.ContactMessage{}).Where("created_at >= ?", since).Count(&resp.ContactMessages.RecentCount).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "failed to count recent messages"})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{"data": resp})
}
