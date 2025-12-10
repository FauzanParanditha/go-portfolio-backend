package http

import (
	"net/http"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	fiberSwagger "github.com/gofiber/swagger"
)

type AppDeps struct {
	DB     *gorm.DB
	Config *config.Config
}

func NewRouter(deps AppDeps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: NewErrorHandler(),
	})

	middleware.RegisterGlobal(app, deps.Config)

	// Swagger UI
	app.Get("/swagger/*", fiberSwagger.HandlerDefault)

	registerHealthRoutes(app, deps)

	registerAuthRoutes(app, deps)

	registerPublicProjectRoutes(app, deps)
	registerPublicExperienceRoutes(app, deps)
	registerPublicContactRoutes(app, deps)

	registerAdminProjectRoutes(app, deps)
	registerAdminTagRoutes(app, deps)
	registerAdminExperienceRoutes(app, deps)
	registerAdminContactRoutes(app, deps)

	return app
}

func registerHealthRoutes(app *fiber.App, deps AppDeps) {
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"status": "ok",
		})
	})

	app.Get("/readyz", func(c *fiber.Ctx) error {
		sqlDB, err := deps.DB.DB()
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"status": "db_error",
				"error":  "cannot get db handle",
			})
		}

		if err := sqlDB.Ping(); err != nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "db_unreachable",
			})
		}

		return c.Status(http.StatusOK).JSON(fiber.Map{
			"status": "ready",
		})
	})
}

// Public project routes (yang sebelumnya sudah ada)
func registerPublicProjectRoutes(app *fiber.App, deps AppDeps) {
	projectRepo := repository.NewProjectRepository(deps.DB)
	projectHandler := handlers.NewProjectHandler(projectRepo)

	api := app.Group("/api/v1")
	projects := api.Group("/projects")
	projects.Get("/", projectHandler.List)
	projects.Get("/:slug", projectHandler.DetailBySlug)
}

// Auth routes
func registerAuthRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	authHandler := handlers.NewAuthHandler(deps.DB, deps.Config)
	api.Post("/auth/login", authHandler.Login)
}

// Admin project routes
func registerAdminProjectRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config))

	adminProjectHandler := handlers.NewAdminProjectHandler(deps.DB)

	p := admin.Group("/projects")
	p.Get("/", adminProjectHandler.List)
	p.Get("/:id", adminProjectHandler.GetByID)
	p.Post("/", adminProjectHandler.Create)
	p.Put("/:id", adminProjectHandler.Update)
	p.Delete("/:id", adminProjectHandler.Delete)
}

// Admin tag routes
func registerAdminTagRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config))

	repo := repository.NewTagRepository(deps.DB)
	handler := handlers.NewAdminTagHandler(repo)

	t := admin.Group("/tags")
	t.Get("/", handler.List)
	t.Get("/:id", handler.GetByID)
	t.Post("/", handler.Create)
	t.Put("/:id", handler.Update)
	t.Delete("/:id", handler.Delete)
}

// Public experience route
func registerPublicExperienceRoutes(app *fiber.App, deps AppDeps) {
	expRepo := repository.NewExperienceRepository(deps.DB)
	expHandler := handlers.NewExperienceHandler(expRepo)

	api := app.Group("/api/v1")
	exps := api.Group("/experiences")
	exps.Get("/", expHandler.List)
}

// Admin experience route
func registerAdminExperienceRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config))

	handler := handlers.NewAdminExperienceHandler(deps.DB)

	e := admin.Group("/experiences")
	e.Get("/", handler.List)
	e.Get("/:id", handler.GetByID)
	e.Post("/", handler.Create)
	e.Put("/:id", handler.Update)
	e.Delete("/:id", handler.Delete)
}

// Public contact route
func registerPublicContactRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	contactRepo := repository.NewContactMessageRepository(deps.DB)
	contactHandler := handlers.NewContactHandler(contactRepo)

	api.Post("/contact", contactHandler.Create)
}

// Admin contact route
func registerAdminContactRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config))

	contactRepo := repository.NewContactMessageRepository(deps.DB)
	contactHandler := handlers.NewAdminContactHandler(deps.DB, contactRepo)

	g := admin.Group("/contact-messages")
	g.Get("/", contactHandler.List)
	g.Get("/:id", contactHandler.GetByID)
	g.Patch("/:id/read", contactHandler.MarkRead)
	g.Delete("/:id", contactHandler.Delete)
}
