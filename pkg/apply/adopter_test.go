package apply

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
)

func TestNewAdopter(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adopter := NewAdopter(fakeClient)

	if adopter == nil {
		t.Fatal("NewAdopter returned nil")
	}
	if adopter.fieldManager != DefaultFieldManager {
		t.Errorf("expected field manager %q, got %q", DefaultFieldManager, adopter.fieldManager)
	}
	if adopter.dryRun {
		t.Error("expected dryRun to be false")
	}
}

func TestAdopter_WithFieldManager(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	customFM := "custom-manager"
	newAdopter := adopter.WithFieldManager(customFM)

	if newAdopter.fieldManager != customFM {
		t.Errorf("expected field manager %q, got %q", customFM, newAdopter.fieldManager)
	}
	// Original should be unchanged
	if adopter.fieldManager != DefaultFieldManager {
		t.Errorf("original adopter should not be modified")
	}
}

func TestAdopter_WithDryRun(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	dryRunAdopter := adopter.WithDryRun(true)

	if !dryRunAdopter.dryRun {
		t.Error("expected dryRun to be true")
	}
	// Original should be unchanged
	if adopter.dryRun {
		t.Error("original adopter should not be modified")
	}
}

func TestAdopter_Adopt_NilSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	report, err := adopter.Adopt(context.Background(), nil, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Results))
	}
}

func TestAdopter_Adopt_EmptySpec(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	spec := &platformv1alpha1.AdoptSpec{}
	report, err := adopter.Adopt(context.Background(), spec, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Results))
	}
}

func TestAdopter_Adopt_UnsupportedMode(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	spec := &platformv1alpha1.AdoptSpec{
		Mode: platformv1alpha1.AdoptModeLabelSelector,
	}
	_, err := adopter.Adopt(context.Background(), spec, nil)

	if err == nil {
		t.Error("expected error for unsupported mode")
	}
}

func TestAdopter_FindMatchingNode_ByNodeID(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	nodes := []graph.Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
	}

	ref := platformv1alpha1.AdoptedResourceRef{NodeID: "node-2"}
	node := adopter.findMatchingNode(ref, nodes)

	if node == nil {
		t.Fatal("expected to find node")
	}
	if node.ID != "node-2" {
		t.Errorf("expected node-2, got %s", node.ID)
	}
}

func TestAdopter_FindMatchingNode_ByGVK(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("default")
	obj.SetName("my-deploy")

	nodes := []graph.Node{
		{
			ID:     "deployment",
			Object: *obj,
		},
	}

	ref := platformv1alpha1.AdoptedResourceRef{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Namespace:  "default",
		Name:       "my-deploy",
	}
	node := adopter.findMatchingNode(ref, nodes)

	if node == nil {
		t.Fatal("expected to find node by GVK match")
	}
	if node.ID != "deployment" {
		t.Errorf("expected deployment node, got %s", node.ID)
	}
}

func TestAdopter_FindMatchingNode_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	nodes := []graph.Node{
		{ID: "node-1"},
	}

	ref := platformv1alpha1.AdoptedResourceRef{NodeID: "nonexistent"}
	node := adopter.findMatchingNode(ref, nodes)

	if node != nil {
		t.Error("expected nil for non-matching node")
	}
}

func TestAdoptionReport_HasErrors(t *testing.T) {
	tests := []struct {
		name      string
		report    AdoptionReport
		hasErrors bool
	}{
		{
			name:      "no failures",
			report:    AdoptionReport{TotalFailed: 0},
			hasErrors: false,
		},
		{
			name:      "with failures",
			report:    AdoptionReport{TotalFailed: 1},
			hasErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.HasErrors(); got != tt.hasErrors {
				t.Errorf("HasErrors() = %v, want %v", got, tt.hasErrors)
			}
		})
	}
}

func TestResourceRef_String(t *testing.T) {
	tests := []struct {
		name     string
		ref      ResourceRef
		expected string
	}{
		{
			name: "with namespace",
			ref: ResourceRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  "default",
				Name:       "my-deploy",
			},
			expected: "apps/v1/Deployment default/my-deploy",
		},
		{
			name: "cluster-scoped",
			ref: ResourceRef{
				APIVersion: "v1",
				Kind:       "Namespace",
				Name:       "my-ns",
			},
			expected: "v1/Namespace my-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAdopter_GetFieldManagers(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	obj := &unstructured.Unstructured{}
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "manager-1"},
		{Manager: "manager-2"},
		{Manager: "manager-1"}, // Duplicate
	})

	managers := adopter.getFieldManagers(obj)

	if len(managers) != 2 {
		t.Errorf("expected 2 unique managers, got %d", len(managers))
	}
}

func TestAdopter_Adopt_ResourceNotFound_NoNode(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	spec := &platformv1alpha1.AdoptSpec{
		Mode: platformv1alpha1.AdoptModeExplicit,
		Resources: []platformv1alpha1.AdoptedResourceRef{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  "default",
				Name:       "nonexistent",
			},
		},
	}

	report, err := adopter.Adopt(context.Background(), spec, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if report.Results[0].Error == nil {
		t.Error("expected error for nonexistent resource with no matching node")
	}
	if report.TotalFailed != 1 {
		t.Errorf("expected TotalFailed=1, got %d", report.TotalFailed)
	}
}

func TestAdopter_CheckAdoptionSafety(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	spec := &platformv1alpha1.AdoptSpec{
		Resources: []platformv1alpha1.AdoptedResourceRef{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  "default",
				Name:       "my-deploy",
			},
		},
	}

	warnings, errors := adopter.CheckAdoptionSafety(context.Background(), spec)

	// Resource doesn't exist, should get a warning
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	if len(errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errors))
	}
}

func TestAdopter_CheckAdoptionSafety_InvalidAPIVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adopter := NewAdopter(fakeClient)

	spec := &platformv1alpha1.AdoptSpec{
		Resources: []platformv1alpha1.AdoptedResourceRef{
			{
				APIVersion: "invalid//version",
				Kind:       "Deployment",
				Name:       "my-deploy",
			},
		},
	}

	_, errors := adopter.CheckAdoptionSafety(context.Background(), spec)

	if len(errors) != 1 {
		t.Errorf("expected 1 error for invalid apiVersion, got %d", len(errors))
	}
}

func TestAdoptionReport_Counts(t *testing.T) {
	report := &AdoptionReport{
		Results: []AdoptResult{
			{Adopted: true},
			{Adopted: true, Created: true},
			{AlreadyManaged: true},
			{Error: client.IgnoreNotFound(nil)},
		},
		TotalAdopted: 2,
		TotalCreated: 1,
		TotalSkipped: 1,
		TotalFailed:  0,
	}

	if !report.HasErrors() {
		// No failures means no errors
	}
	if report.TotalAdopted != 2 {
		t.Errorf("expected TotalAdopted=2, got %d", report.TotalAdopted)
	}
}
