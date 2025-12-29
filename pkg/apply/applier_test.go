package apply

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chazu/pequod/pkg/graph"
)

func TestApplier_ApplySSA(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		policy  graph.ApplyPolicy
		wantErr bool
	}{
		{
			name: "apply with default policy",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			policy: graph.ApplyPolicy{
				Mode:           graph.ApplyModeApply,
				ConflictPolicy: graph.ConflictPolicyError,
				FieldManager:   "test-manager",
			},
			wantErr: false,
		},
		{
			name: "apply with force conflict policy",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm-force",
						"namespace": "default",
					},
				},
			},
			policy: graph.ApplyPolicy{
				Mode:           graph.ApplyModeApply,
				ConflictPolicy: graph.ConflictPolicyForce,
				FieldManager:   "test-manager",
			},
			wantErr: false,
		},
		{
			name:    "nil object",
			obj:     nil,
			policy:  graph.ApplyPolicy{Mode: graph.ApplyModeApply},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().Build()
			applier := NewApplier(c)

			err := applier.Apply(context.Background(), tt.obj, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}

			// If successful, verify the object was created
			if !tt.wantErr && tt.obj != nil {
				key := client.ObjectKeyFromObject(tt.obj)
				got := &unstructured.Unstructured{}
				got.SetGroupVersionKind(tt.obj.GroupVersionKind())

				if err := c.Get(context.Background(), key, got); err != nil {
					t.Errorf("Failed to get applied object: %v", err)
				}
			}
		})
	}
}

func TestApplier_ApplyCreate(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	applier := NewApplier(c)

	policy := graph.ApplyPolicy{
		Mode:         graph.ApplyModeCreate,
		FieldManager: "test-manager",
	}

	// First apply should succeed
	obj1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-create",
				"namespace": "default",
			},
		},
	}
	err := applier.Apply(context.Background(), obj1, policy)
	if err != nil {
		t.Fatalf("First Apply() failed: %v", err)
	}

	// Second apply with same name should also succeed (idempotent - already exists)
	obj2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-create",
				"namespace": "default",
			},
		},
	}
	err = applier.Apply(context.Background(), obj2, policy)
	if err != nil {
		t.Errorf("Second Apply() failed: %v", err)
	}
}

func TestApplier_ApplyAdopt(t *testing.T) {
	// Create an existing resource
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-adopt",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"existing": "value",
			},
		},
	}

	c := fake.NewClientBuilder().WithObjects(existing).Build()
	applier := NewApplier(c)

	// Adopt the existing resource
	adoptObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-adopt",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"adopted": "value",
			},
		},
	}

	policy := graph.ApplyPolicy{
		Mode:         graph.ApplyModeAdopt,
		FieldManager: "test-manager",
	}

	err := applier.Apply(context.Background(), adoptObj, policy)
	if err != nil {
		t.Fatalf("Apply() with Adopt mode failed: %v", err)
	}

	// Verify the resource was adopted
	key := client.ObjectKeyFromObject(adoptObj)
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(adoptObj.GroupVersionKind())

	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Failed to get adopted object: %v", err)
	}
}

func TestApplier_DryRun(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-dryrun",
				"namespace": "default",
			},
		},
	}

	c := fake.NewClientBuilder().Build()
	applier := NewApplier(c).WithDryRun(true)

	policy := graph.ApplyPolicy{
		Mode:         graph.ApplyModeApply,
		FieldManager: "test-manager",
	}

	// Apply with dry-run should not create the resource
	err := applier.Apply(context.Background(), obj, policy)
	if err != nil {
		t.Fatalf("Apply() with dry-run failed: %v", err)
	}

	// Verify the resource was NOT created (dry-run)
	key := client.ObjectKeyFromObject(obj)
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(obj.GroupVersionKind())

	err = c.Get(context.Background(), key, got)
	if err == nil {
		t.Error("Resource should not exist after dry-run apply")
	}
}

func TestApplier_PolicyValidation(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}

	c := fake.NewClientBuilder().Build()
	applier := NewApplier(c)

	tests := []struct {
		name    string
		policy  graph.ApplyPolicy
		wantErr bool
	}{
		{
			name: "valid policy with defaults",
			policy: graph.ApplyPolicy{
				Mode: graph.ApplyModeApply,
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			policy: graph.ApplyPolicy{
				Mode: graph.ApplyMode("Invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid conflict policy",
			policy: graph.ApplyPolicy{
				Mode:           graph.ApplyModeApply,
				ConflictPolicy: graph.ConflictPolicy("Invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := applier.Apply(context.Background(), obj, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplier_AdoptNonExistent(t *testing.T) {
	// Test adopting a resource that doesn't exist - should create it
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm-adopt-new",
				"namespace": "default",
			},
		},
	}

	c := fake.NewClientBuilder().Build()
	applier := NewApplier(c)

	policy := graph.ApplyPolicy{
		Mode:         graph.ApplyModeAdopt,
		FieldManager: "test-manager",
	}

	err := applier.Apply(context.Background(), obj, policy)
	if err != nil {
		t.Fatalf("Apply() with Adopt mode for non-existent resource failed: %v", err)
	}

	// Verify the resource was created
	key := client.ObjectKeyFromObject(obj)
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(obj.GroupVersionKind())

	if err := c.Get(context.Background(), key, got); err != nil {
		t.Errorf("Failed to get created object: %v", err)
	}
}

func TestConflictError(t *testing.T) {
	err := &ConflictError{
		Resource:     "default/test-cm",
		FieldManager: "test-manager",
		Err:          fmt.Errorf("field conflict"),
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("ConflictError.Error() returned empty string")
	}

	if err.Unwrap() == nil {
		t.Error("ConflictError.Unwrap() returned nil")
	}
}
