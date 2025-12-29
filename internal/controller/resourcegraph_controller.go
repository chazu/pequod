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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
)

// ResourceGraphReconciler reconciles a ResourceGraph object
type ResourceGraphReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Applier  *apply.Applier
	Checker  *readiness.Checker
	Executor *graph.Executor
}

// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=*,verbs=get;list;watch;create;update;patch;delete

// Reconcile executes the ResourceGraph by applying all nodes in dependency order
func (r *ResourceGraphReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling ResourceGraph", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ResourceGraph
	rg := &platformv1alpha1.ResourceGraph{}
	if err := r.Get(ctx, req.NamespacedName, rg); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ResourceGraph not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ResourceGraph")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !rg.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rg)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rg, resourceGraphFinalizer) {
		controllerutil.AddFinalizer(rg, resourceGraphFinalizer)
		if err := r.Update(ctx, rg); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if already completed
	if rg.Status.Phase == "Completed" || rg.Status.Phase == "Failed" {
		logger.Info("ResourceGraph already in terminal state", "phase", rg.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Execute the graph
	return r.executeGraph(ctx, rg)
}

// executeGraph executes the ResourceGraph by building a DAG and applying resources
func (r *ResourceGraphReconciler) executeGraph(ctx context.Context, rg *platformv1alpha1.ResourceGraph) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Convert ResourceGraph to internal Graph type
	internalGraph, err := r.convertToInternalGraph(rg)
	if err != nil {
		logger.Error(err, "Failed to convert ResourceGraph to internal Graph")
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Failed to convert graph: %v", err))
	}

	// Validate the graph
	if err := internalGraph.Validate(); err != nil {
		logger.Error(err, "Graph validation failed")
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Graph validation failed: %v", err))
	}

	// Build DAG
	dag, err := graph.BuildDAG(internalGraph)
	if err != nil {
		logger.Error(err, "Failed to build DAG")
		return r.updateStatusFailed(ctx, rg, fmt.Sprintf("Failed to build DAG: %v", err))
	}

	// Update status to Executing
	if err := r.updateStatusExecuting(ctx, rg); err != nil {
		logger.Error(err, "Failed to update status to Executing")
		return ctrl.Result{}, err
	}

	// Execute the DAG
	logger.Info("Executing DAG", "nodeCount", len(internalGraph.Nodes))
	executionState, err := r.Executor.Execute(ctx, dag)
	if err != nil {
		logger.Error(err, "DAG execution failed")
		return r.updateStatusFromExecution(ctx, rg, executionState, false)
	}

	// Update status from execution state
	return r.updateStatusFromExecution(ctx, rg, executionState, true)
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
	if r.Checker == nil {
		r.Checker = readiness.NewChecker(r.Client)
	}
	if r.Executor == nil {
		r.Executor = graph.NewExecutor(r.Applier, r.Checker, r.Client, graph.DefaultExecutorConfig())
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.ResourceGraph{}).
		Named("resourcegraph").
		Complete(r)
}


// convertToInternalGraph converts a ResourceGraph CR to the internal Graph type
func (r *ResourceGraphReconciler) convertToInternalGraph(rg *platformv1alpha1.ResourceGraph) (*graph.Graph, error) {
	nodes := make([]graph.Node, 0, len(rg.Spec.Nodes))

	for _, rgNode := range rg.Spec.Nodes {
		// Decode RawExtension to Unstructured
		unstructuredObj := &unstructured.Unstructured{}
		if err := unstructuredObj.UnmarshalJSON(rgNode.Object.Raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal node %s object: %w", rgNode.ID, err)
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
	now := metav1.Now()
	rg.Status.Phase = "Executing"
	rg.Status.StartedAt = &now
	rg.Status.ObservedGeneration = rg.Generation

	// Initialize node states
	if rg.Status.NodeStates == nil {
		rg.Status.NodeStates = make(map[string]platformv1alpha1.NodeExecutionState)
	}
	for _, node := range rg.Spec.Nodes {
		if _, exists := rg.Status.NodeStates[node.ID]; !exists {
			rg.Status.NodeStates[node.ID] = platformv1alpha1.NodeExecutionState{
				Phase:              "Pending",
				LastTransitionTime: &now,
			}
		}
	}

	return r.Status().Update(ctx, rg)
}

// updateStatusFailed updates the ResourceGraph status to Failed
func (r *ResourceGraphReconciler) updateStatusFailed(ctx context.Context, rg *platformv1alpha1.ResourceGraph, message string) (ctrl.Result, error) {
	now := metav1.Now()
	rg.Status.Phase = "Failed"
	rg.Status.CompletedAt = &now
	rg.Status.ObservedGeneration = rg.Generation

	// Add condition
	rg.Status.Conditions = []metav1.Condition{
		{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ExecutionFailed",
			Message:            message,
		},
	}

	if err := r.Status().Update(ctx, rg); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatusFromExecution updates the ResourceGraph status from execution state
func (r *ResourceGraphReconciler) updateStatusFromExecution(ctx context.Context, rg *platformv1alpha1.ResourceGraph, state *graph.ExecutionState, success bool) (ctrl.Result, error) {
	now := metav1.Now()

	// Update node states
	if rg.Status.NodeStates == nil {
		rg.Status.NodeStates = make(map[string]platformv1alpha1.NodeExecutionState)
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

		rg.Status.NodeStates[nodeID] = execState
	}

	// Update overall phase
	if success && state.IsComplete() {
		rg.Status.Phase = "Completed"
		rg.Status.CompletedAt = &now
		rg.Status.Conditions = []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "ExecutionCompleted",
				Message:            "All resources applied successfully",
			},
		}
	} else {
		rg.Status.Phase = "Failed"
		rg.Status.CompletedAt = &now
		rg.Status.Conditions = []metav1.Condition{
			{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "ExecutionFailed",
				Message:            "One or more resources failed to apply",
			},
		}
	}

	rg.Status.ObservedGeneration = rg.Generation

	if err := r.Status().Update(ctx, rg); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if not complete to check readiness
	if !state.IsComplete() {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

