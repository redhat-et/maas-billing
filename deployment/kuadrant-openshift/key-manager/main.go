package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KeyManager struct {
	clientset           *kubernetes.Clientset
	keyNamespace        string
	secretSelectorLabel string
	secretSelectorValue string
	discoveryRoute      string
}

type GenerateKeyRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type GenerateKeyResponse struct {
	APIKey     string `json:"api_key"`
	UserID     string `json:"user_id"`
	SecretName string `json:"secret_name"`
}

type DeleteKeyRequest struct {
	Key string `json:"key" binding:"required"`
}

type DiscoverEndpointResponse struct {
	Host     string `json:"host"`
	BasePath string `json:"base_path"`
}

func main() {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create in-cluster config: %v", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %v", err)
	}

	// Initialize KeyManager with environment variables
	km := &KeyManager{
		clientset:           clientset,
		keyNamespace:        getEnvOrDefault("KEY_NAMESPACE", "llm"),
		secretSelectorLabel: getEnvOrDefault("SECRET_SELECTOR_LABEL", "kuadrant.io/apikeys-by"),
		secretSelectorValue: getEnvOrDefault("SECRET_SELECTOR_VALUE", "rhcl-keys"),
		discoveryRoute:      getEnvOrDefault("DISCOVERY_ROUTE", "inference-route"),
	}

	// Initialize Gin router
	r := gin.Default()

	// Health check endpoint (no auth required)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Admin endpoints (require admin key)
	adminRoutes := r.Group("/", km.requireAdminAuth())
	adminRoutes.POST("/generate_key", km.generateKey)
	adminRoutes.DELETE("/delete_key", km.deleteKey)

	// OpenAI-compatible endpoints (require admin auth)
	v1 := adminRoutes.Group("/v1")
	v1.GET("/models", km.listModels)

	// Start server
	port := getEnvOrDefault("PORT", "8080")
	log.Printf("Starting key-manager on port %s", port)
	log.Fatal(r.Run(":" + port))
}

func (km *KeyManager) generateKey(c *gin.Context) {
	var req GenerateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate user ID format (RFC 1123 subdomain rules)
	if !isValidUserID(req.UserID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "user_id must contain only lowercase alphanumeric characters and hyphens, start and end with alphanumeric character, and be 1-63 characters long",
		})
		return
	}

	// Generate API key
	apiKey, err := generateSecureToken(48)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

	// Create SHA256 hash of the key
	hasher := sha256.New()
	hasher.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	// Create secret name (user ID is already validated)
	secretName := fmt.Sprintf("apikey-%s-%s", req.UserID, keyHash[:8])

	// Create Kubernetes Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: km.keyNamespace,
			Labels: map[string]string{
				"authorino.kuadrant.io/managed-by": "authorino",
				km.secretSelectorLabel:             km.secretSelectorValue,
				"maas/user-id":                     req.UserID,
				"maas/key-sha256":                  keyHash[:32], // Truncate to fit label limit
			},
			Annotations: map[string]string{
				"secret.kuadrant.io/user-id": req.UserID,
				"kuadrant.io/groups":         "free", // Default to free tier, TODO: sort out groups
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"api_key": apiKey,
		},
	}

	_, err = km.clientset.CoreV1().Secrets(km.keyNamespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key secret"})
		return
	}

	c.JSON(http.StatusOK, GenerateKeyResponse{
		APIKey:     apiKey,
		UserID:     req.UserID,
		SecretName: secretName,
	})
}

func (km *KeyManager) deleteKey(c *gin.Context) {
	var req DeleteKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create SHA256 hash of the provided key
	hasher := sha256.New()
	hasher.Write([]byte(req.Key))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	// Find and delete secret by label selector (use truncated hash)
	labelSelector := fmt.Sprintf("maas/key-sha256=%s", keyHash[:32])

	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		log.Printf("Failed to list secrets: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find API key"})
		return
	}

	if len(secrets.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	// Delete the secret
	secretName := secrets.Items[0].Name
	err = km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(context.Background(), secretName, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Failed to delete secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "API key deleted successfully",
		"secret_name": secretName,
	})
}

func (km *KeyManager) listModels(c *gin.Context) {
	// Return OpenAI-compatible models list
	// TODO: actual HTTPRoute resources from HTTPRoutes for admin persona and tie user model access to user secret
	// metadata. Groups should define the limits and model access?
	models := []gin.H{
		{
			"id":       "qwen3-0-6b-instruct",
			"object":   "model",
			"created":  1677610602,
			"owned_by": "qwen3",
		},
		{
			"id":       "simulator-model",
			"object":   "model",
			"created":  1677610602,
			"owned_by": "simulator",
		},
	}

	response := gin.H{
		"object": "list",
		"data":   models,
	}
	c.JSON(http.StatusOK, response)
}

func generateSecureToken(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64 URL-safe string
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// requireAdminAuth middleware to protect admin endpoints
func (km *KeyManager) requireAdminAuth() gin.HandlerFunc {
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

// isValidUserID validates user ID according to Kubernetes RFC 1123 subdomain rules
func isValidUserID(userID string) bool {
	// Must be 1-63 characters long
	if len(userID) == 0 || len(userID) > 63 {
		return false
	}

	// Must contain only lowercase alphanumeric characters and hyphens
	// Must start and end with an alphanumeric character
	validPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	return validPattern.MatchString(userID)
}
