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

	// EnvFrom specifies sources to populate environment variables in the container
	// This allows referencing existing Secrets and ConfigMaps
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`
}

// EnvFromSource represents a source for environment variables
type EnvFromSource struct {
	// SecretRef references a Secret in the same namespace
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// ConfigMapRef references a ConfigMap in the same namespace
	// +optional
	ConfigMapRef *ConfigMapReference `json:"configMapRef,omitempty"`
}

// SecretReference contains enough information to locate a Secret
type SecretReference struct {
	// Name of the Secret
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ConfigMapReference contains enough information to locate a ConfigMap
type ConfigMapReference struct {
	// Name of the ConfigMap
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// WebServiceStatus defines the observed state of WebService.
type WebServiceStatus struct {
	// ResourceGraphRef references the current ResourceGraph created from this WebService
	// The ResourceGraph contains the rendered resources and execution state
	// +optional
	ResourceGraphRef *ObjectReference `json:"resourceGraphRef,omitempty"`

	// Conditions represent the current state of the WebService resource.
	// Condition types include:
	// - "Rendered": ResourceGraph created successfully
	// - "PolicyPassed": Policy validation succeeded
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
// +kubebuilder:resource:categories=transform

// WebService is the Schema for the webservices API
// WebService resources should have the label "pequod.io/transform: true" to be managed by the Transform controller
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
