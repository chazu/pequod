package platformloader

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/chazu/pequod/pkg/graph"
)

func TestNewPolicyValidator(t *testing.T) {
	loader := NewLoader()
	validator := NewPolicyValidator(loader)

	if validator == nil {
		t.Fatal("expected non-nil validator")
	}

	if validator.loader == nil {
		t.Error("expected non-nil loader")
	}
}

func TestValidateInput(t *testing.T) {
	loader := NewLoader()
	validator := NewPolicyValidator(loader)
	ctx := context.Background()

	tests := []struct {
		name      string
		input     map[string]interface{}
		wantError bool
		wantWarn  bool
	}{
		{
			name: "valid input",
			input: map[string]interface{}{
				"image":    "nginx:latest",
				"port":     8080,
				"replicas": 3,
			},
			wantError: false,
			wantWarn:  false,
		},
		{
			name: "empty image",
			input: map[string]interface{}{
				"image":    "",
				"port":     8080,
				"replicas": 3,
			},
			wantError: true,
			wantWarn:  false,
		},
		{
			name: "invalid port - too low",
			input: map[string]interface{}{
				"image":    "nginx:latest",
				"port":     0,
				"replicas": 3,
			},
			wantError: true,
			wantWarn:  false,
		},
		{
			name: "invalid port - too high",
			input: map[string]interface{}{
				"image":    "nginx:latest",
				"port":     70000,
				"replicas": 3,
			},
			wantError: true,
			wantWarn:  false,
		},
		{
			name: "negative replicas",
			input: map[string]interface{}{
				"image":    "nginx:latest",
				"port":     8080,
				"replicas": -1,
			},
			wantError: true,
			wantWarn:  false,
		},
		{
			name: "high replicas warning",
			input: map[string]interface{}{
				"image":    "nginx:latest",
				"port":     8080,
				"replicas": 15,
			},
			wantError: false,
			wantWarn:  true,
		},
		{
			name: "nil replicas",
			input: map[string]interface{}{
				"image": "nginx:latest",
				"port":  8080,
			},
			wantError: false,
			wantWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := validator.ValidateInput(ctx, tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasError := false
			hasWarning := false
			for _, v := range violations {
				if v.Severity == "Error" {
					hasError = true
				}
				if v.Severity == "Warning" {
					hasWarning = true
				}
			}

			if hasError != tt.wantError {
				t.Errorf("expected error=%v, got error=%v, violations: %+v", tt.wantError, hasError, violations)
			}

			if hasWarning != tt.wantWarn {
				t.Errorf("expected warning=%v, got warning=%v, violations: %+v", tt.wantWarn, hasWarning, violations)
			}
		})
	}
}

func TestValidateOutput(t *testing.T) {
	loader := NewLoader()
	validator := NewPolicyValidator(loader)
	ctx := context.Background()

	// Create a valid graph
	validGraph := &graph.Graph{
		Metadata: graph.GraphMetadata{
			Name:    "test-graph",
			Version: "v1alpha1",
		},
		Nodes: []graph.Node{
			{
				ID: "deployment",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
							"labels": map[string]interface{}{
								"app.kubernetes.io/managed-by": "pequod",
							},
						},
					},
				},
				ApplyPolicy: graph.ApplyPolicy{
					Mode: graph.ApplyModeApply,
				},
			},
			{
				ID: "service",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
							"labels": map[string]interface{}{
								"app.kubernetes.io/managed-by": "pequod",
							},
						},
					},
				},
				ApplyPolicy: graph.ApplyPolicy{
					Mode: graph.ApplyModeApply,
				},
			},
		},
	}

	violations, err := validator.ValidateOutput(ctx, validGraph)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no errors, might have warnings
	for _, v := range violations {
		if v.Severity == "Error" {
			t.Errorf("unexpected error violation: %+v", v)
		}
	}
}
