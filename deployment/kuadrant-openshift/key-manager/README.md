# Key Manager Service

API key management service for MaaS (Models as a Service) platform with Kuadrant integration.

## Prerequisites

Set your admin key as an environment variable before running any commands:

```bash
export ADMIN_KEY=<INSERT_KEY_HERE>
```

## Overview

The key-manager provides secure API key CRUD operations for accessing LLM model endpoints. It integrates with Kuadrant for authentication and token-based rate limiting.

## API Endpoints

### Admin Endpoints (require admin authentication)
- `POST /generate_key` - Generate new API key for a user
- `DELETE /delete_key` - Delete an existing API key
- `GET /v1/models` - List available models (TODO: currently hardcoded, simple fix, just needs to query the HTTPRoute resources)
- `GET /health` - Service health check

**Workflow:**
1. **Generate** API keys for users via REST API
2. **Store** keys as Kubernetes Secrets with proper labels/annotations
3. **Authenticate** requests using Kuadrant AuthPolicy
4. **Rate limit** by token consumption using TokenRateLimitPolicy
5. **Monitor** usage via Prometheus metrics

## Quick Test (Remote Access)

Complete workflow test using remote endpoints (Make sure `echo $ADMIN_KEY` prints a key:

```bash
# 1. Generate API key
export ADMIN_KEY=<INSERT_ADMIN_KEY>
# You can skup the ADMIN_KEY if already exported
export MAAS_USER=${MAAS_USER:-mittens}

echo "Step 1: Generate API key for user: $MAAS_USER"
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/generate_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$MAAS_USER\"}")
echo "Response: $API_RESPONSE"

# 2. Extract API key
API_KEY=$(echo $API_RESPONSE | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"

# 3. List available models
echo -e "\nStep 2: List models"
curl -s http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/models \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .

# 4. Test model chat completion (simulator)
echo -e "\nStep 3: Test simulator model"
curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello from $MAAS_USER!"}],"max_tokens":20}' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 5. Test qwen model
echo -e "\nStep 4: Test qwen model"
curl -s -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Cats or dogs? Discuss"}],"max_tokens":15}' \
     http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions | jq .

# 6. Delete the API key
echo -e "\nStep 5: Clean up - delete API key"
curl -s -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
     -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}" | jq .

echo -e "\nQuick test completed"
```

## Installation

### 1. Configure Admin Key

**REQUIRED**: Update the admin key before installation:

```bash
# Make sure ADMIN_KEY environment variable is set (see Prerequisites above)
echo "Using admin key: $ADMIN_KEY"

# Replace placeholder in admin secret file
sed -i "s/<INSERT-ADMIN-KEY>/$ADMIN_KEY/g" 07-admin-secret.yaml
```

### 2. Build and Deploy

```bash
# Build container image
docker build -t ghcr.io/<GH_ID>/maas-key-manager:latest .
docker push ghcr.io/<GH_ID>/maas-key-manager:latest

**Note**: Make sure you've configured the admin key in step 1 before running:

```bash
kubectl apply -k .
```

### 4. Enable Remote Access

The key-manager is accessible via both HTTPRoute and OpenShift Route for external access:

**Verify HTTPRoute (already included):**
```bash
# HTTPRoute is applied automatically with kustomize
kubectl get httproute key-manager-domain-route -n llm
```

**Verify OpenShift Route (already included):**
```bash
# OpenShift Route is applied automatically with kustomize
kubectl get route key-manager-route -n llm
```

> **Note**: Both routes are required in this OpenShift environment. The OpenShift Route handles external `*.apps` domain traffic and forwards it to the Istio gateway, while the HTTPRoute handles internal routing within the Istio mesh.

**Access URLs:**
- **HTTPRoute**: `http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com`
- **Models**: `https://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com`

> **Note**: External examples use HTTP. For HTTPS access, use `curl -k` to ignore self-signed certificate warnings, or configure proper TLS termination in the OpenShift Route.

## API Endpoints

### Health Check

**Via kubectl:**
```bash
kubectl exec -it deployment/key-manager -n platform-services -- \
  curl http://localhost:8080/health
```

**Response:**
```json
{"status":"healthy"}
```

### Generate API Key

**External curl:**
```bash
curl -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/generate_key \
  -H 'Authorization: ADMIN $ADMIN_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"test-user-1"}'
```

**Via kubectl (with admin auth):**
```bash
kubectl exec -it deployment/key-manager -n platform-services -- \
  curl -X POST http://localhost:8080/generate_key \
    -H 'Authorization: ADMIN $ADMIN_KEY' \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"test-user-1"}'
```

**Response:**
```json
{
  "api_key": "6wt6GPkomz-xI_3udGqiI-1QkUmLcvspm2WmxM-ECsec0EPj",
  "user_id": "test-user-1",
  "secret_name": "apikey-test-user-1-948b05b1"
}
```

### Delete API Key

**External curl:**
```bash
curl -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H 'Authorization: ADMIN $ADMIN_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"key":"your-api-key-here"}'
```

**Via kubectl (with admin auth):**
```bash
kubectl exec -it deployment/key-manager -n platform-services -- \
  curl -X DELETE http://localhost:8080/delete_key \
    -H 'Authorization: ADMIN $ADMIN_KEY' \
    -H 'Content-Type: application/json' \
    -d '{"key":"your-api-key-here"}'
```

**Response:**
```json
{
  "message": "API key deleted successfully",
  "secret_name": "apikey-test-user-1-948b05b1"
}
```

### List Models

**External curl:**
```bash
curl "http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/models" \
  -H 'Authorization: ADMIN $ADMIN_KEY'
```

**Via kubectl:**
```bash
kubectl exec -it deployment/key-manager -n platform-services -- \
  curl "http://localhost:8080/v1/models" \
    -H 'Authorization: ADMIN $ADMIN_KEY'
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "qwen3-0-6b-instruct",
      "object": "model",
      "created": 1677610602,
      "owned_by": "qwen3"
    },
    {
      "id": "simulator-model",
      "object": "model",
      "created": 1677610602,
      "owned_by": "simulator"
    }
  ]
}
```

## User ID Requirements

User IDs must follow Kubernetes RFC 1123 subdomain rules:
- ✅ Valid: `test-user-1`, `alice`, `user123`
- ❌ Invalid: `test_user_1` (underscore), `Test-User` (uppercase)
- Must be 1-63 characters, lowercase letters/numbers/hyphens only

## Testing Model Access

### Step 1: Extract Your API Key
```bash
# Get the secret name
kubectl get secrets -n llm -l kuadrant.io/apikeys-by=rhcl-keys

# Extract API key (replace <hash> with actual secret name)
API_KEY=$(kubectl get secret apikey-test-user-1-<hash> -n llm -o jsonpath='{.data.api_key}' | base64 -d)
echo $API_KEY
```

### Step 2: Test Remote Access

**Simulator Model (Lightweight):**
```bash
curl -H "Authorization: APIKEY YOUR_API_KEY_HERE" \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello"}],"max_tokens":5}' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions
```

**Qwen3 Model (GPU Required):**
```bash
curl -H "Authorization: APIKEY YOUR_API_KEY_HERE" \
     -H 'Content-Type: application/json' \
     -d '{"model":"qwen3-0-6b-instruct","messages":[{"role":"user","content":"Explain AI"}],"max_tokens":50}' \
     http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions
```

**Expected Response:**
```json
{
  "id": "chatcmpl-1755756773",
  "object": "chat.completion",
  "model": "simulator-model",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "This is a simulated response to: Hello"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
}
```

### Step 3: Complete End-to-End Test

```bash
# 1. Generate API key
API_RESPONSE=$(curl -s -X POST http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/generate_key \
  -H 'Authorization: ADMIN $ADMIN_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"end-to-end-test"}')

echo "API Key Generation Response:"
echo $API_RESPONSE

# 2. Extract the API key
API_KEY=$(echo $API_RESPONSE | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "Extracted API Key: $API_KEY"

# 3. Test model access with the API key (both formats)
echo "Testing model access with APIKEY format:"
curl -H "Authorization: APIKEY $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello world!"}],"max_tokens":25}' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions

echo -e "\nTesting model access with Bearer format:"
curl -H "Authorization: Bearer $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"simulator-model","messages":[{"role":"user","content":"Hello Bearer!"}],"max_tokens":25}' \
     http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions

# 4. Clean up - delete the API key
echo -e "\nCleaning up:"
curl -X DELETE http://key-manager.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H 'Authorization: ADMIN $ADMIN_KEY' \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$API_KEY\"}"
```

**Output:**

```bash
# API Key Generation Response:
{"api_key":"SzCAEQTBuKpZ7TDVGPoHNtIKF_YRy7Uc5yG_MNE-xTrScjw4","user_id":"end-to-end-test","secret_name":"apikey-end-to-end-test-9240eea6"}

Extracted API Key: SzCAEQTBuKpZ7TDVGPoHNtIKF_YRy7Uc5yG_MNE-xTrScjw4

Testing model access:
{"id": "chatcmpl-1755759495", "object": "chat.completion", "created": 1755759495, "model": "simulator-model", "choices": [{"index": 0, "message": {"role": "assistant", "content": "This is a simulated response to: Hello world!"}, "finish_reason": "stop"}], "usage": {"prompt_tokens": 15, "completion_tokens": 30, "total_tokens": 45}}

# Del Key Reponse
{"message":"API key deleted successfully","secret_name":"apikey-end-to-end-test-9240eea6"}
```

### Step 4: Test Token Rate Limiting

The manifests have a large limit atm. Make the limit in token rate limit manifet smol to validate quota violations.

```bash
# Make multiple requests to trigger rate limit
for i in {1..5}; do
  echo "Request $i:"
  curl -w "HTTP Status: %{http_code}\n" \
    -H "Authorization: APIKEY YOUR_API_KEY_HERE" \
    -H 'Content-Type: application/json' \
    -d '{"model":"simulator-model","messages":[{"role":"user","content":"Generate response"}],"max_tokens":30}' \
    http://simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions
  echo ""
done
```

**Rate Limit Response (HTTP 429):**
```json
{"error": "Token rate limit exceeded"}
```

## Validation

Run an endpoint validation:

```bash
./validate-key-manager-endpoints.sh
```

This tests:
- Health check
- API key generation/deletion
- Secret management
- Model authentication
- User ID validation
- Token rate limiting

## Management

```bash
# List all API keys
kubectl get secrets -n llm -l kuadrant.io/apikeys-by=rhcl-keys

# View service logs
kubectl logs -n platform-services deployment/key-manager

# Check policies
kubectl get authpolicy,tokenratelimitpolicy -n llm
```

## Token Rate Limiting

- **Configurable limits**: Token consumption limits per user
- **Enforcement**: Automatic via Kuadrant TokenRateLimitPolicy
- **Monitoring**: Prometheus metrics via Istio gateway

## Traffic Flow

### Admin Operations (Key Management)
```
External Request
  ↓ (ADMIN your-configured-admin-key)
OpenShift Router (*.apps domain)
  ↓
Istio Gateway (inference-gateway)
  ↓ (AuthPolicy override: anonymous)
HTTPRoute (key-manager-domain-route)
  ↓ (ReferenceGrant: cross-namespace)
Key-Manager Service (platform-services namespace)
  ↓ (Admin middleware validation)
Key-Manager Pod (Go application)
```

**Admin Endpoints:**
- **Generate API Key**: `POST /generate_key` - Creates new user API keys
- **Delete API Key**: `DELETE /delete_key` - Removes user API keys
- **Health Check**: `GET /health` - Service health status

**Admin Authentication**: `Authorization: ADMIN your-configured-admin-key`

### User Operations (Model Access)
```
External Request
  ↓ (APIKEY user-generated-key)
OpenShift Router (*.apps domain)
  ↓
Istio Gateway (inference-gateway)
  ↓ (AuthPolicy: API key validation)
HTTPRoute (model-specific routes)
  ↓
Model Service (qwen3, simulator, etc.)
  ↓ (Token rate limiting enforced)
Model Pod (VLLM/inference service)
```

**User Endpoints:**
- **Model Inference**: `POST /v1/chat/completions` - LLM model requests
- **Discover Endpoint**: `GET /discover_endpoint?user_id=<user>` - Find available models

**User Authentication**: `Authorization: APIKEY <user-api-key>` or `Authorization: Bearer <user-api-key>`

## 🔑 Authentication Formats

**For Admin Operations (Key Management):**
```bash
-H 'Authorization: ADMIN your-configured-admin-key'
```

**For User Operations (Model Access) - Both formats supported:**
```bash
# APIKEY format (custom)
-H 'Authorization: APIKEY your-generated-api-key'

# Bearer format (OpenAI-compatible)
-H 'Authorization: Bearer your-generated-api-key'
```

### Components

1. **OpenShift Route**: Handles `*.apps` domain traffic to Istio gateway
2. **HTTPRoute**: Routes traffic within Istio mesh based on hostname
3. **AuthPolicy**:
   - Gateway-level: Validates API keys for model access
   - Override: Allows key-manager to handle own authentication
4. **ReferenceGrant**: Permits cross-namespace service references
5. **TokenRateLimitPolicy**: Enforces token-based rate limiting per user

## Sec

- Non-root container execution
- RBAC-limited to secret management in `llm` namespace only
- API keys stored as Kubernetes Secrets with SHA256 hashing
- Validation prevents malformed user IDs
