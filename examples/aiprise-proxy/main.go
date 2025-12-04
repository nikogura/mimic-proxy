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
	// This example shows how to use mimic-proxy as a library to proxy Aiprise KYC requests
	config := &mimicproxy.Config{
		Routes: []*mimicproxy.RouteConfig{
			{
				Name:               "aiprise-verify",
				PathPrefix:         "/v1/verify",
				Upstream:           "https://api.aiprise.com",
				UpstreamPathPrefix: "/api/v1/verify",
				PreserveHost:       false,
				RewriteRedirects:   true, // Critical for OAuth flows
				Headers: mimicproxy.HeaderConfig{
					// Strip all proxy-identifying headers for perfect transparency
					StripIncoming: []string{
						"X-Forwarded-*",
						"Via",
						"X-Real-IP",
						"X-Request-Id",
						"X-Envoy-*",
					},
					StripOutgoing: []string{
						"Server",
						"X-Powered-By",
						"X-Runtime",
					},
					// Inject Aiprise API key from environment
					AddUpstream: map[string]string{
						"X-API-Key": "${AIPRISE_API_KEY}",
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
			DialTimeout:           10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
		},
	}

	// Validate environment variable
	if os.Getenv("AIPRISE_API_KEY") == "" {
		log.Fatal("AIPRISE_API_KEY environment variable not set")
	}

	// Create proxy
	proxy, err := mimicproxy.New(config)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	// Setup HTTP server
	mux := http.NewServeMux()

	// Mount proxy at /v1/verify
	mux.Handle("/v1/verify/", proxy)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Start server
	addr := ":8080"
	log.Printf("Aiprise proxy listening on %s", addr)
	log.Printf("Proxying /v1/verify/* -> https://api.aiprise.com/api/v1/verify/*")
	log.Fatal(http.ListenAndServe(addr, mux))
}
