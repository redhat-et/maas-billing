# Enhanced Key Manager Testing Guide

## Overview

This document provides comprehensive testing procedures for the enhanced key-manager service, covering team management, user management, enhanced API key creation, and policy integration.

## Prerequisites

- Enhanced key-manager deployed and running
- Admin API key configured
- Default policy templates loaded
- kubectl/oc access to cluster
- curl and jq installed for API testing

## Test Environment Setup

```bash
# Set up test environment variables
export ADMIN_KEY="your-admin-key-here"
export BASE_URL="https://key-manager.apps.cluster.com"
export TEST_TEAM_ID="test-team-$(date +%s)"
export TEST_USER_EMAIL="testuser@example.com"
export TEST_USER_ID="testuser"

# Verify service is running
curl -s "$BASE_URL/health" | jq .
```

## Test Categories

### 1. Health and Status Tests

#### Basic Health Check
```bash
echo "=== Basic Health Check ==="
curl -s "$BASE_URL/health" | jq .
# Expected: {"status":"healthy"}
```

#### Policy Health Check (if enabled)
```bash
echo "=== Policy Health Check ==="
curl -s -X GET "$BASE_URL/v2/admin/policies/health" \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
# Expected: overall_status: "healthy"
```

#### Default Policies Check
```bash
echo "=== Default Policies Check ==="
curl -s -X GET "$BASE_URL/v2/admin/policies/defaults" \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
# Expected: default_policies with tier configurations
```

### 2. Team Management Tests

#### Test 2.1: Create Team
```bash
echo "=== Test 2.1: Create Team ==="
TEAM_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"team_id\": \"$TEST_TEAM_ID\",
    \"team_name\": \"Test Team\",
    \"description\": \"Automated test team\",
    \"default_budget_usd\": 1000.0,
    \"default_tier\": \"standard\"
  }")

echo "$TEAM_RESPONSE" | jq .

# Validate response
if echo "$TEAM_RESPONSE" | jq -e '.team_id' > /dev/null; then
  echo "✅ Team creation successful"
else
  echo "❌ Team creation failed"
  exit 1
fi
```

#### Test 2.2: List Teams
```bash
echo "=== Test 2.2: List Teams ==="
TEAMS_LIST=$(curl -s -X GET "$BASE_URL/v2/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$TEAMS_LIST" | jq .

# Validate our test team is in the list
if echo "$TEAMS_LIST" | jq -e ".teams[] | select(.team_id == \"$TEST_TEAM_ID\")" > /dev/null; then
  echo "✅ Test team found in teams list"
else
  echo "❌ Test team not found in teams list"
fi
```

#### Test 2.3: Get Team Details
```bash
echo "=== Test 2.3: Get Team Details ==="
TEAM_DETAILS=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$TEAM_DETAILS" | jq .

# Validate team details
if echo "$TEAM_DETAILS" | jq -e '.team_id' > /dev/null; then
  echo "✅ Team details retrieved successfully"
else
  echo "❌ Failed to get team details"
fi
```

### 3. User Management Tests

#### Test 3.1: Add User to Team
```bash
echo "=== Test 3.1: Add User to Team ==="
USER_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/$TEST_TEAM_ID/members" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_email\": \"$TEST_USER_EMAIL\",
    \"role\": \"member\",
    \"custom_budget_usd\": 500.0
  }")

echo "$USER_RESPONSE" | jq .

# Validate response
if echo "$USER_RESPONSE" | jq -e '.user_id' > /dev/null; then
  TEST_USER_ID=$(echo "$USER_RESPONSE" | jq -r '.user_id')
  echo "✅ User added to team successfully. User ID: $TEST_USER_ID"
else
  echo "❌ Failed to add user to team"
  exit 1
fi
```

#### Test 3.2: List Team Members
```bash
echo "=== Test 3.2: List Team Members ==="
MEMBERS_LIST=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID/members" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$MEMBERS_LIST" | jq .

# Validate our test user is in the list
if echo "$MEMBERS_LIST" | jq -e ".members[] | select(.user_id == \"$TEST_USER_ID\")" > /dev/null; then
  echo "✅ Test user found in team members"
else
  echo "❌ Test user not found in team members"
fi
```

### 4. Enhanced API Key Creation Tests

#### Test 4.1: Create Team API Key
```bash
echo "=== Test 4.1: Create Team API Key ==="
KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/$TEST_TEAM_ID/keys" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$TEST_USER_ID\",
    \"alias\": \"test-api-key\",
    \"models\": [\"simulator-model\", \"qwen3-0-6b-instruct\"],
    \"budget_usd\": 200.0,
    \"inherit_team_limits\": true,
    \"custom_limits\": {
      \"max_tokens_per_request\": 4000
    }
  }")

echo "$KEY_RESPONSE" | jq .

# Extract API key for later tests
if echo "$KEY_RESPONSE" | jq -e '.api_key' > /dev/null; then
  TEST_API_KEY=$(echo "$KEY_RESPONSE" | jq -r '.api_key')
  TEST_SECRET_NAME=$(echo "$KEY_RESPONSE" | jq -r '.secret_name')
  echo "✅ Team API key created successfully"
  echo "API Key: ${TEST_API_KEY:0:20}..."
  echo "Secret Name: $TEST_SECRET_NAME"
else
  echo "❌ Failed to create team API key"
  exit 1
fi
```

#### Test 4.2: List Team API Keys
```bash
echo "=== Test 4.2: List Team API Keys ==="
KEYS_LIST=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID/keys" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$KEYS_LIST" | jq .

# Validate our test key is in the list
if echo "$KEYS_LIST" | jq -e ".keys[] | select(.secret_name == \"$TEST_SECRET_NAME\")" > /dev/null; then
  echo "✅ Test API key found in team keys list"
else
  echo "❌ Test API key not found in team keys list"
fi
```

#### Test 4.3: Update API Key
```bash
echo "=== Test 4.3: Update API Key ==="
UPDATE_RESPONSE=$(curl -s -X PATCH "$BASE_URL/v2/keys/$TEST_SECRET_NAME" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"budget_usd\": 300.0,
    \"status\": \"active\"
  }")

echo "$UPDATE_RESPONSE" | jq .

if echo "$UPDATE_RESPONSE" | jq -e '.message' > /dev/null; then
  echo "✅ API key updated successfully"
else
  echo "❌ Failed to update API key"
fi
```

### 5. Policy Management Tests

#### Test 5.1: Get Team Policies
```bash
echo "=== Test 5.1: Get Team Policies ==="
POLICIES_RESPONSE=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID/policies" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$POLICIES_RESPONSE" | jq .

if echo "$POLICIES_RESPONSE" | jq -e '.team_id' > /dev/null; then
  echo "✅ Team policies retrieved successfully"
else
  echo "❌ Failed to get team policies or policies not applied"
fi
```

#### Test 5.2: Validate Team Policies
```bash
echo "=== Test 5.2: Validate Team Policies ==="
VALIDATION_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/$TEST_TEAM_ID/policies/validate" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{}')

echo "$VALIDATION_RESPONSE" | jq .

if echo "$VALIDATION_RESPONSE" | jq -e '.overall_status' > /dev/null; then
  OVERALL_STATUS=$(echo "$VALIDATION_RESPONSE" | jq -r '.overall_status')
  if [ "$OVERALL_STATUS" = "true" ]; then
    echo "✅ Team policies validation passed"
  else
    echo "⚠️  Team policies validation failed"
    echo "$VALIDATION_RESPONSE" | jq '.tests[] | select(.status == false)'
  fi
else
  echo "❌ Policy validation endpoint failed"
fi
```

#### Test 5.3: Sync Team Policies
```bash
echo "=== Test 5.3: Sync Team Policies ==="
SYNC_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/$TEST_TEAM_ID/policies/sync" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{}')

echo "$SYNC_RESPONSE" | jq .

if echo "$SYNC_RESPONSE" | jq -e '.message' > /dev/null; then
  echo "✅ Team policies synchronized successfully"
else
  echo "❌ Failed to sync team policies"
fi
```

### 6. Team Activity and Usage Tests

#### Test 6.1: Get Team Activity
```bash
echo "=== Test 6.1: Get Team Activity ==="
ACTIVITY_RESPONSE=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID/activity" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$ACTIVITY_RESPONSE" | jq .

if echo "$ACTIVITY_RESPONSE" | jq -e '.team_id' > /dev/null; then
  echo "✅ Team activity retrieved successfully"
else
  echo "❌ Failed to get team activity"
fi
```

#### Test 6.2: Get Team Usage
```bash
echo "=== Test 6.2: Get Team Usage ==="
USAGE_RESPONSE=$(curl -s -X GET "$BASE_URL/v2/teams/$TEST_TEAM_ID/usage" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$USAGE_RESPONSE" | jq .

if echo "$USAGE_RESPONSE" | jq -e '.team_id' > /dev/null; then
  echo "✅ Team usage retrieved successfully"
else
  echo "❌ Failed to get team usage"
fi
```

### 7. API Key Authentication Tests

#### Test 7.1: Test API Key with Simulator Model
```bash
echo "=== Test 7.1: Test API Key with Simulator Model ==="
MODEL_RESPONSE=$(curl -s -X POST "http://simulator-llm.apps.cluster.com/v1/chat/completions" \
  -H "Authorization: APIKEY $TEST_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "simulator-model",
    "messages": [
      {"role": "user", "content": "Hello from test API key!"}
    ],
    "max_tokens": 20
  }')

echo "$MODEL_RESPONSE" | jq .

if echo "$MODEL_RESPONSE" | jq -e '.choices[0].message.content' > /dev/null; then
  echo "✅ API key authentication with simulator model successful"
else
  echo "❌ API key authentication failed"
  echo "Response: $MODEL_RESPONSE"
fi
```

#### Test 7.2: Test API Key with Invalid Model (Should Fail)
```bash
echo "=== Test 7.2: Test API Key with Invalid Model ==="
INVALID_RESPONSE=$(curl -s -X POST "http://qwen3-llm.apps.cluster.com/v1/chat/completions" \
  -H "Authorization: APIKEY invalid-key-12345" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-0-6b-instruct",
    "messages": [{"role": "user", "content": "test"}],
    "max_tokens": 10
  }')

# Should receive 401 Unauthorized
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://qwen3-llm.apps.cluster.com/v1/chat/completions" \
  -H "Authorization: APIKEY invalid-key-12345" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"test"}],"max_tokens":10}')

if [ "$HTTP_STATUS" = "401" ]; then
  echo "✅ Invalid API key correctly rejected"
else
  echo "❌ Invalid API key was not rejected (HTTP $HTTP_STATUS)"
fi
```

### 8. Admin and Compliance Tests

#### Test 8.1: Get Policy Compliance Report
```bash
echo "=== Test 8.1: Get Policy Compliance Report ==="
COMPLIANCE_RESPONSE=$(curl -s -X GET "$BASE_URL/v2/admin/policies/compliance" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$COMPLIANCE_RESPONSE" | jq .

if echo "$COMPLIANCE_RESPONSE" | jq -e '.total_teams' > /dev/null; then
  TOTAL_TEAMS=$(echo "$COMPLIANCE_RESPONSE" | jq -r '.total_teams')
  COMPLIANT_TEAMS=$(echo "$COMPLIANCE_RESPONSE" | jq -r '.compliant_teams')
  echo "✅ Compliance report retrieved: $COMPLIANT_TEAMS/$TOTAL_TEAMS teams compliant"
else
  echo "❌ Failed to get compliance report"
fi
```

### 9. Error Scenario Tests

#### Test 9.1: Create Team with Invalid Tier
```bash
echo "=== Test 9.1: Create Team with Invalid Tier ==="
ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "invalid-tier-team",
    "team_name": "Invalid Tier Team",
    "default_tier": "nonexistent-tier"
  }')

echo "$ERROR_RESPONSE" | jq .

if echo "$ERROR_RESPONSE" | jq -e '.error' > /dev/null; then
  echo "✅ Invalid tier correctly rejected"
else
  echo "❌ Invalid tier was not rejected"
fi
```

#### Test 9.2: Add User to Non-existent Team
```bash
echo "=== Test 9.2: Add User to Non-existent Team ==="
ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/nonexistent-team/members" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_email": "test@example.com",
    "role": "member"
  }')

echo "$ERROR_RESPONSE" | jq .

if echo "$ERROR_RESPONSE" | jq -e '.error' > /dev/null; then
  echo "✅ Non-existent team correctly rejected"
else
  echo "❌ Non-existent team was not rejected"
fi
```

#### Test 9.3: Create API Key for Non-member User
```bash
echo "=== Test 9.3: Create API Key for Non-member User ==="
ERROR_RESPONSE=$(curl -s -X POST "$BASE_URL/v2/teams/$TEST_TEAM_ID/keys" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "nonexistent-user",
    "alias": "invalid-key"
  }')

echo "$ERROR_RESPONSE" | jq .

if echo "$ERROR_RESPONSE" | jq -e '.error' > /dev/null; then
  echo "✅ Non-member user correctly rejected"
else
  echo "❌ Non-member user was not rejected"
fi
```

### 10. Cleanup Tests

#### Test 10.1: Delete API Key
```bash
echo "=== Test 10.1: Delete API Key ==="
DELETE_KEY_RESPONSE=$(curl -s -X DELETE "$BASE_URL/v2/keys/$TEST_SECRET_NAME" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$DELETE_KEY_RESPONSE" | jq .

if echo "$DELETE_KEY_RESPONSE" | jq -e '.message' > /dev/null; then
  echo "✅ API key deleted successfully"
else
  echo "❌ Failed to delete API key"
fi
```

#### Test 10.2: Remove User from Team
```bash
echo "=== Test 10.2: Remove User from Team ==="
REMOVE_USER_RESPONSE=$(curl -s -X DELETE "$BASE_URL/v2/teams/$TEST_TEAM_ID/members/$TEST_USER_ID" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$REMOVE_USER_RESPONSE" | jq .

if echo "$REMOVE_USER_RESPONSE" | jq -e '.message' > /dev/null; then
  echo "✅ User removed from team successfully"
else
  echo "❌ Failed to remove user from team"
fi
```

#### Test 10.3: Delete Team
```bash
echo "=== Test 10.3: Delete Team ==="
DELETE_TEAM_RESPONSE=$(curl -s -X DELETE "$BASE_URL/v2/teams/$TEST_TEAM_ID" \
  -H "Authorization: ADMIN $ADMIN_KEY")

echo "$DELETE_TEAM_RESPONSE" | jq .

if echo "$DELETE_TEAM_RESPONSE" | jq -e '.message' > /dev/null; then
  echo "✅ Team deleted successfully"
else
  echo "❌ Failed to delete team"
fi
```

## Performance Tests

### Load Testing Team Operations
```bash
echo "=== Performance Test: Multiple Team Operations ==="

# Create multiple teams concurrently
for i in {1..5}; do
  (
    TEAM_ID="perf-test-$i"
    curl -s -X POST "$BASE_URL/v2/teams" \
      -H "Authorization: ADMIN $ADMIN_KEY" \
      -H "Content-Type: application/json" \
      -d "{
        \"team_id\": \"$TEAM_ID\",
        \"team_name\": \"Performance Test Team $i\",
        \"default_tier\": \"standard\"
      }" > /dev/null
    echo "Team $i created"
  ) &
done

wait
echo "✅ Concurrent team creation completed"

# Cleanup performance test teams
for i in {1..5}; do
  curl -s -X DELETE "$BASE_URL/v2/teams/perf-test-$i" \
    -H "Authorization: ADMIN $ADMIN_KEY" > /dev/null
done
echo "✅ Performance test cleanup completed"
```

## Kubernetes Resource Validation

### Validate Secrets Created
```bash
echo "=== Validate Kubernetes Resources ==="

# Check team configuration secrets
kubectl get secrets -n llm -l maas/resource-type=team-config

# Check team member secrets
kubectl get secrets -n llm -l maas/resource-type=team-member

# Check team API key secrets
kubectl get secrets -n llm -l maas/resource-type=team-key

# Check policy templates ConfigMap
kubectl get configmap platform-default-policies -n llm

echo "✅ Kubernetes resource validation completed"
```

## Test Summary and Reporting

```bash
echo "=== Test Summary ==="
echo "All tests completed. Check individual test results above."
echo "Expected results:"
echo "  ✅ - Test passed"
echo "  ❌ - Test failed"
echo "  ⚠️  - Test passed with warnings"
echo ""
echo "If any tests failed, check:"
echo "1. Service deployment and health"
echo "2. Admin API key configuration"  
echo "3. Policy management enabled/disabled state"
echo "4. RBAC permissions"
echo "5. Default policy templates loaded"
```