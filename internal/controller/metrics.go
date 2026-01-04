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

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Reconciliation metrics
	reconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_reconcile_total",
		Help: "Total number of reconciliations",
	}, []string{"controller", "result"})

	reconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pequod_reconcile_duration_seconds",
		Help:    "Duration of reconciliations",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 12), // 10ms to ~40s
	}, []string{"controller"})

	reconcileErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_reconcile_errors_total",
		Help: "Total number of reconciliation errors",
	}, []string{"controller", "error_type"})

	// DAG execution metrics
	dagNodesTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pequod_dag_nodes_total",
		Help: "Total number of nodes in DAG being executed",
	}, []string{"resourcegraph"})

	dagExecutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pequod_dag_execution_duration_seconds",
		Help:    "Duration of DAG executions",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~100s
	}, []string{"resourcegraph", "result"})

	dagNodeExecutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pequod_dag_node_execution_duration_seconds",
		Help:    "Duration of individual node executions",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
	}, []string{"node_id", "result"})

	// Adoption metrics
	adoptionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pequod_adoption_total",
		Help: "Total number of resource adoptions",
	}, []string{"result"})

	adoptionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pequod_adoption_duration_seconds",
		Help:    "Duration of resource adoption operations",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
	})
)

func init() {
	// Register all controller metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileDuration,
		reconcileErrorsTotal,
		dagNodesTotal,
		dagExecutionDuration,
		dagNodeExecutionDuration,
		adoptionTotal,
		adoptionDuration,
	)
}

// RecordReconcile records a reconciliation
func RecordReconcile(controller, result string, durationSeconds float64) {
	reconcileTotal.WithLabelValues(controller, result).Inc()
	reconcileDuration.WithLabelValues(controller).Observe(durationSeconds)
}

// RecordReconcileError records a reconciliation error
func RecordReconcileError(controller, errorType string) {
	reconcileErrorsTotal.WithLabelValues(controller, errorType).Inc()
}

// SetDAGNodes sets the current number of nodes in a DAG
func SetDAGNodes(resourceGraph string, count int) {
	dagNodesTotal.WithLabelValues(resourceGraph).Set(float64(count))
}

// RecordDAGExecution records a DAG execution
func RecordDAGExecution(resourceGraph, result string, durationSeconds float64) {
	dagExecutionDuration.WithLabelValues(resourceGraph, result).Observe(durationSeconds)
}

// RecordNodeExecution records a node execution
func RecordNodeExecution(nodeID, result string, durationSeconds float64) {
	dagNodeExecutionDuration.WithLabelValues(nodeID, result).Observe(durationSeconds)
}

// RecordAdoption records an adoption operation
func RecordAdoption(result string, durationSeconds float64) {
	adoptionTotal.WithLabelValues(result).Inc()
	adoptionDuration.Observe(durationSeconds)
}
