package mimicproxy

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// HeaderManipulator handles header transformation rules.
type HeaderManipulator struct {
	config    *HeaderConfig
	routeName string
	logger    Logger
}

// NewHeaderManipulator creates a new header manipulator.
func NewHeaderManipulator(config *HeaderConfig, routeName string, logger Logger) (hm *HeaderManipulator) {
	hm = &HeaderManipulator{
		config:    config,
		routeName: routeName,
		logger:    logger,
	}
	return hm
}

// ProcessIncoming applies header rules to client request before forwarding.
// Returns a new http.Header with transformations applied.
func (hm *HeaderManipulator) ProcessIncoming(inHeader http.Header) (outHeader http.Header) {
	outHeader = hm.processHeaders(
		inHeader,
		hm.config.StripIncoming,
		hm.config.ReplaceIncoming,
		hm.config.AddUpstream,
		"incoming",
		"upstream",
	)
	return outHeader
}

// ProcessOutgoing applies header rules to upstream response before returning.
// Returns a new http.Header with transformations applied.
func (hm *HeaderManipulator) ProcessOutgoing(inHeader http.Header) (outHeader http.Header) {
	outHeader = hm.processHeaders(
		inHeader,
		hm.config.StripOutgoing,
		hm.config.ReplaceOutgoing,
		hm.config.AddDownstream,
		"outgoing",
		"downstream",
	)
	return outHeader
}

// processHeaders is a helper function that processes headers according to the given rules.
func (hm *HeaderManipulator) processHeaders(
	inHeader http.Header,
	stripPatterns []string,
	replaceHeaders map[string]string,
	addHeaders map[string]string,
	direction string,
	addDirection string,
) (outHeader http.Header) {
	outHeader = make(http.Header)

	// Copy all headers first
	for key, values := range inHeader {
		outHeader[key] = values
	}

	// Count stripped headers for metrics
	originalCount := len(outHeader)

	// Strip headers matching patterns
	outHeader = stripHeaders(outHeader, stripPatterns)
	strippedCount := originalCount - len(outHeader)

	if strippedCount > 0 {
		hm.logger.Debug("Stripped "+direction+" headers",
			"route", hm.routeName,
			"count", strippedCount)
	}

	// Replace headers
	for key, value := range replaceHeaders {
		outHeader.Set(key, value)
		hm.logger.Debug("Replaced "+direction+" header",
			"route", hm.routeName,
			"header", key)
	}

	// Add headers with environment variable expansion
	addedCount := 0
	for key, value := range addHeaders {
		expanded := expandEnvVars(value)
		outHeader.Set(key, expanded)
		addedCount++
	}

	if addedCount > 0 {
		hm.logger.Debug("Added "+addDirection+" headers",
			"route", hm.routeName,
			"count", addedCount)
	}

	return outHeader
}

// stripHeaders removes headers matching patterns (supports wildcards).
func stripHeaders(header http.Header, patterns []string) (result http.Header) {
	result = make(http.Header)

	for key, values := range header {
		shouldStrip := false

		for _, pattern := range patterns {
			if matchesPattern(key, pattern) {
				shouldStrip = true
				break
			}
		}

		if !shouldStrip {
			result[key] = values
		}
	}

	return result
}

// matchesPattern checks if a header name matches a pattern (supports "*" wildcard).
func matchesPattern(headerName, pattern string) (matches bool) {
	// Case-insensitive comparison
	headerName = strings.ToLower(headerName)
	pattern = strings.ToLower(pattern)

	// Exact match
	if headerName == pattern {
		matches = true
		return matches
	}

	// Wildcard match
	if strings.Contains(pattern, "*") {
		// Simple wildcard: "x-forwarded-*" matches "x-forwarded-for", "x-forwarded-proto", etc.
		prefix := strings.TrimSuffix(pattern, "*")
		matches = strings.HasPrefix(headerName, prefix)
		return matches
	}

	return matches
}

// expandEnvVars expands environment variables in header values.
// Supports ${VAR_NAME} syntax.
func expandEnvVars(value string) (expanded string) {
	expanded = value

	// Find and replace all ${VAR} patterns
	start := 0
	for {
		idx := strings.Index(expanded[start:], "${")
		if idx == -1 {
			break
		}
		idx += start

		endIdx := strings.Index(expanded[idx:], "}")
		if endIdx == -1 {
			// Unclosed variable reference, leave as-is
			break
		}
		endIdx += idx

		varName := expanded[idx+2 : endIdx]
		varValue := os.Getenv(varName)

		// Replace ${VAR} with value
		expanded = expanded[:idx] + varValue + expanded[endIdx+1:]

		// Continue searching after the replacement
		start = idx + len(varValue)
	}

	return expanded
}

// RewriteRedirect rewrites a Location header in a redirect response to route through the proxy.
// Returns the rewritten location, whether it was rewritten, and the rewrite type.
func RewriteRedirect(
	location string,
	incomingHost string,
	incomingScheme string,
	routes []*RouteConfig,
	currentRoute *RouteConfig,
) (rewrittenLocation string, rewritten bool, rewriteType string) {
	// Parse the location URL
	var locationURL *url.URL
	var err error

	locationURL, err = url.Parse(location)
	if err != nil {
		// Invalid URL, leave as-is
		rewrittenLocation = location
		rewritten = false
		rewriteType = "invalid_url"
		return rewrittenLocation, rewritten, rewriteType
	}

	// If redirect is relative, no rewriting needed
	if !locationURL.IsAbs() {
		rewrittenLocation = location
		rewritten = false
		rewriteType = "relative"
		return rewrittenLocation, rewritten, rewriteType
	}

	// Parse current route's upstream URL
	var currentUpstreamURL *url.URL
	currentUpstreamURL, err = url.Parse(currentRoute.Upstream)
	if err != nil {
		rewrittenLocation = location
		rewritten = false
		rewriteType = "invalid_upstream"
		return rewrittenLocation, rewritten, rewriteType
	}

	// Check if redirect is to the same host (internal redirect)
	if locationURL.Host == currentUpstreamURL.Host {
		// Internal redirect on same upstream
		// Rewrite path to route through proxy
		rewrittenLocation = buildProxyURL(
			incomingScheme,
			incomingHost,
			currentRoute,
			locationURL.Path,
			locationURL.RawQuery,
			locationURL.Fragment,
		)
		rewritten = true
		rewriteType = "internal"
		return rewrittenLocation, rewritten, rewriteType
	}

	// Check if redirect points to another known upstream
	for _, route := range routes {
		var routeUpstreamURL *url.URL
		routeUpstreamURL, err = url.Parse(route.Upstream)
		if err != nil {
			continue
		}

		// Check if redirect host matches this route's upstream host
		if locationURL.Host == routeUpstreamURL.Host {
			// Found matching route, rewrite to route through proxy
			rewrittenLocation = buildProxyURL(
				incomingScheme,
				incomingHost,
				route,
				locationURL.Path,
				locationURL.RawQuery,
				locationURL.Fragment,
			)
			rewritten = true
			rewriteType = "external_known"
			return rewrittenLocation, rewritten, rewriteType
		}
	}

	// Redirect points to unknown external service
	// Leave as-is but mark as unknown
	rewrittenLocation = location
	rewritten = false
	rewriteType = "external_unknown"
	return rewrittenLocation, rewritten, rewriteType
}

// buildProxyURL constructs a rewritten URL that routes through the proxy.
func buildProxyURL(
	scheme string,
	host string,
	route *RouteConfig,
	path string,
	query string,
	fragment string,
) (proxyURL string) {
	// Use RedirectBaseURL if configured, otherwise use incoming host
	if route.RedirectBaseURL != "" {
		// Parse the base URL to get scheme and host
		var baseURL *url.URL
		var err error
		baseURL, err = url.Parse(route.RedirectBaseURL)
		if err == nil {
			scheme = baseURL.Scheme
			host = baseURL.Host
		}
	}

	// Start with scheme and host
	if scheme == "" {
		scheme = "https"
	}
	proxyURL = scheme + "://" + host

	// Add route path prefix
	proxyURL += route.PathPrefix

	// Rewrite path if upstream path prefix is configured
	if route.UpstreamPathPrefix != "" {
		// Remove upstream path prefix from path if present
		path = strings.TrimPrefix(path, route.UpstreamPathPrefix)
	}

	// Ensure path starts with /
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Add path (but avoid double slashes)
	if strings.HasSuffix(proxyURL, "/") && strings.HasPrefix(path, "/") {
		proxyURL += path[1:]
	} else if !strings.HasSuffix(proxyURL, "/") && !strings.HasPrefix(path, "/") {
		proxyURL += "/" + path
	} else {
		proxyURL += path
	}

	// Add query string
	if query != "" {
		proxyURL += "?" + query
	}

	// Add fragment
	if fragment != "" {
		proxyURL += "#" + fragment
	}

	return proxyURL
}
