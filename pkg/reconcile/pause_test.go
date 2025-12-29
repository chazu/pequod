package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/authzed/controller-idioms/handler"
	"github.com/authzed/controller-idioms/pause"
	"github.com/authzed/controller-idioms/queue"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

func TestCheckPauseHandler_Paused(t *testing.T) {
	// Create a paused WebService (using labels, not annotations)
	ws := &platformv1alpha1.WebService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ws",
			Namespace: "default",
			Labels: map[string]string{
				PausedAnnotation: "true",
			},
		},
		Spec: platformv1alpha1.WebServiceSpec{
			Image: "nginx:latest",
			Port:  80,
		},
	}

	// Verify pause.IsPaused works
	if !pause.IsPaused(ws, PausedAnnotation) {
		t.Error("Expected IsPaused to return true")
	}

	// Verify NewPausedCondition creates correct condition
	cond := pause.NewPausedCondition(PausedAnnotation)
	if cond.Type != pause.ConditionTypePaused {
		t.Errorf("Expected condition type %s, got %s", pause.ConditionTypePaused, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Expected condition status True, got %s", cond.Status)
	}
	// The reason is "PausedByLabel" when using labels
	if cond.Reason != "PausedByLabel" {
		t.Errorf("Expected reason 'PausedByLabel', got %s", cond.Reason)
	}
}

func TestCheckPauseHandler_NotPaused(t *testing.T) {
	// Create a non-paused WebService
	ws := &platformv1alpha1.WebService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ws",
			Namespace: "default",
		},
		Spec: platformv1alpha1.WebServiceSpec{
			Image: "nginx:latest",
			Port:  80,
		},
	}

	scheme := runtime.NewScheme()
	_ = platformv1alpha1.AddToScheme(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ws).
		WithStatusSubresource(ws).
		Build()

	nextCalled := false
	h := &CheckPauseHandler{
		client: client,
		next: handler.NewHandler(&mockHandler{
			handleFunc: func(ctx context.Context) {
				nextCalled = true
			},
		}, "mock"),
	}

	ctx := context.Background()
	ctx = CtxWebService.WithValue(ctx, ws)

	queueOps := queue.NewOperations(
		func() {},
		func(d time.Duration) {},
		nil,
	)
	ctx = CtxQueue.WithValue(ctx, queueOps)

	h.Handle(ctx)

	// Verify next handler was called
	if !nextCalled {
		t.Error("Expected next handler to be called")
	}

	// Verify no Paused condition
	cond := ws.FindStatusCondition(pause.ConditionTypePaused)
	if cond != nil {
		t.Error("Expected no Paused condition")
	}
}

func TestCheckPauseHandler_Unpaused(t *testing.T) {
	// Create a WebService that was paused but is now unpaused
	ws := &platformv1alpha1.WebService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ws",
			Namespace: "default",
			// No pause annotation
		},
		Spec: platformv1alpha1.WebServiceSpec{
			Image: "nginx:latest",
			Port:  80,
		},
		Status: platformv1alpha1.WebServiceStatus{
			Conditions: []metav1.Condition{
				{
					Type:   pause.ConditionTypePaused,
					Status: metav1.ConditionTrue,
					Reason: "Paused",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = platformv1alpha1.AddToScheme(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ws).
		WithStatusSubresource(ws).
		Build()

	nextCalled := false
	h := &CheckPauseHandler{
		client: client,
		next: handler.NewHandler(&mockHandler{
			handleFunc: func(ctx context.Context) {
				nextCalled = true
			},
		}, "mock"),
	}

	ctx := context.Background()
	ctx = CtxWebService.WithValue(ctx, ws)

	queueOps := queue.NewOperations(
		func() {},
		func(d time.Duration) {},
		nil,
	)
	ctx = CtxQueue.WithValue(ctx, queueOps)

	h.Handle(ctx)

	// Verify next handler was called
	if !nextCalled {
		t.Error("Expected next handler to be called")
	}

	// Verify Paused condition was removed
	cond := ws.FindStatusCondition(pause.ConditionTypePaused)
	if cond != nil {
		t.Error("Expected Paused condition to be removed")
	}
}

// mockHandler is a simple handler for testing
type mockHandler struct {
	handleFunc func(ctx context.Context)
}

func (m *mockHandler) Handle(ctx context.Context) {
	if m.handleFunc != nil {
		m.handleFunc(ctx)
	}
}
