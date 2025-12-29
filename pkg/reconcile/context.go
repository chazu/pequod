package reconcile

import (
	"github.com/authzed/controller-idioms/queue"
	"github.com/authzed/controller-idioms/typedctx"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

// Context keys for reconciliation pipeline
//
// These typed context keys provide type-safe access to values passed between
// handlers in the reconciliation pipeline. Using typedctx eliminates runtime
// errors from context.Value() type assertions.
var (
	// CtxQueue provides queue operations for controlling reconciliation
	CtxQueue = queue.NewQueueOperationsCtx()

	// CtxNamespacedName is the resource being reconciled
	CtxNamespacedName = typedctx.NewKey[types.NamespacedName]()

	// CtxWebService is the fetched WebService resource
	CtxWebService = typedctx.NewKey[*platformv1alpha1.WebService]()

	// CtxGraph is the rendered dependency graph
	CtxGraph = typedctx.NewKey[*graph.Graph]()

	// CtxResourceGraph is the created ResourceGraph CR
	CtxResourceGraph = typedctx.NewKey[*platformv1alpha1.ResourceGraph]()

	// CtxExecutionState tracks DAG execution (deprecated - moved to ResourceGraph controller)
	// Kept for backward compatibility but not used
	CtxExecutionState = typedctx.NewKey[*graph.ExecutionState]()

	// CtxRenderer is the CUE renderer
	CtxRenderer = typedctx.NewKey[*platformloader.Renderer]()
)
