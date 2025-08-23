# Team Management Workflow Visualization

## Complete Workflow Overview

```mermaid
graph TB
    subgraph "Platform Admin"
        Admin[Platform Administrator]
        DefaultPolicies[Default Policy Templates<br/>ConfigMaps]
    end
    
    subgraph "Team Management Layer"
        CreateTeam[1. Create Team]
        TeamConfig[Team Configuration<br/>Secret]
        TeamAuth[Team AuthPolicy<br/>Auto-generated]
        TeamRate[Team TokenRateLimit<br/>Auto-generated]
    end
    
    subgraph "User Management Layer"
        AddUser[2. Add User to Team]
        UserMember[User-Team Membership<br/>Secret]
        InheritPolicy[Inherit Team Policies]
    end
    
    subgraph "API Key Management Layer"
        CreateKey[3. Create Team API Key]
        KeySecret[API Key Secret<br/>Enhanced Labels]
        UpdatePolicies[Update Kuadrant Policies]
    end
    
    subgraph "Policy Application Engine"
        PolicyEngine[Policy Rule Engine]
        ApplyDefaults[Apply Default Policies]
        EventTrigger[Policy Events]
    end
    
    subgraph "OpenShift/Kuadrant Layer"
        AuthPolicy[AuthPolicy CRD]
        TokenRateLimit[TokenRateLimitPolicy CRD]
        Gateway[Istio Gateway]
        Models[Model Services]
    end
    
    subgraph "User/Application Layer"
        UserApp[User Application]
        APIRequest[API Request with Key]
        ModelResponse[Model Response]
    end
    
    %% Flow connections
    Admin --> CreateTeam
    DefaultPolicies --> CreateTeam
    CreateTeam --> TeamConfig
    CreateTeam --> PolicyEngine
    PolicyEngine --> ApplyDefaults
    ApplyDefaults --> TeamAuth
    ApplyDefaults --> TeamRate
    
    TeamConfig --> AddUser
    AddUser --> UserMember
    AddUser --> InheritPolicy
    InheritPolicy --> PolicyEngine
    
    UserMember --> CreateKey
    CreateKey --> KeySecret
    CreateKey --> EventTrigger
    EventTrigger --> PolicyEngine
    PolicyEngine --> UpdatePolicies
    UpdatePolicies --> AuthPolicy
    UpdatePolicies --> TokenRateLimit
    
    AuthPolicy --> Gateway
    TokenRateLimit --> Gateway
    Gateway --> Models
    
    UserApp --> APIRequest
    APIRequest --> Gateway
    Gateway --> ModelResponse
    ModelResponse --> UserApp
    
    %% Styling
    classDef adminClass fill:#e1f5fe,stroke:#0277bd,stroke-width:2px
    classDef teamClass fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef userClass fill:#e8f5e8,stroke:#388e3c,stroke-width:2px
    classDef keyClass fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef policyClass fill:#fce4ec,stroke:#c2185b,stroke-width:2px
    classDef k8sClass fill:#f1f8e9,stroke:#689f38,stroke-width:2px
    classDef appClass fill:#e3f2fd,stroke:#1976d2,stroke-width:2px
    
    class Admin,DefaultPolicies adminClass
    class CreateTeam,TeamConfig,TeamAuth,TeamRate teamClass
    class AddUser,UserMember,InheritPolicy userClass
    class CreateKey,KeySecret,UpdatePolicies keyClass
    class PolicyEngine,ApplyDefaults,EventTrigger policyClass
    class AuthPolicy,TokenRateLimit,Gateway,Models k8sClass
    class UserApp,APIRequest,ModelResponse appClass
```

## Detailed Team Creation Workflow

```mermaid
sequenceDiagram
    participant Admin as Platform Admin
    participant API as Key Manager API
    participant PE as Policy Engine
    participant K8s as Kubernetes API
    participant KC as Kuadrant Controller
    
    Note over Admin, KC: Team Creation with Default Policies
    
    Admin->>API: POST /v2/teams<br/>{team_name, tier, budget}
    
    API->>API: Validate team data<br/>Check name uniqueness
    
    API->>K8s: Create team config secret<br/>with metadata & labels
    
    API->>PE: Trigger team.created event<br/>{team_id, tier}
    
    PE->>PE: Load default policies<br/>from ConfigMap
    
    PE->>K8s: Create team AuthPolicy<br/>team-{id}-auth
    
    PE->>K8s: Create team TokenRateLimit<br/>team-{id}-limits
    
    K8s->>KC: AuthPolicy CRD created
    K8s->>KC: TokenRateLimit CRD created
    
    KC->>KC: Process new policies<br/>Configure gateway rules
    
    PE-->>API: Policies applied successfully
    API-->>Admin: Team created with policies
    
    Note over Admin, KC: Team now ready for users & keys
```

## User Addition to Team Workflow

```mermaid
sequenceDiagram
    participant Admin as Team Admin
    participant API as Key Manager API  
    participant PE as Policy Engine
    participant AUTH as External Auth Service
    participant K8s as Kubernetes API
    
    Note over Admin, K8s: Adding User to Team
    
    Admin->>API: POST /v2/teams/{id}/members<br/>{user_email, role, budget}
    
    API->>AUTH: Validate user exists<br/>Get user_id from email
    AUTH-->>API: User validation response
    
    API->>K8s: Check team membership<br/>limits & permissions
    
    API->>K8s: Create user-team membership<br/>secret with role & budget
    
    API->>PE: Trigger user.added_to_team<br/>{user_id, team_id, role}
    
    PE->>PE: Load team policies<br/>Apply user overrides
    
    PE->>K8s: Update team AuthPolicy<br/>to include user context
    
    PE-->>API: User policies applied
    API-->>Admin: User added to team
    
    Note over Admin, K8s: User can now create team keys
```

## API Key Creation with Policy Application

```mermaid
sequenceDiagram
    participant User as Team Member
    participant API as Key Manager API
    participant PE as Policy Engine
    participant K8s as Kubernetes API
    participant KC as Kuadrant Controller
    
    Note over User, KC: Team-scoped API Key Creation
    
    User->>API: POST /v2/teams/{id}/keys<br/>{user_id, models, budget}
    
    API->>K8s: Validate user team membership<br/>Check permissions & quotas
    
    API->>API: Generate secure API key<br/>Create key hash
    
    API->>K8s: Create enhanced key secret<br/>team+user labels & policies
    
    API->>PE: Trigger api_key.created<br/>{key_id, user_id, team_id}
    
    PE->>PE: Load user+team policies<br/>Apply key-specific overrides
    
    PE->>K8s: Update team AuthPolicy<br/>include new key selector
    
    PE->>K8s: Update TokenRateLimit<br/>key-specific counters
    
    K8s->>KC: Policy CRDs updated
    KC->>KC: Reconfigure gateway<br/>Apply new key permissions
    
    PE-->>API: Key policies applied
    API-->>User: API key ready for use
    
    Note over User, KC: Key active with team policies
```

## Policy Inheritance and Application Flow

```mermaid
graph TD
    subgraph "Policy Inheritance Chain"
        PlatformDefaults[Platform Default Policies<br/>ConfigMap]
        TierPolicies[Tier-specific Policies<br/>free/standard/premium]
        TeamPolicies[Team Policies<br/>team-specific overrides]
        UserPolicies[User Policies<br/>role-based + custom budget]
        KeyPolicies[API Key Policies<br/>model access + key budget]
    end
    
    subgraph "Policy Application Rules"
        NewTeamRule[New Team Rule<br/>Apply tier defaults]
        NewUserRule[New User Rule<br/>Inherit team + role]
        NewKeyRule[New Key Rule<br/>User + specific config]
        BudgetRule[Budget Rule<br/>Aggregate team spending]
    end
    
    subgraph "Generated Kuadrant Resources"
        TeamAuth[Team AuthPolicy<br/>team-{id}-auth]
        TeamRate[Team TokenRateLimit<br/>team-{id}-limits]
        GlobalAuth[Global AuthPolicy<br/>all teams aggregated]
        GlobalRate[Global TokenRateLimit<br/>tier-based limits]
    end
    
    PlatformDefaults --> TierPolicies
    TierPolicies --> TeamPolicies
    TeamPolicies --> UserPolicies
    UserPolicies --> KeyPolicies
    
    NewTeamRule --> TeamPolicies
    NewUserRule --> UserPolicies
    NewKeyRule --> KeyPolicies
    BudgetRule --> TeamPolicies
    
    TeamPolicies --> TeamAuth
    TeamPolicies --> TeamRate
    UserPolicies --> GlobalAuth
    KeyPolicies --> GlobalRate
    
    %% Styling
    classDef policyClass fill:#e8f5e8,stroke:#388e3c,stroke-width:2px
    classDef ruleClass fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef resourceClass fill:#e3f2fd,stroke:#1976d2,stroke-width:2px
    
    class PlatformDefaults,TierPolicies,TeamPolicies,UserPolicies,KeyPolicies policyClass
    class NewTeamRule,NewUserRule,NewKeyRule,BudgetRule ruleClass
    class TeamAuth,TeamRate,GlobalAuth,GlobalRate resourceClass
```