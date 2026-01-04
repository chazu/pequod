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
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/apply"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/readiness"
)

const (
	resourceGraphFinalizer = "platform.pequod.io/resourcegraph-finalizer"

	// Phase constants for ResourceGraph status
	PhasePending   = "Pending"
	PhaseExecuting = "Executing"
	PhaseCompleted = "Completed"
	PhaseFailed    = "Failed"

	// Condition type constants
	ConditionTypeReady  = "Ready"
	ConditionTypeFailed = "Failed"
)

// ResourceGraphReconciler reconciles a ResourceGraph object
type ResourceGraphReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Applier  *apply.Applier
	Adopter  *apply.Adopter
	Checker  *readiness.Checker
	Executor *graph.Executor
	Recorder record.EventRecorder

	// RequeueInterval is the interval to requeue when waiting for readiness
	// Default: 5 seconds
	RequeueInterval time.Duration
}

// DefaultRequeueInterval is the default interval for requeuing
const DefaultRequeueInterval = 5 * time.Second

// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=*,verbs=get;list;watch;create;update;patch;delete

// Reconcile executes the ResourceGraph by applying all nodes in dependency order
func (r *ResourceGraphReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling ResourceGraph", "name", req.Name, "namespace", req.Namespace)

	// Defer metrics recording
	var result string
	defer func() {
		duration := time.Since(startTime).Seconds()
		RecordReconcile("resourcegraph", result, duration)
	}()

	// Fetch the ResourceGraph
	rg := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, req.NamespacedName, rg); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ResourceGraph not found, ignoring")
			result = "not_found"
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ResourceGraph")
		result = "error"
		RecordReconcileError("resourcegraph", "get_failed")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !rg.DeletionTimestamp.IsZero() {
		result = "deleted"
		return r.handleDeletion(ctx, rg)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rg, resourceGraphFinalizer) {
		controllerutil.AddFinalizer(rg, resourceGraphFinalizer)
		if err := r.Update(ctx, rg); err != nil {
			logger.Error(err, "Failed to add finalizer")
			result = "error"
			RecordReconcileError("resourcegraph", "finalizer_failed")
			return ctrl.Result{}, err
		}
		result = "requeue"
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if already completed
	if rg.Status.Phase == PhaseCompleted || rg.Status.Phase == PhaseFailed {
		// Allow re-execution if the spec has changed (generation mismatch)
		if rg.Status.ObservedGeneration == rg.Generation {
			logger.Info("ResourceGraph already in terminal state", "phase", rg.Status.Phase)
			result = "terminal"
			return ctrl.Result{}, nil
		}
		// Spec changed since last execution - allow re-execution
		logger.Info("ResourceGraph spec changed, allowing re-execution",
			"phase", rg.Status.Phase,
			"observedGeneration", rg.Status.ObservedGeneration,
			"currentGeneration", rg.Generation)
		r.recordEvent(rg, "Normal", "ReExecuting", "Spec changed, re-executing graph")
	}

	// Execute the graph
	ctrlResult, err := r.executeGraph(ctx, rg)
	if err != nil {
		result = "error"
	} else if ctrlResult.RequeueAfter > 0 {
		result = "requeue"
	} else {
		result = "success"
	}
	return ctrlResult, err
}

// executeGraph executes the ResourceGraph by building a DAG and applying resources
func (r *ResourceGraphReconciler) executeGraph(ctx context.Context, rg *platformv1alpha1.ResourceGraph) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Convert ResourceGraph to internal Graph type
	internalGraph, err := r.convertToInternalGraph(rg)
	if err != nil {
		logger.Error(err, "Failed to convert ResourceGraph to internal Graph")
		r.recordEvent(rg, "Warning", "ConversionFailed", fmt.Sprintf("Failed to convert graph: %v", err))
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Failed to convert graph: %v", err))
	}

	// Validate the graph before DAG building for better error reporting.
	// Note: BuildDAG also validates, but we do it here to provide user-friendly
	// events and status updates before attempting DAG construction.
	if err := internalGraph.Validate(); err != nil {
		logger.Error(err, "Graph validation failed")
		r.recordEvent(rg, "Warning", "ValidationFailed", fmt.Sprintf("Graph validation failed: %v", err))
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Graph validation failed: %v", err))
	}

	// Build DAG
	dag, err := graph.BuildDAG(internalGraph)
	if err != nil {
		logger.Error(err, "Failed to build DAG")
		r.recordEvent(rg, "Warning", "DAGBuildFailed", fmt.Sprintf("Failed to build DAG: %v", err))
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Failed to build DAG: %v", err))
	}

	// Update status to Executing
	if err := r.updateStatusExecuting(ctx, rg); err != nil {
		logger.Error(err, "Failed to update status to Executing")
		// Requeue to retry status update
		return ctrl.Result{Requeue: true}, err
	}

	// Run adoption phase before DAG execution
	if rg.Spec.Adopt != nil && len(rg.Spec.Adopt.Resources) > 0 {
		adoptionReport, err := r.runAdoption(ctx, rg, internalGraph.Nodes)
		if err != nil {
			logger.Error(err, "Adoption phase failed")
			r.recordEvent(rg, "Warning", "AdoptionFailed", fmt.Sprintf("Adoption failed: %v", err))
			// Continue with execution - adoption failures are not blocking
		} else if adoptionReport != nil {
			r.recordAdoptionEvents(rg, adoptionReport)
			if err := r.updateStatusWithAdoption(ctx, rg, adoptionReport); err != nil {
				logger.Error(err, "Failed to update status with adoption results")
			}
		}
	}

	// Record execution start event
	r.recordEvent(rg, "Normal", "ExecutionStarted", fmt.Sprintf("Starting execution of %d nodes", len(internalGraph.Nodes)))

	// Record DAG node count metric
	SetDAGNodes(rg.Name, len(internalGraph.Nodes))

	// Execute the DAG with timing
	dagStartTime := time.Now()
	logger.Info("Executing DAG", "nodeCount", len(internalGraph.Nodes))
	executionState, err := r.Executor.Execute(ctx, dag)
	dagDuration := time.Since(dagStartTime).Seconds()

	if err != nil {
		logger.Error(err, "DAG execution failed")
		r.recordEvent(rg, "Warning", "ExecutionFailed", fmt.Sprintf("DAG execution failed: %v", err))
		RecordDAGExecution(rg.Name, "failed", dagDuration)
		return r.updateStatusFromExecution(ctx, rg, executionState, false)
	}

	// Record success event and metrics
	r.recordEvent(rg, "Normal", "ExecutionCompleted", fmt.Sprintf("Successfully applied %d resources", len(internalGraph.Nodes)))
	RecordDAGExecution(rg.Name, "success", dagDuration)

	// Update status from execution state
	return r.updateStatusFromExecution(ctx, rg, executionState, true)
}

// runAdoption executes the adoption phase
func (r *ResourceGraphReconciler) runAdoption(
	ctx context.Context,
	rg *platformv1alpha1.ResourceGraph,
	nodes []graph.Node,
) (*apply.AdoptionReport, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Running adoption phase", "resourceCount", len(rg.Spec.Adopt.Resources))

	adoptionStart := time.Now()
	report, err := r.Adopter.Adopt(ctx, rg.Spec.Adopt, nodes)
	adoptionDuration := time.Since(adoptionStart).Seconds()

	if err != nil {
		RecordAdoption("error", adoptionDuration)
		return nil, fmt.Errorf("adoption failed: %w", err)
	}

	// Record adoption metrics
	if report.TotalFailed > 0 {
		RecordAdoption("partial", adoptionDuration)
	} else {
		RecordAdoption("success", adoptionDuration)
	}

	logger.Info("Adoption phase completed",
		"adopted", report.TotalAdopted,
		"failed", report.TotalFailed,
		"skipped", report.TotalSkipped,
		"created", report.TotalCreated)

	return report, nil
}

// recordAdoptionEvents records events for adoption results
func (r *ResourceGraphReconciler) recordAdoptionEvents(rg *platformv1alpha1.ResourceGraph, report *apply.AdoptionReport) {
	for _, result := range report.Results {
		if result.Error != nil {
			r.recordEvent(rg, "Warning", "AdoptionFailed",
				fmt.Sprintf("Failed to adopt %s: %v", result.Resource.String(), result.Error))
		} else if result.Adopted {
			eventType := "Normal"
			reason := "ResourceAdopted"
			message := fmt.Sprintf("Adopted %s", result.Resource.String())
			if result.Created {
				reason = "ResourceCreated"
				message = fmt.Sprintf("Created %s (was missing)", result.Resource.String())
			}
			r.recordEvent(rg, eventType, reason, message)
		}
	}
}

// updateStatusWithAdoption updates the ResourceGraph status with adoption results
func (r *ResourceGraphReconciler) updateStatusWithAdoption(
	ctx context.Context,
	rg *platformv1alpha1.ResourceGraph,
	report *apply.AdoptionReport,
) error {
	// Re-fetch to get latest resourceVersion
	latest := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(rg), latest); err != nil {
		return fmt.Errorf("failed to get latest ResourceGraph: %w", err)
	}

	now := metav1.Now()

	// Initialize node states if needed
	if latest.Status.NodeStates == nil {
		latest.Status.NodeStates = make(map[string]platformv1alpha1.NodeExecutionState)
	}

	// Update node states with adoption info
	for _, result := range report.Results {
		if result.NodeID == "" {
			continue
		}

		state := latest.Status.NodeStates[result.NodeID]
		if result.Adopted {
			state.Adopted = true
			state.AdoptedAt = &now
			state.PreviousManagers = result.ConflictingManagers
			if result.Created {
				state.Message = "Created (resource was missing)"
			} else {
				state.Message = "Adopted from existing resource"
			}
		}
		if result.Error != nil {
			state.LastError = result.Error.Error()
		}
		state.LastTransitionTime = &now
		latest.Status.NodeStates[result.NodeID] = state
	}

	return r.Status().Update(ctx, latest)
}

// recordEvent records an event if the recorder is available
func (r *ResourceGraphReconciler) recordEvent(rg *platformv1alpha1.ResourceGraph, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(rg, eventType, reason, message)
	}
}

// handleDeletion handles ResourceGraph deletion
func (r *ResourceGraphReconciler) handleDeletion(ctx context.Context, rg *platformv1alpha1.ResourceGraph) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling ResourceGraph deletion")

	// ResourceGraph doesn't own the resources it applies
	// Those are owned by the source (WebService, etc.)
	// So we just remove the finalizer

	if controllerutil.ContainsFinalizer(rg, resourceGraphFinalizer) {
		controllerutil.RemoveFinalizer(rg, resourceGraphFinalizer)
		if err := r.Update(ctx, rg); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceGraphReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize components if not already set
	if r.Applier == nil {
		r.Applier = apply.NewApplier(r.Client)
	}
	if r.Adopter == nil {
		r.Adopter = apply.NewAdopter(r.Client)
	}
	if r.Checker == nil {
		r.Checker = readiness.NewChecker(r.Client)
	}
	if r.Executor == nil {
		r.Executor = graph.NewExecutor(r.Applier, r.Checker, r.Client, graph.DefaultExecutorConfig())
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("resourcegraph-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.ResourceGraph{}).
		// Watch common resource types that ResourceGraph may create
		// Changes to these resources will trigger reconciliation of the owning ResourceGraph
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Named("resourcegraph").
		Complete(r)
}

// convertToInternalGraph converts a ResourceGraph CR to the internal Graph type
func (r *ResourceGraphReconciler) convertToInternalGraph(rg *platformv1alpha1.ResourceGraph) (*graph.Graph, error) {
	nodes := make([]graph.Node, 0, len(rg.Spec.Nodes))

	// Create owner reference for applied resources
	// Resources will be owned by the ResourceGraph for proper cleanup
	ownerRef := metav1.OwnerReference{
		APIVersion:         rg.APIVersion,
		Kind:               rg.Kind,
		Name:               rg.Name,
		UID:                rg.UID,
		Controller:         ptr(true),
		BlockOwnerDeletion: ptr(true),
	}

	for _, rgNode := range rg.Spec.Nodes {
		// Decode RawExtension to Unstructured
		unstructuredObj := &unstructured.Unstructured{}
		if err := unstructuredObj.UnmarshalJSON(rgNode.Object.Raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal node %s object: %w", rgNode.ID, err)
		}

		// Inject owner reference into the object
		// This ensures resources are cleaned up when ResourceGraph is deleted
		existingRefs := unstructuredObj.GetOwnerReferences()
		// Check if owner reference already exists to avoid duplicates
		hasOwnerRef := false
		for _, ref := range existingRefs {
			if ref.UID == rg.UID {
				hasOwnerRef = true
				break
			}
		}
		if !hasOwnerRef {
			existingRefs = append(existingRefs, ownerRef)
			unstructuredObj.SetOwnerReferences(existingRefs)
		}

		// Convert ApplyPolicy
		applyPolicy := graph.ApplyPolicy{
			Mode:           graph.ApplyMode(rgNode.ApplyPolicy.Mode),
			ConflictPolicy: graph.ConflictPolicy(rgNode.ApplyPolicy.ConflictPolicy),
			FieldManager:   rgNode.ApplyPolicy.FieldManager,
		}

		// Convert ReadyWhen predicates
		readyWhen := make([]graph.ReadinessPredicate, 0, len(rgNode.ReadyWhen))
		for _, rw := range rgNode.ReadyWhen {
			pred := graph.ReadinessPredicate{
				Type:            graph.PredicateType(rw.Type),
				ConditionType:   rw.ConditionType,
				ConditionStatus: rw.ConditionStatus,
			}
			readyWhen = append(readyWhen, pred)
		}

		// Create internal node with unstructured object
		node := graph.Node{
			ID:          rgNode.ID,
			Object:      *unstructuredObj,
			ApplyPolicy: applyPolicy,
			DependsOn:   rgNode.DependsOn,
			ReadyWhen:   readyWhen,
		}

		nodes = append(nodes, node)
	}

	// Convert violations
	violations := make([]graph.Violation, 0, len(rg.Spec.Violations))
	for _, v := range rg.Spec.Violations {
		violations = append(violations, graph.Violation{
			Path:     v.Path,
			Message:  v.Message,
			Severity: graph.ViolationSeverity(v.Severity),
		})
	}

	return &graph.Graph{
		Metadata: graph.GraphMetadata{
			Name:        rg.Spec.Metadata.Name,
			Version:     rg.Spec.Metadata.Version,
			PlatformRef: rg.Spec.Metadata.PlatformRef,
			RenderHash:  rg.Spec.RenderHash,
		},
		Nodes:      nodes,
		Violations: violations,
	}, nil
}

// updateStatusExecuting updates the ResourceGraph status to Executing
func (r *ResourceGraphReconciler) updateStatusExecuting(ctx context.Context, rg *platformv1alpha1.ResourceGraph) error {
	// Re-fetch the object to get the latest resourceVersion to avoid conflicts
	latest := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(rg), latest); err != nil {
		return fmt.Errorf("failed to get latest ResourceGraph: %w", err)
	}

	now := metav1.Now()
	latest.Status.Phase = PhaseExecuting
	latest.Status.StartedAt = &now
	latest.Status.ObservedGeneration = latest.Generation

	// Initialize node states
	if latest.Status.NodeStates == nil {
		latest.Status.NodeStates = make(map[string]platformv1alpha1.NodeExecutionState)
	}
	for _, node := range latest.Spec.Nodes {
		if _, exists := latest.Status.NodeStates[node.ID]; !exists {
			latest.Status.NodeStates[node.ID] = platformv1alpha1.NodeExecutionState{
				Phase:              PhasePending,
				LastTransitionTime: &now,
			}
		}
	}

	return r.Status().Update(ctx, latest)
}

// updateStatusFailed updates the ResourceGraph status to Failed
func (r *ResourceGraphReconciler) updateStatusFailed(ctx context.Context, rg *platformv1alpha1.ResourceGraph, message string) (ctrl.Result, error) {
	// Re-fetch the object to get the latest resourceVersion to avoid conflicts
	latest := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(rg), latest); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("failed to get latest ResourceGraph: %w", err)
	}

	now := metav1.Now()
	latest.Status.Phase = PhaseFailed
	latest.Status.CompletedAt = &now
	latest.Status.ObservedGeneration = latest.Generation

	// Add condition
	latest.Status.Conditions = []metav1.Condition{
		{
			Type:               ConditionTypeFailed,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ExecutionFailed",
			Message:            message,
		},
	}

	if err := r.Status().Update(ctx, latest); err != nil {
		// Requeue to retry status update
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

// updateStatusFromExecution updates the ResourceGraph status from execution state
func (r *ResourceGraphReconciler) updateStatusFromExecution(ctx context.Context, rg *platformv1alpha1.ResourceGraph, state *graph.ExecutionState, success bool) (ctrl.Result, error) {
	// Re-fetch the object to get the latest resourceVersion to avoid conflicts
	latest := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(rg), latest); err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("failed to get latest ResourceGraph: %w", err)
	}

	now := metav1.Now()

	// Update node states
	if latest.Status.NodeStates == nil {
		latest.Status.NodeStates = make(map[string]platformv1alpha1.NodeExecutionState)
	}

	// Get all node states
	allStates := state.GetAllStates()
	for nodeID, nodeState := range allStates {
		// Get full status for timestamps and error info
		nodeStatus, err := state.GetStatus(nodeID)
		if err != nil {
			continue // Skip if we can't get status
		}

		execState := platformv1alpha1.NodeExecutionState{
			Phase:              string(nodeState),
			LastTransitionTime: &now,
		}

		if nodeStatus.Error != "" {
			execState.LastError = nodeStatus.Error
		}

		// Set timestamps based on state
		if nodeStatus.StartTime != nil {
			execState.AppliedAt = &metav1.Time{Time: *nodeStatus.StartTime}
		}
		if nodeStatus.ReadyTime != nil {
			execState.ReadyAt = &metav1.Time{Time: *nodeStatus.ReadyTime}
		}

		latest.Status.NodeStates[nodeID] = execState
	}

	// Update overall phase
	if success && state.IsComplete() {
		latest.Status.Phase = PhaseCompleted
		latest.Status.CompletedAt = &now
		latest.Status.Conditions = []metav1.Condition{
			{
				Type:               ConditionTypeReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "ExecutionCompleted",
				Message:            "All resources applied successfully",
			},
		}
	} else {
		latest.Status.Phase = PhaseFailed
		latest.Status.CompletedAt = &now
		latest.Status.Conditions = []metav1.Condition{
			{
				Type:               ConditionTypeFailed,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "ExecutionFailed",
				Message:            "One or more resources failed to apply",
			},
		}
	}

	latest.Status.ObservedGeneration = latest.Generation

	if err := r.Status().Update(ctx, latest); err != nil {
		// Requeue to retry status update
		return ctrl.Result{Requeue: true}, err
	}

	// Requeue if not complete to check readiness
	if !state.IsComplete() {
		return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
	}

	return ctrl.Result{}, nil
}

// getRequeueInterval returns the configured requeue interval or the default
func (r *ResourceGraphReconciler) getRequeueInterval() time.Duration {
	if r.RequeueInterval > 0 {
		return r.RequeueInterval
	}
	return DefaultRequeueInterval
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}
