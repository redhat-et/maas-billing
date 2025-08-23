# Policy Application Decisions and Recommendations

## Executive Summary

After analyzing TokenRateLimitPolicy structure, alternative approaches, and default policy workflows, this document provides final recommendations for implementing Kuadrant policy management in the MaaS platform.

## Key Decision Points

### 1. Policy Application Location: Go Key-Manager ✅

**Decision**: Policies should be applied through the Go key-manager application using Kubernetes dynamic client APIs.

**Rationale**:
- TokenRateLimitPolicy requires complex, dynamic CEL expressions based on team/user context
- Real-time policy updates needed for team/user lifecycle management
- Seamless integration with existing API key secret creation workflow
- Direct Kubernetes API control for immediate synchronization

### 2. Expression Language: CEL (Not OPA/Rego) ✅

**Decision**: Use CEL (Common Expression Language) for all policy predicates and counters.

**Findings**:
- TokenRateLimitPolicy only supports CEL, not OPA/Rego
- CEL provides sufficient expression power for our use cases:
  - Team-based predicates: `auth.identity.metadata.labels.maas/team-id == "data-science"`
  - User-based counters: `auth.identity.metadata.labels.maas/user-id + "-" + auth.identity.metadata.labels.maas/team-id`
  - Model-based limits: `requestBodyJSON("model") == "qwen3-0-6b-instruct"`
  - Budget enforcement: `auth.identity.metadata.annotations.maas/budget-usd`

### 3. Default Policy Management: ConfigMap + API Combination ✅

**Decision**: Hybrid approach using ConfigMaps for templates and Go application for dynamic generation.

**Implementation**:
```yaml
# Static policy templates in ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: platform-default-policies
  namespace: llm
data:
  tier-standard-policy.yaml: |
    token_limit_per_hour: 50000
    models_allowed: ["simulator-model", "qwen3-0-6b-instruct"]
    # ... template values
```

```go
// Dynamic policy generation in Go app
func (pe *PolicyEngine) generateTeamRateLimitPolicy(teamID string, policy *PolicyTemplate) (*unstructured.Unstructured, error) {
    return &unstructured.Unstructured{
        Object: map[string]interface{}{
            "spec": map[string]interface{}{
                "limits": map[string]interface{}{
                    fmt.Sprintf("team-%s-hourly", teamID): map[string]interface{}{
                        "rates": []interface{}{
                            map[string]interface{}{
                                "limit":  policy.TokenLimitPerHour,
                                "window": "1h",
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
                },
            },
        },
    }, nil
}
```

## Implementation Architecture

### 1. Policy Lifecycle Management

```go
// Complete policy lifecycle in key-manager
type PolicyLifecycleManager struct {
    kuadrantClient  dynamic.Interface
    templateCache   map[string]*PolicyTemplate
    eventRecorder   record.EventRecorder
}

// Team creation triggers policy application
func (km *KeyManager) createTeam(c *gin.Context) {
    // 1. Create team config secret
    teamSecret, err := km.createTeamConfigSecret(&req)
    
    // 2. Apply default tier policies
    err = km.policyEngine.ApplyTeamDefaultPolicies(req.TeamID, req.DefaultTier)
    
    // 3. Record events for audit trail
    km.eventRecorder.Event(teamSecret, corev1.EventTypeNormal, "PoliciesApplied",
        fmt.Sprintf("Default %s tier policies applied", req.DefaultTier))
}

// API key creation updates existing policies
func (km *KeyManager) createTeamKey(c *gin.Context) {
    // 1. Create enhanced API key secret with team metadata
    keySecret, err := km.createEnhancedKeySecret(teamID, &req, apiKey, teamMember)
    
    // 2. Update team policies to include new key
    err = km.policyEngine.UpdateTeamPoliciesForNewKey(teamID, keySecret)
}
```

### 2. CEL Expression Patterns

**Team-Based Rate Limiting**:
```yaml
limits:
  team-data-science-hourly:
    rates:
    - limit: 50000
      window: 1h
    counters:
    - expression: 'auth.identity.metadata.labels.maas/team-id'
    when:
    - selector: 'auth.identity.metadata.labels.maas/team-id'
      operator: eq
      value: "data-science"
```

**User-Within-Team Limiting**:
```yaml
limits:
  team-data-science-per-user:
    rates:
    - limit: 12500  # 25% of team limit
      window: 1h
    counters:
    - expression: 'auth.identity.metadata.labels.maas/user-id + "-" + auth.identity.metadata.labels.maas/team-id'
    when:
    - selector: 'auth.identity.metadata.labels.maas/team-id'
      operator: eq
      value: "data-science"
```

**Model-Specific Limiting**:
```yaml
limits:
  premium-model-limits:
    rates:
    - limit: 5000   # Lower limit for expensive models
      window: 1h
    counters:
    - expression: 'auth.identity.metadata.labels.maas/team-id + "-" + requestBodyJSON("model")'
    when:
    - selector: 'requestBodyJSON("model")'
      operator: matches
      value: ".*premium.*"
    - selector: 'auth.identity.metadata.annotations.kuadrant\.io/groups'
      operator: matches
      value: "tier-premium"
```

**Budget-Based Limiting**:
```yaml
limits:
  budget-enforcement:
    rates:
    - limit: 1000   # Conservative limit when approaching budget
      window: 1h
    counters:
    - expression: 'auth.identity.metadata.labels.maas/user-id'
    when:
    - selector: 'float(auth.identity.metadata.annotations.maas/spend-current) / float(auth.identity.metadata.annotations.maas/budget-usd)'
      operator: gt
      value: "0.8"  # When 80% of budget used
```

### 3. Policy Synchronization Strategy

**Immediate Synchronization**:
- Team creation/deletion → Immediate policy apply/delete
- API key creation → Immediate policy update to include key selector
- Budget updates → Immediate policy limit adjustments

**Batch Operations**:
- Admin policy template updates → Notification to teams, opt-in synchronization
- Platform-wide policy changes → Staged rollout with validation

**Error Handling**:
```go
func (pe *PolicyEngine) ApplyTeamPoliciesWithRollback(teamID, tier string) error {
    appliedResources := make([]ResourceRef, 0)
    
    // Apply AuthPolicy
    authPolicy, err := pe.generateAndApplyAuthPolicy(teamID, tier)
    if err != nil {
        return err
    }
    appliedResources = append(appliedResources, ResourceRef{Kind: "AuthPolicy", Name: authPolicy.GetName()})
    
    // Apply TokenRateLimit
    rateLimitPolicy, err := pe.generateAndApplyRateLimitPolicy(teamID, tier)
    if err != nil {
        pe.rollbackResources(appliedResources)  // Cleanup on failure
        return err
    }
    appliedResources = append(appliedResources, ResourceRef{Kind: "TokenRateLimitPolicy", Name: rateLimitPolicy.GetName()})
    
    // Wait for policies to be ready
    err = pe.waitForPoliciesReady(appliedResources, 30*time.Second)
    if err != nil {
        pe.rollbackResources(appliedResources)
        return fmt.Errorf("policies failed to become ready: %w", err)
    }
    
    return nil
}
```

## Alternative Approaches Considered and Rejected

### 1. GitOps Approach - Rejected

**Reasons**:
- Too slow for dynamic team/user operations
- Difficult to implement complex CEL expressions in static YAML
- Manual Git workflow incompatible with API-driven team management
- Merge conflicts when multiple teams create keys simultaneously

### 2. Custom Operator - Rejected

**Reasons**:
- Excessive complexity for current requirements
- Additional maintenance burden
- Not enough benefit over direct Kubernetes API usage
- Would duplicate logic already in key-manager

### 3. OPA/Rego Integration - Not Available

**Reasons**:
- TokenRateLimitPolicy only supports CEL, not OPA/Rego
- CEL provides sufficient expression power for our use cases
- No need for external policy engine integration

## Implementation Roadmap

### Phase 1: Core Policy Integration (Week 1-2)
1. ✅ Enhance KeyManager struct with policy management fields
2. ✅ Implement PolicyEngine with template loading
3. ✅ Add team creation policy application
4. ✅ Implement CEL-based TokenRateLimitPolicy generation

### Phase 2: API Key Integration (Week 3)
1. Enhance API key creation to update team policies
2. Implement policy updates for key deletion
3. Add team membership changes policy updates
4. Implement policy validation endpoints

### Phase 3: Advanced Features (Week 4)
1. Budget-based rate limiting with CEL expressions
2. Model-specific rate limiting
3. Policy compliance monitoring
4. Admin policy template management APIs

### Phase 4: Production Readiness (Week 5-6)
1. Comprehensive error handling and rollback
2. Policy health monitoring and alerting
3. Performance optimization
4. Integration testing and validation

## Configuration Requirements

### 1. RBAC Permissions
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: key-manager-policy-manager
rules:
- apiGroups: ["kuadrant.io"]
  resources: ["authpolicies", "tokenratelimitpolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create"]
```

### 2. Environment Variables
```yaml
env:
- name: GATEWAY_NAME
  value: "inference-gateway"
- name: GATEWAY_NAMESPACE  
  value: "llm"
- name: POLICY_TEMPLATE_CONFIGMAP
  value: "platform-default-policies"
- name: ENABLE_POLICY_MANAGEMENT
  value: "true"
```

## Monitoring and Observability

### 1. Key Metrics
- Policy application success/failure rates
- Policy synchronization latency
- Active team policies count
- Policy compliance status

### 2. Health Checks
- Policy readiness validation
- Template loading status  
- Kuadrant operator connectivity
- Policy drift detection

### 3. Audit Trail
- All policy operations logged as Kubernetes events
- Policy template changes tracked
- Team policy inheritance recorded

## Risk Mitigation

### 1. Policy Application Failures
- **Risk**: Team creation succeeds but policy application fails
- **Mitigation**: Atomic operations with rollback on failure

### 2. Policy Drift
- **Risk**: Policies modified outside of key-manager
- **Mitigation**: Regular compliance checking and drift detection

### 3. Performance Impact
- **Risk**: Policy operations slow down team/key management
- **Mitigation**: Asynchronous policy updates where possible, caching

### 4. RBAC Security
- **Risk**: Over-privileged key-manager service
- **Mitigation**: Minimal RBAC permissions, namespace-scoped where possible

## Conclusion

The Go key-manager application approach provides the optimal balance of flexibility, performance, and integration for dynamic Kuadrant policy management in the MaaS platform. Using CEL expressions within TokenRateLimitPolicy enables sophisticated rate limiting scenarios while maintaining real-time responsiveness to team and user lifecycle events.