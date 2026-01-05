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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator()
	if g == nil {
		t.Fatal("expected non-nil generator")
	}
}

func TestGenerate_EmptyManagedResources(t *testing.T) {
	g := NewGenerator()
	tf := &platformv1alpha1.Transform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-transform",
			Namespace: "default",
		},
		Spec: platformv1alpha1.TransformSpec{
			// No ManagedResources
		},
	}

	result := g.Generate(tf, "controller-manager", "pequod-system")
	if result != nil {
		t.Error("expected nil result for empty ManagedResources")
	}
}

func TestGenerate_ClusterScope(t *testing.T) {
	g := NewGenerator()
	tf := &platformv1alpha1.Transform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webservice",
			Namespace: "default",
		},
		Spec: platformv1alpha1.TransformSpec{
			RBACScope: platformv1alpha1.RBACScopeCluster,
			ManagedResources: []platformv1alpha1.ManagedResource{
				{
					APIGroup:  "apps",
					Resources: []string{"deployments"},
				},
				{
					APIGroup:  "",
					Resources: []string{"services", "configmaps"},
				},
			},
		},
	}

	result := g.Generate(tf, "controller-manager", "pequod-system")
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have ClusterRole, not Role
	if result.ClusterRole == nil {
		t.Fatal("expected ClusterRole for Cluster scope")
	}
	if result.Role != nil {
		t.Error("expected no Role for Cluster scope")
	}
	if result.RoleBinding != nil {
		t.Error("expected no RoleBinding for Cluster scope")
	}

	// Verify ClusterRole structure
	cr := result.ClusterRole
	if cr.Name != "pequod:transform:default.webservice" {
		t.Errorf("expected name 'pequod:transform:default.webservice', got %q", cr.Name)
	}

	// Verify labels
	if cr.Labels[AggregationLabel] != "true" {
		t.Errorf("expected aggregation label to be 'true', got %q", cr.Labels[AggregationLabel])
	}
	if cr.Labels[TransformLabel] != "webservice" {
		t.Errorf("expected transform label to be 'webservice', got %q", cr.Labels[TransformLabel])
	}
	if cr.Labels[TransformNamespaceLabel] != "default" {
		t.Errorf("expected transform namespace label to be 'default', got %q", cr.Labels[TransformNamespaceLabel])
	}

	// Verify rules
	if len(cr.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cr.Rules))
	}

	// First rule: apps/deployments
	if len(cr.Rules[0].APIGroups) != 1 || cr.Rules[0].APIGroups[0] != "apps" {
		t.Errorf("expected first rule APIGroup 'apps', got %v", cr.Rules[0].APIGroups)
	}
	if len(cr.Rules[0].Resources) != 1 || cr.Rules[0].Resources[0] != "deployments" {
		t.Errorf("expected first rule resources [deployments], got %v", cr.Rules[0].Resources)
	}

	// Second rule: core/services,configmaps
	if len(cr.Rules[1].APIGroups) != 1 || cr.Rules[1].APIGroups[0] != "" {
		t.Errorf("expected second rule APIGroup '', got %v", cr.Rules[1].APIGroups)
	}
	if len(cr.Rules[1].Resources) != 2 {
		t.Errorf("expected second rule to have 2 resources, got %v", cr.Rules[1].Resources)
	}

	// Verify verbs
	expectedVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	for _, rule := range cr.Rules {
		if len(rule.Verbs) != len(expectedVerbs) {
			t.Errorf("expected %d verbs, got %d", len(expectedVerbs), len(rule.Verbs))
		}
	}
}

func TestGenerate_NamespaceScope(t *testing.T) {
	g := NewGenerator()
	tf := &platformv1alpha1.Transform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp",
			Namespace: "team-a",
		},
		Spec: platformv1alpha1.TransformSpec{
			RBACScope: platformv1alpha1.RBACScopeNamespace,
			ManagedResources: []platformv1alpha1.ManagedResource{
				{
					APIGroup:  "apps",
					Resources: []string{"deployments"},
				},
			},
		},
	}

	result := g.Generate(tf, "controller-manager", "pequod-system")
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have Role and RoleBinding, not ClusterRole
	if result.ClusterRole != nil {
		t.Error("expected no ClusterRole for Namespace scope")
	}
	if result.Role == nil {
		t.Fatal("expected Role for Namespace scope")
	}
	if result.RoleBinding == nil {
		t.Fatal("expected RoleBinding for Namespace scope")
	}

	// Verify Role structure
	role := result.Role
	expectedName := "pequod:transform:team-a.myapp"
	if role.Name != expectedName {
		t.Errorf("expected Role name %q, got %q", expectedName, role.Name)
	}
	if role.Namespace != "team-a" {
		t.Errorf("expected Role namespace 'team-a', got %q", role.Namespace)
	}

	// Verify Role labels (no aggregation label for namespace scope)
	if _, exists := role.Labels[AggregationLabel]; exists {
		t.Error("expected no aggregation label for namespace-scoped Role")
	}
	if role.Labels[TransformLabel] != "myapp" {
		t.Errorf("expected transform label 'myapp', got %q", role.Labels[TransformLabel])
	}

	// Verify RoleBinding structure
	rb := result.RoleBinding
	if rb.Name != expectedName {
		t.Errorf("expected RoleBinding name %q, got %q", expectedName, rb.Name)
	}
	if rb.Namespace != "team-a" {
		t.Errorf("expected RoleBinding namespace 'team-a', got %q", rb.Namespace)
	}
	if rb.RoleRef.Kind != "Role" {
		t.Errorf("expected RoleRef kind 'Role', got %q", rb.RoleRef.Kind)
	}
	if rb.RoleRef.Name != expectedName {
		t.Errorf("expected RoleRef name %q, got %q", expectedName, rb.RoleRef.Name)
	}

	// Verify subjects
	if len(rb.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(rb.Subjects))
	}
	if rb.Subjects[0].Kind != "ServiceAccount" {
		t.Errorf("expected subject kind 'ServiceAccount', got %q", rb.Subjects[0].Kind)
	}
	if rb.Subjects[0].Name != "controller-manager" {
		t.Errorf("expected subject name 'controller-manager', got %q", rb.Subjects[0].Name)
	}
	if rb.Subjects[0].Namespace != "pequod-system" {
		t.Errorf("expected subject namespace 'pequod-system', got %q", rb.Subjects[0].Namespace)
	}
}

func TestGenerate_DefaultScope(t *testing.T) {
	g := NewGenerator()
	tf := &platformv1alpha1.Transform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: platformv1alpha1.TransformSpec{
			// RBACScope not set, should default to Cluster
			ManagedResources: []platformv1alpha1.ManagedResource{
				{
					APIGroup:  "apps",
					Resources: []string{"deployments"},
				},
			},
		},
	}

	result := g.Generate(tf, "controller-manager", "pequod-system")
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should default to ClusterRole
	if result.ClusterRole == nil {
		t.Error("expected ClusterRole when scope is not specified (defaults to Cluster)")
	}
	if result.Role != nil {
		t.Error("expected no Role when scope defaults to Cluster")
	}
}

func TestRoleName(t *testing.T) {
	g := NewGenerator()

	tests := []struct {
		name      string
		namespace string
		expected  string
	}{
		{"webservice", "default", "pequod:transform:default.webservice"},
		{"MyApp", "TeamA", "pequod:transform:teama.myapp"}, // Should be lowercased
		{"test-app", "prod-ns", "pequod:transform:prod-ns.test-app"},
	}

	for _, tc := range tests {
		result := g.RoleName(tc.name, tc.namespace)
		if result != tc.expected {
			t.Errorf("RoleName(%q, %q) = %q, expected %q", tc.name, tc.namespace, result, tc.expected)
		}
	}
}

func TestBuildPolicyRules(t *testing.T) {
	g := NewGenerator()

	resources := []platformv1alpha1.ManagedResource{
		{
			APIGroup:  "apps",
			Resources: []string{"deployments", "statefulsets"},
		},
		{
			APIGroup:  "",
			Resources: []string{"services"},
		},
		{
			APIGroup:  "batch",
			Resources: []string{"jobs", "cronjobs"},
		},
	}

	rules := g.buildPolicyRules(resources)

	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// Verify first rule
	if rules[0].APIGroups[0] != "apps" {
		t.Errorf("expected first rule APIGroup 'apps', got %q", rules[0].APIGroups[0])
	}
	if len(rules[0].Resources) != 2 {
		t.Errorf("expected first rule to have 2 resources, got %d", len(rules[0].Resources))
	}

	// Verify all rules have standard verbs
	for i, rule := range rules {
		if len(rule.Verbs) != 7 {
			t.Errorf("rule %d: expected 7 verbs, got %d", i, len(rule.Verbs))
		}
	}
}

func TestToGeneratedRBACReference(t *testing.T) {
	g := NewGenerator()

	// Test nil input
	ref := g.ToGeneratedRBACReference(nil)
	if ref != nil {
		t.Error("expected nil reference for nil input")
	}

	// Test ClusterRole
	tf := &platformv1alpha1.Transform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: platformv1alpha1.TransformSpec{
			RBACScope: platformv1alpha1.RBACScopeCluster,
			ManagedResources: []platformv1alpha1.ManagedResource{
				{APIGroup: "apps", Resources: []string{"deployments"}},
				{APIGroup: "", Resources: []string{"services"}},
			},
		},
	}

	rbac := g.Generate(tf, "sa", "ns")
	ref = g.ToGeneratedRBACReference(rbac)

	if ref == nil {
		t.Fatal("expected non-nil reference")
	}
	if ref.ClusterRoleName == "" {
		t.Error("expected ClusterRoleName to be set")
	}
	if ref.RuleCount != 2 {
		t.Errorf("expected RuleCount 2, got %d", ref.RuleCount)
	}
	if ref.RoleName != "" {
		t.Error("expected RoleName to be empty for cluster scope")
	}

	// Test Namespace scope
	tf.Spec.RBACScope = platformv1alpha1.RBACScopeNamespace
	rbac = g.Generate(tf, "sa", "ns")
	ref = g.ToGeneratedRBACReference(rbac)

	if ref.RoleName == "" {
		t.Error("expected RoleName to be set for namespace scope")
	}
	if ref.RoleBindingName == "" {
		t.Error("expected RoleBindingName to be set for namespace scope")
	}
	if ref.ClusterRoleName != "" {
		t.Error("expected ClusterRoleName to be empty for namespace scope")
	}
}

func TestRuleCount(t *testing.T) {
	g := NewGenerator()

	tests := []struct {
		resources []platformv1alpha1.ManagedResource
		expected  int
	}{
		{nil, 0},
		{[]platformv1alpha1.ManagedResource{}, 0},
		{[]platformv1alpha1.ManagedResource{
			{APIGroup: "apps", Resources: []string{"deployments"}},
		}, 1},
		{[]platformv1alpha1.ManagedResource{
			{APIGroup: "apps", Resources: []string{"deployments"}},
			{APIGroup: "", Resources: []string{"services", "configmaps"}},
		}, 2},
	}

	for _, tc := range tests {
		count := g.RuleCount(tc.resources)
		if count != tc.expected {
			t.Errorf("RuleCount(%v) = %d, expected %d", tc.resources, count, tc.expected)
		}
	}
}
