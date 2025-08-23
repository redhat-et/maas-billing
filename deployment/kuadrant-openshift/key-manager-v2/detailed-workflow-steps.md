# Complete Team Management Workflow - Detailed Steps

## Overview

This document provides step-by-step instructions for implementing the complete team management workflow, from platform setup through API key usage. Each step includes specific commands, configurations, and validation steps.

## Prerequisites

- OpenShift cluster with Kuadrant operator installed
- kubectl/oc CLI tools configured
- Admin access to the platform
- Go development environment for key-manager enhancements

## Phase 1: Platform Setup and Default Policies

### Step 1.1: Create Default Policy Templates

Create the platform-wide default policy configuration:

```bash
# Create default policy ConfigMap
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: platform-default-policies
  namespace: llm
data:
  default-team-policy.yaml: |
    tier: "free"
    token_limit_per_hour: 10000
    token_limit_per_day: 50000
    budget_usd_monthly: 100.0
    models_allowed: ["simulator-model"]
    rate_limit_window: "1h"
    max_concurrent_requests: 5
  
  tier-standard-policy.yaml: |
    tier: "standard" 
    token_limit_per_hour: 50000
    token_limit_per_day: 500000
    budget_usd_monthly: 1000.0
    models_allowed: ["simulator-model", "qwen3-0-6b-instruct"]
    rate_limit_window: "1h"
    max_concurrent_requests: 10
    
  tier-premium-policy.yaml: |
    tier: "premium"
    token_limit_per_hour: 200000
    token_limit_per_day: 2000000  
    budget_usd_monthly: 5000.0
    models_allowed: ["simulator-model", "qwen3-0-6b-instruct", "premium-models"]
    rate_limit_window: "1h"
    max_concurrent_requests: 25
EOF
```

### Step 1.2: Verify Platform Readiness

```bash
# Check Kuadrant operator status
kubectl get pods -n kuadrant-system

# Verify existing gateway and policies
kubectl get gateway -n llm
kubectl get authpolicy -n llm
kubectl get tokenratelimitpolicy -n llm

# Check default policy ConfigMap
kubectl get configmap platform-default-policies -n llm -o yaml
```

## Phase 2: Team Creation Workflow

### Step 2.1: Create Team via API

**API Request:**
```bash
# Create a new team
curl -X POST https://key-manager.apps.cluster.com/v2/teams \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "team_name": "data-science",
    "description": "Data Science Team",
    "default_budget_usd": 1000.0,
    "default_tier": "standard"
  }'
```

### Step 2.2: Verify Team Creation

```bash
# Check team configuration secret was created
kubectl get secret team-data-science-config -n llm -o yaml

# Expected structure:
# metadata:
#   labels:
#     maas/resource-type: "team-config"
#     maas/team-id: "data-science"
#   annotations:
#     maas/team-name: "Data Science Team"
#     maas/default-budget: "1000.0"
#     maas/default-tier: "standard"
```

### Step 2.3: Verify Team Policies Were Applied

```bash
# Check team-specific AuthPolicy was created
kubectl get authpolicy team-data-science-auth -n llm -o yaml

# Check team-specific TokenRateLimitPolicy was created  
kubectl get tokenratelimitpolicy team-data-science-limits -n llm -o yaml

# Verify policy is targeting the correct gateway
kubectl describe authpolicy team-data-science-auth -n llm
```

## Phase 3: User Addition Workflow

### Step 3.1: Add User to Team

**API Request:**
```bash
# Add user to team
curl -X POST https://key-manager.apps.cluster.com/v2/teams/data-science/members \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_email": "alice@company.com",
    "role": "member",
    "custom_budget_usd": 500.0
  }'
```

### Step 3.2: Verify User Addition

```bash
# Check user-team membership secret was created
kubectl get secret member-alice-data-science -n llm -o yaml

# Expected structure:
# metadata:
#   labels:
#     maas/resource-type: "team-member"
#     maas/team-id: "data-science" 
#     maas/user-id: "alice"
#     maas/role: "member"
#   annotations:
#     maas/user-email: "alice@company.com"
#     maas/budget-usd: "500.0"
```

### Step 3.3: Verify User Policy Inheritance

```bash
# Check that team AuthPolicy was updated for user context
kubectl get authpolicy team-data-science-auth -n llm -o yaml | grep -A 10 "response:"

# Should include user identification in response filters
```

## Phase 4: API Key Creation Workflow

### Step 4.1: Create Team-Scoped API Key

**API Request:**
```bash
# Create API key for user in team
curl -X POST https://key-manager.apps.cluster.com/v2/teams/data-science/keys \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "alice",
    "alias": "alice-research-key",
    "models": ["qwen3-0-6b-instruct"],
    "budget_usd": 300.0
  }'
```

**Expected Response:**
```json
{
  "api_key": "abc123...xyz789",
  "user_id": "alice",
  "team_id": "data-science", 
  "secret_name": "apikey-alice-data-science-a1b2c3d4",
  "budget_usd": 300.0,
  "models_allowed": ["qwen3-0-6b-instruct"],
  "tier": "standard"
}
```

### Step 4.2: Verify API Key Secret Creation

```bash
# Check enhanced API key secret was created
kubectl get secret apikey-alice-data-science-a1b2c3d4 -n llm -o yaml

# Expected enhanced structure:
# metadata:
#   labels:
#     kuadrant.io/apikeys-by: rhcl-keys
#     maas/user-id: "alice"
#     maas/team-id: "data-science"
#     maas/team-role: "member"
#     maas/key-sha256: "f8d92a1b..."
#   annotations:
#     maas/team-name: "Data Science Team"
#     maas/user-email: "alice@company.com"
#     maas/budget-usd: "300.0"
#     maas/spend-current: "0.0"
#     maas/models-allowed: "qwen3-0-6b-instruct"
#     maas/tier: "standard"
#     kuadrant.io/groups: "team-data-science,tier-standard"
```

### Step 4.3: Verify Policy Updates

```bash
# Check that team AuthPolicy includes the new key
kubectl get authpolicy team-data-science-auth -n llm -o yaml | grep -A 20 "selector:"

# Should include label selector for team keys:
# selector:
#   matchLabels:
#     kuadrant.io/apikeys-by: rhcl-keys
#     maas/team-id: "data-science"

# Check TokenRateLimit policy includes team-specific limits
kubectl get tokenratelimitpolicy team-data-science-limits -n llm -o yaml
```

## Phase 5: API Key Usage and Validation

### Step 5.1: Test API Key Authentication

```bash
# Test API key with model request
curl -X POST https://qwen3-llm.apps.cluster.com/v1/chat/completions \
  -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-0-6b-instruct",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ],
    "max_tokens": 100
  }'
```

**Expected Success Response:**
```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1703123456,
  "model": "qwen3-0-6b-instruct",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant", 
        "content": "Hello! I'm doing well, thank you for asking..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 25,
    "total_tokens": 37
  }
}
```

### Step 5.2: Verify Token Counting and Rate Limiting

```bash
# Check token consumption was recorded
kubectl get secret apikey-alice-data-science-a1b2c3d4 -n llm -o jsonpath='{.metadata.annotations.maas/spend-current}'

# Should show updated spend amount

# Test rate limiting by making rapid requests
for i in {1..20}; do
  curl -X POST https://qwen3-llm.apps.cluster.com/v1/chat/completions \
    -H "Authorization: APIKEY $API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"test"}],"max_tokens":10}' &
done
wait

# Should see 429 Rate Limited responses after hitting team limits
```

### Step 5.3: Test Invalid API Key

```bash
# Test with invalid API key
curl -X POST https://qwen3-llm.apps.cluster.com/v1/chat/completions \
  -H "Authorization: APIKEY invalid-key-12345" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-0-6b-instruct",
    "messages": [{"role": "user", "content": "test"}]
  }'

# Expected: 401 Unauthorized
```

## Phase 6: Team Management Operations

### Step 6.1: List Team Members

```bash
# Get team details including members
curl -X GET https://key-manager.apps.cluster.com/v2/teams/data-science \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Expected response:
# {
#   "team_id": "data-science",
#   "team_name": "Data Science Team", 
#   "description": "Data Science Team",
#   "members": [
#     {
#       "user_id": "alice",
#       "role": "member",
#       "email": "alice@company.com",
#       "budget_usd": 500.0,
#       "joined_at": "2024-01-15T11:00:00Z"
#     }
#   ],
#   "keys": ["apikey-alice-data-science-a1b2c3d4"]
# }
```

### Step 6.2: List Team API Keys

```bash
# Get all API keys for team
curl -X GET https://key-manager.apps.cluster.com/v2/teams/data-science/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Using kubectl to verify
kubectl get secrets -n llm -l maas/team-id=data-science,kuadrant.io/apikeys-by=rhcl-keys
```

### Step 6.3: Update API Key Budget

```bash
# Update API key budget
curl -X PATCH https://key-manager.apps.cluster.com/v2/keys/apikey-alice-data-science-a1b2c3d4 \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "budget_usd": 750.0,
    "status": "active"
  }'

# Verify update
kubectl get secret apikey-alice-data-science-a1b2c3d4 -n llm -o jsonpath='{.metadata.annotations.maas/budget-usd}'
```

## Phase 7: Team Activity Monitoring

### Step 7.1: Get Team Usage Report

```bash
# Get team activity and spending
curl -X GET https://key-manager.apps.cluster.com/v2/teams/data-science/activity?start_date=2024-01-01&end_date=2024-01-31&group_by=user \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Expected response format:
# {
#   "team_id": "data-science",
#   "period": {"start": "2024-01-01", "end": "2024-01-31"},
#   "summary": {
#     "total_requests": 1250,
#     "total_tokens": 125000,
#     "total_cost_usd": 45.50,
#     "unique_users": 1
#   },
#   "usage_by_user": [
#     {
#       "user_id": "alice",
#       "requests": 1250,
#       "tokens": 125000,
#       "cost_usd": 45.50,
#       "models_used": ["qwen3-0-6b-instruct"]
#     }
#   ]
# }
```

## Phase 8: Cleanup and Resource Management

### Step 8.1: Remove User from Team

```bash
# Remove user from team
curl -X DELETE https://key-manager.apps.cluster.com/v2/teams/data-science/members/alice \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Verify membership secret is removed
kubectl get secret member-alice-data-science -n llm
# Should return: Error from server (NotFound)

# Check that user's API keys are deactivated
kubectl get secret apikey-alice-data-science-a1b2c3d4 -n llm -o jsonpath='{.metadata.annotations.maas/status}'
# Should return: "inactive" or secret should be deleted
```

### Step 8.2: Delete API Key

```bash
# Delete specific API key
curl -X DELETE https://key-manager.apps.cluster.com/v2/keys/apikey-alice-data-science-a1b2c3d4 \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Verify key secret is removed
kubectl get secret apikey-alice-data-science-a1b2c3d4 -n llm
# Should return: Error from server (NotFound)

# Verify policies are updated
kubectl get authpolicy team-data-science-auth -n llm -o yaml
# Should no longer reference the deleted key
```

### Step 8.3: Delete Team

```bash
# Delete entire team (removes all members and keys)
curl -X DELETE https://key-manager.apps.cluster.com/v2/teams/data-science \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Verify all team resources are cleaned up
kubectl get secrets -n llm -l maas/team-id=data-science
kubectl get authpolicy team-data-science-auth -n llm
kubectl get tokenratelimitpolicy team-data-science-limits -n llm
# All should return: No resources found
```

## Troubleshooting Steps

### Issue: API Key Not Working

1. **Check Authentication:**
   ```bash
   # Verify key secret exists
   kubectl get secrets -n llm -l kuadrant.io/apikeys-by=rhcl-keys
   
   # Check AuthPolicy includes key selector
   kubectl get authpolicy -n llm -o yaml | grep -A 10 selector
   ```

2. **Check Policy Synchronization:**
   ```bash
   # Verify Kuadrant operator logs
   kubectl logs -n kuadrant-system deployment/kuadrant-operator
   
   # Check gateway configuration
   kubectl get gateway inference-gateway -n llm -o yaml
   ```

### Issue: Rate Limiting Not Working

1. **Check TokenRateLimit Policy:**
   ```bash
   # Verify policy exists and is correctly configured
   kubectl get tokenratelimitpolicy -n llm -o yaml
   
   # Check policy targets correct gateway
   kubectl describe tokenratelimitpolicy -n llm
   ```

2. **Check Rate Limit Counters:**
   ```bash
   # Check if counters are being updated
   kubectl logs -n kuadrant-system deployment/kuadrant-operator | grep -i rate
   ```

### Issue: Team Policies Not Applied

1. **Check Policy Creation:**
   ```bash
   # Verify all expected resources exist
   kubectl get secrets,authpolicy,tokenratelimitpolicy -n llm -l maas/team-id=TEAM_NAME
   ```

2. **Check Key Manager Logs:**
   ```bash
   # Check for policy application errors
   kubectl logs -n platform-services deployment/key-manager | grep -i policy
   ```

## Validation Checklist

### Team Creation Validation
- [ ] Team config secret created with correct labels
- [ ] Team AuthPolicy generated and applied
- [ ] Team TokenRateLimit policy created
- [ ] Policies target correct gateway
- [ ] Default tier policies applied

### User Addition Validation  
- [ ] User-team membership secret created
- [ ] User inherits team policies
- [ ] Team policies updated for user context
- [ ] User can create API keys

### API Key Creation Validation
- [ ] Enhanced key secret created with team labels
- [ ] Key included in team AuthPolicy
- [ ] TokenRateLimit updated for key
- [ ] Key authentication works
- [ ] Rate limiting enforced

### Usage Validation
- [ ] API requests succeed with valid key
- [ ] Token consumption tracked
- [ ] Rate limiting triggers at limits
- [ ] Invalid keys rejected
- [ ] Budget tracking functional