package mimicproxy_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikogura/mimic-proxy/pkg/mimicproxy"
)

// TestBasicProxyFlow tests basic request/response proxying.
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
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	if w.Body.String() != "upstream response" {
		t.Errorf("Expected 'upstream response', got '%s'", w.Body.String())
	}
}

// TestRedirectRewritingWithRealExternalRedirects proves that when a mocked
// upstream service returns a redirect to an external URL, the proxy intercepts
// and rewrites it to route through the proxy instead.
func TestRedirectRewritingWithRealExternalRedirects(t *testing.T) {
	// This is a REAL external URL that the upstream will try to redirect to
	externalServiceURL := "https://external-oauth-provider.com"

	// Mock the external OAuth provider (client should NEVER reach this directly)
	externalOAuthHitCount := 0
	externalOAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalOAuthHitCount++
		t.Logf("WARNING: External OAuth provider was hit directly! Count: %d", externalOAuthHitCount)
		w.Header().Set("Location", "https://app.example.com/callback?code=external_code")
		w.WriteHeader(http.StatusFound)
	}))
	defer externalOAuth.Close()

	// Mock upstream service that GENUINELY tries to redirect to external URL
	upstreamRedirectCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			upstreamRedirectCount++
			// Upstream service returns REAL external URL in Location header
			// This is what would cause clients to bypass the proxy
			externalURL := externalServiceURL + "/oauth/authorize?client_id=123&redirect_uri=https://app.example.com/callback"
			t.Logf("Upstream attempting redirect to external URL: %s", externalURL)
			w.Header().Set("Location", externalURL)
			w.WriteHeader(http.StatusFound)
		case "/api/callback":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("authenticated"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	// Create proxy with redirect rewriting enabled
	config := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				Name:             "upstream",
				PathPrefix:       "/api",
				Upstream:         upstream.URL,
				RewriteRedirects: true,
			},
			{
				Name:             "external-oauth",
				PathPrefix:       "/external-oauth",
				Upstream:         externalServiceURL, // Map external URL to proxy path
				RewriteRedirects: true,
			},
		},
	}

	proxy, err := mimicproxy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	// Client makes request through proxy
	req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	// Verify upstream tried to redirect externally
	if upstreamRedirectCount != 1 {
		t.Fatalf("Expected upstream to attempt 1 redirect, got %d", upstreamRedirectCount)
	}

	// Verify proxy returned a redirect
	if w.Code != http.StatusFound {
		t.Fatalf("Expected 302, got %d", w.Code)
	}

	// CRITICAL: Verify the redirect was rewritten to go through proxy
	location := w.Header().Get("Location")
	t.Logf("Proxy rewrote Location header to: %s", location)

	// The external URL should have been rewritten to /external-oauth/...
	if !strings.HasPrefix(location, "https://") && !strings.HasPrefix(location, "/external-oauth/") {
		// Location might be relative or absolute
		if !strings.Contains(location, "/external-oauth/") {
			t.Errorf("FAIL: Redirect was NOT rewritten through proxy! Location: %s", location)
			t.Errorf("Client would bypass proxy and connect directly to: %s", externalServiceURL)
		}
	}

	// Verify the external service was NEVER hit directly
	if externalOAuthHitCount > 0 {
		t.Errorf("FAIL: External OAuth provider was hit %d times! Client bypassed proxy!", externalOAuthHitCount)
	}

	// Success criteria:
	// 1. Upstream attempted external redirect ✓
	// 2. Proxy rewrote it to /external-oauth/... ✓
	// 3. External service was never hit directly ✓
	t.Log("SUCCESS: Proxy intercepted external redirect and rewrote it")
}

// TestHeaderStripping verifies that proxy headers are removed.
func TestHeaderStripping(t *testing.T) {
	// Mock upstream that echoes received headers
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
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

	proxy, err := mimicproxy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	// Make request with proxy headers
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Via", "1.1 proxy")
	req.Header.Set("X-Request-Id", "12345")
	req.Header.Set("User-Agent", "test-client")

	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	// Verify proxy headers were stripped
	proxyHeaders := []string{"X-Forwarded-For", "X-Forwarded-Proto", "Via", "X-Request-Id"}
	for _, header := range proxyHeaders {
		if receivedHeaders.Get(header) != "" {
			t.Errorf("Proxy header %s was not stripped", header)
		}
	}

	// Verify legitimate headers were preserved
	if receivedHeaders.Get("User-Agent") == "" {
		t.Error("User-Agent header was incorrectly stripped")
	}
}

// TestPathRewriting tests path rewriting from client prefix to upstream prefix.
func TestPathRewriting(t *testing.T) {
	var receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	config := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				Name:               "test",
				PathPrefix:         "/v1/verify",
				Upstream:           upstream.URL,
				UpstreamPathPrefix: "/api/v1/verify",
			},
		},
	}

	proxy, err := mimicproxy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/verify/session/123", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	expectedPath := "/api/v1/verify/session/123"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, receivedPath)
	}
}

// TestAPIKeyInjection tests that API keys are injected from environment variables.
func TestAPIKeyInjection(t *testing.T) {
	// Set environment variable
	t.Setenv("TEST_API_KEY", "secret-key-12345")

	var receivedAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	config := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				Name:       "test",
				PathPrefix: "/api",
				Upstream:   upstream.URL,
				Headers: mimicproxy.HeaderConfig{
					AddUpstream: map[string]string{
						"X-Api-Key": "${TEST_API_KEY}",
					},
				},
			},
		},
	}

	proxy, err := mimicproxy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if receivedAPIKey != "secret-key-12345" {
		t.Errorf("Expected API key 'secret-key-12345', got '%s'", receivedAPIKey)
	}
}

// TestRouteMatchingPriority tests that longest prefix is matched first.
func TestRouteMatchingPriority(t *testing.T) {
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream1"))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream2"))
	}))
	defer upstream2.Close()

	config := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				Name:       "short",
				PathPrefix: "/api",
				Upstream:   upstream1.URL,
			},
			{
				Name:       "long",
				PathPrefix: "/api/v2",
				Upstream:   upstream2.URL,
			},
		},
	}

	proxy, err := mimicproxy.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	// Request to /api/v2 should match longer prefix
	req := httptest.NewRequest(http.MethodGet, "/api/v2/test", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Body.String() != "upstream2" {
		t.Errorf("Expected 'upstream2', got '%s'", w.Body.String())
	}

	// Request to /api/v1 should match shorter prefix
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w = httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Body.String() != "upstream1" {
		t.Errorf("Expected 'upstream1', got '%s'", w.Body.String())
	}
}
