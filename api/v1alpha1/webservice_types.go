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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebServiceSpec defines the desired state of WebService
type WebServiceSpec struct {
	// Image is the container image to deploy
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Replicas is the number of replicas to deploy
	// If not specified, the default from the platform policy will be used
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Port is the service port to expose
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// PlatformRef references the platform module to use for rendering
	// For now, only embedded version is supported
	// +optional
	// +kubebuilder:default="embedded"
	PlatformRef string `json:"platformRef,omitempty"`
}

// WebServiceStatus defines the observed state of WebService.
type WebServiceStatus struct {
	// Conditions represent the current state of the WebService resource.
	// Condition types include:
	// - "Rendered": Graph artifact created successfully
	// - "PolicyPassed": Policy validation succeeded
	// - "Applying": Resources are being applied
	// - "Ready": All resources are ready
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Inventory tracks all resources managed by this WebService
	// +optional
	Inventory []InventoryItem `json:"inventory,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// InventoryItem represents a single managed resource
type InventoryItem struct {
	// NodeID is the identifier of the node in the Graph artifact
	NodeID string `json:"nodeId"`

	// Group is the API group of the resource
	// +optional
	Group string `json:"group,omitempty"`

	// Version is the API version of the resource
	Version string `json:"version"`

	// Kind is the kind of the resource
	Kind string `json:"kind"`

	// Namespace is the namespace of the resource (empty for cluster-scoped)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name is the name of the resource
	Name string `json:"name"`

	// UID is the UID of the resource
	// +optional
	UID string `json:"uid,omitempty"`

	// Mode indicates how the resource is managed
	// +kubebuilder:validation:Enum=Managed;Adopted;Orphaned
	Mode string `json:"mode"`

	// LastAppliedHash is the hash of the last applied configuration
	// +optional
	LastAppliedHash string `json:"lastAppliedHash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WebService is the Schema for the webservices API
type WebService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WebService
	// +required
	Spec WebServiceSpec `json:"spec"`

	// status defines the observed state of WebService
	// +optional
	Status WebServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WebServiceList contains a list of WebService
type WebServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WebService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebService{}, &WebServiceList{})
}
