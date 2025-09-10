package handlers

import (
	"errors"
	"net/http"

	"github.com/FauzanParanditha/portfolio-backend/config"
	"github.com/FauzanParanditha/portfolio-backend/models"
	"github.com/FauzanParanditha/portfolio-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

var validate = validator.New()

func AdminLogin(c *gin.Context) {
	var input LoginInput

	// Lakukan bind JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		// Validasi manual jika bind gagal
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			errMap := make(map[string]string)
			for _, e := range ve {
				errMap[e.Field()] = utils.ValidationMessage(e)
			}
			c.JSON(http.StatusBadRequest, gin.H{"errors": errMap})
			return
		}

		// Fallback error message jika bukan validation error
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
		return
	}

	// Validasi manual (opsional karena binding sudah memverifikasi)
	if err := validate.Struct(input); err != nil {
		errs := err.(validator.ValidationErrors)
		errMap := make(map[string]string)
		for _, e := range errs {
			errMap[e.Field()] = utils.ValidationMessage(e)
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errMap})
		return
	}

	var admin models.Admin
	if err := config.DB.Where("email = ?", input.Email).First(&admin).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, err := utils.GenerateJWT(admin.Email, admin.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	refreshToken, err := utils.GenerateRefreshToken(admin.Email, admin.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":         token,
		"refresh_token": refreshToken,
		"admin": gin.H{
			"email":    admin.Email,
			"fullName": admin.FullName,
			"role":     admin.Role,
		},
	})
}

func AdminRefreshToken(c *gin.Context) {
	type RefreshInput struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}

	var input RefreshInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing refresh token"})
		return
	}

	token, err := utils.VerifyJWT(input.RefreshToken)
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	email := claims["email"].(string)
	role := claims["role"].(string)

	// Generate new access token
	newAccessToken, err := utils.GenerateJWT(email, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": newAccessToken,
	})
}
