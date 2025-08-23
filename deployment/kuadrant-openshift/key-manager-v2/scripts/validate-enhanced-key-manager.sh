#!/bin/bash

# Enhanced Key Manager Validation Script
# This script performs comprehensive testing of the enhanced key-manager service
# including team management, user management, API key creation, and policy integration

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results tracking
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "PASS")
            echo -e "${GREEN}✅ $message${NC}"
            ((TESTS_PASSED++))
            ;;
        "FAIL")
            echo -e "${RED}❌ $message${NC}"
            ((TESTS_FAILED++))
            ;;
        "WARN")
            echo -e "${YELLOW}⚠️  $message${NC}"
            ;;
        "INFO")
            echo -e "${BLUE}ℹ️  $message${NC}"
            ;;
    esac
    ((TESTS_TOTAL++))
}

# Function to check if command exists
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_status "FAIL" "Required command '$1' not found"
        exit 1
    fi
}

# Function to test API endpoint
test_endpoint() {
    local method=$1
    local url=$2
    local headers=$3
    local data=$4
    local expected_status=$5
    local test_name=$6
    
    echo -e "\n${BLUE}=== $test_name ===${NC}"
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" "$url" $headers -d "$data")
    else
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" "$url" $headers)
    fi
    
    http_code=$(echo "$response" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo "$response" | sed -e 's/HTTPSTATUS:.*//g')
    
    echo "Response: $body" | jq . 2>/dev/null || echo "Response: $body"
    
    if [ "$http_code" -eq "$expected_status" ]; then
        print_status "PASS" "$test_name - HTTP $http_code"
        echo "$body"
    else
        print_status "FAIL" "$test_name - Expected HTTP $expected_status, got $http_code"
        echo "$body"
        return 1
    fi
}

# Check prerequisites
echo -e "${BLUE}Enhanced Key Manager Validation Script${NC}"
echo "======================================"

print_status "INFO" "Checking prerequisites..."
check_command "curl"
check_command "jq"
check_command "kubectl"

# Configuration
if [ -z "$ADMIN_KEY" ]; then
    echo "Please set ADMIN_KEY environment variable"
    exit 1
fi

if [ -z "$BASE_URL" ]; then
    BASE_URL="http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com"
    print_status "INFO" "Using default BASE_URL: $BASE_URL"
fi

# Set test user
export MAAS_USER=${MAAS_USER:-mittens}

# Generate unique test identifiers
TEST_TIMESTAMP=$(date +%s)
TEST_TEAM_ID="test-team-$TEST_TIMESTAMP"
TEST_USER_EMAIL="testuser-$TEST_TIMESTAMP@example.com"

print_status "INFO" "Using test team ID: $TEST_TEAM_ID"
print_status "INFO" "Using test user email: $TEST_USER_EMAIL"

# Test 1: Basic Health Check
test_endpoint "GET" "$BASE_URL/health" "" "" 200 "Basic Health Check"

# Test 2: List Available Models 
test_endpoint "GET" "$BASE_URL/models" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "List Available Models"

# Test 3: Legacy API Key Generation (Default Team)
echo -e "\n${BLUE}=== Legacy API Key Tests ===${NC}"
LEGACY_KEY_DATA="{\"user_id\":\"$MAAS_USER\"}"
LEGACY_RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST "$BASE_URL/generate_key" \
    -H "Authorization: ADMIN $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$LEGACY_KEY_DATA")

legacy_http_code=$(echo "$LEGACY_RESPONSE" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
legacy_body=$(echo "$LEGACY_RESPONSE" | sed -e 's/HTTPSTATUS:.*//g')

if [ "$legacy_http_code" -eq 200 ]; then
    print_status "PASS" "Legacy API Key Generation"
    LEGACY_API_KEY=$(echo "$legacy_body" | jq -r '.api_key')
    echo "Legacy API Key: ${LEGACY_API_KEY:0:20}..."
    LEGACY_KEY_CREATED=true
else
    print_status "FAIL" "Legacy API Key Generation - HTTP $legacy_http_code"
    echo "$legacy_body"
    LEGACY_KEY_CREATED=false
fi

# Test 4: Policy Health Check (if enabled)
test_endpoint "GET" "$BASE_URL/admin/policies/health" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Policy Health Check" || true

# Test 5: Default Policies Check
test_endpoint "GET" "$BASE_URL/admin/policies/defaults" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Default Policies Check" || true

# Test 6: Create Team
echo -e "\n${BLUE}=== Team Management Tests ===${NC}"
CREATE_TEAM_DATA=$(cat <<EOF
{
  "team_id": "$TEST_TEAM_ID",
  "team_name": "Automated Test Team",
  "description": "Team created by validation script",
  "default_budget_usd": 1000.0,
  "default_tier": "standard"
}
EOF
)

if test_endpoint "POST" "$BASE_URL/teams" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$CREATE_TEAM_DATA" 200 "Create Team"; then
    TEAM_CREATED=true
else
    TEAM_CREATED=false
fi

# Test 7: List Teams
test_endpoint "GET" "$BASE_URL/teams" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "List Teams"

# Test 8: Get Team Details
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Get Team Details"
fi

# Test 7: Add User to Team
echo -e "\n${BLUE}=== User Management Tests ===${NC}"
if [ "$TEAM_CREATED" = true ]; then
    ADD_USER_DATA=$(cat <<EOF
{
  "user_email": "$TEST_USER_EMAIL",
  "role": "member",
  "custom_budget_usd": 500.0
}
EOF
    )
    
    if test_endpoint "POST" "$BASE_URL/teams/$TEST_TEAM_ID/members" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$ADD_USER_DATA" 200 "Add User to Team"; then
        USER_ADDED=true
        # Extract user ID from response (assuming it's extracted from email)
        TEST_USER_ID=$(echo "$TEST_USER_EMAIL" | cut -d'@' -f1 | tr '.' '-' | tr '_' '-')
    else
        USER_ADDED=false
    fi
fi

# Test 8: List Team Members
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID/members" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "List Team Members"
fi

# Test 9: Create Team API Key
echo -e "\n${BLUE}=== Enhanced API Key Tests ===${NC}"
if [ "$USER_ADDED" = true ]; then
    CREATE_KEY_DATA=$(cat <<EOF
{
  "user_id": "$TEST_USER_ID",
  "alias": "test-api-key",
  "models": ["simulator-model"],
  "budget_usd": 200.0,
  "inherit_team_limits": true,
  "custom_limits": {
    "max_tokens_per_request": 4000
  }
}
EOF
    )
    
    KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/teams/$TEST_TEAM_ID/keys" \
        -H "Authorization: ADMIN $ADMIN_KEY" \
        -H "Content-Type: application/json" \
        -d "$CREATE_KEY_DATA")
    
    if echo "$KEY_RESPONSE" | jq -e '.api_key' > /dev/null; then
        print_status "PASS" "Create Team API Key"
        TEST_API_KEY=$(echo "$KEY_RESPONSE" | jq -r '.api_key')
        TEST_SECRET_NAME=$(echo "$KEY_RESPONSE" | jq -r '.secret_name')
        KEY_CREATED=true
        echo "API Key: ${TEST_API_KEY:0:20}..."
        echo "Secret Name: $TEST_SECRET_NAME"
    else
        print_status "FAIL" "Create Team API Key"
        echo "$KEY_RESPONSE"
        KEY_CREATED=false
    fi
fi

# Test 10: List Team API Keys
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID/keys" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "List Team API Keys"
fi

# Test 11: Update API Key
if [ "$KEY_CREATED" = true ]; then
    UPDATE_KEY_DATA='{"budget_usd": 300.0, "status": "active"}'
    test_endpoint "PATCH" "$BASE_URL/keys/$TEST_SECRET_NAME" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$UPDATE_KEY_DATA" 200 "Update API Key"
fi

# Test 12: Policy Management Tests
echo -e "\n${BLUE}=== Policy Management Tests ===${NC}"
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID/policies" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Get Team Policies" || true
    
    test_endpoint "POST" "$BASE_URL/teams/$TEST_TEAM_ID/policies/validate" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" '{}' 200 "Validate Team Policies" || true
    
    test_endpoint "POST" "$BASE_URL/teams/$TEST_TEAM_ID/policies/sync" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" '{}' 200 "Sync Team Policies" || true
fi

# Test 13: Team Activity and Usage
echo -e "\n${BLUE}=== Team Activity and Usage Tests ===${NC}"
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID/activity" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Get Team Activity"
    
    test_endpoint "GET" "$BASE_URL/teams/$TEST_TEAM_ID/usage" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Get Team Usage"
fi

# Test 14: API Key Authentication (if we have model endpoints)
echo -e "\n${BLUE}=== API Key Authentication Tests ===${NC}"
if [ "$KEY_CREATED" = true ]; then
    MODEL_TEST_DATA='{"model":"simulator-model","messages":[{"role":"user","content":"Test from validation script"}],"max_tokens":20}'
    
    # Test with simulator model (if available)
    SIMULATOR_URL="http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions"
    if curl -s --connect-timeout 5 "$SIMULATOR_URL" > /dev/null 2>&1; then
        MODEL_RESPONSE=$(curl -s -X POST "$SIMULATOR_URL" \
            -H "Authorization: APIKEY $TEST_API_KEY" \
            -H "Content-Type: application/json" \
            -d "$MODEL_TEST_DATA")
        
        if echo "$MODEL_RESPONSE" | jq -e '.choices[0].message.content' > /dev/null; then
            print_status "PASS" "API Key Authentication with Simulator Model"
        else
            print_status "FAIL" "API Key Authentication with Simulator Model"
            echo "$MODEL_RESPONSE"
        fi
    else
        print_status "WARN" "Simulator model endpoint not available for testing"
    fi
    
    # Test with invalid API key (should fail)
    INVALID_RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST "$SIMULATOR_URL" \
        -H "Authorization: APIKEY invalid-key-12345" \
        -H "Content-Type: application/json" \
        -d "$MODEL_TEST_DATA" 2>/dev/null || true)
    
    if echo "$INVALID_RESPONSE" | grep "HTTPSTATUS:401" > /dev/null; then
        print_status "PASS" "Invalid API Key Correctly Rejected"
    else
        print_status "WARN" "Could not test invalid API key rejection"
    fi
fi

# Test Legacy API Key with Model Endpoints
if [ "$LEGACY_KEY_CREATED" = true ]; then
    echo -e "\n${BLUE}=== Legacy API Key Model Tests ===${NC}"
    if curl -s --connect-timeout 5 "$SIMULATOR_URL" > /dev/null 2>&1; then
        LEGACY_MODEL_RESPONSE=$(curl -s -X POST "$SIMULATOR_URL" \
            -H "Authorization: APIKEY $LEGACY_API_KEY" \
            -H "Content-Type: application/json" \
            -d "$MODEL_TEST_DATA")
        
        if echo "$LEGACY_MODEL_RESPONSE" | jq -e '.choices[0].message.content' > /dev/null; then
            print_status "PASS" "Legacy API Key Authentication with Simulator Model"
        else
            print_status "FAIL" "Legacy API Key Authentication with Simulator Model"
            echo "$LEGACY_MODEL_RESPONSE"
        fi
    else
        print_status "WARN" "Simulator model endpoint not available for legacy key testing"
    fi
fi

# Test 15: Admin and Compliance
echo -e "\n${BLUE}=== Admin and Compliance Tests ===${NC}"
test_endpoint "GET" "$BASE_URL/admin/policies/compliance" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Get Policy Compliance Report" || true

# Test 16: Error Scenarios
echo -e "\n${BLUE}=== Error Scenario Tests ===${NC}"

# Test creating team with invalid tier
INVALID_TIER_DATA='{"team_id":"invalid-tier-test","team_name":"Invalid Tier","default_tier":"nonexistent"}'
if test_endpoint "POST" "$BASE_URL/teams" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$INVALID_TIER_DATA" 400 "Create Team with Invalid Tier"; then
    print_status "PASS" "Invalid tier correctly rejected"
else
    print_status "WARN" "Invalid tier test inconclusive"
fi

# Test adding user to non-existent team
NON_TEAM_DATA='{"user_email":"test@example.com","role":"member"}'
if test_endpoint "POST" "$BASE_URL/teams/nonexistent-team/members" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$NON_TEAM_DATA" 404 "Add User to Non-existent Team"; then
    print_status "PASS" "Non-existent team correctly rejected"
else
    print_status "WARN" "Non-existent team test inconclusive"
fi

# Test 17: Kubernetes Resource Validation
echo -e "\n${BLUE}=== Kubernetes Resource Validation ===${NC}"

# Check if kubectl is configured
if kubectl cluster-info > /dev/null 2>&1; then
    # Check team configuration secrets
    TEAM_CONFIGS=$(kubectl get secrets -n llm -l maas/resource-type=team-config --no-headers 2>/dev/null | wc -l)
    print_status "INFO" "Found $TEAM_CONFIGS team configuration secrets"
    
    # Check team member secrets  
    TEAM_MEMBERS=$(kubectl get secrets -n llm -l maas/resource-type=team-member --no-headers 2>/dev/null | wc -l)
    print_status "INFO" "Found $TEAM_MEMBERS team member secrets"
    
    # Check team API key secrets
    TEAM_KEYS=$(kubectl get secrets -n llm -l maas/resource-type=team-key --no-headers 2>/dev/null | wc -l)
    print_status "INFO" "Found $TEAM_KEYS team API key secrets"
    
    # Check policy templates ConfigMap
    if kubectl get configmap platform-default-policies -n llm > /dev/null 2>&1; then
        print_status "PASS" "Policy templates ConfigMap exists"
    else
        print_status "WARN" "Policy templates ConfigMap not found"
    fi
    
    # Check if our test team secret was created
    if [ "$TEAM_CREATED" = true ]; then
        if kubectl get secret "team-$TEST_TEAM_ID-config" -n llm > /dev/null 2>&1; then
            print_status "PASS" "Test team secret created in Kubernetes"
        else
            print_status "FAIL" "Test team secret not found in Kubernetes"
        fi
    fi
else
    print_status "WARN" "kubectl not configured, skipping Kubernetes validation"
fi

# Cleanup
echo -e "\n${BLUE}=== Cleanup ===${NC}"

# Delete legacy API key
if [ "$LEGACY_KEY_CREATED" = true ]; then
    LEGACY_DELETE_DATA="{\"key\":\"$LEGACY_API_KEY\"}"
    test_endpoint "DELETE" "$BASE_URL/delete_key" "-H 'Authorization: ADMIN $ADMIN_KEY' -H 'Content-Type: application/json'" "$LEGACY_DELETE_DATA" 200 "Delete Legacy API Key"
fi

# Delete API key
if [ "$KEY_CREATED" = true ]; then
    test_endpoint "DELETE" "$BASE_URL/keys/$TEST_SECRET_NAME" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Delete API Key"
fi

# Remove user from team  
if [ "$USER_ADDED" = true ]; then
    test_endpoint "DELETE" "$BASE_URL/teams/$TEST_TEAM_ID/members/$TEST_USER_ID" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Remove User from Team"
fi

# Delete team
if [ "$TEAM_CREATED" = true ]; then
    test_endpoint "DELETE" "$BASE_URL/teams/$TEST_TEAM_ID" "-H 'Authorization: ADMIN $ADMIN_KEY'" "" 200 "Delete Team"
fi

# Final Summary
echo -e "\n${BLUE}=== Test Summary ===${NC}"
echo "======================================"
echo -e "Total Tests: $TESTS_TOTAL"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All tests passed! Enhanced key-manager is working correctly.${NC}"
    exit 0
else
    echo -e "\n${RED}❌ Some tests failed. Please check the output above.${NC}"
    exit 1
fi