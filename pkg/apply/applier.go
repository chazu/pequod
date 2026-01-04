package apply

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/chazu/pequod/pkg/graph"
	"github.com/chazu/pequod/pkg/metrics"
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

// gvkString returns a string representation of an object's GVK
func gvkString(obj *unstructured.Unstructured) string {
	gvk := obj.GroupVersionKind()
	if gvk.Group == "" {
		return fmt.Sprintf("%s/%s", gvk.Version, gvk.Kind)
	}
	return fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
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

	logger := log.FromContext(ctx).WithValues(
		"gvk", gvkString(obj),
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"mode", string(policy.Mode),
	)

	startTime := time.Now()
	var err error

	// Apply based on mode
	switch policy.Mode {
	case graph.ApplyModeApply:
		err = a.applySSA(ctx, obj, policy, logger)
	case graph.ApplyModeCreate:
		err = a.applyCreate(ctx, obj, logger)
	case graph.ApplyModeAdopt:
		err = a.applyAdopt(ctx, obj, policy, logger)
	default:
		err = fmt.Errorf("unknown apply mode: %s", policy.Mode)
	}

	// Record metrics
	duration := time.Since(startTime).Seconds()
	gvk := gvkString(obj)
	if err != nil {
		metrics.RecordApply("failure", string(policy.Mode), gvk, duration)
		logger.Error(err, "Failed to apply resource")
	} else {
		metrics.RecordApply("success", string(policy.Mode), gvk, duration)
		logger.V(1).Info("Successfully applied resource", "duration_ms", duration*1000)
		// Increment managed resources gauge on successful apply
		metrics.IncrementManagedResources(gvk, obj.GetNamespace())
	}

	return err
}

// applySSA applies a resource using Server-Side Apply
func (a *Applier) applySSA(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy, logger logr.Logger) error {
	// Build patch options
	patchOpts := []client.PatchOption{
		client.FieldOwner(policy.FieldManager),
	}

	// Add force ownership if conflict policy is Force
	if policy.ConflictPolicy == graph.ConflictPolicyForce {
		patchOpts = append(patchOpts, client.ForceOwnership)
		logger.V(2).Info("Using force ownership for SSA")
	}

	// Add dry-run if enabled
	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
		logger.V(1).Info("Running in dry-run mode")
	}

	logger.V(2).Info("Applying resource via SSA", "fieldManager", policy.FieldManager)

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
func (a *Applier) applyCreate(ctx context.Context, obj *unstructured.Unstructured, logger logr.Logger) error {
	// Try to create the resource
	createOpts := []client.CreateOption{}
	if a.dryRun {
		createOpts = append(createOpts, client.DryRunAll)
		logger.V(1).Info("Running in dry-run mode")
	}

	logger.V(2).Info("Creating resource")

	if err := a.client.Create(ctx, obj, createOpts...); err != nil {
		if errors.IsAlreadyExists(err) {
			// Resource already exists - this is not an error for Create mode
			logger.V(1).Info("Resource already exists, skipping creation")
			return nil
		}
		return fmt.Errorf("failed to create resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	logger.V(1).Info("Resource created successfully")
	return nil
}

// applyAdopt adopts an existing resource by applying field management
func (a *Applier) applyAdopt(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy, logger logr.Logger) error {
	// First, check if the resource exists
	key := client.ObjectKeyFromObject(obj)
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())

	logger.V(2).Info("Checking if resource exists for adoption")

	if err := a.client.Get(ctx, key, existing); err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist - create it
			logger.V(1).Info("Resource not found, creating instead of adopting")
			return a.applyCreate(ctx, obj, logger)
		}
		return fmt.Errorf("failed to check if resource exists: %w", err)
	}

	logger.V(1).Info("Adopting existing resource", "existingUID", existing.GetUID())

	// Resource exists - adopt it using SSA with force ownership
	// This takes ownership of all fields
	patchOpts := []client.PatchOption{
		client.FieldOwner(policy.FieldManager),
		client.ForceOwnership, // Always force for adoption
	}

	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
		logger.V(1).Info("Running in dry-run mode")
	}

	if err := a.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to adopt resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	logger.V(1).Info("Resource adopted successfully")
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
