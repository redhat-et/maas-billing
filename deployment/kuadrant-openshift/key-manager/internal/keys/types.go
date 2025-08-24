package keys

// API key structures
type CreateTeamKeyRequest struct {
	UserID            string                 `json:"user_id" binding:"required"`
	UserEmail         string                 `json:"user_email"`
	Alias             string                 `json:"alias"`
	Models            []string               `json:"models"`
	InheritTeamLimits bool                   `json:"inherit_team_limits"`
	// Rate limit overrides
	TokenLimit   int                    `json:"token_limit,omitempty"`
	RequestLimit int                    `json:"request_limit,omitempty"`
	TimeWindow   string                 `json:"time_window,omitempty"`
	CustomLimits map[string]interface{} `json:"custom_limits"`
}

type CreateTeamKeyResponse struct {
	APIKey            string                 `json:"api_key"`
	UserID            string                 `json:"user_id"`
	TeamID            string                 `json:"team_id"`
	SecretName        string                 `json:"secret_name"`
	Policy            string                 `json:"policy"`
	CreatedAt         string                 `json:"created_at"`
	InheritedPolicies map[string]interface{} `json:"inherited_policies"`
}

// Legacy structures (keep for backward compatibility)
type GenerateKeyRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type DeleteKeyRequest struct {
	Key string `json:"key" binding:"required"`
}
