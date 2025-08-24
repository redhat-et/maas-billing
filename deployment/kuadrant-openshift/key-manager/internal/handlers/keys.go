package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/keys"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/teams"
)

// KeysHandler handles key-related endpoints
type KeysHandler struct {
	keyMgr  *keys.Manager
	teamMgr *teams.Manager
}

// NewKeysHandler creates a new keys handler
func NewKeysHandler(keyMgr *keys.Manager, teamMgr *teams.Manager) *KeysHandler {
	return &KeysHandler{
		keyMgr:  keyMgr,
		teamMgr: teamMgr,
	}
}

// CreateTeamKey handles POST /teams/:team_id/keys
func (h *KeysHandler) CreateTeamKey(c *gin.Context) {
	teamID := c.Param("team_id")
	var req keys.CreateTeamKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate team exists
	if !h.teamMgr.Exists(teamID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	response, err := h.keyMgr.CreateTeamKey(teamID, &req)
	if err != nil {
		log.Printf("Failed to create team key: %v", err)
		if strings.Contains(err.Error(), "already has an active API key") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		}
		return
	}

	log.Printf("Team API key created successfully for user %s in team %s", req.UserID, teamID)
	c.JSON(http.StatusOK, response)
}

// ListTeamKeys handles GET /teams/:team_id/keys
func (h *KeysHandler) ListTeamKeys(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	if !h.teamMgr.Exists(teamID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team details for response context
	team, err := h.teamMgr.Get(teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get detailed team API keys
	keys, err := h.keyMgr.ListTeamKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team keys: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get team keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"team_id":     teamID,
		"team_name":   team.TeamName,
		"policy":      team.Policy,
		"keys":        keys,
		"users":       team.Members,
		"total_keys":  len(keys),
		"total_users": len(team.Members),
	})
}

// DeleteTeamKey handles DELETE /keys/:key_name
func (h *KeysHandler) DeleteTeamKey(c *gin.Context) {
	keyName := c.Param("key_name")

	keyName, teamID, err := h.keyMgr.DeleteTeamKey(keyName)
	if err != nil {
		log.Printf("Failed to delete team key: %v", err)
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		} else if strings.Contains(err.Error(), "not associated with a team") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "API key is not associated with a team"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "API key deleted successfully",
		"key_name": keyName,
		"team_id":  teamID,
	})
}

// ListUserKeys handles GET /users/:user_id/keys
func (h *KeysHandler) ListUserKeys(c *gin.Context) {
	userID := c.Param("user_id")

	keys, err := h.keyMgr.ListUserKeys(userID)
	if err != nil {
		log.Printf("Failed to get user keys: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    userID,
		"keys":       keys,
		"total_keys": len(keys),
	})
}