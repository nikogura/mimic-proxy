package mimicproxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewTransport creates a customized http.Transport with connection pooling
// and timeouts configured for optimal proxy performance.
func NewTransport(config *TransportConfig, tlsConfig *tls.Config) (transport *http.Transport, err error) {
	transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		DisableKeepAlives:     config.DisableKeepAlives,
		DisableCompression:    config.DisableCompression,
		TLSClientConfig:       tlsConfig,
	}

	return transport, err
}
