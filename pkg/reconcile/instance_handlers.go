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

package reconcile

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

const (
	// InstanceFinalizer is the finalizer added to platform instances
	InstanceFinalizer = "platform.pequod.io/instance-finalizer"

	// TransformAnnotation links an instance to its Transform
	TransformAnnotation = "platform.pequod.io/transform"

	// TransformNamespaceAnnotation is the namespace of the Transform
	TransformNamespaceAnnotation = "platform.pequod.io/transform-namespace"
)

// InstanceHandlers contains handlers for platform instance reconciliation.
// Platform instances are CRs created from dynamically generated CRDs (e.g., WebService).
// This handler renders the CUE template with the instance spec to produce a ResourceGraph.
type InstanceHandlers struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	renderer *platformloader.Renderer
}

// NewInstanceHandlers creates a new handler collection for platform instances
func NewInstanceHandlers(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	renderer *platformloader.Renderer,
) *InstanceHandlers {
	return &InstanceHandlers{
		client:   k8sClient,
		scheme:   scheme,
		recorder: recorder,
		renderer: renderer,
	}
}

// Reconcile handles reconciliation of a platform instance.
// It finds the source Transform, renders the CUE template, and creates a ResourceGraph.
func (h *InstanceHandlers) Reconcile(
	ctx context.Context,
	instance *unstructured.Unstructured,
	transform *platformv1alpha1.Transform,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling platform instance",
		"gvk", instance.GroupVersionKind(),
		"name", instance.GetName(),
		"namespace", instance.GetNamespace())

	// Handle deletion
	if !instance.GetDeletionTimestamp().IsZero() {
		return h.handleDeletion(ctx, instance)
	}

	// Ensure finalizer
	if !containsFinalizer(instance, InstanceFinalizer) {
		logger.Info("Adding finalizer to instance")
		addFinalizer(instance, InstanceFinalizer)
		if err := h.client.Update(ctx, instance); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Build CueRef from Transform
	cueRef := platformloader.CueRefInput{
		Type: string(transform.Spec.CueRef.Type),
		Ref:  transform.Spec.CueRef.Ref,
		Path: transform.Spec.CueRef.Path,
	}
	if transform.Spec.CueRef.PullSecretRef != nil {
		cueRef.PullSecretRef = &transform.Spec.CueRef.PullSecretRef.Name
	}

	// Extract the spec from the instance
	spec, found, err := unstructured.NestedMap(instance.Object, "spec")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get spec from instance: %w", err)
	}
	if !found {
		spec = make(map[string]interface{})
	}

	// Create raw extension with the spec
	rawSpec := runtime.RawExtension{}
	if len(spec) > 0 {
		specJSON, err := json.Marshal(spec)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to marshal spec: %w", err)
		}
		rawSpec.Raw = specJSON
	}

	// Render the graph
	logger.Info("Rendering CUE template",
		"cueRefType", cueRef.Type,
		"cueRef", cueRef.Ref)

	g, fetchResult, err := h.renderer.RenderTransformWithCueRef(
		ctx,
		instance.GetName(),
		instance.GetNamespace(),
		rawSpec,
		cueRef,
	)
	if err != nil {
		logger.Error(err, "Failed to render CUE template")
		h.recordEvent(instance, "Warning", "RenderFailed", "Failed to render CUE template: %v", err)
		return ctrl.Result{}, err
	}

	g.SetHash()

	logger.Info("CUE template rendered successfully",
		"nodeCount", len(g.Nodes),
		"hash", g.Metadata.RenderHash,
		"source", fetchResult.Source)

	// Build the ResourceGraph
	rg, err := h.buildResourceGraph(instance, transform, g)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to build ResourceGraph: %w", err)
	}

	// Create or update the ResourceGraph
	if err := h.applyResourceGraph(ctx, rg); err != nil {
		logger.Error(err, "Failed to apply ResourceGraph")
		h.recordEvent(instance, "Warning", "ApplyFailed", "Failed to apply ResourceGraph: %v", err)
		return ctrl.Result{}, err
	}

	logger.Info("ResourceGraph applied successfully",
		"resourceGraph", rg.Name,
		"nodeCount", len(rg.Spec.Nodes))

	h.recordEvent(instance, "Normal", "Rendered", "Created ResourceGraph %s with %d nodes", rg.Name, len(rg.Spec.Nodes))

	return ctrl.Result{}, nil
}

// buildResourceGraph creates a ResourceGraph from the rendered graph
func (h *InstanceHandlers) buildResourceGraph(
	instance *unstructured.Unstructured,
	transform *platformv1alpha1.Transform,
	g *graph.Graph,
) (*platformv1alpha1.ResourceGraph, error) {
	// Build resource graph name from instance
	rgName := fmt.Sprintf("%s-%s", instance.GetName(), g.Metadata.RenderHash[:8])

	// Convert graph nodes to ResourceGraph nodes
	nodes := make([]platformv1alpha1.ResourceNode, len(g.Nodes))
	for i, node := range g.Nodes {
		objBytes, err := node.Object.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal node object: %w", err)
		}

		nodes[i] = platformv1alpha1.ResourceNode{
			ID:     node.ID,
			Object: runtime.RawExtension{Raw: objBytes},
			ApplyPolicy: platformv1alpha1.ApplyPolicy{
				Mode:           string(node.ApplyPolicy.Mode),
				ConflictPolicy: string(node.ApplyPolicy.ConflictPolicy),
			},
			DependsOn: node.DependsOn,
		}

		// Convert readiness predicates
		if len(node.ReadyWhen) > 0 {
			nodes[i].ReadyWhen = make([]platformv1alpha1.ReadinessPredicate, len(node.ReadyWhen))
			for j, pred := range node.ReadyWhen {
				nodes[i].ReadyWhen[j] = platformv1alpha1.ReadinessPredicate{
					Type:            string(pred.Type),
					ConditionType:   pred.ConditionType,
					ConditionStatus: pred.ConditionStatus,
				}
			}
		}
	}

	gvk := instance.GroupVersionKind()

	rg := &platformv1alpha1.ResourceGraph{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgName,
			Namespace: instance.GetNamespace(),
			Labels: map[string]string{
				"pequod.io/instance":       instance.GetName(),
				"pequod.io/instance-kind":  gvk.Kind,
				"pequod.io/instance-group": gvk.Group,
				"pequod.io/transform":      transform.Name,
			},
			Annotations: map[string]string{
				TransformAnnotation:          transform.Name,
				TransformNamespaceAnnotation: transform.Namespace,
			},
		},
		Spec: platformv1alpha1.ResourceGraphSpec{
			SourceRef: platformv1alpha1.ObjectReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       instance.GetName(),
				Namespace:  instance.GetNamespace(),
			},
			Metadata: platformv1alpha1.GraphMetadata{
				Name:    g.Metadata.Name,
				Version: g.Metadata.Version,
			},
			Nodes:      nodes,
			RenderHash: g.Metadata.RenderHash,
			RenderedAt: metav1.Now(),
		},
	}

	// Set owner reference to the instance
	// Note: We use unstructured owner reference since the instance is dynamic
	ownerRef := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               instance.GetName(),
		UID:                instance.GetUID(),
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}
	rg.OwnerReferences = []metav1.OwnerReference{ownerRef}

	return rg, nil
}

// applyResourceGraph creates or updates the ResourceGraph
func (h *InstanceHandlers) applyResourceGraph(ctx context.Context, rg *platformv1alpha1.ResourceGraph) error {
	logger := log.FromContext(ctx)

	// Check if a ResourceGraph already exists for this instance
	existing := &platformv1alpha1.ResourceGraphList{}
	if err := h.client.List(ctx, existing,
		client.InNamespace(rg.Namespace),
		client.MatchingLabels{"pequod.io/instance": rg.Labels["pequod.io/instance"]},
	); err != nil {
		return fmt.Errorf("failed to list existing ResourceGraphs: %w", err)
	}

	// Delete old ResourceGraphs with different hashes
	for _, oldRG := range existing.Items {
		if oldRG.Name != rg.Name {
			logger.Info("Deleting old ResourceGraph", "name", oldRG.Name)
			if err := h.client.Delete(ctx, &oldRG); err != nil {
				logger.Error(err, "Failed to delete old ResourceGraph", "name", oldRG.Name)
				// Continue - don't fail on cleanup errors
			}
		}
	}

	// Create or update the new ResourceGraph
	existingRG := &platformv1alpha1.ResourceGraph{}
	err := h.client.Get(ctx, types.NamespacedName{Name: rg.Name, Namespace: rg.Namespace}, existingRG)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		// Create new
		logger.Info("Creating ResourceGraph", "name", rg.Name)
		return h.client.Create(ctx, rg)
	}

	// Update existing
	existingRG.Spec = rg.Spec
	existingRG.Labels = rg.Labels
	existingRG.Annotations = rg.Annotations
	logger.Info("Updating ResourceGraph", "name", rg.Name)
	return h.client.Update(ctx, existingRG)
}

// handleDeletion handles instance deletion
func (h *InstanceHandlers) handleDeletion(ctx context.Context, instance *unstructured.Unstructured) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !containsFinalizer(instance, InstanceFinalizer) {
		return ctrl.Result{}, nil
	}

	logger.Info("Handling instance deletion", "name", instance.GetName())

	// Delete associated ResourceGraphs
	rgList := &platformv1alpha1.ResourceGraphList{}
	if err := h.client.List(ctx, rgList,
		client.InNamespace(instance.GetNamespace()),
		client.MatchingLabels{"pequod.io/instance": instance.GetName()},
	); err != nil {
		logger.Error(err, "Failed to list ResourceGraphs for deletion")
		// Continue with finalizer removal
	} else {
		for _, rg := range rgList.Items {
			logger.Info("Deleting ResourceGraph", "name", rg.Name)
			if err := h.client.Delete(ctx, &rg); err != nil && client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete ResourceGraph", "name", rg.Name)
			}
		}
	}

	// Record event
	h.recordEvent(instance, "Normal", "Deleting", "Instance is being deleted")

	// Remove finalizer
	removeFinalizer(instance, InstanceFinalizer)
	if err := h.client.Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Instance deletion handled successfully")
	return ctrl.Result{}, nil
}

// recordEvent records an event for the instance
func (h *InstanceHandlers) recordEvent(instance *unstructured.Unstructured, eventType, reason, messageFmt string, args ...interface{}) {
	if h.recorder == nil {
		return
	}
	h.recorder.Eventf(instance, eventType, reason, messageFmt, args...)
}

// FindTransformForGVK finds the Transform that generated the CRD for the given GVK
func FindTransformForGVK(ctx context.Context, c client.Client, gvk schema.GroupVersionKind) (*platformv1alpha1.Transform, error) {
	logger := log.FromContext(ctx)

	// List all Transforms
	transforms := &platformv1alpha1.TransformList{}
	if err := c.List(ctx, transforms); err != nil {
		return nil, fmt.Errorf("failed to list Transforms: %w", err)
	}

	// Find the Transform whose GeneratedCRD matches this GVK
	for _, tf := range transforms.Items {
		if tf.Status.GeneratedCRD == nil {
			continue
		}

		// Parse the APIVersion from the GeneratedCRD reference
		generatedGV, err := schema.ParseGroupVersion(tf.Status.GeneratedCRD.APIVersion)
		if err != nil {
			logger.Error(err, "Failed to parse GeneratedCRD APIVersion", "transform", tf.Name)
			continue
		}

		// Check if this Transform generated the CRD for our GVK
		if generatedGV.Group == gvk.Group &&
			generatedGV.Version == gvk.Version &&
			tf.Status.GeneratedCRD.Kind == gvk.Kind {
			return &tf, nil
		}
	}

	return nil, fmt.Errorf("no Transform found for GVK %s", gvk.String())
}

// Helper functions for finalizer management on unstructured objects

func containsFinalizer(obj *unstructured.Unstructured, finalizer string) bool {
	finalizers := obj.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func addFinalizer(obj *unstructured.Unstructured, finalizer string) {
	finalizers := obj.GetFinalizers()
	finalizers = append(finalizers, finalizer)
	obj.SetFinalizers(finalizers)
}

func removeFinalizer(obj *unstructured.Unstructured, finalizer string) {
	finalizers := obj.GetFinalizers()
	var newFinalizers []string
	for _, f := range finalizers {
		if f != finalizer {
			newFinalizers = append(newFinalizers, f)
		}
	}
	obj.SetFinalizers(newFinalizers)
}

func boolPtr(b bool) *bool {
	return &b
}
