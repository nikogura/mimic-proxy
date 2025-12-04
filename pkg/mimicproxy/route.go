package mimicproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Route represents a compiled route from client to upstream.
type Route struct {
	config            *RouteConfig
	upstream          *url.URL
	reverseProxy      *httputil.ReverseProxy
	headerManipulator *HeaderManipulator
	logger            Logger
}

// NewRoute creates a new route from configuration.
func NewRoute(config *RouteConfig, transport *http.Transport, logger Logger) (route *Route, err error) {
	// Parse upstream URL
	var upstreamURL *url.URL
	upstreamURL, err = url.Parse(config.Upstream)
	if err != nil {
		return route, err
	}

	route = &Route{
		config:            config,
		upstream:          upstreamURL,
		headerManipulator: NewHeaderManipulator(&config.Headers, config.Name, logger),
		logger:            logger,
	}

	// Wrap transport to ensure headers are stripped after ReverseProxy processes them
	wrappedTransport := &headerStrippingTransport{
		base:  transport,
		route: route,
	}

	// Create reverse proxy with custom director
	route.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			route.director(req)
		},
		Transport: wrappedTransport,
	}

	return route, err
}

// headerStrippingTransport wraps http.RoundTripper to ensure headers
// are properly stripped even after ReverseProxy adds its own headers.
type headerStrippingTransport struct {
	base  http.RoundTripper
	route *Route
}

// RoundTrip implements http.RoundTripper.
func (t *headerStrippingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	// Strip headers one more time right before sending
	// This catches any headers that ReverseProxy added after Director ran
	for _, pattern := range t.route.config.Headers.StripIncoming {
		if pattern == "X-Forwarded-*" {
			// Remove all X-Forwarded-* headers
			for key := range req.Header {
				if matchesPattern(key, pattern) {
					req.Header.Del(key)
				}
			}
		}
	}

	resp, err = t.base.RoundTrip(req)
	return resp, err
}

// Match returns true if this route should handle the given request.
func (r *Route) Match(req *http.Request) (matched bool) {
	matched = strings.HasPrefix(req.URL.Path, r.config.PathPrefix)
	return matched
}

// director modifies the request before forwarding to upstream.
func (r *Route) director(req *http.Request) {
	// Apply header manipulations FIRST (before ReverseProxy adds its own headers)
	req.Header = r.headerManipulator.ProcessIncoming(req.Header)

	// Set upstream target
	req.URL.Scheme = r.upstream.Scheme
	req.URL.Host = r.upstream.Host

	// Rewrite path if upstream path prefix is configured
	if r.config.UpstreamPathPrefix != "" {
		// Remove route path prefix and add upstream path prefix
		path := strings.TrimPrefix(req.URL.Path, r.config.PathPrefix)
		req.URL.Path = r.config.UpstreamPathPrefix + path

		// Clean up double slashes (e.g., "//health" -> "/health")
		if strings.HasPrefix(req.URL.Path, "//") {
			req.URL.Path = req.URL.Path[1:]
		}
	}

	// Set Host header
	if !r.config.PreserveHost {
		req.Host = r.upstream.Host
	}

	// Remove hop-by-hop headers
	removeHopByHopHeaders(req.Header)

	// ReverseProxy will add X-Forwarded-For after this function returns
	// We need to remove it if it's in our strip list
	// This is handled by setting X-Forwarded-For to empty if needed
	if r.shouldStripHeader("X-Forwarded-For") {
		// Clear X-Forwarded-For to prevent ReverseProxy from adding it
		req.Header.Del("X-Forwarded-For")
	}
}

// shouldStripHeader checks if a header should be stripped based on strip patterns.
func (r *Route) shouldStripHeader(headerName string) (should bool) {
	for _, pattern := range r.config.Headers.StripIncoming {
		if matchesPattern(headerName, pattern) {
			should = true
			return should
		}
	}
	return should
}

// removeHopByHopHeaders removes hop-by-hop headers from request.
// These headers are connection-specific and should not be forwarded.
func removeHopByHopHeaders(header http.Header) {
	// Standard hop-by-hop headers defined in RFC 2616
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, h := range hopByHopHeaders {
		header.Del(h)
	}

	// Also remove headers listed in Connection header
	if connections := header.Get("Connection"); connections != "" {
		for _, connection := range strings.Split(connections, ",") {
			header.Del(strings.TrimSpace(connection))
		}
	}
}
