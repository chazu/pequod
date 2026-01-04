package platformloader

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Cache metrics
	cacheHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pequod_cue_cache_hits_total",
		Help: "Total number of CUE module cache hits",
	})

	cacheMissesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pequod_cue_cache_misses_total",
		Help: "Total number of CUE module cache misses",
	})

	cacheEvictionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pequod_cue_cache_evictions_total",
		Help: "Total number of CUE module cache evictions",
	})

	cacheEntriesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pequod_cue_cache_entries",
		Help: "Current number of entries in the CUE module cache",
	})

	cacheSizeBytesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pequod_cue_cache_size_bytes",
		Help: "Current size of the CUE module cache in bytes",
	})

	// Fetch metrics
	fetchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pequod_cue_fetch_duration_seconds",
		Help:    "Duration of CUE module fetch operations",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
	}, []string{"type", "status"})

	fetchTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_cue_fetch_total",
		Help: "Total number of CUE module fetch operations",
	}, []string{"type", "status"})

	// Render metrics
	renderDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pequod_cue_render_duration_seconds",
		Help:    "Duration of CUE rendering operations",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
	})

	renderErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pequod_cue_render_errors_total",
		Help: "Total number of CUE rendering errors",
	})

	// Policy validation metrics
	policyViolationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_policy_violations_total",
		Help: "Total number of policy violations detected",
	}, []string{"severity"})
)

func init() {
	// Register all metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		cacheHitsTotal,
		cacheMissesTotal,
		cacheEvictionsTotal,
		cacheEntriesGauge,
		cacheSizeBytesGauge,
		fetchDuration,
		fetchTotal,
		renderDuration,
		renderErrorsTotal,
		policyViolationsTotal,
	)
}

// RecordCacheHit records a cache hit
func RecordCacheHit() {
	cacheHitsTotal.Inc()
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss() {
	cacheMissesTotal.Inc()
}

// RecordCacheEviction records a cache eviction
func RecordCacheEviction() {
	cacheEvictionsTotal.Inc()
}

// UpdateCacheStats updates the cache gauge metrics
func UpdateCacheStats(entries int, sizeBytes int64) {
	cacheEntriesGauge.Set(float64(entries))
	cacheSizeBytesGauge.Set(float64(sizeBytes))
}

// RecordFetch records a fetch operation
func RecordFetch(fetcherType string, status string, durationSeconds float64) {
	fetchDuration.WithLabelValues(fetcherType, status).Observe(durationSeconds)
	fetchTotal.WithLabelValues(fetcherType, status).Inc()
}

// RecordRender records a render operation
func RecordRender(durationSeconds float64) {
	renderDuration.Observe(durationSeconds)
}

// RecordRenderError records a render error
func RecordRenderError() {
	renderErrorsTotal.Inc()
}

// RecordPolicyViolation records a policy violation
func RecordPolicyViolation(severity string) {
	policyViolationsTotal.WithLabelValues(severity).Inc()
}
