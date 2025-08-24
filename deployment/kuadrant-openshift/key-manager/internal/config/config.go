package config

import "os"

// Config holds application configuration
type Config struct {
	// Server configuration
	Port        string
	ServiceName string

	// Kubernetes configuration
	KeyNamespace        string
	SecretSelectorLabel string
	SecretSelectorValue string

	// Kuadrant configuration
	TokenRateLimitPolicyName string
	AuthPolicyName           string

	// Default team configuration
	CreateDefaultTeam bool
	AdminAPIKey       string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		// Server configuration
		Port:        getEnvOrDefault("PORT", "8080"),
		ServiceName: getEnvOrDefault("SERVICE_NAME", "key-manager"),

		// Kubernetes configuration
		KeyNamespace:        getEnvOrDefault("KEY_NAMESPACE", "llm"),
		SecretSelectorLabel: getEnvOrDefault("SECRET_SELECTOR_LABEL", "kuadrant.io/apikeys-by"),
		SecretSelectorValue: getEnvOrDefault("SECRET_SELECTOR_VALUE", "rhcl-keys"),

		// Kuadrant configuration
		TokenRateLimitPolicyName: getEnvOrDefault("TOKEN_RATE_LIMIT_POLICY_NAME", "gateway-token-rate-limits"),
		AuthPolicyName:           getEnvOrDefault("AUTH_POLICY_NAME", "gateway-auth-policy"),

		// Default team configuration
		CreateDefaultTeam: getEnvOrDefault("CREATE_DEFAULT_TEAM", "true") == "true",
		AdminAPIKey:       getEnvOrDefault("ADMIN_API_KEY", ""),
	}
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}