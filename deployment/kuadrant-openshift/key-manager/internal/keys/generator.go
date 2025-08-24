package keys

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
)

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64 URL-safe string
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// isValidUserID validates user ID according to Kubernetes RFC 1123 subdomain rules
func isValidUserID(userID string) bool {
	// Must be 1-63 characters long
	if len(userID) == 0 || len(userID) > 63 {
		return false
	}

	// Must contain only lowercase alphanumeric characters and hyphens
	// Must start and end with an alphanumeric character
	validPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	return validPattern.MatchString(userID)
}

// ValidateUserID validates a user ID using Kubernetes naming rules
func ValidateUserID(userID string) bool {
	return isValidUserID(userID)
}