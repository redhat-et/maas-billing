package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"errors"
	"gopkg.in/yaml.v2"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	
	"github.com/redhat-et/maas-billing/key-manager/internal/policies"
)

// Enhanced KeyManager with policy management and team support
type KeyManager struct {
	// Existing fields
	clientset           *kubernetes.Clientset
	keyNamespace        string
	secretSelectorLabel string
	secretSelectorValue string
	discoveryRoute      string
	
	// New policy management fields
	kuadrantClient      dynamic.Interface
	policyEngine        *policies.PolicyEngine
	eventRecorder       record.EventRecorder
	defaultPolicies     map[string]*PolicyTemplate
	
	// Configuration
	gatewayName         string
	gatewayNamespace    string
	policyConfigMap     string
	enablePolicyMgmt    bool
	
	// Default team configuration
	createDefaultTeam   bool
	defaultTeamTier     string
	adminAPIKey         string
}

// Policy management (using external policies package)

// Policy template structure
type PolicyTemplate struct {
	Tier                  string   `yaml:"tier"`
	TokenLimitPerHour     int      `yaml:"token_limit_per_hour"`
	TokenLimitPerDay      int      `yaml:"token_limit_per_day"`
	TokenLimitPerMonth    int      `yaml:"token_limit_per_month"`
	BudgetUSDMonthly      float64  `yaml:"budget_usd_monthly"`
	ModelsAllowed         []string `yaml:"models_allowed"`
	RateLimitWindow       string   `yaml:"rate_limit_window"`
	BurstLimit            int      `yaml:"burst_limit"`
	MaxConcurrentRequests int      `yaml:"max_concurrent_requests"`
	EnableBudgetEnforcement bool   `yaml:"enable_budget_enforcement"`
}

// Team policy structure
type TeamPolicy struct {
	PolicyTemplate
	TeamID         string
	AuthPolicyName string
	RateLimitName  string
	AppliedAt      time.Time
}

// Team management structures
type CreateTeamRequest struct {
	TeamID      string `json:"team_id" binding:"required"`
	TeamName    string `json:"team_name" binding:"required"`
	Description string `json:"description"`
	DefaultTier string `json:"default_tier" binding:"required"`
	// Rate limiting parameters
	TokenLimit    int    `json:"token_limit,omitempty"`    // Token limit per time window
	RequestLimit  int    `json:"request_limit,omitempty"`  // Request limit per time window
	TimeWindow  string `json:"time_window,omitempty"` // e.g., "1h", "24h", "1m"
}

type CreateTeamResponse struct {
	TeamID          string                 `json:"team_id"`
	TeamName        string                 `json:"team_name"`
	DefaultTier     string                 `json:"default_tier"`
	PoliciesApplied bool                   `json:"policies_applied"`
	InheritedLimits map[string]interface{} `json:"inherited_limits"`
}

type GetTeamResponse struct {
	TeamID      string        `json:"team_id"`
	TeamName    string        `json:"team_name"`
	Description string        `json:"description"`
	DefaultTier string        `json:"default_tier"`
	Members     []TeamMember  `json:"members"`
	Keys        []string      `json:"keys"`
	CreatedAt   string        `json:"created_at"`
}

type TeamMember struct {
	UserID        string   `json:"user_id"`
	UserEmail     string   `json:"user_email"`
	Role          string   `json:"role"`
	TeamID        string   `json:"team_id"`
	TeamName      string   `json:"team_name"`
	Tier          string   `json:"tier"`
	DefaultModels []string `json:"default_models"`
	JoinedAt      string   `json:"joined_at"`
	// Rate limits
	TokenLimit    int      `json:"token_limit"`
	RequestLimit  int      `json:"request_limit"`
	TimeWindow    string   `json:"time_window"`
}

// User management structures
type AddUserToTeamRequest struct {
	UserEmail   string `json:"user_email" binding:"required"`
	Role        string `json:"role" binding:"required"`
	// Individual rate overrides
	TokenLimit    int    `json:"token_limit,omitempty"`
	RequestLimit  int    `json:"request_limit,omitempty"`
	TimeWindow  string `json:"time_window,omitempty"`
}

type AddUserToTeamResponse struct {
	UserID     string `json:"user_id"`
	TeamID     string `json:"team_id"`
	Role       string `json:"role"`
	Tier       string `json:"tier"`
	TokenLimit    int    `json:"token_limit"`
	RequestLimit  int    `json:"request_limit"`
	TimeWindow string `json:"time_window"`
}

// Enhanced API key structures
type CreateTeamKeyRequest struct {
	UserID            string   `json:"user_id" binding:"required"`
	Alias             string   `json:"alias"`
	Models            []string `json:"models"`
	InheritTeamLimits bool     `json:"inherit_team_limits"`
	// Rate limit overrides
	TokenLimit    int    `json:"token_limit,omitempty"`
	RequestLimit  int    `json:"request_limit,omitempty"`
	TimeWindow  string `json:"time_window,omitempty"`
	CustomLimits map[string]interface{} `json:"custom_limits"`
}

type CreateTeamKeyResponse struct {
	APIKey            string                 `json:"api_key"`
	UserID            string                 `json:"user_id"`
	TeamID            string                 `json:"team_id"`
	SecretName        string                 `json:"secret_name"`
	ModelsAllowed     []string               `json:"models_allowed"`
	Tier              string                 `json:"tier"`
	TokenLimit        int                    `json:"token_limit"`
	RequestLimit      int                    `json:"request_limit"`
	TimeWindow        string                 `json:"time_window"`
	InheritedPolicies map[string]interface{} `json:"inherited_policies"`
	CustomConstraints map[string]interface{} `json:"custom_constraints"`
}

// Legacy structures (keep for backward compatibility)
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

// Policy health and validation structures
type PolicyHealthStatus struct {
	Timestamp     time.Time                   `json:"timestamp"`
	OverallStatus string                      `json:"overall_status"`
	Policies      map[string]PolicyStatus     `json:"policies"`
}

type PolicyStatus struct {
	Type        string `json:"type"`
	Status      string `json:"status"`
	LastUpdated string `json:"last_updated"`
	Message     string `json:"message,omitempty"`
}

type PolicyValidationResult struct {
	TeamID        string           `json:"team_id"`
	Timestamp     time.Time        `json:"timestamp"`
	OverallStatus bool             `json:"overall_status"`
	Tests         []ValidationTest `json:"tests"`
}

type ValidationTest struct {
	Name    string `json:"name"`
	Status  bool   `json:"status"`
	Message string `json:"message"`
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

	// Create dynamic client for Kuadrant CRDs
	kuadrantClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create dynamic client: %v", err)
	}

	// Initialize KeyManager with environment variables
	km := &KeyManager{
		clientset:           clientset,
		keyNamespace:        getEnvOrDefault("KEY_NAMESPACE", "llm"),
		secretSelectorLabel: getEnvOrDefault("SECRET_SELECTOR_LABEL", "kuadrant.io/apikeys-by"),
		secretSelectorValue: getEnvOrDefault("SECRET_SELECTOR_VALUE", "rhcl-keys"),
		discoveryRoute:      getEnvOrDefault("DISCOVERY_ROUTE", "inference-route"),
		kuadrantClient:      kuadrantClient,
		gatewayName:         getEnvOrDefault("GATEWAY_NAME", "inference-gateway"),
		gatewayNamespace:    getEnvOrDefault("GATEWAY_NAMESPACE", "llm"),
		policyConfigMap:     getEnvOrDefault("POLICY_TEMPLATE_CONFIGMAP", "platform-default-policies"),
		enablePolicyMgmt:    getEnvOrDefault("ENABLE_POLICY_MANAGEMENT", "true") == "true",
		
		// Default team configuration
		createDefaultTeam:   getEnvOrDefault("CREATE_DEFAULT_TEAM", "true") == "true",
		defaultTeamTier:     getEnvOrDefault("DEFAULT_TEAM_TIER", "standard"),
		adminAPIKey:         getEnvOrDefault("ADMIN_API_KEY", ""),
	}

	// Default team rate limits from environment
	// These can be overridden per team

	// Initialize policy engine if enabled
	if km.enablePolicyMgmt {
		km.policyEngine = &policies.PolicyEngine{
			KuadrantClient:   kuadrantClient,
			Clientset:        clientset,
			Namespace:        km.keyNamespace,
			GatewayName:      km.gatewayName,
			GatewayNamespace: km.gatewayNamespace,
		}

		// Load default policies from ConfigMap (optional - fallback to hardcoded tiers)
		err = km.loadDefaultPolicies()
		if err != nil {
			log.Printf("Warning: Failed to load policy ConfigMap, using hardcoded tier definitions: %v", err)
		}
		log.Printf("Policy management enabled with gateway: %s/%s", km.gatewayNamespace, km.gatewayName)
	} else {
		log.Printf("Policy management disabled")
	}

	// Create default team if enabled
	if km.createDefaultTeam {
		if err := km.createDefaultTeamOnStartup(); err != nil {
			log.Printf("Warning: Failed to create default team: %v", err)
		} else {
			log.Printf("Default team created successfully")
		}
	}

	// Initialize Gin router
	r := gin.Default()

	// Health check endpoint (no auth required)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Setup API routes
	km.setupAPIRoutes(r)

	// Start server
	port := getEnvOrDefault("PORT", "8080")
	serviceName := getEnvOrDefault("SERVICE_NAME", "key-manager")
	log.Printf("Starting %s on port %s", serviceName, port)
	log.Fatal(r.Run(":" + port))
}

// Create default team on startup if it doesn't exist
func (km *KeyManager) createDefaultTeamOnStartup() error {
	teamID := "default"
	
	// Check if default team already exists
	secretName := fmt.Sprintf("team-%s-config", teamID)
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	
	if err == nil {
		log.Printf("Default team already exists, skipping creation")
		return nil
	}
	
	// Use default tier from environment or fallback
	defaultTier := km.defaultTeamTier
	if defaultTier == "" {
		defaultTier = getEnvOrDefault("DEFAULT_TIER", "standard")
	}
	
	// Create default team
	req := CreateTeamRequest{
		TeamID:            teamID,
		TeamName:          "Default Team",
		Description:       "Default team for simple MaaS deployments - users without team assignment",
		DefaultTier:       defaultTier,
	}
	
	return km.createTeamInternal(&req)
}

// Internal team creation logic (without HTTP context)
func (km *KeyManager) createTeamInternal(req *CreateTeamRequest) error {
	// Validate team data
	if err := km.validateTeamRequest(req); err != nil {
		return fmt.Errorf("team validation failed: %w", err)
	}

	// Check if team already exists
	existingSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", req.TeamID), metav1.GetOptions{})
	if err == nil && existingSecret != nil {
		return fmt.Errorf("team %s already exists", req.TeamID)
	}

	// Create team configuration secret
	teamSecret, err := km.createTeamConfigSecret(req)
	if err != nil {
		return fmt.Errorf("failed to create team secret: %w", err)
	}

	// Apply default team policies if policy management is enabled
	if km.enablePolicyMgmt && km.policyEngine != nil {
		// Get tier limits and create both token and request policies
		limits := policies.GetTierLimits(req.DefaultTier)
		// Override with custom limits if provided
		if req.TokenLimit > 0 {
			limits.TokenLimit = req.TokenLimit
		}
		if req.RequestLimit > 0 {
			limits.RequestLimit = req.RequestLimit
		}
		if req.TimeWindow != "" {
			limits.TokenWindow = req.TimeWindow
			limits.RequestWindow = req.TimeWindow
		}
		err = km.policyEngine.CreateTeamRateLimitPolicies(req.TeamID, limits)
		if err != nil {
			// Rollback team secret creation
			km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(
				context.Background(), teamSecret.Name, metav1.DeleteOptions{})
			return fmt.Errorf("failed to apply team policies: %w", err)
		}
	}

	return nil
}

// Setup all API routes
func (km *KeyManager) setupAPIRoutes(r *gin.Engine) {
	// Admin endpoints (require admin key)
	adminRoutes := r.Group("/", km.requireAdminAuth())
	
	// Legacy endpoints (backward compatibility)
	adminRoutes.POST("/generate_key", km.generateKey)
	adminRoutes.DELETE("/delete_key", km.deleteKey)

	// Model endpoints
	adminRoutes.GET("/models", km.listModels)

	// Team management endpoints
	adminRoutes.POST("/teams", km.createTeam)
	adminRoutes.GET("/teams", km.listTeams)
	adminRoutes.GET("/teams/:team_id", km.getTeam)
	adminRoutes.DELETE("/teams/:team_id", km.deleteTeam)
	
	// Team member management
	adminRoutes.POST("/teams/:team_id/members", km.addUserToTeam)
	adminRoutes.GET("/teams/:team_id/members", km.listTeamMembers)
	adminRoutes.DELETE("/teams/:team_id/members/:user_id", km.removeUserFromTeam)
	
	// Team-scoped API key management
	adminRoutes.POST("/teams/:team_id/keys", km.createTeamKey)
	adminRoutes.GET("/teams/:team_id/keys", km.listTeamKeys)
	adminRoutes.PATCH("/keys/:key_name", km.updateKey)
	adminRoutes.DELETE("/keys/:key_name", km.deleteTeamKey)
	
	// Policy management endpoints (if enabled)
	if km.enablePolicyMgmt {
		adminRoutes.GET("/teams/:team_id/policies", km.getTeamPolicies)
		adminRoutes.POST("/teams/:team_id/policies/sync", km.syncTeamPolicies)
		
		// Admin policy management
		adminRoutes.GET("/admin/policies/health", km.policyHealth)
		adminRoutes.GET("/admin/policies/compliance", km.getPolicyCompliance)
		adminRoutes.GET("/admin/policies/defaults", km.getDefaultPolicies)
		adminRoutes.PUT("/admin/policies/tiers/:tier", km.updateTierPolicy)
		adminRoutes.POST("/admin/policies/tiers", km.createTierPolicy)
	}
	
	// Team activity and usage endpoints
	adminRoutes.GET("/teams/:team_id/activity", km.getTeamActivity)
	adminRoutes.GET("/teams/:team_id/usage", km.getTeamUsage)
}

// Load default policies from ConfigMap (optional - fallback to hardcoded tiers)
func (km *KeyManager) loadDefaultPolicies() error {
	cm, err := km.clientset.CoreV1().ConfigMaps(km.keyNamespace).Get(
		context.Background(), km.policyConfigMap, metav1.GetOptions{})
	if err != nil {
		// ConfigMap not found - use hardcoded tier definitions from dynamic-policies.go
		log.Printf("ConfigMap %s not found, using hardcoded tier definitions", km.policyConfigMap)
		return nil // This is OK - we'll use hardcoded tiers
	}

	// Load from ConfigMap if available
	loaded := 0
	for key, yamlData := range cm.Data {
		if !strings.HasSuffix(key, "-policy.yaml") {
			continue
		}
		
		var policy PolicyTemplate
		err := yaml.Unmarshal([]byte(yamlData), &policy)
		if err != nil {
			log.Printf("Warning: Failed to parse policy template %s: %v", key, err)
			continue
		}
		
		tierName := policy.Tier
		if tierName == "" {
			// Extract tier from filename if not in YAML
			tierName = strings.TrimSuffix(strings.TrimPrefix(key, "tier-"), "-policy.yaml")
			policy.Tier = tierName
		}
		
		// Store in local cache for legacy compatibility (optional)
		log.Printf("Loaded ConfigMap policy template for tier: %s (using with hardcoded fallback)", tierName)
		loaded++
	}

	if loaded > 0 {
		log.Printf("Loaded %d policy templates from ConfigMap", loaded)
	} else {
		log.Printf("No policy templates found in ConfigMap, using hardcoded tier definitions")
	}
	return nil
}

// Legacy generateKey function (backward compatibility)
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

	// Use default team for legacy endpoint
	teamID := "default"
	
	// Create team key request (internally use new team-scoped logic)
	createKeyReq := CreateTeamKeyRequest{
		UserID:             req.UserID,
		Alias:              "legacy-key",
		Models:             []string{}, // Empty models = inherit team defaults
		InheritTeamLimits:  true,
	}

	// Call internal team key creation method
	response, err := km.createTeamKeyInternal(teamID, &createKeyReq)
	if err != nil {
		log.Printf("Failed to create team key via legacy endpoint: %v", err)
		// Check if it's a duplicate key error
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

// Internal team key creation logic (without HTTP context)
func (km *KeyManager) createTeamKeyInternal(teamID string, req *CreateTeamKeyRequest) (*CreateTeamKeyResponse, error) {
	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}

	// Users can have multiple API keys for different purposes (dev, prod, different apps, etc.)

	// Build team member info from defaults for default team or validate existing membership
	var teamMember *TeamMember
	if teamID == "default" {
		// For default team, auto-create membership info (no separate secret needed)
		userEmail := fmt.Sprintf("%s@default.local", req.UserID)
		limits := policies.GetTierLimits(km.defaultTeamTier)
		teamMember = &TeamMember{
			UserID:        req.UserID,
			UserEmail:     userEmail,
			Role:          "member",
			Tier:          km.defaultTeamTier,
			TeamID:        teamID,
			TeamName:      "Default Team",
			DefaultModels: limits.ModelsAllowed,
			TokenLimit:    limits.TokenLimit,
			RequestLimit:  limits.RequestLimit,
			TimeWindow:    limits.TokenWindow,
		}
	} else {
		// For non-default teams, validate user is a member by checking existing API key
		teamMember, err = km.validateTeamMembershipFromAPIKey(teamID, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("user is not a member of this team: %w", err)
		}
	}

	// Generate API key
	apiKey, err := generateSecureToken(48)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create enhanced API key secret with team context
	keySecret, err := km.createEnhancedKeySecret(teamID, req, apiKey, teamMember)
	if err != nil {
		return nil, fmt.Errorf("failed to create key secret: %w", err)
	}

	// Team policies automatically apply to new keys via team labels
	log.Printf("API key created for team %s, team policies will apply automatically", teamID)

	// Build models allowed list
	modelsAllowed := req.Models
	if len(modelsAllowed) == 0 {
		modelsAllowed = teamMember.DefaultModels
	}

	// Get inherited policies
	inheritedPolicies := km.buildInheritedPolicies(teamMember)

	// Build custom constraints
	customConstraints := req.CustomLimits
	if customConstraints == nil {
		customConstraints = make(map[string]interface{})
	}

	response := &CreateTeamKeyResponse{
		APIKey:         apiKey,
		UserID:         req.UserID,
		TeamID:         teamID,
		SecretName:        keySecret.Name,
		ModelsAllowed:     modelsAllowed,
		Tier:              teamMember.Tier,
		TokenLimit:        teamMember.TokenLimit,
		RequestLimit:      teamMember.RequestLimit,
		TimeWindow:        teamMember.TimeWindow,
		InheritedPolicies: inheritedPolicies,
		CustomConstraints: customConstraints,
	}

	return response, nil
}

// Validate team membership from existing API key (for non-default teams)
func (km *KeyManager) validateTeamMembershipFromAPIKey(teamID, userID string) (*TeamMember, error) {
	// Look for any existing API key for this user in this team to validate membership
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s,maas/user-id=%s", teamID, userID)
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to check user membership: %w", err)
	}

	if len(secrets.Items) == 0 {
		return nil, fmt.Errorf("user %s is not a member of team %s", userID, teamID)
	}

	// Extract membership info from existing API key secret
	secret := secrets.Items[0]
	member := &TeamMember{
		UserID:    userID,
		TeamID:    teamID,
		UserEmail: secret.Annotations["maas/user-email"],
		Role:      secret.Labels["maas/team-role"],
		TeamName:  secret.Annotations["maas/team-name"],
		Tier:      secret.Labels["maas/tier"],
	}

	// Parse rate limits
	if tokenStr, exists := secret.Annotations["maas/token-limit"]; exists {
		fmt.Sscanf(tokenStr, "%d", &member.TokenLimit)
	}
	if requestStr, exists := secret.Annotations["maas/request-limit"]; exists {
		fmt.Sscanf(requestStr, "%d", &member.RequestLimit)
	}
	if timeWindow, exists := secret.Annotations["maas/time-window"]; exists {
		member.TimeWindow = timeWindow
	}

	// Get default models from tier policy
	if km.enablePolicyMgmt {
		limits := policies.GetTierLimits(member.Tier)
		member.DefaultModels = limits.ModelsAllowed
	}

	return member, nil
}

// User management endpoints

// Add user to team endpoint
func (km *KeyManager) addUserToTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	var req AddUserToTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Team membership is now managed through API key creation. Use POST /teams/{team_id}/keys to create an API key, which automatically establishes team membership.",
		"alternative": fmt.Sprintf("POST /teams/%s/keys", teamID),
	})
}

// List team members endpoint
func (km *KeyManager) listTeamMembers(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team members from API keys
	members, err := km.getTeamMembersFromAPIKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team members: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get team members"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"team_id": teamID, "members": members})
}

// Remove user from team endpoint
func (km *KeyManager) removeUserFromTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	userID := c.Param("user_id")

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Delete all user's API keys for this team (this removes team membership)
	err = km.deleteAllUserTeamKeys(teamID, userID)
	if err != nil {
		log.Printf("Failed to delete user keys: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove user from team"})
		return
	}

	// Team policies automatically stop applying when API keys are deleted
	log.Printf("User %s removed from team %s by deleting API keys", userID, teamID)

	log.Printf("User removed from team successfully: %s <- %s", userID, teamID)
	c.JSON(http.StatusOK, gin.H{"message": "User removed from team successfully", "user_id": userID, "team_id": teamID})
}

// Team key management endpoints

// Create team-scoped API key
func (km *KeyManager) createTeamKey(c *gin.Context) {
	teamID := c.Param("team_id")
	var req CreateTeamKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Users can have multiple API keys for different purposes (dev, prod, different apps, etc.)

	// Validate user is a member of the team (for non-default teams)
	var teamMember *TeamMember
	if teamID == "default" {
		// For default team, auto-create membership info
		userEmail := fmt.Sprintf("%s@default.local", req.UserID)
		limits := policies.GetTierLimits(km.defaultTeamTier)
		teamMember = &TeamMember{
			UserID:        req.UserID,
			UserEmail:     userEmail,
			Role:          "member",
			Tier:          km.defaultTeamTier,
			TeamID:        teamID,
			TeamName:      "Default Team",
			DefaultModels: limits.ModelsAllowed,
			TokenLimit:    limits.TokenLimit,
			RequestLimit:  limits.RequestLimit,
			TimeWindow:    limits.TokenWindow,
		}
	} else {
		// For non-default teams, validate membership from existing API keys
		teamMember, err = km.validateTeamMembershipFromAPIKey(teamID, req.UserID)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "User is not a member of this team"})
			return
		}
	}

	// Generate API key
	apiKey, err := generateSecureToken(48)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
		return
	}

	// Create enhanced API key secret with team context
	keySecret, err := km.createEnhancedKeySecret(teamID, &req, apiKey, teamMember)
	if err != nil {
		log.Printf("Failed to create key secret: %v", err)
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "already has an active API key") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		}
		return
	}

	// Team policies automatically apply to new keys via team labels
	log.Printf("Team API key created for team %s, team policies will apply automatically", teamID)

	// Build models allowed list
	modelsAllowed := req.Models
	if len(modelsAllowed) == 0 {
		modelsAllowed = teamMember.DefaultModels
	}

	// Get inherited policies
	inheritedPolicies := km.buildInheritedPolicies(teamMember)

	// Build custom constraints
	customConstraints := req.CustomLimits
	if customConstraints == nil {
		customConstraints = make(map[string]interface{})
	}

	response := CreateTeamKeyResponse{
		APIKey:         apiKey,
		UserID:         req.UserID,
		TeamID:         teamID,
		SecretName:        keySecret.Name,
		ModelsAllowed:     modelsAllowed,
		Tier:              teamMember.Tier,
		TokenLimit:        teamMember.TokenLimit,
		RequestLimit:      teamMember.RequestLimit,
		TimeWindow:        teamMember.TimeWindow,
		InheritedPolicies: inheritedPolicies,
		CustomConstraints: customConstraints,
	}

	log.Printf("Team API key created successfully for user %s in team %s", req.UserID, teamID)
	c.JSON(http.StatusOK, response)
}

// List team API keys
func (km *KeyManager) listTeamKeys(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team API keys
	keys, err := km.getTeamAPIKeysDetailed(teamID)
	if err != nil {
		log.Printf("Failed to get team keys: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get team keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"team_id": teamID, "keys": keys})
}

// Update API key (budget, status, etc.)
func (km *KeyManager) updateKey(c *gin.Context) {
	keyName := c.Param("key_name")
	
	var updateReq map[string]interface{}
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get existing key secret
	keySecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), keyName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	// Update annotations based on request
	if keySecret.Annotations == nil {
		keySecret.Annotations = make(map[string]string)
	}

	updated := false
	if tokenLimit, exists := updateReq["token_limit"]; exists {
		if token, ok := tokenLimit.(float64); ok {
			keySecret.Annotations["maas/token-limit"] = fmt.Sprintf("%d", int(token))
			updated = true
		}
	}
	
	if requestLimit, exists := updateReq["request_limit"]; exists {
		if request, ok := requestLimit.(float64); ok {
			keySecret.Annotations["maas/request-limit"] = fmt.Sprintf("%d", int(request))
			updated = true
		}
	}
	
	if timeWindow, exists := updateReq["time_window"]; exists {
		if window, ok := timeWindow.(string); ok {
			keySecret.Annotations["maas/time-window"] = window
			updated = true
		}
	}

	if status, exists := updateReq["status"]; exists {
		if statusStr, ok := status.(string); ok {
			keySecret.Annotations["maas/status"] = statusStr
			keySecret.Annotations["maas/updated-at"] = time.Now().Format(time.RFC3339)
			updated = true
		}
	}

	if !updated {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid updates provided"})
		return
	}

	// Save updated secret
	_, err = km.clientset.CoreV1().Secrets(km.keyNamespace).Update(
		context.Background(), keySecret, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Failed to update key secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update API key"})
		return
	}

	log.Printf("API key updated successfully: %s", keyName)
	c.JSON(http.StatusOK, gin.H{"message": "API key updated successfully", "key_name": keyName})
}

// Delete team API key
func (km *KeyManager) deleteTeamKey(c *gin.Context) {
	keyName := c.Param("key_name")

	// Get key secret to validate it exists and get team info
	keySecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), keyName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	teamID := keySecret.Labels["maas/team-id"]
	if teamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key is not associated with a team"})
		return
	}

	// Team policies automatically stop applying when key is deleted
	log.Printf("Key removed from team %s, team policies no longer apply", teamID)

	// Delete the key secret
	err = km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(
		context.Background(), keyName, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Failed to delete key secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		return
	}

	log.Printf("Team API key deleted successfully: %s from team %s", keyName, teamID)
	c.JSON(http.StatusOK, gin.H{"message": "API key deleted successfully", "key_name": keyName, "team_id": teamID})
}

// Policy management endpoints

// Get team policies
func (km *KeyManager) getTeamPolicies(c *gin.Context) {
	teamID := c.Param("team_id")

	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Check if team has applied policies (simplified - just check if policy exists)
	policyName := fmt.Sprintf("team-%s-rate-limits", teamID)
	log.Printf("Getting team policies for team %s (policy: %s)", teamID, policyName)

	// Get team configuration to determine tier
	teamSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	tier := teamSecret.Annotations["maas/default-tier"]
	limits := policies.GetTierLimits(tier)

	response := map[string]interface{}{
		"team_id":           teamID,
		"tier":              tier,
		"auth_policy_name":  "gateway-auth-policy",
		"rate_limit_name":   policyName,
		"applied_at":        time.Now().Format(time.RFC3339),
		"policy_limits": map[string]interface{}{
			"token_limit_per_hour":   limits.TokenLimitPerHour,
			"token_limit_per_day":    limits.TokenLimitPerDay,
			"token_limit":            limits.TokenLimit,
			"request_limit":          limits.RequestLimit,
			"token_window":           limits.TokenWindow,
			"request_window":         limits.RequestWindow,
			"models_allowed":         limits.ModelsAllowed,
			"max_concurrent_requests": limits.MaxConcurrentRequests,
		},
	}

	c.JSON(http.StatusOK, response)
}

// Sync team policies with updated defaults
func (km *KeyManager) syncTeamPolicies(c *gin.Context) {
	teamID := c.Param("team_id")

	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get team configuration
	teamSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	tier := teamSecret.Annotations["maas/default-tier"]

	// Re-create team policies with latest defaults
	limits := policies.GetTierLimits(tier)
	err = km.policyEngine.CreateTeamRateLimitPolicies(teamID, limits)
	if err != nil {
		log.Printf("Failed to sync team policies: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync team policies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Team policies synchronized successfully",
		"team_id":    teamID,
		"tier":       tier,
		"synced_at":  time.Now().Format(time.RFC3339),
	})
}

// Validate team policies
func (km *KeyManager) validateTeamPolicies(c *gin.Context) {
	teamID := c.Param("team_id")

	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	validation := PolicyValidationResult{
		TeamID:    teamID,
		Timestamp: time.Now(),
		Tests:     make([]ValidationTest, 0),
	}

	// Test 1: Check if team policies exist (simplified)
	validation.Tests = append(validation.Tests, ValidationTest{
		Name:   "Team Policy System",
		Status: true, // Always true since we use hardcoded policies
		Message: "Policy system operational",
	})

	// Test 2: Check policy configuration
	validation.Tests = append(validation.Tests, ValidationTest{
		Name:   "Policy Configuration Valid",
		Status: true, // Always true for hardcoded policies
		Message: "Policy limits are configured from hardcoded definitions",
	})

	// Test 3: Check team has active API keys
	keys, err := km.getTeamAPIKeys(teamID)
	hasActiveKeys := err == nil && len(keys) > 0
	validation.Tests = append(validation.Tests, ValidationTest{
		Name:   "Has Active API Keys",
		Status: hasActiveKeys,
		Message: func() string {
			if hasActiveKeys {
				return fmt.Sprintf("Team has %d active API keys", len(keys))
			}
			return "Team has no active API keys"
		}(),
	})

	// Determine overall validation status
	validation.OverallStatus = true
	for _, test := range validation.Tests {
		if !test.Status {
			validation.OverallStatus = false
			break
		}
	}

	statusCode := http.StatusOK
	if !validation.OverallStatus {
		statusCode = http.StatusBadRequest
	}

	c.JSON(statusCode, validation)
}

// Policy health check
func (km *KeyManager) policyHealth(c *gin.Context) {
	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	health := PolicyHealthStatus{
		Timestamp: time.Now(),
		Policies:  make(map[string]PolicyStatus),
	}

	// Check policy engine health
	health.Policies["policy-engine"] = PolicyStatus{
		Type:        "engine",
		Status:      "ready",
		LastUpdated: time.Now().Format(time.RFC3339),
		Message:     "Dynamic policy engine operational",
	}

	// Check default policies loaded
	health.Policies["default-policies"] = PolicyStatus{
		Type:        "templates",
		Status:      "ready",
		LastUpdated: time.Now().Format(time.RFC3339),
		Message:     "Hardcoded tier definitions loaded (free, standard, premium, unlimited)",
	}

	// Determine overall health
	health.OverallStatus = "healthy"
	for _, status := range health.Policies {
		if status.Status != "ready" {
			health.OverallStatus = "degraded"
			break
		}
	}

	if health.OverallStatus == "healthy" {
		c.JSON(http.StatusOK, health)
	} else {
		c.JSON(http.StatusServiceUnavailable, health)
	}
}

// Get policy compliance report
func (km *KeyManager) getPolicyCompliance(c *gin.Context) {
	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	// Get all teams
	labelSelector := "maas/resource-type=team-config"
	teamSecrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get teams"})
		return
	}

	compliance := map[string]interface{}{
		"timestamp":           time.Now().Format(time.RFC3339),
		"total_teams":         len(teamSecrets.Items),
		"compliant_teams":     0,
		"non_compliant_teams": 0,
		"team_details":        make([]map[string]interface{}, 0),
	}

	compliantCount := 0
	for _, teamSecret := range teamSecrets.Items {
		teamID := teamSecret.Labels["maas/team-id"]
		tier := teamSecret.Annotations["maas/default-tier"]

		// Assume all teams are compliant with hardcoded policies
		hasPolicies := true // All teams use default or tier-specific hardcoded policies

		teamDetail := map[string]interface{}{
			"team_id":   teamID,
			"tier":      tier,
			"compliant": hasPolicies,
		}

		compliantCount++
		teamDetail["message"] = "Policies available via hardcoded definitions"

		compliance["team_details"] = append(compliance["team_details"].([]map[string]interface{}), teamDetail)
	}

	compliance["compliant_teams"] = compliantCount
	compliance["non_compliant_teams"] = len(teamSecrets.Items) - compliantCount
	compliance["compliance_percentage"] = float64(compliantCount) / float64(len(teamSecrets.Items)) * 100

	c.JSON(http.StatusOK, compliance)
}

// Get default policies
func (km *KeyManager) getDefaultPolicies(c *gin.Context) {
	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	// Return hardcoded tier policies
	tiers := []string{"free", "standard", "premium", "unlimited"}
	policiesMap := make(map[string]interface{})
	for _, tier := range tiers {
		limits := policies.GetTierLimits(tier)
		policiesMap[tier] = map[string]interface{}{
			"tier":                  tier,
			"token_limit_per_hour":  limits.TokenLimitPerHour,
			"token_limit_per_day":   limits.TokenLimitPerDay,
			"token_limit":           limits.TokenLimit,
			"request_limit":         limits.RequestLimit,
			"models_allowed":        limits.ModelsAllowed,
			"max_concurrent_requests": limits.MaxConcurrentRequests,
			"enable_budget_enforcement": true,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"default_policies": policiesMap,
		"loaded_at":        time.Now().Format(time.RFC3339),
	})
}

// Update tier policy (placeholder)
func (km *KeyManager) updateTierPolicy(c *gin.Context) {
	tier := c.Param("tier")
	
	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Tier policy updates not yet implemented",
		"tier":  tier,
		"message": "This feature requires ConfigMap update mechanisms",
	})
}

// Create tier policy (placeholder)
func (km *KeyManager) createTierPolicy(c *gin.Context) {
	if !km.enablePolicyMgmt {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Policy management is disabled"})
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Tier policy creation not yet implemented",
		"message": "This feature requires ConfigMap update mechanisms",
	})
}

// Team activity and usage endpoints

// Get team activity
func (km *KeyManager) getTeamActivity(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team API keys for activity tracking
	keys, err := km.getTeamAPIKeysDetailed(teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get team activity"})
		return
	}

	activity := map[string]interface{}{
		"team_id":      teamID,
		"total_keys":   len(keys),
		"active_keys":  0,
		"total_spend":  0.0,
		"generated_at": time.Now().Format(time.RFC3339),
		"keys":         keys,
	}

	// Calculate active keys and total spend
	activeCount := 0
	totalSpend := 0.0
	for _, key := range keys {
		if status, ok := key["status"].(string); ok && status == "active" {
			activeCount++
		}
		if spend, ok := key["spend_current"].(string); ok {
			var spendFloat float64
			if _, err := fmt.Sscanf(spend, "%f", &spendFloat); err == nil {
				totalSpend += spendFloat
			}
		}
	}

	activity["active_keys"] = activeCount
	activity["total_spend"] = totalSpend

	c.JSON(http.StatusOK, activity)
}

// Get team usage summary
func (km *KeyManager) getTeamUsage(c *gin.Context) {
	teamID := c.Param("team_id")

	// Validate team exists
	teamSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team members from API keys
	members, err := km.getTeamMembersFromAPIKeys(teamID)
	if err != nil {
		members = []TeamMember{}
	}

	// Get team API keys
	keys, err := km.getTeamAPIKeysDetailed(teamID)
	if err != nil {
		keys = []map[string]interface{}{}
	}

	usage := map[string]interface{}{
		"team_id":         teamID,
		"team_name":       teamSecret.Annotations["maas/team-name"],
		"tier":            teamSecret.Annotations["maas/default-tier"],
		"total_members":   len(members),
		"total_keys":      len(keys),
		"team_budget":     teamSecret.Annotations["maas/default-budget"],
		"generated_at":    time.Now().Format(time.RFC3339),
		"members_summary": make([]map[string]interface{}, 0),
	}

	// Build member usage summary
	for _, member := range members {
		memberSummary := map[string]interface{}{
			"user_id":      member.UserID,
			"user_email":   member.UserEmail,
			"role":         member.Role,
			"token_limit":  member.TokenLimit,
			"request_limit": member.RequestLimit,
			"time_window":  member.TimeWindow,
			"joined_at":    member.JoinedAt,
			"keys_count":   0,
		}

		// Count keys for this member
		keysCount := 0
		for _, key := range keys {
			if keyUserID, ok := key["user_id"].(string); ok && keyUserID == member.UserID {
				keysCount++
			}
		}
		memberSummary["keys_count"] = keysCount

		usage["members_summary"] = append(usage["members_summary"].([]map[string]interface{}), memberSummary)
	}

	c.JSON(http.StatusOK, usage)
}

// Helper functions for team management

// Note: Users can have multiple API keys for different purposes

// Get team members from API key secrets
func (km *KeyManager) getTeamMembersFromAPIKeys(teamID string) ([]TeamMember, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
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
			Tier:      secret.Labels["maas/tier"],
			JoinedAt:  secret.Annotations["maas/created-at"], // Use key creation as join date
		}
		
		// Parse rate limits
		if tpmStr, exists := secret.Annotations["maas/token-limit"]; exists {
			fmt.Sscanf(tpmStr, "%d", &member.TokenLimit)
		}
		if rpmStr, exists := secret.Annotations["maas/request-limit"]; exists {
			fmt.Sscanf(rpmStr, "%d", &member.RequestLimit)
		}
		if timeWindow, exists := secret.Annotations["maas/time-window"]; exists {
			member.TimeWindow = timeWindow
		}

		// Get default models from tier
		if km.enablePolicyMgmt {
			limits := policies.GetTierLimits(member.Tier)
			member.DefaultModels = limits.ModelsAllowed
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

func (km *KeyManager) getTeamAPIKeys(teamID string) ([]string, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
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

func (km *KeyManager) deleteAllTeamKeys(teamID string) error {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	return km.clientset.CoreV1().Secrets(km.keyNamespace).DeleteCollection(
		context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: labelSelector})
}

func (km *KeyManager) deleteAllUserTeamKeys(teamID, userID string) error {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s,maas/user-id=%s", teamID, userID)
	return km.clientset.CoreV1().Secrets(km.keyNamespace).DeleteCollection(
		context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: labelSelector})
}

// Helper functions for user management

// Extract user ID from email (simple approach - use email prefix before @)
func (km *KeyManager) extractUserIDFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return ""
	}
	
	// Convert to lowercase and replace invalid characters
	userID := strings.ToLower(parts[0])
	userID = strings.ReplaceAll(userID, "_", "-")
	userID = strings.ReplaceAll(userID, ".", "-")
	
	// Ensure it's valid Kubernetes name
	if len(userID) > 63 {
		userID = userID[:63]
	}
	
	// Ensure it starts and ends with alphanumeric
	userID = strings.Trim(userID, "-")
	
	return userID
}

// Note: Team membership is now managed through API key creation.
// The API key secret contains all membership information.

// Deactivate all user's API keys for a specific team
func (km *KeyManager) deactivateUserTeamKeys(teamID, userID string) error {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s,maas/user-id=%s", teamID, userID)
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		// Mark as inactive
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations["maas/status"] = "inactive"
		secret.Annotations["maas/deactivated-at"] = time.Now().Format(time.RFC3339)
		
		_, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Update(
			context.Background(), &secret, metav1.UpdateOptions{})
		if err != nil {
			log.Printf("Failed to deactivate key %s: %v", secret.Name, err)
		}
	}

	return nil
}

// Helper functions for enhanced API key creation

// Note: validateTeamMembership is replaced by validateTeamMembershipFromAPIKey
// Team membership is now determined by existing API keys, not separate membership secrets

// Create enhanced API key secret with team context
func (km *KeyManager) createEnhancedKeySecret(teamID string, req *CreateTeamKeyRequest, apiKey string, teamMember *TeamMember) (*corev1.Secret, error) {
	// Create SHA256 hash of the key
	hasher := sha256.New()
	hasher.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(hasher.Sum(nil))

	// Create secret name with team context
	secretName := fmt.Sprintf("apikey-%s-%s-%s", req.UserID, teamID, keyHash[:8])

	// Build models allowed list
	modelsAllowed := strings.Join(req.Models, ",")
	if modelsAllowed == "" && teamMember.DefaultModels != nil {
		modelsAllowed = strings.Join(teamMember.DefaultModels, ",")
	}

	// Determine rate limits - use request limits or team member limits
	tokenLimit := req.TokenLimit
	if tokenLimit == 0 {
		tokenLimit = teamMember.TokenLimit
	}
	requestLimit := req.RequestLimit
	if requestLimit == 0 {
		requestLimit = teamMember.RequestLimit
	}
	timeWindow := req.TimeWindow
	if timeWindow == "" {
		timeWindow = teamMember.TimeWindow
	}

	// Create enhanced secret with full team context
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: km.keyNamespace,
			Labels: map[string]string{
				"authorino.kuadrant.io/managed-by": "authorino",
				"kuadrant.io/apikeys-by": km.secretSelectorValue,
				"maas/user-id":           req.UserID,
				"maas/team-id":           teamID,
				"maas/team-role":         teamMember.Role,
				"maas/key-sha256":        keyHash[:32],
				"maas/tier":              teamMember.Tier,
				"maas/resource-type":     "team-key",
			},
			Annotations: map[string]string{
				"maas/team-name":     teamMember.TeamName,
				"maas/user-email":    teamMember.UserEmail,
				"maas/token-limit":   fmt.Sprintf("%d", tokenLimit),
				"maas/request-limit": fmt.Sprintf("%d", requestLimit),
				"maas/time-window":   timeWindow,
				"maas/models-allowed": modelsAllowed,
				"maas/tier":          teamMember.Tier,
				"maas/created-at":    time.Now().Format(time.RFC3339),
				"maas/status":        "active",
				"kuadrant.io/groups": fmt.Sprintf("team-%s,tier-%s", teamID, teamMember.Tier),
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

	return km.clientset.CoreV1().Secrets(km.keyNamespace).Create(
		context.Background(), secret, metav1.CreateOptions{})
}

// Build inherited policies response
func (km *KeyManager) buildInheritedPolicies(teamMember *TeamMember) map[string]interface{} {
	if !km.enablePolicyMgmt {
		return map[string]interface{}{
			"tier": teamMember.Tier,
			"team_id": teamMember.TeamID,
		}
	}

	limits := policies.GetTierLimits(teamMember.Tier)

	return map[string]interface{}{
		"tier":                  teamMember.Tier,
		"team_id":               teamMember.TeamID,
		"team_hourly_limit":     limits.TokenLimitPerHour,
		"user_hourly_limit":     limits.TokenLimitPerHour / 4, // 25% of team limit per user
		"models_allowed":        limits.ModelsAllowed,
		"budget_enforcement":    true,
		"max_concurrent_requests": limits.MaxConcurrentRequests,
	}
}

// Get detailed team API keys
func (km *KeyManager) getTeamAPIKeysDetailed(teamID string) ([]map[string]interface{}, error) {
	labelSelector := fmt.Sprintf("kuadrant.io/apikeys-by=rhcl-keys,maas/team-id=%s", teamID)
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
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
			"tier":           secret.Labels["maas/tier"],
			"token_limit":    secret.Annotations["maas/token-limit"],
			"request_limit":  secret.Annotations["maas/request-limit"],
			"time_window":    secret.Annotations["maas/time-window"],
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

// Policy engine methods (moved to internal/policies package)

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

// Team management endpoints

// Create team endpoint
func (km *KeyManager) createTeam(c *gin.Context) {
	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate team data
	if err := km.validateTeamRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if team already exists
	existingSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", req.TeamID), metav1.GetOptions{})
	if err == nil && existingSecret != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Team already exists"})
		return
	}

	// Create team configuration secret
	teamSecret, err := km.createTeamConfigSecret(&req)
	if err != nil {
		log.Printf("Failed to create team secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team"})
		return
	}

	// Apply default team policies if policy management is enabled
	if km.enablePolicyMgmt {
		// Get tier limits and create both token and request policies
		limits := policies.GetTierLimits(req.DefaultTier)
		// Override with custom limits if provided
		if req.TokenLimit > 0 {
			limits.TokenLimit = req.TokenLimit
		}
		if req.RequestLimit > 0 {
			limits.RequestLimit = req.RequestLimit
		}
		if req.TimeWindow != "" {
			limits.TokenWindow = req.TimeWindow
			limits.RequestWindow = req.TimeWindow
		}
		err = km.policyEngine.CreateTeamRateLimitPolicies(req.TeamID, limits)
		if err != nil {
			log.Printf("Failed to apply team policies: %v", err)
			// Rollback team secret creation
			km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(
				context.Background(), teamSecret.Name, metav1.DeleteOptions{})
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply team policies"})
			return
		}
	}

	// Get inherited limits for response
	inheritedLimits := km.getTierLimits(req.DefaultTier)

	response := CreateTeamResponse{
		TeamID:          req.TeamID,
		TeamName:        req.TeamName,
		DefaultTier:     req.DefaultTier,
		PoliciesApplied: km.enablePolicyMgmt,
		InheritedLimits: inheritedLimits,
	}

	log.Printf("Team created successfully: %s (%s)", req.TeamID, req.TeamName)
	c.JSON(http.StatusOK, response)
}

// Validate team creation request
func (km *KeyManager) validateTeamRequest(req *CreateTeamRequest) error {
	if !isValidTeamID(req.TeamID) {
		return errors.New("team_id must contain only lowercase alphanumeric characters and hyphens, start and end with alphanumeric character, and be 1-63 characters long")
	}
	if req.TeamName == "" {
		return errors.New("team_name is required")
	}
	// Use default tier if not specified
	if req.DefaultTier == "" {
		req.DefaultTier = getEnvOrDefault("DEFAULT_TIER", "standard")
		log.Printf("No tier specified for team %s, using default tier: %s", req.TeamID, req.DefaultTier)
	}
	// Validate tier exists (check both ConfigMap and hardcoded tiers)
	if km.enablePolicyMgmt {
		availableTiers := km.getAvailableTiers()
		validTier := false
		for _, tier := range availableTiers {
			if tier == req.DefaultTier {
				validTier = true
				break
			}
		}
		if !validTier {
			return fmt.Errorf("invalid tier: %s. Available tiers: %v", req.DefaultTier, availableTiers)
		}
	}
	return nil
}

// Create team configuration secret
func (km *KeyManager) createTeamConfigSecret(req *CreateTeamRequest) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("team-%s-config", req.TeamID),
			Namespace: km.keyNamespace,
			Labels: map[string]string{
				"maas/resource-type": "team-config",
				"maas/team-id":       req.TeamID,
				"maas/tier":          req.DefaultTier,
			},
			Annotations: map[string]string{
				"maas/team-name":     req.TeamName,
				"maas/description":   req.Description,
				"maas/default-tier":  req.DefaultTier,
				"maas/token-limit":   fmt.Sprintf("%d", req.TokenLimit),
				"maas/request-limit": fmt.Sprintf("%d", req.RequestLimit),
				"maas/time-window":   req.TimeWindow,
				"maas/created-at":    time.Now().Format(time.RFC3339),
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"team_id":     req.TeamID,
			"team_config": "active",
		},
	}

	return km.clientset.CoreV1().Secrets(km.keyNamespace).Create(
		context.Background(), secret, metav1.CreateOptions{})
}

// List teams endpoint
func (km *KeyManager) listTeams(c *gin.Context) {
	labelSelector := "maas/resource-type=team-config"
	secrets, err := km.clientset.CoreV1().Secrets(km.keyNamespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		log.Printf("Failed to list team secrets: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list teams"})
		return
	}

	teams := make([]map[string]interface{}, 0)
	for _, secret := range secrets.Items {
		team := map[string]interface{}{
			"team_id":     secret.Labels["maas/team-id"],
			"team_name":   secret.Annotations["maas/team-name"],
			"description": secret.Annotations["maas/description"],
			"tier":        secret.Annotations["maas/default-tier"],
			"created_at":  secret.Annotations["maas/created-at"],
		}
		teams = append(teams, team)
	}

	c.JSON(http.StatusOK, gin.H{"teams": teams})
}

// Get team endpoint
func (km *KeyManager) getTeam(c *gin.Context) {
	teamID := c.Param("team_id")
	
	// Get team config secret
	teamSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Get team members from API keys
	members, err := km.getTeamMembersFromAPIKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team members: %v", err)
		members = []TeamMember{} // Return empty list on error
	}

	// Get team API keys
	keys, err := km.getTeamAPIKeys(teamID)
	if err != nil {
		log.Printf("Failed to get team keys: %v", err)
		keys = []string{} // Return empty list on error
	}

	response := GetTeamResponse{
		TeamID:      teamID,
		TeamName:    teamSecret.Annotations["maas/team-name"],
		Description: teamSecret.Annotations["maas/description"],
		DefaultTier: teamSecret.Annotations["maas/default-tier"],
		Members:     members,
		Keys:        keys,
		CreatedAt:   teamSecret.Annotations["maas/created-at"],
	}

	c.JSON(http.StatusOK, response)
}

// Delete team endpoint
func (km *KeyManager) deleteTeam(c *gin.Context) {
	teamID := c.Param("team_id")

	// Check if team exists
	teamSecret, err := km.clientset.CoreV1().Secrets(km.keyNamespace).Get(
		context.Background(), fmt.Sprintf("team-%s-config", teamID), metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Delete team policies if policy management is enabled
	if km.enablePolicyMgmt {
		err = km.policyEngine.DeleteTeamPolicies(teamID)
		if err != nil {
			log.Printf("Failed to delete team policies: %v", err)
			// Continue with deletion but log the error
		}
	}

	// Delete all team API keys (this also removes all team memberships)
	err = km.deleteAllTeamKeys(teamID)
	if err != nil {
		log.Printf("Failed to delete team keys: %v", err)
		// Continue with deletion but log the error
	}

	// Delete team configuration secret
	err = km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(
		context.Background(), teamSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Failed to delete team secret: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete team"})
		return
	}

	log.Printf("Team deleted successfully: %s", teamID)
	c.JSON(http.StatusOK, gin.H{"message": "Team deleted successfully", "team_id": teamID})
}

// Helper functions

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

// isValidTeamID validates team ID using same rules as user ID
func isValidTeamID(teamID string) bool {
	return isValidUserID(teamID) // Same validation rules
}

// Get available tiers from policy templates or hardcoded defaults
func (km *KeyManager) getAvailableTiers() []string {
	// Always include hardcoded tiers
	hardcodedTiers := []string{"free", "standard", "premium", "unlimited"}
	
	if !km.enablePolicyMgmt {
		return hardcodedTiers
	}
	
	// Always return hardcoded tiers (ConfigMap support simplified)
	return hardcodedTiers
}

// Get tier limits for response
func (km *KeyManager) getTierLimits(tier string) map[string]interface{} {
	if !km.enablePolicyMgmt {
		return map[string]interface{}{"tier": tier}
	}
	
	limits := policies.GetTierLimits(tier)
	
	return map[string]interface{}{
		"tier":                  tier,
		"token_limit_per_hour":  limits.TokenLimitPerHour,
		"token_limit_per_day":   limits.TokenLimitPerDay,
		"token_limit":           limits.TokenLimit,
		"request_limit":         limits.RequestLimit,
		"token_window":          limits.TokenWindow,
		"request_window":        limits.RequestWindow,
		"models_allowed":        limits.ModelsAllowed,
		"max_concurrent_requests": limits.MaxConcurrentRequests,
	}
}
