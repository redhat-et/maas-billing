# MaaS Key Manager API Specification

## Complete API Endpoint Reference

| Endpoint                           | Method | Purpose                          | Auth Required | Request Body | Response |
| ---------------------------------- | ------ | -------------------------------- | ------------- | ------------ | -------- |
| `/health`                          | GET    | Service health check             | None          | None         | Health status |
| `/generate_key`                    | POST   | Legacy API key generation        | Admin         | `{"user_id": "string"}` | API key details |
| `/delete_key`                      | DELETE | Legacy API key deletion          | Admin         | `{"key": "string"}` | Success confirmation |
| `/models`                          | GET    | List available AI models         | Admin         | None         | OpenAI-compatible models list |
| `/teams`                           | POST   | Create new team                  | Admin         | Team config  | Team details with policies |
| `/teams`                           | GET    | List all teams                   | Admin         | None         | Array of team summaries |
| `/teams/{team_id}`                 | GET    | Get team details                 | Admin         | None         | Complete team info |
| `/teams/{team_id}`                 | DELETE | Delete team and resources        | Admin         | None         | Success confirmation |
| `/teams/{team_id}/members`         | POST   | Add user to team                 | Admin         | User details | User membership info |
| `/teams/{team_id}/members`         | GET    | List team members                | Admin         | None         | Array of team members |
| `/teams/{team_id}/members/{user}`  | DELETE | Remove user from team            | Admin         | None         | Success confirmation |
| `/teams/{team_id}/keys`            | POST   | Create team-scoped API key       | Admin         | Key config   | API key with team context |
| `/teams/{team_id}/keys`            | GET    | List team API keys               | Admin         | None         | Array of team API keys |
| `/keys/{key_name}`                 | PATCH  | Update API key properties        | Admin         | Key updates  | Updated key details |
| `/keys/{key_name}`                 | DELETE | Delete API key                   | Admin         | None         | Success confirmation |
| `/teams/{team_id}/policies`        | GET    | View team policies               | Admin         | None         | Active policy details |
| `/teams/{team_id}/policies/sync`   | POST   | Sync team policies               | Admin         | Optional config | Sync status |
| `/admin/policies/health`           | GET    | Policy system health             | Admin         | None         | Policy system status |
| `/admin/policies/compliance`       | GET    | Policy compliance report         | Admin         | None         | Compliance summary |
| `/admin/policies/defaults`         | GET    | Default policy templates         | Admin         | None         | Tier-based templates |
| `/admin/policies/tiers/{tier}`     | PUT    | Update tier policy               | Admin         | Tier config  | Updated tier info |
| `/admin/policies/tiers`            | POST   | Create tier policy               | Admin         | Tier config  | New tier details |
| `/teams/{team_id}/activity`        | GET    | Team activity and spending       | Admin         | Query params | Usage analytics |
| `/teams/{team_id}/usage`           | GET    | Team usage breakdown             | Admin         | Query params | Detailed usage stats |

## Architecture Overview

The MaaS Key Manager implements a stateless, Kubernetes-native team management system using:

- **Teams**: Kubernetes Secrets with team metadata and policy references
- **Team Membership**: Kubernetes Secrets linking users to teams with roles and permissions
- **Team Policies**: Kuadrant TokenRateLimitPolicy and RateLimitPolicy CRDs that target team members
- **API Keys**: Kubernetes Secrets with team/user labels that inherit team policies automatically

### Label-Based Association Strategy

All resources use standardized labels for stateless discovery and policy targeting:

#### Team Resources
```yaml
labels:
  maas/resource-type: "team"
  maas/team-id: "data-science"
  maas/tier: "premium"
```

#### Team Membership Resources
```yaml
labels:
  maas/resource-type: "team-membership"
  maas/team-id: "data-science"
  maas/user-id: "alice"
  maas/role: "member"
```

#### Team Policy Resources
```yaml
labels:
  maas/resource-type: "team-policy"
  maas/team-id: "data-science"
  maas/policy-type: "token-rate-limit"
```

#### API Key Resources
```yaml
labels:
  maas/resource-type: "api-key"
  maas/team-id: "data-science"
  maas/user-id: "alice"
  kuadrant.io/apikeys-by: "rhcl-keys"
```

### Policy Enforcement Flow

1. **API Key Creation**: Gets team labels from user's team membership
2. **Kuadrant Discovery**: AuthPolicy finds API keys via `kuadrant.io/apikeys-by=rhcl-keys`
3. **Policy Targeting**: TokenRateLimitPolicy/RateLimitPolicy target based on team labels
4. **Request Enforcement**: Policies evaluate against API key metadata during requests

## API Endpoints

### Core Service Endpoints

#### `GET /health`
**Purpose**: Service health check  
**Authentication**: None  
**Response**: Service status and readiness

#### `GET /models`
**Purpose**: List available AI models  
**Authentication**: Admin API key  
**Response**: OpenAI-compatible models list with pricing and availability

---

### Team Management

#### `POST /teams`
**Purpose**: Create new team with policies  
**Authentication**: Admin API key  
**Request Body**:
```json
{
  "team_id": "data-science",
  "team_name": "Data Science Team",
  "description": "ML research and analytics team",
  "tier": "premium",
  "token_limit": 100000,
  "request_limit": 1000,
  "time_window": "1h"
}
```

**Implementation**:
1. Creates team secret with labels `maas/resource-type=team`, `maas/team-id={id}`
2. Generates team TokenRateLimitPolicy with selector targeting team members
3. Generates team RateLimitPolicy if request limits specified
4. Policies target using: `auth.identity.metadata.labels['maas/team-id'] == 'data-science'`

**Response**: Team details with inherited policy limits

#### `GET /teams`
**Purpose**: List all teams  
**Authentication**: Admin API key  
**Implementation**: Query secrets with label `maas/resource-type=team`  
**Response**: Array of team summaries with member counts and policy status

#### `GET /teams/{team_id}`
**Purpose**: Get team details including members and API keys  
**Authentication**: Admin API key  
**Implementation**:
1. Get team secret by name `team-{team_id}-config`
2. Query team memberships: `maas/resource-type=team-membership,maas/team-id={team_id}`
3. Query team API keys: `maas/resource-type=api-key,maas/team-id={team_id}`
4. Query team policies: `maas/team-id={team_id}` on TokenRateLimitPolicy/RateLimitPolicy CRDs

**Response**: Complete team details with members, keys, and active policies

#### `DELETE /teams/{team_id}`
**Purpose**: Delete team and all associated resources  
**Authentication**: Admin API key  
**Implementation**:
1. Delete team policies: TokenRateLimitPolicy and RateLimitPolicy CRDs
2. Delete team memberships: All secrets with `maas/team-id={team_id},maas/resource-type=team-membership`
3. Delete team API keys: All secrets with `maas/team-id={team_id},maas/resource-type=api-key`
4. Delete team configuration secret

---

### Team Membership Management

#### `POST /teams/{team_id}/members`
**Purpose**: Add user to team  
**Authentication**: Admin API key  
**Request Body**:
```json
{
  "user_id": "alice",
  "user_email": "alice@company.com",
  "role": "member",
  "token_limit": 50000,
  "request_limit": 500,
  "time_window": "1h"
}
```

**Implementation**:
1. Validate team exists
2. Create team membership secret with labels:
   - `maas/resource-type=team-membership`
   - `maas/team-id={team_id}`
   - `maas/user-id={user_id}`
   - `maas/role={role}`
3. Store user limits in secret annotations
4. User can now create API keys that inherit team policies

#### `GET /teams/{team_id}/members`
**Purpose**: List team members  
**Authentication**: Admin API key  
**Implementation**: Query secrets with labels `maas/resource-type=team-membership,maas/team-id={team_id}`  
**Response**: Array of team members with roles and individual limits

#### `DELETE /teams/{team_id}/members/{user_id}`
**Purpose**: Remove user from team  
**Authentication**: Admin API key  
**Implementation**:
1. Delete team membership secret
2. Optionally orphan or delete user's team API keys
3. User loses access to team policies

---

### API Key Management

#### `POST /teams/{team_id}/keys`
**Purpose**: Create team-scoped API key  
**Authentication**: Admin API key  
**Request Body**:
```json
{
  "user_id": "alice",
  "alias": "production-key",
  "models": ["qwen3-0-6b-instruct"],
  "inherit_team_limits": true,
  "token_limit": 25000,
  "request_limit": 250
}
```

**Implementation**:
1. Validate user is team member (check team membership secret exists)
2. Generate secure API key
3. Create API key secret with labels:
   - `maas/resource-type=api-key`
   - `maas/team-id={team_id}`
   - `maas/user-id={user_id}`
   - `kuadrant.io/apikeys-by=rhcl-keys`
4. Team policies automatically target this key via label selectors

#### `GET /teams/{team_id}/keys`
**Purpose**: List team API keys  
**Authentication**: Admin API key  
**Implementation**: Query secrets with labels `maas/resource-type=api-key,maas/team-id={team_id}`  
**Response**: Array of API keys with usage metadata and associated users

#### `PATCH /keys/{key_name}`
**Purpose**: Update API key properties  
**Authentication**: Admin API key  
**Request Body**:
```json
{
  "status": "active",
  "token_limit": 30000,
  "models": ["qwen3-0-6b-instruct", "simulator-model"]
}
```

**Implementation**: Update API key secret annotations and labels

#### `DELETE /keys/{key_name}`
**Purpose**: Delete specific API key  
**Authentication**: Admin API key  
**Implementation**: Delete API key secret by name

---

### Policy Management

#### `GET /teams/{team_id}/policies`
**Purpose**: View active team policies  
**Authentication**: Admin API key  
**Implementation**:
1. Query TokenRateLimitPolicy CRDs with label `maas/team-id={team_id}`
2. Query RateLimitPolicy CRDs with label `maas/team-id={team_id}`
3. Show policy configurations and targeting selectors

**Response**: Policy details with limits, windows, and enforcement status

#### `POST /teams/{team_id}/policies/sync`
**Purpose**: Synchronize team policies with latest configuration  
**Authentication**: Admin API key  
**Implementation**:
1. Get current team configuration from team secret
2. Update or recreate TokenRateLimitPolicy and RateLimitPolicy CRDs
3. Ensure policy selectors correctly target team members

#### `GET /admin/policies/health`
**Purpose**: Policy system health check  
**Authentication**: Admin API key  
**Implementation**:
1. Check Kuadrant operator availability
2. Validate policy CRD status across all teams
3. Report policy enforcement statistics

#### `GET /admin/policies/compliance`
**Purpose**: Team policy compliance report  
**Authentication**: Admin API key  
**Implementation**:
1. Audit all teams for proper policy associations
2. Check for orphaned API keys without team policies
3. Validate policy targeting selectors

#### `GET /admin/policies/defaults`
**Purpose**: Default policy templates by tier  
**Authentication**: Admin API key  
**Response**: Tier-based policy templates (free, standard, premium, unlimited)

#### `PUT /admin/policies/tiers/{tier}`
**Purpose**: Update tier policy template  
**Authentication**: Admin API key  
**Implementation**: Update ConfigMap with tier policy definitions

#### `POST /admin/policies/tiers`
**Purpose**: Create new tier policy template  
**Authentication**: Admin API key  
**Implementation**: Add new tier to ConfigMap policy definitions

---

### Usage Analytics

#### `GET /teams/{team_id}/activity`
**Purpose**: Team activity and spending summary  
**Authentication**: Admin API key  
**Query Parameters**: `?start_date=2024-01-01&end_date=2024-01-31&group_by=user`  
**Implementation**:
1. Aggregate usage from API key secret annotations
2. Calculate costs based on token consumption
3. Group by user, model, or time period

#### `GET /teams/{team_id}/usage`
**Purpose**: Detailed team usage breakdown  
**Authentication**: Admin API key  
**Implementation**:
1. Get team members from membership secrets
2. Get API key usage from secret annotations
3. Calculate per-user and per-model consumption

---

### Legacy Endpoints (Backward Compatibility)

#### `POST /generate_key`
**Purpose**: Generate API key using default team  
**Authentication**: Admin API key  
**Implementation**: Creates API key in "default" team with standard tier limits

#### `DELETE /delete_key`
**Purpose**: Delete API key by key value  
**Authentication**: Admin API key  
**Implementation**: Find and delete API key secret by SHA256 hash label

---

## Kubernetes Resource Patterns

### Team Secret Structure
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: team-data-science-config
  namespace: llm
  labels:
    maas/resource-type: "team"
    maas/team-id: "data-science"
    maas/tier: "premium"
  annotations:
    maas/team-name: "Data Science Team"
    maas/description: "ML research and analytics"
    maas/token-limit: "100000"
    maas/request-limit: "1000"
    maas/time-window: "1h"
    maas/created-at: "2024-01-15T10:30:00Z"
```

### Team Membership Secret Structure
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: membership-alice-data-science
  namespace: llm
  labels:
    maas/resource-type: "team-membership"
    maas/team-id: "data-science"
    maas/user-id: "alice"
    maas/role: "member"
  annotations:
    maas/user-email: "alice@company.com"
    maas/token-limit: "50000"
    maas/request-limit: "500"
    maas/time-window: "1h"
    maas/joined-at: "2024-01-15T14:22:00Z"
```

### API Key Secret Structure
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: apikey-alice-data-science-a1b2c3d4
  namespace: llm
  labels:
    # Required for resource identification and querying
    maas/resource-type: "api-key"
    maas/team-id: "data-science"
    maas/user-id: "alice"
    maas/role: "member"
    maas/tier: "premium"
    
    # Required for Kuadrant AuthPolicy discovery
    kuadrant.io/apikeys-by: "rhcl-keys"
    authorino.kuadrant.io/managed-by: "authorino"
    
    # Used for policy targeting and inheritance
    maas/key-sha256: "a1b2c3d4e5f6..."  # Truncated SHA256 for key lookup
    maas/status: "active"
    
  annotations:
    # Human-readable metadata
    maas/alias: "production-key"
    maas/user-email: "alice@company.com"
    maas/team-name: "Data Science Team"
    
    # Model access configuration
    maas/models-allowed: "qwen3-0-6b-instruct,simulator-model"
    maas/models-denied: ""
    
    # Rate limiting configuration (overrides team defaults)
    maas/token-limit: "50000"
    maas/request-limit: "500"
    maas/time-window: "1h"
    maas/burst-limit: "100"
    maas/max-concurrent-requests: "10"
    
    # Usage tracking and billing
    maas/budget-monthly: "1000.00"
    maas/spend-current: "245.67"
    maas/spend-last-reset: "2024-01-01T00:00:00Z"
    maas/tokens-consumed-total: "1234567"
    maas/requests-made-total: "8901"
    
    # Lifecycle management
    maas/created-at: "2024-01-15T15:45:00Z"
    maas/created-by: "admin-service"
    maas/last-used: "2024-01-20T09:15:00Z"
    maas/expires-at: "2024-12-31T23:59:59Z"
    maas/auto-rotate: "false"
    
    # Custom constraints (JSON-encoded)
    maas/custom-limits: '{"special_model_access": true, "priority": "high"}'
    
    # Audit and compliance
    maas/purpose: "Production ML inference for customer-facing application"
    maas/compliance-tags: "pci-dss,gdpr"
    maas/data-classification: "internal"

type: Opaque
data:
  # The actual API key (base64-encoded)
  api_key: "QkhZOEJ4cUsyUWU5YnZnUGJEU1hlYzR0SEVhQVd1b3d4X2FfVU5wMkZEdFRSdlhF"
  
  # Optional: Store key metadata in data section
  key_metadata: "eyJ2ZXJzaW9uIjoidjIiLCJhbGdvcml0aG0iOiJIUzI1NiJ9"
```

### API Key Value Format

**Generated API Key Structure:**
```
BK08BxqK2Qe9bvgPbDSXec4tHEaAWuowx_a_UNp2FDtTRvXE
│                                                    │
├── Base64URL-encoded random bytes (48 characters)  │
└── Cryptographically secure, URL-safe             ─┘
```

**Key Properties:**
- **Length**: 48 characters (36 random bytes base64url-encoded)
- **Character Set**: `A-Z`, `a-z`, `0-9`, `-`, `_` (URL-safe base64)
- **Entropy**: 288 bits (cryptographically secure)
- **Collision Probability**: Negligible (2^-144 for birthday attack)
- **Format**: No special prefixes or versioning (for simplicity)

**Key Generation Process:**
1. Generate 36 random bytes using `crypto/rand`
2. Encode using base64.URLEncoding
3. Truncate to 48 characters
4. Create SHA256 hash for secret lookup label
5. Store full key in secret data, hash in labels

**Usage in Requests:**
```bash
# Bearer token format (standard)
curl -H "Authorization: Bearer BK08BxqK2Qe9bvgPbDSXec4tHEaAWuowx_a_UNp2FDtTRvXE" \
     https://api.example.com/v1/chat/completions

# Custom APIKEY format (alternative)
curl -H "Authorization: APIKEY BK08BxqK2Qe9bvgPbDSXec4tHEaAWuowx_a_UNp2FDtTRvXE" \
     https://api.example.com/v1/chat/completions
```

### Team TokenRateLimitPolicy Structure
```yaml
apiVersion: kuadrant.io/v1alpha1
kind: TokenRateLimitPolicy
metadata:
  name: team-data-science-token-limits
  namespace: llm
  labels:
    maas/resource-type: "team-policy"
    maas/team-id: "data-science"
    maas/policy-type: "token-rate-limit"
    maas/tier: "premium"
  annotations:
    maas/team-name: "Data Science Team"
    maas/created-by: "key-manager-service"
    maas/policy-version: "v1"
    maas/last-updated: "2024-01-15T10:30:00Z"
spec:
  # Target the gateway where inference services are exposed
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: inference-gateway
    namespace: llm
  
  limits:
    # Team-level token consumption limits
    team-token-limit:
      rates:
        - limit: 100000      # 100k tokens per hour for entire team
          window: 1h
        - limit: 2000000     # 2M tokens per day for entire team  
          window: 24h
        - limit: 50000000    # 50M tokens per month for entire team
          window: 720h
      
      # Target all API keys belonging to this team
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
    
    # Per-user limits within the team (optional)
    individual-user-limit:
      rates:
        - limit: 25000       # 25k tokens per hour per user
          window: 1h
        - limit: 500000      # 500k tokens per day per user
          window: 24h
      
      # Apply to each user individually
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
      
      # Group by user to enforce per-user limits
      counters:
        - auth.identity.metadata.labels['maas/user-id']
    
    # Role-based limits (admins get higher limits)
    admin-user-limit:
      rates:
        - limit: 50000       # 50k tokens per hour for admin users
          window: 1h
      
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
        - selector: "auth.identity.metadata.labels['maas/role']"
          operator: "eq"
          value: "admin"
```

### Team RateLimitPolicy Structure
```yaml
apiVersion: kuadrant.io/v1beta2
kind: RateLimitPolicy
metadata:
  name: team-data-science-request-limits
  namespace: llm
  labels:
    maas/resource-type: "team-policy"
    maas/team-id: "data-science"
    maas/policy-type: "request-rate-limit"
    maas/tier: "premium"
  annotations:
    maas/team-name: "Data Science Team"
    maas/created-by: "key-manager-service"
    maas/policy-version: "v1"
    maas/last-updated: "2024-01-15T10:30:00Z"
spec:
  # Target the gateway where inference services are exposed
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: inference-gateway
    namespace: llm
  
  limits:
    # Team-level HTTP request limits
    team-request-limit:
      rates:
        - limit: 1000        # 1000 requests per hour for entire team
          window: 1h
        - limit: 20000       # 20k requests per day for entire team
          window: 24h
      
      # Target all API keys belonging to this team
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
    
    # Per-user request limits within the team
    individual-request-limit:
      rates:
        - limit: 250         # 250 requests per hour per user
          window: 1h
        - limit: 5000        # 5k requests per day per user
          window: 24h
      
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
      
      # Group by user to enforce per-user limits
      counters:
        - auth.identity.metadata.labels['maas/user-id']
    
    # Burst protection (short-term rate limiting)
    burst-protection:
      rates:
        - limit: 100         # Max 100 requests per minute (burst protection)
          window: 1m
      
      when:
        - selector: "auth.identity.metadata.labels['maas/team-id']"
          operator: "eq"
          value: "data-science"
      
      # Group by API key for individual burst limits
      counters:
        - auth.identity.metadata.labels['maas/key-sha256']
```

## Policy Application and Enforcement Flow

### 1. **Policy Creation Workflow**

**When a team is created via `POST /teams`:**
1. **Key Manager** creates team configuration secret
2. **Key Manager** generates TokenRateLimitPolicy CRD targeting the team
3. **Key Manager** generates RateLimitPolicy CRD targeting the team  
4. **Kuadrant Operator** watches for new policy CRDs
5. **Kuadrant** configures Envoy filters on the target gateway
6. Policies are now active and ready to enforce limits

### 2. **API Key Policy Association**

**When an API key is created via `POST /teams/{team_id}/keys`:**
1. **Key Manager** creates API key secret with team labels:
   ```yaml
   labels:
     maas/team-id: "data-science"
     maas/user-id: "alice" 
     maas/role: "member"
     kuadrant.io/apikeys-by: "rhcl-keys"
   ```
2. **Kuadrant AuthPolicy** discovers the key via `kuadrant.io/apikeys-by=rhcl-keys`
3. **TokenRateLimitPolicy** targets the key via team label selector
4. **RateLimitPolicy** targets the key via team label selector
5. Key automatically inherits team policies without restart

### 3. **Request Enforcement Flow**

**When a request arrives with an API key:**
1. **Gateway** receives request with `Authorization: Bearer <api-key>`
2. **Kuadrant AuthPolicy** validates API key against secrets
3. **AuthPolicy** extracts key metadata (team-id, user-id, role) from secret labels
4. **TokenRateLimitPolicy** evaluates `when` conditions against key metadata:
   ```yaml
   when:
     - selector: "auth.identity.metadata.labels['maas/team-id']"
       operator: "eq"
       value: "data-science"
   ```
5. **RateLimitPolicy** evaluates same `when` conditions
6. **Envoy** enforces rate limits based on policy configuration
7. Request **proceeds** if within limits, **rejected** if exceeded

### 4. **Policy Targeting Mechanisms**

**Team-Level Targeting:**
```yaml
when:
  - selector: "auth.identity.metadata.labels['maas/team-id']"
    operator: "eq"
    value: "data-science"
```
- Applies to **all users** in the team
- Enforces **collective team limits**

**User-Level Targeting:**
```yaml
when:
  - selector: "auth.identity.metadata.labels['maas/team-id']"
    operator: "eq" 
    value: "data-science"
counters:
  - auth.identity.metadata.labels['maas/user-id']
```
- Applies **per-user limits** within the team
- Each user gets **individual quota**

**Role-Based Targeting:**
```yaml
when:
  - selector: "auth.identity.metadata.labels['maas/team-id']"
    operator: "eq"
    value: "data-science"
  - selector: "auth.identity.metadata.labels['maas/role']"
    operator: "eq"
    value: "admin"
```
- Applies **different limits** based on user role
- Admins get **higher quotas** than members

**API Key-Level Targeting:**
```yaml
when:
  - selector: "auth.identity.metadata.labels['maas/team-id']"
    operator: "eq"
    value: "data-science"
counters:
  - auth.identity.metadata.labels['maas/key-sha256']
```
- Applies limits **per individual API key**
- Useful for **burst protection** and **concurrent request limits**

### 5. **Policy Priority and Precedence**

**Multiple policies can target the same API key:**

1. **Most Restrictive Wins**: If multiple policies apply, the **lowest limit** is enforced
2. **Policy Evaluation Order**: 
   - TokenRateLimitPolicy (token consumption)
   - RateLimitPolicy (HTTP requests)  
   - Both are evaluated independently

3. **Limit Hierarchy** (most to least restrictive):
   - API key-specific limits (stored in key secret annotations)
   - User-specific limits (role-based policies)
   - Team-level collective limits
   - Tier default limits

### 6. **Dynamic Policy Updates**

**When team configuration changes:**
1. **`POST /teams/{team_id}/policies/sync`** triggers policy update
2. **Key Manager** updates TokenRateLimitPolicy and RateLimitPolicy CRDs
3. **Kuadrant Operator** detects CRD changes
4. **Envoy configuration** is updated automatically
5. **New limits** take effect within seconds (no restart required)

### 7. **Policy Discovery and Debugging**

**Check which policies apply to a team:**
```bash
# List team policies
kubectl get tokenratelimitpolicy,ratelimitpolicy -l "maas/team-id=data-science" -n llm

# Check policy status
kubectl describe tokenratelimitpolicy team-data-science-token-limits -n llm

# View policy targeting
kubectl get tokenratelimitpolicy team-data-science-token-limits -o yaml
```

**Check API key policy association:**
```bash
# Find API key secret
kubectl get secrets -l "maas/team-id=data-science,maas/user-id=alice" -n llm

# Check key labels (used for policy targeting)
kubectl get secret apikey-alice-data-science-a1b2c3d4 -o yaml
```

This architecture ensures:
- **Automatic policy inheritance** when API keys are created
- **Real-time policy updates** without service restart
- **Flexible targeting** by team, user, role, or individual key
- **Stateless enforcement** using Kubernetes label selectors
- **Scalable policy management** with Kuadrant CRDs

## Policy Targeting and Inheritance

### Label Selector Targeting
Kuadrant policies target API keys using label selectors in their `when` conditions:

1. **Team-Level Targeting**: `auth.identity.metadata.labels['maas/team-id'] == 'data-science'`
2. **User-Level Targeting**: `auth.identity.metadata.labels['maas/user-id'] == 'alice'`
3. **Role-Based Targeting**: `auth.identity.metadata.labels['maas/role'] == 'admin'`

### Policy Precedence (Highest to Lowest)
1. **API Key Annotations**: Per-key custom limits in secret annotations
2. **User Membership**: Individual limits in team membership secret
3. **Team Configuration**: Team-specific limits in team secret
4. **Tier Defaults**: Tier-based limits from ConfigMap or hardcoded values

### Stateless Discovery Patterns
All associations are discoverable via Kubernetes label selectors:

```bash
# List all teams
kubectl get secrets -l "maas/resource-type=team" -n llm

# Get team members
kubectl get secrets -l "maas/resource-type=team-membership,maas/team-id=data-science" -n llm

# Get team API keys
kubectl get secrets -l "maas/resource-type=api-key,maas/team-id=data-science" -n llm

# Get team policies
kubectl get tokenratelimitpolicy,ratelimitpolicy -l "maas/team-id=data-science" -n llm
```

This architecture provides:
- **Stateless operation**: No database required, all state in Kubernetes
- **Policy isolation**: Each team gets dedicated policy instances
- **Flexible targeting**: Policies can target by team, user, or role
- **Scalable associations**: Label selectors scale with cluster size
- **Kubernetes-native**: Uses standard label/annotation patterns