# Alternative Policy Application Approaches in OpenShift

## Overview

This document evaluates different approaches for applying Kuadrant policies in OpenShift, comparing the benefits and trade-offs of each method for the MaaS platform's team and API key management workflow.

## Approach 1: Go Key-Manager Application (RECOMMENDED)

### Description
Policies are dynamically created and managed by the Go key-manager application using Kubernetes dynamic client APIs.

### Implementation
```go
// Dynamic policy management through Go application
func (pe *PolicyEngine) ApplyTeamPolicies(teamID, tier string) error {
    // Generate policies from templates
    authPolicy := pe.generateTeamAuthPolicy(teamID, tier)
    rateLimitPolicy := pe.generateTeamRateLimitPolicy(teamID, tier)
    
    // Apply using dynamic client
    err := pe.kuadrantClient.Resource(authPolicyGVR).
        Namespace("llm").Create(context.Background(), authPolicy, metav1.CreateOptions{})
    
    return err
}
```

### Advantages
- **Dynamic Policy Generation**: Policies generated with real-time team/user context
- **Immediate Synchronization**: Policy updates happen instantly when teams/users change
- **Complex Logic Support**: Can implement sophisticated policy rules and inheritance
- **Error Handling**: Robust rollback and validation mechanisms
- **Centralized Control**: All policy logic in one place with the key management
- **Integration**: Seamless integration with existing API key workflow
- **Performance**: Direct API calls without external dependencies

### Disadvantages
- **Code Complexity**: More complex Go code to maintain
- **RBAC Requirements**: Key-manager needs extensive Kubernetes permissions
- **Single Point of Failure**: If key-manager is down, no policy updates
- **Testing Complexity**: Requires comprehensive integration testing

### Use Cases
- Dynamic team creation/deletion
- Real-time policy inheritance
- Complex conditional policies
- Immediate policy synchronization

## Approach 2: GitOps with ArgoCD/Flux

### Description
Policies are defined as YAML files in Git repositories and automatically applied via GitOps operators.

### Implementation
```yaml
# Git repository structure
policies/
├── teams/
│   ├── data-science/
│   │   ├── auth-policy.yaml
│   │   └── rate-limit-policy.yaml
│   └── ml-ops/
│       ├── auth-policy.yaml
│       └── rate-limit-policy.yaml
├── tiers/
│   ├── free-tier-defaults.yaml
│   ├── standard-tier-defaults.yaml
│   └── premium-tier-defaults.yaml
└── global/
    └── platform-defaults.yaml
```

```yaml
# Example team policy file
apiVersion: kuadrant.io/v1alpha1
kind: TokenRateLimitPolicy
metadata:
  name: team-data-science-limits
  namespace: llm
spec:
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: inference-gateway
  limits:
    team-data-science-standard:
      rates:
      - limit: 50000
        window: 1h
      counters:
      - expression: 'auth.identity.metadata.labels.maas/team-id'
      when:
      - selector: 'auth.identity.metadata.labels.maas/team-id'
        operator: eq
        value: "data-science"
```

### Advantages
- **Version Control**: All policy changes tracked in Git
- **Audit Trail**: Complete history of policy modifications
- **Declarative**: Infrastructure as Code principles
- **Multi-cluster**: Can sync policies across multiple clusters
- **Rollback**: Easy rollback to previous policy versions
- **Separation of Concerns**: Policy definitions separate from application code
- **GitOps Best Practices**: Follows established GitOps patterns

### Disadvantages
- **Manual Process**: Requires manual Git commits for policy changes
- **Slower Updates**: Policy changes require Git workflow (commit → sync → apply)
- **Template Complexity**: Difficult to implement dynamic policy generation
- **Coordination**: Need to coordinate between key-manager and Git repository
- **Merge Conflicts**: Potential conflicts when multiple teams update policies
- **Bootstrap Problem**: Initial policy setup more complex

### Use Cases
- Static team policies
- Audit-heavy environments
- Multi-cluster deployments
- Organizations with strong GitOps culture

## Approach 3: Kubernetes Operators (Custom Resource Definitions)

### Description
Create custom CRDs (TeamPolicy, UserPolicy) and a custom operator to translate them into Kuadrant policies.

### Implementation
```yaml
# Custom CRD definition
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: teampolicies.maas.platform.com
spec:
  group: maas.platform.com
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              teamId:
                type: string
              tier:
                type: string
              tokenLimits:
                type: object
              budgetLimits:
                type: object
              modelsAllowed:
                type: array
```

```yaml
# Team policy resource
apiVersion: maas.platform.com/v1
kind: TeamPolicy
metadata:
  name: data-science-policy
  namespace: llm
spec:
  teamId: "data-science"
  tier: "standard"
  tokenLimits:
    hourly: 50000
    daily: 500000
  budgetLimits:
    monthly: 1000.0
  modelsAllowed:
  - "simulator-model"
  - "qwen3-0-6b-instruct"
```

```go
// Custom operator controller
func (r *TeamPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get TeamPolicy resource
    teamPolicy := &maasv1.TeamPolicy{}
    err := r.Get(ctx, req.NamespacedName, teamPolicy)
    
    // Generate Kuadrant policies from TeamPolicy spec
    authPolicy := r.generateAuthPolicy(teamPolicy)
    rateLimitPolicy := r.generateRateLimitPolicy(teamPolicy)
    
    // Apply generated policies
    return r.applyPolicies(ctx, authPolicy, rateLimitPolicy)
}
```

### Advantages
- **Kubernetes Native**: Follows Kubernetes patterns and conventions
- **Declarative**: Desired state management
- **Controller Pattern**: Automatic reconciliation and drift correction
- **Custom Logic**: Can implement complex policy translation logic
- **Resource Management**: Proper lifecycle management of generated policies
- **Event-driven**: Responds to resource changes automatically
- **Scalable**: Can handle many teams and policies efficiently

### Disadvantages
- **Development Overhead**: Requires building and maintaining a custom operator
- **Learning Curve**: Team needs to understand operator development
- **Debugging Complexity**: More complex debugging with multiple layers
- **Resource Overhead**: Additional operator pods and CRDs
- **Integration**: Need to integrate operator with key-manager workflow
- **Testing**: Requires extensive controller testing

### Use Cases
- Large-scale deployments
- Organizations wanting pure Kubernetes-native solutions
- Complex policy transformation requirements
- Multi-tenant platforms

## Approach 4: Helm Charts with Values Injection

### Description
Use Helm charts to template policies and inject values from the key-manager or external configuration.

### Implementation
```yaml
# Helm template for team policies
# templates/team-rate-limit-policy.yaml
{{- range .Values.teams }}
apiVersion: kuadrant.io/v1alpha1
kind: TokenRateLimitPolicy
metadata:
  name: team-{{ .id }}-limits
  namespace: {{ $.Values.namespace }}
spec:
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: {{ $.Values.gateway.name }}
  limits:
    team-{{ .id }}-{{ .tier }}:
      rates:
      - limit: {{ .limits.hourly }}
        window: 1h
      counters:
      - expression: 'auth.identity.metadata.labels.maas/team-id'
      when:
      - selector: 'auth.identity.metadata.labels.maas/team-id'
        operator: eq
        value: "{{ .id }}"
---
{{- end }}
```

```yaml
# values.yaml
namespace: llm
gateway:
  name: inference-gateway

teams:
- id: "data-science"
  tier: "standard"
  limits:
    hourly: 50000
    daily: 500000
  budget: 1000.0
- id: "ml-ops"
  tier: "premium"
  limits:
    hourly: 200000
    daily: 2000000
  budget: 5000.0
```

```go
// Key-manager triggers Helm deployment
func (km *KeyManager) createTeamWithHelm(teamID, tier string) error {
    // Update values.yaml with new team
    values := km.loadHelmValues()
    values.Teams = append(values.Teams, TeamConfig{
        ID:   teamID,
        Tier: tier,
        Limits: TierLimits[tier],
    })
    
    // Deploy/upgrade Helm chart
    return km.helmClient.Upgrade("team-policies", values)
}
```

### Advantages
- **Templating**: Powerful templating engine for policy generation
- **Values Management**: External configuration through values files
- **Packaging**: Policies bundled as installable packages
- **Versioning**: Helm chart versioning for policy versions
- **Rollback**: Easy rollback to previous chart versions
- **Ecosystem**: Leverages existing Helm ecosystem and tools

### Disadvantages
- **Batch Updates**: All policies updated together, not individually
- **Helm Dependency**: Adds Helm as a dependency in the workflow
- **Values Management**: Complex values file management for many teams
- **Limited Dynamism**: Less dynamic than direct API calls
- **Coordination**: Need to coordinate between key-manager and Helm
- **Performance**: Slower than direct API calls

### Use Cases
- Environments already using Helm extensively
- Batch policy updates
- Policy packaging and distribution
- Development/staging environments

## Approach 5: Terraform with Kubernetes Provider

### Description
Use Terraform to manage policies as infrastructure, with the key-manager triggering Terraform runs.

### Implementation
```hcl
# teams.tf
variable "teams" {
  description = "Team configurations"
  type = map(object({
    tier = string
    token_limit_hourly = number
    budget_monthly = number
    models_allowed = list(string)
  }))
}

resource "kubernetes_manifest" "team_rate_limit_policy" {
  for_each = var.teams
  
  manifest = {
    apiVersion = "kuadrant.io/v1alpha1"
    kind       = "TokenRateLimitPolicy"
    metadata = {
      name      = "team-${each.key}-limits"
      namespace = "llm"
    }
    spec = {
      targetRef = {
        group = "gateway.networking.k8s.io"
        kind  = "Gateway"
        name  = "inference-gateway"
      }
      limits = {
        "team-${each.key}-${each.value.tier}" = {
          rates = [{
            limit  = each.value.token_limit_hourly
            window = "1h"
          }]
          counters = [{
            expression = "auth.identity.metadata.labels.maas/team-id"
          }]
          when = [{
            selector = "auth.identity.metadata.labels.maas/team-id"
            operator = "eq"
            value    = each.key
          }]
        }
      }
    }
  }
}
```

```go
// Key-manager triggers Terraform
func (km *KeyManager) createTeamWithTerraform(teamID, tier string) error {
    // Update terraform.tfvars
    tfVars := km.loadTerraformVars()
    tfVars.Teams[teamID] = TeamConfig{
        Tier: tier,
        TokenLimitHourly: TierLimits[tier].Hourly,
        BudgetMonthly: TierLimits[tier].Budget,
        ModelsAllowed: TierLimits[tier].Models,
    }
    
    // Run terraform apply
    return km.terraformClient.Apply(tfVars)
}
```

### Advantages
- **Infrastructure as Code**: Mature IaC practices and tooling
- **State Management**: Terraform state tracking and drift detection
- **Planning**: Plan/apply workflow with change preview
- **Provider Ecosystem**: Rich ecosystem of Terraform providers
- **Enterprise Features**: Terraform Cloud/Enterprise features
- **Multi-cloud**: Can manage resources across multiple clouds

### Disadvantages
- **Complexity**: Adds Terraform complexity to the stack
- **State Management**: Need to manage Terraform state safely
- **Performance**: Slower than direct API calls
- **Learning Curve**: Team needs Terraform expertise
- **Dependency**: Another external dependency in the workflow
- **Locking**: Terraform state locking considerations

### Use Cases
- Organizations already using Terraform extensively
- Multi-cloud deployments
- Complex infrastructure management requirements
- Compliance-heavy environments

## Approach 6: External Policy Management Service

### Description
Separate microservice dedicated to policy management, with the key-manager calling its APIs.

### Implementation
```yaml
# Policy management service
apiVersion: apps/v1
kind: Deployment
metadata:
  name: policy-manager
  namespace: platform-services
spec:
  template:
    spec:
      containers:
      - name: policy-manager
        image: policy-manager:v1.0.0
        env:
        - name: KUBECONFIG_PATH
          value: /etc/kubeconfig/config
        ports:
        - containerPort: 8080
```

```go
// Policy manager service API
type PolicyManagerService struct {
    kubeClient     client.Client
    templateStore  PolicyTemplateStore
    policyApplier  PolicyApplier
}

func (pms *PolicyManagerService) CreateTeamPolicies(ctx context.Context, req *CreateTeamPoliciesRequest) (*CreateTeamPoliciesResponse, error) {
    // Load policy templates
    template := pms.templateStore.GetTierTemplate(req.Tier)
    
    // Generate team policies
    policies := pms.generateTeamPolicies(req.TeamID, template)
    
    // Apply policies to cluster
    return pms.policyApplier.ApplyPolicies(ctx, policies)
}

// Key-manager calls policy service
func (km *KeyManager) createTeamPolicies(teamID, tier string) error {
    client := policysvc.NewClient(km.policyServiceURL)
    
    req := &policysvc.CreateTeamPoliciesRequest{
        TeamID: teamID,
        Tier:   tier,
    }
    
    _, err := client.CreateTeamPolicies(context.Background(), req)
    return err
}
```

### Advantages
- **Separation of Concerns**: Policy logic separated from key management
- **Specialized Service**: Dedicated service for policy management
- **API-driven**: Clean API interface for policy operations
- **Scalability**: Can scale policy service independently
- **Reusability**: Policy service can be used by other services
- **Language Agnostic**: Policy service can be implemented in any language

### Disadvantages
- **Additional Service**: More services to deploy and maintain
- **Network Latency**: Network calls between services
- **Service Discovery**: Need service discovery and load balancing
- **Error Handling**: Distributed system error handling complexity
- **Coordination**: Need to coordinate between multiple services
- **Dependencies**: Additional service dependencies

### Use Cases
- Microservices architectures
- Organizations wanting specialized policy services
- Large-scale deployments with many policy consumers
- Service-oriented architectures

## Recommendation Matrix

| Approach | Complexity | Performance | Flexibility | Maintenance | Best For |
|----------|------------|-------------|-------------|-------------|----------|
| **Go Key-Manager** | Medium | High | High | Medium | Dynamic, real-time policies |
| **GitOps** | Low | Medium | Medium | Low | Audit-heavy, static policies |
| **Custom Operator** | High | High | High | High | Large-scale, Kubernetes-native |
| **Helm Charts** | Medium | Medium | Medium | Medium | Helm-centric environments |
| **Terraform** | Medium | Low | Medium | Medium | IaC-heavy organizations |
| **External Service** | High | Medium | High | High | Microservices architectures |

## Final Recommendation

**For the MaaS platform, the Go Key-Manager approach (Approach 1) is recommended** because:

1. **Dynamic Requirements**: The platform needs real-time policy generation based on team/user context
2. **Integration**: Seamless integration with existing API key management workflow
3. **Performance**: Direct API calls provide immediate policy updates
4. **Complexity Balance**: Reasonable complexity for the dynamic features required
5. **Control**: Full control over policy logic and error handling
6. **Existing Architecture**: Builds on existing Go key-manager service

The hybrid approach can also be considered where:
- **Static policies** (platform defaults, tier templates) managed via GitOps
- **Dynamic policies** (team-specific, user-specific) managed via Go key-manager
- **Audit trail** maintained through Kubernetes events and logging