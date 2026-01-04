package apply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/graph"
)

// DefaultFieldManager is the default field manager name for Pequod
const DefaultFieldManager = "pequod-operator"

// AdoptResult represents the result of adopting a single resource
type AdoptResult struct {
	// NodeID is the graph node this adoption maps to
	NodeID string
	// Resource identifies the adopted resource
	Resource ResourceRef
	// Adopted indicates if the resource was successfully adopted
	Adopted bool
	// AlreadyManaged indicates if the resource was already managed by Pequod
	AlreadyManaged bool
	// Created indicates if the resource was created (didn't exist)
	Created bool
	// Error is set if adoption failed
	Error error
	// ConflictingManagers lists other field managers that own parts of the resource
	ConflictingManagers []string
}

// ResourceRef identifies a Kubernetes resource
type ResourceRef struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// String returns a human-readable representation
func (r ResourceRef) String() string {
	if r.Namespace == "" {
		return fmt.Sprintf("%s/%s %s", r.APIVersion, r.Kind, r.Name)
	}
	return fmt.Sprintf("%s/%s %s/%s", r.APIVersion, r.Kind, r.Namespace, r.Name)
}

// AdoptionReport contains the results of an adoption operation
type AdoptionReport struct {
	// Results contains per-resource adoption results
	Results []AdoptResult
	// TotalAdopted is the count of successfully adopted resources
	TotalAdopted int
	// TotalFailed is the count of failed adoptions
	TotalFailed int
	// TotalSkipped is the count of resources that were already managed
	TotalSkipped int
	// TotalCreated is the count of resources that were created (didn't exist)
	TotalCreated int
}

// HasErrors returns true if any adoption failed
func (r *AdoptionReport) HasErrors() bool {
	return r.TotalFailed > 0
}

// Adopter handles adopting existing resources into Pequod management
type Adopter struct {
	client       client.Client
	fieldManager string
	dryRun       bool
}

// NewAdopter creates a new resource adopter
func NewAdopter(c client.Client) *Adopter {
	return &Adopter{
		client:       c,
		fieldManager: DefaultFieldManager,
		dryRun:       false,
	}
}

// WithFieldManager returns a new adopter with a custom field manager
func (a *Adopter) WithFieldManager(fm string) *Adopter {
	return &Adopter{
		client:       a.client,
		fieldManager: fm,
		dryRun:       a.dryRun,
	}
}

// WithDryRun returns a new adopter with dry-run mode enabled
func (a *Adopter) WithDryRun(dryRun bool) *Adopter {
	return &Adopter{
		client:       a.client,
		fieldManager: a.fieldManager,
		dryRun:       dryRun,
	}
}

// Adopt processes an adoption spec and adopts matching resources
func (a *Adopter) Adopt(
	ctx context.Context,
	adoptSpec *platformv1alpha1.AdoptSpec,
	graphNodes []graph.Node,
) (*AdoptionReport, error) {
	if adoptSpec == nil {
		return &AdoptionReport{}, nil
	}

	report := &AdoptionReport{}

	switch adoptSpec.Mode {
	case "", platformv1alpha1.AdoptModeExplicit:
		return a.adoptExplicit(ctx, adoptSpec, graphNodes, report)
	case platformv1alpha1.AdoptModeLabelSelector:
		return nil, fmt.Errorf("LabelSelector mode not yet implemented")
	default:
		return nil, fmt.Errorf("unknown adoption mode: %s", adoptSpec.Mode)
	}
}

// adoptExplicit processes explicit adoption resources
func (a *Adopter) adoptExplicit(
	ctx context.Context,
	adoptSpec *platformv1alpha1.AdoptSpec,
	graphNodes []graph.Node,
	report *AdoptionReport,
) (*AdoptionReport, error) {
	for _, resource := range adoptSpec.Resources {
		result := a.adoptResource(ctx, resource, graphNodes, adoptSpec.Strategy)
		report.Results = append(report.Results, result)

		if result.Error != nil {
			report.TotalFailed++
		} else if result.Created {
			report.TotalCreated++
			report.TotalAdopted++
		} else if result.AlreadyManaged {
			report.TotalSkipped++
		} else if result.Adopted {
			report.TotalAdopted++
		}
	}

	return report, nil
}

// adoptResource attempts to adopt a single resource
func (a *Adopter) adoptResource(
	ctx context.Context,
	ref platformv1alpha1.AdoptedResourceRef,
	graphNodes []graph.Node,
	strategy platformv1alpha1.AdoptStrategy,
) AdoptResult {
	result := AdoptResult{
		NodeID: ref.NodeID,
		Resource: ResourceRef{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Namespace:  ref.Namespace,
			Name:       ref.Name,
		},
	}

	// Find matching graph node
	node := a.findMatchingNode(ref, graphNodes)
	if node == nil && ref.NodeID != "" {
		result.Error = fmt.Errorf("no graph node found with ID %q", ref.NodeID)
		return result
	}
	if node != nil {
		result.NodeID = node.ID
	}

	// Build GVK
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		result.Error = fmt.Errorf("invalid apiVersion %q: %w", ref.APIVersion, err)
		return result
	}
	gvk := gv.WithKind(ref.Kind)

	// Check if resource exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}

	err = a.client.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist - if we have a node, create from node spec
			if node != nil {
				result.Error = a.createFromNode(ctx, node)
				if result.Error == nil {
					result.Created = true
					result.Adopted = true
				}
			} else {
				result.Error = fmt.Errorf("resource not found and no matching graph node to create from")
			}
			return result
		}
		result.Error = fmt.Errorf("failed to get resource: %w", err)
		return result
	}

	// Check if already managed by Pequod
	result.ConflictingManagers = a.getFieldManagers(existing)
	for _, fm := range result.ConflictingManagers {
		if fm == a.fieldManager {
			result.AlreadyManaged = true
			return result
		}
	}

	// Adopt based on strategy
	switch strategy {
	case "", platformv1alpha1.AdoptStrategyTakeOwnership:
		result.Error = a.takeOwnership(ctx, existing, node)
	case platformv1alpha1.AdoptStrategyMirror:
		// Mirror mode: don't modify the resource, just track it
		// Future: implement mirror tracking
		result.Error = nil
	default:
		result.Error = fmt.Errorf("unknown strategy: %s", strategy)
	}

	if result.Error == nil {
		result.Adopted = true
	}

	return result
}

// findMatchingNode finds a graph node that matches the resource ref
func (a *Adopter) findMatchingNode(ref platformv1alpha1.AdoptedResourceRef, nodes []graph.Node) *graph.Node {
	// First try to match by NodeID
	if ref.NodeID != "" {
		for i := range nodes {
			if nodes[i].ID == ref.NodeID {
				return &nodes[i]
			}
		}
		return nil
	}

	// Try to match by GVK/namespace/name
	for i := range nodes {
		obj := &nodes[i].Object
		// Check if object is empty (no kind set)
		if obj.GetKind() == "" {
			continue
		}
		gvk := obj.GroupVersionKind()
		apiVersion := gvk.GroupVersion().String()
		if apiVersion == ref.APIVersion &&
			gvk.Kind == ref.Kind &&
			obj.GetNamespace() == ref.Namespace &&
			obj.GetName() == ref.Name {
			return &nodes[i]
		}
	}

	return nil
}

// getFieldManagers extracts field manager names from managed fields
func (a *Adopter) getFieldManagers(obj *unstructured.Unstructured) []string {
	managedFields := obj.GetManagedFields()
	managers := make([]string, 0, len(managedFields))
	seen := make(map[string]bool)

	for _, mf := range managedFields {
		if !seen[mf.Manager] {
			managers = append(managers, mf.Manager)
			seen[mf.Manager] = true
		}
	}

	return managers
}

// takeOwnership takes ownership of an existing resource using SSA
func (a *Adopter) takeOwnership(
	ctx context.Context,
	existing *unstructured.Unstructured,
	node *graph.Node,
) error {
	// Prepare the object to apply
	var obj *unstructured.Unstructured
	if node != nil && node.Object.GetKind() != "" {
		obj = node.Object.DeepCopy()
	} else {
		// Use existing resource but with our field manager
		obj = existing.DeepCopy()
	}

	// Build patch options with force ownership
	patchOpts := []client.PatchOption{
		client.FieldOwner(a.fieldManager),
		client.ForceOwnership,
	}

	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	// Apply with SSA to take ownership
	if err := a.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to take ownership: %w", err)
	}

	return nil
}

// createFromNode creates a resource from a graph node
func (a *Adopter) createFromNode(ctx context.Context, node *graph.Node) error {
	if node == nil || node.Object.GetKind() == "" {
		return fmt.Errorf("node or node object is empty")
	}

	obj := node.Object.DeepCopy()

	// Build patch options
	patchOpts := []client.PatchOption{
		client.FieldOwner(a.fieldManager),
	}

	if a.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	// Create using SSA
	if err := a.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	return nil
}

// CheckAdoptionSafety checks if adoption is safe for the given resources
// Returns warnings and blocking errors
func (a *Adopter) CheckAdoptionSafety(
	ctx context.Context,
	adoptSpec *platformv1alpha1.AdoptSpec,
) (warnings []string, blockingErrors []error) {
	if adoptSpec == nil {
		return nil, nil
	}

	for _, ref := range adoptSpec.Resources {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			blockingErrors = append(blockingErrors, fmt.Errorf("invalid apiVersion %q: %w", ref.APIVersion, err))
			continue
		}
		gvk := gv.WithKind(ref.Kind)

		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(gvk)
		key := client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}

		if err := a.client.Get(ctx, key, existing); err != nil {
			if errors.IsNotFound(err) {
				warnings = append(warnings, fmt.Sprintf("resource %s not found, will be created", ref.Name))
			} else {
				blockingErrors = append(blockingErrors, fmt.Errorf("failed to check resource %s: %w", ref.Name, err))
			}
			continue
		}

		// Check for conflicting managers
		managers := a.getFieldManagers(existing)
		hasPequod := false
		for _, m := range managers {
			if m == a.fieldManager {
				hasPequod = true
			}
		}

		if !hasPequod && len(managers) > 0 {
			warnings = append(warnings,
				fmt.Sprintf("resource %s/%s has field managers %v that will be overwritten",
					ref.Namespace, ref.Name, managers))
		}

		// Check for owner references that might conflict
		ownerRefs := existing.GetOwnerReferences()
		for _, ref := range ownerRefs {
			if ref.Controller != nil && *ref.Controller {
				warnings = append(warnings,
					fmt.Sprintf("resource %s/%s is owned by controller %s/%s",
						existing.GetNamespace(), existing.GetName(),
						ref.Kind, ref.Name))
			}
		}
	}

	return warnings, blockingErrors
}

// AdoptionStatus represents the adoption state of a node
type AdoptionStatus struct {
	// Adopted indicates if the node was adopted
	Adopted bool `json:"adopted"`
	// AdoptedAt is when the resource was adopted
	AdoptedAt *metav1.Time `json:"adoptedAt,omitempty"`
	// PreviousManagers lists field managers before adoption
	PreviousManagers []string `json:"previousManagers,omitempty"`
}
