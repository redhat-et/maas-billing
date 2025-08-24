package types

import "time"

// UserUsage represents aggregated usage for a specific user
type UserUsage struct {
	UserID              string                 `json:"user_id"`
	TotalTokenUsage     int64                  `json:"total_token_usage"`
	TotalAuthorizedCalls int64                 `json:"total_authorized_calls"`
	TotalLimitedCalls   int64                  `json:"total_limited_calls"`
	TeamBreakdown       []TeamUserUsage        `json:"team_breakdown"`
	LastUpdated         time.Time              `json:"last_updated"`
}

// TeamUsage represents aggregated usage for a specific team (group)
type TeamUsage struct {
	TeamID              string                 `json:"team_id"`
	TeamName            string                 `json:"team_name"`
	Policy              string                 `json:"policy"`
	TotalTokenUsage     int64                  `json:"total_token_usage"`
	TotalAuthorizedCalls int64                 `json:"total_authorized_calls"`
	TotalLimitedCalls   int64                  `json:"total_limited_calls"`
	UserBreakdown       []UserTeamUsage        `json:"user_breakdown"`
	LastUpdated         time.Time              `json:"last_updated"`
}

// TeamUserUsage represents a user's usage within a specific team context
type TeamUserUsage struct {
	TeamID              string `json:"team_id"`
	TeamName            string `json:"team_name"`
	Policy              string `json:"policy"`
	TokenUsage          int64  `json:"token_usage"`
	AuthorizedCalls     int64  `json:"authorized_calls"`
	LimitedCalls        int64  `json:"limited_calls"`
}

// UserTeamUsage represents team usage broken down by user
type UserTeamUsage struct {
	UserID              string `json:"user_id"`
	UserEmail           string `json:"user_email"`
	TokenUsage          int64  `json:"token_usage"`
	AuthorizedCalls     int64  `json:"authorized_calls"`
	LimitedCalls        int64  `json:"limited_calls"`
}

// PrometheusMetric represents a parsed Prometheus metric
type PrometheusMetric struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     int64             `json:"value"`
	Help      string            `json:"help"`
	Type      string            `json:"type"`
}