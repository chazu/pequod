/*
Copyright 2025.

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

// CueRefType defines the type of CUE reference
// +kubebuilder:validation:Enum=oci;git;configmap;inline;embedded
type CueRefType string

const (
	// CueRefTypeOCI references a CUE module in an OCI registry
	CueRefTypeOCI CueRefType = "oci"
	// CueRefTypeGit references a CUE module in a Git repository
	CueRefTypeGit CueRefType = "git"
	// CueRefTypeConfigMap references a CUE module in a ConfigMap
	CueRefTypeConfigMap CueRefType = "configmap"
	// CueRefTypeInline contains CUE code directly in the spec
	CueRefTypeInline CueRefType = "inline"
	// CueRefTypeEmbedded references a CUE module embedded in the operator
	CueRefTypeEmbedded CueRefType = "embedded"
)

// CueReference defines how to locate and load a CUE platform module
type CueReference struct {
	// Type specifies the source type for the CUE module
	// +kubebuilder:validation:Required
	Type CueRefType `json:"type"`

	// Ref is the reference to the CUE module
	// For oci: "ghcr.io/org/platforms/webservice:v1.0.0"
	// For git: "https://github.com/org/platforms.git?ref=v1.0.0"
	// For configmap: "my-platform-configmap"
	// For inline: the CUE code itself
	// For embedded: the platform type name (e.g., "webservice")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Ref string `json:"ref"`

	// Path is the path within the CUE module to the platform definition
	// Used for git and oci types when the module contains multiple platforms
	// +optional
	Path string `json:"path,omitempty"`

	// PullSecretRef references a Secret containing credentials for private OCI/Git
	// The secret should contain keys like "username", "password" or ".dockerconfigjson"
	// +optional
	PullSecretRef *LocalObjectReference `json:"pullSecretRef,omitempty"`
}

// LocalObjectReference contains enough information to locate a local object
type LocalObjectReference struct {
	// Name of the referent
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ResolvedCueReference contains the resolved state of a CUE reference
type ResolvedCueReference struct {
	// Digest is the content hash of the resolved CUE module
	// For OCI this is the manifest digest, for Git the commit SHA
	// +optional
	Digest string `json:"digest,omitempty"`

	// FetchedAt is when the module was last fetched
	// +optional
	FetchedAt *metav1.Time `json:"fetchedAt,omitempty"`
}

// AdoptMode defines how resources are selected for adoption
// +kubebuilder:validation:Enum=Explicit;LabelSelector
type AdoptMode string

const (
	// AdoptModeExplicit requires resources to be explicitly listed
	AdoptModeExplicit AdoptMode = "Explicit"
	// AdoptModeLabelSelector selects resources by label (future)
	AdoptModeLabelSelector AdoptMode = "LabelSelector"
)

// AdoptStrategy defines how adopted resources are managed
// +kubebuilder:validation:Enum=TakeOwnership;Mirror
type AdoptStrategy string

const (
	// AdoptStrategyTakeOwnership adopts resources by taking full ownership with SSA
	AdoptStrategyTakeOwnership AdoptStrategy = "TakeOwnership"
	// AdoptStrategyMirror mirrors the resource state without full ownership
	AdoptStrategyMirror AdoptStrategy = "Mirror"
)

// AdoptSpec defines resources to adopt into management
type AdoptSpec struct {
	// Mode specifies how resources are selected for adoption
	// +kubebuilder:default=Explicit
	// +optional
	Mode AdoptMode `json:"mode,omitempty"`

	// Strategy specifies how adopted resources are managed
	// +kubebuilder:default=TakeOwnership
	// +optional
	Strategy AdoptStrategy `json:"strategy,omitempty"`

	// Resources lists specific resources to adopt (used when Mode=Explicit)
	// +optional
	Resources []AdoptedResourceRef `json:"resources,omitempty"`

	// LabelSelector selects resources to adopt by labels (used when Mode=LabelSelector)
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// AdoptedResourceRef identifies a resource to adopt
type AdoptedResourceRef struct {
	// NodeID is the ID of the graph node this resource maps to
	// If empty, the system will try to match by GVK/namespace/name
	// +optional
	NodeID string `json:"nodeId,omitempty"`

	// APIVersion of the resource to adopt
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource to adopt
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the resource to adopt
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the resource to adopt (empty for cluster-scoped)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TransformSpec defines the desired state of Transform
type TransformSpec struct {
	// CueRef specifies how to locate the CUE platform module
	// +kubebuilder:validation:Required
	CueRef CueReference `json:"cueRef"`

	// Input is the free-form input data for the CUE platform module
	// This will be validated against the CUE schema defined in the platform module
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Input runtime.RawExtension `json:"input"`

	// Adopt specifies existing resources to adopt into management
	// +optional
	Adopt *AdoptSpec `json:"adopt,omitempty"`
}

// TransformStatus defines the observed state of Transform
type TransformStatus struct {
	// ResourceGraphRef references the ResourceGraph created from this Transform
	// +optional
	ResourceGraphRef *ObjectReference `json:"resourceGraphRef,omitempty"`

	// ResolvedCueRef contains the resolved CUE module reference
	// +optional
	ResolvedCueRef *ResolvedCueReference `json:"resolvedCueRef,omitempty"`

	// Conditions represent the current state of the Transform
	// Condition types include:
	// - "CueFetched": CUE module fetched successfully
	// - "Validated": Input validated against CUE schema
	// - "Rendered": ResourceGraph created successfully
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=tf
// +kubebuilder:printcolumn:name="CueRef",type=string,JSONPath=`.spec.cueRef.ref`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Transform is the Schema for the transforms API
// Transform is the single user-facing CRD for all platform types.
// Platform definitions (WebService, Database, Queue, etc.) are CUE artifacts, not separate CRDs.
type Transform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransformSpec   `json:"spec,omitempty"`
	Status TransformStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TransformList contains a list of Transform
type TransformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Transform `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Transform{}, &TransformList{})
}

// SetCondition sets a condition on the Transform status
func (t *Transform) SetCondition(condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i := range t.Status.Conditions {
		if t.Status.Conditions[i].Type == condType {
			if t.Status.Conditions[i].Status != status {
				t.Status.Conditions[i].LastTransitionTime = now
			}
			t.Status.Conditions[i].Status = status
			t.Status.Conditions[i].Reason = reason
			t.Status.Conditions[i].Message = message
			t.Status.Conditions[i].ObservedGeneration = t.Generation
			return
		}
	}
	// Condition not found, add it
	t.Status.Conditions = append(t.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: t.Generation,
	})
}

// GetCondition returns the condition with the given type, or nil if not found
func (t *Transform) GetCondition(condType string) *metav1.Condition {
	for i := range t.Status.Conditions {
		if t.Status.Conditions[i].Type == condType {
			return &t.Status.Conditions[i]
		}
	}
	return nil
}
