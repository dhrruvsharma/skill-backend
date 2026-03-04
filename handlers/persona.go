package handlers

import (
	"errors"
	"net/http"

	"github.com/dhrruvsharma/skill-charge-backend/dto"
	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func CreatePersona(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		var req dto.CreatePersonaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}

		// Demote existing default before promoting the new one
		if req.IsDefault {
			db.Model(&models.Persona{}).
				Where("user_id = ? AND is_default = true", userID).
				Update("is_default", false)
		}

		isActive := true
		if req.IsActive != nil {
			isActive = *req.IsActive
		}

		durationMins := req.InterviewDurationMins
		if durationMins == 0 {
			durationMins = 30
		}

		domain := req.Domain
		if domain == "" {
			domain = models.DomainGeneral
		}

		difficulty := req.Difficulty
		if difficulty == "" {
			difficulty = models.DifficultyMedium
		}

		persona := models.Persona{
			UserID:                userID,
			Name:                  req.Name,
			Description:           req.Description,
			IsDefault:             req.IsDefault,
			IsActive:              isActive,
			TargetRole:            req.TargetRole,
			ExperienceYears:       req.ExperienceYears,
			Domain:                domain,
			Difficulty:            difficulty,
			Skills:                req.Skills,
			SystemPrompt:          req.SystemPrompt,
			InterviewDurationMins: durationMins,
			EnableVideoProctoring: req.EnableVideoProctoring,
			EnableAudioProctoring: req.EnableAudioProctoring,
			EnableTabDetection:    req.EnableTabDetection,
		}

		if err := db.Create(&persona).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create persona"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"data":    dto.FromPersona(persona),
		})
	}
}

func ListPersonas(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		var personas []models.Persona
		if err := db.Where("user_id = ?", userID).Find(&personas).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch personas"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    dto.FromPersonaList(personas),
		})
	}
}

func GetPersona(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		personaID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid persona id"})
			return
		}

		var persona models.Persona
		if err := db.Where("id = ? AND user_id = ?", personaID, userID).First(&persona).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "persona not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch persona"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    dto.FromPersona(persona),
		})
	}
}

func UpdatePersona(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		personaID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid persona id"})
			return
		}

		var req dto.UpdatePersonaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}

		var persona models.Persona
		if err := db.Where("id = ? AND user_id = ?", personaID, userID).First(&persona).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "persona not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch persona"})
			return
		}

		if req.IsDefault != nil && *req.IsDefault {
			db.Model(&models.Persona{}).
				Where("user_id = ? AND is_default = true AND id != ?", userID, personaID).
				Update("is_default", false)
		}

		updates := req.ToUpdateMap()
		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no fields provided to update"})
			return
		}

		if err := db.Model(&persona).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update persona"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    dto.FromPersona(persona),
		})
	}
}

func DeletePersona(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthenticated"})
			return
		}

		personaID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid persona id"})
			return
		}

		result := db.Where("id = ? AND user_id = ?", personaID, userID).Delete(&models.Persona{})
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete persona"})
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "persona not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "persona deleted successfully"})
	}
}
