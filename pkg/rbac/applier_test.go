/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rbac

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = rbacv1.AddToScheme(scheme)
	_ = platformv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestNewApplier(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	if a == nil {
		t.Fatal("expected non-nil applier")
	}
}

func TestApplyClusterRole_Create(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	err := a.ApplyClusterRole(ctx, role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was created
	got, err := a.GetClusterRole(ctx, "test-role")
	if err != nil {
		t.Fatalf("failed to get ClusterRole: %v", err)
	}
	if len(got.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(got.Rules))
	}
}

func TestApplyClusterRole_Update(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get"},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	// Update with new rules
	updated := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "statefulsets"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	err := a.ApplyClusterRole(ctx, updated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was updated
	got, err := a.GetClusterRole(ctx, "test-role")
	if err != nil {
		t.Fatalf("failed to get ClusterRole: %v", err)
	}
	if len(got.Rules[0].Resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(got.Rules[0].Resources))
	}
}

func TestDeleteClusterRole(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-role",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteClusterRole(ctx, "test-role")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was deleted
	_, err = a.GetClusterRole(ctx, "test-role")
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestDeleteClusterRole_NotFound(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	// Should not error when deleting non-existent role
	err := a.DeleteClusterRole(ctx, "nonexistent")
	if err != nil {
		t.Errorf("expected no error when deleting non-existent role, got %v", err)
	}
}

func TestApplyRole_Create(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	err := a.ApplyRole(ctx, role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was created
	got, err := a.GetRole(ctx, "test-role", "default")
	if err != nil {
		t.Fatalf("failed to get Role: %v", err)
	}
	if len(got.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(got.Rules))
	}
}

func TestApplyRole_Update(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get"},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	// Update with new rules
	updated := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "statefulsets"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	err := a.ApplyRole(ctx, updated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was updated
	got, err := a.GetRole(ctx, "test-role", "default")
	if err != nil {
		t.Fatalf("failed to get Role: %v", err)
	}
	if len(got.Rules[0].Resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(got.Rules[0].Resources))
	}
}

func TestDeleteRole(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteRole(ctx, "test-role", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was deleted
	_, err = a.GetRole(ctx, "test-role", "default")
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestDeleteRole_NotFound(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteRole(ctx, "nonexistent", "default")
	if err != nil {
		t.Errorf("expected no error when deleting non-existent role, got %v", err)
	}
}

func TestApplyRoleBinding_Create(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "test-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-sa",
				Namespace: "default",
			},
		},
	}

	err := a.ApplyRoleBinding(ctx, binding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was created
	got := &rbacv1.RoleBinding{}
	err = c.Get(ctx, types.NamespacedName{Name: "test-binding", Namespace: "default"}, got)
	if err != nil {
		t.Fatalf("failed to get RoleBinding: %v", err)
	}
	if got.RoleRef.Name != "test-role" {
		t.Errorf("expected roleRef.name 'test-role', got %q", got.RoleRef.Name)
	}
}

func TestDeleteRoleBinding(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "test-role",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteRoleBinding(ctx, "test-binding", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was deleted
	got := &rbacv1.RoleBinding{}
	err = c.Get(ctx, types.NamespacedName{Name: "test-binding", Namespace: "default"}, got)
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestDeleteRoleBinding_NotFound(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteRoleBinding(ctx, "nonexistent", "default")
	if err != nil {
		t.Errorf("expected no error when deleting non-existent binding, got %v", err)
	}
}

func TestApplyGeneratedRBAC_Nil(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.ApplyGeneratedRBAC(ctx, nil)
	if err != nil {
		t.Errorf("expected no error for nil input, got %v", err)
	}
}

func TestApplyGeneratedRBAC_ClusterRole(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	rbac := &GeneratedRBAC{
		ClusterRole: &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-role",
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     StandardVerbs,
				},
			},
		},
	}

	err := a.ApplyGeneratedRBAC(ctx, rbac)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ClusterRole was created
	got, err := a.GetClusterRole(ctx, "test-cluster-role")
	if err != nil {
		t.Fatalf("failed to get ClusterRole: %v", err)
	}
	if len(got.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(got.Rules))
	}
}

func TestApplyGeneratedRBAC_Namespace(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	a := NewApplier(c)
	ctx := context.Background()

	rbac := &GeneratedRBAC{
		Role: &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-role",
				Namespace: "default",
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     StandardVerbs,
				},
			},
		},
		RoleBinding: &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-role",
				Namespace: "default",
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     "test-role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      "controller-manager",
					Namespace: "pequod-system",
				},
			},
		},
	}

	err := a.ApplyGeneratedRBAC(ctx, rbac)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Role was created
	gotRole, err := a.GetRole(ctx, "test-role", "default")
	if err != nil {
		t.Fatalf("failed to get Role: %v", err)
	}
	if len(gotRole.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(gotRole.Rules))
	}

	// Verify RoleBinding was created
	gotBinding := &rbacv1.RoleBinding{}
	err = c.Get(ctx, types.NamespacedName{Name: "test-role", Namespace: "default"}, gotBinding)
	if err != nil {
		t.Fatalf("failed to get RoleBinding: %v", err)
	}
}

func TestDeleteGeneratedRBAC_ClusterScope(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pequod:transform:default.test",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteGeneratedRBAC(ctx, "test", "default", platformv1alpha1.RBACScopeCluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was deleted
	_, err = a.GetClusterRole(ctx, "pequod:transform:default.test")
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestDeleteGeneratedRBAC_NamespaceScope(t *testing.T) {
	scheme := newTestScheme()

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pequod:transform:default.test",
			Namespace: "default",
		},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pequod:transform:default.test",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "pequod:transform:default.test",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role, binding).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	err := a.DeleteGeneratedRBAC(ctx, "test", "default", platformv1alpha1.RBACScopeNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Role was deleted
	_, err = a.GetRole(ctx, "pequod:transform:default.test", "default")
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error for Role, got %v", err)
	}

	// Verify RoleBinding was deleted
	gotBinding := &rbacv1.RoleBinding{}
	err = c.Get(ctx, types.NamespacedName{Name: "pequod:transform:default.test", Namespace: "default"}, gotBinding)
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error for RoleBinding, got %v", err)
	}
}

func TestDeleteGeneratedRBAC_DefaultScope(t *testing.T) {
	scheme := newTestScheme()

	existing := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pequod:transform:default.test",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	a := NewApplier(c)
	ctx := context.Background()

	// Empty scope should default to Cluster
	err := a.DeleteGeneratedRBAC(ctx, "test", "default", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ClusterRole was deleted
	_, err = a.GetClusterRole(ctx, "pequod:transform:default.test")
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}
