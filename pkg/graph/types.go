package graph

import (
	"encoding/json"
	"fmt"

	"github.com/cespare/xxhash/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Graph represents a dependency graph of Kubernetes resources to be applied
type Graph struct {
	// Metadata contains information about the graph
	Metadata GraphMetadata `json:"metadata"`

	// Nodes contains all the resources to be applied
	Nodes []Node `json:"nodes"`

	// Violations contains any policy violations found during rendering
	Violations []Violation `json:"violations,omitempty"`
}

// GraphMetadata contains metadata about the graph
type GraphMetadata struct {
	// Name is a human-readable name for the graph
	Name string `json:"name"`

	// Version is the version of the graph format
	Version string `json:"version"`

	// PlatformRef is the reference to the platform module used
	PlatformRef string `json:"platformRef,omitempty"`

	// RenderHash is a hash of the rendered graph for change detection
	RenderHash string `json:"renderHash,omitempty"`
}

// Node represents a single resource in the graph
type Node struct {
	// ID is a unique identifier for this node within the graph
	ID string `json:"id"`

	// Object is the Kubernetes resource to apply
	Object unstructured.Unstructured `json:"object"`

	// ApplyPolicy defines how this resource should be applied
	ApplyPolicy ApplyPolicy `json:"applyPolicy"`

	// DependsOn lists the IDs of nodes that must be ready before this node
	DependsOn []string `json:"dependsOn,omitempty"`

	// ReadyWhen defines the conditions for this resource to be considered ready
	ReadyWhen []ReadinessPredicate `json:"readyWhen,omitempty"`
}

// ApplyPolicy defines how a resource should be applied
type ApplyPolicy struct {
	// Mode determines the apply behavior
	// - "Apply": Use Server-Side Apply (default)
	// - "Create": Only create if it doesn't exist
	// - "Adopt": Adopt existing resource
	Mode ApplyMode `json:"mode,omitempty"`

	// ConflictPolicy determines how to handle field manager conflicts
	// - "Error": Fail on conflicts (default)
	// - "Force": Force ownership of conflicting fields
	ConflictPolicy ConflictPolicy `json:"conflictPolicy,omitempty"`

	// FieldManager is the name to use for field management
	// Defaults to "pequod-operator"
	FieldManager string `json:"fieldManager,omitempty"`
}

// ApplyMode defines the apply behavior
type ApplyMode string

const (
	// ApplyModeApply uses Server-Side Apply
	ApplyModeApply ApplyMode = "Apply"

	// ApplyModeCreate only creates if the resource doesn't exist
	ApplyModeCreate ApplyMode = "Create"

	// ApplyModeAdopt adopts an existing resource
	ApplyModeAdopt ApplyMode = "Adopt"
)

// ConflictPolicy defines how to handle field manager conflicts
type ConflictPolicy string

const (
	// ConflictPolicyError fails on conflicts
	ConflictPolicyError ConflictPolicy = "Error"

	// ConflictPolicyForce forces ownership of conflicting fields
	ConflictPolicyForce ConflictPolicy = "Force"
)

// ReadinessPredicate defines a condition that must be met for a resource to be ready
type ReadinessPredicate struct {
	// Type is the type of predicate
	Type PredicateType `json:"type"`

	// ConditionType is the condition type to check (for ConditionMatch predicates)
	ConditionType string `json:"conditionType,omitempty"`

	// ConditionStatus is the expected status (for ConditionMatch predicates)
	ConditionStatus string `json:"conditionStatus,omitempty"`

	// Timeout is the maximum time to wait for this predicate (in seconds)
	Timeout int `json:"timeout,omitempty"`
}

// PredicateType defines the type of readiness predicate
type PredicateType string

const (
	// PredicateTypeConditionMatch checks for a specific condition
	PredicateTypeConditionMatch PredicateType = "ConditionMatch"

	// PredicateTypeDeploymentAvailable checks if a Deployment is available
	PredicateTypeDeploymentAvailable PredicateType = "DeploymentAvailable"

	// PredicateTypeExists checks if the resource exists
	PredicateTypeExists PredicateType = "Exists"
)

// Violation represents a policy violation
type Violation struct {
	// Path is the JSON path to the violating field
	Path string `json:"path"`

	// Message is a human-readable description of the violation
	Message string `json:"message"`

	// Severity indicates how serious the violation is
	Severity ViolationSeverity `json:"severity"`
}

// ViolationSeverity indicates the severity of a policy violation
type ViolationSeverity string

const (
	// ViolationSeverityError indicates a blocking violation
	ViolationSeverityError ViolationSeverity = "Error"

	// ViolationSeverityWarning indicates a non-blocking violation
	ViolationSeverityWarning ViolationSeverity = "Warning"
)

// ComputeHash computes a hash of the graph for drift detection
// This hashes the nodes (excluding metadata) to detect changes
func (g *Graph) ComputeHash() string {
	// We only hash the nodes, not the metadata
	// This way we can detect if the actual resources changed
	type hashableGraph struct {
		Nodes      []Node      `json:"nodes"`
		Violations []Violation `json:"violations"`
	}

	h := hashableGraph{
		Nodes:      g.Nodes,
		Violations: g.Violations,
	}

	// Use a simple JSON-based hash for now
	// In production, you might want to use the controller-idioms hash package
	data, err := json.Marshal(h)
	if err != nil {
		return ""
	}

	// Use xxhash for fast hashing
	return fmt.Sprintf("%x", xxhash.Sum64(data))
}

// SetHash computes and sets the RenderHash field
func (g *Graph) SetHash() {
	g.Metadata.RenderHash = g.ComputeHash()
}

// HasChanged returns true if the graph has changed since the last hash
func (g *Graph) HasChanged(previousHash string) bool {
	if previousHash == "" {
		return true // No previous hash means this is new
	}
	return g.ComputeHash() != previousHash
}
