package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"

	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/inferno-autoscaler/internal/logger"
)

// CreateTLSConfig creates a TLS configuration from PrometheusConfig
func CreateTLSConfig(promConfig *interfaces.PrometheusConfig) (*tls.Config, error) {
	if promConfig == nil || !promConfig.EnableTLS {
		return nil, nil
	}

	config := &tls.Config{
		InsecureSkipVerify: promConfig.InsecureSkipVerify,
		ServerName:         promConfig.ServerName,
		MinVersion:         tls.VersionTLS12, // Enforce minimum TLS version
	}

	// Load CA certificate if provided
	if promConfig.CACertPath != "" {
		caCert, err := os.ReadFile(promConfig.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate from %s: %w", promConfig.CACertPath, err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", promConfig.CACertPath)
		}
		config.RootCAs = caCertPool
		logger.Log.Info("CA certificate loaded successfully", "path", promConfig.CACertPath)
	}

	// Load client certificate and key if provided
	if promConfig.ClientCertPath != "" && promConfig.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(promConfig.ClientCertPath, promConfig.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate from %s and key from %s: %w",
				promConfig.ClientCertPath, promConfig.ClientKeyPath, err)
		}
		config.Certificates = []tls.Certificate{cert}
		logger.Log.Info("Client certificate loaded successfully",
			"cert_path", promConfig.ClientCertPath, "key_path", promConfig.ClientKeyPath)
	}

	return config, nil
}

// ValidateTLSConfig validates TLS configuration
func ValidateTLSConfig(promConfig *interfaces.PrometheusConfig) error {
	if promConfig == nil || !promConfig.EnableTLS {
		return nil
	}

	// If InsecureSkipVerify is true, we don't need to validate certificate files
	// since we're intentionally skipping certificate verification
	if promConfig.InsecureSkipVerify {
		logger.Log.Warn("TLS certificate verification is disabled - this is not recommended for production")
		return nil
	}

	// Check if certificate files exist (only when not skipping verification)
	if promConfig.CACertPath != "" {
		if _, err := os.Stat(promConfig.CACertPath); os.IsNotExist(err) {
			return fmt.Errorf("CA certificate file not found: %s", promConfig.CACertPath)
		}
	}

	if promConfig.ClientCertPath != "" {
		if _, err := os.Stat(promConfig.ClientCertPath); os.IsNotExist(err) {
			return fmt.Errorf("client certificate file not found: %s", promConfig.ClientCertPath)
		}
	}

	if promConfig.ClientKeyPath != "" {
		if _, err := os.Stat(promConfig.ClientKeyPath); os.IsNotExist(err) {
			return fmt.Errorf("client key file not found: %s", promConfig.ClientKeyPath)
		}
	}

	return nil
}

// ParsePrometheusConfigFromEnv parses Prometheus configuration from environment variables
func ParsePrometheusConfigFromEnv() *interfaces.PrometheusConfig {
	config := &interfaces.PrometheusConfig{
		BaseURL: getEnvOrDefault("PROMETHEUS_BASE_URL", "https://prometheus:9090"),
		Timeout: DefaultTimeout,
	}

	// Enable TLS based on environment variable, default to true for HTTPS-only support
	config.EnableTLS = getEnvOrDefault("PROMETHEUS_TLS_ENABLED", "true") == "true"
	config.InsecureSkipVerify = getEnvOrDefault("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "false") == "true"
	config.CACertPath = getEnvOrDefault("PROMETHEUS_CA_CERT_PATH", "")
	config.ClientCertPath = getEnvOrDefault("PROMETHEUS_CLIENT_CERT_PATH", "")
	config.ClientKeyPath = getEnvOrDefault("PROMETHEUS_CLIENT_KEY_PATH", "")
	config.ServerName = getEnvOrDefault("PROMETHEUS_SERVER_NAME", "")

	// Support both direct bearer token and token path
	config.BearerToken = getEnvOrDefault("PROMETHEUS_BEARER_TOKEN", "")
	config.TokenPath = getEnvOrDefault("PROMETHEUS_TOKEN_PATH", "")

	return config
}

// getEnvOrDefault gets environment variable value or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsHTTPS checks if the given URL string uses HTTPS scheme
func IsHTTPS(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Scheme == "https"
}

// GetScheme extracts the scheme from a URL string
func GetScheme(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL %s: %w", urlStr, err)
	}
	return u.Scheme, nil
}
