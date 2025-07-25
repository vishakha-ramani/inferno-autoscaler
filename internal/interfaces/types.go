package controller

import inferno "github.com/llm-inferno/optimizer-light/pkg/core"

// Captures response from ModelAnalyzer(s) per model
type ModelAnalyzeResponse struct {
	// feasible allocations for all accelerators
	Allocations map[string]*ModelAcceleratorAllocation // accelerator name -> allocation
}

// Allocation details of an accelerator to a variant
type ModelAcceleratorAllocation struct {
	Allocation *inferno.Allocation // allocation result of model analyzer

	RequiredPrefillQPS float64
	RequiredDecodeQPS  float64
	Reason             string
}

type ServiceClassEntry struct {
	Model  string `yaml:"model"`
	SLOITL int    `yaml:"slo-itl"`
	SLOTTW int    `yaml:"slo-ttw"`
}

type ServiceClass struct {
	Name     string              `yaml:"name"`
	Priority int                 `yaml:"priority"`
	Data     []ServiceClassEntry `yaml:"data"`
}
