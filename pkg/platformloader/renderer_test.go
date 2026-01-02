package platformloader

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewRenderer(t *testing.T) {
	loader := NewLoader()
	renderer := NewRenderer(loader)

	if renderer == nil {
		t.Fatal("expected non-nil renderer")
	}

	if renderer.loader == nil {
		t.Error("expected non-nil loader")
	}
}

func TestRenderTransform(t *testing.T) {
	loader := NewLoader()
	renderer := NewRenderer(loader)
	ctx := context.Background()

	// Build input as raw JSON (matching Transform input format)
	input := map[string]interface{}{
		"image":    "nginx:latest",
		"port":     8080,
		"replicas": 3,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	g, err := renderer.RenderTransform(ctx, "test-app", "default", runtime.RawExtension{Raw: inputJSON}, "webservice")
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if g == nil {
		t.Fatal("expected non-nil graph")
	}

	// Verify metadata
	if g.Metadata.Name != "test-app-graph" {
		t.Errorf("expected metadata.name 'test-app-graph', got '%s'", g.Metadata.Name)
	}

	if g.Metadata.Version != "v1alpha1" {
		t.Errorf("expected metadata.version 'v1alpha1', got '%s'", g.Metadata.Version)
	}

	if g.Metadata.PlatformRef != "webservice" {
		t.Errorf("expected metadata.platformRef 'webservice', got '%s'", g.Metadata.PlatformRef)
	}

	// Verify nodes
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}

	// Check deployment node
	deploymentNode := g.Nodes[0]
	if deploymentNode.ID != "deployment" {
		t.Errorf("expected first node ID 'deployment', got '%s'", deploymentNode.ID)
	}

	if deploymentNode.Object.GetKind() != "Deployment" {
		t.Errorf("expected deployment kind, got '%s'", deploymentNode.Object.GetKind())
	}

	if deploymentNode.Object.GetName() != "test-app" {
		t.Errorf("expected deployment name 'test-app', got '%s'", deploymentNode.Object.GetName())
	}

	if deploymentNode.Object.GetNamespace() != "default" {
		t.Errorf("expected deployment namespace 'default', got '%s'", deploymentNode.Object.GetNamespace())
	}

	// Check replicas in deployment spec
	replicasVal, found, err := unstructured.NestedFieldNoCopy(deploymentNode.Object.Object, "spec", "replicas")
	if err != nil || !found {
		t.Fatalf("failed to get replicas: %v", err)
	}

	// JSON unmarshaling converts numbers to float64
	replicasFloat, ok := replicasVal.(float64)
	if !ok {
		t.Fatalf("replicas is not a number, got %T", replicasVal)
	}

	if int(replicasFloat) != 3 {
		t.Errorf("expected replicas 3, got %v", replicasFloat)
	}

	// Check service node
	serviceNode := g.Nodes[1]
	if serviceNode.ID != "service" {
		t.Errorf("expected second node ID 'service', got '%s'", serviceNode.ID)
	}

	if serviceNode.Object.GetKind() != "Service" {
		t.Errorf("expected service kind, got '%s'", serviceNode.Object.GetKind())
	}

	// Check dependencies
	if len(serviceNode.DependsOn) != 1 || serviceNode.DependsOn[0] != "deployment" {
		t.Errorf("expected service to depend on deployment, got %v", serviceNode.DependsOn)
	}

	// Verify no violations
	if len(g.Violations) != 0 {
		t.Errorf("expected no violations, got %d", len(g.Violations))
	}
}

func TestRenderTransformWithDefaultReplicas(t *testing.T) {
	loader := NewLoader()
	renderer := NewRenderer(loader)
	ctx := context.Background()

	// Build input without replicas (should default to 1)
	input := map[string]interface{}{
		"image": "nginx:latest",
		"port":  8080,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	g, err := renderer.RenderTransform(ctx, "test-app", "default", runtime.RawExtension{Raw: inputJSON}, "webservice")
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	// Check replicas defaults to 1
	deploymentNode := g.Nodes[0]
	replicasVal, found, err := unstructured.NestedFieldNoCopy(deploymentNode.Object.Object, "spec", "replicas")
	if err != nil || !found {
		t.Fatalf("failed to get replicas: %v", err)
	}

	// JSON unmarshaling converts numbers to float64
	replicasFloat, ok := replicasVal.(float64)
	if !ok {
		t.Fatalf("replicas is not a number, got %T", replicasVal)
	}

	if int(replicasFloat) != 1 {
		t.Errorf("expected default replicas 1, got %v", replicasFloat)
	}
}

func TestRenderTransformWithEnvFrom(t *testing.T) {
	// For now, skip this test - we'll implement envFrom support in the renderer later
	// The CUE template is ready, but we need to update the input format
	// to accept and pass envFrom to the CUE template
	t.Skip("EnvFrom support in renderer not yet implemented - CUE template is ready")
}
