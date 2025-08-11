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

// PrometheusTLSConfig holds TLS configuration for Prometheus client connections
type PrometheusTLSConfig struct {
	// EnableTLS enables TLS encryption for Prometheus connections
	EnableTLS bool `json:"enableTLS"`

	// InsecureSkipVerify skips certificate verification (not recommended for production)
	InsecureSkipVerify bool `json:"insecureSkipVerify"`

	// CACertPath path to CA certificate file
	CACertPath string `json:"caCertPath,omitempty"`

	// ClientCertPath path to client certificate file
	ClientCertPath string `json:"clientCertPath,omitempty"`

	// ClientKeyPath path to client private key file
	ClientKeyPath string `json:"clientKeyPath,omitempty"`

	// ServerName for certificate validation
	ServerName string `json:"serverName,omitempty"`
}

// PrometheusConfig holds complete Prometheus client configuration
type PrometheusConfig struct {
	// BaseURL is the Prometheus server URL
	BaseURL string `json:"baseURL"`

	// TLS configuration for secure connections
	TLS *PrometheusTLSConfig `json:"tls,omitempty"`

	// BearerToken for authentication
	BearerToken string `json:"bearerToken,omitempty"`

	// Timeout for API requests
	Timeout time.Duration `json:"timeout,omitempty"`
}
