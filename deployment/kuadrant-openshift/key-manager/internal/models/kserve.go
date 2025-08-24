package models

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Manager handles model discovery and listing
type Manager struct {
	kuadrantClient dynamic.Interface
}

// NewManager creates a new model manager
func NewManager(kuadrantClient dynamic.Interface) *Manager {
	return &Manager{
		kuadrantClient: kuadrantClient,
	}
}

// ListAvailableModels lists all InferenceServices across all namespaces
func (m *Manager) ListAvailableModels() ([]ModelInfo, error) {
	// Define InferenceService GVR
	inferenceServiceGVR := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	log.Printf("DEBUG: Attempting to list InferenceServices with GVR: %+v", inferenceServiceGVR)

	// List all InferenceServices across all namespaces
	list, err := m.kuadrantClient.Resource(inferenceServiceGVR).List(
		context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("DEBUG: Failed to list InferenceServices: %v", err)
		return nil, fmt.Errorf("failed to list InferenceServices: %w", err)
	}

	log.Printf("DEBUG: Found %d InferenceServices", len(list.Items))

	var modelList []ModelInfo

	for _, item := range list.Items {
		model := ModelInfo{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Ready:     false,
		}

		// Extract URL from status
		if status, ok := item.Object["status"].(map[string]interface{}); ok {
			if url, ok := status["url"].(string); ok {
				model.URL = url
			}

			// Check if ready - look for conditions
			if conditions, ok := status["conditions"].([]interface{}); ok {
				for _, condition := range conditions {
					if condMap, ok := condition.(map[string]interface{}); ok {
						if condType, ok := condMap["type"].(string); ok && condType == "Ready" {
							if condStatus, ok := condMap["status"].(string); ok && condStatus == "True" {
								model.Ready = true
								break
							}
						}
					}
				}
			}
		}

		modelList = append(modelList, model)
	}

	return modelList, nil
}