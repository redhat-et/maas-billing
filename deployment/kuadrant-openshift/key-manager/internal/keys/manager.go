package keys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/teams"
)

// Manager handles API key operations
type Manager struct {
	clientset    *kubernetes.Clientset
	keyNamespace string
	teamMgr      *teams.Manager
}

// NewManager creates a new key manager
func NewManager(clientset *kubernetes.Clientset, keyNamespace string, teamMgr *teams.Manager) *Manager {
	return &Manager{
		clientset:    clientset,
		keyNamespace: keyNamespace,
		teamMgr:      teamMgr,
	}
}

// CreateTeamKey creates a new API key for a team member
func (m *Manager) CreateTeamKey(teamID string, req *CreateTeamKeyRequest) (*CreateTeamKeyResponse, error) {
	// Validate team exists
	if !m.teamMgr.Exists(teamID) {
		return nil, fmt.Errorf("team not found")
	}

	// Get team policy
	teamPolicy, err := m.teamMgr.GetPolicy(teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team policy: %w", err)
	}

	// Build team member info
	var teamMember *teams.TeamMember
	if teamID == "default" {
		// For default team, auto-create membership info
		userEmail := fmt.Sprintf("%s@default.local", req.UserID)
		teamMember = &teams.TeamMember{
			UserID:    req.UserID,
			UserEmail: userEmail,
			Role:      "member",
			TeamID:    teamID,
			TeamName:  "Default Team",
			Policy:    teamPolicy,
		}
	} else {
		// For non-default teams, validate membership or create new
		teamMember, err = m.validateTeamMembership(teamID, req.UserID)
		if err != nil {
			// User is not yet a member, create new membership info from request
			userEmail := req.UserEmail
			if userEmail == "" {
				userEmail = fmt.Sprintf("%s@company.com", req.UserID)
			}
			
			// Get team details for member creation
			teamDetails, err := m.teamMgr.Get(teamID)
			if err != nil {
				return nil, fmt.Errorf("failed to get team details: %w", err)
			}
			
			teamMember = &teams.TeamMember{
				UserID:    req.UserID,
				UserEmail: userEmail,
				Role:      "member",
				TeamID:    teamID,
				TeamName:  teamDetails.TeamName,
				Policy:    teamPolicy,
			}
		} else {
			// Update policy from team config for existing member
			teamMember.Policy = teamPolicy
		}
	}

	// Generate API key
	apiKey, err := GenerateSecureToken(48)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create enhanced API key secret with team context
	keySecret, err := m.createKeySecret(teamID, req, apiKey, teamMember)
	if err != nil {
		return nil, fmt.Errorf("failed to create key secret: %w", err)
	}

	log.Printf("API key created for team %s, team policies will apply automatically", teamID)

	// Restart Authorino to reload API key configuration immediately
	// This is critical for the new API key to be discovered by Kuadrant
	err = m.restartAuthorino()
	if err != nil {
		log.Printf("Warning: Failed to restart Authorino after key creation: %v", err)
	} else {
		log.Printf("Restarted Authorino to reload API key configuration")
	}

	// Get inherited policies
	inheritedPolicies := m.buildInheritedPolicies(teamMember)

	response := &CreateTeamKeyResponse{
		APIKey:            apiKey,
		UserID:            req.UserID,
		TeamID:            teamID,
		SecretName:        keySecret.Name,
		Policy:            teamMember.Policy,
		CreatedAt:         time.Now().Format(time.RFC3339),
		InheritedPolicies: inheritedPolicies,
	}

	return response, nil
}

// CreateLegacyKey creates a key using the legacy format (for backward compatibility)
func (m *Manager) CreateLegacyKey(req *GenerateKeyRequest) (*CreateTeamKeyResponse, error) {
	// Use default team for legacy endpoint
	teamID := "default"

	// Create team key request (internally use new team-scoped logic)
	createKeyReq := &CreateTeamKeyRequest{
		UserID:            req.UserID,
		Alias:             "legacy-key",
		Models:            []string{}, // Empty models = inherit team defaults
		InheritTeamLimits: true,
	}

	// Call CreateTeamKey which includes Authorino restart
	return m.CreateTeamKey(teamID, createKeyReq)
}

// DeleteKey deletes an API key by its value
func (m *Manager) DeleteKey(apiKey string) (string, error) {
	// Create SHA256 hash of the provided key
	hasher := sha256.New()
	hasher.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	// Find and delete secret by label selector (use truncated hash)
	labelSelector := fmt.Sprintf("maas/key-sha256=%s", keyHash[:32])

	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("failed to find API key: %w", err)
	}

	if len(secrets.Items) == 0 {
		return "", fmt.Errorf("API key not found")
	}

	// Delete the secret
	secretName := secrets.Items[0].Name
	err = m.clientset.CoreV1().Secrets(m.keyNamespace).Delete(context.Background(), secretName, metav1.DeleteOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to delete API key: %w", err)
	}

	return secretName, nil
}

// DeleteTeamKey deletes a specific team API key by name
func (m *Manager) DeleteTeamKey(keyName string) (string, string, error) {
	// Get key secret to validate it exists and get team info
	keySecret, err := m.clientset.CoreV1().Secrets(m.keyNamespace).Get(
		context.Background(), keyName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("API key not found: %w", err)
	}

	teamID := keySecret.Labels["maas/team-id"]
	if teamID == "" {
		return "", "", fmt.Errorf("API key is not associated with a team")
	}

	// Delete the key secret
	err = m.clientset.CoreV1().Secrets(m.keyNamespace).Delete(
		context.Background(), keyName, metav1.DeleteOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to delete API key: %w", err)
	}

	log.Printf("Team API key deleted successfully: %s from team %s", keyName, teamID)
	return keyName, teamID, nil
}

// ListTeamKeys lists all API keys for a team with details
func (m *Manager) ListTeamKeys(teamID string) ([]map[string]interface{}, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	keys := make([]map[string]interface{}, 0)
	for _, secret := range secrets.Items {
		keyInfo := map[string]interface{}{
			"secret_name":    secret.Name,
			"user_id":        secret.Labels["maas/user-id"],
			"user_email":     secret.Annotations["maas/user-email"],
			"role":           secret.Labels["maas/team-role"],
			"policy":         secret.Annotations["maas/policy"],
			"models_allowed": secret.Annotations["maas/models-allowed"],
			"status":         secret.Annotations["maas/status"],
			"created_at":     secret.Annotations["maas/created-at"],
		}

		// Add alias if present
		if alias, exists := secret.Annotations["maas/alias"]; exists {
			keyInfo["alias"] = alias
		}

		// Add custom limits if present
		if customLimits, exists := secret.Annotations["maas/custom-limits"]; exists {
			var limits map[string]interface{}
			if err := json.Unmarshal([]byte(customLimits), &limits); err == nil {
				keyInfo["custom_limits"] = limits
			}
		}

		keys = append(keys, keyInfo)
	}

	return keys, nil
}

// ListUserKeys lists all API keys for a user across all teams
func (m *Manager) ListUserKeys(userID string) ([]map[string]interface{}, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/user-id=%s", userID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	keys := make([]map[string]interface{}, 0)
	for _, secret := range secrets.Items {
		keyInfo := map[string]interface{}{
			"secret_name":    secret.Name,
			"team_id":        secret.Labels["maas/team-id"],
			"team_name":      secret.Annotations["maas/team-name"],
			"user_email":     secret.Annotations["maas/user-email"],
			"role":           secret.Labels["maas/team-role"],
			"policy":         secret.Annotations["maas/policy"],
			"models_allowed": secret.Annotations["maas/models-allowed"],
			"status":         secret.Annotations["maas/status"],
			"created_at":     secret.Annotations["maas/created-at"],
		}

		// Add alias if present
		if alias, exists := secret.Annotations["maas/alias"]; exists {
			keyInfo["alias"] = alias
		}

		// Add custom limits if present
		if customLimits, exists := secret.Annotations["maas/custom-limits"]; exists {
			var limits map[string]interface{}
			if err := json.Unmarshal([]byte(customLimits), &limits); err == nil {
				keyInfo["custom_limits"] = limits
			}
		}

		keys = append(keys, keyInfo)
	}

	return keys, nil
}

// validateTeamMembership validates team membership from existing API key
func (m *Manager) validateTeamMembership(teamID, userID string) (*teams.TeamMember, error) {
	// Look for any existing API key for this user in this team to validate membership
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s,maas/user-id=%s", teamID, userID)
	secrets, err := m.clientset.CoreV1().Secrets(m.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to check user membership: %w", err)
	}

	if len(secrets.Items) == 0 {
		return nil, fmt.Errorf("user %s is not a member of team %s", userID, teamID)
	}

	// Extract membership info from existing API key secret
	secret := secrets.Items[0]
	member := &teams.TeamMember{
		UserID:    userID,
		TeamID:    teamID,
		UserEmail: secret.Annotations["maas/user-email"],
		Role:      secret.Labels["maas/team-role"],
		TeamName:  secret.Annotations["maas/team-name"],
		Policy:    secret.Annotations["maas/policy"],
		JoinedAt:  secret.Annotations["maas/created-at"],
	}

	return member, nil
}

// createKeySecret creates the API key secret with team context
func (m *Manager) createKeySecret(teamID string, req *CreateTeamKeyRequest, apiKey string, teamMember *teams.TeamMember) (*corev1.Secret, error) {
	// Create SHA256 hash of the key
	hasher := sha256.New()
	hasher.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	// Create secret name with team context
	secretName := fmt.Sprintf("apikey-%s-%s-%s", req.UserID, teamID, keyHash[:8])

	// Build models allowed list
	modelsAllowed := strings.Join(req.Models, ",")

	// Create enhanced secret with full team context
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: m.keyNamespace,
			Labels: map[string]string{
				"kuadrant.io/auth-secret": "true",        // Required for working AuthPolicy
				"kuadrant.io/apikeys-by":  "rhcl-keys",   // Required for key listing functions
				"app":                     "llm-gateway", // Required for working AuthPolicy
				"maas/user-id":            req.UserID,
				"maas/team-id":            teamID,
				"maas/team-role":          teamMember.Role,
				"maas/key-sha256":         keyHash[:32],
				"maas/resource-type":      "team-key",
				// Policy targeting label - this is how Kuadrant policies find API keys
				fmt.Sprintf("maas/policy-%s", teamMember.Policy): "true",
			},
			Annotations: map[string]string{
				"secret.kuadrant.io/user-id": req.UserID,        // Required for working AuthPolicy
				"kuadrant.io/groups":         teamMember.Policy, // Use policy name as group
				"maas/team-name":             teamMember.TeamName,
				"maas/user-email":            teamMember.UserEmail,
				"maas/models-allowed":        modelsAllowed,
				"maas/policy":                teamMember.Policy,
				"maas/created-at":            time.Now().Format(time.RFC3339),
				"maas/status":                "active",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"api_key": apiKey,
		},
	}

	// Add alias if provided
	if req.Alias != "" {
		secret.Annotations["maas/alias"] = req.Alias
	}

	// Add custom limits as JSON if provided
	if req.CustomLimits != nil && len(req.CustomLimits) > 0 {
		customLimitsJSON, _ := json.Marshal(req.CustomLimits)
		secret.Annotations["maas/custom-limits"] = string(customLimitsJSON)
	}

	return m.clientset.CoreV1().Secrets(m.keyNamespace).Create(
		context.Background(), secret, metav1.CreateOptions{})
}

// buildInheritedPolicies builds the inherited policies response
func (m *Manager) buildInheritedPolicies(teamMember *teams.TeamMember) map[string]interface{} {
	return map[string]interface{}{
		"policy":    teamMember.Policy,
		"team_id":   teamMember.TeamID,
		"team_name": teamMember.TeamName,
		"role":      teamMember.Role,
	}
}

// restartAuthorino restarts the Authorino deployment to reload API key configuration
func (m *Manager) restartAuthorino() error {
	// Create patch to trigger rolling restart
	restartPatch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().Format(time.RFC3339)))

	// Apply patch to Authorino deployment
	_, err := m.clientset.AppsV1().Deployments("kuadrant-system").Patch(
		context.Background(),
		"authorino",
		types.StrategicMergePatchType,
		restartPatch,
		metav1.PatchOptions{})

	return err
}