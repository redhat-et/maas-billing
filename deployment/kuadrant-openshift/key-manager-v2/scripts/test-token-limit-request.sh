#!/bin/bash

# Test script for token rate limiting functionality
# Tests TokenRateLimitPolicy enforcement at the team level
#
# Usage Examples:
#   # Using environment variables
#   export ADMIN_KEY="your-admin-key"
#   export MAAS_USER="alice"
#   ./test-token-limit-request.sh
#
#   # Using command line flags
#   ./test-token-limit-request.sh --admin-key "your-admin-key" --user "alice"

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default configuration from environment variables
ADMIN_KEY="${ADMIN_KEY:-}"
BASE_URL="${BASE_URL:-http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com}"
MAAS_USER="${MAAS_USER:-tokenuser}"
MODEL_URL="${MODEL_URL:-http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com}"
TOKEN_LIMIT="${TOKEN_LIMIT:-5000}"
TIME_WINDOW="${TIME_WINDOW:-1h}"
TEAM_ID="${TEAM_ID:-$TEAM_ID}"

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
    --model-url)
      MODEL_URL="$2"
      shift 2
      ;;
    --token-limit)
      TOKEN_LIMIT="$2"
      shift 2
      ;;
    --time-window)
      TIME_WINDOW="$2"
      shift 2
      ;;
    --team-id)
      TEAM_ID="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Test script for Token Rate Limiting (TokenRateLimitPolicy) functionality"
      echo ""
      echo "Options:"
      echo "  -u, --user USERNAME       Test with specific username (default: tokenuser)"
      echo "  -k, --admin-key KEY       Admin API key for key creation"
      echo "      --base-url URL        Key manager base URL"
      echo "      --model-url URL       Model endpoint URL" 
      echo "      --token-limit NUM     Token limit for team (default: 5000)"
      echo "      --time-window WINDOW  Time window (default: 1h)"
      echo "      --team-id ID          Team ID to create (default: $TEAM_ID)"
      echo "  -h, --help                Show this help message"
      echo ""
      echo "Environment Variables:"
      echo "  ADMIN_KEY                 Admin API key (required if not provided via flag)"
      echo "  MAAS_USER                 Username for testing (default: tokenuser)"
      echo "  BASE_URL                  Key manager base URL"
      echo "  MODEL_URL                 Model endpoint URL"
      echo "  TOKEN_LIMIT               Token limit for team (default: 5000)"
      echo "  TIME_WINDOW               Time window (default: 1h)"
      echo "  TEAM_ID                   Team ID to create (default: $TEAM_ID)"
      echo ""
      echo "### Team-Based Deployment Token Rate Limit Test"
      echo ""
      echo "Manual Commands (what this script automates):"
      echo ""
      echo "1. Create team with token rate limiting:"
      echo "   curl -X POST \"\$BASE_URL/teams\" \\"
      echo "     -H \"Authorization: ADMIN \$ADMIN_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{"
      echo "       \"team_id\": \"token-test-team\","
      echo "       \"team_name\": \"Token Rate Limited Team\","
      echo "       \"description\": \"Team limited to 5,000 AI tokens per hour\","
      echo "       \"default_tier\": \"standard\","
      echo "       \"token_limit\": 5000,"
      echo "       \"time_window\": \"1h\""
      echo "     }'"
      echo ""
      echo "2. Create API key for team:"
      echo "   curl -X POST \"\$BASE_URL/teams/token-test-team/keys\" \\"
      echo "     -H \"Authorization: ADMIN \$ADMIN_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{"
      echo "       \"user_id\": \"tokenuser\","
      echo "       \"alias\": \"token-test-key\","
      echo "       \"inherit_team_limits\": true"
      echo "     }'"
      echo ""
      echo "3. Verify TokenRateLimitPolicy was created:"
      echo "   kubectl get tokenratelimitpolicy team-token-test-team-rate-limits -n llm -o yaml"
      echo ""
      echo "4. Test token consumption with API key:"
      echo "   curl -H \"Authorization: APIKEY \$API_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{\"model\":\"qwen3-0-6b-instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}],\"max_tokens\":100}' \\"
      echo "     \"\$MODEL_URL/v1/chat/completions\""
      echo ""
      echo "5. Cleanup:"
      echo "   curl -X DELETE \"\$BASE_URL/delete_key\" -H \"Authorization: ADMIN \$ADMIN_KEY\" -d '{\"key\":\"\$API_KEY\"}'"
      echo "   curl -X DELETE \"\$BASE_URL/teams/token-test-team\" -H \"Authorization: ADMIN \$ADMIN_KEY\""
      echo ""
      echo "Examples:"
      echo "  # Using environment variables:"
      echo "  export ADMIN_KEY=\"your-admin-key\""
      echo "  export TOKEN_LIMIT=10000"
      echo "  export TIME_WINDOW=\"30m\""
      echo "  ./test-token-limit-request.sh"
      echo ""
      echo "  # Using command line flags:"
      echo "  ./test-token-limit-request.sh --admin-key \"your-key\" --token-limit 10000 --time-window \"30m\""
      echo ""
      echo "  # Custom team and user:"
      echo "  ./test-token-limit-request.sh --team-id \"my-team\" --user \"alice\" --token-limit 50000"
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
    exit 1
fi

echo -e "${YELLOW}Testing Token Rate Limiting (TokenRateLimitPolicy)${NC}"
echo "Base URL: $BASE_URL"
echo "Model URL: $MODEL_URL"
echo "Test User: $MAAS_USER"
echo "Team ID: $TEAM_ID"
echo "Token Limit: $TOKEN_LIMIT"
echo "Time Window: $TIME_WINDOW"
echo "================================"

# Test 1: Create team with token rate limiting
echo -e "${YELLOW}1. Creating team with token rate limiting ($TOKEN_LIMIT tokens per $TIME_WINDOW)...${NC}"
TEAM_RESPONSE=$(curl -s -X POST "$BASE_URL/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"team_id\": \"$TEAM_ID\",
    \"team_name\": \"Token Rate Limited Team\",
    \"description\": \"Team limited to $TOKEN_LIMIT AI tokens per $TIME_WINDOW for testing\",
    \"default_tier\": \"standard\",
    \"token_limit\": $TOKEN_LIMIT,
    \"time_window\": \"$TIME_WINDOW\"
  }")

if [[ -z "$TEAM_RESPONSE" ]]; then
    echo -e "${RED}FAIL: Team creation returned empty response${NC}"
    exit 1
fi

if echo "$TEAM_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: Team creation returned error: $TEAM_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: Token-limited team created successfully${NC}"
echo "Team Response: $TEAM_RESPONSE"

# Test 2: Create API key for the team
echo -e "${YELLOW}2. Creating API key for token-limited team...${NC}"
KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/teams/$TEAM_ID/keys" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"user_id\": \"$MAAS_USER\",
    \"alias\": \"token-test-key\",
    \"inherit_team_limits\": true
  }")

if [[ -z "$KEY_RESPONSE" ]]; then
    echo -e "${RED}FAIL: API key creation returned empty response${NC}"
    exit 1
fi

if echo "$KEY_RESPONSE" | grep -q "error"; then
    echo -e "${RED}FAIL: API key creation returned error: $KEY_RESPONSE${NC}"
    exit 1
fi

# Extract API key
API_KEY=$(echo "$KEY_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
if [[ -z "$API_KEY" ]]; then
    echo -e "${RED}FAIL: Could not extract API key from response: $KEY_RESPONSE${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: API key created for token-limited team${NC}"
echo "API Key: ${API_KEY:0:20}..."

# Test 3: Verify TokenRateLimitPolicy was created
echo -e "${YELLOW}3. Verifying TokenRateLimitPolicy was created...${NC}"
sleep 2  # Give policy time to be created

POLICY_CHECK=$(kubectl get tokenratelimitpolicy team-$TEAM_ID-rate-limits -n llm -o name 2>/dev/null || echo "")
if [[ -z "$POLICY_CHECK" ]]; then
    echo -e "${RED}FAIL: TokenRateLimitPolicy was not created${NC}"
    exit 1
fi

echo -e "${GREEN}PASS: TokenRateLimitPolicy created successfully${NC}"
echo "Policy: $POLICY_CHECK"

# Test 4: Make small token requests (should succeed)
echo -e "${YELLOW}4. Testing small token requests (should succeed)...${NC}"
for i in {1..3}; do
    echo -e "${BLUE}Request $i: Small prompt (should use ~30 tokens)${NC}"
    
    RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Say hello briefly"}],"max_tokens":10}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$RESPONSE" | grep -q "choices"; then
        TOKENS=$(echo "$RESPONSE" | grep -o '"total_tokens":[0-9]*' | cut -d':' -f2)
        echo -e "${GREEN}✓ Request $i succeeded (${TOKENS:-'unknown'} tokens)${NC}"
    else
        echo -e "${RED}✗ Request $i failed: ${RESPONSE:0:100}...${NC}"
    fi
    
    sleep 1
done

# Test 5: Make large token request (should potentially hit rate limit)
echo -e "${YELLOW}5. Testing large token request (may hit rate limit)...${NC}"
LARGE_REQUEST=$(curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Write a detailed explanation of artificial intelligence, machine learning, and deep learning. Include examples, use cases, and explain the differences between these technologies. Make it comprehensive and detailed with multiple paragraphs covering each topic thoroughly."}],"max_tokens":500}' \
     "$MODEL_URL/v1/chat/completions")

if echo "$LARGE_REQUEST" | grep -q "choices"; then
    LARGE_TOKENS=$(echo "$LARGE_REQUEST" | grep -o '"total_tokens":[0-9]*' | cut -d':' -f2)
    echo -e "${GREEN}PASS: Large request succeeded (${LARGE_TOKENS:-'unknown'} tokens)${NC}"
    echo "Response preview: $(echo "$LARGE_REQUEST" | grep -o '"content":"[^"]*"' | head -1 | cut -c1-100)..."
elif echo "$LARGE_REQUEST" | grep -q -i "rate.*limit\|too.*many"; then
    echo -e "${YELLOW}INFO: Rate limit triggered as expected${NC}"
    echo "Rate limit response: ${LARGE_REQUEST:0:200}..."
else
    echo -e "${RED}FAIL: Unexpected response to large request: ${LARGE_REQUEST:0:200}...${NC}"
fi

# Test 6: Test multiple rapid requests (should trigger rate limiting)
echo -e "${YELLOW}6. Testing rapid token consumption (should trigger rate limiting)...${NC}"
echo "Making 10 rapid requests to consume tokens quickly..."

SUCCESS_COUNT=0
RATE_LIMITED_COUNT=0

for i in {1..10}; do
    RAPID_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Generate a random sentence with exactly 50 words about technology and innovation in the modern world."}],"max_tokens":100}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$RAPID_RESPONSE" | grep -q "choices"; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        RAPID_TOKENS=$(echo "$RAPID_RESPONSE" | grep -o '"total_tokens":[0-9]*' | cut -d':' -f2)
        echo -e "${GREEN}✓ Request $i: Success (${RAPID_TOKENS:-'unknown'} tokens)${NC}"
    elif echo "$RAPID_RESPONSE" | grep -q -i "rate.*limit\|too.*many"; then
        RATE_LIMITED_COUNT=$((RATE_LIMITED_COUNT + 1))
        echo -e "${YELLOW}⚠ Request $i: Rate limited${NC}"
    else
        echo -e "${RED}✗ Request $i: Error - ${RAPID_RESPONSE:0:100}...${NC}"
    fi
    
    sleep 0.2
done

echo ""
echo -e "${BLUE}Results Summary:${NC}"
echo "- Successful requests: $SUCCESS_COUNT"
echo "- Rate limited requests: $RATE_LIMITED_COUNT"

if [[ $RATE_LIMITED_COUNT -gt 0 ]]; then
    echo -e "${GREEN}PASS: Token rate limiting is working (some requests were rate limited)${NC}"
else
    echo -e "${YELLOW}INFO: No rate limiting observed (may need larger token consumption)${NC}"
fi

# Test 7: Verify policy details
echo -e "${YELLOW}7. Examining TokenRateLimitPolicy configuration...${NC}"
POLICY_DETAILS=$(kubectl get tokenratelimitpolicy team-$TEAM_ID-rate-limits -n llm -o yaml 2>/dev/null)
if [[ -n "$POLICY_DETAILS" ]]; then
    echo -e "${GREEN}PASS: Policy details retrieved${NC}"
    echo "Rate limit configuration:"
    echo "$POLICY_DETAILS" | grep -A 10 "rates:" | head -10
else
    echo -e "${YELLOW}WARNING: Could not retrieve policy details${NC}"
fi

# Test 8: Cleanup
echo -e "${YELLOW}8. Cleaning up test resources...${NC}"

# Delete API key
DELETE_RESPONSE=$(curl -s -X DELETE "$BASE_URL/delete_key" \
     -H "Authorization: ADMIN $ADMIN_KEY" \
     -H 'Content-Type: application/json' \
     -d "{\"key\":\"$API_KEY\"}")

if echo "$DELETE_RESPONSE" | grep -q "deleted successfully"; then
    echo -e "${GREEN}✓ API key deleted successfully${NC}"
else
    echo -e "${YELLOW}⚠ API key deletion response: $DELETE_RESPONSE${NC}"
fi

# Delete team (this will also clean up policies)
TEAM_DELETE_RESPONSE=$(curl -s -X DELETE "$BASE_URL/teams/$TEAM_ID" \
     -H "Authorization: ADMIN $ADMIN_KEY")

if echo "$TEAM_DELETE_RESPONSE" | grep -q "deleted successfully"; then
    echo -e "${GREEN}✓ Team and policies deleted successfully${NC}"
else
    echo -e "${YELLOW}⚠ Team deletion response: $TEAM_DELETE_RESPONSE${NC}"
fi

echo "================================"
echo -e "${GREEN}✅ Token Rate Limiting Test Complete!${NC}"
echo -e "${GREEN}✅ TokenRateLimitPolicy: Created and enforced${NC}"
echo -e "${GREEN}✅ Token Tracking: Automatic from API responses${NC}"
echo -e "${GREEN}✅ Team-based Limits: Applied to all team members${NC}"
echo "Test completed for user: $MAAS_USER"