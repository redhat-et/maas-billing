package policies

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Dynamic policy creation for teams
// Simple approach: default policy = unlimited, teams can attach specific limits

// PolicyEngine manages Kuadrant policy generation
type PolicyEngine struct {
	KuadrantClient   dynamic.Interface
	Clientset        *kubernetes.Clientset
	Namespace        string
	GatewayName      string
	GatewayNamespace string
}

// TierLimits defines the limits for a specific tier
type TierLimits struct {
	// Token-based rate limits (for TokenRateLimitPolicy)
	TokenLimit    int      `json:"token_limit"`     // Token limit per time window
	TokenWindow   string   `json:"token_window"`    // Time window for token limits (e.g., "1h", "1m")
	// Request-based rate limits (for RateLimitPolicy)
	RequestLimit  int      `json:"request_limit"`   // Request limit per time window
	RequestWindow string   `json:"request_window"`  // Time window for request limits
	// Model access control
	ModelsAllowed []string `json:"models_allowed"`
	// Backwards compatibility (deprecated)
	TokenLimitPerHour     int `json:"token_limit_per_hour,omitempty"`
	TokenLimitPerDay      int `json:"token_limit_per_day,omitempty"`
	MaxConcurrentRequests int `json:"max_concurrent_requests,omitempty"`
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetDefaultTier returns the default tier from environment or fallback
func GetDefaultTier() string {
	if defaultTier := getEnvOrDefault("DEFAULT_TIER", ""); defaultTier != "" {
		return defaultTier
	}
	return "standard" // Safe default with reasonable limits
}

// GetTierLimits returns the limits for a given tier with fallback to default
func GetTierLimits(tier string) *TierLimits {
	// If tier is empty or invalid, use default tier
	if tier == "" {
		tier = GetDefaultTier()
	}
	
	switch tier {
	case "free":
		return &TierLimits{
			TokenLimit:            2000,  // 2000 tokens per minute
			TokenWindow:           "1m",
			RequestLimit:          60,    // 60 requests per minute
			RequestWindow:         "1m",
			ModelsAllowed:         []string{"simulator-model"},
			// Backwards compatibility
			TokenLimitPerHour:     10000,
			TokenLimitPerDay:      50000,
			MaxConcurrentRequests: 5,
		}
	case "standard":
		return &TierLimits{
			TokenLimit:            10000, // 10k tokens per minute
			TokenWindow:           "1m",
			RequestLimit:          120,   // 120 requests per minute
			RequestWindow:         "1m",
			ModelsAllowed:         []string{"simulator-model", "qwen3-0-6b-instruct"},
			// Backwards compatibility
			TokenLimitPerHour:     50000,
			TokenLimitPerDay:      500000,
			MaxConcurrentRequests: 10,
		}
	case "premium":
		return &TierLimits{
			TokenLimit:            50000, // 50k tokens per minute
			TokenWindow:           "1m",
			RequestLimit:          600,   // 600 requests per minute
			RequestWindow:         "1m",
			ModelsAllowed:         []string{"simulator-model", "qwen3-0-6b-instruct", "premium-models"},
			// Backwards compatibility
			TokenLimitPerHour:     200000,
			TokenLimitPerDay:      2000000,
			MaxConcurrentRequests: 25,
		}
	case "unlimited":
		return &TierLimits{
			TokenLimit:            -1, // -1 = unlimited
			TokenWindow:           "1h",
			RequestLimit:          -1,
			RequestWindow:         "1h",
			ModelsAllowed:         []string{"*"}, // All models
			// Backwards compatibility
			TokenLimitPerHour:     -1,
			TokenLimitPerDay:      -1,
			MaxConcurrentRequests: -1,
		}
	default:
		// Fallback to default tier if tier not recognized
		log.Printf("Unknown tier '%s', falling back to default tier: %s", tier, GetDefaultTier())
		return GetTierLimits(GetDefaultTier())
	}
}

// CreateTeamRateLimitPolicies creates both TokenRateLimitPolicy and RateLimitPolicy for a team
func (pe *PolicyEngine) CreateTeamRateLimitPolicies(teamID string, limits *TierLimits) error {
	// Create TokenRateLimitPolicy for token-based limits
	if err := pe.CreateTeamTokenRateLimit(teamID, fmt.Sprintf("team-%s-token-limits", teamID), limits); err != nil {
		return fmt.Errorf("failed to create token rate limit policy: %w", err)
	}
	
	// Create RateLimitPolicy for request-based limits
	if err := pe.CreateTeamRequestRateLimit(teamID, fmt.Sprintf("team-%s-request-limits", teamID), limits); err != nil {
		return fmt.Errorf("failed to create request rate limit policy: %w", err)
	}
	
	return nil
}

// CreateTeamTokenRateLimit creates a team-specific TokenRateLimitPolicy
func (pe *PolicyEngine) CreateTeamTokenRateLimit(teamID, policyName string, limits *TierLimits) error {
	// Skip creating policy if unlimited tier
	if limits.TokenLimit == -1 {
		log.Printf("Team %s using unlimited tier - no rate limit policy needed", teamID)
		return nil
	}
	
	// Skip creating policy for default team - let it use the default unlimited policy
	if teamID == "default" {
		log.Printf("Skipping policy creation for default team - using default unlimited policy")
		return nil
	}

	// Define the TokenRateLimitPolicy resource
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	// Create the policy manifest
	policy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kuadrant.io/v1alpha1",
			"kind":       "TokenRateLimitPolicy",
			"metadata": map[string]interface{}{
				"name":      policyName,
				"namespace": pe.Namespace,
				"labels": map[string]interface{}{
					"maas/managed-by":    "key-manager",
					"maas/team-id":       teamID,
					"maas/resource-type": "team-rate-limit",
				},
				"annotations": map[string]interface{}{
					"maas/created-at": time.Now().Format(time.RFC3339),
					"maas/description": fmt.Sprintf("Rate limiting policy for team %s", teamID),
				},
			},
			"spec": map[string]interface{}{
				"targetRef": map[string]interface{}{
					"group": "gateway.networking.k8s.io",
					"kind":  "Gateway",
					"name":  pe.GatewayName,
				},
				"limits": map[string]interface{}{
					fmt.Sprintf("team-%s-tokens", teamID): map[string]interface{}{
						"rates": []map[string]interface{}{
							{
								"limit":  limits.TokenLimit,
								"window": limits.TokenWindow,
							},
						},
						"counters": []map[string]interface{}{
							{
								"expression": "auth.identity.userid",
							},
						},
						"when": []map[string]interface{}{
							{
								"predicate": fmt.Sprintf("has(auth.identity.metadata.labels) && auth.identity.metadata.labels[\"maas/team-id\"] == \"%s\"", teamID),
							},
						},
					},
				},
			},
		},
	}

	// Create the policy using dynamic client (or update if it exists)
	_, err := pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Create(
		context.Background(), policy, metav1.CreateOptions{})
	if err != nil {
		// If policy already exists, get the existing one and update it
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("TokenRateLimitPolicy %s already exists, fetching for update", policyName)
			
			// Get existing policy to obtain resource version
			existing, getErr := pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Get(
				context.Background(), policyName, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("failed to get existing TokenRateLimitPolicy for update: %w", getErr)
			}
			
			// Preserve resource version and UID for update
			policy.SetResourceVersion(existing.GetResourceVersion())
			policy.SetUID(existing.GetUID())
			
			_, updateErr := pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Update(
				context.Background(), policy, metav1.UpdateOptions{})
			if updateErr != nil {
				return fmt.Errorf("failed to update existing TokenRateLimitPolicy: %w", updateErr)
			}
			log.Printf("Updated existing TokenRateLimitPolicy: %s for team %s (limit: %d tokens/%s)", 
				policyName, teamID, limits.TokenLimit, limits.TokenWindow)
			return nil
		}
		return fmt.Errorf("failed to create TokenRateLimitPolicy: %w", err)
	}

	log.Printf("Created team TokenRateLimitPolicy: %s for team %s (limit: %d tokens/%s)", 
		policyName, teamID, limits.TokenLimit, limits.TokenWindow)
	return nil
}

// CreateTeamRequestRateLimit creates a team-specific RateLimitPolicy for request-based limits
func (pe *PolicyEngine) CreateTeamRequestRateLimit(teamID, policyName string, limits *TierLimits) error {
	// Skip creating policy if unlimited tier
	if limits.RequestLimit == -1 {
		log.Printf("Team %s using unlimited tier - no request rate limit policy needed", teamID)
		return nil
	}
	
	// Skip creating policy for default team - let it use the default unlimited policy
	if teamID == "default" {
		log.Printf("Skipping request policy creation for default team - using default unlimited policy")
		return nil
	}

	// Define the RateLimitPolicy resource
	rateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1",
		Resource: "ratelimitpolicies",
	}

	// Create the policy manifest
	policy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kuadrant.io/v1",
			"kind":       "RateLimitPolicy",
			"metadata": map[string]interface{}{
				"name":      policyName,
				"namespace": pe.Namespace,
				"labels": map[string]interface{}{
					"maas/managed-by":    "key-manager",
					"maas/team-id":       teamID,
					"maas/resource-type": "team-request-limit",
				},
				"annotations": map[string]interface{}{
					"maas/created-at": time.Now().Format(time.RFC3339),
					"maas/description": fmt.Sprintf("Request rate limiting policy for team %s", teamID),
				},
			},
			"spec": map[string]interface{}{
				"targetRef": map[string]interface{}{
					"group": "gateway.networking.k8s.io",
					"kind":  "Gateway",
					"name":  pe.GatewayName,
				},
				"limits": map[string]interface{}{
					fmt.Sprintf("team-%s-requests", teamID): map[string]interface{}{
						"rates": []map[string]interface{}{
							{
								"limit":  limits.RequestLimit,
								"window": limits.RequestWindow,
							},
						},
						"counters": []map[string]interface{}{
							{
								"expression": "auth.identity.userid",
							},
						},
						"when": []map[string]interface{}{
							{
								"predicate": fmt.Sprintf("has(auth.identity.metadata.labels) && auth.identity.metadata.labels[\"maas/team-id\"] == \"%s\"", teamID),
							},
						},
					},
				},
			},
		},
	}

	// Create the policy using dynamic client (or update if it exists)
	_, err := pe.KuadrantClient.Resource(rateLimitGVR).Namespace(pe.Namespace).Create(
		context.Background(), policy, metav1.CreateOptions{})
	if err != nil {
		// If policy already exists, get the existing one and update it
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("RateLimitPolicy %s already exists, fetching for update", policyName)
			
			// Get existing policy to obtain resource version
			existing, getErr := pe.KuadrantClient.Resource(rateLimitGVR).Namespace(pe.Namespace).Get(
				context.Background(), policyName, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("failed to get existing RateLimitPolicy for update: %w", getErr)
			}
			
			// Preserve resource version and UID for update
			policy.SetResourceVersion(existing.GetResourceVersion())
			policy.SetUID(existing.GetUID())
			
			_, updateErr := pe.KuadrantClient.Resource(rateLimitGVR).Namespace(pe.Namespace).Update(
				context.Background(), policy, metav1.UpdateOptions{})
			if updateErr != nil {
				return fmt.Errorf("failed to update existing RateLimitPolicy: %w", updateErr)
			}
			log.Printf("Updated existing RateLimitPolicy: %s for team %s (limit: %d requests/%s)", 
				policyName, teamID, limits.RequestLimit, limits.RequestWindow)
			return nil
		}
		return fmt.Errorf("failed to create RateLimitPolicy: %w", err)
	}

	log.Printf("Created team RateLimitPolicy: %s for team %s (limit: %d requests/%s)", 
		policyName, teamID, limits.RequestLimit, limits.RequestWindow)
	return nil
}

// DeleteTeamTokenRateLimit deletes a team TokenRateLimitPolicy
func (pe *PolicyEngine) DeleteTeamTokenRateLimit(policyName string) error {
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	err := pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Delete(
		context.Background(), policyName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete TokenRateLimitPolicy: %w", err)
	}

	log.Printf("Deleted team TokenRateLimitPolicy: %s", policyName)
	return nil
}

// DeleteTeamRequestRateLimit deletes a team RateLimitPolicy
func (pe *PolicyEngine) DeleteTeamRequestRateLimit(policyName string) error {
	rateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1beta3",
		Resource: "ratelimitpolicies",
	}

	err := pe.KuadrantClient.Resource(rateLimitGVR).Namespace(pe.Namespace).Delete(
		context.Background(), policyName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete RateLimitPolicy: %w", err)
	}

	log.Printf("Deleted team RateLimitPolicy: %s", policyName)
	return nil
}

// DeleteTeamPolicies deletes all team-related policies
func (pe *PolicyEngine) DeleteTeamPolicies(teamID string) error {
	// Delete token rate limit policy
	tokenPolicyName := fmt.Sprintf("team-%s-token-limits", teamID)
	if err := pe.DeleteTeamTokenRateLimit(tokenPolicyName); err != nil {
		log.Printf("Warning: Failed to delete token policy %s: %v", tokenPolicyName, err)
	}
	
	// Delete request rate limit policy
	requestPolicyName := fmt.Sprintf("team-%s-request-limits", teamID)
	if err := pe.DeleteTeamRequestRateLimit(requestPolicyName); err != nil {
		log.Printf("Warning: Failed to delete request policy %s: %v", requestPolicyName, err)
	}
	
	return nil
}

// UpdateTeamTokenRateLimitUsers updates a team TokenRateLimitPolicy when users change
func (pe *PolicyEngine) UpdateTeamTokenRateLimitUsers(teamID, policyName string) error {
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	// Get current policy
	policy, err := pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Get(
		context.Background(), policyName, metav1.GetOptions{})
	if err != nil {
		// Policy might not exist for unlimited teams - that's OK
		log.Printf("Team policy %s not found (might be unlimited tier): %v", policyName, err)
		return nil
	}

	// Update timestamp annotation
	metadata := policy.Object["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	annotations["maas/updated-at"] = time.Now().Format(time.RFC3339)

	// Update the policy
	_, err = pe.KuadrantClient.Resource(tokenRateLimitGVR).Namespace(pe.Namespace).Update(
		context.Background(), policy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update TokenRateLimitPolicy: %w", err)
	}

	log.Printf("Updated team TokenRateLimitPolicy: %s for team %s", policyName, teamID)
	return nil
}