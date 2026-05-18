// Package metrics registers and exposes Prometheus metrics for the proxy and engine.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metric definitions for the ffuuzz proxy and engine.
var (
	reg = prometheus.NewRegistry()

	TestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_tests_total",
		Help: "Total number of fuzz tests executed.",
	})

	FindingsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ffuuzz_findings_total",
		Help: "Total number of findings by type.",
	}, []string{"type"})

	RequestDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ffuuzz_request_duration_seconds",
		Help:    "Histogram of upstream request durations in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	CorpusSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ffuuzz_corpus_size",
		Help: "Current number of recording sessions in the corpus.",
	})

	CertCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_cert_cache_hits_total",
		Help: "Certificate LRU cache hits.",
	})

	CertCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_cert_cache_misses_total",
		Help: "Certificate LRU cache misses.",
	})

	CertCacheEvictions = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_cert_cache_evictions_total",
		Help: "Certificate LRU cache evictions.",
	})

	ConnectErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ffuuzz_connect_errors_total",
		Help: "CONNECT/hijack errors by class.",
	}, []string{"error_class"})

	CertErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_cert_errors_total",
		Help: "Certificate generation or storage errors.",
	})

	EndpointCollapses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_endpoint_collapses_total",
		Help: "Total endpoint pattern collapses detected.",
	})

	EndpointMerges = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ffuuzz_endpoint_merges_total",
		Help: "Total recording merges from endpoint collapses.",
	})
)

func init() {
	collectors := []prometheus.Collector{
		TestsTotal,
		FindingsTotal,
		RequestDuration,
		CorpusSize,
		CertCacheHits,
		CertCacheMisses,
		CertCacheEvictions,
		ConnectErrors,
		CertErrors,
		EndpointCollapses,
		EndpointMerges,
	}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			panic("failed to register Prometheus metric: " + err.Error())
		}
	}
}

// Registry returns the custom Prometheus registry with all ffuuzz metrics.
func Registry() *prometheus.Registry {
	return reg
}
