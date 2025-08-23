# MaaS Key Manager v2 API Implementation Plan

## Overview

This document outlines the implementation plan for the v2 MaaS Key Manager API, designed to provide stateless, Kubernetes-native team and key management using label-based groupings and Kuadrant integration.

## Architecture Principles

### 1. Stateless Design
- No persistent database required
- All state stored in Kubernetes secrets with labels and annotations
- Teams exist as label-based groupings of API keys
- Discovery through label selectors

### 2. Kubernetes-Native Patterns
- Secrets store API keys with rich metadata
- Labels enable efficient filtering and grouping
- Annotations store non-indexable metadata (budgets, usage)
- RBAC controls access to secret management

### 3. Service Organization
All endpoints implemented in single Go service with logical separation:
- **Core Key Manager**: API key lifecycle, team key management, usage tracking
- **External Service Logic**: User authentication, team membership management (marked with comments)
- **Admin Logic**: Model deployment, cluster configuration (marked with comments)

## Team Management Model

### Team Storage Pattern
Teams are implicit collections of API keys with shared labels:

```yaml
# Example: API key belonging to "data-science" team
apiVersion: v1
kind: Secret
metadata:
  name: apikey-alice-datascience-1a2b3c
  namespace: llm
  labels:
    kuadrant.io/apikeys-by: rhcl-keys
    maas/user-id: "alice"
    maas/team-id: "data-science"
    maas/team-role: "member"  # member, admin, viewer
  annotations:
    maas/team-name: "Data Science Team"
    maas/user-email: "alice@company.com"
    maas/budget-monthly: "1000.00"
    maas/spend-current: "150.50"
    maas/models-allowed: "qwen3-0-6b-instruct,simulator"
    maas/created-at: "2024-01-15T10:30:00Z"
    maas/last-used: "2024-01-20T14:22:00Z"
data:
  api_key: <base64-encoded-key>
```

### Team Discovery
- `GET /users/me` → Extract user from OIDC token
- `GET /teams` → List unique `maas/team-id` label values
- `GET /teams/{id}` → Aggregate data from secrets with matching team label
- No separate team storage required

## API Endpoints Implementation

### Core Key Manager Endpoints

#### 1. `POST /teams/{team_id}/keys`
**Purpose**: Create API key scoped to team with budget and model access  
**Request**:
```json
{
  "user_id": "alice",
  "user_email": "alice@company.com",
  "alias": "alice-research-key",
  "models": ["qwen3-0-6b-instruct"],
  "budget_usd": 500.00,
  "role": "member"
}
```
**Implementation**:
- Generate secure API key
- Create secret with team labels and budget annotations
- Update Kuadrant policies if needed
- Return key details

#### 2. `GET /teams/{team_id}/keys`
**Purpose**: List all API keys for a team  
**Implementation**:
- Label selector: `maas/team-id={team_id}`
- Return aggregated key metadata
- Include usage/budget information from annotations

#### 3. `PATCH /keys/{key_id}`
**Purpose**: Update API key properties (budget, status, models)  
**Request**:
```json
{
  "budget_usd": 750.00,
  "status": "active",
  "models": ["qwen3-0-6b-instruct", "simulator"]
}
```
**Implementation**:
- Find secret by key value or metadata
- Update annotations and labels
- Sync changes to Kuadrant policies

#### 4. `DELETE /keys/{key_id}`
**Purpose**: Remove API key and update policies  
**Implementation**:
- Find and delete secret
- Update Kuadrant AuthPolicy if needed
- Return confirmation

#### 5. `GET /models`
**Purpose**: Discover available models from HTTPRoutes  
**Implementation**:
- Query HTTPRoutes in llm namespace
- Extract model endpoints and metadata
- Return user-facing model catalog

#### 6. `GET /teams/{team_id}/activity`
**Purpose**: Usage and cost reporting for team  
**Query Parameters**: `?start_date=2024-01-01&end_date=2024-01-31&group_by=user`  
**Implementation**:
- Aggregate usage from secret annotations
- Calculate costs based on token consumption
- Group by user, model, or time period

### External Service Logic Endpoints (in same Go app)

#### 7. `GET /users/me`
**Purpose**: Retrieves current user profile from OIDC token  
**Response**:
```json
{
  "user_id": "alice",
  "email": "alice@company.com",
  "teams": ["data-science", "research"]
}
```
**Implementation**:
```go
// EXTERNAL SERVICE: This would typically be handled by an OIDC/auth service
// For now, implemented as stub that validates JWT token and extracts user info
func (s *Server) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
    // Extract JWT from Authorization header
    // Validate token with OIDC provider
    // Return user profile information
}
```

#### 8. `POST /teams`
**Purpose**: Create new team  
**Request**:
```json
{
  "team_name": "Machine Learning",
  "description": "ML research and development team"
}
```
**Implementation**:
```go
// EXTERNAL SERVICE: Team creation logic
// Creates team metadata and initial policies
func (s *Server) CreateTeam(w http.ResponseWriter, r *http.Request) {
    // Validate team name uniqueness
    // Create team configuration
    // Set up initial Kuadrant policies for team
}
```

#### 9. `GET /teams/{id}`
**Purpose**: Retrieve team details including members  
**Response**:
```json
{
  "team_id": "data-science",
  "team_name": "Data Science Team",
  "description": "Advanced analytics and ML",
  "members": [
    {"user_id": "alice", "role": "admin", "email": "alice@company.com"},
    {"user_id": "bob", "role": "member", "email": "bob@company.com"}
  ],
  "keys": ["key-abc123", "key-def456"]
}
```
**Implementation**:
```go
// EXTERNAL SERVICE: Team membership management
// Aggregates team data from multiple sources
func (s *Server) GetTeam(w http.ResponseWriter, r *http.Request) {
    // Get team basic info
    // List team members (from secrets or external system)
    // Get team keys via label selector
}
```

#### 10. `POST /teams/{id}/members`
**Purpose**: Add team member  
**Request**:
```json
{
  "user_email": "charlie@company.com",
  "role": "member"
}
```
**Implementation**:
```go
// EXTERNAL SERVICE: Team membership management
// Handles user invitation and role assignment
func (s *Server) AddTeamMember(w http.ResponseWriter, r *http.Request) {
    // Validate user exists in OIDC
    // Add user to team (update team metadata)
    // Send invitation if needed
}
```

#### 11. `DELETE /teams/{id}/members/{user}`
**Purpose**: Remove team member  
**Implementation**:
```go
// EXTERNAL SERVICE: Team membership management
// Removes user from team and revokes team-based access
func (s *Server) RemoveTeamMember(w http.ResponseWriter, r *http.Request) {
    // Remove user from team metadata
    // Optionally revoke/disable user's team API keys
    // Update team policies
}
```

### Admin Logic Endpoints (in same Go app)

#### 12. `POST /admin/models`
**Purpose**: Add new model to the platform  
**Request**:
```json
{
  "model_name": "llama3-8b-instruct",
  "description": "Llama 3 8B instruction-tuned model",
  "pricing": {
    "input_cost_per_1k_tokens": 0.003,
    "output_cost_per_1k_tokens": 0.006
  },
  "kserve_config": {
    "runtime": "vllm-latest",
    "storage_uri": "hf://meta-llama/Llama-3-8B-Instruct",
    "gpu_required": true
  }
}
```
**Implementation**:
```go
// ADMIN SERVICE: Model deployment and configuration
// Creates KServe InferenceService and associated routing
func (s *Server) CreateModel(w http.ResponseWriter, r *http.Request) {
    // Validate model configuration
    // Create KServe InferenceService
    // Create HTTPRoute for model access
    // Update model registry/pricing config
}
```

#### 13. `PATCH /admin/models/{id}`
**Purpose**: Update model configuration  
**Request**:
```json
{
  "pricing": {
    "input_cost_per_1k_tokens": 0.0025,
    "output_cost_per_1k_tokens": 0.005
  },
  "status": "active"
}
```
**Implementation**:
```go
// ADMIN SERVICE: Model configuration management
// Updates model pricing, routing, or deployment parameters
func (s *Server) UpdateModel(w http.ResponseWriter, r *http.Request) {
    // Update model configuration
    // Modify KServe InferenceService if needed
    // Update pricing in ConfigMap
}
```

## Kuadrant Integration

### AuthPolicy Integration
```yaml
# Existing AuthPolicy continues to work
apiVersion: kuadrant.io/v1
kind: AuthPolicy
metadata:
  name: gateway-auth-policy
  namespace: llm
spec:
  rules:
    authentication:
      api-key-users:
        apiKey:
          allNamespaces: true
          selector:
            matchLabels:
              kuadrant.io/apikeys-by: rhcl-keys
```

### TokenRateLimitPolicy Enhancement
```yaml
# Team-based rate limiting
apiVersion: kuadrant.io/v1alpha1
kind: TokenRateLimitPolicy
metadata:
  name: team-token-limits
  namespace: llm
spec:
  limits:
    team-data-science:
      rates:
        - limit: 100000
          window: 1h
      when:
        - selector: auth.identity.metadata.labels.maas/team-id
          operator: eq
          value: "data-science"
    team-engineering:
      rates:
        - limit: 50000
          window: 1h
      when:
        - selector: auth.identity.metadata.labels.maas/team-id
          operator: eq
          value: "engineering"
```

## Implementation Steps

### Phase 1: Core Key Management
1. **Implement core endpoints**:
   - `POST /teams/{team_id}/keys`
   - `GET /teams/{team_id}/keys`
   - `PATCH /keys/{key_id}`
   - `DELETE /keys/{key_id}`

2. **Enhance secret management**:
   - Add team labels to secret creation
   - Implement label-based queries
   - Add budget/usage annotations

### Phase 2: External Service Logic
1. **User management stubs**:
   - `GET /users/me` (JWT validation)
   - `POST /teams` (team creation)
   - `GET /teams/{id}` (team details)
   - Team membership endpoints

### Phase 3: Discovery and Admin
1. **Model discovery**:
   - `GET /models` from HTTPRoute discovery
   - Cache model metadata

2. **Admin endpoints**:
   - `POST /admin/models`
   - `PATCH /admin/models/{id}`

3. **Usage reporting**:
   - `GET /teams/{team_id}/activity`
   - Aggregate usage from annotations

## Code Organization

```go
// Server structure with logical separation
type Server struct {
    // Core key management
    keyManager     *KeyManager
    secretClient   SecretClient
    
    // External service logic (marked clearly)
    authProvider   AuthProvider    // EXTERNAL SERVICE
    teamManager    TeamManager     // EXTERNAL SERVICE
    
    // Admin logic (marked clearly)  
    modelManager   ModelManager    // ADMIN SERVICE
    kserveClient   KServeClient    // ADMIN SERVICE
}

// Route registration with clear separation
func (s *Server) RegisterRoutes() {
    // Core Key Manager endpoints
    s.router.POST("/teams/:team_id/keys", s.CreateTeamKey)
    s.router.GET("/teams/:team_id/keys", s.ListTeamKeys)
    s.router.PATCH("/keys/:key_id", s.UpdateKey)
    s.router.DELETE("/keys/:key_id", s.DeleteKey)
    s.router.GET("/models", s.ListModels)
    s.router.GET("/teams/:team_id/activity", s.GetTeamActivity)
    
    // EXTERNAL SERVICE endpoints (implemented here for now)
    s.router.GET("/users/me", s.GetCurrentUser)
    s.router.POST("/teams", s.CreateTeam)
    s.router.GET("/teams/:id", s.GetTeam)
    s.router.POST("/teams/:id/members", s.AddTeamMember)
    s.router.DELETE("/teams/:id/members/:user", s.RemoveTeamMember)
    
    // ADMIN SERVICE endpoints (implemented here for now)
    s.router.POST("/admin/models", s.CreateModel)
    s.router.PATCH("/admin/models/:id", s.UpdateModel)
}
```

## Migration from Current API

### Backward Compatibility
- Keep existing `/generate_key` endpoint for transition
- Map current keys to team structure using default team
- Gradual migration of clients to new endpoints

### Data Migration
```bash
# Add team labels to existing secrets
kubectl label secrets -n llm -l kuadrant.io/apikeys-by=rhcl-keys maas/team-id=default
kubectl annotate secrets -n llm -l kuadrant.io/apikeys-by=rhcl-keys maas/team-name="Default Team"
```

## Next Steps

1. **Start with Phase 1 implementation**
2. **Create handler stubs for all endpoints**
3. **Implement secret label patterns**
4. **Test with sample team scenarios**
5. **Add clear separation comments in code**