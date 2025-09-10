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

func SubmitContact(c *gin.Context) {
	var input models.Contact

	// Bind JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
		return
	}

	// Validasi menggunakan validator
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

	if err := config.DB.Create(&input).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact message sent successfully"})
}
