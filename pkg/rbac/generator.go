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
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

const (
	// AggregationLabel is the label used to aggregate ClusterRoles into the manager role
	AggregationLabel = "pequod.io/aggregate-to-manager"

	// TransformLabel identifies which Transform owns a role
	TransformLabel = "platform.pequod.io/transform"

	// TransformNamespaceLabel identifies the namespace of the Transform
	TransformNamespaceLabel = "platform.pequod.io/transform-namespace"

	// RolePrefix is the prefix for generated role names
	RolePrefix = "pequod:transform:"
)

// StandardVerbs are the verbs granted for managed resources
var StandardVerbs = []string{"get", "list", "watch", "create", "update", "patch", "delete"}

// Generator creates RBAC resources from Transform specifications
type Generator struct{}

// NewGenerator creates a new RBAC generator
func NewGenerator() *Generator {
	return &Generator{}
}

// GeneratedRBAC contains all RBAC resources generated for a Transform
type GeneratedRBAC struct {
	// ClusterRole is generated for scope=Cluster
	ClusterRole *rbacv1.ClusterRole

	// Role is generated for scope=Namespace
	Role *rbacv1.Role

	// RoleBinding is generated for scope=Namespace
	RoleBinding *rbacv1.RoleBinding
}

// Generate creates RBAC resources based on Transform scope.
// Returns nil if no managedResources are defined.
func (g *Generator) Generate(tf *platformv1alpha1.Transform, serviceAccountName, serviceAccountNamespace string) *GeneratedRBAC {
	if len(tf.Spec.ManagedResources) == 0 {
		return nil
	}

	result := &GeneratedRBAC{}

	// Determine scope (default to Cluster)
	scope := tf.Spec.RBACScope
	if scope == "" {
		scope = platformv1alpha1.RBACScopeCluster
	}

	switch scope {
	case platformv1alpha1.RBACScopeCluster:
		result.ClusterRole = g.generateClusterRole(tf)
	case platformv1alpha1.RBACScopeNamespace:
		result.Role = g.generateRole(tf)
		result.RoleBinding = g.generateRoleBinding(tf, serviceAccountName, serviceAccountNamespace)
	}

	return result
}

// generateClusterRole creates a ClusterRole with aggregation label
func (g *Generator) generateClusterRole(tf *platformv1alpha1.Transform) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: g.RoleName(tf.Name, tf.Namespace),
			Labels: map[string]string{
				AggregationLabel:        "true",
				TransformLabel:          tf.Name,
				TransformNamespaceLabel: tf.Namespace,
			},
		},
		Rules: g.buildPolicyRules(tf.Spec.ManagedResources),
	}
}

// generateRole creates a namespaced Role
func (g *Generator) generateRole(tf *platformv1alpha1.Transform) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.RoleName(tf.Name, tf.Namespace),
			Namespace: tf.Namespace,
			Labels: map[string]string{
				TransformLabel:          tf.Name,
				TransformNamespaceLabel: tf.Namespace,
			},
		},
		Rules: g.buildPolicyRules(tf.Spec.ManagedResources),
	}
}

// generateRoleBinding creates a RoleBinding to the operator's ServiceAccount
func (g *Generator) generateRoleBinding(tf *platformv1alpha1.Transform, saName, saNamespace string) *rbacv1.RoleBinding {
	roleName := g.RoleName(tf.Name, tf.Namespace)
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: tf.Namespace,
			Labels: map[string]string{
				TransformLabel:          tf.Name,
				TransformNamespaceLabel: tf.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: saNamespace,
			},
		},
	}
}

// buildPolicyRules converts ManagedResources to PolicyRules
func (g *Generator) buildPolicyRules(resources []platformv1alpha1.ManagedResource) []rbacv1.PolicyRule {
	rules := make([]rbacv1.PolicyRule, 0, len(resources))

	for _, mr := range resources {
		rule := rbacv1.PolicyRule{
			APIGroups: []string{mr.APIGroup},
			Resources: mr.Resources,
			Verbs:     StandardVerbs,
		}
		rules = append(rules, rule)
	}

	return rules
}

// RoleName returns the name of the Role/ClusterRole for a Transform.
// For cluster-scoped roles, the namespace is included to avoid collisions.
func (g *Generator) RoleName(transformName, transformNamespace string) string {
	// Sanitize the name to be valid for Kubernetes
	// Replace any characters that aren't alphanumeric or allowed punctuation
	name := fmt.Sprintf("%s%s.%s", RolePrefix, transformNamespace, transformName)
	return strings.ToLower(name)
}

// RuleCount returns the number of policy rules that would be generated
func (g *Generator) RuleCount(resources []platformv1alpha1.ManagedResource) int {
	return len(resources)
}

// ToGeneratedRBACReference creates a status reference from generated RBAC
func (g *Generator) ToGeneratedRBACReference(rbac *GeneratedRBAC) *platformv1alpha1.GeneratedRBACReference {
	if rbac == nil {
		return nil
	}

	ref := &platformv1alpha1.GeneratedRBACReference{}

	if rbac.ClusterRole != nil {
		ref.ClusterRoleName = rbac.ClusterRole.Name
		ref.RuleCount = len(rbac.ClusterRole.Rules)
	}

	if rbac.Role != nil {
		ref.RoleName = rbac.Role.Name
		ref.RuleCount = len(rbac.Role.Rules)
	}

	if rbac.RoleBinding != nil {
		ref.RoleBindingName = rbac.RoleBinding.Name
	}

	return ref
}
