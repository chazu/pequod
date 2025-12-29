package reconcile

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/platformloader"
)

func TestTypedContextKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("CtxNamespacedName", func(t *testing.T) {
		nn := types.NamespacedName{
			Namespace: "default",
			Name:      "test",
		}

		ctx := CtxNamespacedName.WithValue(ctx, nn)
		got := CtxNamespacedName.MustValue(ctx)

		if got != nn {
			t.Errorf("Expected %v, got %v", nn, got)
		}
	})

	t.Run("CtxWebService", func(t *testing.T) {
		ws := &platformv1alpha1.WebService{}
		ws.Name = "test-ws"

		ctx := CtxWebService.WithValue(ctx, ws)
		got := CtxWebService.MustValue(ctx)

		if got.Name != "test-ws" {
			t.Errorf("Expected name 'test-ws', got %s", got.Name)
		}
	})

	t.Run("CtxGraph", func(t *testing.T) {
		g := &graph.Graph{
			Metadata: graph.GraphMetadata{
				Name:    "test-graph",
				Version: "v1",
			},
		}

		ctx := CtxGraph.WithValue(ctx, g)
		got := CtxGraph.MustValue(ctx)

		if got.Metadata.Name != "test-graph" {
			t.Errorf("Expected graph name 'test-graph', got %s", got.Metadata.Name)
		}
	})

	t.Run("CtxResourceGraph", func(t *testing.T) {
		rg := &platformv1alpha1.ResourceGraph{}
		rg.Name = "test-rg"

		ctx := CtxResourceGraph.WithValue(ctx, rg)
		got := CtxResourceGraph.MustValue(ctx)

		if got.Name != "test-rg" {
			t.Errorf("Expected name 'test-rg', got %s", got.Name)
		}
	})

	t.Run("CtxRenderer", func(t *testing.T) {
		renderer := &platformloader.Renderer{}

		ctx := CtxRenderer.WithValue(ctx, renderer)
		got := CtxRenderer.MustValue(ctx)

		if got == nil {
			t.Error("Expected non-nil renderer")
		}
	})

	t.Run("CtxExecutionState - Deprecated", func(t *testing.T) {
		// CtxExecutionState is deprecated but kept for backward compatibility
		// Just verify the key is defined
		if CtxExecutionState == nil {
			t.Error("Expected non-nil key")
		}
	})
}

func TestTypedContextOptionalValues(t *testing.T) {
	ctx := context.Background()

	t.Run("Value returns nil for missing key", func(t *testing.T) {
		// Value() returns nil if key not set (doesn't panic)
		got, ok := CtxWebService.Value(ctx)
		if ok {
			t.Error("Expected ok=false for missing key")
		}
		if got != nil {
			t.Error("Expected nil for missing key")
		}
	})

	t.Run("MustValue panics for missing key", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for missing key")
			}
		}()

		// MustValue() panics if key not set
		_ = CtxWebService.MustValue(ctx)
	})
}
