package reconcile

import (
	"context"
	"fmt"

	"github.com/authzed/controller-idioms/handler"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

// Handler IDs for the reconciliation pipeline
const (
	FetchWebServiceID       handler.Key = "fetch-webservice"
	RenderGraphID           handler.Key = "render-graph"
	CreateResourceGraphID   handler.Key = "create-resourcegraph"
	ExecuteDAGID            handler.Key = "execute-dag"       // Deprecated - moved to ResourceGraph controller
	PruneOrphanedID         handler.Key = "prune-orphaned"    // Deprecated - moved to ResourceGraph controller
	UpdateStatusID          handler.Key = "update-status"
)

// ReconcileHandlers contains all handlers for WebService reconciliation
type ReconcileHandlers struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	// Components used by handlers
	renderer *platformloader.Renderer
}

// NewReconcileHandlers creates a new handler collection
func NewReconcileHandlers(
	client client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	renderer *platformloader.Renderer,
) *ReconcileHandlers {
	return &ReconcileHandlers{
		client:   client,
		scheme:   scheme,
		recorder: recorder,
		renderer: renderer,
	}
}

// FetchWebServiceHandler fetches the WebService resource from the cluster
type FetchWebServiceHandler struct {
	client client.Client
	next   handler.Handler
}

func (h *FetchWebServiceHandler) Handle(ctx context.Context) {
	nn := CtxNamespacedName.MustValue(ctx)

	ws := &platformv1alpha1.WebService{}
	if err := h.client.Get(ctx, nn, ws); err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted - stop reconciliation
			CtxQueue.Done(ctx)
			return
		}
		CtxQueue.RequeueErr(ctx, err)
		return
	}

	// Add to context and continue
	ctx = CtxWebService.WithValue(ctx, ws)
	h.next.Handle(ctx)
}

// FetchWebService returns a handler builder for fetching the WebService
func (r *ReconcileHandlers) FetchWebService() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			&FetchWebServiceHandler{
				client: r.client,
				next:   handler.Handlers(next).MustOne(),
			},
			FetchWebServiceID,
		)
	}
}

// AdoptResources returns a handler builder for adopting external resources
func (r *ReconcileHandlers) AdoptResources() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			NewAdoptResourcesHandler(
				r.client,
				handler.Handlers(next).MustOne(),
			),
			"adopt-resources",
		)
	}
}

// RenderGraphHandler renders the CUE graph from the WebService spec
type RenderGraphHandler struct {
	renderer *platformloader.Renderer
	next     handler.Handler
}

func (h *RenderGraphHandler) Handle(ctx context.Context) {
	logger := log.FromContext(ctx)
	ws := CtxWebService.MustValue(ctx)

	// Render graph from WebService spec
	g, err := h.renderer.Render(
		ctx,
		ws.Name,
		ws.Namespace,
		ws.Spec.Image,
		ws.Spec.Port,
		ws.Spec.Replicas,
		"embedded", // TODO: Support platformRef from spec
	)
	if err != nil {
		CtxQueue.RequeueErr(ctx, fmt.Errorf("failed to render graph: %w", err))
		return
	}

	// Validate graph
	if err := g.Validate(); err != nil {
		CtxQueue.RequeueErr(ctx, fmt.Errorf("invalid graph: %w", err))
		return
	}

	// Compute hash for the graph
	g.SetHash()

	logger.Info("graph rendered successfully",
		"hash", g.Metadata.RenderHash,
		"nodes", len(g.Nodes))

	// Add to context and continue
	ctx = CtxGraph.WithValue(ctx, g)
	h.next.Handle(ctx)
}

// RenderGraph returns a handler builder for rendering the graph
func (r *ReconcileHandlers) RenderGraph() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			&RenderGraphHandler{
				renderer: r.renderer,
				next:     handler.Handlers(next).MustOne(),
			},
			RenderGraphID,
		)
	}
}

// CreateResourceGraphHandler creates a ResourceGraph CR from the rendered graph
type CreateResourceGraphHandler struct {
	client client.Client
	scheme *runtime.Scheme
	next   handler.Handler
}

func (h *CreateResourceGraphHandler) Handle(ctx context.Context) {
	ws := CtxWebService.MustValue(ctx)
	g := CtxGraph.MustValue(ctx)

	// Build ResourceGraph CR from the rendered graph
	rg, err := buildResourceGraphFromGraph(ws, g, h.scheme)
	if err != nil {
		CtxQueue.RequeueErr(ctx, fmt.Errorf("failed to build ResourceGraph: %w", err))
		return
	}

	// Create or update the ResourceGraph
	if err := createOrUpdateResourceGraph(ctx, h.client, rg); err != nil {
		CtxQueue.RequeueErr(ctx, fmt.Errorf("failed to create ResourceGraph: %w", err))
		return
	}

	// Store ResourceGraph in context for status update
	ctx = CtxResourceGraph.WithValue(ctx, rg)

	log.FromContext(ctx).Info("ResourceGraph created/updated",
		"resourceGraph", rg.Name,
		"nodes", len(rg.Spec.Nodes))

	h.next.Handle(ctx)
}

// CreateResourceGraph returns a handler builder for creating ResourceGraph CRs
func (r *ReconcileHandlers) CreateResourceGraph() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			&CreateResourceGraphHandler{
				client: r.client,
				scheme: r.scheme,
				next:   handler.Handlers(next).MustOne(),
			},
			CreateResourceGraphID,
		)
	}
}

// ExecuteDAG and PruneOrphaned handlers have been removed
// Execution is now handled by the ResourceGraph controller

// UpdateStatusHandler updates the WebService status with ResourceGraph reference
type UpdateStatusHandler struct {
	client   client.Client
	recorder record.EventRecorder
}

func (h *UpdateStatusHandler) Handle(ctx context.Context) {
	ws := CtxWebService.MustValue(ctx)
	rg := CtxResourceGraph.MustValue(ctx)

	// Update ResourceGraph reference
	ws.Status.ResourceGraphRef = &platformv1alpha1.ObjectReference{
		APIVersion: rg.APIVersion,
		Kind:       rg.Kind,
		Name:       rg.Name,
		Namespace:  rg.Namespace,
		UID:        string(rg.UID),
	}

	// Set Rendered condition
	ws.SetCondition(
		"Rendered",
		metav1.ConditionTrue,
		"GraphRendered",
		fmt.Sprintf("ResourceGraph %s created successfully", rg.Name),
	)

	// Update observed generation
	ws.Status.ObservedGeneration = ws.Generation

	// Update status
	if err := h.client.Status().Update(ctx, ws); err != nil {
		CtxQueue.RequeueErr(ctx, fmt.Errorf("failed to update status: %w", err))
		return
	}

	// Record event
	h.recorder.Eventf(ws, "Normal", "Rendered",
		"ResourceGraph %s created with %d nodes", rg.Name, len(rg.Spec.Nodes))

	// Done - successful reconciliation
	CtxQueue.Done(ctx)
}

// UpdateStatus returns a handler builder for updating status
func (r *ReconcileHandlers) UpdateStatus() handler.Builder {
	return func(next ...handler.Handler) handler.Handler {
		return handler.NewHandler(
			&UpdateStatusHandler{
				client:   r.client,
				recorder: r.recorder,
			},
			UpdateStatusID,
		)
	}
}



// Helper functions for ResourceGraph creation

// buildResourceGraphFromGraph converts a graph.Graph to a ResourceGraph CR
func buildResourceGraphFromGraph(ws *platformv1alpha1.WebService, g *graph.Graph, scheme *runtime.Scheme) (*platformv1alpha1.ResourceGraph, error) {
	// Generate a name for the ResourceGraph
	// Use WebService name + hash to make it unique and deterministic
	rgName := fmt.Sprintf("%s-%s", ws.Name, g.Metadata.RenderHash[:8])

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
			Namespace: ws.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         ws.APIVersion,
					Kind:               ws.Kind,
					Name:               ws.Name,
					UID:                ws.UID,
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
			Labels: map[string]string{
				"pequod.io/webservice": ws.Name,
				"pequod.io/transform":  "webservice",
			},
		},
		Spec: platformv1alpha1.ResourceGraphSpec{
			SourceRef: platformv1alpha1.ObjectReference{
				APIVersion: ws.APIVersion,
				Kind:       ws.Kind,
				Name:       ws.Name,
				Namespace:  ws.Namespace,
				UID:        string(ws.UID),
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
func createOrUpdateResourceGraph(ctx context.Context, c client.Client, rg *platformv1alpha1.ResourceGraph) error {
	// Try to get existing ResourceGraph
	existing := &platformv1alpha1.ResourceGraph{}
	err := c.Get(ctx, client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ResourceGraph
			if err := c.Create(ctx, rg); err != nil {
				return fmt.Errorf("failed to create ResourceGraph: %w", err)
			}
			log.FromContext(ctx).Info("Created ResourceGraph", "name", rg.Name)
			return nil
		}
		return fmt.Errorf("failed to get ResourceGraph: %w", err)
	}

	// Update existing ResourceGraph spec
	existing.Spec = rg.Spec
	if err := c.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ResourceGraph: %w", err)
	}

	// Copy the updated resource back to rg so we have the latest version
	*rg = *existing

	log.FromContext(ctx).Info("Updated ResourceGraph", "name", rg.Name)
	return nil
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}
