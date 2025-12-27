package graph

import (
	"fmt"
)

// Validate checks the integrity of the Graph
func (g *Graph) Validate() error {
	if g.Metadata.Name == "" {
		return fmt.Errorf("graph metadata.name is required")
	}

	if g.Metadata.Version == "" {
		return fmt.Errorf("graph metadata.version is required")
	}

	// Check for duplicate node IDs
	nodeIDs := make(map[string]bool)
	for _, node := range g.Nodes {
		if node.ID == "" {
			return fmt.Errorf("node ID is required")
		}
		if nodeIDs[node.ID] {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true
	}

	// Validate each node
	for _, node := range g.Nodes {
		if err := node.Validate(nodeIDs); err != nil {
			return fmt.Errorf("node %s: %w", node.ID, err)
		}
	}

	// Check for cycles (will be implemented in DAG builder)
	// For now, just validate that dependencies exist
	for _, node := range g.Nodes {
		for _, depID := range node.DependsOn {
			if !nodeIDs[depID] {
				return fmt.Errorf("node %s depends on non-existent node: %s", node.ID, depID)
			}
		}
	}

	return nil
}

// Validate checks the integrity of a Node
func (n *Node) Validate(allNodeIDs map[string]bool) error {
	if n.ID == "" {
		return fmt.Errorf("node ID is required")
	}

	// Validate the object has required fields
	if n.Object.GetKind() == "" {
		return fmt.Errorf("object kind is required")
	}

	if n.Object.GetAPIVersion() == "" {
		return fmt.Errorf("object apiVersion is required")
	}

	if n.Object.GetName() == "" {
		return fmt.Errorf("object name is required")
	}

	// Validate apply policy
	if err := n.ApplyPolicy.Validate(); err != nil {
		return fmt.Errorf("applyPolicy: %w", err)
	}

	// Validate dependencies exist
	for _, depID := range n.DependsOn {
		if !allNodeIDs[depID] {
			return fmt.Errorf("dependency %s does not exist", depID)
		}
	}

	// Validate readiness predicates
	for i, pred := range n.ReadyWhen {
		if err := pred.Validate(); err != nil {
			return fmt.Errorf("readyWhen[%d]: %w", i, err)
		}
	}

	return nil
}

// Validate checks the integrity of an ApplyPolicy
func (ap *ApplyPolicy) Validate() error {
	// Set defaults
	if ap.Mode == "" {
		ap.Mode = ApplyModeApply
	}

	if ap.ConflictPolicy == "" {
		ap.ConflictPolicy = ConflictPolicyError
	}

	if ap.FieldManager == "" {
		ap.FieldManager = "pequod-operator"
	}

	// Validate mode
	switch ap.Mode {
	case ApplyModeApply, ApplyModeCreate, ApplyModeAdopt:
		// Valid
	default:
		return fmt.Errorf("invalid apply mode: %s", ap.Mode)
	}

	// Validate conflict policy
	switch ap.ConflictPolicy {
	case ConflictPolicyError, ConflictPolicyForce:
		// Valid
	default:
		return fmt.Errorf("invalid conflict policy: %s", ap.ConflictPolicy)
	}

	return nil
}

// Validate checks the integrity of a ReadinessPredicate
func (rp *ReadinessPredicate) Validate() error {
	// Validate type
	switch rp.Type {
	case PredicateTypeConditionMatch:
		if rp.ConditionType == "" {
			return fmt.Errorf("conditionType is required for ConditionMatch predicate")
		}
		if rp.ConditionStatus == "" {
			return fmt.Errorf("conditionStatus is required for ConditionMatch predicate")
		}
	case PredicateTypeDeploymentAvailable, PredicateTypeExists:
		// No additional validation needed
	default:
		return fmt.Errorf("invalid predicate type: %s", rp.Type)
	}

	// Validate timeout
	if rp.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}

	return nil
}
