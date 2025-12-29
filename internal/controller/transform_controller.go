/*
Copyright 2024.

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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

const (
	transformFinalizer = "platform.pequod.io/transform-finalizer"
	transformLabel     = "pequod.io/transform"
)

// TransformReconciler reconciles any Transform resource (WebService, Database, etc.)
type TransformReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	PlatformLoader *platformloader.Loader
	Renderer       *platformloader.Renderer
	Recorder       record.EventRecorder
}

// +kubebuilder:rbac:groups=platform.pequod.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.pequod.io,resources=*/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.pequod.io,resources=*/finalizers,verbs=update

// Reconcile handles any Transform resource
func (r *TransformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling Transform", "name", req.Name, "namespace", req.Namespace)

	// For now, we only support WebService
	// In the future, we'll need to determine the GVK from the request or use a registry
	gvk := schema.GroupVersionKind{
		Group:   "platform.platform.example.com",
		Version: "v1alpha1",
		Kind:    "WebService",
	}

	// Fetch the resource as unstructured (could be WebService, Database, etc.)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Transform resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Transform resource")
		return ctrl.Result{}, err
	}

	logger.Info("Processing Transform", "gvk", gvk.String())

	// Check if resource has transform label
	labels := obj.GetLabels()
	if labels == nil || labels[transformLabel] != "true" {
		logger.Info("Resource does not have transform label, ignoring")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if !obj.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, obj)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(obj, transformFinalizer) {
		controllerutil.AddFinalizer(obj, transformFinalizer)
		if err := r.Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Render the transform to a Graph
	g, err := r.renderTransform(ctx, obj)
	if err != nil {
		logger.Error(err, "Failed to render transform")
		return r.updateStatusFailed(ctx, obj, fmt.Sprintf("Failed to render: %v", err))
	}

	// Create ResourceGraph from the rendered graph
	rg, err := r.buildResourceGraph(obj, g)
	if err != nil {
		logger.Error(err, "Failed to build ResourceGraph")
		return r.updateStatusFailed(ctx, obj, fmt.Sprintf("Failed to build ResourceGraph: %v", err))
	}

	// Create or update ResourceGraph
	if err := r.createOrUpdateResourceGraph(ctx, rg); err != nil {
		logger.Error(err, "Failed to create/update ResourceGraph")
		return ctrl.Result{}, err
	}

	// Update status
	return r.updateStatusRendered(ctx, obj, rg)
}

// SetupWithManager sets up the controller with the Manager
// This will be called with dynamic watches for each discovered platform
func (r *TransformReconciler) SetupWithManager(mgr ctrl.Manager, gvks []schema.GroupVersionKind) error {
	// Create predicate to only handle resources with transform label
	transformPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		return labels != nil && labels[transformLabel] == "true"
	})

	builder := ctrl.NewControllerManagedBy(mgr).Named("transform")

	// Add watches for each GVK
	for _, gvk := range gvks {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)

		// Watch with predicate to only handle resources with transform label
		builder = builder.For(obj).WithEventFilter(transformPredicate)
	}

	return builder.Complete(r)
}




// renderTransform renders a Transform resource to a Graph using CUE
func (r *TransformReconciler) renderTransform(ctx context.Context, obj *unstructured.Unstructured) (*graph.Graph, error) {
	// Extract spec from the unstructured object
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("failed to get spec: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("spec not found in object")
	}

	// For now, we'll use the existing Renderer which is hardcoded for WebService
	// TODO: Make this generic by passing the full object to CUE

	// Extract WebService-specific fields (temporary until we make renderer generic)
	image, _, _ := unstructured.NestedString(spec, "image")
	port, _, _ := unstructured.NestedInt64(spec, "port")
	replicas, _, _ := unstructured.NestedInt64(spec, "replicas")

	replicasPtr := new(int32)
	*replicasPtr = int32(replicas)

	// Use the renderer (hardcoded for WebService for now)
	return r.Renderer.Render(ctx, obj.GetName(), obj.GetNamespace(), image, int32(port), replicasPtr, "embedded")
}

// buildResourceGraph creates a ResourceGraph CR from a rendered Graph
func (r *TransformReconciler) buildResourceGraph(obj *unstructured.Unstructured, g *graph.Graph) (*platformv1alpha1.ResourceGraph, error) {
	// Generate name for ResourceGraph
	hashSuffix := "unknown"
	if len(g.Metadata.RenderHash) >= 8 {
		hashSuffix = g.Metadata.RenderHash[:8]
	} else if len(g.Metadata.RenderHash) > 0 {
		hashSuffix = g.Metadata.RenderHash
	}
	rgName := fmt.Sprintf("%s-%s", obj.GetName(), hashSuffix)

	// Convert graph nodes to ResourceGraph nodes
	nodes := make([]platformv1alpha1.ResourceNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		objJSON, err := node.Object.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal node %s: %w", node.ID, err)
		}

		applyPolicy := platformv1alpha1.ApplyPolicy{
			Mode:           string(node.ApplyPolicy.Mode),
			ConflictPolicy: string(node.ApplyPolicy.ConflictPolicy),
			FieldManager:   node.ApplyPolicy.FieldManager,
		}

		readyWhen := make([]platformv1alpha1.ReadinessPredicate, 0, len(node.ReadyWhen))
		for _, rw := range node.ReadyWhen {
			pred := platformv1alpha1.ReadinessPredicate{
				Type:            string(rw.Type),
				ConditionType:   rw.ConditionType,
				ConditionStatus: rw.ConditionStatus,
			}
			readyWhen = append(readyWhen, pred)
		}

		rgNode := platformv1alpha1.ResourceNode{
			ID:          node.ID,
			Object:      runtime.RawExtension{Raw: objJSON},
			ApplyPolicy: applyPolicy,
			DependsOn:   node.DependsOn,
			ReadyWhen:   readyWhen,
		}

		nodes = append(nodes, rgNode)
	}

	// Convert violations
	violations := make([]platformv1alpha1.PolicyViolation, 0, len(g.Violations))
	for _, v := range g.Violations {
		violations = append(violations, platformv1alpha1.PolicyViolation{
			Path:     v.Path,
			Message:  v.Message,
			Severity: string(v.Severity),
		})
	}

	// Create ResourceGraph CR
	rg := &platformv1alpha1.ResourceGraph{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgName,
			Namespace: obj.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         obj.GetAPIVersion(),
					Kind:               obj.GetKind(),
					Name:               obj.GetName(),
					UID:                obj.GetUID(),
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
			Labels: map[string]string{
				"pequod.io/transform":      obj.GetKind(),
				"pequod.io/transform-name": obj.GetName(),
			},
		},
		Spec: platformv1alpha1.ResourceGraphSpec{
			SourceRef: platformv1alpha1.ObjectReference{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
				Name:       obj.GetName(),
				Namespace:  obj.GetNamespace(),
				UID:        string(obj.GetUID()),
			},
			Metadata: platformv1alpha1.GraphMetadata{
				Name:        g.Metadata.Name,
				Version:     g.Metadata.Version,
				PlatformRef: g.Metadata.PlatformRef,
			},
			Nodes:      nodes,
			Violations: violations,
			RenderHash: g.Metadata.RenderHash,
			RenderedAt: metav1.Now(),
		},
	}

	return rg, nil
}

// createOrUpdateResourceGraph creates or updates a ResourceGraph CR
func (r *TransformReconciler) createOrUpdateResourceGraph(ctx context.Context, rg *platformv1alpha1.ResourceGraph) error {
	existing := &platformv1alpha1.ResourceGraph{}
	err := r.Get(ctx, client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, rg); err != nil {
				return fmt.Errorf("failed to create ResourceGraph: %w", err)
			}
			logf.FromContext(ctx).Info("Created ResourceGraph", "name", rg.Name)
			return nil
		}
		return fmt.Errorf("failed to get ResourceGraph: %w", err)
	}

	// Update existing
	existing.Spec = rg.Spec
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ResourceGraph: %w", err)
	}

	*rg = *existing
	logf.FromContext(ctx).Info("Updated ResourceGraph", "name", rg.Name)
	return nil
}


// updateStatusRendered updates the Transform status to Rendered
func (r *TransformReconciler) updateStatusRendered(ctx context.Context, obj *unstructured.Unstructured, rg *platformv1alpha1.ResourceGraph) (ctrl.Result, error) {
	// Update status with ResourceGraph reference
	status := map[string]interface{}{
		"resourceGraphRef": map[string]interface{}{
			"apiVersion": rg.APIVersion,
			"kind":       rg.Kind,
			"name":       rg.Name,
			"namespace":  rg.Namespace,
			"uid":        string(rg.UID),
		},
		"observedGeneration": obj.GetGeneration(),
		"conditions": []interface{}{
			map[string]interface{}{
				"type":               "Rendered",
				"status":             "True",
				"lastTransitionTime": metav1.Now().Format(metav1.RFC3339Micro),
				"reason":             "GraphRendered",
				"message":            fmt.Sprintf("ResourceGraph %s created successfully", rg.Name),
			},
		},
	}

	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set status: %w", err)
	}

	if err := r.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	r.Recorder.Eventf(obj, "Normal", "Rendered",
		"ResourceGraph %s created with %d nodes", rg.Name, len(rg.Spec.Nodes))

	return ctrl.Result{}, nil
}

// updateStatusFailed updates the Transform status to Failed
func (r *TransformReconciler) updateStatusFailed(ctx context.Context, obj *unstructured.Unstructured, message string) (ctrl.Result, error) {
	status := map[string]interface{}{
		"observedGeneration": obj.GetGeneration(),
		"conditions": []interface{}{
			map[string]interface{}{
				"type":               "Rendered",
				"status":             "False",
				"lastTransitionTime": metav1.Now().Format(metav1.RFC3339Micro),
				"reason":             "RenderFailed",
				"message":            message,
			},
		},
	}

	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set status: %w", err)
	}

	if err := r.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	r.Recorder.Event(obj, "Warning", "RenderFailed", message)

	return ctrl.Result{}, nil
}

// handleDeletion handles Transform deletion
func (r *TransformReconciler) handleDeletion(ctx context.Context, obj *unstructured.Unstructured) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling Transform deletion")

	// ResourceGraph will be deleted automatically via OwnerReference
	// Just remove the finalizer

	if controllerutil.ContainsFinalizer(obj, transformFinalizer) {
		controllerutil.RemoveFinalizer(obj, transformFinalizer)
		if err := r.Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func ptr[T any](v T) *T {
	return &v
}

