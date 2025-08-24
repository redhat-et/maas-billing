package teams

import "regexp"

// Team management structures
type CreateTeamRequest struct {
	TeamID      string `json:"team_id" binding:"required"`
	TeamName    string `json:"team_name" binding:"required"`
	Description string `json:"description"`
	Policy      string `json:"policy,omitempty"`
	TokenLimit  int    `json:"token_limit,omitempty"` // Token limit per window (default: 100000)
	TimeWindow  string `json:"time_window,omitempty"` // Time window (default: "1h")
}

type UpdateTeamRequest struct {
	TeamName    *string `json:"team_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Policy      *string `json:"policy,omitempty"`
	TokenLimit  *int    `json:"token_limit,omitempty"`
	TimeWindow  *string `json:"time_window,omitempty"`
}

type CreateTeamResponse struct {
	TeamID      string `json:"team_id"`
	TeamName    string `json:"team_name"`
	Description string `json:"description"`
	Policy      string `json:"policy"`
	CreatedAt   string `json:"created_at"`
}

type GetTeamResponse struct {
	TeamID      string       `json:"team_id"`
	TeamName    string       `json:"team_name"`
	Description string       `json:"description"`
	Policy      string       `json:"policy"`
	Members     []TeamMember `json:"users"`
	Keys        []string     `json:"keys"`
	CreatedAt   string       `json:"created_at"`
}

type TeamMember struct {
	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	Role      string `json:"role"`
	TeamID    string `json:"team_id"`
	TeamName  string `json:"team_name"`
	JoinedAt  string `json:"joined_at"`
	Policy    string `json:"policy"` // Inherited from team
}

// User management structures
type AddUserToTeamRequest struct {
	UserEmail string `json:"user_email" binding:"required"`
	Role      string `json:"role" binding:"required"`
	// Individual rate overrides
	TokenLimit   int    `json:"token_limit,omitempty"`
	RequestLimit int    `json:"request_limit,omitempty"`
	TimeWindow   string `json:"time_window,omitempty"`
}

// Validation helpers

// isValidTeamID validates team ID according to Kubernetes RFC 1123 subdomain rules
func isValidTeamID(teamID string) bool {
	// Must be 1-63 characters long
	if len(teamID) == 0 || len(teamID) > 63 {
		return false
	}

	// Must contain only lowercase alphanumeric characters and hyphens
	// Must start and end with an alphanumeric character
	validPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	return validPattern.MatchString(teamID)
}