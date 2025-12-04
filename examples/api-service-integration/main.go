package main

// Example: Integrating mimic-proxy into api-service for Aiprise KYC
//
// This example demonstrates how to embed mimic-proxy as a library within
// the api-service to provide transparent proxying for Aiprise identity
// verification endpoints.
//
// Usage:
//   AIPRISE_API_KEY=your-key-here go run main.go

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nikogura/mimic-proxy/pkg/mimicproxy"
)

func main() {
	// Configure transparent proxy for Aiprise KYC endpoints
	aipriseProxyConfig := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				// Route 1: Aiprise verification session endpoints
				Name:               "aiprise-verify",
				PathPrefix:         "/id-verification/v1/verify",
				Upstream:           "https://api.aiprise.com",
				UpstreamPathPrefix: "/api/v1/verify",
				PreserveHost:       false, // Replace Host header with upstream
				Headers: mimicproxy.HeaderConfig{
					// Strip proxy-identifying headers from client request
					StripIncoming: []string{
						"X-Forwarded-*",
						"X-Real-IP",
						"Via",
						"Forwarded",
						"X-Request-Id",
						"X-Envoy-*",
					},
					// Strip server-identifying headers from upstream response
					StripOutgoing: []string{
						"Server",
						"X-Powered-By",
						"X-Runtime",
						"X-Envoy-*",
					},
					// Add Aiprise API key for authentication
					AddUpstream: map[string]string{
						"X-API-Key": os.Getenv("AIPRISE_API_KEY"),
					},
				},
				Timeout: 30 * time.Second,
				TLSMode: "terminate",
			},
			{
				// Route 2: Aiprise callback verification (for HMAC signature verification)
				Name:               "aiprise-callback",
				PathPrefix:         "/id-verification/v1/aiprise-callback",
				Upstream:           "https://api.aiprise.com",
				UpstreamPathPrefix: "/api/v1/callback",
				PreserveHost:       false,
				Headers: mimicproxy.HeaderConfig{
					StripIncoming: []string{"X-Forwarded-*", "Via"},
					StripOutgoing: []string{"Server", "X-Powered-By"},
					AddUpstream: map[string]string{
						"X-API-Key": os.Getenv("AIPRISE_API_KEY"),
					},
				},
				Timeout: 10 * time.Second,
				TLSMode: "terminate",
			},
		},
		Transport: mimicproxy.TransportConfig{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			DialTimeout:           5 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
		},
		TLS: mimicproxy.TLSConfig{
			// Downstream (client-facing) TLS
			CertFile: "/etc/api-service/tls/cert.pem",
			KeyFile:  "/etc/api-service/tls/key.pem",
			// Upstream (Aiprise) TLS verification
			CAFile:             "/etc/ssl/certs/ca-certificates.crt",
			InsecureSkipVerify: false, // NEVER true in production
			MinVersion:         "1.2",
		},
		Metrics: mimicproxy.MetricsConfig{
			Enabled:   true,
			Path:      "/metrics",
			Namespace: "api_service_aiprise_proxy",
		},
		Logger: mimicproxy.LoggerConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}

	// Create the Aiprise proxy instance
	aipriseProxy, err := mimicproxy.New(aipriseProxyConfig)
	if err != nil {
		log.Fatalf("Failed to create Aiprise proxy: %v", err)
	}
	defer aipriseProxy.Close()

	// Set up HTTP routing
	mux := http.NewServeMux()

	// Mount Aiprise proxy at /id-verification/v1/*
	mux.Handle("/id-verification/v1/", aipriseProxy)

	// Other API service routes
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)
	mux.HandleFunc("/orders/v1/place", placeOrderHandler)
	mux.HandleFunc("/orders/v1/status", orderStatusHandler)

	// Start server
	addr := ":8080"
	log.Printf("API service with Aiprise proxy running on %s", addr)
	log.Printf("Aiprise endpoints available at:")
	log.Printf("  - https://api.example.com/id-verification/v1/verify/*")
	log.Printf("  - https://api.example.com/id-verification/v1/aiprise-callback")
	log.Fatal(http.ListenAndServe(addr, mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"healthy"}`))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func placeOrderHandler(w http.ResponseWriter, r *http.Request) {
	// Placeholder for actual order placement logic
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"order_id":"12345","status":"placed"}`))
}

func orderStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Placeholder for actual order status logic
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"order_id":"12345","status":"filled"}`))
}
