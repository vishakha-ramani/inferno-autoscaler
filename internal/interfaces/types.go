package controller

import (
	"time"

	inferno "github.com/llm-inferno/optimizer-light/pkg/core"
)

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
	Model   string `yaml:"model"`
	SLOTPOT int    `yaml:"slo-tpot"`
	SLOTTFT int    `yaml:"slo-ttft"`
}

type ServiceClass struct {
	Name     string              `yaml:"name"`
	Priority int                 `yaml:"priority"`
	Data     []ServiceClassEntry `yaml:"data"`
}

// PrometheusConfig holds complete Prometheus client configuration including TLS settings
type PrometheusConfig struct {
	// BaseURL is the Prometheus server URL
	BaseURL string `json:"baseURL"`

	// TLS configuration fields
	EnableTLS          bool   `json:"enableTLS,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
	CACertPath         string `json:"caCertPath,omitempty"`
	ClientCertPath     string `json:"clientCertPath,omitempty"`
	ClientKeyPath      string `json:"clientKeyPath,omitempty"`
	ServerName         string `json:"serverName,omitempty"`

	// Authentication fields
	BearerToken string `json:"bearerToken,omitempty"`
	TokenPath   string `json:"tokenPath,omitempty"`

	// Timeout for API requests
	Timeout time.Duration `json:"timeout,omitempty"`
}
