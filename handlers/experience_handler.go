package handlers

import (
	"net/http"
	"time"

	"github.com/FauzanParanditha/portfolio-backend/config"
	"github.com/FauzanParanditha/portfolio-backend/models"
	"github.com/FauzanParanditha/portfolio-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

func GetExperiences(c *gin.Context) {
	var experiences []models.Experience

	if err := config.DB.Order("start_date desc").Find(&experiences).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch experiences"})
		return
	}

	c.JSON(http.StatusOK, experiences)
}

func CreateExperience(c *gin.Context) {
	var input models.Experience

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
		return
	}

	if err := validate.Struct(input); err != nil {
		errs := err.(validator.ValidationErrors)
		errMap := make(map[string]string)
		for _, e := range errs {
			errMap[e.Field()] = utils.ValidationMessage(e)
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errMap})
		return
	}

	input.ID = uuid.New()
	input.CreatedAt = time.Now()
	input.UpdatedAt = time.Now()

	if err := config.DB.Create(&input).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create experience"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Experience created successfully"})
}
