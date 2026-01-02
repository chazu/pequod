package reconcile

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/chazu/pequod/pkg/platformloader"
)

// TransformReconciler implements the Transform reconciliation logic using handlers
type TransformReconciler struct {
	handlers *TransformHandlers
}

// NewTransformReconciler creates a new handler-based reconciler for Transform
func NewTransformReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	renderer *platformloader.Renderer,
) *TransformReconciler {
	// Create handlers
	handlers := NewTransformHandlers(
		client,
		scheme,
		nil, // recorder will be set by controller
		renderer,
	)

	return &TransformReconciler{
		handlers: handlers,
	}
}

// SetRecorder sets the event recorder for the handlers
func (r *TransformReconciler) SetRecorder(recorder record.EventRecorder) {
	r.handlers.recorder = recorder
}

// Reconcile executes the reconciliation pipeline for Transform
func (r *TransformReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling Transform", "namespacedName", req.NamespacedName)

	// Execute the reconciliation pipeline
	return r.handlers.Reconcile(ctx, req.NamespacedName)
}
