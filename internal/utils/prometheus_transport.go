package utils

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	"github.com/llm-d-incubation/inferno-autoscaler/internal/logger"
	"github.com/prometheus/client_golang/api"
)

// CreatePrometheusTransport creates a custom HTTP transport for Prometheus client with TLS support
func CreatePrometheusTransport(config *interfaces.PrometheusConfig) (http.RoundTripper, error) {
	// Create base transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Configure TLS if enabled
	if config.TLS != nil && config.TLS.EnableTLS {
		tlsConfig, err := CreateTLSConfig(config.TLS)
		if err != nil {
			return nil, err
		}
		if tlsConfig != nil {
			transport.TLSClientConfig = tlsConfig
			logger.Log.Info("TLS configuration applied to Prometheus transport")
		}
	}

	return transport, nil
}

// CreatePrometheusClientConfig creates a complete Prometheus client configuration
func CreatePrometheusClientConfig(config *interfaces.PrometheusConfig) (*api.Config, error) {
	clientConfig := &api.Config{
		Address: config.BaseURL,
	}

	// Create custom transport with TLS support
	transport, err := CreatePrometheusTransport(config)
	if err != nil {
		return nil, err
	}

	// Add bearer token authentication if provided
	bearerToken := config.BearerToken

	// If no direct bearer token but token path is provided, read from file
	if bearerToken == "" && config.TokenPath != "" {
		tokenBytes, err := os.ReadFile(config.TokenPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read bearer token from %s: %w", config.TokenPath, err)
		}
		bearerToken = strings.TrimSpace(string(tokenBytes))
		logger.Log.Info("Bearer token loaded from file", "path", config.TokenPath)
	}

	if bearerToken != "" {
		// Create a custom round tripper that adds the bearer token
		transport = &bearerTokenRoundTripper{
			base:  transport,
			token: bearerToken,
		}
	}

	clientConfig.RoundTripper = transport

	return clientConfig, nil
}

// bearerTokenRoundTripper adds bearer token authentication to requests
type bearerTokenRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (b *bearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(req)
}
