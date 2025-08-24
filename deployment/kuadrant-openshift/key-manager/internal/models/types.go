package models

// Model listing structures
type ModelInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url"`
	Ready     bool   `json:"ready"`
}

type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}