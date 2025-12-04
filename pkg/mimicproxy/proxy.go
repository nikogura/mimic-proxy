package mimicproxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Proxy is a transparent reverse proxy that provides perfect transparency
// between clients and upstream servers.
type Proxy struct {
	config    *Config
	routes    []*Route
	transport *http.Transport
	logger    Logger
}

// New creates a new Proxy instance with the given configuration.
func New(config *Config) (proxy *Proxy, err error) {
	// Apply defaults
	config.ApplyDefaults()

	// Validate configuration
	err = config.Validate()
	if err != nil {
		err = fmt.Errorf("configuration validation failed: %w", err)
		return proxy, err
	}

	// Create logger
	var logger Logger
	if config.Logger.Level == "" || config.Logger.Level == "none" {
		logger = &NoOpLogger{}
	} else {
		var logLevel LogLevel
		switch config.Logger.Level {
		case "debug":
			logLevel = LogLevelDebug
		case "info":
			logLevel = LogLevelInfo
		case "warn":
			logLevel = LogLevelWarn
		case "error":
			logLevel = LogLevelError
		default:
			logLevel = LogLevelInfo
		}
		logger = NewStandardLogger(logLevel)
	}

	// Create TLS configuration for upstream connections
	var tlsConfig *tls.Config
	if config.TLS.CAFile != "" || config.TLS.InsecureSkipVerify {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: config.TLS.InsecureSkipVerify,
		}
	}

	// Create HTTP transport
	var transport *http.Transport
	transport, err = NewTransport(&config.Transport, tlsConfig)
	if err != nil {
		err = fmt.Errorf("failed to create transport: %w", err)
		return proxy, err
	}

	proxy = &Proxy{
		config:    config,
		routes:    make([]*Route, 0, len(config.Routes)),
		transport: transport,
		logger:    logger,
	}

	// Log proxy initialization
	logger.Info("Initializing mimic-proxy",
		"num_routes", len(config.Routes),
		"metrics_enabled", config.Metrics.Enabled)

	// Create routes
	for _, routeConfig := range config.Routes {
		var route *Route
		route, err = NewRoute(routeConfig, transport, logger)
		if err != nil {
			err = fmt.Errorf("failed to create route %s: %w", routeConfig.Name, err)
			return proxy, err
		}
		proxy.routes = append(proxy.routes, route)
		logger.Debug("Created route",
			"name", routeConfig.Name,
			"path_prefix", routeConfig.PathPrefix,
			"upstream", routeConfig.Upstream)
	}

	// Sort routes by path prefix length (longest first) for correct matching
	sortRoutesByPrefixLength(proxy.routes)

	logger.Info("Mimic-proxy initialized successfully")

	return proxy, err
}

// ServeHTTP implements http.Handler for use in HTTP servers.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Find matching route
	var matchedRoute *Route
	for _, route := range p.routes {
		if route.Match(r) {
			matchedRoute = route
			break
		}
	}

	if matchedRoute == nil {
		p.logger.Warn("No matching route found",
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)

		if p.config.Metrics.Enabled {
			ProxyRequestErrorsTotal.WithLabelValues("none", r.Method).Inc()
		}

		http.Error(w, "No route found", http.StatusNotFound)
		return
	}

	routeName := matchedRoute.config.Name

	p.logger.Debug("Handling request",
		"route", routeName,
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)

	// Track metrics if enabled
	if p.config.Metrics.Enabled {
		ProxyRequestsTotal.WithLabelValues(routeName, r.Method).Inc()
	}

	// If redirect rewriting is enabled, wrap the response writer
	if matchedRoute.config.RewriteRedirects {
		// Determine incoming scheme
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
			scheme = forwardedProto
		}

		// Wrap response writer to intercept redirects
		wrappedWriter := &redirectRewritingResponseWriter{
			ResponseWriter: w,
			route:          matchedRoute,
			routes:         p.routes,
			incomingHost:   r.Host,
			incomingScheme: scheme,
			logger:         p.logger,
			metricsEnabled: p.config.Metrics.Enabled,
		}
		w = wrappedWriter
	}

	// Wrap response writer to capture status code
	statusWriter := &statusCapturingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Proxy the request
	matchedRoute.reverseProxy.ServeHTTP(statusWriter, r)

	// Record metrics and log completion
	duration := time.Since(startTime)

	if p.config.Metrics.Enabled {
		ProxyRequestDuration.WithLabelValues(routeName, r.Method).Observe(duration.Seconds())
		ProxyResponsesTotal.WithLabelValues(routeName, r.Method, strconv.Itoa(statusWriter.statusCode)).Inc()
	}

	// Log completion at appropriate level based on status code
	switch {
	case statusWriter.statusCode >= 500:
		p.logger.Error("Request completed",
			"route", routeName,
			"path", r.URL.Path,
			"method", r.Method,
			"status", statusWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr)
	case statusWriter.statusCode >= 400:
		p.logger.Warn("Request completed",
			"route", routeName,
			"path", r.URL.Path,
			"method", r.Method,
			"status", statusWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr)
	default:
		p.logger.Debug("Request completed",
			"route", routeName,
			"path", r.URL.Path,
			"method", r.Method,
			"status", statusWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr)
	}
}

// Close gracefully shuts down the proxy, closing all connections.
func (p *Proxy) Close() (err error) {
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
	return err
}

// statusCapturingResponseWriter wraps http.ResponseWriter to capture the status code.
type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

// WriteHeader captures the status code.
func (w *statusCapturingResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.statusCode = statusCode
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the status code if not already written.
func (w *statusCapturingResponseWriter) Write(data []byte) (n int, err error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err = w.ResponseWriter.Write(data)
	return n, err
}

// redirectRewritingResponseWriter wraps http.ResponseWriter to intercept
// and rewrite redirect responses.
type redirectRewritingResponseWriter struct {
	http.ResponseWriter
	route          *Route
	routes         []*Route
	incomingHost   string
	incomingScheme string
	logger         Logger
	metricsEnabled bool
	wroteHeader    bool
}

// WriteHeader intercepts the status code and rewrites Location header for redirects.
func (rw *redirectRewritingResponseWriter) WriteHeader(statusCode int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true

	// Handle redirect rewriting if applicable
	if isRedirect(statusCode) {
		rw.handleRedirectRewrite()
	}

	// Apply outgoing header manipulations
	rw.applyHeaderManipulations()

	rw.ResponseWriter.WriteHeader(statusCode)
}

// handleRedirectRewrite processes and rewrites redirect Location headers.
func (rw *redirectRewritingResponseWriter) handleRedirectRewrite() {
	location := rw.Header().Get("Location")
	if location == "" {
		return
	}

	// Attempt to rewrite the redirect
	rewrittenLocation, rewritten, rewriteType := RewriteRedirect(
		location,
		rw.incomingHost,
		rw.incomingScheme,
		routesToConfigs(rw.routes),
		rw.route.config,
	)

	if rewritten {
		rw.logSuccessfulRewrite(location, rewrittenLocation, rewriteType)
		rw.Header().Set("Location", rewrittenLocation)
		return
	}

	if rewriteType == "external_unknown" {
		rw.logUnknownExternalRedirect(location)
	}
}

// logSuccessfulRewrite logs a successful redirect rewrite and updates metrics.
func (rw *redirectRewritingResponseWriter) logSuccessfulRewrite(
	original string,
	rewritten string,
	rewriteType string,
) {
	rw.logger.Info("Rewrote redirect",
		"route", rw.route.config.Name,
		"original", original,
		"rewritten", rewritten,
		"type", rewriteType)

	if rw.metricsEnabled {
		ProxyRedirectRewritesTotal.WithLabelValues(rw.route.config.Name, rewriteType).Inc()
	}
}

// logUnknownExternalRedirect logs when a redirect points to an unknown external service.
func (rw *redirectRewritingResponseWriter) logUnknownExternalRedirect(location string) {
	rw.logger.Warn("Redirect to unknown external service",
		"route", rw.route.config.Name,
		"location", location)

	if rw.metricsEnabled {
		ProxyRedirectRewritesTotal.WithLabelValues(rw.route.config.Name, "external_unknown").Inc()
	}
}

// applyHeaderManipulations applies outgoing header transformations.
func (rw *redirectRewritingResponseWriter) applyHeaderManipulations() {
	processedHeaders := rw.route.headerManipulator.ProcessOutgoing(rw.Header())

	// Clear existing headers and set processed ones
	for key := range rw.Header() {
		rw.Header().Del(key)
	}
	for key, values := range processedHeaders {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}
}

// Write writes the response body.
func (rw *redirectRewritingResponseWriter) Write(data []byte) (n int, err error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err = rw.ResponseWriter.Write(data)
	return n, err
}

// isRedirect checks if a status code is a redirect.
func isRedirect(statusCode int) (redirect bool) {
	redirect = statusCode == http.StatusMovedPermanently ||
		statusCode == http.StatusFound ||
		statusCode == http.StatusSeeOther ||
		statusCode == http.StatusTemporaryRedirect ||
		statusCode == http.StatusPermanentRedirect
	return redirect
}

// routesToConfigs converts []*Route to []*RouteConfig for RewriteRedirect.
func routesToConfigs(routes []*Route) (configs []*RouteConfig) {
	configs = make([]*RouteConfig, len(routes))
	for i, route := range routes {
		configs[i] = route.config
	}
	return configs
}

// sortRoutesByPrefixLength sorts routes by path prefix length (longest first) for correct matching.
func sortRoutesByPrefixLength(routes []*Route) {
	sort.Slice(routes, func(i, j int) (less bool) {
		less = len(routes[i].config.PathPrefix) > len(routes[j].config.PathPrefix)
		return less
	})
}
