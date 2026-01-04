package apply

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/chazu/pequod/pkg/inventory"
)

// DeletionPolicy defines how orphaned resources should be handled
type DeletionPolicy string

const (
	// DeletionPolicyDelete deletes orphaned resources
	DeletionPolicyDelete DeletionPolicy = "Delete"

	// DeletionPolicyOrphan removes resources from management without deleting them
	DeletionPolicyOrphan DeletionPolicy = "Orphan"

	// ProtectionAnnotation prevents a resource from being pruned
	ProtectionAnnotation = "pequod.io/prune-protection"

	// GracePeriodAnnotation specifies a grace period before pruning
	GracePeriodAnnotation = "pequod.io/prune-grace-period"

	// DefaultGracePeriod is the default grace period before pruning
	DefaultGracePeriod = 30 * time.Second
)

// PruneOptions configures pruning behavior
type PruneOptions struct {
	// DeletionPolicy determines how to handle orphaned resources
	DeletionPolicy DeletionPolicy

	// GracePeriod is the minimum time to wait before pruning
	GracePeriod time.Duration

	// DryRun if true, only reports what would be pruned without deleting
	DryRun bool

	// PropagationPolicy for deletion (Orphan, Background, Foreground)
	PropagationPolicy *metav1.DeletionPropagation
}

// DefaultPruneOptions returns default pruning options
func DefaultPruneOptions() PruneOptions {
	background := metav1.DeletePropagationBackground
	return PruneOptions{
		DeletionPolicy:    DeletionPolicyDelete,
		GracePeriod:       DefaultGracePeriod,
		DryRun:            false,
		PropagationPolicy: &background,
	}
}

// PruneResult contains the result of a prune operation
type PruneResult struct {
	// Pruned contains resources that were deleted
	Pruned []PrunedResource

	// Protected contains resources that were protected from pruning
	Protected []PrunedResource

	// Orphaned contains resources that were orphaned (removed from management)
	Orphaned []PrunedResource

	// Errors contains any errors that occurred during pruning
	Errors []PruneError
}

// PrunedResource describes a resource that was pruned
type PrunedResource struct {
	ID        string
	GVK       schema.GroupVersionKind
	Namespace string
	Name      string
}

// PruneError describes an error that occurred during pruning
type PruneError struct {
	Resource PrunedResource
	Error    error
}

// Pruner handles deletion of orphaned resources
type Pruner struct {
	client client.Client
}

// NewPruner creates a new pruner
func NewPruner(c client.Client) *Pruner {
	return &Pruner{
		client: c,
	}
}

// Prune removes orphaned resources from the cluster
func (p *Pruner) Prune(ctx context.Context, tracker *inventory.Tracker, currentNodeIDs map[string]bool, opts PruneOptions) (*PruneResult, error) {
	logger := log.FromContext(ctx)
	result := &PruneResult{}

	// Find orphaned resources
	orphaned := tracker.FindOrphaned(currentNodeIDs)

	if len(orphaned) == 0 {
		logger.V(1).Info("No orphaned resources to prune")
		return result, nil
	}

	logger.Info("Found orphaned resources", "count", len(orphaned))

	for _, item := range orphaned {
		prunedResource := PrunedResource{
			ID:        item.ID,
			GVK:       item.GVK,
			Namespace: item.Namespace,
			Name:      item.Name,
		}

		// Check if the resource still exists
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(item.GVK)

		err := p.client.Get(ctx, client.ObjectKey{
			Namespace: item.Namespace,
			Name:      item.Name,
		}, obj)

		if errors.IsNotFound(err) {
			// Resource already gone, just update the tracker
			logger.V(1).Info("Resource already deleted", "id", item.ID)
			tracker.Remove(item.ID)
			continue
		}

		if err != nil {
			result.Errors = append(result.Errors, PruneError{
				Resource: prunedResource,
				Error:    fmt.Errorf("failed to get resource: %w", err),
			})
			continue
		}

		// Check for protection annotation
		if p.isProtected(obj) {
			logger.Info("Resource is protected from pruning", "id", item.ID)
			result.Protected = append(result.Protected, prunedResource)
			continue
		}

		// Check grace period
		if !p.gracePeriodExpired(obj, opts.GracePeriod) {
			logger.V(1).Info("Resource grace period not expired", "id", item.ID)
			continue
		}

		// Handle based on deletion policy
		switch opts.DeletionPolicy {
		case DeletionPolicyDelete:
			if opts.DryRun {
				logger.Info("Would prune resource (dry-run)", "id", item.ID, "gvk", item.GVK)
				result.Pruned = append(result.Pruned, prunedResource)
			} else {
				if err := p.deleteResource(ctx, obj, opts); err != nil {
					result.Errors = append(result.Errors, PruneError{
						Resource: prunedResource,
						Error:    err,
					})
				} else {
					logger.Info("Pruned resource", "id", item.ID, "gvk", item.GVK)
					tracker.RecordPruned(item.ID)
					result.Pruned = append(result.Pruned, prunedResource)
				}
			}

		case DeletionPolicyOrphan:
			logger.Info("Orphaning resource", "id", item.ID, "gvk", item.GVK)
			tracker.Remove(item.ID)
			result.Orphaned = append(result.Orphaned, prunedResource)
		}
	}

	return result, nil
}

// PruneByIDs prunes specific resources by their IDs
func (p *Pruner) PruneByIDs(ctx context.Context, tracker *inventory.Tracker, ids []string, opts PruneOptions) (*PruneResult, error) {
	logger := log.FromContext(ctx)
	result := &PruneResult{}

	for _, id := range ids {
		item, ok := tracker.Get(id)
		if !ok {
			continue // Not in inventory
		}

		prunedResource := PrunedResource{
			ID:        item.ID,
			GVK:       item.GVK,
			Namespace: item.Namespace,
			Name:      item.Name,
		}

		// Get the resource
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(item.GVK)

		err := p.client.Get(ctx, client.ObjectKey{
			Namespace: item.Namespace,
			Name:      item.Name,
		}, obj)

		if errors.IsNotFound(err) {
			tracker.Remove(item.ID)
			continue
		}

		if err != nil {
			result.Errors = append(result.Errors, PruneError{
				Resource: prunedResource,
				Error:    fmt.Errorf("failed to get resource: %w", err),
			})
			continue
		}

		// Delete or orphan based on policy
		switch opts.DeletionPolicy {
		case DeletionPolicyDelete:
			if opts.DryRun {
				logger.Info("Would prune resource (dry-run)", "id", item.ID)
				result.Pruned = append(result.Pruned, prunedResource)
			} else {
				if err := p.deleteResource(ctx, obj, opts); err != nil {
					result.Errors = append(result.Errors, PruneError{
						Resource: prunedResource,
						Error:    err,
					})
				} else {
					tracker.RecordPruned(item.ID)
					result.Pruned = append(result.Pruned, prunedResource)
				}
			}

		case DeletionPolicyOrphan:
			tracker.Remove(item.ID)
			result.Orphaned = append(result.Orphaned, prunedResource)
		}
	}

	return result, nil
}

// isProtected checks if a resource has protection annotations
func (p *Pruner) isProtected(obj *unstructured.Unstructured) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	// Check for protection annotation
	if val, ok := annotations[ProtectionAnnotation]; ok {
		return val == "true" || val == "yes" || val == "1"
	}

	return false
}

// gracePeriodExpired checks if the grace period has expired
func (p *Pruner) gracePeriodExpired(obj *unstructured.Unstructured, defaultGracePeriod time.Duration) bool {
	// Check for custom grace period annotation
	annotations := obj.GetAnnotations()
	gracePeriod := defaultGracePeriod

	if annotations != nil {
		if val, ok := annotations[GracePeriodAnnotation]; ok {
			if parsed, err := time.ParseDuration(val); err == nil {
				gracePeriod = parsed
			}
		}
	}

	// Check if enough time has passed since the resource was last modified
	// For now, we use creationTimestamp as a proxy
	// In a real implementation, you'd track when the resource became orphaned
	creationTime := obj.GetCreationTimestamp()
	if creationTime.IsZero() {
		return true
	}

	return time.Since(creationTime.Time) > gracePeriod
}

// deleteResource deletes a resource from the cluster
func (p *Pruner) deleteResource(ctx context.Context, obj *unstructured.Unstructured, opts PruneOptions) error {
	deleteOpts := []client.DeleteOption{}

	if opts.PropagationPolicy != nil {
		deleteOpts = append(deleteOpts, client.PropagationPolicy(*opts.PropagationPolicy))
	}

	if err := p.client.Delete(ctx, obj, deleteOpts...); err != nil {
		if errors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	return nil
}

// CleanupOrphaned removes all orphaned items from the tracker
// without deleting them from the cluster
func (p *Pruner) CleanupOrphaned(tracker *inventory.Tracker) int {
	items := tracker.GetAll()
	removed := 0

	for _, item := range items {
		if item.Status == inventory.ItemStatusOrphaned || item.Status == inventory.ItemStatusPruned {
			tracker.Remove(item.ID)
			removed++
		}
	}

	return removed
}
