package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/inferno-autoscaler/internal/logger"
)

// CreateTLSConfig creates a TLS configuration from PrometheusTLSConfig
func CreateTLSConfig(tlsConfig *interfaces.PrometheusTLSConfig) (*tls.Config, error) {
	if tlsConfig == nil || !tlsConfig.EnableTLS {
		return nil, nil
	}

	config := &tls.Config{
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ServerName:         tlsConfig.ServerName,
		MinVersion:         tls.VersionTLS12, // Enforce minimum TLS version
	}

	// Load CA certificate if provided
	if tlsConfig.CACertPath != "" {
		caCert, err := os.ReadFile(tlsConfig.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate from %s: %w", tlsConfig.CACertPath, err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", tlsConfig.CACertPath)
		}
		config.RootCAs = caCertPool
		logger.Log.Info("CA certificate loaded successfully", "path", tlsConfig.CACertPath)
	}

	// Load client certificate and key if provided
	if tlsConfig.ClientCertPath != "" && tlsConfig.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(tlsConfig.ClientCertPath, tlsConfig.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate from %s and key from %s: %w",
				tlsConfig.ClientCertPath, tlsConfig.ClientKeyPath, err)
		}
		config.Certificates = []tls.Certificate{cert}
		logger.Log.Info("Client certificate loaded successfully",
			"cert_path", tlsConfig.ClientCertPath, "key_path", tlsConfig.ClientKeyPath)
	}

	return config, nil
}

// ValidateTLSConfig validates TLS configuration
func ValidateTLSConfig(tlsConfig *interfaces.PrometheusTLSConfig) error {
	if tlsConfig == nil || !tlsConfig.EnableTLS {
		return nil
	}

	// Check if certificate files exist
	if tlsConfig.CACertPath != "" {
		if _, err := os.Stat(tlsConfig.CACertPath); os.IsNotExist(err) {
			return fmt.Errorf("CA certificate file not found: %s", tlsConfig.CACertPath)
		}
	}

	if tlsConfig.ClientCertPath != "" {
		if _, err := os.Stat(tlsConfig.ClientCertPath); os.IsNotExist(err) {
			return fmt.Errorf("client certificate file not found: %s", tlsConfig.ClientCertPath)
		}
	}

	if tlsConfig.ClientKeyPath != "" {
		if _, err := os.Stat(tlsConfig.ClientKeyPath); os.IsNotExist(err) {
			return fmt.Errorf("client key file not found: %s", tlsConfig.ClientKeyPath)
		}
	}

	// Warn about insecure configuration
	if tlsConfig.InsecureSkipVerify {
		logger.Log.Warn("TLS certificate verification is disabled - this is not recommended for production")
	}

	return nil
}

// ParsePrometheusConfigFromEnv parses Prometheus configuration from environment variables
func ParsePrometheusConfigFromEnv() *interfaces.PrometheusConfig {
	config := &interfaces.PrometheusConfig{
		BaseURL: getEnvOrDefault("PROMETHEUS_BASE_URL", "http://prometheus:9090"),
		Timeout: 30 * time.Second,
	}

	// Check if TLS is enabled (HTTPS URL or explicit flag)
	enableTLS := getEnvOrDefault("PROMETHEUS_TLS_ENABLED", "") == "true" ||
		(len(config.BaseURL) > 8 && config.BaseURL[:8] == "https://")

	if enableTLS {
		config.TLS = &interfaces.PrometheusTLSConfig{
			EnableTLS:          true,
			InsecureSkipVerify: getEnvOrDefault("PROMETHEUS_TLS_INSECURE_SKIP_VERIFY", "false") == "true",
			CACertPath:         getEnvOrDefault("PROMETHEUS_CA_CERT_PATH", ""),
			ClientCertPath:     getEnvOrDefault("PROMETHEUS_CLIENT_CERT_PATH", ""),
			ClientKeyPath:      getEnvOrDefault("PROMETHEUS_CLIENT_KEY_PATH", ""),
			ServerName:         getEnvOrDefault("PROMETHEUS_SERVER_NAME", ""),
		}
	}

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
