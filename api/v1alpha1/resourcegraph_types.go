/*
Copyright 2024.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// ResourceGraphSpec defines the rendered graph of Kubernetes resources
type ResourceGraphSpec struct {
	// SourceRef references the source resource that generated this graph
	// (e.g., WebService, Database, etc.)
	// +kubebuilder:validation:Required
	SourceRef ObjectReference `json:"sourceRef"`

	// Metadata contains information about the graph
	// +kubebuilder:validation:Required
	Metadata GraphMetadata `json:"metadata"`

	// Nodes contains all the resources to be applied
	// Limited to prevent etcd size issues
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:MinItems=1
	Nodes []ResourceNode `json:"nodes"`

	// Violations contains any policy violations found during rendering
	// +optional
	Violations []PolicyViolation `json:"violations,omitempty"`

	// RenderHash is a hash of the rendered graph for change detection
	// +kubebuilder:validation:Required
	RenderHash string `json:"renderHash"`

	// RenderedAt is when this graph was rendered
	// +kubebuilder:validation:Required
	RenderedAt metav1.Time `json:"renderedAt"`
}

// ObjectReference contains enough information to locate a Kubernetes object
type ObjectReference struct {
	// APIVersion of the referent
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the referent
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the referent
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the referent (empty for cluster-scoped)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// UID of the referent
	// +optional
	UID string `json:"uid,omitempty"`
}

// GraphMetadata contains metadata about the graph
type GraphMetadata struct {
	// Name is a human-readable name for the graph
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Version is the version of the graph format
	// +kubebuilder:validation:Required
	// +kubebuilder:default="v1alpha1"
	Version string `json:"version"`

	// PlatformRef is the reference to the platform module used
	// +optional
	PlatformRef string `json:"platformRef,omitempty"`
}

// ResourceNode represents a single resource in the graph
type ResourceNode struct {
	// ID is a unique identifier for this node within the graph
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Object is the Kubernetes resource to apply
	// Stored as RawExtension to preserve the full object
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Object runtime.RawExtension `json:"object"`

	// ApplyPolicy defines how this resource should be applied
	// +kubebuilder:validation:Required
	ApplyPolicy ApplyPolicy `json:"applyPolicy"`

	// DependsOn lists the IDs of nodes that must be ready before this node
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// ReadyWhen defines the conditions for this resource to be considered ready
	// +optional
	ReadyWhen []ReadinessPredicate `json:"readyWhen,omitempty"`
}

// ApplyPolicy defines how a resource should be applied
type ApplyPolicy struct {
	// Mode specifies the apply mode
	// +kubebuilder:validation:Enum=Apply;Create;Adopt
	// +kubebuilder:default="Apply"
	Mode string `json:"mode"`

	// ConflictPolicy specifies how to handle conflicts
	// +kubebuilder:validation:Enum=Error;Force
	// +kubebuilder:default="Error"
	ConflictPolicy string `json:"conflictPolicy"`

	// FieldManager is the name of the field manager for server-side apply
	// +optional
	FieldManager string `json:"fieldManager,omitempty"`
}

// ReadinessPredicate defines a condition for resource readiness
type ReadinessPredicate struct {
	// Type is the type of predicate
	// +kubebuilder:validation:Enum=ConditionMatch;DeploymentAvailable;Exists
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// ConditionType is the condition type to check (for ConditionMatch)
	// +optional
	ConditionType string `json:"conditionType,omitempty"`

	// ConditionStatus is the expected status (for ConditionMatch)
	// +optional
	ConditionStatus string `json:"conditionStatus,omitempty"`
}

// PolicyViolation represents a policy violation found during rendering
type PolicyViolation struct {
	// Path is the JSON path to the violating field
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Message describes the violation
	// +kubebuilder:validation:Required
	Message string `json:"message"`

	// Severity indicates the severity of the violation
	// +kubebuilder:validation:Enum=Error;Warning;Info
	// +kubebuilder:validation:Required
	Severity string `json:"severity"`
}

// ResourceGraphStatus defines the execution state of the graph
type ResourceGraphStatus struct {
	// Phase indicates the overall execution phase
	// +kubebuilder:validation:Enum=Pending;Executing;Completed;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// NodeStates tracks the execution state of each node
	// Key is the node ID
	// +optional
	NodeStates map[string]NodeExecutionState `json:"nodeStates,omitempty"`

	// StartedAt is when execution started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is when execution completed (success or failure)
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Conditions represent the current state of the ResourceGraph
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeExecutionState tracks the execution state of a single node
type NodeExecutionState struct {
	// Phase indicates the node's execution phase
	// +kubebuilder:validation:Enum=Pending;Applying;WaitingReady;Ready;Error
	// +kubebuilder:validation:Required
	Phase string `json:"phase"`

	// Message provides human-readable details about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// LastError contains the last error encountered
	// +optional
	LastError string `json:"lastError,omitempty"`

	// LastTransitionTime is when the phase last changed
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// AppliedAt is when the resource was successfully applied
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// ReadyAt is when the resource became ready
	// +optional
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`

	// ResourceRef contains the reference to the applied resource
	// +optional
	ResourceRef *ObjectReference `json:"resourceRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rg
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ResourceGraph represents a rendered graph of Kubernetes resources to be applied
// It is the output of rendering a high-level abstraction (like WebService) through CUE templates
type ResourceGraph struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceGraphSpec   `json:"spec,omitempty"`
	Status ResourceGraphStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceGraphList contains a list of ResourceGraph
type ResourceGraphList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceGraph `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceGraph{}, &ResourceGraphList{})
}
