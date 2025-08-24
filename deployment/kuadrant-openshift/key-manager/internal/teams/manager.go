package teams

import (
	"context"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Manager handles team operations
type Manager struct {
	clientset    *kubernetes.Clientset
	keyNamespace string
	policyMgr    *PolicyManager
}

// NewManager creates a new team manager
func NewManager(clientset *kubernetes.Clientset, keyNamespace string, policyMgr *PolicyManager) *Manager {
	return &Manager{
		clientset:    clientset,
		keyNamespace: keyNamespace,
		policyMgr:    policyMgr,
	}
}

// Create creates a new team with policy integration
func (m *Manager) Create(req *CreateTeamRequest) error {
	// Validate team data
	if err := m.validateTeamRequest(req); err != nil {
		return fmt.Errorf("team validation failed: %w", err)
	}

	// Check if team already exists
	existingSecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", req.TeamID), metav1.GetOptions{})
	if err == nil && existingSecret != nil {
		return fmt.Errorf("team %s already exists", req.TeamID)
	}

	// Create team configuration secret
	_, err = m.createTeamConfigSecret(req)
	if err != nil {
		return fmt.Errorf("failed to create team secret: %w", err)
	}

	// Update policies via PolicyManager
	if m.policyMgr != nil {
		err = m.policyMgr.AddTeamToAuthPolicy(req.Policy)
		if err != nil {
			log.Printf("Warning: Failed to update AuthPolicy for team %s: %v", req.TeamID, err)
		}

		err = m.policyMgr.AddTeamToTokenRateLimit(req.Policy, req.TokenLimit, req.TimeWindow)
		if err != nil {
			log.Printf("Warning: Failed to update TokenRateLimitPolicy for team %s: %v", req.TeamID, err)
		}

		err = m.policyMgr.RestartKuadrantComponents()
		if err != nil {
			log.Printf("Warning: Failed to restart Kuadrant components for team %s: %v", req.TeamID, err)
		}
	}

	log.Printf("Team %s created with policy reference: %s", req.TeamID, req.Policy)
	return nil
}

// Get retrieves team details
func (m *Manager) Get(teamID string) (*GetTeamResponse, error) {
	// Get team config secret
	teamSecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}

	// Get team members from API keys
	members, err := m.getTeamMembersFromAPIKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team members: %v", err)
		members = []TeamMember{}
	}

	// Get simple key names
	keys, err := m.getTeamAPIKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team key names: %v", err)
		keys = []string{}
	}

	return &GetTeamResponse{
		TeamID:      teamID,
		TeamName:    teamSecret.Annotations["maas/team-name"],
		Description: teamSecret.Annotations["maas/description"],
		Policy:      teamSecret.Annotations["maas/policy"],
		Members:     members,
		Keys:        keys,
		CreatedAt:   teamSecret.Annotations["maas/created-at"],
	}, nil
}

// List retrieves all teams
func (m *Manager) List() ([]map[string]interface{}, error) {
	labelSelector := "maas/resource-type=team-config"
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list team secrets: %w", err)
	}

	teams := make([]map[string]interface{}, 0)
	for _, secret := range secrets.Items {
		teamID := secret.Labels["maas/team-id"]
		
		// Get team key count
		keyCount := 0
		userCount := 0
		if keys, err := m.getTeamAPIKeys(teamID); err == nil {
			keyCount = len(keys)
		}
		if members, err := m.getTeamMembersFromAPIKeys(teamID); err == nil {
			userCount = len(members)
		}

		team := map[string]interface{}{
			"team_id":    secret.Labels["maas/team-id"],
			"team_name":  secret.Annotations["maas/team-name"],
			"description": secret.Annotations["maas/description"],
			"policy":     secret.Annotations["maas/policy"],
			"created_at": secret.Annotations["maas/created-at"],
			"key_count":  keyCount,
			"user_count": userCount,
		}
		teams = append(teams, team)
	}

	return teams, nil
}

// Update performs partial updates on team configuration
func (m *Manager) Update(teamID string, req *UpdateTeamRequest) error {
	// Get current team config secret
	teamSecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// Store original policy for comparison
	originalPolicy := teamSecret.Annotations["maas/policy"]

	// Update annotations with new values (only if provided)
	if req.TeamName != nil {
		teamSecret.Annotations["maas/team-name"] = *req.TeamName
	}
	if req.Description != nil {
		teamSecret.Annotations["maas/description"] = *req.Description
	}
	if req.Policy != nil {
		teamSecret.Annotations["maas/policy"] = *req.Policy
	}

	// Update team secret
	_, err = m.clientset.CoreV1().Secrets(m.keyNamespace).Update(
		context.Background(), teamSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}

	// Handle policy changes via PolicyManager
	if m.policyMgr != nil {
		if req.Policy != nil && *req.Policy != originalPolicy {
			// Validate new policy exists
			if !m.policyMgr.PolicyExists(*req.Policy) {
				return fmt.Errorf("policy '%s' does not exist in TokenRateLimitPolicy", *req.Policy)
			}

			// Remove old policy
			if originalPolicy != "" {
				err = m.policyMgr.RemoveTeamFromAuthPolicy(originalPolicy)
				if err != nil {
					log.Printf("Warning: Failed to remove old AuthPolicy group %s: %v", originalPolicy, err)
				}

				err = m.policyMgr.RemoveTeamFromTokenRateLimit(originalPolicy)
				if err != nil {
					log.Printf("Warning: Failed to remove old TokenRateLimitPolicy group %s: %v", originalPolicy, err)
				}
			}

			// Add new policy
			existingTokenLimit, existingTimeWindow, err := m.policyMgr.GetPolicyLimits(*req.Policy)
			if err != nil {
				return fmt.Errorf("failed to get policy limits: %w", err)
			}

			err = m.policyMgr.AddTeamToAuthPolicy(*req.Policy)
			if err != nil {
				log.Printf("Warning: Failed to update AuthPolicy for new policy %s: %v", *req.Policy, err)
			}

			err = m.policyMgr.AddTeamToTokenRateLimit(*req.Policy, existingTokenLimit, existingTimeWindow)
			if err != nil {
				log.Printf("Warning: Failed to update TokenRateLimitPolicy for new policy %s: %v", *req.Policy, err)
			}

			// Restart components and update team keys
			err = m.policyMgr.RestartKuadrantComponents()
			if err != nil {
				log.Printf("Warning: Failed to restart Kuadrant components: %v", err)
			}

			err = m.updateTeamKeysPolicy(teamID, *req.Policy)
			if err != nil {
				log.Printf("Warning: Failed to update team keys policy: %v", err)
			}
		} else if (req.TokenLimit != nil || req.TimeWindow != nil) && originalPolicy != "" {
			// Update token limits for existing policy
			currentTokenLimit, currentTimeWindow, err := m.policyMgr.GetPolicyLimits(originalPolicy)
			if err != nil {
				return fmt.Errorf("policy '%s' does not exist in TokenRateLimitPolicy", originalPolicy)
			}

			// Use existing values as defaults, override only what's specified
			tokenLimit := currentTokenLimit
			timeWindow := currentTimeWindow
			
			if req.TokenLimit != nil {
				tokenLimit = *req.TokenLimit
			}
			if req.TimeWindow != nil {
				timeWindow = *req.TimeWindow
			}

			err = m.policyMgr.AddTeamToTokenRateLimit(originalPolicy, tokenLimit, timeWindow)
			if err != nil {
				log.Printf("Warning: Failed to update TokenRateLimitPolicy limits: %v", err)
			}

			err = m.policyMgr.RestartKuadrantComponents()
			if err != nil {
				log.Printf("Warning: Failed to restart Kuadrant components: %v", err)
			}
		}
	}

	log.Printf("Team %s updated successfully", teamID)
	return nil
}

// Delete removes team and all associated resources
func (m *Manager) Delete(teamID string) error {
	// Check if team exists
	teamSecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// Get team policy before deletion for cleanup
	teamPolicy := teamSecret.Annotations["maas/policy"]

	// Update TokenRateLimitPolicy to remove the team's policy
	if m.policyMgr != nil {
		err = m.policyMgr.RemoveTeamFromTokenRateLimit(teamPolicy)
		if err != nil {
			log.Printf("Warning: Failed to update TokenRateLimitPolicy for team deletion %s: %v", teamID, err)
		}
	}

	// Delete all team API keys
	err = m.deleteAllTeamKeys(teamID)
	if err != nil {
		log.Printf("Failed to delete team keys: %v", err)
	}

	// Delete team configuration secret
	err = m.clientset.CoreV1().Secrets(m.keyNamespace).Delete(
		context.Background(), teamSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	log.Printf("Team deleted successfully: %s", teamID)
	return nil
}

// Exists checks if a team exists
func (m *Manager) Exists(teamID string) bool {
	_, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	return err == nil
}

// GetPolicy returns the policy for a team
func (m *Manager) GetPolicy(teamID string) (string, error) {
	teamSecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("team not found: %w", err)
	}

	policy := teamSecret.Annotations["maas/policy"]
	if policy == "" {
		policy = "unlimited-policy" // fallback
	}
	return policy, nil
}

// CreateDefaultTeam creates the default team if it doesn't exist
func (m *Manager) CreateDefaultTeam() error {
	teamID := "default"

	// Check if default team already exists
	if m.Exists(teamID) {
		log.Printf("Default team already exists, skipping creation")
		return nil
	}

	// Create default team with unlimited policy
	req := &CreateTeamRequest{
		TeamID:      teamID,
		TeamName:    "Default Team",
		Description: "Default team for simple MaaS deployments - users without team assignment",
		Policy:      "unlimited-policy",
	}

	return m.Create(req)
}

// validateTeamRequest validates team creation/update data
func (m *Manager) validateTeamRequest(req *CreateTeamRequest) error {
	if !isValidTeamID(req.TeamID) {
		return fmt.Errorf("team_id must contain only lowercase alphanumeric characters and hyphens, start and end with alphanumeric character, and be 1-63 characters long")
	}
	if req.TeamName == "" {
		return fmt.Errorf("team_name is required")
	}
	// Policy is optional - will default to "unlimited-policy" if not specified
	if req.Policy != "" && req.Policy != "unlimited-policy" {
		// Validate policy name format if specified
		if !isValidTeamID(req.Policy) {
			return fmt.Errorf("policy name must contain only lowercase alphanumeric characters and hyphens")
		}
	}
	return nil
}

// createTeamConfigSecret creates the team configuration secret
func (m *Manager) createTeamConfigSecret(req *CreateTeamRequest) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("team-%s-config", req.TeamID),
			Namespace: m.keyNamespace,
			Labels: map[string]string{
				"maas/resource-type": "team-config",
				"maas/team-id":       req.TeamID,
			},
			Annotations: map[string]string{
				"maas/team-name":   req.TeamName,
				"maas/description": req.Description,
				"maas/policy":      req.Policy,
				"maas/created-at":  time.Now().Format(time.RFC3339),
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"team_id":     req.TeamID,
			"team_config": "active",
		},
	}

	return m.clientset.CoreV1().Secrets(m.keyNamespace).Create(
		context.Background(), secret, metav1.CreateOptions{})
}

// Helper methods for team API keys and members

func (m *Manager) getTeamAPIKeys(teamID string) ([]string, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	for _, secret := range secrets.Items {
		keys = append(keys, secret.Name)
	}

	return keys, nil
}

func (m *Manager) getTeamMembersFromAPIKeys(teamID string) ([]TeamMember, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	// Create a map to deduplicate members (one user might have multiple keys)
	memberMap := make(map[string]TeamMember)
	for _, secret := range secrets.Items {
		userID := secret.Labels["maas/user-id"]
		if userID == "" {
			continue // Skip invalid secrets
		}

		member := TeamMember{
			UserID:    userID,
			UserEmail: secret.Annotations["maas/user-email"],
			Role:      secret.Labels["maas/team-role"],
			TeamID:    teamID,
			TeamName:  secret.Annotations["maas/team-name"],
			Policy:    secret.Annotations["maas/policy"],
			JoinedAt:  secret.Annotations["maas/created-at"],
		}

		// Only keep the first occurrence of each user
		if _, exists := memberMap[userID]; !exists {
			memberMap[userID] = member
		}
	}

	// Convert map to slice
	members := make([]TeamMember, 0, len(memberMap))
	for _, member := range memberMap {
		members = append(members, member)
	}

	return members, nil
}

func (m *Manager) deleteAllTeamKeys(teamID string) error {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	return m.clientset.CoreV1().Secrets(m.keyNamespace).DeleteCollection(
		context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: labelSelector})
}

// updateTeamKeysPolicy updates the kuadrant.io/groups annotation for all team API keys
func (m *Manager) updateTeamKeysPolicy(teamID, newPolicy string) error {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("failed to list team API keys: %w", err)
	}

	for _, secret := range secrets.Items {
		// Update the groups annotation with new policy
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations["kuadrant.io/groups"] = newPolicy
		secret.Annotations["maas/policy"] = newPolicy

		// Update policy-specific label
		oldPolicy := secret.Annotations["maas/policy"]
		if oldPolicy != "" && oldPolicy != newPolicy {
			delete(secret.Labels, fmt.Sprintf("maas/policy-%s", oldPolicy))
		}
		secret.Labels[fmt.Sprintf("maas/policy-%s", newPolicy)] = "true"

		// Update secret
		_, err = m.clientset.CoreV1().Secrets(m.keyNamespace).Update(
			context.Background(), &secret, metav1.UpdateOptions{})
		if err != nil {
			log.Printf("Warning: Failed to update API key %s policy: %v", secret.Name, err)
		}
	}

	log.Printf("Updated %d API keys for team %s to policy %s", len(secrets.Items), teamID, newPolicy)
	return nil
}