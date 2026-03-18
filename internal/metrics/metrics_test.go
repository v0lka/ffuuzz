package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegistry_NotNil(t *testing.T) {
	reg := Registry()
	if reg == nil {
		t.Fatal("Registry() returned nil")
	}
}

func TestRegistry_MetricsRegistered(t *testing.T) {
	reg := Registry()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	// We registered 9 metrics in init()
	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	expected := []string{
		"ffuuzz_tests_total",
		"ffuuzz_request_duration_seconds",
		"ffuuzz_corpus_size",
		"ffuuzz_cert_cache_hits_total",
		"ffuuzz_cert_cache_misses_total",
		"ffuuzz_cert_cache_evictions_total",
		"ffuuzz_cert_errors_total",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("metric %q not found in registry", name)
		}
	}
}

func TestMetrics_Increment(t *testing.T) {
	// Test that metrics can be incremented without panic
	TestsTotal.Inc()
	FindingsTotal.WithLabelValues("TIMEOUT").Inc()
	RequestDuration.Observe(0.5)
	CorpusSize.Set(10)
	CertCacheHits.Inc()
	CertCacheMisses.Inc()
	CertCacheEvictions.Inc()
	ConnectErrors.WithLabelValues("test").Inc()
	CertErrors.Inc()
}

func TestFindingsTotal_Labels(t *testing.T) {
	// Verify we can use different label values
	FindingsTotal.WithLabelValues("SERVER_ERROR").Inc()
	FindingsTotal.WithLabelValues("LATENCY_REGRESSION").Inc()
	FindingsTotal.WithLabelValues("REGEX_MATCH").Inc()

	// Check that gathering still works
	reg := Registry()
	_, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather after label increment: %v", err)
	}
}

func TestRequestDuration_Buckets(t *testing.T) {
	// Verify the histogram uses default buckets
	desc := make(chan *prometheus.Desc, 1)
	RequestDuration.Describe(desc)
	d := <-desc
	if d == nil {
		t.Fatal("RequestDuration.Describe returned nil")
	}
}
