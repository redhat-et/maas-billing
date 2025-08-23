# MaaS Platform Key Manager

API key management service for MaaS (Models as a Service) platform with team management and seamless Kuadrant integration.

## Overview

The MaaS Platform Key Manager provides enterprise-grade AI model access through a comprehensive management system. This implementation features team-based management, automatic policy inheritance, and seamless integration with Kuadrant and KServe.

## Prerequisites

- OpenShift cluster with Kuadrant operator installed
- KServe operator installed for model serving
- kubectl/oc CLI tools configured with admin access
- Admin API key configured for service authentication
- Docker for building images

Set your admin key as an environment variable before running any commands:

```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"
```

## Key Features

- **Intelligent Defaults**: Zero-configuration deployment with automatic default team and tier assignment
- **Team Management**: Multi-tenant teams with inherited policies
- **Dynamic Policy Generation**: Automatically creates Kuadrant policies when teams are created
- **Flexible Tier System**: Hardcoded tiers (free/standard/premium/unlimited) with optional ConfigMap override
- **Default Unlimited Policy**: Simple deployments get unlimited access by default
- **Team-Scoped API Keys**: Keys with rich metadata and rate limiting
- **KServe Integration**: Seamless model serving with automatic scaling
- **Hierarchical Policies**: Platform → Tier → Team → User → Key inheritance

## API Endpoints

### Legacy Endpoints
- `POST /generate_key` - Generate API key (now uses default team)
- `DELETE /delete_key` - Delete an existing API key
- `GET /health` - Service health check
- `GET /models` - List available models

### Team Management Endpoints

#### Team Management
- `POST /teams` - Create new team with tier and rate limit defaults
- `GET /teams` - List all teams
- `GET /teams/{team_id}` - Get team details with members and keys
- `DELETE /teams/{team_id}` - Remove team and cleanup all resources

#### User Management  
- `POST /teams/{team_id}/members` - Add user to team with role and limits
- `GET /teams/{team_id}/members` - List team members
- `DELETE /teams/{team_id}/members/{user_id}` - Remove user from team

#### API Key Management
- `POST /teams/{team_id}/keys` - Create team-scoped API key with rate/model limits
- `GET /teams/{team_id}/keys` - List team API keys
- `PATCH /keys/{key_id}` - Update key status/models
- `DELETE /keys/{key_id}` - Delete specific API key

#### Policy Management
- `GET /admin/policies` - List all team policies 
- `GET /admin/policies/{team_id}` - Get team-specific policy details
- `POST /admin/policies/sync` - Force policy synchronization
- `GET /admin/policies/defaults` - View default tier policies
- `GET /admin/policies/compliance` - Team compliance report

#### Activity & Usage
- `GET /teams/{team_id}/activity` - Team activity and spending summary
- `GET /teams/{team_id}/usage` - Detailed usage breakdown by user/model

## Deployment

### Quick Start

```bash
# Apply all manifests
kubectl apply -f .

# Set admin key
kubectl create secret generic admin-key-secret \
  --from-literal=admin-key="your-secure-admin-key" \
  -n platform-services

# Wait for service to be ready
kubectl rollout status deployment/key-manager -n platform-services
```

### Build and Deploy Custom Image

```bash
# Build the image
docker build -t ghcr.io/nerdalert/maas-key-manager:teams .

# Push to registry
docker push ghcr.io/nerdalert/maas-key-manager:teams

# Wait for service to be ready
kubectl rollout status deployment/key-manager -n platform-services
```

## Testing and Deployment Examples

The Key Manager supports different deployment patterns for various use cases. Each pattern includes both manual commands and automated test scripts.

### Simple Deployment Test

Basic functionality testing with the default team configuration.

**What it tests:**
- Health endpoint functionality
- Legacy API key generation
- Default team assignment
- Basic model access
- Key cleanup

**Manual Commands:**
```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"
# You can skip the ADMIN_KEY if already exported
export MAAS_USER="${MAAS_USER:-testuser}"

# 1. Health check
curl http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/health

# 2. Generate API key (uses default team automatically)
# Note: Users can have multiple API keys for different purposes
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/generate_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$MAAS_USER\"}")

echo "API Key Response: $API_RESPONSE"

# 3. Extract API key
API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"

# 4. Test model access
curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello!"}],"max_tokens":20}' \
     http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 5. Delete API key when done (prevents key accumulation)
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}"

echo "API key deleted for user: $MAAS_USER"
```

**Automated Script:**
```bash
export ADMIN_KEY="your-admin-key"
./scripts/test-request.sh
```

### Team Deployment with Default Policy

Tests team creation with default tier policies (no custom rate limiting).

**What it tests:**
- Team creation with default settings
- Automatic policy inheritance from tier
- Team-scoped API key creation
- Policy verification in Kubernetes

**Manual Commands:**
```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"
# You can skip the ADMIN_KEY if already exported
export MAAS_USER="${MAAS_USER:-teamuser}"

# 1. Create team with default tier
curl -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "default-policy-team",
    "team_name": "Default Policy Team",
    "description": "Team using standard tier defaults",
    "default_tier": "standard"
  }'

# 2. Create team API key
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/default-policy-team/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$MAAS_USER\",
    \"alias\": \"default-policy-key\",
    \"inherit_team_limits\": true
  }")

echo "Team API Key Response: $API_RESPONSE"

# 3. Extract API key
API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"

# 4. Verify TokenRateLimitPolicy was created
kubectl get tokenratelimitpolicy team-default-policy-team-rate-limits -n llm -o yaml

# 5. Test model access
curl -s -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello team"}],"max_tokens":50}' \
  http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 6. Cleanup
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}"
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/default-policy-team \
  -H "Authorization: ADMIN $ADMIN_KEY"

echo "API key deleted and team removed"
```

**Environment Variables:**
```bash
export ADMIN_KEY="your-admin-key"
export TEAM_ID="default-policy-team"
export MAAS_USER="teamuser"
```

### Team Deployment with Rate-Limiting Policy

Tests HTTP request rate limiting (RateLimitPolicy) - limits number of API calls regardless of content.

**What it tests:**
- Team creation with HTTP request limits
- RateLimitPolicy generation (if implemented)
- Request counting and rate limiting
- Rapid request testing to trigger limits

**Manual Commands:**
```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"
# You can skip the ADMIN_KEY if already exported
export MAAS_USER="${MAAS_USER:-requestuser}"

# 1. Create team with request rate limiting
curl -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "request-limited-team",
    "team_name": "Request Rate Limited Team",
    "description": "Team limited to 10 HTTP requests per hour",
    "default_tier": "standard",
    "request_limit": 10,
    "time_window": "1h"
  }'

# 2. Create API key for team
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/request-limited-team/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$MAAS_USER\",
    \"alias\": \"request-test-key\",
    \"inherit_team_limits\": true
  }")

echo "Team API Key Response: $API_RESPONSE"

# 3. Extract API key
API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"

# 4. Check for RateLimitPolicy (may not be implemented yet)
kubectl get ratelimitpolicy team-request-limited-team-request-limits -n llm -o yaml

# 5. Test multiple HTTP requests (should trigger rate limiting after 10 requests)
for i in {1..15}; do
  echo "Request $i:"
  curl -s -H "Authorization: APIKEY $API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Request '$i'"}],"max_tokens":5}' \
    http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .
  sleep 0.5
done

# 6. Cleanup
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}"
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/request-limited-team \
  -H "Authorization: ADMIN $ADMIN_KEY"

echo "API key deleted and team removed"
```

**Automated Script:**
```bash
export ADMIN_KEY="your-admin-key"
export REQUEST_LIMIT=10
export TIME_WINDOW="1h"
export TEAM_ID="request-limited-team"
./scripts/test-rate-limit-request.sh
```

**Script with custom parameters:**
```bash
./scripts/test-rate-limit-request.sh \
  --admin-key "your-key" \
  --request-limit 20 \
  --time-window "30m" \
  --team-id "my-request-team"
```

### Team Deployment with Token Rate Policy

Tests AI token consumption rate limiting (TokenRateLimitPolicy) - limits based on AI tokens consumed.

**What it tests:**
- Team creation with AI token limits
- TokenRateLimitPolicy generation and enforcement
- Token consumption tracking from API responses
- Rate limiting based on cumulative token usage

**Manual Commands:**
```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"
# You can skip the ADMIN_KEY if already exported
export MAAS_USER="${MAAS_USER:-tokenuser}"

# 1. Create team with token rate limiting
curl -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "token-limited-team",
    "team_name": "Token Rate Limited Team",
    "description": "Team limited to 5,000 AI tokens per hour",
    "default_tier": "standard",
    "token_limit": 5000,
    "time_window": "1h"
  }'

# 2. Create API key for team
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/token-limited-team/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$MAAS_USER\",
    \"alias\": \"token-test-key\",
    \"inherit_team_limits\": true
  }")

echo "Team API Key Response: $API_RESPONSE"

# 3. Extract API key
API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"

# 4. Verify TokenRateLimitPolicy was created
kubectl get tokenratelimitpolicy team-token-limited-team-rate-limits -n llm -o yaml

# 5. Test small token requests (should succeed)
echo "Testing small token request:"
curl -s -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Hello"}],"max_tokens":10}' \
  http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 6. Test large token request (may hit rate limit)
echo "Testing large token request:"
curl -s -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Write a detailed explanation of artificial intelligence with examples and use cases. Make it comprehensive."}],"max_tokens":500}' \
  http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 7. Cleanup
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}"
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/token-limited-team \
  -H "Authorization: ADMIN $ADMIN_KEY"

echo "API key deleted and team removed"
```

**Automated Script:**
```bash
export ADMIN_KEY="your-admin-key"
export TOKEN_LIMIT=5000
export TIME_WINDOW="1h"
export TEAM_ID="token-limited-team"
./scripts/test-token-limit-request.sh
```

**Script with custom parameters:**
```bash
./scripts/test-token-limit-request.sh \
  --admin-key "your-key" \
  --token-limit 10000 \
  --time-window "30m" \
  --team-id "my-token-team"
```

## Rate Limiting Comparison

| Type | Controls | Use Case | Kuadrant Resource |
|------|----------|----------|-------------------|
| **Request Rate Limiting** | HTTP requests per time window | API abuse prevention, fair access | RateLimitPolicy |
| **Token Rate Limiting** | AI tokens consumed per time window | Cost control, resource allocation | TokenRateLimitPolicy |

### Combined Rate Limiting

You can apply both types to the same team - the team will be limited by whichever constraint is reached first:

```bash
export ADMIN_KEY="<INSERT_ADMIN_KEY>"

curl -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "dual-limited-team",
    "team_name": "Dual Rate Limited Team",
    "description": "Limited by both requests and tokens",
    "default_tier": "standard",
    "request_limit": 200,
    "token_limit": 75000,
    "time_window": "1h"
  }'
```

## Architecture Overview

### Policy Architecture

The system uses a three-tier policy architecture:

1. **Default Unlimited Policy** (`gateway-default-unlimited`)
   - Applied to all users by default
   - No rate limits = unlimited usage
   - Simple deployments can stop here

2. **Team-Specific Policies** (created dynamically)
   - Generated when teams are created: `team-{id}-rate-limits`
   - Override default policy for team members
   - Based on team tier (free/standard/premium/unlimited)

3. **Tier System** (flexible definitions)
   - **Hardcoded tiers**: free, standard, premium, unlimited
   - **Optional ConfigMap**: Custom tier definitions override hardcoded ones
   - **Environment defaults**: `DEFAULT_TIER=standard` (configurable)

### User Assignment Logic

```
User Creates API Key
├─ Has team assignment?
│  ├─ YES → Use team-specific policy (team tier limits)
│  └─ NO  → Auto-assign to "default" team (uses DEFAULT_TIER)
└─ Team has policy?
   ├─ YES → Apply team rate limits
   └─ NO  → Use unlimited default policy
```

### Dynamic Policy Generation

When a team is created:
1. Key-manager validates tier (hardcoded or ConfigMap)
2. Generates TokenRateLimitPolicy manifest using Kubernetes dynamic client
3. Policy targets users with `maas/team-id` label
4. Tier limits determine token rates, model access

### Components
- **Key Manager**: Team-based API key management with intelligent defaults (`cmd/key-manager/`)
- **Policy Engine**: Dynamic Kuadrant policy generation (`internal/policies/`) 
- **Default Team**: Auto-created for backwards compatibility
- **Flexible Tiers**: Hardcoded definitions with optional ConfigMap override

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `KEY_NAMESPACE` | `llm` | Kubernetes namespace for secrets |
| `SECRET_SELECTOR_LABEL` | `kuadrant.io/apikeys-by` | Label selector for API key secrets |
| `SECRET_SELECTOR_VALUE` | `rhcl-keys` | Label value for API key secrets |
| `DISCOVERY_ROUTE` | `inference-route` | Route name for model discovery |
| `ENABLE_POLICY_MGMT` | `true` | Enable Kuadrant policy management |
| `CREATE_DEFAULT_TEAM` | `true` | Auto-create default team on startup |
| `DEFAULT_TEAM_TIER` | `standard` | Default tier for auto-created team |
| `DEFAULT_TIER` | `standard` | Fallback tier for teams without specification |
| `GATEWAY_NAME` | `inference-gateway` | Target gateway for policies |
| `GATEWAY_NAMESPACE` | `llm` | Gateway namespace |
| `POLICY_CONFIG_MAP` | `tier-config` | ConfigMap name for custom tier definitions |

### Deployment Modes

#### 1. Simple Mode (Default)
```bash
# Zero configuration - just deploy
# Users get unlimited access via default team
DEFAULT_TIER=unlimited
CREATE_DEFAULT_TEAM=true
```

#### 2. Controlled Mode (Recommended)
```bash
# Sensible defaults with team management
DEFAULT_TIER=standard
ENABLE_POLICY_MGMT=true
# Teams can be created with specific limits
```

#### 3. Restrictive Mode (Minimal Default)
```bash
# Start with minimal limits
DEFAULT_TIER=free
# Users get basic limits, teams can upgrade as needed
```

## Testing Scripts

The `scripts/` directory contains comprehensive testing tools:

- `scripts/test-request.sh` - Basic functionality testing
- `scripts/test-token-limit-request.sh` - Token rate limiting validation  
- `scripts/test-rate-limit-request.sh` - Request rate limiting validation

Run tests with:
```bash
export ADMIN_KEY="your-admin-key"
./scripts/test-token-limit-request.sh
./scripts/test-rate-limit-request.sh
```

## Migration Support

- **Backwards Compatible**: Legacy endpoints continue to work
- **Gradual Migration**: Move from simple to team-based management at your own pace
- **Zero Downtime**: Deploy new version alongside existing implementation
- **Existing Keys**: All current API keys continue to function
- **Intelligent Defaults**: New users automatically get sensible limits

For detailed implementation guidance, see the documentation files:
- `TESTING.md` - Comprehensive testing procedures
- `maas-architecture.md` - Complete system architecture
- `v2-key-api.md` - Detailed API specifications