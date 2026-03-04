package middleware

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type Claims struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

type contextKey string

const claimsKey contextKey = "claims"

var exemptRoutes = map[string]struct{}{
	"/api/v1/auth/signup":              {},
	"/api/v1/auth/login":               {},
	"/api/v1/auth/verify-email":        {},
	"/api/v1/auth/verify-otp":          {},
	"/api/v1/auth/resend-verification": {},
	"/api/v1/auth/forgot-password":     {},
	"/api/v1/auth/reset-password":      {},
	"/api/v1/auth/refresh":             {}, // uses refresh token, not access token
	"/api/v1/auth/google":              {},
	"/api/v1/auth/google/callback":     {},
}

func isExempt(path string) bool {
	_, ok := exemptRoutes[path]
	return ok
}

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET environment variable is not set")
	}
	return []byte(secret)
}

func GenerateAccessToken(userID uuid.UUID, email, role string) (string, error) {
	claims := Claims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenType: AccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(120 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "interview-ai",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func GenerateRefreshToken(userID uuid.UUID, email, role string) (string, error) {
	claims := Claims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenType: RefreshToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "interview-ai",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func ParseToken(raw string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExempt(c.FullPath()) {
			c.Next()
			return
		}

		raw, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "authorization header missing or malformed",
			})
			return
		}

		claims, err := ParseToken(raw)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "token has expired",
					"code":    "TOKEN_EXPIRED",
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "invalid token",
			})
			return
		}

		if claims.TokenType != AccessToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "refresh token cannot be used to access protected routes",
			})
			return
		}

		c.Set(string(claimsKey), claims)
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := GetClaims(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "unauthenticated",
			})
			return
		}
		if claims.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "you do not have permission to access this resource",
			})
			return
		}
		c.Next()
	}
}

func GetClaims(c *gin.Context) (*Claims, bool) {
	raw, exists := c.Get(string(claimsKey))
	if !exists {
		return nil, false
	}
	claims, ok := raw.(*Claims)
	return claims, ok
}

func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	claims, ok := GetClaims(c)
	if !ok {
		return uuid.Nil, false
	}
	return claims.UserID, true
}

func extractBearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("authorization header is empty")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", errors.New("authorization header must be in the format: Bearer <token>")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("token is empty")
	}
	return token, nil
}
