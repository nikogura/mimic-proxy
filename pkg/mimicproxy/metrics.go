package mimicproxy

import "github.com/prometheus/client_golang/prometheus"

// Label constants for metrics.
const (
	// LabelRoute identifies the route name.
	LabelRoute = "route"
	// LabelMethod identifies the HTTP method.
	LabelMethod = "method"
	// LabelStatusCode identifies the HTTP response status code.
	LabelStatusCode = "status_code"
	// LabelRedirectType identifies the type of redirect (relative, internal, external_known, external_unknown).
	LabelRedirectType = "redirect_type"
)

var (
	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// RequestLabels are common labels for request metrics.
	RequestLabels = []string{LabelRoute, LabelMethod}

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// RequestStatusLabels include status code in addition to common labels.
	RequestStatusLabels = []string{LabelRoute, LabelMethod, LabelStatusCode}

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// RedirectLabels are labels for redirect rewriting metrics.
	RedirectLabels = []string{LabelRoute, LabelRedirectType}

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyRequestsTotal tracks the total number of requests handled by the proxy.
	ProxyRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_requests_total",
			Help: "Total number of requests handled by the mimic proxy",
		},
		RequestLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyRequestDuration tracks the duration of proxy requests in seconds.
	ProxyRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mimic_proxy_request_duration_seconds",
			Help:    "Duration of proxy requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		RequestLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyRequestErrorsTotal tracks the total number of proxy request errors.
	ProxyRequestErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_request_errors_total",
			Help: "Total number of errors when handling proxy requests",
		},
		RequestLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyResponsesTotal tracks responses by status code.
	ProxyResponsesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_responses_total",
			Help: "Total number of responses by status code",
		},
		RequestStatusLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyRedirectRewritesTotal tracks the number of redirect rewrites performed.
	ProxyRedirectRewritesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_redirect_rewrites_total",
			Help: "Total number of redirect rewrites performed by type",
		},
		RedirectLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyHeaderStripsTotal tracks the number of headers stripped.
	ProxyHeaderStripsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_header_strips_total",
			Help: "Total number of headers stripped for transparency",
		},
		[]string{LabelRoute},
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyHeaderAddsTotal tracks the number of headers added.
	ProxyHeaderAddsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_header_adds_total",
			Help: "Total number of headers added to upstream requests",
		},
		[]string{LabelRoute},
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyUpstreamDuration tracks the duration of upstream requests in seconds.
	ProxyUpstreamDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mimic_proxy_upstream_duration_seconds",
			Help:    "Duration of upstream requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		RequestLabels,
	)

	//nolint:gochecknoglobals // This is how the prometheus magic works.
	// ProxyUpstreamErrorsTotal tracks upstream request errors.
	ProxyUpstreamErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mimic_proxy_upstream_errors_total",
			Help: "Total number of upstream request errors",
		},
		RequestLabels,
	)
)

//nolint:gochecknoinits // This is how the prometheus magic works.
func init() {
	_ = prometheus.Register(ProxyRequestsTotal)
	_ = prometheus.Register(ProxyRequestDuration)
	_ = prometheus.Register(ProxyRequestErrorsTotal)
	_ = prometheus.Register(ProxyResponsesTotal)
	_ = prometheus.Register(ProxyRedirectRewritesTotal)
	_ = prometheus.Register(ProxyHeaderStripsTotal)
	_ = prometheus.Register(ProxyHeaderAddsTotal)
	_ = prometheus.Register(ProxyUpstreamDuration)
	_ = prometheus.Register(ProxyUpstreamErrorsTotal)
}
