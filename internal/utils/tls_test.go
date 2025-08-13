package utils

import (
	"os"
	"testing"

	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/inferno-autoscaler/internal/logger"
	"github.com/stretchr/testify/assert"
)

func init() {
	// Initialize logger for tests
	_, err := logger.InitLogger()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
}

func TestCreateTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		promConfig  *interfaces.PrometheusConfig
		expectError bool
	}{
		{
			name:        "nil config",
			promConfig:  nil,
			expectError: false,
		},
		{
			name: "TLS with insecure skip verify",
			promConfig: &interfaces.PrometheusConfig{
				InsecureSkipVerify: true,
			},
			expectError: false,
		},
		{
			name: "TLS with server name",
			promConfig: &interfaces.PrometheusConfig{
				ServerName: "prometheus.example.com",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := CreateTLSConfig(tt.promConfig)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.promConfig != nil {
				assert.NotNil(t, config)
			} else {
				assert.Nil(t, config)
			}
		})
	}
}

func TestParsePrometheusConfigFromEnv(t *testing.T) {
	// Test with HTTPS URL (default)
	os.Setenv("PROMETHEUS_BASE_URL", "https://prometheus:9090")

	config := ParsePrometheusConfigFromEnv()
	assert.Equal(t, "https://prometheus:9090", config.BaseURL)

	// Test with explicit TLS configuration
	os.Setenv("PROMETHEUS_BASE_URL", "https://prometheus:9090")
	os.Setenv("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "true")

	config = ParsePrometheusConfigFromEnv()
	assert.Equal(t, "https://prometheus:9090", config.BaseURL)
	assert.True(t, config.InsecureSkipVerify)

	// Test OpenShift configuration
	os.Setenv("PROMETHEUS_BASE_URL", "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091")
	os.Setenv("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "false")
	os.Setenv("PROMETHEUS_CA_CERT_PATH", "/etc/openshift-ca/ca.crt")
	os.Setenv("PROMETHEUS_CLIENT_CERT_PATH", "")
	os.Setenv("PROMETHEUS_CLIENT_KEY_PATH", "")
	os.Setenv("PROMETHEUS_SERVER_NAME", "thanos-querier.openshift-monitoring.svc")
	os.Setenv("PROMETHEUS_TOKEN_PATH", "/var/run/secrets/kubernetes.io/serviceaccount/token")

	config = ParsePrometheusConfigFromEnv()
	assert.Equal(t, "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091", config.BaseURL)
	assert.False(t, config.InsecureSkipVerify)
	assert.Equal(t, "/etc/openshift-ca/ca.crt", config.CACertPath)
	assert.Equal(t, "", config.ClientCertPath)
	assert.Equal(t, "", config.ClientKeyPath)
	assert.Equal(t, "thanos-querier.openshift-monitoring.svc", config.ServerName)
	assert.Equal(t, "/var/run/secrets/kubernetes.io/serviceaccount/token", config.TokenPath)

	// Clean up
	os.Unsetenv("PROMETHEUS_BASE_URL")
	os.Unsetenv("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY")
	os.Unsetenv("PROMETHEUS_CA_CERT_PATH")
	os.Unsetenv("PROMETHEUS_CLIENT_CERT_PATH")
	os.Unsetenv("PROMETHEUS_CLIENT_KEY_PATH")
	os.Unsetenv("PROMETHEUS_SERVER_NAME")
	os.Unsetenv("PROMETHEUS_TOKEN_PATH")
}

func TestValidateTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		promConfig  *interfaces.PrometheusConfig
		expectError bool
	}{
		{
			name:        "nil config",
			promConfig:  nil,
			expectError: true,
		},
		{
			name: "HTTP URL - should fail",
			promConfig: &interfaces.PrometheusConfig{
				BaseURL: "http://prometheus:9090",
			},
			expectError: true,
		},
		{
			name: "TLS with insecure skip verify",
			promConfig: &interfaces.PrometheusConfig{
				InsecureSkipVerify: true,
				BaseURL:            "https://prometheus:9090",
			},
			expectError: false,
		},
		{
			name: "TLS with server name",
			promConfig: &interfaces.PrometheusConfig{
				ServerName: "prometheus.example.com",
				BaseURL:    "https://prometheus:9090",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTLSConfig(tt.promConfig)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsHTTPS(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "valid HTTPS URL",
			url:      "https://prometheus:9090",
			expected: true,
		},
		{
			name:     "valid HTTPS URL with path",
			url:      "https://prometheus:9090/api/v1/query",
			expected: true,
		},
		{
			name:     "invalid URL",
			url:      "not-a-url",
			expected: false,
		},
		{
			name:     "empty URL",
			url:      "",
			expected: false,
		},
		{
			name:     "URL with different scheme",
			url:      "http://prometheus:9090",
			expected: false,
		},
		{
			name:     "URL with uppercase HTTPS",
			url:      "HTTPS://prometheus:9090",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHTTPS(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
