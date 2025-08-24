package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/models"
)

// ModelsHandler handles model-related endpoints
type ModelsHandler struct {
	modelMgr *models.Manager
}

// NewModelsHandler creates a new models handler
func NewModelsHandler(modelMgr *models.Manager) *ModelsHandler {
	return &ModelsHandler{
		modelMgr: modelMgr,
	}
}

// ListModels handles GET /models
func (h *ModelsHandler) ListModels(c *gin.Context) {
	modelList, err := h.modelMgr.ListAvailableModels()
	if err != nil {
		log.Printf("Failed to get available models: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve models"})
		return
	}

	response := models.ModelsResponse{
		Models: modelList,
	}

	c.JSON(http.StatusOK, response)
}