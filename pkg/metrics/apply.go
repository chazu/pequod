/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Apply operation metrics
	applyTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_apply_total",
		Help: "Total number of resource apply operations",
	}, []string{"result", "mode", "gvk"})

	applyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pequod_apply_duration_seconds",
		Help:    "Duration of resource apply operations",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
	}, []string{"mode", "gvk"})

	resourcesManaged = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pequod_resources_managed",
		Help: "Number of resources currently managed by pequod",
	}, []string{"gvk", "namespace"})
)

func init() {
	// Register apply metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		applyTotal,
		applyDuration,
		resourcesManaged,
	)
}

// RecordApply records an apply operation
// result: "success" or "failure"
// mode: "apply", "create", or "adopt"
// gvk: GroupVersionKind as string (e.g., "apps/v1/Deployment")
func RecordApply(result, mode, gvk string, durationSeconds float64) {
	applyTotal.WithLabelValues(result, mode, gvk).Inc()
	applyDuration.WithLabelValues(mode, gvk).Observe(durationSeconds)
}

// SetManagedResources sets the gauge for managed resources
func SetManagedResources(gvk, namespace string, count int) {
	resourcesManaged.WithLabelValues(gvk, namespace).Set(float64(count))
}

// IncrementManagedResources increments the managed resources gauge
func IncrementManagedResources(gvk, namespace string) {
	resourcesManaged.WithLabelValues(gvk, namespace).Inc()
}

// DecrementManagedResources decrements the managed resources gauge
func DecrementManagedResources(gvk, namespace string) {
	resourcesManaged.WithLabelValues(gvk, namespace).Dec()
}
