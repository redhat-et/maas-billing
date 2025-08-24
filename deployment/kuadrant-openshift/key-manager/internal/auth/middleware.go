package auth

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"net/http"
)

// AdminAuthMiddleware creates a middleware for admin authentication
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		adminKey := getEnvOrDefault("ADMIN_API_KEY", "")

		// If no admin key is set, allow access (backward compatibility)
		if adminKey == "" {
			c.Next()
			return
		}

		// Check Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Support both "Bearer" and "ADMIN" prefixes
		var providedKey string
		if strings.HasPrefix(authHeader, "Bearer ") {
			providedKey = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "ADMIN ") {
			providedKey = strings.TrimPrefix(authHeader, "ADMIN ")
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format. Use: Authorization: ADMIN <key>"})
			c.Abort()
			return
		}

		// Verify admin key
		if providedKey != adminKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid admin key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}