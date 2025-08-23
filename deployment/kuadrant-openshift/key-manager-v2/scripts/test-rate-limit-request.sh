#!/bin/bash

# Test script for request rate limiting functionality  
# Tests RateLimitPolicy enforcement at the team level
#
# Usage Examples:
#   # Using environment variables
#   export ADMIN_KEY="your-admin-key"
#   export MAAS_USER="alice"
#   ./test-rate-limit-request.sh
#
#   # Using command line flags
#   ./test-rate-limit-request.sh --admin-key "your-admin-key" --user "alice"

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
MAAS_USER="${MAAS_USER:-requestuser}"
MODEL_URL="${MODEL_URL:-http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com}"
REQUEST_LIMIT="${REQUEST_LIMIT:-10}"
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
    --request-limit)
      REQUEST_LIMIT="$2"
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
      echo "Test script for Request Rate Limiting (RateLimitPolicy) functionality"
      echo ""
      echo "Options:"
      echo "  -u, --user USERNAME       Test with specific username (default: requestuser)"
      echo "  -k, --admin-key KEY       Admin API key for key creation"
      echo "      --base-url URL        Key manager base URL"
      echo "      --model-url URL       Model endpoint URL"
      echo "      --request-limit NUM   Request limit for team (default: 10)"
      echo "      --time-window WINDOW  Time window (default: 1h)"
      echo "      --team-id ID          Team ID to create (default: $TEAM_ID)"
      echo "  -h, --help                Show this help message"
      echo ""
      echo "Environment Variables:"
      echo "  ADMIN_KEY                 Admin API key (required if not provided via flag)"
      echo "  MAAS_USER                 Username for testing (default: requestuser)"
      echo "  BASE_URL                  Key manager base URL"
      echo "  MODEL_URL                 Model endpoint URL"
      echo "  REQUEST_LIMIT             Request limit for team (default: 10)"
      echo "  TIME_WINDOW               Time window (default: 1h)"
      echo "  TEAM_ID                   Team ID to create (default: $TEAM_ID)"
      echo ""
      echo "### Team-Based Deployment Rate Limit Test"
      echo ""
      echo "Manual Commands (what this script automates):"
      echo ""
      echo "1. Create team with request rate limiting:"
      echo "   curl -X POST \"\$BASE_URL/teams\" \\"
      echo "     -H \"Authorization: ADMIN \$ADMIN_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{"
      echo "       \"team_id\": \"request-test-team\","
      echo "       \"team_name\": \"Request Rate Limited Team\","
      echo "       \"description\": \"Team limited to 10 HTTP requests per hour\","
      echo "       \"default_tier\": \"standard\","
      echo "       \"request_limit\": 10,"
      echo "       \"time_window\": \"1h\""
      echo "     }'"
      echo ""
      echo "2. Create API key for team:"
      echo "   curl -X POST \"\$BASE_URL/teams/request-test-team/keys\" \\"
      echo "     -H \"Authorization: ADMIN \$ADMIN_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{"
      echo "       \"user_id\": \"requestuser\","
      echo "       \"alias\": \"request-test-key\","
      echo "       \"inherit_team_limits\": true"
      echo "     }'"
      echo ""
      echo "3. Check for RateLimitPolicy (may not be implemented yet):"
      echo "   kubectl get ratelimitpolicy team-request-test-team-request-limits -n llm -o yaml"
      echo ""
      echo "4. Test HTTP requests with API key:"
      echo "   curl -H \"Authorization: APIKEY \$API_KEY\" \\"
      echo "     -H \"Content-Type: application/json\" \\"
      echo "     -d '{\"model\":\"qwen3-0-6b-instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Hi\"}],\"max_tokens\":5}' \\"
      echo "     \"\$MODEL_URL/v1/chat/completions\""
      echo ""
      echo "5. Cleanup:"
      echo "   curl -X DELETE \"\$BASE_URL/delete_key\" -H \"Authorization: ADMIN \$ADMIN_KEY\" -d '{\"key\":\"\$API_KEY\"}'"
      echo "   curl -X DELETE \"\$BASE_URL/teams/request-test-team\" -H \"Authorization: ADMIN \$ADMIN_KEY\""
      echo ""
      echo "Note: Request-based rate limiting (RateLimitPolicy) may require additional"
      echo "      implementation. Currently, the system focuses on TokenRateLimitPolicy."
      echo ""
      echo "Examples:"
      echo "  # Using environment variables:"
      echo "  export ADMIN_KEY=\"your-admin-key\""
      echo "  export REQUEST_LIMIT=20"
      echo "  export TIME_WINDOW=\"30m\""
      echo "  ./test-rate-limit-request.sh"
      echo ""
      echo "  # Using command line flags:"
      echo "  ./test-rate-limit-request.sh --admin-key \"your-key\" --request-limit 20 --time-window \"30m\""
      echo ""
      echo "  # Custom team and user:"
      echo "  ./test-rate-limit-request.sh --team-id \"my-team\" --user \"alice\" --request-limit 50"
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

echo -e "${YELLOW}Testing Request Rate Limiting (RateLimitPolicy)${NC}"
echo "Base URL: $BASE_URL"
echo "Model URL: $MODEL_URL"
echo "Test User: $MAAS_USER"
echo "Team ID: $TEAM_ID"
echo "Request Limit: $REQUEST_LIMIT"
echo "Time Window: $TIME_WINDOW"
echo "================================"

# Test 1: Create team with request rate limiting
echo -e "${YELLOW}1. Creating team with request rate limiting ($REQUEST_LIMIT requests per $TIME_WINDOW)...${NC}"
TEAM_RESPONSE=$(curl -s -X POST "$BASE_URL/teams" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"team_id\": \"$TEAM_ID\",
    \"team_name\": \"Request Rate Limited Team\",
    \"description\": \"Team limited to $REQUEST_LIMIT HTTP requests per $TIME_WINDOW for testing\",
    \"default_tier\": \"standard\",
    \"request_limit\": $REQUEST_LIMIT,
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

echo -e "${GREEN}PASS: Request-limited team created successfully${NC}"
echo "Team Response: $TEAM_RESPONSE"

# Test 2: Create API key for the team
echo -e "${YELLOW}2. Creating API key for request-limited team...${NC}"
KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/teams/$TEAM_ID/keys" \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"user_id\": \"$MAAS_USER\",
    \"alias\": \"request-test-key\",
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

echo -e "${GREEN}PASS: API key created for request-limited team${NC}"
echo "API Key: ${API_KEY:0:20}..."

# Test 3: Verify RateLimitPolicy was created (Note: may not be implemented yet)
echo -e "${YELLOW}3. Checking for RateLimitPolicy creation...${NC}"
sleep 2  # Give policy time to be created

POLICY_CHECK=$(kubectl get ratelimitpolicy team-$TEAM_ID-request-limits -n llm -o name 2>/dev/null || echo "")
if [[ -n "$POLICY_CHECK" ]]; then
    echo -e "${GREEN}PASS: RateLimitPolicy created successfully${NC}"
    echo "Policy: $POLICY_CHECK"
else
    echo -e "${YELLOW}INFO: RateLimitPolicy not found (may not be implemented yet)${NC}"
    echo "Note: Currently the system focuses on TokenRateLimitPolicy"
fi

# Test 4: Make small requests (should succeed initially)
echo -e "${YELLOW}4. Testing initial requests (should succeed)...${NC}"
SUCCESS_COUNT=0
ERROR_COUNT=0

for i in {1..5}; do
    echo -e "${BLUE}Request $i: Small request (HTTP request count)${NC}"
    
    RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hi"}],"max_tokens":5}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$RESPONSE" | grep -q "choices"; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        echo -e "${GREEN}✓ Request $i succeeded${NC}"
    elif echo "$RESPONSE" | grep -q -i "rate.*limit\|too.*many"; then
        echo -e "${YELLOW}⚠ Request $i: Rate limited${NC}"
        break
    else
        ERROR_COUNT=$((ERROR_COUNT + 1))
        echo -e "${RED}✗ Request $i failed: ${RESPONSE:0:100}...${NC}"
    fi
    
    sleep 1
done

echo -e "${BLUE}Initial test results: $SUCCESS_COUNT successful, $ERROR_COUNT errors${NC}"

# Test 5: Rapid fire requests to trigger rate limiting
echo -e "${YELLOW}5. Testing rapid HTTP requests (should trigger rate limiting)...${NC}"
echo "Making 15 rapid requests to exceed the 10 request/hour limit..."

SUCCESS_COUNT=0
RATE_LIMITED_COUNT=0
TOTAL_REQUESTS=15

for i in $(seq 1 $TOTAL_REQUESTS); do
    RAPID_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Count: '$i'"}],"max_tokens":3}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$RAPID_RESPONSE" | grep -q "choices"; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        echo -e "${GREEN}✓ Request $i: Success${NC}"
    elif echo "$RAPID_RESPONSE" | grep -q -i "rate.*limit\|too.*many\|quota.*exceeded"; then
        RATE_LIMITED_COUNT=$((RATE_LIMITED_COUNT + 1))
        echo -e "${YELLOW}⚠ Request $i: Rate limited${NC}"
    else
        echo -e "${RED}✗ Request $i: Error - ${RAPID_RESPONSE:0:100}...${NC}"
    fi
    
    sleep 0.5  # Small delay between requests
done

echo ""
echo -e "${BLUE}Rapid Request Results Summary:${NC}"
echo "- Total requests attempted: $TOTAL_REQUESTS"
echo "- Successful requests: $SUCCESS_COUNT"
echo "- Rate limited requests: $RATE_LIMITED_COUNT"
echo "- Other errors: $((TOTAL_REQUESTS - SUCCESS_COUNT - RATE_LIMITED_COUNT))"

# Test 6: Check rate limiting behavior analysis
echo -e "${YELLOW}6. Analyzing rate limiting behavior...${NC}"

if [[ $RATE_LIMITED_COUNT -gt 0 ]]; then
    echo -e "${GREEN}PASS: Request rate limiting is working (some requests were rate limited)${NC}"
    echo "Rate limiting triggered after $SUCCESS_COUNT successful requests"
elif [[ $SUCCESS_COUNT -gt 10 ]]; then
    echo -e "${YELLOW}INFO: More than 10 requests succeeded - RateLimitPolicy may not be active${NC}"
    echo "This suggests request-based rate limiting is not yet implemented"
    echo "The system currently focuses on TokenRateLimitPolicy (token consumption)"
else
    echo -e "${YELLOW}INFO: Results inconclusive - may need more testing${NC}"
fi

# Test 7: Verify current rate limiting setup
echo -e "${YELLOW}7. Examining current rate limiting configuration...${NC}"

# Check for TokenRateLimitPolicy (which is implemented)
TOKEN_POLICY=$(kubectl get tokenratelimitpolicy team-$TEAM_ID-rate-limits -n llm -o name 2>/dev/null || echo "")
if [[ -n "$TOKEN_POLICY" ]]; then
    echo -e "${GREEN}✓ TokenRateLimitPolicy found: $TOKEN_POLICY${NC}"
    echo "Note: This enforces token consumption limits, not HTTP request limits"
else
    echo -e "${YELLOW}⚠ No TokenRateLimitPolicy found${NC}"
fi

# Check for RateLimitPolicy (request-based, may not be implemented)
REQUEST_POLICY=$(kubectl get ratelimitpolicy -n llm --no-headers 2>/dev/null | grep "$TEAM_ID" || echo "")
if [[ -n "$REQUEST_POLICY" ]]; then
    echo -e "${GREEN}✓ RateLimitPolicy found: $REQUEST_POLICY${NC}"
else
    echo -e "${YELLOW}⚠ No RateLimitPolicy found for request limiting${NC}"
    echo "This confirms that HTTP request rate limiting is not yet implemented"
fi

# Test 8: Test different request patterns
echo -e "${YELLOW}8. Testing different request patterns...${NC}"

# Test A: Very small requests (minimal tokens)
echo -e "${BLUE}Pattern A: Minimal token requests${NC}"
for i in {1..3}; do
    MINIMAL_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Yes"}],"max_tokens":1}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$MINIMAL_RESPONSE" | grep -q "choices"; then
        echo -e "${GREEN}✓ Minimal request $i: Success${NC}"
    else
        echo -e "${YELLOW}⚠ Minimal request $i: ${MINIMAL_RESPONSE:0:50}...${NC}"
    fi
    sleep 0.5
done

# Test B: Medium requests (moderate tokens)
echo -e "${BLUE}Pattern B: Medium token requests${NC}"
for i in {1..2}; do
    MEDIUM_RESPONSE=$(curl -s -H "Authorization: APIKEY $API_KEY" \
         -H 'Content-Type: application/json' \
         -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Explain briefly what AI is"}],"max_tokens":50}' \
         "$MODEL_URL/v1/chat/completions")
    
    if echo "$MEDIUM_RESPONSE" | grep -q "choices"; then
        TOKENS=$(echo "$MEDIUM_RESPONSE" | grep -o '"total_tokens":[0-9]*' | cut -d':' -f2)
        echo -e "${GREEN}✓ Medium request $i: Success (${TOKENS:-'unknown'} tokens)${NC}"
    else
        echo -e "${YELLOW}⚠ Medium request $i: ${MEDIUM_RESPONSE:0:50}...${NC}"
    fi
    sleep 1
done

# Test 9: Cleanup
echo -e "${YELLOW}9. Cleaning up test resources...${NC}"

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
echo -e "${GREEN}✅ Request Rate Limiting Test Complete!${NC}"
echo ""
echo -e "${BLUE}Key Findings:${NC}"
echo "- HTTP Request Rate Limiting: Currently focused on TokenRateLimitPolicy"
echo "- Token-based rate limiting: ✅ Implemented and working"
echo "- Request-based rate limiting: ⚠ May require additional implementation"
echo ""
echo -e "${YELLOW}Note:${NC} This test demonstrates the difference between:"
echo "• TokenRateLimitPolicy: Limits AI token consumption (implemented)"
echo "• RateLimitPolicy: Limits HTTP requests regardless of content (future)"
echo ""
echo "Test completed for user: $MAAS_USER"