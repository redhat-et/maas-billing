package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/keys"
)

// LegacyHandler handles legacy endpoint compatibility
type LegacyHandler struct {
	keyMgr *keys.Manager
}

// NewLegacyHandler creates a new legacy handler
func NewLegacyHandler(keyMgr *keys.Manager) *LegacyHandler {
	return &LegacyHandler{
		keyMgr: keyMgr,
	}
}

// GenerateKey handles POST /generate_key (legacy endpoint)
func (h *LegacyHandler) GenerateKey(c *gin.Context) {
	var req keys.GenerateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate user ID format (RFC 1123 subdomain rules)
	if !keys.ValidateUserID(req.UserID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "user_id must contain only lowercase alphanumeric characters and hyphens, start and end with alphanumeric character, and be 1-63 characters long",
		})
		return
	}

	response, err := h.keyMgr.CreateLegacyKey(&req)
	if err != nil {
		log.Printf("Failed to create legacy key: %v", err)
		if strings.Contains(err.Error(), "already has an active API key") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		}
		return
	}

	// Return legacy format response
	c.JSON(http.StatusOK, gin.H{
		"api_key": response.APIKey,
		"user_id": response.UserID,
	})
}

// DeleteKey handles DELETE /delete_key (legacy endpoint)
func (h *LegacyHandler) DeleteKey(c *gin.Context) {
	var req keys.DeleteKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	secretName, err := h.keyMgr.DeleteKey(req.Key)
	if err != nil {
		log.Printf("Failed to delete key: %v", err)
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "API key deleted successfully",
		"secret_name": secretName,
	})
}