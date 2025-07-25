package controller

import (
	"fmt"

	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	"gopkg.in/yaml.v3"
)

// Helper to find SLOs for a model variant
func findModelSLO(cmData map[string]string, targetModel string) (*interfaces.ServiceClassEntry, string /* class name */, error) {
	for key, val := range cmData {
		var sc interfaces.ServiceClass
		if err := yaml.Unmarshal([]byte(val), &sc); err != nil {
			return nil, "", fmt.Errorf("failed to parse %s: %w", key, err)
		}

		for _, entry := range sc.Data {
			if entry.Model == targetModel {
				return &entry, sc.Name, nil
			}
		}
	}
	return nil, "", fmt.Errorf("model %q not found in any service class", targetModel)
}

func ptr[T any](v T) *T {
	return &v
}
