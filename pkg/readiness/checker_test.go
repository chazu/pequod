package readiness

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chazu/pequod/pkg/graph"
)

func TestChecker_Check(t *testing.T) {
	tests := []struct {
		name       string
		obj        *unstructured.Unstructured
		predicates []graph.ReadinessPredicate
		wantReady  bool
		wantErr    bool
	}{
		{
			name: "no predicates - ready",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "test-pod",
						"namespace": "default",
					},
				},
			},
			predicates: []graph.ReadinessPredicate{},
			wantReady:  true,
			wantErr:    false,
		},
		{
			name: "exists predicate - resource exists",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
				},
			},
			predicates: []graph.ReadinessPredicate{
				{
					Type: graph.PredicateTypeExists,
				},
			},
			wantReady: true,
			wantErr:   false,
		},
		{
			name:       "nil object",
			obj:        nil,
			predicates: []graph.ReadinessPredicate{},
			wantReady:  false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client and add the object if it exists
			builder := fake.NewClientBuilder()
			if tt.obj != nil {
				builder = builder.WithObjects(tt.obj)
			}
			c := builder.Build()

			checker := NewChecker(c)

			ready, err := checker.Check(context.Background(), tt.obj, tt.predicates)
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ready != tt.wantReady {
				t.Errorf("Check() ready = %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

func TestChecker_ConditionMatch(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "test-pod",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithObjects(obj).Build()
	checker := NewChecker(c)

	predicates := []graph.ReadinessPredicate{
		{
			Type:            graph.PredicateTypeConditionMatch,
			ConditionType:   "Ready",
			ConditionStatus: "True",
		},
	}

	ready, err := checker.Check(context.Background(), obj, predicates)
	if err != nil {
		t.Fatalf("Check() failed: %v", err)
	}
	if !ready {
		t.Error("Check() should return ready=true for matching condition")
	}
}
