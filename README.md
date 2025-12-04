# mimic-proxy

A transparent reverse proxy designed for perfect transparency between clients and upstream servers. Built to be imperceptible to both ends while providing complete control over network traffic.

## Design Philosophy

**Primary Goal:** Transparency. The client cannot tell the difference between connecting directly to the upstream server and connecting through mimic-proxy. The upstream server cannot tell it's receiving proxied traffic.

**Secondary Goal:** Library-first design. The core proxy logic is packaged in `pkg/mimicproxy` for embedding into other services (e.g., api-service for Aiprise integration), while `cmd/mimic-proxy` provides a standalone binary for development use cases.

**Tertiary Goal:** Performance. Fast enough for production trading platform use, with connection pooling, zero-copy where possible, and minimal overhead.

## Requirements

This section documents all functional and non-functional requirements for mimic-proxy.

### Functional Requirements

#### FR-1: Perfect Transparency

**Requirement**: The proxy must be completely imperceptible to both clients and upstream servers.

**Details**:
- Clients cannot detect they are connecting through a proxy
- Upstream servers cannot detect they are receiving proxied traffic
- All proxy-identifying headers must be stripped by default
- Host headers must be rewritten appropriately

**Verification**: Integration tests capturing upstream request headers verify zero proxy-identifying headers present.

#### FR-2: Redirect Rewriting

**Requirement**: When upstream servers return HTTP redirects to external URLs, the proxy MUST intercept and rewrite these redirects to route through the proxy.

**Problem**: Upstream service returns `Location: https://external.com/oauth/callback`, causing client to bypass the proxy and connect directly to external.com.

**Solution**: Proxy detects redirect responses (301, 302, 303, 307, 308) and rewrites Location headers to map external URLs to proxy routes.

**Details**:
- Supports multi-hop redirect chains across multiple services
- Handles OAuth/OIDC flows (Aiprise, Dex, FusionAuth)
- Maps known external URLs to configured proxy routes
- Preserves query parameters and fragments in rewritten redirects
- Logs warnings when redirects point to unconfigured external services

**Configuration**:
```go
type RouteConfig struct {
    // ... existing fields

    // RewriteRedirects enables automatic rewriting of Location headers
    // to route redirects through the proxy instead of directly to external services
    RewriteRedirects bool

    // RedirectBaseURL is the base URL clients use to access the proxy
    // Example: "https://api.example.com"
    // If empty, uses the incoming request's Host header
    RedirectBaseURL string
}
```

**Verification**: Integration tests with mock services that genuinely attempt external redirects prove no direct external connections occur.

#### FR-3: Header Manipulation

**Requirement**: Support flexible header manipulation for security and authentication.

**Capabilities**:
- **Strip headers**: Remove headers matching patterns (supports wildcards)
  - Default strips: X-Forwarded-*, Via, X-Real-IP, X-Request-Id, X-Envoy-*
- **Add headers**: Inject authentication tokens, API keys
  - Supports environment variable expansion: ${VARIABLE_NAME}
- **Replace headers**: Modify specific header values
- **Direction control**: Separate rules for incoming (client), upstream, downstream (client), outgoing (upstream)

**Verification**: Integration tests verify headers are stripped/added/replaced correctly.

#### FR-4: Multiple Routes

**Requirement**: Support multiple independent routes with different upstream targets and configurations.

**Details**:
- Path-based routing (e.g., /aiprise -> api.aiprise.com, /binance -> api.binance.com)
- Per-route header manipulation rules
- Per-route timeout configuration
- Per-route TLS settings
- Longest-prefix-match routing

**Verification**: Unit tests verify correct route matching.

#### FR-5: Library-First Design

**Requirement**: Core proxy logic must be importable as a library for embedding in other Go services.

**Details**:
- Package: github.com/nikogura/mimic-proxy/pkg/mimicproxy
- Zero external dependencies beyond standard library and Prometheus
- Implements http.Handler interface
- No globals, all state in Proxy struct
- Thread-safe for concurrent requests

**Verification**: Example integration in api-service compiles and runs.

#### FR-6: Configuration Validation

**Requirement**: Validate all configuration at startup and fail fast with clear error messages.

**Validations**:
- Upstream URLs are parseable
- No conflicting route paths
- TLS certificates exist and are valid
- Header patterns are valid
- Timeouts are reasonable
- Environment variables referenced in headers exist

**Verification**: Unit tests verify validation catches common errors.

### Non-Functional Requirements

#### NFR-1: Performance

**Latency**: Target <1ms overhead per request

**Throughput**: Support 10,000+ requests/second on modern hardware (single instance)

**Memory**: <50MB baseline, scales with concurrent connections

**CPU**: <5% per 1,000 RPS

**Techniques**:
- Connection pooling (configurable limits)
- Zero-copy streaming of request/response bodies
- Buffer pool reuse
- Minimal allocations in hot paths

**Verification**: Load tests with wrk or vegeta measure latency and throughput.

#### NFR-2: Reliability

**Uptime**: Designed for 99.9% availability

**Error Handling**:
- Gracefully handle upstream failures (return 502 Bad Gateway)
- Gracefully handle timeouts (return 504 Gateway Timeout)
- Propagate client disconnect to upstream
- Never panic, always return proper HTTP errors

**Connection Management**:
- Automatic connection pooling with configurable limits
- Idle connection reaping
- Connection health checks

**Verification**: Integration tests verify error handling and connection cleanup.

#### NFR-3: Observability

**Requirement**: Comprehensive metrics, logging, and tracing for production operations.

**Metrics** (Prometheus):
- Request metrics (count, duration, status) by route, method
- Traffic metrics (bytes sent/received) by route
- Connection metrics (active, total, duration) by route
- Error metrics (upstream errors, 4xx, 5xx) by route and type
- Header manipulation metrics (stripped, added) by route
- **Redirect metrics (total, rewritten, passthrough) by route and rewrite type**
- TLS metrics (handshakes, duration, errors) by route and direction
- Client identification metrics (by IP, user agent) by route
- Upstream metrics (TTFB, health) by route
- Configuration metrics (routes count, uptime, reloads)

**Logging** (structured JSON):
- Request/response details with duration
- Header manipulation actions
- Redirect rewriting actions
- Errors with full context
- TLS handshake failures

**Tracing** (optional OpenTelemetry):
- Propagate trace context from client to upstream
- Generate spans for proxy operations

**Verification**: Integration tests verify metrics are recorded correctly.

#### NFR-4: Security

**TLS**:
- Support TLS 1.2+ (configurable minimum)
- Terminate client TLS, re-encrypt to upstream
- Verify upstream certificates (never InsecureSkipVerify in production)
- Support custom CA bundles

**Header Sanitization**:
- Strip proxy-identifying headers by default
- Prevent information leakage about internal topology
- Never log API keys or secrets

**Secrets Management**:
- API keys and tokens via environment variables only
- Support external secrets management (Vault, Kubernetes Secrets)
- Never store secrets in configuration files

**Verification**: Security audit checklist, integration tests verify TLS and header sanitization.

#### NFR-5: Maintainability

**Code Quality**:
- Must pass golangci-lint with YourCompany organization config
- Named return values required for all functions
- Comprehensive unit and integration tests
- Clear, concise documentation

**Configuration**:
- Declarative YAML configuration
- Environment variable expansion
- Validation at startup with clear error messages

**Deployment**:
- Single binary (no external dependencies)
- Docker container support
- Kubernetes manifest examples
- Library import for embedding

**Verification**: golangci-lint passes with zero violations, test coverage >80%.

### Use Case Requirements

#### UC-1: Aiprise KYC Integration (Library Mode)

**Requirement**: Embed mimic-proxy in api-service to transparently proxy Aiprise identity verification traffic.

**Details**:
- Client connects to api.example.com/v1/verify
- Proxy forwards to api.aiprise.com/api/v1/verify
- Strip all proxy headers
- Inject Aiprise API key
- Handle OAuth redirects (Aiprise may redirect to external OAuth providers)
- Both client and Aiprise unaware of proxying

**Verification**: Integration test simulating Aiprise OAuth flow proves no direct external connections.

#### UC-2: CEX Development Access (Standalone Mode)

**Requirement**: Run mimic-proxy as standalone service for YourCompany employees worldwide to access cryptocurrency exchanges with IP whitelisting.

**Details**:
- Standalone binary deployment
- Multiple exchange routes (Binance, OKX, Bybit, Gate.io, B2C2)
- Per-exchange authentication headers
- Strip all proxy headers
- Employees connect through proxy, exchanges see only whitelisted YourCompany IPs
- High throughput (trading traffic)

**Verification**: Load tests demonstrate sufficient throughput for trading operations.

#### UC-3: OAuth/OIDC Flow Support

**Requirement**: Support multi-hop OAuth flows where services redirect to external identity providers.

**Details**:
- Application redirects to OAuth provider (external URL)
- OAuth provider redirects back to application callback
- All redirects must route through proxy
- Preserve OAuth state, codes, and tokens
- Support Aiprise, Dex, FusionAuth, Google, etc.

**Verification**: OAuth flow integration tests prove complete flow works through proxy.

### Test Requirements

#### TR-1: Unit Tests

**Coverage**: >80% line coverage for all packages

**Tests**:
- Header manipulation logic
- Route matching
- Configuration validation
- TLS configuration parsing
- Redirect rewriting logic

#### TR-2: Integration Tests

**Critical Tests**:
- Basic proxy flow (request/response)
- **Redirect rewriting with real external URL mocks**
- Multi-hop redirect chains
- OAuth/OIDC flows
- TLS termination and re-encryption
- Header stripping verification
- Error handling (timeouts, connection failures)
- Concurrent requests

**Redirect Testing Philosophy**: Mock upstream services that GENUINELY attempt to redirect to external URLs. Use hit counters on external mocks to PROVE client never connects directly.

#### TR-3: Load Tests

**Tests**:
- Throughput benchmarks (wrk, vegeta)
- Latency percentiles (p50, p95, p99)
- Memory profiling (pprof)
- Connection pool behavior under load
- Concurrent request handling

**Targets**:
- >10,000 RPS sustained
- p99 latency <10ms
- <50MB memory for 1,000 concurrent connections

#### TR-4: Transparency Tests

**Tests**:
- Capture upstream request headers, verify zero proxy headers
- Verify Host header correctly set
- Verify response headers clean
- **Verify redirects properly rewritten**

## Use Cases

### 1. Aiprise KYC Integration (Library Mode)

Import mimic-proxy into api-service to provide transparent proxying of Aiprise identity verification traffic. Client connects to `api.example.com/v1/verify`, mimic-proxy forwards to `api.aiprise.com/api/v1/verify`, both parties remain unaware of proxying.

### 2. CEX Development Access (Standalone Mode)

Run mimic-proxy as a standalone service to provide YourCompany employees worldwide access to cryptocurrency exchanges that use IP whitelisting. Employee connects through mimic, exchange sees only whitelisted YourCompany IPs.

### 3. Header Sanitization & Security

Strip sensitive headers, add authentication tokens, enforce security policies, and disguise network topology from both clients and upstream servers.

## Architecture

```
Client                 mimic-proxy                 Upstream
  |                         |                          |
  |---- TLS handshake ----->|                          |
  |<--- terrace.fi cert ----|                          |
  |                         |                          |
  |---- HTTP request ------>|                          |
  |    Host: api.example.com |                          |
  |    Headers: [...]       |                          |
  |                         |                          |
  |                         |---- TLS handshake ------>|
  |                         |<--- upstream cert -------|
  |                         |                          |
  |                         |---- HTTP request ------->|
  |                         |  Host: api.upstream.com  |
  |                         |  Headers: [sanitized]    |
  |                         |                          |
  |                         |<---- HTTP response ------|
  |                         |      Headers: [...]      |
  |                         |                          |
  |<---- HTTP response -----|                          |
  |      Headers: [sanitized]                          |
  |                         |                          |
```

## Directory Structure

```
mimic-proxy/
├── README.md                    # This file
├── go.mod
├── go.sum
├── .golangci.yml               # YourCompany standard lint config
│
├── pkg/
│   └── mimicproxy/             # Core library (importable)
│       ├── proxy.go            # Main Proxy type and ServeHTTP
│       ├── config.go           # Configuration structures
│       ├── route.go            # Route matching and upstream selection
│       ├── headers.go          # Header manipulation logic
│       ├── transport.go        # Custom http.Transport with pooling
│       ├── metrics.go          # Prometheus metrics
│       ├── logger.go           # Structured logging interface
│       └── tls.go              # TLS configuration helpers
│
├── cmd/
│   └── mimic-proxy/
│       ├── main.go             # Standalone binary entrypoint
│       ├── config.go           # CLI config loading (Viper/Cobra)
│       └── signals.go          # Graceful shutdown handling
│
├── examples/
│   ├── api-service-integration/
│   │   └── main.go             # Example: embedding in api-service
│   ├── aiprise-proxy/
│   │   ├── main.go             # Example: Aiprise-specific proxy
│   │   └── config.yaml
│   └── cefi-proxy/
│       ├── main.go             # Example: CEX development proxy
│       └── config.yaml
│
└── docs/
    ├── library-usage.md        # How to use as library
    ├── standalone-usage.md     # How to run standalone
    ├── configuration.md        # Configuration reference
    └── header-manipulation.md  # Header manipulation patterns
```

## Core Library Design

### pkg/mimicproxy/proxy.go

```go
package mimicproxy

import (
    "net/http"
    "net/http/httputil"
)

// Proxy is a transparent reverse proxy that provides perfect transparency
// between clients and upstream servers.
type Proxy struct {
    config    *Config
    routes    []*Route
    transport *http.Transport
    metrics   *Metrics
    logger    Logger
}

// New creates a new Proxy instance with the given configuration.
func New(config *Config) (*Proxy, error)

// ServeHTTP implements http.Handler for use in HTTP servers.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request)

// Close gracefully shuts down the proxy, closing all connections.
func (p *Proxy) Close() error
```

### pkg/mimicproxy/config.go

```go
package mimicproxy

import (
    "crypto/tls"
    "time"
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

    // Logging configuration
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
    // When true, redirect responses (3xx) have their Location headers analyzed
    // and rewritten to map external URLs to proxy routes
    RewriteRedirects bool

    // RedirectBaseURL is the base URL clients use to access the proxy
    // Example: "https://api.example.com"
    // Used to construct rewritten redirect URLs
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
```

### pkg/mimicproxy/route.go

```go
package mimicproxy

import (
    "net/http"
    "net/url"
)

// Route represents a compiled route from client to upstream.
type Route struct {
    config       *RouteConfig
    upstream     *url.URL
    reverseProxy *httputil.ReverseProxy
}

// Match returns true if this route should handle the given request.
func (r *Route) Match(req *http.Request) (matched bool)

// Handle proxies the request to the upstream server.
func (r *Route) Handle(w http.ResponseWriter, req *http.Request)
```

### pkg/mimicproxy/headers.go

```go
package mimicproxy

import (
    "net/http"
    "strings"
)

// HeaderManipulator handles header transformation rules.
type HeaderManipulator struct {
    config *HeaderConfig
}

// ProcessIncoming applies header rules to client request before forwarding.
// Returns a new http.Header with transformations applied.
func (hm *HeaderManipulator) ProcessIncoming(inHeader http.Header) (outHeader http.Header)

// ProcessOutgoing applies header rules to upstream response before returning.
// Returns a new http.Header with transformations applied.
func (hm *HeaderManipulator) ProcessOutgoing(inHeader http.Header) (outHeader http.Header)

// stripHeaders removes headers matching patterns (supports wildcards).
func stripHeaders(header http.Header, patterns []string) (result http.Header)

// matchesPattern checks if a header name matches a pattern (supports "*" wildcard).
func matchesPattern(headerName, pattern string) (matches bool)

// expandEnvVars expands environment variables in header values.
// Supports ${VAR_NAME} syntax.
func expandEnvVars(value string) (expanded string)
```

### pkg/mimicproxy/transport.go

```go
package mimicproxy

import (
    "crypto/tls"
    "net"
    "net/http"
    "time"
)

// NewTransport creates a customized http.Transport with connection pooling
// and timeouts configured for optimal proxy performance.
func NewTransport(config *TransportConfig, tlsConfig *tls.Config) (transport *http.Transport, err error)

// ConnectionPool manages persistent connections to upstream servers.
type ConnectionPool struct {
    transport *http.Transport
    config    *TransportConfig
}

// Stats returns current connection pool statistics.
func (cp *ConnectionPool) Stats() (stats ConnectionPoolStats)

// ConnectionPoolStats provides visibility into connection pool health.
type ConnectionPoolStats struct {
    IdleConnections    int
    ActiveConnections  int
    TotalConnections   int
    ConnectionsCreated uint64
    ConnectionsClosed  uint64
}
```

### pkg/mimicproxy/metrics.go

```go
package mimicproxy

import (
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the proxy.
type Metrics struct {
    // Request Metrics
    // ---------------

    // Counter: Total requests by route, method, and status code
    // Labels: route, method, status
    // Usage: Track request volume and success rates per route
    RequestsTotal *prometheus.CounterVec

    // Histogram: Request duration by route and method
    // Labels: route, method
    // Buckets: .001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
    // Usage: Analyze latency percentiles (p50, p95, p99)
    RequestDuration *prometheus.HistogramVec

    // Traffic Metrics
    // ---------------

    // Counter: Total bytes sent to clients (responses) by route
    // Labels: route
    // Usage: Monitor bandwidth consumption per route
    BytesSent *prometheus.CounterVec

    // Counter: Total bytes received from clients (requests) by route
    // Labels: route
    // Usage: Monitor upload bandwidth per route
    BytesReceived *prometheus.CounterVec

    // Counter: Total bytes sent to upstream servers by route
    // Labels: route
    // Usage: Track upstream request sizes
    UpstreamBytesSent *prometheus.CounterVec

    // Counter: Total bytes received from upstream servers by route
    // Labels: route
    // Usage: Track upstream response sizes
    UpstreamBytesReceived *prometheus.CounterVec

    // Connection Metrics
    // ------------------

    // Gauge: Currently active connections by route
    // Labels: route
    // Usage: Monitor concurrent load per route
    ActiveConnections *prometheus.GaugeVec

    // Counter: Total connections opened by route
    // Labels: route
    // Usage: Track connection churn rate
    ConnectionsTotal *prometheus.CounterVec

    // Histogram: Connection duration by route
    // Labels: route
    // Buckets: 1, 5, 10, 30, 60, 120, 300, 600
    // Usage: Analyze how long connections stay open
    ConnectionDuration *prometheus.HistogramVec

    // Error Metrics
    // -------------

    // Counter: Upstream errors by route and error type
    // Labels: route, error_type
    // Error types: timeout, connection_refused, tls_error, dns_error,
    //              connection_reset, bad_gateway, unknown
    // Usage: Detect and alert on upstream failures
    UpstreamErrors *prometheus.CounterVec

    // Counter: Client errors (4xx) by route and status code
    // Labels: route, status
    // Usage: Monitor client-side issues
    ClientErrors *prometheus.CounterVec

    // Counter: Server errors (5xx) by route and status code
    // Labels: route, status
    // Usage: Monitor server-side issues
    ServerErrors *prometheus.CounterVec

    // Header Manipulation Metrics
    // ---------------------------

    // Counter: Headers stripped from requests by route and header name
    // Labels: route, header, direction (incoming/outgoing)
    // Usage: Verify header manipulation is working
    HeadersStripped *prometheus.CounterVec

    // Counter: Headers added to requests by route and header name
    // Labels: route, header, direction (upstream/downstream)
    // Usage: Verify header injection is working
    HeadersAdded *prometheus.CounterVec

    // Redirect Metrics
    // ----------------

    // Counter: Total redirects encountered by route
    // Labels: route, status (301, 302, 303, 307, 308)
    // Usage: Track redirect frequency
    RedirectsTotal *prometheus.CounterVec

    // Counter: Redirects rewritten by route
    // Labels: route, rewrite_type (internal, external_known, external_unknown)
    // Usage: Verify redirect rewriting is working correctly
    RedirectsRewritten *prometheus.CounterVec

    // Counter: Redirects passed through unchanged by route
    // Labels: route, reason (relative, same_host, unknown_external)
    // Usage: Monitor redirects that weren't rewritten
    RedirectsPassthrough *prometheus.CounterVec

    // TLS Metrics
    // -----------

    // Counter: TLS handshakes by route and direction
    // Labels: route, direction (downstream/upstream)
    // Usage: Monitor TLS overhead
    TLSHandshakes *prometheus.CounterVec

    // Histogram: TLS handshake duration by route and direction
    // Labels: route, direction
    // Buckets: .01, .025, .05, .1, .25, .5, 1, 2.5, 5
    // Usage: Identify slow TLS handshakes
    TLSHandshakeDuration *prometheus.HistogramVec

    // Counter: TLS errors by route, direction, and error type
    // Labels: route, direction, error_type
    // Error types: cert_invalid, cert_expired, unknown_ca, handshake_failure
    // Usage: Alert on TLS issues
    TLSErrors *prometheus.CounterVec

    // Client Identification Metrics
    // -----------------------------

    // Counter: Requests by client IP (top N IPs tracked)
    // Labels: route, client_ip, client_type (internal/external)
    // Usage: Identify which clients/IPs are using the proxy
    // Note: May need cardinality limits for high-traffic scenarios
    RequestsByClientIP *prometheus.CounterVec

    // Counter: Requests by User-Agent
    // Labels: route, user_agent_family (browser, bot, api_client)
    // Usage: Identify client types
    RequestsByUserAgent *prometheus.CounterVec

    // Upstream Metrics
    // ----------------

    // Histogram: Time to first byte from upstream by route
    // Labels: route
    // Buckets: .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
    // Usage: Measure upstream server responsiveness
    UpstreamTTFB *prometheus.HistogramVec

    // Gauge: Upstream health status by route
    // Labels: route
    // Values: 1 (healthy), 0 (unhealthy)
    // Usage: Monitor upstream availability
    UpstreamHealth *prometheus.GaugeVec

    // Configuration Metrics
    // ---------------------

    // Gauge: Number of configured routes
    // Usage: Monitor proxy configuration
    ConfiguredRoutes prometheus.Gauge

    // Gauge: Proxy uptime in seconds
    // Usage: Track proxy restarts
    UptimeSeconds prometheus.Gauge

    // Counter: Configuration reloads
    // Labels: success (true/false)
    // Usage: Track dynamic config updates
    ConfigReloads *prometheus.CounterVec
}

// NewMetrics creates and registers all proxy metrics.
func NewMetrics(config *MetricsConfig) (metrics *Metrics, err error)

// RecordRequest records metrics for a completed request.
// Should be called after each request completes.
func (m *Metrics) RecordRequest(
    route string,
    method string,
    statusCode int,
    duration time.Duration,
    bytesSent int64,
    bytesReceived int64,
    upstreamBytesSent int64,
    upstreamBytesReceived int64,
    clientIP string,
    userAgent string,
)

// RecordError records an error metric.
func (m *Metrics) RecordError(route string, errorType string, isUpstream bool)

// RecordRedirect records redirect metrics.
func (m *Metrics) RecordRedirect(route string, status int, rewritten bool, rewriteType string)

// RecordTLS records TLS handshake metrics.
func (m *Metrics) RecordTLS(route string, direction string, duration time.Duration, err error)

// RecordHeaderManipulation records header manipulation metrics.
func (m *Metrics) RecordHeaderManipulation(route string, headerName string, operation string, direction string)
```

### pkg/mimicproxy/logger.go

```go
package mimicproxy

// Logger is the interface for structured logging.
// Implementations can use zerolog, slog, or any other logging library.
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    With(fields ...Field) Logger
}

// Field represents a structured log field.
type Field struct {
    Key   string
    Value interface{}
}

// DefaultLogger returns a logger that writes JSON to stdout.
func DefaultLogger(config *LoggerConfig) (logger Logger, err error)
```

### pkg/mimicproxy/tls.go

```go
package mimicproxy

import (
    "crypto/tls"
    "crypto/x509"
)

// LoadTLSConfig loads TLS certificates and creates tls.Config for both
// downstream (client) and upstream (server) connections.
func LoadTLSConfig(config *TLSConfig) (downstreamTLS, upstreamTLS *tls.Config, err error)

// ParseTLSVersion converts string version to tls constant.
func ParseTLSVersion(version string) (tlsVersion uint16, err error)

// ParseCipherSuites converts cipher suite names to constants.
func ParseCipherSuites(suites []string) (cipherSuites []uint16, err error)

// LoadCAPool loads CA certificates for upstream verification.
func LoadCAPool(caFile string) (pool *x509.CertPool, err error)
```

## Usage Examples

### Example 1: Embed in api-service for Aiprise

```go
package main

import (
    "log"
    "net/http"
    "os"
    "time"

    "github.com/nikogura/mimic-proxy/pkg/mimicproxy"
)

func main() {
    // Configure transparent proxy for Aiprise
    proxyConfig := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:               "aiprise-verify",
                PathPrefix:         "/v1/verify",
                Upstream:           "https://api.aiprise.com",
                UpstreamPathPrefix: "/api/v1/verify",
                PreserveHost:       false,
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{
                        "X-Forwarded-*",
                        "Via",
                        "X-Real-IP",
                        "X-Request-Id",
                    },
                    StripOutgoing: []string{
                        "Server",
                        "X-Powered-By",
                        "X-Envoy-*",
                    },
                    AddUpstream: map[string]string{
                        "X-API-Key": os.Getenv("AIPRISE_API_KEY"),
                    },
                },
                Timeout: 30 * time.Second,
                TLSMode: "terminate",
            },
        },
        Transport: mimicproxy.TransportConfig{
            MaxIdleConns:          100,
            MaxIdleConnsPerHost:   10,
            IdleConnTimeout:       90 * time.Second,
            TLSHandshakeTimeout:   10 * time.Second,
            ResponseHeaderTimeout: 30 * time.Second,
        },
        Metrics: mimicproxy.MetricsConfig{
            Enabled:   true,
            Path:      "/metrics",
            Namespace: "api_service",
        },
    }

    // Create proxy
    proxy, err := mimicproxy.New(proxyConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer proxy.Close()

    // Integrate into existing API service HTTP mux
    mux := http.NewServeMux()
    mux.Handle("/v1/verify/", proxy)
    mux.HandleFunc("/health", healthHandler)
    // ... other API routes

    log.Println("API service with Aiprise proxy running on :8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

### Example 2: Standalone CEX Development Proxy

```yaml
# config.yaml
routes:
  - name: binance-api
    path_prefix: /binance
    upstream: https://api.binance.com
    upstream_path_prefix: /api
    preserve_host: false
    timeout: 30s
    tls_mode: terminate
    headers:
      strip_incoming:
        - "X-Forwarded-*"
        - "Via"
        - "X-Real-IP"
      strip_outgoing:
        - "Server"
        - "X-Powered-By"
      add_upstream:
        X-MBX-APIKEY: ${BINANCE_API_KEY}

  - name: okx-api
    path_prefix: /okx
    upstream: https://www.okx.com
    upstream_path_prefix: /api
    preserve_host: false
    timeout: 30s
    headers:
      strip_incoming:
        - "X-Forwarded-*"
      add_upstream:
        OK-ACCESS-KEY: ${OKX_API_KEY}
        OK-ACCESS-SIGN: ${OKX_API_SIGN}
        OK-ACCESS-TIMESTAMP: ${OKX_API_TIMESTAMP}
        OK-ACCESS-PASSPHRASE: ${OKX_API_PASSPHRASE}

transport:
  max_idle_conns: 100
  max_idle_conns_per_host: 10
  idle_conn_timeout: 90s
  dial_timeout: 10s
  tls_handshake_timeout: 10s
  response_header_timeout: 30s

tls:
  cert_file: /etc/mimic-proxy/tls/cert.pem
  key_file: /etc/mimic-proxy/tls/key.pem
  min_version: "1.2"

metrics:
  enabled: true
  path: /metrics
  port: 9090
  namespace: mimic_proxy

logger:
  level: info
  format: json
  output: stdout
```

Run the standalone proxy:

```bash
./mimic-proxy --config config.yaml --listen :8443
```

### Example 3: Multiple Upstream Targets

```go
package main

import (
    "net/http"
    "os"

    "github.com/nikogura/mimic-proxy/pkg/mimicproxy"
)

func main() {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            // Aiprise KYC
            {
                Name:         "aiprise",
                PathPrefix:   "/v1/verify",
                Upstream:     "https://api.aiprise.com/api/v1/verify",
                PreserveHost: false,
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*", "Via"},
                    AddUpstream:   map[string]string{"X-API-Key": os.Getenv("AIPRISE_API_KEY")},
                },
            },
            // Binance
            {
                Name:         "binance",
                PathPrefix:   "/binance",
                Upstream:     "https://api.binance.com/api",
                PreserveHost: false,
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*"},
                    AddUpstream:   map[string]string{"X-MBX-APIKEY": os.Getenv("BINANCE_API_KEY")},
                },
            },
            // OKX
            {
                Name:         "okx",
                PathPrefix:   "/okx",
                Upstream:     "https://www.okx.com/api",
                PreserveHost: false,
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*"},
                    AddUpstream: map[string]string{
                        "OK-ACCESS-KEY":        os.Getenv("OKX_API_KEY"),
                        "OK-ACCESS-SIGN":       os.Getenv("OKX_API_SIGN"),
                        "OK-ACCESS-TIMESTAMP":  os.Getenv("OKX_API_TIMESTAMP"),
                        "OK-ACCESS-PASSPHRASE": os.Getenv("OKX_API_PASSPHRASE"),
                    },
                },
            },
        },
    }

    proxy, _ := mimicproxy.New(config)
    defer proxy.Close()

    http.ListenAndServe(":8080", proxy)
}
```

## Header Manipulation Patterns

### Perfect Transparency Pattern

Strip ALL proxy-identifying headers:

```yaml
headers:
  strip_incoming:
    - "X-Forwarded-*"
    - "X-Real-IP"
    - "X-Request-Id"
    - "Via"
    - "Forwarded"
    - "X-Envoy-*"
  strip_outgoing:
    - "Server"
    - "X-Powered-By"
    - "X-Runtime"
    - "X-Envoy-*"
```

### API Key Injection Pattern

Add authentication headers for upstream:

```yaml
headers:
  add_upstream:
    X-API-Key: ${API_KEY}
    Authorization: "Bearer ${AUTH_TOKEN}"
```

### Security Sanitization Pattern

Remove sensitive headers before forwarding:

```yaml
headers:
  strip_incoming:
    - "Cookie"
    - "Authorization"
    - "X-*"  # Strip all X- headers
  add_upstream:
    X-API-Key: ${UPSTREAM_API_KEY}  # Add controlled auth
```

### Development Debug Pattern

Add headers for debugging while preserving transparency to upstream:

```yaml
headers:
  strip_incoming:
    - "X-Forwarded-*"
  add_downstream:
    X-Debug-Proxy: "mimic-proxy"
    X-Debug-Route: "${ROUTE_NAME}"
```

## Performance Characteristics

### Connection Pooling

- Default: 100 idle connections globally, 10 per host
- Configurable per environment (dev: fewer, prod: more)
- Automatic connection reuse reduces latency

### Zero-Copy Where Possible

- Request/response bodies streamed using io.Copy
- No buffering of large bodies in memory
- Configurable buffer pools for optimal memory usage

### Benchmarks (Expected)

Based on Go's httputil.ReverseProxy performance:

- **Latency overhead**: ~0.5-1ms per request
- **Throughput**: 10k-50k RPS on modern hardware (single instance)
- **Memory**: ~10-50MB baseline, scales with concurrent connections
- **CPU**: ~1-5% per 1k RPS

For comparison, Envoy achieves ~100k RPS but requires significant configuration complexity and doesn't meet transparency requirements.

## Configuration Validation

The proxy performs extensive configuration validation at startup:

```go
func (c *Config) Validate() (err error) {
    // Validates:
    // - All upstream URLs are parseable
    // - No conflicting route paths
    // - TLS certificates exist and are valid
    // - Header patterns are valid
    // - Timeouts are reasonable
    // - Environment variables for header values exist
    return
}
```

## Error Handling

### Upstream Errors

When upstream returns errors:

- 5xx errors: logged, metrics incremented, passed through to client
- Connection failures: logged, metrics incremented, 502 Bad Gateway returned
- Timeouts: logged, metrics incremented, 504 Gateway Timeout returned

### Client Errors

When client disconnects:

- Context cancellation propagates to upstream
- Connections cleaned up immediately
- Metrics recorded

### TLS Errors

When TLS fails:

- Downstream (client) TLS errors: connection closed, logged
- Upstream (server) TLS errors: 502 returned, logged with details

## Security Considerations

### Header Sanitization

By default, mimic-proxy strips common proxy headers to prevent:

- Information leakage about internal topology
- Client IP exposure (unless explicitly configured)
- Proxy detection by upstream servers

### TLS Configuration

Recommended TLS settings:

- MinVersion: TLS 1.2 (or 1.3 for modern deployments)
- Disable weak cipher suites
- Verify upstream certificates (never use InsecureSkipVerify in production)
- Rotate certificates regularly

### Environment Variable Security

API keys and secrets should:

- Be passed via environment variables, not config files
- Use secrets management (Vault, Kubernetes Secrets, etc.)
- Never be logged or included in error messages

### Rate Limiting

Currently NOT implemented in core library (intentional). Use external rate limiting:

- Kubernetes: Ingress controllers with rate limiting
- Cloud: CloudFlare, AWS WAF
- Application: Implement in api-service before calling proxy

## Observability

### Prometheus Metrics

All metrics follow naming convention: `{namespace}_{subsystem}_{metric}_{unit}`

Example metrics:

```
# Request metrics
mimic_proxy_requests_total{route="aiprise",status="200"} 1234
mimic_proxy_request_duration_seconds{route="aiprise",quantile="0.5"} 0.123
mimic_proxy_request_duration_seconds{route="aiprise",quantile="0.95"} 0.456
mimic_proxy_request_duration_seconds{route="aiprise",quantile="0.99"} 0.789

# Traffic metrics
mimic_proxy_bytes_sent_total{route="aiprise"} 1234567
mimic_proxy_bytes_received_total{route="aiprise"} 7654321

# Connection metrics
mimic_proxy_active_connections{route="aiprise"} 42

# Error metrics
mimic_proxy_upstream_errors_total{route="aiprise",type="timeout"} 5
mimic_proxy_upstream_errors_total{route="aiprise",type="connection_refused"} 2

# Header manipulation metrics
mimic_proxy_headers_stripped_total{route="aiprise",direction="incoming"} 100
mimic_proxy_headers_added_total{route="aiprise",direction="upstream"} 100
```

### Structured Logging

All log entries include structured fields:

```json
{
  "level": "info",
  "time": "2025-11-11T12:34:56Z",
  "msg": "proxied request",
  "route": "aiprise",
  "method": "POST",
  "path": "/v1/verify",
  "upstream": "https://api.aiprise.com/api/v1/verify",
  "status": 200,
  "duration_ms": 123.45,
  "bytes_sent": 1234,
  "bytes_received": 5678
}
```

### Distributed Tracing

Optional OpenTelemetry integration:

- Propagate trace context from client to upstream
- Generate spans for each proxy operation
- Export to Jaeger, Zipkin, or cloud tracing services

## Testing Strategy

### Unit Tests

- Header manipulation logic
- Route matching
- Configuration validation
- TLS configuration parsing

### Integration Tests

#### Basic Proxy Flow Tests

End-to-end proxy flow with test HTTP servers:

```go
func TestBasicProxyFlow(t *testing.T) {
    // Create mock upstream server
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("upstream response"))
    }))
    defer upstream.Close()

    // Create proxy pointing to mock upstream
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "test",
                PathPrefix: "/api",
                Upstream:   upstream.URL,
            },
        },
    }

    proxy, err := mimicproxy.New(config)
    if err != nil {
        t.Fatal(err)
    }
    defer proxy.Close()

    // Make request through proxy
    req := httptest.NewRequest("GET", "/api/test", nil)
    w := httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Expected 200, got %d", w.Code)
    }
}
```

#### Redirect Rewriting Tests

**Critical**: When upstream servers return redirects to external resources, the proxy MUST rewrite these redirects to route through itself.

**Problem**: Upstream returns `Location: https://external.com/oauth/callback`, client tries to connect directly to external.com, bypassing the proxy.

**Solution**: Proxy detects redirect headers and rewrites them to route through itself.

```go
func TestRedirectRewriting(t *testing.T) {
    // Mock upstream that redirects to external service
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/start":
            // Upstream redirects client to external OAuth provider
            w.Header().Set("Location", "https://external-oauth.com/authorize?client_id=123")
            w.WriteHeader(http.StatusFound)
        case "/api/callback":
            // After OAuth, client is redirected back
            w.WriteHeader(http.StatusOK)
            w.Write([]byte("authenticated"))
        default:
            w.WriteHeader(http.StatusNotFound)
        }
    }))
    defer upstream.Close()

    // Mock external OAuth provider
    externalOAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/authorize" {
            // OAuth provider redirects back to upstream callback
            w.Header().Set("Location", upstream.URL+"/api/callback?code=abc123")
            w.WriteHeader(http.StatusFound)
        }
    }))
    defer externalOAuth.Close()

    // Create proxy with redirect rewriting enabled
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:              "test-upstream",
                PathPrefix:        "/api",
                Upstream:          upstream.URL,
                RewriteRedirects:  true, // Enable redirect rewriting
            },
            {
                Name:              "external-oauth",
                PathPrefix:        "/external-oauth",
                Upstream:          externalOAuth.URL,
                RewriteRedirects:  true,
            },
        },
    }

    proxy, err := mimicproxy.New(config)
    if err != nil {
        t.Fatal(err)
    }
    defer proxy.Close()

    // Step 1: Client requests /api/start through proxy
    req1 := httptest.NewRequest("GET", "/api/start", nil)
    w1 := httptest.NewRecorder()
    proxy.ServeHTTP(w1, req1)

    // Verify proxy rewrote the redirect to route through itself
    if w1.Code != http.StatusFound {
        t.Errorf("Expected 302, got %d", w1.Code)
    }

    location := w1.Header().Get("Location")
    // Should be rewritten to: /external-oauth/authorize?client_id=123
    if !strings.HasPrefix(location, "/external-oauth/") {
        t.Errorf("Redirect not rewritten through proxy: %s", location)
    }

    // Step 2: Client follows rewritten redirect through proxy
    req2 := httptest.NewRequest("GET", location, nil)
    w2 := httptest.NewRecorder()
    proxy.ServeHTTP(w2, req2)

    // Verify second redirect is also rewritten
    if w2.Code != http.StatusFound {
        t.Errorf("Expected 302, got %d", w2.Code)
    }

    location2 := w2.Header().Get("Location")
    // Should be rewritten to: /api/callback?code=abc123
    if !strings.HasPrefix(location2, "/api/") {
        t.Errorf("OAuth callback redirect not rewritten through proxy: %s", location2)
    }

    // Step 3: Client follows callback redirect through proxy
    req3 := httptest.NewRequest("GET", location2, nil)
    w3 := httptest.NewRecorder()
    proxy.ServeHTTP(w3, req3)

    if w3.Code != http.StatusOK {
        t.Errorf("Expected 200, got %d", w3.Code)
    }

    body := w3.Body.String()
    if body != "authenticated" {
        t.Errorf("Expected 'authenticated', got '%s'", body)
    }
}
```

#### Redirect Rewriting Configuration

Add to `RouteConfig`:

```go
type RouteConfig struct {
    // ... existing fields

    // RewriteRedirects enables automatic rewriting of Location headers
    // to route redirects through the proxy instead of directly to external services
    RewriteRedirects bool

    // RedirectBaseURL is the base URL clients use to access the proxy
    // Example: "https://api.example.com" - used to construct rewritten redirect URLs
    // If empty, uses the incoming request's Host header
    RedirectBaseURL string
}
```

#### Redirect Rewriting Implementation

In `pkg/mimicproxy/headers.go`, add redirect detection and rewriting:

```go
// RewriteRedirects rewrites Location headers in responses to route through the proxy.
func (hm *HeaderManipulator) RewriteRedirects(
    respHeader http.Header,
    incomingHost string,
    proxyBasePath string,
    upstreamURL *url.URL,
    routes []*RouteConfig,
) (rewritten http.Header) {
    location := respHeader.Get("Location")
    if location == "" {
        return respHeader // No redirect
    }

    locationURL, err := url.Parse(location)
    if err != nil {
        return respHeader // Invalid URL, leave as-is
    }

    // If redirect is relative, no rewriting needed
    if !locationURL.IsAbs() {
        return respHeader
    }

    // Find which route matches the redirect target
    for _, route := range routes {
        routeURL, _ := url.Parse(route.Upstream)

        // Check if redirect points to a known upstream
        if locationURL.Host == routeURL.Host {
            // Rewrite to route through proxy
            newLocation := proxyBasePath + route.PathPrefix + locationURL.Path
            if locationURL.RawQuery != "" {
                newLocation += "?" + locationURL.RawQuery
            }
            if locationURL.Fragment != "" {
                newLocation += "#" + locationURL.Fragment
            }

            rewritten = respHeader.Clone()
            rewritten.Set("Location", newLocation)
            return rewritten
        }
    }

    // Redirect points to unknown external service
    // Log warning but leave as-is (or optionally block)
    return respHeader
}
```

#### Multi-Hop Redirect Chain Tests

Test complex redirect chains across multiple services:

```go
func TestMultiHopRedirectChain(t *testing.T) {
    // Service A redirects to Service B
    serviceA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Location", "https://service-b.external.com/step2")
        w.WriteHeader(http.StatusFound)
    }))
    defer serviceA.Close()

    // Service B redirects to Service C
    serviceB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Location", "https://service-c.external.com/step3")
        w.WriteHeader(http.StatusFound)
    }))
    defer serviceB.Close()

    // Service C returns final response
    serviceC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("final destination"))
    }))
    defer serviceC.Close()

    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {Name: "service-a", PathPrefix: "/service-a", Upstream: serviceA.URL, RewriteRedirects: true},
            {Name: "service-b", PathPrefix: "/service-b", Upstream: serviceB.URL, RewriteRedirects: true},
            {Name: "service-c", PathPrefix: "/service-c", Upstream: serviceC.URL, RewriteRedirects: true},
        },
    }

    proxy, _ := mimicproxy.New(config)
    defer proxy.Close()

    // Client makes initial request
    req := httptest.NewRequest("GET", "/service-a/start", nil)
    w := httptest.NewRecorder()

    // Follow redirect chain through proxy
    for i := 0; i < 3; i++ {
        proxy.ServeHTTP(w, req)

        if w.Code == http.StatusOK {
            break
        }

        if w.Code != http.StatusFound {
            t.Fatalf("Unexpected status at hop %d: %d", i+1, w.Code)
        }

        location := w.Header().Get("Location")
        if !strings.HasPrefix(location, "/service-") {
            t.Fatalf("Redirect at hop %d not rewritten through proxy: %s", i+1, location)
        }

        // Follow redirect
        req = httptest.NewRequest("GET", location, nil)
        w = httptest.NewRecorder()
    }

    if w.Body.String() != "final destination" {
        t.Errorf("Did not reach final destination")
    }
}
```

#### OAuth/OIDC Flow Tests

Test realistic OAuth flows that involve multiple redirects:

```go
func TestOAuthFlowThroughProxy(t *testing.T) {
    // Mock OAuth provider (like Aiprise, Dex, etc.)
    oauthProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/authorize":
            // OAuth provider shows login page, then redirects back with code
            w.Header().Set("Location", "https://app.example.com/callback?code=auth_code_123&state=xyz")
            w.WriteHeader(http.StatusFound)
        case "/token":
            // Exchange code for token
            w.Header().Set("Content-Type", "application/json")
            w.Write([]byte(`{"access_token":"token_123","token_type":"Bearer"}`))
        }
    }))
    defer oauthProvider.Close()

    // Mock application server
    appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/login":
            // App redirects to OAuth provider
            w.Header().Set("Location", oauthProvider.URL+"/authorize?client_id=app123&state=xyz")
            w.WriteHeader(http.StatusFound)
        case "/callback":
            // App receives OAuth callback
            code := r.URL.Query().Get("code")
            if code == "" {
                w.WriteHeader(http.StatusBadRequest)
                return
            }
            // App exchanges code for token (via proxy)
            // Then returns success
            w.Write([]byte("login successful"))
        }
    }))
    defer appServer.Close()

    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {Name: "app", PathPrefix: "/app", Upstream: appServer.URL, RewriteRedirects: true},
            {Name: "oauth", PathPrefix: "/oauth", Upstream: oauthProvider.URL, RewriteRedirects: true},
        },
    }

    proxy, _ := mimicproxy.New(config)
    defer proxy.Close()

    // Simulate user clicking "Login"
    req := httptest.NewRequest("GET", "/app/login", nil)
    w := httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    // Should redirect to OAuth provider through proxy
    location := w.Header().Get("Location")
    if !strings.HasPrefix(location, "/oauth/") {
        t.Fatalf("OAuth redirect not rewritten: %s", location)
    }

    // Follow to OAuth provider (through proxy)
    req = httptest.NewRequest("GET", location, nil)
    w = httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    // Should redirect back to app callback (through proxy)
    location = w.Header().Get("Location")
    if !strings.HasPrefix(location, "/app/callback") {
        t.Fatalf("OAuth callback redirect not rewritten: %s", location)
    }

    // Follow callback
    req = httptest.NewRequest("GET", location, nil)
    w = httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    if w.Body.String() != "login successful" {
        t.Errorf("OAuth flow did not complete successfully")
    }
}
```

#### TLS Termination and Re-encryption Tests

- TLS termination from client
- Re-encryption to upstream
- Certificate validation
- SNI handling

#### Header Stripping Verification Tests

```go
func TestHeaderStripping(t *testing.T) {
    // Mock upstream that echoes received headers
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(r.Header)
    }))
    defer upstream.Close()

    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "test",
                PathPrefix: "/api",
                Upstream:   upstream.URL,
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*", "Via", "X-Request-Id"},
                },
            },
        },
    }

    proxy, _ := mimicproxy.New(config)
    defer proxy.Close()

    // Make request with proxy headers
    req := httptest.NewRequest("GET", "/api/test", nil)
    req.Header.Set("X-Forwarded-For", "1.2.3.4")
    req.Header.Set("X-Forwarded-Proto", "https")
    req.Header.Set("Via", "1.1 proxy")
    req.Header.Set("X-Request-Id", "12345")
    req.Header.Set("User-Agent", "test-client")

    w := httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    var receivedHeaders map[string][]string
    json.NewDecoder(w.Body).Decode(&receivedHeaders)

    // Verify proxy headers were stripped
    proxyHeaders := []string{"X-Forwarded-For", "X-Forwarded-Proto", "Via", "X-Request-Id"}
    for _, header := range proxyHeaders {
        if _, exists := receivedHeaders[header]; exists {
            t.Errorf("Proxy header %s was not stripped", header)
        }
    }

    // Verify legitimate headers were preserved
    if _, exists := receivedHeaders["User-Agent"]; !exists {
        t.Error("User-Agent header was incorrectly stripped")
    }
}
```

#### Error Handling Tests

Test upstream failures, timeouts, and error propagation:

```go
func TestUpstreamTimeout(t *testing.T) {
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(5 * time.Second) // Exceed timeout
    }))
    defer upstream.Close()

    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "test",
                PathPrefix: "/api",
                Upstream:   upstream.URL,
                Timeout:    1 * time.Second,
            },
        },
    }

    proxy, _ := mimicproxy.New(config)
    defer proxy.Close()

    req := httptest.NewRequest("GET", "/api/slow", nil)
    w := httptest.NewRecorder()
    proxy.ServeHTTP(w, req)

    if w.Code != http.StatusGatewayTimeout {
        t.Errorf("Expected 504, got %d", w.Code)
    }
}
```

### Load Tests

- Benchmark throughput with wrk or vegeta
- Memory profiling with pprof
- Connection pool behavior under load
- Concurrent request handling

### Transparency Tests

- Capture upstream request headers
- Verify NO proxy headers are present
- Verify Host header is correctly set
- Verify response headers are clean
- Verify redirects are properly rewritten

## Deployment

### Standalone Binary

```bash
# Build
go build -o mimic-proxy ./cmd/mimic-proxy

# Run
./mimic-proxy --config config.yaml --listen :8443

# Docker
docker build -t mimic-proxy:latest .
docker run -p 8443:8443 -v $(pwd)/config.yaml:/etc/mimic-proxy/config.yaml mimic-proxy:latest
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mimic-proxy
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: mimic-proxy
        image: mimic-proxy:latest
        ports:
        - containerPort: 8443
        env:
        - name: AIPRISE_API_KEY
          valueFrom:
            secretKeyRef:
              name: aiprise-credentials
              key: api-key
        volumeMounts:
        - name: config
          mountPath: /etc/mimic-proxy
        - name: tls
          mountPath: /etc/mimic-proxy/tls
      volumes:
      - name: config
        configMap:
          name: mimic-proxy-config
      - name: tls
        secret:
          secretName: mimic-proxy-tls
```

### Library Integration

```go
// In api-service/main.go
import "github.com/nikogura/mimic-proxy/pkg/mimicproxy"

func setupAipriseProxy() (handler http.Handler, err error) {
    config := &mimicproxy.Config{ /* ... */ }
    proxy, err := mimicproxy.New(config)
    if err != nil {
        return nil, err
    }
    return proxy, nil
}

func main() {
    mux := http.NewServeMux()

    aipriseProxy, err := setupAipriseProxy()
    if err != nil {
        log.Fatal(err)
    }
    mux.Handle("/v1/verify/", aipriseProxy)

    // ... rest of API service setup
}
```

## Roadmap

### Phase 1: Core Functionality (MVP)

- [x] Design architecture
- [ ] Implement core proxy logic (pkg/mimicproxy/proxy.go)
- [ ] Implement header manipulation (pkg/mimicproxy/headers.go)
- [ ] Implement route matching (pkg/mimicproxy/route.go)
- [ ] Basic configuration (pkg/mimicproxy/config.go)
- [ ] Unit tests

### Phase 2: Production Readiness

- [ ] TLS configuration (pkg/mimicproxy/tls.go)
- [ ] Prometheus metrics (pkg/mimicproxy/metrics.go)
- [ ] Structured logging (pkg/mimicproxy/logger.go)
- [ ] Connection pooling optimization (pkg/mimicproxy/transport.go)
- [ ] Integration tests
- [ ] Performance benchmarks

### Phase 3: Standalone Binary

- [ ] CLI implementation (cmd/mimic-proxy/main.go)
- [ ] YAML configuration loading
- [ ] Graceful shutdown
- [ ] Docker packaging
- [ ] Documentation

### Phase 4: Advanced Features

- [ ] TLS passthrough mode
- [ ] WebSocket support
- [ ] HTTP/2 optimization
- [ ] gRPC proxying
- [ ] Dynamic configuration reload
- [ ] Circuit breaker pattern
- [ ] Request/response body transformation

### Phase 5: Enterprise Features

- [ ] OpenTelemetry tracing
- [ ] Dynamic route updates via API
- [ ] Health check endpoints per route
- [ ] Advanced load balancing (multiple upstreams per route)
- [ ] Request/response caching
- [ ] Traffic mirroring (shadow testing)

## Contributing

This project follows Standard Go Engineering Practices:

1. **All code must pass golangci-lint** with organization config from https://github.com/nikogura/namedreturns (custom linter)
2. **Named return values are required** for all functions returning multiple values or errors
3. **All library code must be exhaustively tested**
4. **GitOps only** - no manual changes to infrastructure
5. **Security first** - every line of code is a potential attack vector

### Code Review Checklist

- [ ] golangci-lint passes with zero violations
- [ ] All functions have named return values
- [ ] Unit tests cover all logic paths
- [ ] Integration tests verify end-to-end behavior
- [ ] No secrets in code or logs
- [ ] Documentation updated
- [ ] Metrics added for new features
- [ ] Error handling is comprehensive

## License

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Copyright (c) 2025 Nik Ogura


