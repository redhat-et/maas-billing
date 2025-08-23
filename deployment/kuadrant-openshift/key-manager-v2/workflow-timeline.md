# Workflow Timeline and Sequence Documentation

## Complete Workflow Timeline

```mermaid
timeline
    title Team Management Workflow Timeline
    
    section Platform Setup
        T0 : Platform Initialization
           : Default policy templates loaded
           : Kuadrant operators ready
           : Base AuthPolicy configured
           
    section Team Creation (T1-T3)
        T1 : Admin creates team
           : POST /v2/teams request
           : Team validation
           
        T2 : Team policies generated
           : Team config secret created
           : Default policies applied
           : Team AuthPolicy generated
           : Team TokenRateLimit created
           
        T3 : Team ready
           : Policies active in Kuadrant
           : Team available for members
           
    section User Management (T4-T6)
        T4 : Add first user
           : POST /v2/teams/{id}/members
           : User validation via OIDC
           
        T5 : User policies applied
           : User-team membership secret
           : Policy inheritance from team
           : Team policies updated
           
        T6 : User ready
           : User can create API keys
           : Team member permissions active
           
    section API Key Lifecycle (T7-T10)
        T7 : User creates API key
           : POST /v2/teams/{id}/keys
           : Key generation and validation
           
        T8 : Key policies applied
           : Enhanced key secret created
           : AuthPolicy updated for key
           : TokenRateLimit updated
           
        T9 : Key active
           : Kuadrant policies synchronized
           : Key ready for API requests
           
        T10 : First API usage
            : User makes model request
            : Authentication successful
            : Token counting begins
            
    section Ongoing Operations (T11+)
        T11 : Scale operations
            : Additional users added
            : Multiple API keys created
            : Policy updates propagated
            
        T12 : Budget monitoring
            : Usage tracking active
            : Spend limits enforced
            : Alerts generated
```

## Detailed Event Sequence Timeline

```mermaid
gantt
    title Team Workflow Implementation Timeline
    dateFormat X
    axisFormat %s
    
    section Platform Setup
    Load default policies        :milestone, m1, 0, 0
    Kuadrant ready              :milestone, m2, 1, 1
    
    section Team Creation
    Validate team request       :active, t1, 2, 5
    Create team config          :t2, 5, 8
    Generate team AuthPolicy    :t3, 8, 12
    Generate TokenRateLimit     :t4, 8, 12
    Apply to Kuadrant          :t5, 12, 15
    Team ready                 :milestone, m3, 15, 15
    
    section User Addition
    Validate user              :u1, 16, 18
    Create membership secret   :u2, 18, 20
    Update team policies       :u3, 20, 23
    User ready                :milestone, m4, 23, 23
    
    section API Key Creation
    Generate API key           :k1, 24, 26
    Create enhanced secret     :k2, 26, 28
    Update AuthPolicy          :k3, 28, 31
    Update TokenRateLimit      :k4, 28, 31
    Sync to gateway           :k5, 31, 34
    Key active               :milestone, m5, 34, 34
    
    section First Usage
    API request received      :r1, 35, 36
    Authentication check      :r2, 36, 37
    Rate limit check         :r3, 37, 38
    Forward to model         :r4, 38, 40
    Response & tracking      :r5, 40, 42
    
    section Scaling
    Add more users           :s1, 43, 50
    Create more keys         :s2, 45, 55
    Monitor usage           :s3, 50, 60
```

## Event-Driven Timeline with Dependencies

```mermaid
journey
    title User Journey: From Team Creation to API Usage
    
    section Platform Admin
        Platform setup      : 5 : Admin
        Create team         : 4 : Admin
        Set team policies   : 4 : Admin, System
        Team operational    : 5 : Admin, System
        
    section Team Admin  
        Add team member     : 4 : TeamAdmin
        Configure user role : 4 : TeamAdmin, System
        Member active       : 5 : TeamAdmin, System
        
    section Team Member
        Request API key     : 3 : User
        Configure key access: 4 : User, System
        Receive API key     : 5 : User, System
        
    section Developer
        Integrate API key   : 3 : Developer
        Make first request  : 4 : Developer, Gateway
        Successful response : 5 : Developer, Gateway, Model
        Monitor usage       : 4 : Developer, System
```

## Policy Application Timeline

```mermaid
timeline
    title Policy Application and Propagation Timeline
    
    section T0 - Platform Defaults
        Platform Policies : Default tier policies loaded
                         : ConfigMaps available
                         : Policy templates ready
                         
    section T1 - Team Creation
        Team Policy Generation : Team-specific AuthPolicy created
                             : Team-specific TokenRateLimit created
                             : Policies applied to Kuadrant
                             
    section T2 - User Addition  
        Policy Inheritance : User inherits team policies
                          : User-specific overrides applied
                          : Membership tracked in secret
                          
    section T3 - Key Creation
        Key Policy Application : Key inherits user+team policies
                              : Key-specific restrictions applied
                              : AuthPolicy updated with key selector
                              : TokenRateLimit updated with key counter
                              
    section T4 - Runtime Enforcement
        Policy Enforcement : Gateway validates API key
                          : Rate limiting enforced per key
                          : Token consumption tracked
                          : Budget limits monitored
```

## Resource Creation Timeline

```mermaid
graph LR
    subgraph "Time: T0 (Platform Setup)"
        CM[ConfigMap: Default Policies]
        GP[Global AuthPolicy: Base Rules]
        GR[Global TokenRateLimit: Basic Limits]
    end
    
    subgraph "Time: T1 (Team Creation)"
        TS[Secret: team-config]
        TAP[AuthPolicy: team-specific]
        TRL[TokenRateLimit: team-specific]
    end
    
    subgraph "Time: T2 (User Addition)"
        MS[Secret: user-team-membership]
        UPL[Policy: user inheritance]
    end
    
    subgraph "Time: T3 (Key Creation)"
        KS[Secret: api-key-enhanced]
        UAP[Updated: team AuthPolicy]
        URL[Updated: team TokenRateLimit]
    end
    
    subgraph "Time: T4 (Usage)"
        REQ[API Request]
        AUTH[Authentication Check]
        RATE[Rate Limit Check]
        RESP[Model Response]
        TRACK[Usage Tracking]
    end
    
    CM --> TS
    GP --> TAP
    GR --> TRL
    
    TS --> MS
    TAP --> UPL
    
    MS --> KS
    UPL --> UAP
    TRL --> URL
    
    KS --> REQ
    UAP --> AUTH
    URL --> RATE
    AUTH --> RESP
    RATE --> TRACK
    
    %% Timeline flow
    CM -.->|T0-T1| TS
    TS -.->|T1-T2| MS
    MS -.->|T2-T3| KS
    KS -.->|T3-T4| REQ
```

## Critical Path Analysis

```mermaid
graph TD
    Start([Start: Platform Ready])
    
    subgraph "Critical Path: Team Setup"
        A[Create Team Request]
        B[Generate Team Policies]
        C[Apply to Kuadrant]
        D{Policies Active?}
        E[Team Ready]
    end
    
    subgraph "Critical Path: User Setup"  
        F[Add User Request]
        G[Create Membership]
        H[Apply User Policies]
        I{User Policies Active?}
        J[User Ready]
    end
    
    subgraph "Critical Path: Key Setup"
        K[Create Key Request]
        L[Generate API Key]
        M[Update Policies]
        N{Key Policies Active?}
        O[Key Ready]
    end
    
    subgraph "Critical Path: Usage"
        P[API Request]
        Q[Auth Check]
        R[Rate Check]
        S[Forward to Model]
        T[Response]
    end
    
    Start --> A
    A --> B
    B --> C
    C --> D
    D -->|Yes| E
    D -->|No| B
    
    E --> F
    F --> G
    G --> H
    H --> I
    I -->|Yes| J
    I -->|No| H
    
    J --> K
    K --> L
    L --> M
    M --> N
    N -->|Yes| O
    N -->|No| M
    
    O --> P
    P --> Q
    Q --> R
    R --> S
    S --> T
    
    %% Critical path styling
    classDef critical fill:#ffcdd2,stroke:#d32f2f,stroke-width:3px
    classDef ready fill:#c8e6c9,stroke:#388e3c,stroke-width:2px
    classDef check fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    
    class A,B,C,F,G,H,K,L,M,P critical
    class E,J,O ready
    class D,I,N,Q,R check
```