package routes

import (
	"github.com/dhrruvsharma/skill-charge-backend/handlers"
	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/services"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, db *gorm.DB, deepseekSvc *services.DeepseekService, voiceSvc *services.VoiceService, videoSvc *services.VideoService) {
	api := r.Group("/api/v1")

	api.Use(middleware.Authenticate())

	auth := api.Group("/auth")
	{
		// Local auth
		auth.POST("/signup", handlers.Signup)
		auth.POST("/login", handlers.Login)
		auth.POST("/verify-otp", handlers.VerifyOTP)
		auth.POST("/resend-verification", handlers.ResendVerificationOTP)
		auth.POST("/forgot-password", handlers.ForgotPassword)
		auth.POST("/reset-password", handlers.ResetPassword)
		auth.POST("/refresh", handlers.RefreshAccessToken)

		// Google OAuth
		auth.GET("/google", handlers.GoogleLogin)              // redirects to Google
		auth.POST("/google/callback", handlers.GoogleCallback) // receives code from frontend
	}

	personas := api.Group("/personas")
	{
		personas.POST("", handlers.CreatePersona(db))
		personas.GET("", handlers.ListPersonas(db))
		personas.GET("/:id", handlers.GetPersona(db))
		personas.PATCH("/:id", handlers.UpdatePersona(db))
		personas.DELETE("/:id", handlers.DeletePersona(db))
	}

	sessions := api.Group("/sessions")
	{
		sessions.POST("", handlers.StartSession(db, deepseekSvc))
		sessions.GET("", handlers.GetUserSessions(db))
		sessions.GET("/:id", handlers.GetSession(db))
		sessions.PATCH("/:id/end", handlers.EndSession(db, deepseekSvc))

		sessions.GET("/:id/messages", handlers.GetHistory(db))
		sessions.POST("/:id/messages", handlers.SendMessage(db, deepseekSvc))
		sessions.POST("/:id/voice", handlers.VoiceChat(db, deepseekSvc, voiceSvc))
		sessions.POST("/:id/video", handlers.VideoChat(db, deepseekSvc, voiceSvc, videoSvc))
	}
}
