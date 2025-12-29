package readiness

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chazu/pequod/pkg/graph"
)

// Checker evaluates readiness predicates for Kubernetes resources
type Checker struct {
	client client.Client
}

// NewChecker creates a new readiness checker
func NewChecker(c client.Client) *Checker {
	return &Checker{
		client: c,
	}
}

// Check evaluates all readiness predicates for a resource
// Returns true if all predicates are satisfied, false otherwise
func (c *Checker) Check(ctx context.Context, obj *unstructured.Unstructured, predicates []graph.ReadinessPredicate) (bool, error) {
	if obj == nil {
		return false, fmt.Errorf("object cannot be nil")
	}

	// If no predicates, consider ready
	if len(predicates) == 0 {
		return true, nil
	}

	// Fetch the latest version of the resource
	key := client.ObjectKeyFromObject(obj)
	latest := &unstructured.Unstructured{}
	latest.SetGroupVersionKind(obj.GroupVersionKind())

	if err := c.client.Get(ctx, key, latest); err != nil {
		return false, fmt.Errorf("failed to get resource: %w", err)
	}

	// Evaluate each predicate
	for _, pred := range predicates {
		evaluator, err := c.createEvaluator(pred)
		if err != nil {
			return false, fmt.Errorf("failed to create evaluator: %w", err)
		}

		ready, err := evaluator.Evaluate(ctx, c.client, latest)
		if err != nil {
			return false, fmt.Errorf("predicate evaluation failed: %w", err)
		}

		if !ready {
			// At least one predicate not satisfied
			return false, nil
		}
	}

	// All predicates satisfied
	return true, nil
}

// createEvaluator creates an Evaluator from a ReadinessPredicate
func (c *Checker) createEvaluator(pred graph.ReadinessPredicate) (Evaluator, error) {
	return NewEvaluator(
		string(pred.Type),
		pred.ConditionType,
		pred.ConditionStatus,
	)
}
