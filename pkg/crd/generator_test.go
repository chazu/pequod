package crd

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestGenerator_GenerateCRD(t *testing.T) {
	generator := NewGenerator()

	inputSchema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"image": {
				Type: "string",
			},
			"port": {
				Type: "integer",
			},
			"replicas": {
				Type: "integer",
			},
		},
		Required: []string{"image", "port"},
	}

	config := GeneratorConfig{
		Group:         "apps.example.com",
		Version:       "v1",
		ShortNames:    []string{"ws"},
		TransformName: "webservice",
	}

	crd := generator.GenerateCRD("webservice", inputSchema, config)

	// Verify CRD metadata
	if crd.Name != "webservices.apps.example.com" {
		t.Errorf("expected CRD name 'webservices.apps.example.com', got %q", crd.Name)
	}

	// Verify group
	if crd.Spec.Group != "apps.example.com" {
		t.Errorf("expected group 'apps.example.com', got %q", crd.Spec.Group)
	}

	// Verify names
	if crd.Spec.Names.Kind != "WebService" {
		t.Errorf("expected kind 'WebService', got %q", crd.Spec.Names.Kind)
	}
	if crd.Spec.Names.Plural != "webservices" {
		t.Errorf("expected plural 'webservices', got %q", crd.Spec.Names.Plural)
	}
	if crd.Spec.Names.Singular != "webservice" {
		t.Errorf("expected singular 'webservice', got %q", crd.Spec.Names.Singular)
	}
	if len(crd.Spec.Names.ShortNames) != 1 || crd.Spec.Names.ShortNames[0] != "ws" {
		t.Errorf("expected short names ['ws'], got %v", crd.Spec.Names.ShortNames)
	}

	// Verify scope
	if crd.Spec.Scope != apiextensionsv1.NamespaceScoped {
		t.Errorf("expected namespaced scope, got %v", crd.Spec.Scope)
	}

	// Verify version
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(crd.Spec.Versions))
	}
	if crd.Spec.Versions[0].Name != "v1" {
		t.Errorf("expected version 'v1', got %q", crd.Spec.Versions[0].Name)
	}
	if !crd.Spec.Versions[0].Served {
		t.Error("expected version to be served")
	}
	if !crd.Spec.Versions[0].Storage {
		t.Error("expected version to be storage")
	}

	// Verify schema structure
	schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	if schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", schema.Type)
	}

	specProp, ok := schema.Properties["spec"]
	if !ok {
		t.Fatal("expected 'spec' property in schema")
	}

	imageProp, ok := specProp.Properties["image"]
	if !ok {
		t.Fatal("expected 'image' property in spec")
	}
	if imageProp.Type != "string" {
		t.Errorf("expected image type 'string', got %q", imageProp.Type)
	}

	// Verify status subresource
	if crd.Spec.Versions[0].Subresources == nil || crd.Spec.Versions[0].Subresources.Status == nil {
		t.Error("expected status subresource to be enabled")
	}

	// Verify labels
	if crd.Labels[ManagedByLabel] != "pequod" {
		t.Errorf("expected managed-by label 'pequod', got %q", crd.Labels[ManagedByLabel])
	}
	if crd.Labels[TransformLabel] != "webservice" {
		t.Errorf("expected transform label 'webservice', got %q", crd.Labels[TransformLabel])
	}
}

func TestGenerator_GenerateCRD_Defaults(t *testing.T) {
	generator := NewGenerator()

	inputSchema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
	}

	// Use empty config to test defaults
	config := GeneratorConfig{}

	crd := generator.GenerateCRD("myplatform", inputSchema, config)

	// Verify defaults
	if crd.Spec.Group != DefaultGroup {
		t.Errorf("expected default group %q, got %q", DefaultGroup, crd.Spec.Group)
	}
	if crd.Spec.Versions[0].Name != DefaultVersion {
		t.Errorf("expected default version %q, got %q", DefaultVersion, crd.Spec.Versions[0].Name)
	}
}

func TestGenerator_GetCRDName(t *testing.T) {
	generator := NewGenerator()

	tests := []struct {
		platformName string
		group        string
		expected     string
	}{
		{
			platformName: "webservice",
			group:        "apps.example.com",
			expected:     "webservices.apps.example.com",
		},
		{
			platformName: "database",
			group:        "",
			expected:     "databases." + DefaultGroup,
		},
		{
			platformName: "proxy",
			group:        "networking.io",
			expected:     "proxies.networking.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.platformName, func(t *testing.T) {
			result := generator.GetCRDName(tt.platformName, tt.group)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerator_GetGVK(t *testing.T) {
	generator := NewGenerator()

	config := GeneratorConfig{
		Group:   "apps.example.com",
		Version: "v1beta1",
	}

	gvk := generator.GetGVK("webservice", config)

	if gvk.Group != "apps.example.com" {
		t.Errorf("expected group 'apps.example.com', got %q", gvk.Group)
	}
	if gvk.Version != "v1beta1" {
		t.Errorf("expected version 'v1beta1', got %q", gvk.Version)
	}
	if gvk.Kind != "WebService" {
		t.Errorf("expected kind 'WebService', got %q", gvk.Kind)
	}
}

func TestToKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"webservice", "WebService"},
		{"database", "Database"},
		{"myapp", "Myapp"},
		{"api", "Api"},
		{"", ""},
		{"test-transform", "TestTransform"},
		{"my-web-app", "MyWebApp"},
		{"web-service", "WebService"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toKind(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestToPlural(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"webservice", "webservices"},
		{"database", "databases"},
		{"proxy", "proxies"},
		{"class", "classes"},
		{"app", "apps"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toPlural(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerator_GenerateCRD_StatusSchema(t *testing.T) {
	generator := NewGenerator()

	inputSchema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
	}

	config := GeneratorConfig{}
	crd := generator.GenerateCRD("test", inputSchema, config)

	schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	statusProp, ok := schema.Properties["status"]
	if !ok {
		t.Fatal("expected 'status' property in schema")
	}

	// Verify status has expected fields
	if _, ok := statusProp.Properties["phase"]; !ok {
		t.Error("expected 'phase' in status")
	}
	if _, ok := statusProp.Properties["conditions"]; !ok {
		t.Error("expected 'conditions' in status")
	}
	if _, ok := statusProp.Properties["resourceGraphRef"]; !ok {
		t.Error("expected 'resourceGraphRef' in status")
	}
	if _, ok := statusProp.Properties["observedGeneration"]; !ok {
		t.Error("expected 'observedGeneration' in status")
	}
}

func TestGenerator_GenerateCRD_PrinterColumns(t *testing.T) {
	generator := NewGenerator()

	inputSchema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
	}

	config := GeneratorConfig{}
	crd := generator.GenerateCRD("test", inputSchema, config)

	columns := crd.Spec.Versions[0].AdditionalPrinterColumns
	if len(columns) < 1 {
		t.Fatal("expected at least 1 printer column")
	}

	// Verify Age column
	found := false
	for _, col := range columns {
		if col.Name == "Age" {
			found = true
			if col.Type != "date" {
				t.Errorf("expected Age column type 'date', got %q", col.Type)
			}
		}
	}
	if !found {
		t.Error("expected 'Age' printer column")
	}
}
