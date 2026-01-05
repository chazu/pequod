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
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

// Applier handles RBAC resource lifecycle operations
type Applier struct {
	client client.Client
}

// NewApplier creates a new RBAC applier
func NewApplier(c client.Client) *Applier {
	return &Applier{
		client: c,
	}
}

// ApplyGeneratedRBAC applies all RBAC resources from GeneratedRBAC
func (a *Applier) ApplyGeneratedRBAC(ctx context.Context, rbac *GeneratedRBAC) error {
	if rbac == nil {
		return nil
	}

	// Apply ClusterRole if present
	if rbac.ClusterRole != nil {
		if err := a.ApplyClusterRole(ctx, rbac.ClusterRole); err != nil {
			return fmt.Errorf("failed to apply ClusterRole: %w", err)
		}
	}

	// Apply Role if present
	if rbac.Role != nil {
		if err := a.ApplyRole(ctx, rbac.Role); err != nil {
			return fmt.Errorf("failed to apply Role: %w", err)
		}
	}

	// Apply RoleBinding if present
	if rbac.RoleBinding != nil {
		if err := a.ApplyRoleBinding(ctx, rbac.RoleBinding); err != nil {
			return fmt.Errorf("failed to apply RoleBinding: %w", err)
		}
	}

	return nil
}

// DeleteGeneratedRBAC removes all RBAC resources for a Transform
func (a *Applier) DeleteGeneratedRBAC(ctx context.Context, transformName, transformNamespace string, scope platformv1alpha1.RBACScope) error {
	g := NewGenerator()
	roleName := g.RoleName(transformName, transformNamespace)

	// Default to Cluster if not specified
	if scope == "" {
		scope = platformv1alpha1.RBACScopeCluster
	}

	switch scope {
	case platformv1alpha1.RBACScopeCluster:
		if err := a.DeleteClusterRole(ctx, roleName); err != nil {
			return fmt.Errorf("failed to delete ClusterRole: %w", err)
		}
	case platformv1alpha1.RBACScopeNamespace:
		// Delete RoleBinding first, then Role
		if err := a.DeleteRoleBinding(ctx, roleName, transformNamespace); err != nil {
			return fmt.Errorf("failed to delete RoleBinding: %w", err)
		}
		if err := a.DeleteRole(ctx, roleName, transformNamespace); err != nil {
			return fmt.Errorf("failed to delete Role: %w", err)
		}
	}

	return nil
}

// ApplyClusterRole creates or updates a ClusterRole
func (a *Applier) ApplyClusterRole(ctx context.Context, role *rbacv1.ClusterRole) error {
	existing := &rbacv1.ClusterRole{}
	err := a.client.Get(ctx, types.NamespacedName{Name: role.Name}, existing)

	if errors.IsNotFound(err) {
		// Create new
		return a.client.Create(ctx, role)
	}

	if err != nil {
		return err
	}

	// Update existing - preserve resource version
	role.ResourceVersion = existing.ResourceVersion
	return a.client.Update(ctx, role)
}

// DeleteClusterRole removes a ClusterRole by name
func (a *Applier) DeleteClusterRole(ctx context.Context, name string) error {
	role := &rbacv1.ClusterRole{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name}, role)

	if errors.IsNotFound(err) {
		return nil // Already deleted
	}

	if err != nil {
		return err
	}

	return a.client.Delete(ctx, role)
}

// ApplyRole creates or updates a Role
func (a *Applier) ApplyRole(ctx context.Context, role *rbacv1.Role) error {
	existing := &rbacv1.Role{}
	err := a.client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, existing)

	if errors.IsNotFound(err) {
		// Create new
		return a.client.Create(ctx, role)
	}

	if err != nil {
		return err
	}

	// Update existing - preserve resource version
	role.ResourceVersion = existing.ResourceVersion
	return a.client.Update(ctx, role)
}

// DeleteRole removes a Role by name and namespace
func (a *Applier) DeleteRole(ctx context.Context, name, namespace string) error {
	role := &rbacv1.Role{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, role)

	if errors.IsNotFound(err) {
		return nil // Already deleted
	}

	if err != nil {
		return err
	}

	return a.client.Delete(ctx, role)
}

// ApplyRoleBinding creates or updates a RoleBinding
func (a *Applier) ApplyRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding) error {
	existing := &rbacv1.RoleBinding{}
	err := a.client.Get(ctx, types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}, existing)

	if errors.IsNotFound(err) {
		// Create new
		return a.client.Create(ctx, binding)
	}

	if err != nil {
		return err
	}

	// Update existing - preserve resource version
	binding.ResourceVersion = existing.ResourceVersion
	return a.client.Update(ctx, binding)
}

// DeleteRoleBinding removes a RoleBinding by name and namespace
func (a *Applier) DeleteRoleBinding(ctx context.Context, name, namespace string) error {
	binding := &rbacv1.RoleBinding{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, binding)

	if errors.IsNotFound(err) {
		return nil // Already deleted
	}

	if err != nil {
		return err
	}

	return a.client.Delete(ctx, binding)
}

// GetClusterRole retrieves a ClusterRole by name
func (a *Applier) GetClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	role := &rbacv1.ClusterRole{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name}, role)
	if err != nil {
		return nil, err
	}
	return role, nil
}

// GetRole retrieves a Role by name and namespace
func (a *Applier) GetRole(ctx context.Context, name, namespace string) (*rbacv1.Role, error) {
	role := &rbacv1.Role{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, role)
	if err != nil {
		return nil, err
	}
	return role, nil
}
