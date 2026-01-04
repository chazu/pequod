package schema

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestExtractor_CueToJSONSchema_ScalarTypes(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	tests := []struct {
		name         string
		cueSource    string
		expectedType string
	}{
		{
			name:         "string type",
			cueSource:    `string`,
			expectedType: "string",
		},
		{
			name:         "int type",
			cueSource:    `int`,
			expectedType: "integer",
		},
		{
			name:         "float type",
			cueSource:    `float`,
			expectedType: "number",
		},
		{
			name:         "bool type",
			cueSource:    `bool`,
			expectedType: "boolean",
		},
		{
			name:         "number type",
			cueSource:    `number`,
			expectedType: "number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString(tt.cueSource)
			if v.Err() != nil {
				t.Fatalf("failed to compile CUE: %v", v.Err())
			}

			schema, err := extractor.CueToJSONSchema(v)
			if err != nil {
				t.Fatalf("failed to extract schema: %v", err)
			}

			if schema.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, schema.Type)
			}
		})
	}
}

func TestExtractor_CueToJSONSchema_StringConstraints(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	// Test non-empty string constraint
	v := ctx.CompileString(`string & !=""`)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "string" {
		t.Errorf("expected type string, got %q", schema.Type)
	}

	if schema.MinLength == nil || *schema.MinLength != 1 {
		t.Errorf("expected minLength 1 for non-empty string")
	}
}

func TestExtractor_CueToJSONSchema_NumericConstraints(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	// Test int with >= 0 constraint
	v := ctx.CompileString(`int & >=0`)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "integer" {
		t.Errorf("expected type integer, got %q", schema.Type)
	}

	if schema.Minimum == nil || *schema.Minimum != 0 {
		t.Errorf("expected minimum 0, got %v", schema.Minimum)
	}
}

func TestExtractor_CueToJSONSchema_StructType(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `{
		name: string
		port: int
		replicas?: int
	}`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("expected type object, got %q", schema.Type)
	}

	if len(schema.Properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(schema.Properties))
	}

	// Check required fields
	if len(schema.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d: %v", len(schema.Required), schema.Required)
	}

	// Verify replicas is not in required (it's optional)
	for _, req := range schema.Required {
		if req == "replicas" {
			t.Errorf("replicas should not be required")
		}
	}
}

func TestExtractor_CueToJSONSchema_ArrayType(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `[...string]`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "array" {
		t.Errorf("expected type array, got %q", schema.Type)
	}

	if schema.Items == nil || schema.Items.Schema == nil {
		t.Errorf("expected items schema to be set")
	}
}

func TestExtractor_CueToJSONSchema_EnumType(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `"small" | "medium" | "large"`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "string" {
		t.Errorf("expected type string for enum, got %q", schema.Type)
	}

	if len(schema.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(schema.Enum))
	}
}

func TestExtractor_CueToJSONSchema_DefaultValues(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `int | *3`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Default == nil {
		t.Errorf("expected default value to be set")
	}
}

func TestExtractor_CueToJSONSchema_NestedStruct(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `{
		name: string
		config: {
			timeout: int
			retries?: int
		}
	}`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("expected type object, got %q", schema.Type)
	}

	configProp, ok := schema.Properties["config"]
	if !ok {
		t.Fatal("expected config property")
	}

	if configProp.Type != "object" {
		t.Errorf("expected config to be object, got %q", configProp.Type)
	}

	if len(configProp.Properties) != 2 {
		t.Errorf("expected 2 config properties, got %d", len(configProp.Properties))
	}
}

func TestExtractor_ExtractInputSchema(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	// Test with #Input definition
	cueSource := `
#Input: {
	image: string & !=""
	port: int & >=1 & <=65535
	replicas?: int & >=0
}
`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.ExtractInputSchema(v)
	if err != nil {
		t.Fatalf("failed to extract input schema: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("expected type object, got %q", schema.Type)
	}

	// Verify required fields
	imageRequired := false
	portRequired := false
	for _, req := range schema.Required {
		if req == "image" {
			imageRequired = true
		}
		if req == "port" {
			portRequired = true
		}
	}

	if !imageRequired {
		t.Error("expected image to be required")
	}
	if !portRequired {
		t.Error("expected port to be required")
	}
}

func TestExtractor_ExtractInputSchema_Spec(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	// Test with #Spec definition (fallback)
	cueSource := `
#Spec: {
	name: string
	size: "small" | "medium" | "large"
}
`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.ExtractInputSchema(v)
	if err != nil {
		t.Fatalf("failed to extract input schema: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("expected type object, got %q", schema.Type)
	}

	sizeProp, ok := schema.Properties["size"]
	if !ok {
		t.Fatal("expected size property")
	}

	if len(sizeProp.Enum) != 3 {
		t.Errorf("expected 3 enum values for size, got %d", len(sizeProp.Enum))
	}
}

func TestExtractor_ExtractInputSchema_NotFound(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	// CUE without #Input or #Spec
	cueSource := `{
		foo: string
	}`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	_, err := extractor.ExtractInputSchema(v)
	if err == nil {
		t.Error("expected error when no #Input or #Spec found")
	}
}

func TestExtractor_CueToJSONSchema_ArrayOfStructs(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `[...{
		name: string
		value: string
	}]`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "array" {
		t.Errorf("expected type array, got %q", schema.Type)
	}
}

func TestExtractor_CueToJSONSchema_IntegerEnum(t *testing.T) {
	ctx := cuecontext.New()
	extractor := NewExtractor()

	cueSource := `1 | 2 | 3`

	v := ctx.CompileString(cueSource)
	if v.Err() != nil {
		t.Fatalf("failed to compile CUE: %v", v.Err())
	}

	schema, err := extractor.CueToJSONSchema(v)
	if err != nil {
		t.Fatalf("failed to extract schema: %v", err)
	}

	if schema.Type != "integer" {
		t.Errorf("expected type integer for int enum, got %q", schema.Type)
	}

	if len(schema.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(schema.Enum))
	}
}
