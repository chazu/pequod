package reconcile

import (
	"context"

	"github.com/authzed/controller-idioms/handler"
	"github.com/authzed/controller-idioms/pause"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// PausedAnnotation is the annotation key to pause reconciliation
	PausedAnnotation = "platform.pequod.io/paused"
)

// CheckPauseHandler checks if the WebService is paused via annotation
type CheckPauseHandler struct {
	client client.Client
	next   handler.Handler
}

func (h *CheckPauseHandler) Handle(ctx context.Context) {
	logger := log.FromContext(ctx)
	ws := CtxWebService.MustValue(ctx)

	// Check if paused
	if pause.IsPaused(ws, PausedAnnotation) {
		logger.Info("resource is paused, skipping reconciliation",
			"annotation", PausedAnnotation)

		// Update Paused condition
		pausedCondition := pause.NewPausedCondition(PausedAnnotation)
		ws.SetStatusCondition(pausedCondition)

		// Update status with paused condition
		if err := h.client.Status().Update(ctx, ws); err != nil {
			logger.Error(err, "failed to update paused condition")
		}

		// Stop reconciliation
		CtxQueue.Done(ctx)
		return
	}

	// Remove Paused condition if it exists
	if ws.FindStatusCondition(pause.ConditionTypePaused) != nil {
		logger.Info("resource unpaused, removing condition")
		ws.RemoveStatusCondition(pause.ConditionTypePaused)

		// Update status
		if err := h.client.Status().Update(ctx, ws); err != nil {
			logger.Error(err, "failed to remove paused condition")
		}
	}

	// Continue with reconciliation
	h.next.Handle(ctx)
}

// CheckPause returns a handler builder for checking pause annotation
func (r *ReconcileHandlers) CheckPause() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			&CheckPauseHandler{
				client: r.client,
				next:   handler.Handlers(next).MustOne(),
			},
			"check-pause",
		)
	}
}
