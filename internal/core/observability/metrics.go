package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the application's Prometheus registry and core metrics.
type Metrics struct {
	registry            *prometheus.Registry
	HTTPRequests        *prometheus.CounterVec
	HTTPDurationSeconds *prometheus.HistogramVec
}

// NewMetrics builds a fresh registry and registers default collectors plus HTTP RED metrics.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests handled, labelled by method, route, and status.",
	}, []string{"method", "route", "status"})

	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "marketplace",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds, labelled by method, route, and status.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
	)

	return &Metrics{
		registry:            reg,
		HTTPRequests:        httpRequests,
		HTTPDurationSeconds: httpDuration,
	}
}

// Handler returns the HTTP handler that serves Prometheus metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry:          m.registry,
		EnableOpenMetrics: true,
	})
}

// Registry exposes the underlying registry for additional registrations.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
