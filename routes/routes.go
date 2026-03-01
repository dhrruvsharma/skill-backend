package routes

import (
	"github.com/dhrruvsharma/skill-charge-backend/handlers"
	"github.com/dhrruvsharma/skill-charge-backend/middleware"

	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine) {
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

}
