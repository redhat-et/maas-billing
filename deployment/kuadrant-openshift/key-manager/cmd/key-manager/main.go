package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/auth"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/config"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/handlers"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/keys"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/models"
	"github.com/redhat-et/maas-billing/deployment/kuadrant-openshift/key-manager-v2/internal/teams"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Create in-cluster config
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create in-cluster config: %v", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %v", err)
	}

	// Create dynamic client for Kuadrant CRDs
	kuadrantClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("Failed to create dynamic client: %v", err)
	}

	// Initialize managers
	policyMgr := teams.NewPolicyManager(
		kuadrantClient,
		clientset,
		cfg.KeyNamespace,
		cfg.TokenRateLimitPolicyName,
		cfg.AuthPolicyName,
	)

	teamMgr := teams.NewManager(clientset, cfg.KeyNamespace, policyMgr)
	keyMgr := keys.NewManager(clientset, cfg.KeyNamespace, teamMgr)
	modelMgr := models.NewManager(kuadrantClient)

	// Initialize handlers
	usageHandler := handlers.NewUsageHandler(clientset, restConfig, cfg.KeyNamespace)
	teamsHandler := handlers.NewTeamsHandler(teamMgr)
	keysHandler := handlers.NewKeysHandler(keyMgr, teamMgr)
	modelsHandler := handlers.NewModelsHandler(modelMgr)
	legacyHandler := handlers.NewLegacyHandler(keyMgr)
	healthHandler := handlers.NewHealthHandler()

	// Create default team if enabled
	if cfg.CreateDefaultTeam {
		if err := teamMgr.CreateDefaultTeam(); err != nil {
			log.Printf("Warning: Failed to create default team: %v", err)
		} else {
			log.Printf("Default team created successfully")
		}
	}

	// Initialize Gin router
	r := gin.Default()

	// Health check endpoint (no auth required)
	r.GET("/health", healthHandler.HealthCheck)

	// Setup API routes with admin authentication
	adminRoutes := r.Group("/", auth.AdminAuthMiddleware())

	// Legacy endpoints (backward compatibility)
	adminRoutes.POST("/generate_key", legacyHandler.GenerateKey)
	adminRoutes.DELETE("/delete_key", legacyHandler.DeleteKey)

	// Team management endpoints
	adminRoutes.POST("/teams", teamsHandler.CreateTeam)
	adminRoutes.GET("/teams", teamsHandler.ListTeams)
	adminRoutes.GET("/teams/:team_id", teamsHandler.GetTeam)
	adminRoutes.PATCH("/teams/:team_id", teamsHandler.UpdateTeam)
	adminRoutes.DELETE("/teams/:team_id", teamsHandler.DeleteTeam)

	// Team-scoped API key management
	adminRoutes.POST("/teams/:team_id/keys", keysHandler.CreateTeamKey)
	adminRoutes.GET("/teams/:team_id/keys", keysHandler.ListTeamKeys)
	adminRoutes.DELETE("/keys/:key_name", keysHandler.DeleteTeamKey)

	// User key management
	adminRoutes.GET("/users/:user_id/keys", keysHandler.ListUserKeys)

	// Usage endpoints
	adminRoutes.GET("/users/:user_id/usage", usageHandler.GetUserUsage)
	adminRoutes.GET("/teams/:team_id/usage", usageHandler.GetTeamUsage)

	// Model listing endpoint
	adminRoutes.GET("/models", modelsHandler.ListModels)

	// Start server
	log.Printf("Starting %s on port %s", cfg.ServiceName, cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
