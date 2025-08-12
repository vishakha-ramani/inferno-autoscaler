package utils

import "time"

// Default timeout and connection constants used across the codebase
const (
	// DefaultTimeout is the default timeout for HTTP requests and operations
	DefaultTimeout = 30 * time.Second

	// DefaultKeepAlive is the default keep-alive duration for HTTP connections
	DefaultKeepAlive = 30 * time.Second

	// DefaultMaxIdleConns is the default maximum number of idle connections per host
	// This is the same as Go's http.Transport default, suitable for HTTP/2 multiplexing
	DefaultMaxIdleConns = 100

	// DefaultIdleConnTimeout is the default timeout for idle connections
	DefaultIdleConnTimeout = 90 * time.Second

	// DefaultTLSHandshakeTimeout is the default timeout for TLS handshakes
	DefaultTLSHandshakeTimeout = 10 * time.Second

	// DefaultExpectContinueTimeout is the default timeout for expect-continue responses
	DefaultExpectContinueTimeout = 1 * time.Second
)
