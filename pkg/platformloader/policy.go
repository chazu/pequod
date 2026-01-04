package platformloader

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pequod/pkg/graph"
)

// PolicyValidator validates inputs and outputs against CUE policies
type PolicyValidator struct {
	loader *Loader
}

// NewPolicyValidator creates a new policy validator
func NewPolicyValidator(loader *Loader) *PolicyValidator {
	return &PolicyValidator{
		loader: loader,
	}
}

// ValidateInput validates platform input against input policies
// The input map should contain the spec fields for the platform type being validated.
// For webservice: image (string), port (int), replicas (int)
func (pv *PolicyValidator) ValidateInput(ctx context.Context, input map[string]interface{}) ([]graph.Violation, error) {
	// For now, we'll implement basic validation in Go
	// In the future, this could use CUE policy evaluation
	var violations []graph.Violation

	// Validate image is not empty (common to webservice)
	if image, ok := input["image"].(string); ok {
		if image == "" {
			violations = append(violations, graph.Violation{
				Path:     "spec.image",
				Message:  "image is required",
				Severity: "Error",
			})
		}
	}

	// Validate port range if present
	if port, ok := getInt(input["port"]); ok {
		if port < 1 || port > 65535 {
			violations = append(violations, graph.Violation{
				Path:     "spec.port",
				Message:  fmt.Sprintf("port must be between 1 and 65535, got %d", port),
				Severity: "Error",
			})
		}
	}

	// Validate replicas if specified
	if replicas, ok := getInt(input["replicas"]); ok {
		if replicas < 0 {
			violations = append(violations, graph.Violation{
				Path:     "spec.replicas",
				Message:  fmt.Sprintf("replicas must be non-negative, got %d", replicas),
				Severity: "Error",
			})
		}

		// Warn if replicas is very high
		if replicas > 10 {
			violations = append(violations, graph.Violation{
				Path:     "spec.replicas",
				Message:  fmt.Sprintf("replicas (%d) is higher than recommended maximum (10)", replicas),
				Severity: "Warning",
			})
		}
	}

	return violations, nil
}

// getInt extracts an integer from various numeric types
func getInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

// ValidateOutput validates a rendered Graph against output policies
func (pv *PolicyValidator) ValidateOutput(ctx context.Context, g *graph.Graph) ([]graph.Violation, error) {
	var violations []graph.Violation

	// Validate graph structure
	if err := g.Validate(); err != nil {
		violations = append(violations, graph.Violation{
			Path:     "graph",
			Message:  fmt.Sprintf("graph validation failed: %v", err),
			Severity: "Error",
		})
		return violations, nil
	}

	// Check for required resources
	hasDeployment := false
	hasService := false

	for _, node := range g.Nodes {
		switch node.Object.GetKind() {
		case "Deployment":
			hasDeployment = true
		case "Service":
			hasService = true
		}
	}

	if !hasDeployment {
		violations = append(violations, graph.Violation{
			Path:     "graph.nodes",
			Message:  "graph must contain at least one Deployment",
			Severity: "Error",
		})
	}

	if !hasService {
		violations = append(violations, graph.Violation{
			Path:     "graph.nodes",
			Message:  "graph must contain at least one Service",
			Severity: "Warning",
		})
	}

	// Validate that all nodes have proper labels
	for i, node := range g.Nodes {
		labels := node.Object.GetLabels()
		if len(labels) == 0 {
			violations = append(violations, graph.Violation{
				Path:     fmt.Sprintf("graph.nodes[%d].object.metadata.labels", i),
				Message:  "resource should have labels",
				Severity: "Warning",
			})
		}

		// Check for managed-by label
		if labels["app.kubernetes.io/managed-by"] != "pequod" {
			violations = append(violations, graph.Violation{
				Path:     fmt.Sprintf("graph.nodes[%d].object.metadata.labels", i),
				Message:  "resource should have 'app.kubernetes.io/managed-by: pequod' label",
				Severity: "Warning",
			})
		}
	}

	return violations, nil
}

// ValidateCUEPolicy validates using CUE policy definitions (future implementation)
func (pv *PolicyValidator) ValidateCUEPolicy(ctx context.Context, cueValue cue.Value, input interface{}) ([]graph.Violation, error) {
	// This is a placeholder for future CUE-based policy validation
	// For now, we use the Go-based validation above
	return nil, fmt.Errorf("CUE policy validation not yet implemented")
}
