package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/types"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/usage"
)

// UsageHandler handles usage-related endpoints
type UsageHandler struct {
	clientset    *kubernetes.Clientset
	config       *rest.Config
	keyNamespace string
	collector    *usage.Collector
}

// NewUsageHandler creates a new usage handler
func NewUsageHandler(clientset *kubernetes.Clientset, config *rest.Config, keyNamespace string) *UsageHandler {
	collector := usage.NewCollector(clientset, config, keyNamespace)
	
	return &UsageHandler{
		clientset:    clientset,
		config:       config,
		keyNamespace: keyNamespace,
		collector:    collector,
	}
}

// GetUserUsage handles GET /users/:user_id/usage
func (h *UsageHandler) GetUserUsage(c *gin.Context) {
	userID := c.Param("user_id")

	// Collect usage data
	userUsage, err := h.collector.GetUserUsage(userID)
	if err != nil {
		log.Printf("Failed to get user usage for %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to collect usage data"})
		return
	}

	// Enrich with team names and user emails from secrets
	err = h.enrichUserUsage(userUsage)
	if err != nil {
		log.Printf("Failed to enrich user usage data: %v", err)
		// Continue with basic data even if enrichment fails
	}

	c.JSON(http.StatusOK, userUsage)
}

// GetTeamUsage handles GET /teams/:team_id/usage (admin only)
func (h *UsageHandler) GetTeamUsage(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	teamSecret, err := h.clientset.CoreV1().Secrets(h.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team policy for metrics lookup
	policyName := teamSecret.Annotations["maas/policy"]
	if policyName == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Team has no policy configured"})
		return
	}

	// Collect usage data
	teamUsage, err := h.collector.GetTeamUsage(teamID, policyName)
	if err != nil {
		log.Printf("Failed to get team usage for %s: %v", teamID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to collect usage data"})
		return
	}

	// Enrich with team metadata
	teamUsage.TeamName = teamSecret.Annotations["maas/team-name"]

	// Enrich with user emails from secrets
	err = h.enrichTeamUsage(teamUsage)
	if err != nil {
		log.Printf("Failed to enrich team usage data: %v", err)
		// Continue with basic data even if enrichment fails
	}

	c.JSON(http.StatusOK, teamUsage)
}

// enrichUserUsage adds team names and other metadata to user usage
func (h *UsageHandler) enrichUserUsage(userUsage *types.UserUsage) error {
	// Get all team config secrets to map policies to teams
	labelSelector := "maas/resource-type=team-config"
	secrets, err := h.clientset.CoreV1().Secrets(h.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("failed to list team configs: %w", err)
	}

	// Create policy -> team mapping
	policyToTeam := make(map[string]struct{
		teamID   string
		teamName string
	})
	
	for _, secret := range secrets.Items {
		policy := secret.Annotations["maas/policy"]
		if policy != "" {
			policyToTeam[policy] = struct {
				teamID   string
				teamName string
			}{
				teamID:   secret.Labels["maas/team-id"],
				teamName: secret.Annotations["maas/team-name"],
			}
		}
	}

	// Enrich team breakdown with actual team info
	for i, teamUsage := range userUsage.TeamBreakdown {
		if teamInfo, exists := policyToTeam[teamUsage.Policy]; exists {
			userUsage.TeamBreakdown[i].TeamID = teamInfo.teamID
			userUsage.TeamBreakdown[i].TeamName = teamInfo.teamName
		}
	}

	return nil
}

// enrichTeamUsage adds user emails and other metadata to team usage
func (h *UsageHandler) enrichTeamUsage(teamUsage *types.TeamUsage) error {
	for i, userUsage := range teamUsage.UserBreakdown {
		// Find user's API key secret to get email
		labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s,maas/user-id=%s", 
			teamUsage.TeamID, userUsage.UserID)
		
		secrets, err := h.clientset.CoreV1().Secrets(h.keyNamespace).List(
			context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			log.Printf("Failed to get user secrets for %s: %v", userUsage.UserID, err)
			continue
		}

		if len(secrets.Items) > 0 {
			secret := secrets.Items[0]
			if email := secret.Annotations["maas/user-email"]; email != "" {
				teamUsage.UserBreakdown[i].UserEmail = email
			}
		}
	}

	return nil
}