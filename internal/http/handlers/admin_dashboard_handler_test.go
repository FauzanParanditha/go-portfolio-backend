package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"gorm.io/gorm"
)

// TestAdminDashboardOverviewSuccess memastikan Overview membalas 200 dengan
// bentuk {data:{projects,experiences,contactMessages,system}} berisi count
// yang berasal dari repo.
func TestAdminDashboardOverviewSuccess(t *testing.T) {
	var gotSince time.Time
	repo := &fakeDashboardRepo{
		overviewFn: func(_ context.Context, since time.Time) (repository.DashboardCounts, error) {
			gotSince = since
			return repository.DashboardCounts{
				ProjectsTotal:      10,
				ProjectsFeatured:   3,
				ProjectsRecent:     2,
				ExperiencesTotal:   5,
				ExperiencesCurrent: 1,
				ExperiencesRecent:  1,
				ContactTotal:       7,
				ContactUnread:      4,
				ContactRecent:      6,
			}, nil
		},
	}

	app := newTestApp()
	app.Get("/admin/dashboard", handlers.NewAdminDashboardHandler(repo).Overview)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil))
	if status != http.StatusOK {
		t.Fatalf("status = %d, mau 200; body=%s", status, string(body))
	}

	// "since" harus di masa lalu (batas recent 30 hari).
	if !gotSince.Before(time.Now()) {
		t.Errorf("since harus di masa lalu, dapat: %v", gotSince)
	}

	data := decode(t, body)["data"].(map[string]any)

	projects := data["projects"].(map[string]any)
	if projects["total"].(float64) != 10 || projects["featured"].(float64) != 3 || projects["recentCount"].(float64) != 2 {
		t.Errorf("projects tidak sesuai: %v", projects)
	}

	exps := data["experiences"].(map[string]any)
	if exps["total"].(float64) != 5 || exps["current"].(float64) != 1 || exps["recentCount"].(float64) != 1 {
		t.Errorf("experiences tidak sesuai: %v", exps)
	}

	contact := data["contactMessages"].(map[string]any)
	if contact["total"].(float64) != 7 || contact["unread"].(float64) != 4 || contact["recentCount"].(float64) != 6 {
		t.Errorf("contactMessages tidak sesuai: %v", contact)
	}

	system := data["system"].(map[string]any)
	if system["recentDays"].(float64) != 30 {
		t.Errorf("system.recentDays = %v, mau 30", system["recentDays"])
	}
}

// TestAdminDashboardOverviewRepoError memastikan error repo dipetakan ke 500
// dengan envelope standar {error:{code:"SERVER_ERROR"}} (handler pakai fiber.NewError).
func TestAdminDashboardOverviewRepoError(t *testing.T) {
	repo := &fakeDashboardRepo{
		overviewFn: func(_ context.Context, _ time.Time) (repository.DashboardCounts, error) {
			return repository.DashboardCounts{}, gorm.ErrInvalidDB
		},
	}

	app := newTestApp()
	app.Get("/admin/dashboard", handlers.NewAdminDashboardHandler(repo).Overview)

	status, body := doJSON(t, app, httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500; body=%s", status, string(body))
	}
	errObj := decode(t, body)["error"].(map[string]any)
	if errObj["code"] != "SERVER_ERROR" {
		t.Errorf("error.code = %v, mau SERVER_ERROR", errObj["code"])
	}
}
