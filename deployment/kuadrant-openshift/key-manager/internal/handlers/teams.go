package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/teams"
)

// TeamsHandler handles team-related endpoints
type TeamsHandler struct {
	teamMgr *teams.Manager
}

// NewTeamsHandler creates a new teams handler
func NewTeamsHandler(teamMgr *teams.Manager) *TeamsHandler {
	return &TeamsHandler{
		teamMgr: teamMgr,
	}
}

// CreateTeam handles POST /teams
func (h *TeamsHandler) CreateTeam(c *gin.Context) {
	var req teams.CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default policy if none specified
	if req.Policy == "" {
		req.Policy = "unlimited-policy"
	}

	err := h.teamMgr.Create(&req)
	if err != nil {
		log.Printf("Failed to create team: %v", err)
		if strings.Contains(err.Error(), "already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team"})
		}
		return
	}

	response := teams.CreateTeamResponse{
		TeamID:      req.TeamID,
		TeamName:    req.TeamName,
		Description: req.Description,
		Policy:      req.Policy,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	log.Printf("Team created successfully: %s (%s)", req.TeamID, req.TeamName)
	c.JSON(http.StatusOK, response)
}

// ListTeams handles GET /teams
func (h *TeamsHandler) ListTeams(c *gin.Context) {
	teams, err := h.teamMgr.List()
	if err != nil {
		log.Printf("Failed to list teams: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list teams"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"teams": teams, "total_teams": len(teams)})
}

// GetTeam handles GET /teams/:team_id
func (h *TeamsHandler) GetTeam(c *gin.Context) {
	teamID := c.Param("team_id")

	team, err := h.teamMgr.Get(teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Build response with additional metadata
	response := map[string]interface{}{
		"team_id":     team.TeamID,
		"team_name":   team.TeamName,
		"description": team.Description,
		"policy":      team.Policy,
		"users":       team.Members,
		"keys":        team.Keys,
		"created_at":  team.CreatedAt,
		"key_count":   len(team.Keys),
		"user_count":  len(team.Members),
	}

	c.JSON(http.StatusOK, response)
}

// UpdateTeam handles PATCH /teams/:team_id
func (h *TeamsHandler) UpdateTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	var req teams.UpdateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.teamMgr.Update(teamID, &req)
	if err != nil {
		log.Printf("Failed to update team %s: %v", teamID, err)
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		} else if strings.Contains(err.Error(), "does not exist") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update team"})
		}
		return
	}

	log.Printf("Team %s updated successfully", teamID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Team updated successfully",
		"team_id": teamID,
	})
}

// DeleteTeam handles DELETE /teams/:team_id
func (h *TeamsHandler) DeleteTeam(c *gin.Context) {
	teamID := c.Param("team_id")

	err := h.teamMgr.Delete(teamID)
	if err != nil {
		log.Printf("Failed to delete team %s: %v", teamID, err)
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete team"})
		}
		return
	}

	log.Printf("Team deleted successfully: %s", teamID)
	c.JSON(http.StatusOK, gin.H{"message": "Team deleted successfully", "team_id": teamID})
}