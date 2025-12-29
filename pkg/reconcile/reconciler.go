package reconcile

import (
	"context"
	"time"

	"github.com/authzed/controller-idioms/handler"
	"github.com/authzed/controller-idioms/queue"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/chazu/pequod/pkg/platformloader"
)

// Reconciler implements the WebService reconciliation logic using handlers
type Reconciler struct {
	handlers *ReconcileHandlers
	pipeline handler.Handler
}

// NewReconciler creates a new handler-based reconciler
// Note: executor and pruner have been removed - execution is now handled by ResourceGraph controller
func NewReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	renderer *platformloader.Renderer,
) *Reconciler {
	// Create handlers
	handlers := NewReconcileHandlers(
		client,
		scheme,
		nil, // recorder will be set by controller
		renderer,
	)

	// Build the reconciliation pipeline
	// Note: ExecuteDAG and PruneOrphaned have been moved to ResourceGraph controller
	pipeline := handler.Chain(
		handlers.FetchWebService(),
		handlers.CheckPause(),          // Check for pause annotation
		handlers.AdoptResources(),      // Adopt referenced Secrets/ConfigMaps
		handlers.RenderGraph(),         // Render CUE to Graph
		handlers.CreateResourceGraph(), // Create ResourceGraph CR
		handlers.UpdateStatus(),        // Update WebService status
	).Handler("webservice-reconcile")

	return &Reconciler{
		handlers: handlers,
		pipeline: pipeline,
	}
}

// SetRecorder sets the event recorder for the handlers
func (r *Reconciler) SetRecorder(recorder record.EventRecorder) {
	r.handlers.recorder = recorder
}

// Reconcile executes the reconciliation pipeline
func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling WebService", "namespacedName", req.NamespacedName)

	// Create queue operations for this reconciliation
	var result ctrl.Result
	var requeueAfter time.Duration
	done := false

	queueOps := queue.NewOperations(
		func() { done = true },
		func(d time.Duration) { requeueAfter = d },
		nil, // cancel not needed
	)

	// Initialize context with request info and queue operations
	ctx = CtxNamespacedName.WithValue(ctx, req.NamespacedName)
	ctx = CtxQueue.WithValue(ctx, queueOps)

	// Execute the pipeline
	r.pipeline.Handle(ctx)

	// Build result from queue operations
	if err := queueOps.Error(); err != nil {
		return ctrl.Result{}, err
	}

	if !done && requeueAfter > 0 {
		result.RequeueAfter = requeueAfter
	} else if !done {
		result.Requeue = true
	}

	return result, nil
}
