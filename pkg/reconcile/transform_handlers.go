package reconcile

import (
	"context"
	"fmt"

	"github.com/authzed/controller-idioms/pause"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

const (
	// PausedAnnotation is the annotation key to pause reconciliation
	PausedAnnotation = "platform.pequod.io/paused"

	// TransformFinalizer is the finalizer added to Transform resources
	TransformFinalizer = "platform.pequod.io/transform-finalizer"
)

// TransformHandlers contains all handlers for Transform reconciliation
type TransformHandlers struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	renderer *platformloader.Renderer
}

// NewTransformHandlers creates a new handler collection for Transform
func NewTransformHandlers(
	client client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	renderer *platformloader.Renderer,
) *TransformHandlers {
	return &TransformHandlers{
		client:   client,
		scheme:   scheme,
		recorder: recorder,
		renderer: renderer,
	}
}

// Reconcile executes the full reconciliation pipeline for a Transform
func (h *TransformHandlers) Reconcile(ctx context.Context, nn types.NamespacedName) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch the Transform
	tf := &platformv1alpha1.Transform{}
	if err := h.client.Get(ctx, nn, tf); err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted - stop reconciliation
			logger.Info("Transform not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 2: Handle finalizer
	if !tf.DeletionTimestamp.IsZero() {
		// Transform is being deleted
		return h.handleDeletion(ctx, tf)
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(tf, TransformFinalizer) {
		logger.Info("Adding finalizer to Transform")
		controllerutil.AddFinalizer(tf, TransformFinalizer)
		if err := h.client.Update(ctx, tf); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Requeue to continue reconciliation with finalizer in place
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 3: Check if paused
	if pause.IsPaused(tf, PausedAnnotation) {
		logger.Info("Transform is paused, skipping reconciliation", "annotation", PausedAnnotation)

		// Only update Paused condition if not already set to True
		existingCond := tf.GetCondition(pause.ConditionTypePaused)
		if existingCond == nil || existingCond.Status != metav1.ConditionTrue {
			tf.SetCondition(
				pause.ConditionTypePaused,
				metav1.ConditionTrue,
				"Paused",
				fmt.Sprintf("Reconciliation paused via %s annotation", PausedAnnotation),
			)

			if err := h.client.Status().Update(ctx, tf); err != nil {
				logger.Error(err, "failed to update paused condition")
			}
		}

		return ctrl.Result{}, nil
	}

	// Remove Paused condition only if it's currently set to True
	existingCond := tf.GetCondition(pause.ConditionTypePaused)
	if existingCond != nil && existingCond.Status == metav1.ConditionTrue {
		logger.Info("Transform unpaused, removing condition")
		tf.SetCondition(
			pause.ConditionTypePaused,
			metav1.ConditionFalse,
			"NotPaused",
			"Reconciliation is not paused",
		)

		if err := h.client.Status().Update(ctx, tf); err != nil {
			logger.Error(err, "failed to remove paused condition")
		}
	}

	// Step 3: Render the graph
	g, err := h.renderGraph(ctx, tf)
	if err != nil {
		tf.SetCondition(
			"Rendered",
			metav1.ConditionFalse,
			"RenderFailed",
			fmt.Sprintf("Failed to render graph: %v", err),
		)
		return ctrl.Result{}, err
	}

	// Step 4: Create/Update ResourceGraph
	rg, err := h.createOrUpdateResourceGraph(ctx, tf, g)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Step 5: Update Transform status
	return h.updateStatus(ctx, tf, rg)
}

// renderGraph renders the CUE graph from the Transform spec
func (h *TransformHandlers) renderGraph(ctx context.Context, tf *platformv1alpha1.Transform) (*graph.Graph, error) {
	logger := log.FromContext(ctx)

	// Get the platform reference from CueRef
	// For now, we only support embedded type
	platformRef := tf.Spec.CueRef.Ref
	if tf.Spec.CueRef.Type != platformv1alpha1.CueRefTypeEmbedded {
		return nil, fmt.Errorf("unsupported CueRef type: %s (only 'embedded' is currently supported)", tf.Spec.CueRef.Type)
	}

	// Render graph from Transform input
	// Note: RenderTransform validates the graph internally
	g, err := h.renderer.RenderTransform(
		ctx,
		tf.Name,
		tf.Namespace,
		tf.Spec.Input,
		platformRef,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to render graph: %w", err)
	}

	// Compute hash for the graph
	g.SetHash()

	logger.Info("graph rendered successfully",
		"hash", g.Metadata.RenderHash,
		"nodes", len(g.Nodes))

	return g, nil
}

// createOrUpdateResourceGraph creates or updates a ResourceGraph CR from the rendered graph
func (h *TransformHandlers) createOrUpdateResourceGraph(ctx context.Context, tf *platformv1alpha1.Transform, g *graph.Graph) (*platformv1alpha1.ResourceGraph, error) {
	logger := log.FromContext(ctx)

	// Build ResourceGraph CR from the rendered graph
	rg, err := buildResourceGraphFromTransform(tf, g, h.scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to build ResourceGraph: %w", err)
	}

	// Try to get existing ResourceGraph
	existing := &platformv1alpha1.ResourceGraph{}
	err = h.client.Get(ctx, client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ResourceGraph
			if err := h.client.Create(ctx, rg); err != nil {
				return nil, fmt.Errorf("failed to create ResourceGraph: %w", err)
			}
			logger.Info("Created ResourceGraph", "name", rg.Name)
			return rg, nil
		}
		return nil, fmt.Errorf("failed to get ResourceGraph: %w", err)
	}

	// Update existing ResourceGraph spec
	existing.Spec = rg.Spec
	if err := h.client.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update ResourceGraph: %w", err)
	}

	logger.Info("Updated ResourceGraph", "name", rg.Name)
	return existing, nil
}

// updateStatus updates the Transform status with ResourceGraph reference
func (h *TransformHandlers) updateStatus(ctx context.Context, tf *platformv1alpha1.Transform, rg *platformv1alpha1.ResourceGraph) (ctrl.Result, error) {
	// Update ResourceGraph reference
	tf.Status.ResourceGraphRef = &platformv1alpha1.ObjectReference{
		APIVersion: rg.APIVersion,
		Kind:       rg.Kind,
		Name:       rg.Name,
		Namespace:  rg.Namespace,
		UID:        string(rg.UID),
	}

	// Set Rendered condition
	tf.SetCondition(
		"Rendered",
		metav1.ConditionTrue,
		"GraphRendered",
		fmt.Sprintf("ResourceGraph %s created successfully", rg.Name),
	)

	// Update observed generation
	tf.Status.ObservedGeneration = tf.Generation

	// Update status
	if err := h.client.Status().Update(ctx, tf); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Record event
	if h.recorder != nil {
		h.recorder.Eventf(tf, "Normal", "Rendered",
			"ResourceGraph %s created with %d nodes", rg.Name, len(rg.Spec.Nodes))
	}

	return ctrl.Result{}, nil
}

// buildResourceGraphFromTransform converts a graph.Graph to a ResourceGraph CR
func buildResourceGraphFromTransform(tf *platformv1alpha1.Transform, g *graph.Graph, scheme *runtime.Scheme) (*platformv1alpha1.ResourceGraph, error) {
	// Generate a name for the ResourceGraph
	// Use Transform name + hash to make it unique and deterministic
	rgName := fmt.Sprintf("%s-%s", tf.Name, g.Metadata.RenderHash[:8])

	// Convert graph nodes to ResourceGraph nodes
	nodes := make([]platformv1alpha1.ResourceNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		// Marshal the unstructured object to JSON
		objJSON, err := node.Object.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal node %s: %w", node.ID, err)
		}

		// Convert ApplyPolicy
		applyPolicy := platformv1alpha1.ApplyPolicy{
			Mode:           string(node.ApplyPolicy.Mode),
			ConflictPolicy: string(node.ApplyPolicy.ConflictPolicy),
			FieldManager:   node.ApplyPolicy.FieldManager,
		}

		// Convert ReadyWhen predicates
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
			Namespace: tf.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         tf.APIVersion,
					Kind:               tf.Kind,
					Name:               tf.Name,
					UID:                tf.UID,
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
			Labels: map[string]string{
				"pequod.io/transform":      tf.Name,
				"pequod.io/transform-type": tf.Spec.CueRef.Ref,
			},
		},
		Spec: platformv1alpha1.ResourceGraphSpec{
			SourceRef: platformv1alpha1.ObjectReference{
				APIVersion: tf.APIVersion,
				Kind:       tf.Kind,
				Name:       tf.Name,
				Namespace:  tf.Namespace,
				UID:        string(tf.UID),
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

// handleDeletion handles Transform deletion by cleaning up and removing the finalizer
func (h *TransformHandlers) handleDeletion(ctx context.Context, tf *platformv1alpha1.Transform) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(tf, TransformFinalizer) {
		// Finalizer already removed, nothing to do
		return ctrl.Result{}, nil
	}

	logger.Info("Handling Transform deletion", "name", tf.Name)

	// Record deletion event
	if h.recorder != nil {
		h.recorder.Event(tf, "Normal", "Deleting", "Transform is being deleted")
	}

	// ResourceGraphs are deleted automatically via owner reference cascade
	// (BlockOwnerDeletion: true is set on the owner reference)
	// No additional cleanup required

	// Remove finalizer to allow deletion to proceed
	logger.Info("Removing finalizer from Transform")
	controllerutil.RemoveFinalizer(tf, TransformFinalizer)
	if err := h.client.Update(ctx, tf); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Transform deletion handled successfully")
	return ctrl.Result{}, nil
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}
