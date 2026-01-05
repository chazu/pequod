// Package crd provides utilities for generating Kubernetes CustomResourceDefinitions
// from extracted JSONSchema. This is used by the Transform controller to dynamically
// create CRDs for platform abstractions.
package crd

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultGroup is the default API group for generated CRDs
	DefaultGroup = "pequod.io"

	// DefaultVersion is the default API version for generated CRDs
	DefaultVersion = "v1alpha1"

	// FieldManager is the field manager name used for SSA
	FieldManager = "pequod-crd-generator"

	// ManagedByLabel identifies CRDs managed by Pequod
	ManagedByLabel = "app.kubernetes.io/managed-by"

	// TransformLabel links a CRD to its source Transform
	TransformLabel = "pequod.io/transform"
)

// GeneratorConfig contains configuration for CRD generation
type GeneratorConfig struct {
	// Group is the API group (default: pequod.io)
	Group string

	// Version is the API version (default: v1alpha1)
	Version string

	// ShortNames are optional short names for the CRD
	ShortNames []string

	// Categories are optional categories for the CRD
	Categories []string

	// TransformName is the name of the source Transform
	TransformName string

	// TransformNamespace is the namespace of the source Transform
	TransformNamespace string
}

// Generator creates Kubernetes CRDs from extracted schemas
type Generator struct{}

// NewGenerator creates a new CRD generator
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateCRD creates a CustomResourceDefinition from a platform name and schema.
// The platformName should be lowercase (e.g., "webservice").
// The kind will be derived by capitalizing appropriately (e.g., "WebService").
func (g *Generator) GenerateCRD(platformName string, inputSchema *apiextensionsv1.JSONSchemaProps, config GeneratorConfig) *apiextensionsv1.CustomResourceDefinition {
	// Apply defaults
	group := config.Group
	if group == "" {
		group = DefaultGroup
	}

	version := config.Version
	if version == "" {
		version = DefaultVersion
	}

	// Derive names
	kind := toKind(platformName)
	plural := toPlural(platformName)
	singular := strings.ToLower(platformName)

	// Build the full OpenAPI schema including metadata
	openAPISchema := buildOpenAPISchema(inputSchema)

	// Build labels
	labels := map[string]string{
		ManagedByLabel: "pequod",
	}
	if config.TransformName != "" {
		labels[TransformLabel] = config.TransformName
	}

	crd := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("%s.%s", plural, group),
			Labels: labels,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:       kind,
				Plural:     plural,
				Singular:   singular,
				ShortNames: config.ShortNames,
				Categories: config.Categories,
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    version,
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: openAPISchema,
					},
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:     "Age",
							Type:     "date",
							JSONPath: ".metadata.creationTimestamp",
						},
					},
				},
			},
		},
	}

	return crd
}

// buildOpenAPISchema wraps the input schema in the full CRD OpenAPI schema structure
func buildOpenAPISchema(inputSchema *apiextensionsv1.JSONSchemaProps) *apiextensionsv1.JSONSchemaProps {
	return &apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"apiVersion": {
				Type: "string",
			},
			"kind": {
				Type: "string",
			},
			"metadata": {
				Type: "object",
			},
			"spec": *inputSchema,
			"status": {
				Type:                   "object",
				XPreserveUnknownFields: boolPtr(true),
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"phase": {
						Type:        "string",
						Description: "Current phase of the resource",
					},
					"conditions": {
						Type: "array",
						Items: &apiextensionsv1.JSONSchemaPropsOrArray{
							Schema: &apiextensionsv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"type": {
										Type: "string",
									},
									"status": {
										Type: "string",
									},
									"reason": {
										Type: "string",
									},
									"message": {
										Type: "string",
									},
									"lastTransitionTime": {
										Type:   "string",
										Format: "date-time",
									},
								},
								Required: []string{"type", "status"},
							},
						},
					},
					"resourceGraphRef": {
						Type:        "object",
						Description: "Reference to the ResourceGraph managing this instance",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"name": {
								Type: "string",
							},
							"namespace": {
								Type: "string",
							},
						},
					},
					"observedGeneration": {
						Type:        "integer",
						Format:      "int64",
						Description: "The generation observed by the controller",
					},
				},
			},
		},
	}
}

// ApplyCRD applies a CRD to the cluster using Server-Side Apply.
// It returns the applied CRD and any error.
func (g *Generator) ApplyCRD(ctx context.Context, c client.Client, crd *apiextensionsv1.CustomResourceDefinition) error {
	// Use Server-Side Apply
	if err := c.Patch(ctx, crd, client.Apply, client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply CRD: %w", err)
	}

	return nil
}

// DeleteCRD deletes a CRD from the cluster.
func (g *Generator) DeleteCRD(ctx context.Context, c client.Client, crdName string) error {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
		},
	}

	if err := c.Delete(ctx, crd); err != nil {
		return fmt.Errorf("failed to delete CRD: %w", err)
	}

	return nil
}

// GetCRDName returns the CRD name for a given platform name and group.
func (g *Generator) GetCRDName(platformName string, group string) string {
	if group == "" {
		group = DefaultGroup
	}
	return fmt.Sprintf("%s.%s", toPlural(platformName), group)
}

// GetGVK returns the GroupVersionKind for a generated CRD.
func (g *Generator) GetGVK(platformName string, config GeneratorConfig) schema.GroupVersionKind {
	group := config.Group
	if group == "" {
		group = DefaultGroup
	}

	version := config.Version
	if version == "" {
		version = DefaultVersion
	}

	return schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    toKind(platformName),
	}
}

// toKind converts a lowercase platform name to a CamelCase kind.
// Examples: "webservice" -> "WebService", "myapp" -> "Myapp"
func toKind(name string) string {
	if name == "" {
		return ""
	}

	// Handle common compound words first (before capitalization)
	name = strings.ToLower(name)

	// Map of known compound words to their CamelCase form
	compounds := map[string]string{
		"webservice":   "WebService",
		"messagequeue": "MessageQueue",
		"apigateway":   "APIGateway",
		"loadbalancer": "LoadBalancer",
	}

	// Check if the name matches a known compound word (without hyphens)
	nameNoHyphens := strings.ReplaceAll(name, "-", "")
	if camelCase, ok := compounds[nameNoHyphens]; ok {
		return camelCase
	}

	// Split on hyphens and capitalize each part
	parts := strings.Split(name, "-")
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}
	return result.String()
}

// toPlural converts a name to its plural form.
// This is a simple implementation - for production, use a proper pluralizer.
func toPlural(name string) string {
	name = strings.ToLower(name)

	// Handle common patterns
	switch {
	case strings.HasSuffix(name, "s"):
		return name + "es"
	case strings.HasSuffix(name, "y"):
		return name[:len(name)-1] + "ies"
	default:
		return name + "s"
	}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
