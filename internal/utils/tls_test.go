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
	logger.InitLogger()
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
			name: "TLS disabled",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS: false,
			},
			expectError: false,
		},
		{
			name: "TLS enabled with insecure skip verify",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS:          true,
				InsecureSkipVerify: true,
			},
			expectError: false,
		},
		{
			name: "TLS enabled with server name",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS:  true,
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
			if tt.promConfig != nil && tt.promConfig.EnableTLS {
				assert.NotNil(t, config)
			} else {
				assert.Nil(t, config)
			}
		})
	}
}

func TestParsePrometheusConfigFromEnv(t *testing.T) {
	// Test with HTTP URL
	os.Setenv("PROMETHEUS_BASE_URL", "http://prometheus:9090")
	os.Setenv("PROMETHEUS_TLS_ENABLED", "false")

	config := ParsePrometheusConfigFromEnv()
	assert.Equal(t, "http://prometheus:9090", config.BaseURL)
	assert.False(t, config.EnableTLS)

	// Test with HTTPS URL
	os.Setenv("PROMETHEUS_BASE_URL", "https://prometheus:9090")
	os.Setenv("PROMETHEUS_TLS_ENABLED", "true")
	os.Setenv("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "true")

	config = ParsePrometheusConfigFromEnv()
	assert.Equal(t, "https://prometheus:9090", config.BaseURL)
	assert.True(t, config.EnableTLS)
	assert.True(t, config.InsecureSkipVerify)

	// Test OpenShift configuration
	os.Setenv("PROMETHEUS_BASE_URL", "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091")
	os.Setenv("PROMETHEUS_TLS_ENABLED", "true")
	os.Setenv("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "false")
	os.Setenv("PROMETHEUS_CA_CERT_PATH", "/etc/openshift-ca/ca.crt")
	os.Setenv("PROMETHEUS_CLIENT_CERT_PATH", "")
	os.Setenv("PROMETHEUS_CLIENT_KEY_PATH", "")
	os.Setenv("PROMETHEUS_SERVER_NAME", "thanos-querier.openshift-monitoring.svc")
	os.Setenv("PROMETHEUS_TOKEN_PATH", "/var/run/secrets/kubernetes.io/serviceaccount/token")

	config = ParsePrometheusConfigFromEnv()
	assert.Equal(t, "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091", config.BaseURL)
	assert.True(t, config.EnableTLS)
	assert.False(t, config.InsecureSkipVerify)
	assert.Equal(t, "/etc/openshift-ca/ca.crt", config.CACertPath)
	assert.Equal(t, "", config.ClientCertPath)
	assert.Equal(t, "", config.ClientKeyPath)
	assert.Equal(t, "thanos-querier.openshift-monitoring.svc", config.ServerName)
	assert.Equal(t, "/var/run/secrets/kubernetes.io/serviceaccount/token", config.TokenPath)

	// Clean up
	os.Unsetenv("PROMETHEUS_BASE_URL")
	os.Unsetenv("PROMETHEUS_TLS_ENABLED")
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
			expectError: false,
		},
		{
			name: "TLS disabled",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS: false,
			},
			expectError: false,
		},
		{
			name: "TLS enabled with insecure skip verify",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS:          true,
				InsecureSkipVerify: true,
			},
			expectError: false,
		},
		{
			name: "TLS enabled with non-existent CA cert",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS:  true,
				CACertPath: "/non/existent/path",
			},
			expectError: true,
		},
		{
			name: "OpenShift TLS configuration",
			promConfig: &interfaces.PrometheusConfig{
				EnableTLS:          true,
				InsecureSkipVerify: false,
				CACertPath:         "/etc/openshift-ca/ca.crt",
				ServerName:         "thanos-querier.openshift-monitoring.svc",
			},
			expectError: true, // Will fail because CA cert doesn't exist in test environment
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
