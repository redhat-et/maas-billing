#!/bin/bash

# Test script for key-manager basic functionality
# Fails if responses are empty or contain errors
#
# Usage Examples:
#   # Using environment variables
#   export ADMIN_KEY="your-admin-key"
#   export MAAS_USER="alice"
#   ./test-request.sh
#
#   # Using command line flags
#   ./test-request.sh --admin-key "your-admin-key" --user "alice"
#
#   # Using mixed approach
#   export ADMIN_KEY="your-admin-key"
#   ./test-request.sh --user "bob"
#
#   # Testing with custom base URL
#   export ADMIN_KEY="your-admin-key"
#   export BASE_URL="http://key-manager.example.com"
#   ./test-request.sh --user "charlie"

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default configuration from environment variables
ADMIN_KEY="${ADMIN_KEY:-}"
BASE_URL="${BASE_URL:-http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com}"
MAAS_USER="${MAAS_USER:-testuser}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -u|--user)
      MAAS_USER="$2"
      shift 2
      ;;
    -k|--admin-key)
      ADMIN_KEY="$2"
      shift 2
      ;;
    --base-url)
      BASE_URL="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Test script for MaaS key-manager functionality validation"
      echo ""
      echo "Options:"
      echo "  -u, --user USERNAME       Test with specific username (default: testuser)"
      echo "  -k, --admin-key KEY       Admin API key for key creation"
      echo "      --base-url URL        Key manager base URL"
      echo "  -h, --help                Show this help message"
      echo ""
      echo "Environment Variables:"
      echo "  ADMIN_KEY                 Admin API key (required if not provided via flag)"
      echo "  MAAS_USER                 Username for testing (default: testuser)"
      echo "  BASE_URL                  Key manager base URL"
      echo ""
      echo "Examples:"
      echo "  # Using environment variables:"
      echo "  export ADMIN_KEY=\"your-admin-key\""
      echo "  export MAAS_USER=\"alice\""
      echo "  ./test-request.sh"
      echo ""
      echo "  # Using command line flags:"
      echo "  ./test-request.sh --admin-key \"your-admin-key\" --user \"alice\""
      echo ""
      echo "  # Mixed approach:"
      echo "  export ADMIN_KEY=\"your-admin-key\""
      echo "  ./test-request.sh --user \"bob\""
      exit 0
      ;;
    *)
      echo "Unknown option $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Validate required parameters
if [[ -z "$ADMIN_KEY" ]]; then
    echo -e "${RED}ERROR: Admin key is required${NC}"
    echo "Please set ADMIN_KEY environment variable or use --admin-key flag"
    echo "Example: export ADMIN_KEY=\"your-admin-key\""
    echo "Or: $0 --admin-key \"your-admin-key\""
    exit 1
fi

echo -e "${YELLOW}Testing Key Manager Basic Functionality${NC}"
echo "Base URL: $BASE_URL"
echo "Test User: $MAAS_USER"
echo "================================"

# Test 1: Health Check
echo -e "${YELLOW}1. Testing health endpoint...${NC}"
HEALTH_RESPONSE=$(curl -s "$BASE_URL/health")
if [[ -z "$HEALTH_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Health endpoint returned empty response${NC}"
    exit 1
fi

if echo "$HEALTH_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: Health endpoint returned error: $HEALTH_RESPONSE${NC}"
    exit 1
fi

if ! echo "$HEALTH_RESPONSE" | grep -q "healthy"; then
    echo -e "${RED}FAIL: Health endpoint did not return healthy status: $HEALTH_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: Health check successful${NC}"

# Test 2: API Key Generation
echo -e "${YELLOW}2. Testing API key generation...${NC}"
API_RESPONSE=$(curl -s -X POST "$BASE_URL/generate_key" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$MAAS_USER\"}")

if [[ -z "$API_RESPONSE" ]]; then
    echo -e "${RED}FAIL: API key generation returned empty response${NC}"
    exit 1
fi

if echo "$API_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: API key generation returned error: $API_RESPONSE${NC}"
    exit 1
fi

# Extract API key
API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
if [[ -z "$API_KEY" ]]; then
    echo -e "${RED}FAIL: Could not extract API key from response: $API_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: API key generated successfully${NC}"
echo "API Key: ${API_KEY:0:20}..."

# Wait for API key to propagate (not nessecary but in case a smoll HW cluster)
sleep 0.5

# Test 3: List Teams
echo -e "${YELLOW}3. Testing teams endpoint...${NC}"
TEAMS_RESPONSE=$(curl -s -X GET "$BASE_URL/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY")

if [[ -z "$TEAMS_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Teams endpoint returned empty response${NC}"
    exit 1
fi

if echo "$TEAMS_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: Teams endpoint returned error: $TEAMS_RESPONSE${NC}"
    exit 1
fi

if ! echo "$TEAMS_RESPONSE" | grep -q "default"; then
    echo -e "${RED}FAIL: Default team not found in teams response: $TEAMS_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: Teams endpoint working${NC}"

# Test 4: Test Model Access
echo -e "${YELLOW}4. Testing model access...${NC}"
MODEL_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello!"}],"max_tokens":20}' \
     "http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions" || echo "Model endpoint not available")

if [[ "$MODEL_RESPONSE" == "Model endpoint not available" ]]; then
    echo -e "${RED}FAIL: Model endpoint not available${NC}"
    exit 1
elif [[ -z "$MODEL_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Model endpoint returned empty response${NC}"
    exit 1
elif echo "$MODEL_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: Model endpoint returned error: ${MODEL_RESPONSE:0:200}...${NC}"
    exit 1
elif echo "$MODEL_RESPONSE" | grep -q "choices"; then
    echo -e "${GREEN}PASS: Model access working${NC}"
    echo -e "${YELLOW}Model Response:${NC}"
    echo "$MODEL_RESPONSE" | jq . 2>/dev/null || echo "$MODEL_RESPONSE"
else
    echo -e "${RED}FAIL: Model response doesn't contain expected 'choices' field: ${MODEL_RESPONSE:0:200}...${NC}"
    exit 1
fi

# Test 5: Negative Test - User API Key Cannot Create Keys
echo -e "${YELLOW}5. Testing negative case - user API key cannot create keys...${NC}"
NEGATIVE_RESPONSE=$(curl -s -X POST "$BASE_URL/generate_key" \
     -H "Authorization: ADMIN $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"user_id":"should-fail"}')

if [[ -z "$NEGATIVE_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Negative test returned empty response${NC}"
    exit 1
elif echo "$NEGATIVE_RESPONSE" | grep -q "error"; then
    echo -e "${GREEN}PASS: User API key correctly rejected (cannot create keys)${NC}"
    echo "Expected error response: $(echo "$NEGATIVE_RESPONSE" | head -c 100)..."
elif echo "$NEGATIVE_RESPONSE" | grep -q "api_key"; then
    echo -e "${RED}FAIL: User API key was able to create keys (security issue!)${NC}"
    echo "Unexpected success: $NEGATIVE_RESPONSE"
    exit 1
else
    echo -e "${RED}FAIL: Unexpected response from negative test: ${NEGATIVE_RESPONSE:0:100}...${NC}"
    exit 1
fi

# Test 6: User Model Access - Verify User API Key Works for Model Queries
echo -e "${YELLOW}6. Testing user model access with the new API key...${NC}"
USER_MODEL_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Say hello as the user '$MAAS_USER'"}],"max_tokens":50}' \
     "http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions" || echo "Model endpoint not available")

if [[ "$USER_MODEL_RESPONSE" == "Model endpoint not available" ]]; then
    echo -e "${RED}FAIL: Model endpoint not available for user access${NC}"
    exit 1
elif [[ -z "$USER_MODEL_RESPONSE" ]]; then
    echo -e "${RED}FAIL: User model access returned empty response${NC}"
    exit 1
elif echo "$USER_MODEL_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: User model access returned error: ${USER_MODEL_RESPONSE:0:200}...${NC}"
    exit 1
elif echo "$USER_MODEL_RESPONSE" | grep -q "choices"; then
    echo -e "${GREEN}PASS: User API key successfully accessed model${NC}"
    echo -e "${YELLOW}User Model Response:${NC}"
    echo "$USER_MODEL_RESPONSE" | jq . 2>/dev/null || echo "$USER_MODEL_RESPONSE"
else
    echo -e "${RED}FAIL: User model response doesn't contain expected 'choices' field: ${USER_MODEL_RESPONSE:0:200}...${NC}"
    exit 1
fi

# Test 7: Multiple Key Creation - Verify Users Can Create Multiple Keys
echo -e "${YELLOW}7. Testing multiple key creation (users can have multiple keys)...${NC}"
SECOND_KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/generate_key" \
     -H "Authorization: ADMIN $ADMIN_KEY" \
     -H 'Content-Type: application/json' \
     -d "{\"user_id\":\"$MAAS_USER\"}")

if [[ -z "$SECOND_KEY_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Second key creation returned empty response${NC}"
    exit 1
elif echo "$SECOND_KEY_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: Second key creation returned error: ${SECOND_KEY_RESPONSE:0:100}...${NC}"
    exit 1
elif echo "$SECOND_KEY_RESPONSE" | grep -q "api_key"; then
    echo -e "${GREEN}PASS: Multiple key creation works (users can have multiple keys)${NC}"
    # Extract second API key for cleanup
    SECOND_API_KEY=$(echo "$SECOND_KEY_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
    echo "Second API Key: ${SECOND_API_KEY:0:20}..."
else
    echo -e "${RED}FAIL: Unexpected response from second key test: ${SECOND_KEY_RESPONSE:0:100}...${NC}"
    exit 1
fi

# Test 8: Key Deletion - Clean up both created API keys
echo -e "${YELLOW}8. Testing API key deletion...${NC}"

# Delete first API key
DELETE_RESPONSE_1=$(curl -s -X DELETE "$BASE_URL/delete_key" \
     -H "Authorization: ADMIN $ADMIN_KEY" \
     -H 'Content-Type: application/json' \
     -d "{\"key\":\"$API_KEY\"}")

# Delete second API key
DELETE_RESPONSE_2=$(curl -s -X DELETE "$BASE_URL/delete_key" \
     -H "Authorization: ADMIN $ADMIN_KEY" \
     -H 'Content-Type: application/json' \
     -d "{\"key\":\"$SECOND_API_KEY\"}")

# Check both deletion responses
if [[ -z "$DELETE_RESPONSE_1" ]] || [[ -z "$DELETE_RESPONSE_2" ]]; then
    echo -e "${RED}FAIL: Key deletion returned empty response${NC}"
    exit 1
elif echo "$DELETE_RESPONSE_1" | grep -q "error" || echo "$DELETE_RESPONSE_2" | grep -q "error"; then
    echo -e "${RED}FAIL: Key deletion returned error${NC}"
    echo "First key: $DELETE_RESPONSE_1"
    echo "Second key: $DELETE_RESPONSE_2"
    exit 1
elif echo "$DELETE_RESPONSE_1" | grep -q "deleted successfully" && echo "$DELETE_RESPONSE_2" | grep -q "deleted successfully"; then
    echo -e "${GREEN}PASS: Both API keys deleted successfully${NC}"
    echo "Key cleanup completed"
else
    echo -e "${RED}FAIL: Unexpected response from key deletion${NC}"
    echo "First key: ${DELETE_RESPONSE_1:0:100}..."
    echo "Second key: ${DELETE_RESPONSE_2:0:100}..."
    exit 1
fi

echo "================================"
echo -e "${GREEN}✅ All key-manager tests passed!${NC}"
echo -e "${GREEN}✅ Security: User API keys cannot create new keys${NC}"
echo -e "${GREEN}✅ Functionality: User API keys can access models${NC}"
echo -e "${GREEN}✅ Key Management: Multiple keys per user supported${NC}"
echo -e "${GREEN}✅ Cleanup: API key deletion works${NC}"
echo "API Keys created and deleted: ${API_KEY:0:20}... and ${SECOND_API_KEY:0:20}..."
echo "User: $MAAS_USER"
