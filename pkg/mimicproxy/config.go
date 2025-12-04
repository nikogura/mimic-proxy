package mimicproxy

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// SchemeHTTP is the HTTP URL scheme.
	SchemeHTTP = "http"
	// SchemeHTTPS is the HTTPS URL scheme.
	SchemeHTTPS = "https"
)

// Config represents the complete proxy configuration.
type Config struct {
	// Routes defines the mapping from incoming paths to upstream servers
	Routes []*RouteConfig

	// Transport configuration
	Transport TransportConfig

	// TLS configuration
	TLS TLSConfig

	// Metrics configuration
	Metrics MetricsConfig

	// Logger configuration
	Logger LoggerConfig
}

// RouteConfig defines a single route from client path to upstream.
type RouteConfig struct {
	// Name is a human-readable identifier for this route (for metrics/logging)
	Name string

	// PathPrefix is the incoming request path prefix to match (e.g., "/v1/verify")
	PathPrefix string

	// Upstream is the target server (e.g., "https://api.aiprise.com")
	Upstream string

	// UpstreamPathPrefix is the path prefix to use on the upstream server
	// If empty, uses PathPrefix. If set, rewrites the path.
	// Example: PathPrefix="/v1/verify", UpstreamPathPrefix="/api/v1/verify"
	UpstreamPathPrefix string

	// PreserveHost controls whether to preserve the incoming Host header
	// or replace it with the upstream host. Default: false (replace)
	PreserveHost bool

	// Headers defines header manipulation rules
	Headers HeaderConfig

	// Timeout for requests to this upstream
	Timeout time.Duration

	// TLSMode controls TLS handling: "terminate" (default) or "passthrough"
	TLSMode string

	// RewriteRedirects enables automatic rewriting of Location headers
	// to route redirects through the proxy instead of directly to external services
	RewriteRedirects bool

	// RedirectBaseURL is the base URL clients use to access the proxy
	// Example: "https://api.example.com"
	// If empty, uses the incoming request's Host header
	RedirectBaseURL string
}

// HeaderConfig defines header manipulation rules.
type HeaderConfig struct {
	// StripIncoming removes headers from client request before forwarding
	// Supports wildcards: "X-Forwarded-*" matches X-Forwarded-For, etc.
	StripIncoming []string

	// StripOutgoing removes headers from upstream response before returning
	StripOutgoing []string

	// AddUpstream adds headers to request before forwarding to upstream
	// Values support environment variable expansion: ${AIPRISE_API_KEY}
	AddUpstream map[string]string

	// AddDownstream adds headers to response before returning to client
	AddDownstream map[string]string

	// ReplaceIncoming replaces headers in client request
	ReplaceIncoming map[string]string

	// ReplaceOutgoing replaces headers in upstream response
	ReplaceOutgoing map[string]string
}

// TransportConfig configures the HTTP transport layer.
type TransportConfig struct {
	// MaxIdleConns controls the maximum number of idle connections across all hosts
	MaxIdleConns int

	// MaxIdleConnsPerHost controls the maximum idle connections per host
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the maximum time an idle connection remains open
	IdleConnTimeout time.Duration

	// DialTimeout is the maximum time to establish a connection
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the maximum time for TLS handshake
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout is the maximum time to wait for response headers
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout for 100-continue responses
	ExpectContinueTimeout time.Duration

	// DisableKeepAlives disables HTTP keep-alives
	DisableKeepAlives bool

	// DisableCompression disables transparent compression
	DisableCompression bool
}

// TLSConfig configures TLS settings.
type TLSConfig struct {
	// CertFile is the path to the TLS certificate for downstream (client) connections
	CertFile string

	// KeyFile is the path to the TLS private key for downstream connections
	KeyFile string

	// CAFile is the path to CA certificates for verifying upstream servers
	CAFile string

	// InsecureSkipVerify disables upstream TLS verification (NOT RECOMMENDED)
	InsecureSkipVerify bool

	// MinVersion is the minimum TLS version (e.g., "1.2", "1.3")
	MinVersion string

	// CipherSuites is the list of enabled cipher suites
	CipherSuites []string
}

// MetricsConfig configures Prometheus metrics.
type MetricsConfig struct {
	// Enabled controls whether metrics are collected
	Enabled bool

	// Path is the HTTP path for the metrics endpoint (default: "/metrics")
	Path string

	// Port is the port for the metrics server (if different from main server)
	Port int

	// Namespace is the Prometheus namespace (default: "mimic_proxy")
	Namespace string
}

// LoggerConfig configures structured logging.
type LoggerConfig struct {
	// Level is the log level: "debug", "info", "warn", "error"
	Level string

	// Format is the log format: "json" or "text"
	Format string

	// Output is where to write logs: "stdout", "stderr", or a file path
	Output string
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() (err error) {
	if len(c.Routes) == 0 {
		err = errors.New("at least one route is required")
		return err
	}

	// Validate each route
	for i, route := range c.Routes {
		err = route.Validate()
		if err != nil {
			err = fmt.Errorf("route %d (%s): %w", i, route.Name, err)
			return err
		}
	}

	// Check for conflicting route paths
	err = c.checkConflictingRoutes()
	if err != nil {
		return err
	}

	// Validate TLS configuration if provided
	if c.TLS.CertFile != "" || c.TLS.KeyFile != "" {
		err = c.TLS.Validate()
		if err != nil {
			err = fmt.Errorf("TLS configuration: %w", err)
			return err
		}
	}

	return err
}

// Validate validates a route configuration.
func (r *RouteConfig) Validate() (err error) {
	if r.Name == "" {
		err = errors.New("route name is required")
		return err
	}

	if r.PathPrefix == "" {
		err = errors.New("path_prefix is required")
		return err
	}

	if !strings.HasPrefix(r.PathPrefix, "/") {
		err = fmt.Errorf("path_prefix must start with /: %s", r.PathPrefix)
		return err
	}

	if r.Upstream == "" {
		err = errors.New("upstream is required")
		return err
	}

	// Validate upstream URL
	var upstreamURL *url.URL
	upstreamURL, err = url.Parse(r.Upstream)
	if err != nil {
		err = fmt.Errorf("invalid upstream URL: %w", err)
		return err
	}

	if upstreamURL.Scheme != SchemeHTTP && upstreamURL.Scheme != SchemeHTTPS {
		err = fmt.Errorf("upstream URL must use http or https scheme: %s", r.Upstream)
		return err
	}

	// Validate TLS mode
	if r.TLSMode != "" && r.TLSMode != "terminate" && r.TLSMode != "passthrough" {
		err = fmt.Errorf("tls_mode must be 'terminate' or 'passthrough': %s", r.TLSMode)
		return err
	}

	// Validate redirect base URL if provided
	if r.RedirectBaseURL != "" {
		var baseURL *url.URL
		baseURL, err = url.Parse(r.RedirectBaseURL)
		if err != nil {
			err = fmt.Errorf("invalid redirect_base_url: %w", err)
			return err
		}
		if baseURL.Scheme == "" || baseURL.Host == "" {
			err = fmt.Errorf("redirect_base_url must include scheme and host: %s", r.RedirectBaseURL)
			return err
		}
	}

	// Validate header configuration
	err = r.Headers.Validate()
	if err != nil {
		err = fmt.Errorf("headers: %w", err)
		return err
	}

	return err
}

// Validate validates header configuration.
func (h *HeaderConfig) Validate() (err error) {
	// Check for environment variables in AddUpstream and AddDownstream
	for key, value := range h.AddUpstream {
		err = checkEnvVars(key, value)
		if err != nil {
			return err
		}
	}

	for key, value := range h.AddDownstream {
		err = checkEnvVars(key, value)
		if err != nil {
			return err
		}
	}

	return err
}

// checkEnvVars verifies that environment variables referenced in values exist.
func checkEnvVars(key, value string) (err error) {
	// Find all ${VAR} patterns
	start := 0
	for {
		idx := strings.Index(value[start:], "${")
		if idx == -1 {
			break
		}
		idx += start

		endIdx := strings.Index(value[idx:], "}")
		if endIdx == -1 {
			err = fmt.Errorf("header %s: unclosed environment variable reference: %s", key, value)
			return err
		}
		endIdx += idx

		varName := value[idx+2 : endIdx]
		if varName == "" {
			err = fmt.Errorf("header %s: empty environment variable reference: %s", key, value)
			return err
		}

		// Check if environment variable exists
		if _, exists := os.LookupEnv(varName); !exists {
			err = fmt.Errorf("header %s: environment variable not set: %s", key, varName)
			return err
		}

		start = endIdx + 1
	}

	return err
}

// Validate validates TLS configuration.
func (t *TLSConfig) Validate() (err error) {
	if t.CertFile != "" && t.KeyFile == "" {
		err = errors.New("cert_file specified but key_file is missing")
		return err
	}

	if t.KeyFile != "" && t.CertFile == "" {
		err = errors.New("key_file specified but cert_file is missing")
		return err
	}

	// Check if files exist
	err = validateTLSFile(t.CertFile, "cert_file")
	if err != nil {
		return err
	}

	err = validateTLSFile(t.KeyFile, "key_file")
	if err != nil {
		return err
	}

	err = validateTLSFile(t.CAFile, "ca_file")
	if err != nil {
		return err
	}

	// Validate TLS version
	if t.MinVersion != "" {
		err = parseTLSVersion(t.MinVersion)
		if err != nil {
			err = fmt.Errorf("min_version: %w", err)
			return err
		}
	}

	return err
}

// validateTLSFile validates that a TLS file exists and is not a directory.
func validateTLSFile(path, name string) (err error) {
	if path == "" {
		return err
	}

	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		err = fmt.Errorf("%s: %w", name, err)
		return err
	}

	if info.IsDir() {
		err = fmt.Errorf("%s is a directory: %s", name, path)
		return err
	}

	return err
}

// parseTLSVersion validates TLS version string.
func parseTLSVersion(version string) (err error) {
	switch version {
	case "1.0", "1.1", "1.2", "1.3":
		return err
	default:
		err = fmt.Errorf("invalid TLS version: %s (must be 1.0, 1.1, 1.2, or 1.3)", version)
		return err
	}
}

// checkConflictingRoutes checks for conflicting route paths.
func (c *Config) checkConflictingRoutes() (err error) {
	seen := make(map[string]string)

	for _, route := range c.Routes {
		// Check exact match
		if existingRoute, exists := seen[route.PathPrefix]; exists {
			err = fmt.Errorf("conflicting routes: %s and %s both use path_prefix: %s",
				existingRoute, route.Name, route.PathPrefix)
			return err
		}
		seen[route.PathPrefix] = route.Name
	}

	return err
}

// DefaultTransportConfig returns default transport configuration.
func DefaultTransportConfig() (config TransportConfig) {
	config = TransportConfig{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
	}
	return config
}

// DefaultMetricsConfig returns default metrics configuration.
func DefaultMetricsConfig() (config MetricsConfig) {
	config = MetricsConfig{
		Enabled:   false,
		Path:      "/metrics",
		Port:      0,
		Namespace: "mimic_proxy",
	}
	return config
}

// DefaultLoggerConfig returns default logger configuration.
func DefaultLoggerConfig() (config LoggerConfig) {
	config = LoggerConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	return config
}

// ApplyDefaults applies default values to the configuration.
func (c *Config) ApplyDefaults() {
	if c.Transport.MaxIdleConns == 0 {
		defaults := DefaultTransportConfig()
		c.Transport = defaults
	}

	if c.Metrics.Namespace == "" {
		c.Metrics.Namespace = "mimic_proxy"
	}

	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}

	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}

	if c.Logger.Format == "" {
		c.Logger.Format = "json"
	}

	if c.Logger.Output == "" {
		c.Logger.Output = "stdout"
	}

	// Apply defaults to routes
	for _, route := range c.Routes {
		if route.TLSMode == "" {
			route.TLSMode = "terminate"
		}
		if route.Timeout == 0 {
			route.Timeout = 30 * time.Second
		}
	}
}
