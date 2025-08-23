# Default Policy Implementation Through Admin + User Workflow

## Overview

This document details how default policies are implemented and applied in the MaaS platform through a coordinated admin and user workflow. The system supports hierarchical policy inheritance with platform defaults, tier-specific policies, and team customizations.

## Policy Hierarchy and Inheritance

### 1. Policy Precedence Levels

```yaml
# Policy precedence (highest to lowest):
1. User-specific overrides (future enhancement)
2. Team-specific policies (dynamic, applied by key-manager)
3. Tier-based default policies (configured by admin)
4. Platform-wide default policies (configured by admin)
```

### 2. Policy Template Structure

The default policies are stored in ConfigMaps and loaded by the key-manager:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: platform-default-policies
  namespace: llm
  labels:
    maas/resource-type: "policy-templates"
    maas/managed-by: "key-manager"
data:
  # Free tier default policy
  tier-free-policy.yaml: |
    tier: "free"
    token_limit_per_hour: 10000
    token_limit_per_day: 50000
    token_limit_per_month: 1000000
    budget_usd_monthly: 100.0
    models_allowed: ["simulator-model"]
    rate_limit_window: "1h"
    burst_limit: 100
    max_concurrent_requests: 5
    enable_budget_enforcement: true
    
  # Standard tier default policy  
  tier-standard-policy.yaml: |
    tier: "standard"
    token_limit_per_hour: 50000
    token_limit_per_day: 500000
    token_limit_per_month: 10000000
    budget_usd_monthly: 1000.0
    models_allowed: ["simulator-model", "qwen3-0-6b-instruct"]
    rate_limit_window: "1h"
    burst_limit: 500
    max_concurrent_requests: 10
    enable_budget_enforcement: true
    
  # Premium tier default policy
  tier-premium-policy.yaml: |
    tier: "premium"
    token_limit_per_hour: 200000
    token_limit_per_day: 2000000
    token_limit_per_month: 50000000
    budget_usd_monthly: 5000.0
    models_allowed: ["simulator-model", "qwen3-0-6b-instruct", "premium-models"]
    rate_limit_window: "1h"
    burst_limit: 2000
    max_concurrent_requests: 25
    enable_budget_enforcement: true
    
  # Platform-wide fallback policy
  platform-default-policy.yaml: |
    tier: "platform-default"
    token_limit_per_hour: 1000
    token_limit_per_day: 5000
    token_limit_per_month: 100000
    budget_usd_monthly: 10.0
    models_allowed: ["simulator-model"]
    rate_limit_window: "1h"
    burst_limit: 10
    max_concurrent_requests: 1
    enable_budget_enforcement: true
```

## Admin Workflow: Policy Configuration

### 1. Platform Administrator Setup

```bash
# Step 1: Create platform-wide default policies
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: platform-default-policies
  namespace: llm
  labels:
    maas/resource-type: "policy-templates"
    maas/managed-by: "key-manager"
    maas/version: "v1.0"
data:
  # Policy templates as shown above
EOF

# Step 2: Create global gateway policy (applies to all unmatched requests)
kubectl apply -f - <<EOF
apiVersion: kuadrant.io/v1alpha1
kind: TokenRateLimitPolicy
metadata:
  name: platform-global-limits
  namespace: llm
  labels:
    maas/policy-type: "global"
    maas/managed-by: "admin"
spec:
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: inference-gateway
  defaults:
    global-fallback:
      rates:
      - limit: 1000
        window: 1h
      - limit: 5000  
        window: 24h
      counters:
      - expression: 'request.headers["authorization"]'
      when:
      - selector: 'request.headers["authorization"]'
        operator: exists
EOF

# Step 3: Verify key-manager can access policies
kubectl logs -n platform-services deployment/key-manager | grep "policy"
```

### 2. Admin API for Policy Management

The key-manager exposes admin endpoints for policy configuration:

```bash
# Get current default policies
curl -X GET https://key-manager.apps.cluster.com/v2/admin/policies/defaults \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Update tier-specific default policy
curl -X PUT https://key-manager.apps.cluster.com/v2/admin/policies/tiers/standard \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "token_limit_per_hour": 75000,
    "token_limit_per_day": 750000,
    "budget_usd_monthly": 1500.0,
    "models_allowed": ["simulator-model", "qwen3-0-6b-instruct", "claude-3-haiku"]
  }'

# Create new tier policy
curl -X POST https://key-manager.apps.cluster.com/v2/admin/policies/tiers \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tier": "enterprise",
    "token_limit_per_hour": 500000,
    "token_limit_per_day": 5000000,
    "budget_usd_monthly": 10000.0,
    "models_allowed": ["*"],
    "max_concurrent_requests": 50
  }'
```

## User Workflow: Policy Application and Inheritance

### 1. Team Creation with Default Policy Inheritance

When a team is created, the key-manager automatically applies the appropriate tier policy:

```go
// Team creation with policy inheritance
func (km *KeyManager) createTeam(c *gin.Context) {
    var req CreateTeamRequest
    // ... validation ...
    
    // Create team configuration
    teamSecret, err := km.createTeamConfigSecret(&req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team"})
        return
    }
    
    // Apply tier-based default policies
    err = km.policyEngine.ApplyTeamDefaultPolicies(req.TeamID, req.DefaultTier)
    if err != nil {
        // Rollback on failure
        km.clientset.CoreV1().Secrets(km.keyNamespace).Delete(
            context.Background(), teamSecret.Name, metav1.DeleteOptions{})
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply default policies"})
        return
    }
    
    // Record policy application event
    km.eventRecorder.Event(teamSecret, corev1.EventTypeNormal, "DefaultPoliciesApplied",
        fmt.Sprintf("Default %s tier policies applied to team %s", req.DefaultTier, req.TeamID))
    
    c.JSON(http.StatusOK, CreateTeamResponse{
        TeamID:           req.TeamID,
        DefaultTier:      req.DefaultTier,
        PoliciesApplied:  true,
        InheritedLimits: km.policyEngine.GetTierLimits(req.DefaultTier),
    })
}
```

### 2. Dynamic Policy Generation and Application

The key-manager dynamically generates TokenRateLimitPolicy based on default templates:

```go
// Generate team-specific TokenRateLimitPolicy from default template
func (pe *PolicyEngine) generateTeamRateLimitPolicy(teamID string, policy *PolicyTemplate) (*unstructured.Unstructured, error) {
    rateLimitPolicy := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "kuadrant.io/v1alpha1",
            "kind":       "TokenRateLimitPolicy",
            "metadata": map[string]interface{}{
                "name":      fmt.Sprintf("team-%s-limits", teamID),
                "namespace": "llm",
                "labels": map[string]interface{}{
                    "maas/team-id":     teamID,
                    "maas/policy-type": "team-rate-limit",
                    "maas/tier":        policy.Tier,
                    "maas/managed-by":  "key-manager",
                },
                "annotations": map[string]interface{}{
                    "maas/inherited-from":  fmt.Sprintf("tier-%s", policy.Tier),
                    "maas/applied-at":      time.Now().Format(time.RFC3339),
                },
            },
            "spec": map[string]interface{}{
                "targetRef": map[string]interface{}{
                    "group": "gateway.networking.k8s.io",
                    "kind":  "Gateway",
                    "name":  "inference-gateway",
                },
                "limits": map[string]interface{}{
                    // Hour-based limit for team
                    fmt.Sprintf("team-%s-hourly", teamID): map[string]interface{}{
                        "rates": []interface{}{
                            map[string]interface{}{
                                "limit":  policy.TokenLimitPerHour,
                                "window": policy.RateLimitWindow,
                            },
                        },
                        "counters": []interface{}{
                            map[string]interface{}{
                                "expression": "auth.identity.metadata.labels.maas/team-id",
                            },
                        },
                        "when": []interface{}{
                            map[string]interface{}{
                                "selector": "auth.identity.metadata.labels.maas/team-id",
                                "operator": "eq",
                                "value":    teamID,
                            },
                        },
                    },
                    // Daily limit for team  
                    fmt.Sprintf("team-%s-daily", teamID): map[string]interface{}{
                        "rates": []interface{}{
                            map[string]interface{}{
                                "limit":  policy.TokenLimitPerDay,
                                "window": "24h",
                            },
                        },
                        "counters": []interface{}{
                            map[string]interface{}{
                                "expression": "auth.identity.metadata.labels.maas/team-id",
                            },
                        },
                        "when": []interface{}{
                            map[string]interface{}{
                                "selector": "auth.identity.metadata.labels.maas/team-id", 
                                "operator": "eq",
                                "value":    teamID,
                            },
                        },
                    },
                    // Per-user limit within team
                    fmt.Sprintf("team-%s-per-user", teamID): map[string]interface{}{
                        "rates": []interface{}{
                            map[string]interface{}{
                                "limit":  policy.TokenLimitPerHour / 4, // 25% of team limit per user
                                "window": policy.RateLimitWindow,
                            },
                        },
                        "counters": []interface{}{
                            map[string]interface{}{
                                "expression": "auth.identity.metadata.labels.maas/user-id + \"-\" + auth.identity.metadata.labels.maas/team-id",
                            },
                        },
                        "when": []interface{}{
                            map[string]interface{}{
                                "selector": "auth.identity.metadata.labels.maas/team-id",
                                "operator": "eq", 
                                "value":    teamID,
                            },
                        },
                    },
                    // Model-specific limits (if configured)
                    fmt.Sprintf("team-%s-model-limits", teamID): pe.generateModelSpecificLimits(teamID, policy),
                },
            },
        },
    }
    
    return rateLimitPolicy, nil
}

// Generate model-specific rate limits
func (pe *PolicyEngine) generateModelSpecificLimits(teamID string, policy *PolicyTemplate) map[string]interface{} {
    modelLimits := make(map[string]interface{})
    
    for _, model := range policy.ModelsAllowed {
        // Different limits for different model types
        var modelLimit int
        switch {
        case strings.Contains(model, "premium"):
            modelLimit = policy.TokenLimitPerHour / 10 // Premium models get 10% of team limit
        case strings.Contains(model, "standard"):
            modelLimit = policy.TokenLimitPerHour / 2  // Standard models get 50% of team limit
        default:
            modelLimit = policy.TokenLimitPerHour      // Basic models get full team limit
        }
        
        modelLimits[fmt.Sprintf("model-%s", model)] = map[string]interface{}{
            "rates": []interface{}{
                map[string]interface{}{
                    "limit":  modelLimit,
                    "window": policy.RateLimitWindow,
                },
            },
            "counters": []interface{}{
                map[string]interface{}{
                    "expression": "auth.identity.metadata.labels.maas/team-id + \"-\" + request.url_path.getQueryParam('model')",
                },
            },
            "when": []interface{}{
                map[string]interface{}{
                    "selector": "auth.identity.metadata.labels.maas/team-id",
                    "operator": "eq",
                    "value":    teamID,
                },
                map[string]interface{}{
                    "selector": "request.url_path.getQueryParam('model')",
                    "operator": "eq",
                    "value":    model,
                },
            },
        }
    }
    
    return modelLimits
}
```

### 3. User API Key Creation with Policy Inheritance

When users create API keys, they inherit team policies and can optionally specify constraints:

```bash
# User creates API key with default team policy inheritance
curl -X POST https://key-manager.apps.cluster.com/v2/teams/data-science/keys \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "alice",
    "alias": "alice-research-key",
    "budget_usd": 300.0,
    "inherit_team_limits": true,
    "custom_limits": {
      "max_tokens_per_request": 4000,
      "preferred_models": ["qwen3-0-6b-instruct"]
    }
  }'

# Response includes inherited policy information
{
  "api_key": "abc123...xyz789",
  "user_id": "alice",
  "team_id": "data-science",
  "inherited_policies": {
    "tier": "standard",
    "team_hourly_limit": 50000,
    "user_hourly_limit": 12500,
    "models_allowed": ["simulator-model", "qwen3-0-6b-instruct"],
    "budget_enforcement": true
  },
  "custom_constraints": {
    "max_tokens_per_request": 4000,
    "preferred_models": ["qwen3-0-6b-instruct"]
  }
}
```

## Policy Update and Synchronization Workflow

### 1. Admin Policy Updates

When admins update default policies, existing teams can choose to inherit the changes:

```bash
# Admin updates standard tier policy
curl -X PUT https://key-manager.apps.cluster.com/v2/admin/policies/tiers/standard \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "token_limit_per_hour": 75000,
    "budget_usd_monthly": 1500.0,
    "apply_to_existing_teams": false,
    "notification_required": true
  }'

# Key-manager sends notifications to affected teams
{
  "policy_update": {
    "tier": "standard",
    "changes": ["token_limit_per_hour", "budget_usd_monthly"],
    "affected_teams": ["data-science", "ml-ops", "research"],
    "auto_apply": false
  }
}
```

### 2. Team Policy Opt-in Updates

Teams can opt-in to receive updated default policies:

```bash
# Team admin opts into policy updates
curl -X POST https://key-manager.apps.cluster.com/v2/teams/data-science/policies/sync \
  -H "Authorization: Bearer $TEAM_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "accept_policy_updates": true,
    "preserve_custom_settings": true,
    "update_source": "tier-standard-v1.1"
  }'

# Key-manager applies updated policies while preserving customizations
{
  "sync_result": {
    "policies_updated": ["team-data-science-limits"],
    "customizations_preserved": ["custom_models", "extended_budget"],
    "new_limits": {
      "token_limit_per_hour": 75000,
      "budget_usd_monthly": 1500.0
    }
  }
}
```

## Policy Validation and Compliance

### 1. Automated Policy Validation

The key-manager continuously validates that applied policies match defaults:

```go
// Policy compliance checker
func (pe *PolicyEngine) ValidatePolicyCompliance() error {
    // Get all team policies
    teamPolicies, err := pe.listTeamPolicies()
    if err != nil {
        return fmt.Errorf("failed to list team policies: %w", err)
    }
    
    complianceReport := PolicyComplianceReport{
        Timestamp: time.Now(),
        Teams:     make([]TeamComplianceStatus, 0),
    }
    
    for _, teamPolicy := range teamPolicies {
        teamID := teamPolicy.GetLabels()["maas/team-id"]
        tier := teamPolicy.GetLabels()["maas/tier"]
        
        // Get expected default policy
        defaultPolicy, exists := pe.defaultPolicyCache[tier]
        if !exists {
            pe.logger.Warn("No default policy found for tier", "tier", tier, "team", teamID)
            continue
        }
        
        // Compare actual vs expected policy
        compliance := pe.comparePolicyWithDefault(teamPolicy, defaultPolicy)
        complianceReport.Teams = append(complianceReport.Teams, TeamComplianceStatus{
            TeamID:     teamID,
            Tier:       tier,
            Compliant:  compliance.IsCompliant,
            Deviations: compliance.Deviations,
        })
    }
    
    // Store compliance report
    return pe.storeComplianceReport(complianceReport)
}
```

### 2. Policy Drift Detection

The system detects when team policies drift from their default templates:

```bash
# Get policy compliance report
curl -X GET https://key-manager.apps.cluster.com/v2/admin/policies/compliance \
  -H "Authorization: Bearer $ADMIN_TOKEN"

{
  "compliance_report": {
    "timestamp": "2024-01-15T10:30:00Z",
    "overall_compliance": 85.7,
    "teams": [
      {
        "team_id": "data-science",
        "tier": "standard", 
        "compliant": false,
        "deviations": [
          "token_limit_per_hour: 60000 (expected: 50000)",
          "models_allowed: includes custom model 'internal-llm'"
        ],
        "drift_reason": "Custom team requirements"
      },
      {
        "team_id": "ml-ops",
        "tier": "standard",
        "compliant": true,
        "deviations": []
      }
    ]
  }
}
```

This comprehensive default policy workflow ensures that:
1. Admins can easily configure platform-wide policy templates
2. Teams automatically inherit appropriate tier-based policies  
3. Users receive consistent policy enforcement based on team membership
4. Policy updates can be managed centrally while allowing team customizations
5. Compliance and drift are continuously monitored