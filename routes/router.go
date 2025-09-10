package routes

import (
	"net/http"

	"github.com/FauzanParanditha/portfolio-backend/handlers"
	"github.com/FauzanParanditha/portfolio-backend/middlewares"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	// Root Endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Welcome to the portfolio API"})
	})

	// Middleware (urutan penting)
	r.Use(gin.Recovery())
	r.Use(middlewares.CORS())
	r.Use(middlewares.SecurityHeaders())
	r.Use(middlewares.RequestID())
	r.Use(middlewares.LimitBodySize(2 * 1024 * 1024)) // 2MB

	// Health
	r.GET("/health", handlers.HealthCheck)

	r.POST("/admin/login", handlers.AdminLogin)

	// API Grouping
	api := r.Group("/api")
	{
		v1 := api.Group("/v1")
		{
			admin := v1.Group("/admin")
			admin.POST("/refresh", handlers.AdminRefreshToken)
			// admin.POST("/logout", handlers.AdminLogout)
			admin.Use(middlewares.AdminAuth())
			{
				// Projects
				projects := admin.Group("/projects")
				{
					projects.GET("/", handlers.GetProjects)
					projects.POST("/", handlers.CreateProject)
				}

				// Experiences
				experiences := admin.Group("/experiences")
				{
					experiences.GET("/", handlers.GetExperiences)
					experiences.POST("/", handlers.CreateExperience)
				}

				// Contact
				contact := admin.Group("/contact")
				{
					contact.GET("/", handlers.SubmitContact)
				}
			}
		}
	}

	return r
}
