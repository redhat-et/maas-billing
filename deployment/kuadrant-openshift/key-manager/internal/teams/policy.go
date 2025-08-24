package teams

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// PolicyManager handles Kuadrant policy operations
type PolicyManager struct {
	kuadrantClient           dynamic.Interface
	clientset                *kubernetes.Clientset
	keyNamespace             string
	tokenRateLimitPolicyName string
	authPolicyName           string
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager(kuadrantClient dynamic.Interface, clientset *kubernetes.Clientset, keyNamespace, tokenRateLimitPolicyName, authPolicyName string) *PolicyManager {
	return &PolicyManager{
		kuadrantClient:           kuadrantClient,
		clientset:                clientset,
		keyNamespace:             keyNamespace,
		tokenRateLimitPolicyName: tokenRateLimitPolicyName,
		authPolicyName:           authPolicyName,
	}
}

// AddTeamToAuthPolicy adds a team policy to the AuthPolicy rego rules
func (p *PolicyManager) AddTeamToAuthPolicy(policyName string) error {
	return p.updateAuthPolicyForTeam(policyName, true)
}

// RemoveTeamFromAuthPolicy removes a team policy from the AuthPolicy rego rules
func (p *PolicyManager) RemoveTeamFromAuthPolicy(policyName string) error {
	return p.updateAuthPolicyForTeam(policyName, false)
}

// AddTeamToTokenRateLimit adds a team policy to the TokenRateLimitPolicy
func (p *PolicyManager) AddTeamToTokenRateLimit(policyName string, tokenLimit int, timeWindow string) error {
	return p.updateTokenRateLimitPolicyForTeam(policyName, true, tokenLimit, timeWindow)
}

// RemoveTeamFromTokenRateLimit removes a team policy from the TokenRateLimitPolicy
func (p *PolicyManager) RemoveTeamFromTokenRateLimit(policyName string) error {
	return p.updateTokenRateLimitPolicyForTeam(policyName, false, 0, "")
}

// PolicyExists checks if a policy exists in the TokenRateLimitPolicy
func (p *PolicyManager) PolicyExists(policyName string) bool {
	_, _, err := p.GetPolicyLimits(policyName)
	return err == nil
}

// GetPolicyLimits retrieves the current token limits for a policy
func (p *PolicyManager) GetPolicyLimits(policyName string) (int, string, error) {
	// Define TokenRateLimitPolicy GVR
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	// Get the current TokenRateLimitPolicy
	policyObj, err := p.kuadrantClient.Resource(tokenRateLimitGVR).Namespace(p.keyNamespace).Get(
		context.Background(), p.tokenRateLimitPolicyName, metav1.GetOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("failed to get TokenRateLimitPolicy: %w", err)
	}

	if spec, ok := policyObj.Object["spec"].(map[string]interface{}); ok {
		if limits, ok := spec["limits"].(map[string]interface{}); ok {
			limitName := fmt.Sprintf("%s", policyName)
			
			if limitConfig, exists := limits[limitName]; exists {
				if limitMap, ok := limitConfig.(map[string]interface{}); ok {
					if rates, ok := limitMap["rates"].([]interface{}); ok && len(rates) > 0 {
						if rate, ok := rates[0].(map[string]interface{}); ok {
							tokenLimit := 100000 // default
							timeWindow := "1h"   // default
							
							if limit, ok := rate["limit"].(float64); ok {
								tokenLimit = int(limit)
							}
							if window, ok := rate["window"].(string); ok {
								timeWindow = window
							}
							
							return tokenLimit, timeWindow, nil
						}
					}
				}
			} else {
				return 0, "", fmt.Errorf("policy '%s' does not exist in TokenRateLimitPolicy", policyName)
			}
		}
	}

	return 0, "", fmt.Errorf("failed to parse TokenRateLimitPolicy structure")
}

// RestartKuadrantComponents restarts Authorino and Kuadrant operator
func (p *PolicyManager) RestartKuadrantComponents() error {
	// Restart Authorino deployment
	err := p.restartDeployment("kuadrant-system", "authorino")
	if err != nil {
		log.Printf("Warning: Failed to restart Authorino: %v", err)
	} else {
		log.Printf("Successfully triggered Authorino deployment restart")
	}

	// Restart Kuadrant operator
	err = p.restartDeployment("kuadrant-system", "kuadrant-operator-controller-manager")
	if err != nil {
		log.Printf("Warning: Failed to restart Kuadrant operator: %v", err)
	} else {
		log.Printf("Successfully triggered Kuadrant operator deployment restart")
	}

	// Verify policies are actually loaded
	log.Printf("Waiting for Kuadrant components to restart and policies to be enforced...")
	err = p.verifyPolicyReload()
	if err != nil {
		log.Printf("Warning: Policy reload verification failed: %v", err)
	} else {
		log.Printf("Kuadrant components restart and policy reload verified")
	}

	return nil
}

// updateAuthPolicyForTeam updates the AuthPolicy rego rules to include/exclude a team's policy
func (p *PolicyManager) updateAuthPolicyForTeam(policyName string, add bool) error {
	// Define AuthPolicy GVR
	authPolicyGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1",
		Resource: "authpolicies",
	}

	// Get the current AuthPolicy
	authPolicyObj, err := p.kuadrantClient.Resource(authPolicyGVR).Namespace(p.keyNamespace).Get(
		context.Background(), p.authPolicyName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get AuthPolicy: %w", err)
	}

	// Parse existing allowed groups from rego
	allowedGroups := []string{"free", "premium", "enterprise"} // default groups

	// Extract current rego to see if there are additional groups
	if spec, ok := authPolicyObj.Object["spec"].(map[string]interface{}); ok {
		if rules, ok := spec["rules"].(map[string]interface{}); ok {
			if auth, ok := rules["authorization"].(map[string]interface{}); ok {
				if allowGroups, ok := auth["allow-groups"].(map[string]interface{}); ok {
					if opa, ok := allowGroups["opa"].(map[string]interface{}); ok {
						if rego, ok := opa["rego"].(string); ok {
							// Parse existing groups from rego (simple regex)
							// Look for: allow { groups[_] == "groupname" }
							lines := strings.Split(rego, "\n")
							for _, line := range lines {
								if strings.Contains(line, "allow { groups[_] ==") {
									start := strings.Index(line, "\"")
									end := strings.LastIndex(line, "\"")
									if start != -1 && end != -1 && end > start {
										groupName := line[start+1 : end]
										// Add to allowedGroups if not already there
										found := false
										for _, existing := range allowedGroups {
											if existing == groupName {
												found = true
												break
											}
										}
										if !found && groupName != "" {
											allowedGroups = append(allowedGroups, groupName)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Add or remove the policy name
	if add {
		// Check if already exists
		found := false
		for _, group := range allowedGroups {
			if group == policyName {
				found = true
				break
			}
		}
		if !found {
			allowedGroups = append(allowedGroups, policyName)
		}
	} else {
		// Remove the policy name
		var newGroups []string
		for _, group := range allowedGroups {
			if group != policyName {
				newGroups = append(newGroups, group)
			}
		}
		allowedGroups = newGroups
	}

	// Generate new rego rules
	newRego := `groups := split(object.get(input.auth.identity.metadata.annotations, "kuadrant.io/groups", ""), ",")`
	for _, group := range allowedGroups {
		newRego += fmt.Sprintf("\nallow { groups[_] == \"%s\" }", group)
	}

	// Update the AuthPolicy spec
	if spec, ok := authPolicyObj.Object["spec"].(map[string]interface{}); ok {
		if rules, ok := spec["rules"].(map[string]interface{}); ok {
			if rules["authorization"] == nil {
				rules["authorization"] = make(map[string]interface{})
			}
			auth := rules["authorization"].(map[string]interface{})
			auth["allow-groups"] = map[string]interface{}{
				"opa": map[string]interface{}{
					"rego": newRego,
				},
			}
		}
	}

	// Apply the updated AuthPolicy
	_, err = p.kuadrantClient.Resource(authPolicyGVR).Namespace(p.keyNamespace).Update(
		context.Background(), authPolicyObj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update AuthPolicy: %w", err)
	}

	log.Printf("Updated AuthPolicy to %s group: %s", map[bool]string{true: "include", false: "exclude"}[add], policyName)
	return nil
}

// updateTokenRateLimitPolicyForTeam updates the TokenRateLimitPolicy limits to include/exclude a team's policy
func (p *PolicyManager) updateTokenRateLimitPolicyForTeam(policyName string, add bool, tokenLimit int, timeWindow string) error {
	// Define TokenRateLimitPolicy GVR
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	// Get the current TokenRateLimitPolicy
	policyObj, err := p.kuadrantClient.Resource(tokenRateLimitGVR).Namespace(p.keyNamespace).Get(
		context.Background(), p.tokenRateLimitPolicyName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get TokenRateLimitPolicy: %w", err)
	}

	if spec, ok := policyObj.Object["spec"].(map[string]interface{}); ok {
		if limits, ok := spec["limits"].(map[string]interface{}); ok {
			limitName := fmt.Sprintf("%s", policyName)

			if add {
				// Set default values if not provided
				if tokenLimit <= 0 {
					tokenLimit = 100000
				}
				if timeWindow == "" {
					timeWindow = "1h"
				}

				// Add new limit for the team
				limits[limitName] = map[string]interface{}{
					"rates": []map[string]interface{}{
						{
							"limit":  tokenLimit,
							"window": timeWindow,
						},
					},
					"when": []map[string]interface{}{
						{
							"predicate": fmt.Sprintf("auth.identity.groups.split(\",\").exists(g, g == \"%s\")", policyName),
						},
					},
					"counters": []map[string]interface{}{
						{
							"expression": "auth.identity.userid",
						},
					},
				}
			} else {
				// Remove limit for the team
				delete(limits, limitName)
			}
		}
	}

	// Apply the updated TokenRateLimitPolicy
	_, err = p.kuadrantClient.Resource(tokenRateLimitGVR).Namespace(p.keyNamespace).Update(
		context.Background(), policyObj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update TokenRateLimitPolicy: %w", err)
	}

	log.Printf("Updated TokenRateLimitPolicy to %s group: %s", map[bool]string{true: "include", false: "exclude"}[add], policyName)
	return nil
}

// restartDeployment restarts a deployment by patching it with a restart annotation
func (p *PolicyManager) restartDeployment(namespace, deploymentName string) error {
	// Create patch to trigger rolling restart
	restartPatch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().Format(time.RFC3339)))

	// Apply patch to deployment
	_, err := p.clientset.AppsV1().Deployments(namespace).Patch(
		context.Background(),
		deploymentName,
		types.StrategicMergePatchType,
		restartPatch,
		metav1.PatchOptions{})

	return err
}

// verifyPolicyReload checks if AuthPolicy and TokenRateLimitPolicy are in Enforced state
func (p *PolicyManager) verifyPolicyReload() error {
	// Define policy GVRs
	authPolicyGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1",
		Resource: "authpolicies",
	}
	tokenRateLimitGVR := schema.GroupVersionResource{
		Group:    "kuadrant.io",
		Version:  "v1alpha1",
		Resource: "tokenratelimitpolicies",
	}

	// Check AuthPolicy status with timeout
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		authPolicy, err := p.kuadrantClient.Resource(authPolicyGVR).Namespace(p.keyNamespace).Get(
			context.Background(), p.authPolicyName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Warning: Failed to get AuthPolicy status: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Check if AuthPolicy is enforced
		if p.isPolicyEnforced(authPolicy.Object) {
			log.Printf("AuthPolicy is enforced and ready")
			break
		}

		log.Printf("Waiting for AuthPolicy to be enforced...")
		time.Sleep(2 * time.Second)
	}

	// Check TokenRateLimitPolicy status with timeout
	timeout = time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		tokenRatePolicy, err := p.kuadrantClient.Resource(tokenRateLimitGVR).Namespace(p.keyNamespace).Get(
			context.Background(), p.tokenRateLimitPolicyName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Warning: Failed to get TokenRateLimitPolicy status: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Check if TokenRateLimitPolicy is enforced
		if p.isPolicyEnforced(tokenRatePolicy.Object) {
			log.Printf("TokenRateLimitPolicy is enforced and ready")
			return nil
		}

		log.Printf("Waiting for TokenRateLimitPolicy to be enforced...")
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for policies to be enforced")
}

// isPolicyEnforced checks if a policy has Enforced status condition
func (p *PolicyManager) isPolicyEnforced(policyObj map[string]interface{}) bool {
	status, ok := policyObj["status"].(map[string]interface{})
	if !ok {
		return false
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return false
	}

	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}

		conditionType, ok := conditionMap["type"].(string)
		if !ok || conditionType != "Enforced" {
			continue
		}

		conditionStatus, ok := conditionMap["status"].(string)
		if ok && conditionStatus == "True" {
			return true
		}
	}

	return false
}