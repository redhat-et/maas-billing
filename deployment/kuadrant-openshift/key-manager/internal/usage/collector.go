package usage

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/types"
)

// Collector handles usage data collection from Istio Prometheus metrics
type Collector struct {
	clientset     *kubernetes.Clientset
	config        *rest.Config
	namespace     string
	metricsURL    string
	httpClient    *http.Client
}

// NewCollector creates a new usage collector
func NewCollector(clientset *kubernetes.Clientset, config *rest.Config, namespace string) *Collector {
	// Construct the service URL for envoy metrics
	// Format: http://service-name.namespace.svc.cluster.local:port/path
	metricsURL := fmt.Sprintf("http://inference-gateway-envoy-metrics.%s.svc.cluster.local:15090/stats/prometheus", namespace)
	
	return &Collector{
		clientset:  clientset,
		config:     config,
		namespace:  namespace,
		metricsURL: metricsURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetUserUsage collects and aggregates usage data for a specific user
func (c *Collector) GetUserUsage(userID string) (*types.UserUsage, error) {
	log.Printf("DEBUG: GetUserUsage called for userID: %s", userID)
	metrics, err := c.collectPrometheusMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	userUsage := &types.UserUsage{
		UserID:        userID,
		TeamBreakdown: []types.TeamUserUsage{},
		LastUpdated:   time.Now(),
	}

	policyMap := make(map[string]*types.TeamUserUsage)
	var userMetricsFound []string

	log.Printf("DEBUG: Looking for user metrics containing: user___%s___", userID)
	
	for _, metric := range metrics {
		if strings.Contains(metric.Name, fmt.Sprintf("user___%s___", userID)) {
			userMetricsFound = append(userMetricsFound, fmt.Sprintf("%s = %d", metric.Name, metric.Value))
			policyName := extractPolicyFromMetric(metric.Name)
			// Convert underscores back to hyphens for policy name (reverse the transformation)
			policyName = strings.ReplaceAll(policyName, "_", "-")
			log.Printf("DEBUG: Found user metric: %s, extracted policy: %s", metric.Name, policyName)
			
			if policyName == "" {
				log.Printf("DEBUG: Skipping metric - no policy extracted: %s", metric.Name)
				continue
			}

			if _, exists := policyMap[policyName]; !exists {
				log.Printf("DEBUG: Creating new policy entry for: %s", policyName)
				policyMap[policyName] = &types.TeamUserUsage{
					TeamID:   policyName, // Will be mapped to actual team later
					TeamName: policyName, // Will be enriched later
					Policy:   policyName,
				}
			}

			switch {
			case strings.Contains(metric.Name, "token_usage_with_user_and_group"):
				log.Printf("DEBUG: Adding token usage: %d to policy %s", metric.Value, policyName)
				policyMap[policyName].TokenUsage += metric.Value
				userUsage.TotalTokenUsage += metric.Value
			case strings.Contains(metric.Name, "authorized_calls_with_user_and_group"):
				log.Printf("DEBUG: Adding authorized calls: %d to policy %s", metric.Value, policyName)
				policyMap[policyName].AuthorizedCalls += metric.Value
				userUsage.TotalAuthorizedCalls += metric.Value
			case strings.Contains(metric.Name, "limited_calls_with_user_and_group"):
				log.Printf("DEBUG: Adding limited calls: %d to policy %s", metric.Value, policyName)
				policyMap[policyName].LimitedCalls += metric.Value
				userUsage.TotalLimitedCalls += metric.Value
			}
		}
	}
	
	log.Printf("DEBUG: Found %d user metrics for %s:", len(userMetricsFound), userID)
	for i, metric := range userMetricsFound {
		if i < 3 { // Show first 3 user metrics
			log.Printf("DEBUG: User metric %d: %s", i, metric)
		}
	}
	log.Printf("DEBUG: Created %d policy entries", len(policyMap))

	for _, teamUsage := range policyMap {
		userUsage.TeamBreakdown = append(userUsage.TeamBreakdown, *teamUsage)
	}

	return userUsage, nil
}

// GetTeamUsage collects and aggregates usage data for a specific team
// teamID is the actual team identifier, but we need to look up its policy first
func (c *Collector) GetTeamUsage(teamID string, policyName string) (*types.TeamUsage, error) {
	metrics, err := c.collectPrometheusMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	teamUsage := &types.TeamUsage{
		TeamID:        teamID,
		TeamName:      teamID, // Will be enriched later
		Policy:        policyName,
		UserBreakdown: []types.UserTeamUsage{},
		LastUpdated:   time.Now(),
	}

	userMap := make(map[string]*types.UserTeamUsage)
	var teamMetricsFound []string

	// Convert hyphens to underscores for metrics lookup (Kuadrant/Envoy converts hyphens to underscores)
	metricsPolicy := strings.ReplaceAll(policyName, "-", "_")
	
	log.Printf("DEBUG: GetTeamUsage called for teamID: %s, policyName: %s", teamID, policyName)
	log.Printf("DEBUG: Looking for team metrics containing: group___%s___ (converted from %s)", metricsPolicy, policyName)

	for _, metric := range metrics {
		if strings.Contains(metric.Name, fmt.Sprintf("group___%s___", metricsPolicy)) {
			teamMetricsFound = append(teamMetricsFound, fmt.Sprintf("%s = %d", metric.Name, metric.Value))
			userID := extractUserFromMetric(metric.Name)
			log.Printf("DEBUG: Found team metric: %s, extracted user: %s", metric.Name, userID)
			
			if userID == "" {
				log.Printf("DEBUG: Skipping metric - no user extracted: %s", metric.Name)
				continue
			}

			if _, exists := userMap[userID]; !exists {
				userMap[userID] = &types.UserTeamUsage{
					UserID:    userID,
					UserEmail: fmt.Sprintf("%s@company.com", userID), // Will be enriched later
				}
			}

			switch {
			case strings.Contains(metric.Name, "token_usage_with_user_and_group"):
				userMap[userID].TokenUsage += metric.Value
				teamUsage.TotalTokenUsage += metric.Value
			case strings.Contains(metric.Name, "authorized_calls_with_user_and_group"):
				userMap[userID].AuthorizedCalls += metric.Value
				teamUsage.TotalAuthorizedCalls += metric.Value
			case strings.Contains(metric.Name, "limited_calls_with_user_and_group"):
				userMap[userID].LimitedCalls += metric.Value
				teamUsage.TotalLimitedCalls += metric.Value
			}
		}
	}

	log.Printf("DEBUG: Found %d team metrics for policy %s:", len(teamMetricsFound), policyName)
	for i, metric := range teamMetricsFound {
		if i < 3 { // Show first 3 team metrics
			log.Printf("DEBUG: Team metric %d: %s", i, metric)
		}
	}
	log.Printf("DEBUG: Created %d user entries for team", len(userMap))

	for _, userUsage := range userMap {
		teamUsage.UserBreakdown = append(teamUsage.UserBreakdown, *userUsage)
	}

	return teamUsage, nil
}

// collectPrometheusMetrics makes HTTP request directly to istio-proxy metrics endpoint
func (c *Collector) collectPrometheusMetrics() ([]types.PrometheusMetric, error) {
	log.Printf("Fetching metrics from: %s", c.metricsURL)
	
	resp, err := c.httpClient.Get(c.metricsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from %s: %w", c.metricsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("Successfully fetched %d bytes of metrics data", len(body))
	
	// Debug: show first few lines of metrics
	lines := strings.Split(string(body), "\n")
	log.Printf("DEBUG: First 10 lines of metrics data:")
	for i, line := range lines {
		if i >= 10 {
			break
		}
		log.Printf("DEBUG: Line %d: %s", i, line)
	}
	
	return c.parsePrometheusOutput(string(body))
}

// parsePrometheusOutput parses the Prometheus metrics output
func (c *Collector) parsePrometheusOutput(output string) ([]types.PrometheusMetric, error) {
	var metrics []types.PrometheusMetric
	scanner := bufio.NewScanner(strings.NewReader(output))

	var currentHelp, currentType string
	var tokenMetricsFound []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if line == "" || strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# HELP") {
				currentHelp = strings.TrimPrefix(line, "# HELP ")
			} else if strings.HasPrefix(line, "# TYPE") {
				currentType = strings.TrimPrefix(line, "# TYPE ")
			}
			continue
		}

		// Parse metric line: metric_name{labels} value
		if strings.Contains(line, "token") || strings.Contains(line, "authorized_calls") || strings.Contains(line, "limited_calls") {
			tokenMetricsFound = append(tokenMetricsFound, line)
			metric := c.parseMetricLine(line, currentHelp, currentType)
			if metric != nil {
				metrics = append(metrics, *metric)
			} else {
				log.Printf("DEBUG: Failed to parse token metric line: %s", line)
			}
		}
	}

	log.Printf("DEBUG: Found %d token-related metrics:", len(tokenMetricsFound))
	for i, metric := range tokenMetricsFound {
		if i < 10 { // Show first 10 token metrics
			log.Printf("DEBUG: Token metric %d: %s", i, metric)
		}
	}
	
	// Show specific metrics we're looking for
	log.Printf("DEBUG: Searching for specific testuser token metrics...")
	for _, metric := range tokenMetricsFound {
		if strings.Contains(metric, "token_usage_with_user_and_group__user___testuser") {
			log.Printf("DEBUG: Found testuser token usage: %s", metric)
		}
	}
	
	log.Printf("DEBUG: Parsed %d total metrics from output", len(metrics))
	return metrics, scanner.Err()
}

// parseMetricLine parses a single metric line
func (c *Collector) parseMetricLine(line, help, metricType string) *types.PrometheusMetric {
	// Match pattern: metric_name{labels} value
	re := regexp.MustCompile(`^([^{]+)(\{[^}]*\})?\s+(.+)$`)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 4 {
		log.Printf("DEBUG: parseMetricLine - regex didn't match line: %s", line)
		return nil
	}

	name := matches[1]
	labelsStr := matches[2]
	valueStr := matches[3]

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		log.Printf("DEBUG: parseMetricLine - failed to parse value '%s' from line: %s, error: %v", valueStr, line, err)
		return nil
	}

	labels := make(map[string]string)
	if labelsStr != "" {
		// Simple label parsing for our use case
		labelsStr = strings.Trim(labelsStr, "{}")
		// For now, we'll extract user and group from the metric name itself
		// since the format is embedded in the name
	}

	log.Printf("DEBUG: parseMetricLine - successfully parsed: name=%s, value=%d", name, value)
	return &types.PrometheusMetric{
		Name:   name,
		Labels: labels,
		Value:  value,
		Help:   help,
		Type:   metricType,
	}
}

// extractUserFromMetric extracts user ID from metric name
// Example: token_usage_with_user_and_group__user___testuser___group___test_tokens___namespace__llm
func extractUserFromMetric(metricName string) string {
	re := regexp.MustCompile(`__user___(.+?)___group__`)
	matches := re.FindStringSubmatch(metricName)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractPolicyFromMetric extracts policy (group) name from metric name
// Example: token_usage_with_user_and_group__user___testuser___group___test_tokens___namespace__llm
func extractPolicyFromMetric(metricName string) string {
	re := regexp.MustCompile(`__group___(.+?)___namespace__`)
	matches := re.FindStringSubmatch(metricName)
	if len(matches) >= 2 {
		log.Printf("DEBUG: extractPolicyFromMetric - extracted '%s' from '%s'", matches[1], metricName)
		return matches[1]
	}
	log.Printf("DEBUG: extractPolicyFromMetric - no match for '%s'", metricName)
	return ""
}