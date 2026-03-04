package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/dhrruvsharma/skill-charge-backend/database"
	"github.com/dhrruvsharma/skill-charge-backend/dto"
	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/models"
	"github.com/dhrruvsharma/skill-charge-backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

type googleUserInfo struct {
	Sub       string `json:"sub"` // Google's unique user ID
	Email     string `json:"email"`
	FirstName string `json:"given_name"`
	LastName  string `json:"family_name"`
	Picture   string `json:"picture"`
	Verified  bool   `json:"email_verified"`
}

func Signup(c *gin.Context) {
	var req dto.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB

	// Check for duplicate email
	var existing models.User
	if err := db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "an account with this email already exists"})
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to process password"})
		return
	}

	// Generate 6-digit OTP
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate OTP"})
		return
	}
	otpExpiry := time.Now().Add(10 * time.Minute)
	hashedOTP := utils.HashToken(otp) // store hashed; compare hash on verify

	user := models.User{
		FirstName:                  req.FirstName,
		LastName:                   req.LastName,
		Email:                      req.Email,
		PasswordHash:               string(hash),
		Provider:                   models.ProviderLocal,
		Role:                       models.RoleUser,
		IsVerified:                 false,
		VerificationToken:          hashedOTP,
		VerificationTokenExpiresAt: &otpExpiry,
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create account"})
		return
	}

	// Send OTP email — fire and forget; don't fail the request if email is slow
	go func() {
		if err := utils.SendVerificationEmail(user.Email, user.FirstName, otp); err != nil {
			fmt.Printf("[WARN] failed to send verification email to %s: %v\n", user.Email, err)
		}
	}()

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "account created — please verify your email with the OTP sent to " + user.Email,
		"data":    true,
	})
}

func Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB
	var user models.User
	if err := db.Where("email = ? AND provider = ?", req.Email, models.ProviderLocal).First(&user).Error; err != nil {
		// Generic message to avoid user enumeration
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "invalid email or password"})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "this account has been deactivated"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "invalid email or password"})
		return
	}

	if !user.IsVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "email not verified",
			"code":    "EMAIL_NOT_VERIFIED",
		})
		return
	}

	tokens, err := issueTokenPair(c, &user)
	if err != nil {
		return // issueTokenPair already wrote the error response
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tokens})
}

func VerifyOTP(c *gin.Context) {
	var req dto.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB
	var user models.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "no account found with that email"})
		return
	}

	if user.IsVerified {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "email is already verified"})
		return
	}

	if user.VerificationToken == "" || user.VerificationTokenExpiresAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no pending verification — request a new OTP"})
		return
	}

	if time.Now().After(*user.VerificationTokenExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "OTP has expired — request a new one", "code": "OTP_EXPIRED"})
		return
	}

	if utils.HashToken(req.OTP) != user.VerificationToken {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid OTP"})
		return
	}

	// Mark verified and clear OTP fields
	if err := db.Model(&user).Updates(map[string]interface{}{
		"is_verified":                   true,
		"verification_token":            "",
		"verification_token_expires_at": nil,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to verify account"})
		return
	}

	tokens, err := issueTokenPair(c, &user)
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "email verified successfully", "data": tokens})
}

func ResendVerificationOTP(c *gin.Context) {
	var req dto.ResendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB
	var user models.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Return 200 deliberately — don't reveal if email exists
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "if an unverified account exists, a new OTP has been sent"})
		return
	}

	if user.IsVerified {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "this email is already verified"})
		return
	}

	if user.VerificationTokenExpiresAt != nil {
		remainingValidity := time.Until(*user.VerificationTokenExpiresAt)
		if remainingValidity > 9*time.Minute { // 10 min expiry means it was issued < 1 min ago
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "please wait before requesting another OTP",
			})
			return
		}
	}

	otp, err := utils.GenerateOTP(6)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate OTP"})
		return
	}
	otpExpiry := time.Now().Add(10 * time.Minute)

	db.Model(&user).Updates(map[string]interface{}{
		"verification_token":            utils.HashToken(otp),
		"verification_token_expires_at": otpExpiry,
	})

	go func() {
		if err := utils.SendVerificationEmail(user.Email, user.FirstName, otp); err != nil {
			fmt.Printf("[WARN] failed to resend OTP to %s: %v\n", user.Email, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "new OTP sent", "data": true})
}

func ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB
	var user models.User

	genericResp := gin.H{"success": true, "message": "if an account exists with that email, a reset link has been sent"}

	if err := db.Where("email = ? AND provider = ?", req.Email, models.ProviderLocal).First(&user).Error; err != nil {
		c.JSON(http.StatusOK, genericResp)
		return
	}

	token, err := utils.GenerateSecureToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate reset token"})
		return
	}
	expiry := time.Now().Add(1 * time.Hour)

	db.Model(&user).Updates(map[string]interface{}{
		"reset_token":            utils.HashToken(token),
		"reset_token_expires_at": expiry,
	})

	resetLink := fmt.Sprintf("%s/auth/reset-password?token=%s", os.Getenv("FRONTEND_URL"), token)

	go func() {
		if err := utils.SendPasswordResetEmail(user.Email, user.FirstName, resetLink); err != nil {
			fmt.Printf("[WARN] failed to send reset email to %s: %v\n", user.Email, err)
		}
	}()

	c.JSON(http.StatusOK, genericResp)
}

func ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db := database.DB
	hashedToken := utils.HashToken(req.Token)

	var user models.User
	if err := db.Where("reset_token = ?", hashedToken).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid or expired reset token"})
		return
	}

	if user.ResetTokenExpiresAt == nil || time.Now().After(*user.ResetTokenExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "reset token has expired — request a new one"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to process password"})
		return
	}

	if err := db.Model(&user).Updates(map[string]interface{}{
		"password_hash":          string(hash),
		"reset_token":            "",
		"reset_token_expires_at": nil,
		// Invalidate any existing refresh tokens so old sessions are logged out
		"refresh_token_hash": "",
		"refresh_token_exp":  nil,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "password reset successfully — please log in"})
}

func RefreshAccessToken(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	claims, err := middleware.ParseToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "invalid or expired refresh token"})
		return
	}

	if claims.TokenType != middleware.RefreshToken {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "token is not a refresh token"})
		return
	}

	db := database.DB
	var user models.User
	if err := db.First(&user, "id = ?", claims.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "user not found"})
		return
	}

	if utils.HashToken(req.RefreshToken) != user.RefreshTokenHash {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "refresh token has been revoked"})
		return
	}

	if user.RefreshTokenExp == nil || time.Now().After(*user.RefreshTokenExp) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "refresh token has expired"})
		return
	}

	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"access_token": accessToken}})
}

func GoogleLogin(c *gin.Context) {
	cfg := googleOAuthConfig()

	// Generate a cryptographically random state token
	stateToken, err := utils.GenerateSecureToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate state token"})
		return
	}

	// Store it in a short-lived, HttpOnly, SameSite=Lax cookie
	c.SetCookie(
		"oauth_state",
		stateToken,
		int((10 * time.Minute).Seconds()), // 10 min — plenty for the OAuth round-trip
		"/",
		"",                                   // domain — leave empty to default to current host
		os.Getenv("APP_ENV") == "production", // Secure flag: true in prod (HTTPS), false in dev
		true,                                 // HttpOnly — not accessible via JS
	)

	url := cfg.AuthCodeURL(stateToken, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func GoogleCallback(c *gin.Context) {
	var req dto.GoogleCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg := googleOAuthConfig()

	gToken, err := cfg.Exchange(context.Background(), req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "failed to exchange auth code with Google"})
		return
	}

	client := cfg.Client(context.Background(), gToken)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch user info from Google"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var gUser googleUserInfo
	if err := json.Unmarshal(body, &gUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to parse Google user info"})
		return
	}

	db := database.DB
	var user models.User

	err = db.Where("provider = ? AND provider_id = ?", models.ProviderGoogle, gUser.Sub).First(&user).Error
	if err != nil {
		err = db.Where("email = ?", gUser.Email).First(&user).Error
		if err == nil && user.Provider != models.ProviderGoogle {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "an account with this email already exists — please log in with your password",
			})
			return
		}

		user = models.User{
			FirstName:  gUser.FirstName,
			LastName:   gUser.LastName,
			Email:      gUser.Email,
			Provider:   models.ProviderGoogle,
			ProviderID: gUser.Sub,
			Role:       models.RoleUser,
			IsVerified: true,
			IsActive:   true,
		}
		if err := db.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create account"})
			return
		}
	} else {
		db.Model(&user).Update("avatar_url", gUser.Picture)
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "this account has been deactivated"})
		return
	}

	tokens, err := issueTokenPair(c, &user)
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tokens})
}

func issueTokenPair(c *gin.Context, user *models.User) (dto.AuthResponse, error) {
	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate access token"})
		return dto.AuthResponse{}, err
	}

	refreshToken, err := middleware.GenerateRefreshToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate refresh token"})
		return dto.AuthResponse{}, err
	}

	exp := time.Now().Add(7 * 24 * time.Hour)
	database.DB.Model(user).Updates(map[string]interface{}{
		"refresh_token_hash": utils.HashToken(refreshToken),
		"refresh_token_exp":  exp,
	})

	return dto.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserSummary{
			ID:         user.ID.String(),
			FirstName:  user.FirstName,
			LastName:   user.LastName,
			Email:      user.Email,
			Role:       string(user.Role),
			IsVerified: user.IsVerified,
		},
	}, nil
}
