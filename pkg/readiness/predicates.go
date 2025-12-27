package readiness

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Evaluator is the interface for evaluating readiness predicates
type Evaluator interface {
	// Evaluate checks if the predicate is satisfied for the given object
	Evaluate(ctx context.Context, c client.Client, obj *unstructured.Unstructured) (bool, error)
}

// ConditionMatchPredicate checks if a specific condition has the expected status
type ConditionMatchPredicate struct {
	ConditionType   string
	ConditionStatus string
}

// Evaluate checks if the condition matches
func (p *ConditionMatchPredicate) Evaluate(ctx context.Context, c client.Client, obj *unstructured.Unstructured) (bool, error) {
	// Get the status.conditions field
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("failed to get conditions: %w", err)
	}
	if !found {
		return false, nil
	}

	// Look for the matching condition
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condMap, "type")
		if condType != p.ConditionType {
			continue
		}

		condStatus, _, _ := unstructured.NestedString(condMap, "status")
		if condStatus == p.ConditionStatus {
			return true, nil
		}

		// Condition found but status doesn't match
		return false, nil
	}

	// Condition not found
	return false, nil
}

// DeploymentAvailablePredicate checks if a Deployment is available
type DeploymentAvailablePredicate struct{}

// Evaluate checks if the Deployment is available
func (p *DeploymentAvailablePredicate) Evaluate(ctx context.Context, c client.Client, obj *unstructured.Unstructured) (bool, error) {
	// Convert to Deployment
	var deployment appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment); err != nil {
		return false, fmt.Errorf("failed to convert to Deployment: %w", err)
	}

	// Check if the Deployment has the Available condition
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

// ExistsPredicate checks if the resource exists
type ExistsPredicate struct{}

// Evaluate checks if the resource exists
func (p *ExistsPredicate) Evaluate(ctx context.Context, c client.Client, obj *unstructured.Unstructured) (bool, error) {
	// Try to get the resource
	key := client.ObjectKeyFromObject(obj)
	err := c.Get(ctx, key, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get resource: %w", err)
	}

	return true, nil
}

// NewEvaluator creates an Evaluator from a predicate type and parameters
func NewEvaluator(predicateType, conditionType, conditionStatus string) (Evaluator, error) {
	switch predicateType {
	case "ConditionMatch":
		if conditionType == "" {
			return nil, fmt.Errorf("conditionType is required for ConditionMatch predicate")
		}
		if conditionStatus == "" {
			return nil, fmt.Errorf("conditionStatus is required for ConditionMatch predicate")
		}
		return &ConditionMatchPredicate{
			ConditionType:   conditionType,
			ConditionStatus: conditionStatus,
		}, nil

	case "DeploymentAvailable":
		return &DeploymentAvailablePredicate{}, nil

	case "Exists":
		return &ExistsPredicate{}, nil

	default:
		return nil, fmt.Errorf("unknown predicate type: %s", predicateType)
	}
}
