package apply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chazu/pequod/pkg/graph"
)

// Applier applies Kubernetes resources with various strategies
type Applier struct {
	client client.Client
	dryRun bool
}

// NewApplier creates a new resource applier
func NewApplier(c client.Client) *Applier {
	return &Applier{
		client: c,
		dryRun: false,
	}
}

// WithDryRun returns a new applier with dry-run mode enabled
func (a *Applier) WithDryRun(dryRun bool) *Applier {
	return &Applier{
		client: a.client,
		dryRun: dryRun,
	}
}

// Apply applies a resource according to its ApplyPolicy
func (a *Applier) Apply(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}

	// Validate and set defaults for policy
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("invalid apply policy: %w", err)
	}

	// Apply based on mode
	switch policy.Mode {
	case graph.ApplyModeApply:
		return a.applySSA(ctx, obj, policy)
	case graph.ApplyModeCreate:
		return a.applyCreate(ctx, obj)
	case graph.ApplyModeAdopt:
		return a.applyAdopt(ctx, obj, policy)
	default:
		return fmt.Errorf("unknown apply mode: %s", policy.Mode)
	}
}

// applySSA applies a resource using Server-Side Apply
func (a *Applier) applySSA(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy) error {
	// Build patch options
	patchOpts := []client.PatchOption{
		client.FieldOwner(policy.FieldManager),
	}

	// Add force ownership if conflict policy is Force
	if policy.ConflictPolicy == graph.ConflictPolicyForce {
		patchOpts = append(patchOpts, client.ForceOwnership)
	}

	// Add dry-run if enabled
	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	// Apply the resource
	if err := a.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		// Check if this is a conflict error
		if errors.IsConflict(err) {
			return &ConflictError{
				Resource:     fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()),
				FieldManager: policy.FieldManager,
				Err:          err,
			}
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// applyCreate creates a resource only if it doesn't exist
func (a *Applier) applyCreate(ctx context.Context, obj *unstructured.Unstructured) error {
	// Try to create the resource
	createOpts := []client.CreateOption{}
	if a.dryRun {
		createOpts = append(createOpts, client.DryRunAll)
	}

	if err := a.client.Create(ctx, obj, createOpts...); err != nil {
		if errors.IsAlreadyExists(err) {
			// Resource already exists - this is not an error for Create mode
			return nil
		}
		return fmt.Errorf("failed to create resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// applyAdopt adopts an existing resource by applying field management
func (a *Applier) applyAdopt(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy) error {
	// First, check if the resource exists
	key := client.ObjectKeyFromObject(obj)
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())

	if err := a.client.Get(ctx, key, existing); err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist - create it
			return a.applyCreate(ctx, obj)
		}
		return fmt.Errorf("failed to check if resource exists: %w", err)
	}

	// Resource exists - adopt it using SSA with force ownership
	// This takes ownership of all fields
	patchOpts := []client.PatchOption{
		client.FieldOwner(policy.FieldManager),
		client.ForceOwnership, // Always force for adoption
	}

	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	if err := a.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to adopt resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// ConflictError represents a field manager conflict
type ConflictError struct {
	Resource     string
	FieldManager string
	Err          error
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("field manager conflict for %s (field manager: %s): %v", e.Resource, e.FieldManager, e.Err)
}

func (e *ConflictError) Unwrap() error {
	return e.Err
}
