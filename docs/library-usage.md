# Library Usage Guide

This guide explains how to use mimic-proxy as a library within your Go applications.

## Installation

```bash
go get github.com/nikogura/mimic-proxy/pkg/mimicproxy
```

## Basic Usage

### Minimal Configuration

```go
package main

import (
    "log"
    "net/http"
    "os"

    "github.com/nikogura/mimic-proxy/pkg/mimicproxy"
)

func main() {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "example",
                PathPrefix: "/api",
                Upstream:   "https://api.example.com",
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*"},
                    AddUpstream:   map[string]string{"X-API-Key": os.Getenv("API_KEY")},
                },
            },
        },
    }

    proxy, err := mimicproxy.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer proxy.Close()

    http.ListenAndServe(":8080", proxy)
}
```

## Embedding in Existing HTTP Server

### Method 1: Mount at Specific Path

```go
func main() {
    // Create your existing HTTP mux
    mux := http.NewServeMux()

    // Add your existing routes
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/api/v1/orders", ordersHandler)

    // Create mimic-proxy for specific path
    aipriseProxy, err := createAipriseProxy()
    if err != nil {
        log.Fatal(err)
    }
    defer aipriseProxy.Close()

    // Mount proxy at specific path
    mux.Handle("/id-verification/", aipriseProxy)

    http.ListenAndServe(":8080", mux)
}

func createAipriseProxy() (proxy *mimicproxy.Proxy, err error) {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "aiprise",
                PathPrefix: "/id-verification",
                Upstream:   "https://api.aiprise.com",
                Headers: mimicproxy.HeaderConfig{
                    StripIncoming: []string{"X-Forwarded-*", "Via"},
                    AddUpstream:   map[string]string{"X-API-Key": os.Getenv("AIPRISE_API_KEY")},
                },
            },
        },
    }

    return mimicproxy.New(config)
}
```

### Method 2: Multiple Proxies for Different Upstreams

```go
func main() {
    mux := http.NewServeMux()

    // Proxy 1: Aiprise KYC
    aipriseProxy, err := createAipriseProxy()
    if err != nil {
        log.Fatal(err)
    }
    defer aipriseProxy.Close()
    mux.Handle("/id-verification/", aipriseProxy)

    // Proxy 2: Binance CEX
    binanceProxy, err := createBinanceProxy()
    if err != nil {
        log.Fatal(err)
    }
    defer binanceProxy.Close()
    mux.Handle("/binance/", binanceProxy)

    // Proxy 3: OKX CEX
    okxProxy, err := createOKXProxy()
    if err != nil {
        log.Fatal(err)
    }
    defer okxProxy.Close()
    mux.Handle("/okx/", okxProxy)

    http.ListenAndServe(":8080", mux)
}
```

## Configuration Patterns

### Perfect Transparency Pattern

Strip all proxy headers for complete transparency:

```go
headers := mimicproxy.HeaderConfig{
    StripIncoming: []string{
        "X-Forwarded-*",
        "X-Real-IP",
        "Via",
        "Forwarded",
        "X-Request-Id",
        "X-Envoy-*",
        "CF-*",
    },
    StripOutgoing: []string{
        "Server",
        "X-Powered-By",
        "X-Runtime",
        "X-Envoy-*",
    },
}
```

### API Key Injection Pattern

Add authentication headers for upstream:

```go
headers := mimicproxy.HeaderConfig{
    StripIncoming: []string{"Authorization"}, // Strip client auth
    AddUpstream: map[string]string{
        "X-API-Key":     os.Getenv("UPSTREAM_API_KEY"),
        "Authorization": "Bearer " + os.Getenv("UPSTREAM_TOKEN"),
    },
}
```

### Header Replacement Pattern

Replace specific headers:

```go
headers := mimicproxy.HeaderConfig{
    ReplaceIncoming: map[string]string{
        "User-Agent": "Terrace-Proxy/1.0",
    },
    ReplaceOutgoing: map[string]string{
        "Server": "Terrace",
    },
}
```

## Advanced Configuration

### Custom Transport Settings

```go
config := &mimicproxy.Config{
    Routes: []*mimicproxy.RouteConfig{
        // ... routes
    },
    Transport: mimicproxy.TransportConfig{
        MaxIdleConns:          200,
        MaxIdleConnsPerHost:   20,
        IdleConnTimeout:       90 * time.Second,
        DialTimeout:           5 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
        DisableKeepAlives:     false,
        DisableCompression:    false,
    },
}
```

### TLS Configuration

```go
config := &mimicproxy.Config{
    Routes: []*mimicproxy.RouteConfig{
        // ... routes
    },
    TLS: mimicproxy.TLSConfig{
        CertFile:           "/etc/tls/cert.pem",
        KeyFile:            "/etc/tls/key.pem",
        CAFile:             "/etc/tls/ca.pem",
        InsecureSkipVerify: false,
        MinVersion:         "1.2",
        CipherSuites: []string{
            "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
            "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
        },
    },
}
```

### Metrics Integration

```go
config := &mimicproxy.Config{
    Routes: []*mimicproxy.RouteConfig{
        // ... routes
    },
    Metrics: mimicproxy.MetricsConfig{
        Enabled:   true,
        Path:      "/metrics",
        Namespace: "my_app_proxy",
    },
}

proxy, err := mimicproxy.New(config)
if err != nil {
    log.Fatal(err)
}

// Metrics exposed at /metrics endpoint automatically
```

### Custom Logging

```go
config := &mimicproxy.Config{
    Routes: []*mimicproxy.RouteConfig{
        // ... routes
    },
    Logger: mimicproxy.LoggerConfig{
        Level:  "debug",
        Format: "json",
        Output: "/var/log/proxy.log",
    },
}
```

## Error Handling

### Checking Configuration Validity

```go
config := &mimicproxy.Config{
    // ... configuration
}

if err := config.Validate(); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}

proxy, err := mimicproxy.New(config)
if err != nil {
    log.Fatalf("Failed to create proxy: %v", err)
}
```

### Handling Upstream Errors

Mimic-proxy automatically handles upstream errors:
- Connection failures → 502 Bad Gateway
- Timeouts → 504 Gateway Timeout
- 5xx errors → Passed through to client

All errors are logged and recorded in metrics.

### Graceful Shutdown

```go
proxy, err := mimicproxy.New(config)
if err != nil {
    log.Fatal(err)
}

// Ensure cleanup on shutdown
defer proxy.Close()

// Or handle graceful shutdown signals
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down proxy...")
    proxy.Close()
    os.Exit(0)
}()

http.ListenAndServe(":8080", proxy)
```

## Testing Your Integration

### Unit Testing

```go
func TestProxyConfiguration(t *testing.T) {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "test",
                PathPrefix: "/api",
                Upstream:   "https://example.com",
            },
        },
    }

    if err := config.Validate(); err != nil {
        t.Fatalf("Configuration validation failed: %v", err)
    }
}
```

### Integration Testing

```go
func TestProxyIntegration(t *testing.T) {
    // Start test upstream server
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    }))
    defer upstream.Close()

    // Create proxy pointing to test server
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

## Performance Considerations

### Connection Pooling

The proxy maintains connection pools to upstream servers. Configure based on your traffic:

```go
// Low traffic (< 100 RPS)
Transport: mimicproxy.TransportConfig{
    MaxIdleConns:        50,
    MaxIdleConnsPerHost: 5,
}

// Medium traffic (100-1000 RPS)
Transport: mimicproxy.TransportConfig{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
}

// High traffic (> 1000 RPS)
Transport: mimicproxy.TransportConfig{
    MaxIdleConns:        200,
    MaxIdleConnsPerHost: 20,
}
```

### Memory Usage

The proxy streams request/response bodies without buffering. Memory usage scales primarily with:
- Number of concurrent connections
- Connection pool size
- Header manipulation complexity

Expected memory: 10-50MB baseline + (1-5MB per 1000 concurrent connections)

### Latency

Expected overhead: 0.5-1ms per request

Factors affecting latency:
- Header manipulation complexity
- TLS termination and re-encryption
- Connection pool efficiency
- Network proximity to upstream

## Common Integration Patterns

### Pattern 1: API Gateway

Use mimic-proxy as a transparent API gateway:

```go
func setupAPIGateway() (handler http.Handler, err error) {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {Name: "service-a", PathPrefix: "/service-a", Upstream: "https://service-a.internal"},
            {Name: "service-b", PathPrefix: "/service-b", Upstream: "https://service-b.internal"},
            {Name: "service-c", PathPrefix: "/service-c", Upstream: "https://service-c.internal"},
        },
    }
    return mimicproxy.New(config)
}
```

### Pattern 2: Development Proxy

Proxy external APIs during development:

```go
func setupDevProxy() (handler http.Handler, err error) {
    config := &mimicproxy.Config{
        Routes: []*mimicproxy.RouteConfig{
            {
                Name:       "production-api",
                PathPrefix: "/api",
                Upstream:   "https://api.production.com",
                Headers: mimicproxy.HeaderConfig{
                    AddUpstream: map[string]string{
                        "X-Environment": "development",
                    },
                },
            },
        },
    }
    return mimicproxy.New(config)
}
```

### Pattern 3: Load Balancer (Future)

Note: Multiple upstreams per route not yet implemented, but planned for Phase 5.

## Troubleshooting

### Issue: Headers Still Present

**Problem:** Proxy headers like X-Forwarded-For still appearing in upstream requests.

**Solution:** Ensure `StripIncoming` includes the header pattern:

```go
Headers: mimicproxy.HeaderConfig{
    StripIncoming: []string{"X-Forwarded-*"}, // Matches X-Forwarded-For, X-Forwarded-Proto, etc.
}
```

### Issue: Environment Variables Not Expanding

**Problem:** `${API_KEY}` appearing literally instead of expanding.

**Solution:** Ensure environment variable exists before starting proxy:

```bash
export API_KEY=your-key-here
go run main.go
```

### Issue: TLS Certificate Errors

**Problem:** `x509: certificate signed by unknown authority`

**Solution:** Specify CA bundle for upstream verification:

```go
TLS: mimicproxy.TLSConfig{
    CAFile: "/etc/ssl/certs/ca-certificates.crt",
}
```

### Issue: Connection Pool Exhaustion

**Problem:** Proxy becomes slow under load.

**Solution:** Increase connection pool limits:

```go
Transport: mimicproxy.TransportConfig{
    MaxIdleConns:        200,
    MaxIdleConnsPerHost: 20,
}
```

## Next Steps

- See [configuration.md](configuration.md) for complete configuration reference
- See [header-manipulation.md](header-manipulation.md) for advanced header patterns
- See [standalone-usage.md](standalone-usage.md) for running the standalone binary
