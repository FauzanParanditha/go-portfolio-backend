package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/models"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type AdminDashboardHandler struct {
	repo repository.DashboardRepository
}

func NewAdminDashboardHandler(repo repository.DashboardRepository) *AdminDashboardHandler {
	return &AdminDashboardHandler{repo: repo}
}

func (h *AdminDashboardHandler) Overview(c *fiber.Ctx) error {
	recentDays := 30
	since := time.Now().AddDate(0, 0, -recentDays)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	counts, err := h.repo.Overview(ctx, since)
	if err != nil {
		log.Error().Err(err).Msg("failed to load dashboard overview")
		return fiber.NewError(http.StatusInternalServerError, "failed to load dashboard overview")
	}

	var resp models.DashboardOverviewResponse
	resp.System.ServerTime = time.Now()
	resp.System.RecentDays = recentDays

	resp.Projects.Total = counts.ProjectsTotal
	resp.Projects.Featured = counts.ProjectsFeatured
	resp.Projects.RecentCount = counts.ProjectsRecent

	resp.Experiences.Total = counts.ExperiencesTotal
	resp.Experiences.Current = counts.ExperiencesCurrent
	resp.Experiences.RecentCount = counts.ExperiencesRecent

	resp.ContactMessages.Total = counts.ContactTotal
	resp.ContactMessages.Unread = counts.ContactUnread
	resp.ContactMessages.RecentCount = counts.ContactRecent

	return c.Status(http.StatusOK).JSON(fiber.Map{"data": resp})
}
