package http

import (
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/internal/config"
	"github.com/FauzanParanditha/portfolio-backend/internal/denylist"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/handlers"
	"github.com/FauzanParanditha/portfolio-backend/internal/http/middleware"
	"github.com/FauzanParanditha/portfolio-backend/internal/mailer"
	"github.com/FauzanParanditha/portfolio-backend/internal/repository"
	"github.com/FauzanParanditha/portfolio-backend/internal/storage"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	fiberSwagger "github.com/gofiber/swagger"
)

type AppDeps struct {
	DB       *gorm.DB
	Config   *config.Config
	Denylist denylist.Denylist
	Mailer   mailer.Mailer
	Storage  storage.Storage
}

func NewRouter(deps AppDeps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: NewErrorHandler(),
		// Batas ini berlaku satu aplikasi penuh, jadi harus cukup untuk berkas
		// unggahan terbesar. Route selain unggahan tetap dijaga 1 MB oleh guard
		// di middleware.RegisterGlobal.
		BodyLimit: deps.Config.UploadMaxBytes + 1*1024*1024, // + slack multipart
	})

	// Denylist token (revocation saat logout). Satu instance dibagikan ke
	// middleware AuthJWT dan handler auth. Dibuat di sini bila belum di-inject.
	if deps.Denylist == nil {
		deps.Denylist = denylist.NewMemory()
	}

	// Mailer dipilih dari config (SMTP / log saat dev / dinonaktifkan di
	// produksi tanpa SMTP). Dibuat di sini bila belum di-inject.
	if deps.Mailer == nil {
		deps.Mailer = mailer.New(deps.Config)
	}

	// Penyimpanan berkas unggahan. Bila foldernya tidak bisa disiapkan, aplikasi
	// tetap jalan — hanya endpoint unggah yang tidak didaftarkan, sehingga
	// kegagalan terlihat sebagai 404 yang jelas, bukan panic saat runtime.
	if deps.Storage == nil {
		disk, err := storage.NewDisk(
			deps.Config.UploadDir,
			deps.Config.AppPublicURL,
			int64(deps.Config.UploadMaxBytes),
		)
		if err != nil {
			log.Error().Err(err).Msg("penyimpanan unggahan tidak aktif")
		} else {
			deps.Storage = disk
		}
	}

	middleware.RegisterGlobal(app, deps.Config)

	// Swagger UI hanya diaktifkan di luar produksi agar tidak membocorkan
	// seluruh permukaan API ke publik.
	if !deps.Config.IsProduction() {
		app.Get("/swagger/*", fiberSwagger.HandlerDefault)
	}

	registerHealthRoutes(app, deps)
	registerUploadRoutes(app, deps)

	registerAuthRoutes(app, deps)
	registerPasswordResetRoutes(app, deps)
	registerAuthMeRoutes(app, deps)

	registerPublicProjectRoutes(app, deps)
	registerPublicTagRoutes(app, deps)
	registerPublicExperienceRoutes(app, deps)
	registerPublicContactRoutes(app, deps)

	registerAdminDasbboardRoute(app, deps)
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

	userRepo := repository.NewUserRepository(deps.DB)
	authHandler := handlers.NewAuthHandler(userRepo, deps.Config, deps.Denylist)

	// Rate limit untuk mencegah brute-force / credential stuffing pada login.
	loginLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(http.StatusTooManyRequests, "too many login attempts, please try again later")
		},
	})

	api.Post("/auth/login", loginLimiter, authHandler.Login)

	// Refresh menerbitkan token baru dari token valid yang sedang dipakai.
	// Butuh auth (AuthJWT) tapi TIDAK pakai rate limiter login. Stateless.
	api.Post("/auth/refresh", middleware.AuthJWT(deps.Config, deps.Denylist), authHandler.Refresh)

	// Logout PUBLIK (tanpa auth) agar user dengan token kedaluwarsa tetap bisa
	// membersihkan cookie HttpOnly `access_token` di browser.
	api.Post("/auth/logout", authHandler.Logout)
}

// Upload routes: endpoint admin untuk mengunggah + penyajian berkasnya.
func registerUploadRoutes(app *fiber.App, deps AppDeps) {
	// Berkas dilayani statis dan PUBLIK — memang harus terbaca pengunjung situs.
	// Browse dimatikan agar isi folder tidak bisa didaftar orang lain.
	app.Static("/uploads", deps.Config.UploadDir, fiber.Static{
		Browse:    false,
		ByteRange: true,
		MaxAge:    int((24 * time.Hour).Seconds()),
	})

	// Unggahan sendiri tetap admin-only.
	api := app.Group("/api/v1")
	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

	if deps.Storage == nil {
		return
	}
	handler := handlers.NewAdminUploadHandler(deps.Storage)
	admin.Post("/uploads", handler.Upload)
}

// Password reset routes (publik, tanpa auth — user memang sedang tidak bisa login)
func registerPasswordResetRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	userRepo := repository.NewUserRepository(deps.DB)
	tokenRepo := repository.NewPasswordResetRepository(deps.DB)
	handler := handlers.NewPasswordResetHandler(userRepo, tokenRepo, deps.Mailer, deps.Config)

	// Lebih ketat daripada login: setiap request yang lolos MENGIRIM email, jadi
	// endpoint ini bisa disalahgunakan untuk membanjiri inbox orang lain.
	forgotLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(http.StatusTooManyRequests, "too many reset requests, please try again later")
		},
	})

	// Menahan brute-force penebakan token reset.
	resetLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(http.StatusTooManyRequests, "too many attempts, please try again later")
		},
	})

	api.Post("/auth/forgot-password", forgotLimiter, handler.ForgotPassword)
	api.Post("/auth/reset-password", resetLimiter, handler.ResetPassword)
}

// Admin project routes
func registerAdminProjectRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

	projectRepo := repository.NewProjectRepository(deps.DB)
	adminProjectHandler := handlers.NewAdminProjectHandler(projectRepo)

	p := admin.Group("/projects")
	p.Get("/", adminProjectHandler.List)
	p.Get("/:id", adminProjectHandler.GetByID)
	p.Post("/", adminProjectHandler.Create)
	p.Put("/:id", adminProjectHandler.Update)
	p.Delete("/:id", adminProjectHandler.Delete)
}

// Public tag route — dipakai situs publik untuk menyusun daftar keahlian.
// Read-only; pembuatan/perubahan tetap lewat grup admin di bawah.
func registerPublicTagRoutes(app *fiber.App, deps AppDeps) {
	repo := repository.NewTagRepository(deps.DB)
	handler := handlers.NewTagHandler(repo)

	api := app.Group("/api/v1")
	api.Get("/tags", handler.List)
}

// Admin tag routes
func registerAdminTagRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

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
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

	expRepo := repository.NewExperienceRepository(deps.DB)
	handler := handlers.NewAdminExperienceHandler(expRepo)

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
	// Alias publik untuk konsistensi penamaan dengan resource admin/contact-messages.
	// Menunjuk ke handler yang SAMA.
	api.Post("/contact-messages", contactHandler.Create)
}

// Admin contact route
func registerAdminContactRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

	contactRepo := repository.NewContactMessageRepository(deps.DB)
	contactHandler := handlers.NewAdminContactHandler(contactRepo)

	g := admin.Group("/contact-messages")
	g.Get("/", contactHandler.List)
	g.Get("/:id", contactHandler.GetByID)
	g.Patch("/:id/read", contactHandler.MarkRead)
	g.Delete("/:id", contactHandler.Delete)
}

func registerAuthMeRoutes(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	userRepo := repository.NewUserRepository(deps.DB)
	meHandler := handlers.NewMeHandler(userRepo)

	// wajib auth
	api.Get("/me", middleware.AuthJWT(deps.Config, deps.Denylist), meHandler.Me)
}

func registerAdminDasbboardRoute(app *fiber.App, deps AppDeps) {
	api := app.Group("/api/v1")

	admin := api.Group("/admin")
	admin.Use(middleware.AuthJWT(deps.Config, deps.Denylist))
	admin.Use(middleware.RequireRole("admin"))

	dashboardRepo := repository.NewDashboardRepository(deps.DB)
	dashboardHandler := handlers.NewAdminDashboardHandler(dashboardRepo)
	admin.Get("/dashboard/overview", dashboardHandler.Overview)
}
