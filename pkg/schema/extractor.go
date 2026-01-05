// Package schema provides utilities for extracting JSONSchema from CUE definitions.
// This is used to generate Kubernetes CRD schemas from CUE platform modules.
package schema

import (
	"fmt"
	"math"
	"strings"

	"cuelang.org/go/cue"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Extractor converts CUE values to JSONSchema for CRD generation
type Extractor struct{}

// NewExtractor creates a new schema extractor
func NewExtractor() *Extractor {
	return &Extractor{}
}

// ExtractInputSchema extracts the input schema from a CUE module.
// It looks for #Input or #Spec definitions and converts them to JSONSchema.
func (e *Extractor) ExtractInputSchema(cueValue cue.Value) (*apiextensionsv1.JSONSchemaProps, error) {
	// Try to find #Input first, then fall back to #Spec
	inputDef := cueValue.LookupPath(cue.ParsePath("#Input"))
	if !inputDef.Exists() {
		inputDef = cueValue.LookupPath(cue.ParsePath("#Spec"))
	}

	if !inputDef.Exists() {
		return nil, fmt.Errorf("no #Input or #Spec definition found in CUE module")
	}

	if inputDef.Err() != nil {
		return nil, fmt.Errorf("error in input definition: %w", inputDef.Err())
	}

	return e.CueToJSONSchema(inputDef)
}

// CueToJSONSchema converts a CUE value to JSONSchema
func (e *Extractor) CueToJSONSchema(v cue.Value) (*apiextensionsv1.JSONSchemaProps, error) {
	schema := &apiextensionsv1.JSONSchemaProps{}

	// Check for errors in the value
	if v.Err() != nil {
		return nil, fmt.Errorf("CUE value has errors: %w", v.Err())
	}

	// Check if it's a disjunction (enum) first - before checking kind
	if op, args := v.Expr(); op == cue.OrOp && len(args) > 0 {
		return e.extractDisjunction(args)
	}

	// Get the kind of the CUE value
	kind := v.IncompleteKind()

	switch {
	case kind == cue.BottomKind:
		return nil, fmt.Errorf("bottom value (error) in CUE")

	case kind == cue.NullKind:
		schema.Type = "null"

	case kind == cue.BoolKind:
		schema.Type = "boolean"
		if def, ok := v.Default(); ok && def.Err() == nil {
			b, _ := def.Bool()
			schema.Default = &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf("%t", b))}
		}

	case kind == cue.IntKind:
		schema.Type = "integer"
		e.extractNumericConstraints(v, schema)

	case kind == cue.FloatKind:
		schema.Type = "number"
		e.extractNumericConstraints(v, schema)

	case kind == cue.NumberKind:
		// Could be int or float - default to number
		schema.Type = "number"
		e.extractNumericConstraints(v, schema)

	case kind == cue.StringKind:
		schema.Type = "string"
		e.extractStringConstraints(v, schema)

	case kind == cue.BytesKind:
		schema.Type = "string"
		schema.Format = "byte"

	case kind == cue.ListKind:
		schema.Type = "array"
		if err := e.extractArraySchema(v, schema); err != nil {
			return nil, err
		}

	case kind == cue.StructKind:
		schema.Type = "object"
		if err := e.extractStructSchema(v, schema); err != nil {
			return nil, err
		}

	case kind&cue.IntKind != 0 && kind&cue.FloatKind != 0:
		// Union of int and float - use number
		schema.Type = "number"
		e.extractNumericConstraints(v, schema)

	default:
		// For other complex types, try to infer from the concrete kind
		if v.IsConcrete() {
			concreteKind := v.Kind()
			switch concreteKind {
			case cue.BoolKind:
				schema.Type = "boolean"
			case cue.IntKind:
				schema.Type = "integer"
			case cue.FloatKind, cue.NumberKind:
				schema.Type = "number"
			case cue.StringKind:
				schema.Type = "string"
			case cue.StructKind:
				schema.Type = "object"
				if err := e.extractStructSchema(v, schema); err != nil {
					return nil, err
				}
			case cue.ListKind:
				schema.Type = "array"
				if err := e.extractArraySchema(v, schema); err != nil {
					return nil, err
				}
			default:
				// Fallback - allow any type
				schema.XPreserveUnknownFields = boolPtr(true)
			}
		} else {
			// Non-concrete value - try to handle as struct if it has fields
			iter, err := v.Fields(cue.Optional(true))
			if err == nil && iter.Next() {
				schema.Type = "object"
				if err := e.extractStructSchema(v, schema); err != nil {
					return nil, err
				}
			} else {
				// Fallback - allow any type
				schema.XPreserveUnknownFields = boolPtr(true)
			}
		}
	}

	// Extract description from CUE comments if available
	for _, doc := range v.Doc() {
		if doc.Text() != "" {
			if schema.Description != "" {
				schema.Description += "\n"
			}
			schema.Description += strings.TrimSpace(doc.Text())
		}
	}

	return schema, nil
}

// extractNumericConstraints extracts min/max constraints from CUE numeric types
func (e *Extractor) extractNumericConstraints(v cue.Value, schema *apiextensionsv1.JSONSchemaProps) {
	// Extract default value
	if def, ok := v.Default(); ok && def.Err() == nil {
		if i, err := def.Int64(); err == nil {
			schema.Default = &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf("%d", i))}
		} else if f, err := def.Float64(); err == nil {
			schema.Default = &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf("%g", f))}
		}
	}

	// Try to extract bounds using CUE's validation
	// Check for minimum by testing against MinInt64
	testMin := v.Context().Encode(int64(math.MinInt64))
	if unified := v.Unify(testMin); unified.Err() != nil {
		// There's a lower bound - try to find it
		// This is a heuristic - we test common bounds
		for _, bound := range []int64{0, 1, -1} {
			testVal := v.Context().Encode(bound)
			if unified := v.Unify(testVal); unified.Err() == nil {
				// This value is valid
				minVal := float64(bound)
				schema.Minimum = &minVal
				break
			}
		}
	}

	// Check for maximum similarly
	testMax := v.Context().Encode(int64(math.MaxInt64))
	if unified := v.Unify(testMax); unified.Err() != nil {
		// There's an upper bound
		for _, bound := range []int64{65535, 100, 10} {
			testVal := v.Context().Encode(bound)
			if unified := v.Unify(testVal); unified.Err() == nil {
				maxVal := float64(bound)
				schema.Maximum = &maxVal
				break
			}
		}
	}
}

// extractStringConstraints extracts constraints from CUE string types
func (e *Extractor) extractStringConstraints(v cue.Value, schema *apiextensionsv1.JSONSchemaProps) {
	// Extract default value
	if def, ok := v.Default(); ok && def.Err() == nil {
		if s, err := def.String(); err == nil {
			schema.Default = &apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf("%q", s))}
		}
	}

	// Check for non-empty constraint (string & !="")
	emptyStr := v.Context().Encode("")
	if unified := v.Unify(emptyStr); unified.Err() != nil {
		// Empty string is not valid - set minLength
		minLen := int64(1)
		schema.MinLength = &minLen
	}
}

// extractArraySchema extracts schema for array/list types
func (e *Extractor) extractArraySchema(v cue.Value, schema *apiextensionsv1.JSONSchemaProps) error {
	// Try to get the element type
	iter, err := v.List()
	if err == nil && iter.Next() {
		itemSchema, err := e.CueToJSONSchema(iter.Value())
		if err != nil {
			return fmt.Errorf("failed to extract array item schema: %w", err)
		}
		schema.Items = &apiextensionsv1.JSONSchemaPropsOrArray{
			Schema: itemSchema,
		}
	} else {
		// Check if there's a type constraint on the list elements
		// Look for [...T] pattern
		op, args := v.Expr()
		if op == cue.SelectorOp && len(args) > 0 {
			itemSchema, err := e.CueToJSONSchema(args[0])
			if err == nil {
				schema.Items = &apiextensionsv1.JSONSchemaPropsOrArray{
					Schema: itemSchema,
				}
			}
		} else {
			// Allow any items
			schema.Items = &apiextensionsv1.JSONSchemaPropsOrArray{
				Schema: &apiextensionsv1.JSONSchemaProps{
					XPreserveUnknownFields: boolPtr(true),
				},
			}
		}
	}

	return nil
}

// extractStructSchema extracts schema for struct types
func (e *Extractor) extractStructSchema(v cue.Value, schema *apiextensionsv1.JSONSchemaProps) error {
	schema.Properties = make(map[string]apiextensionsv1.JSONSchemaProps)
	var required []string

	// Iterate over all fields, including optional ones
	iter, err := v.Fields(cue.Optional(true))
	if err != nil {
		return fmt.Errorf("failed to iterate struct fields: %w", err)
	}

	for iter.Next() {
		fieldName := iter.Selector().String()
		fieldValue := iter.Value()

		// Strip the optional marker '?' suffix from field names
		// CUE's Selector().String() includes this for optional fields
		fieldName = strings.TrimSuffix(fieldName, "?")

		// Skip hidden fields (starting with _)
		if strings.HasPrefix(fieldName, "_") {
			continue
		}

		// Skip definition fields (starting with #)
		if strings.HasPrefix(fieldName, "#") {
			continue
		}

		// Extract field schema
		fieldSchema, err := e.CueToJSONSchema(fieldValue)
		if err != nil {
			return fmt.Errorf("failed to extract schema for field %s: %w", fieldName, err)
		}

		schema.Properties[fieldName] = *fieldSchema

		// Check if field is required (not optional)
		if !iter.IsOptional() {
			required = append(required, fieldName)
		}
	}

	if len(required) > 0 {
		schema.Required = required
	}

	return nil
}

// extractDisjunction handles CUE disjunctions (enums or union types)
func (e *Extractor) extractDisjunction(args []cue.Value) (*apiextensionsv1.JSONSchemaProps, error) {
	// Check if all values are concrete (enum case)
	allConcrete := true
	var enumValues []string
	var firstKind cue.Kind

	for i, arg := range args {
		if !arg.IsConcrete() {
			allConcrete = false
			break
		}

		if i == 0 {
			firstKind = arg.Kind()
		}

		// For enums, collect the string representation
		switch arg.Kind() {
		case cue.StringKind:
			if s, err := arg.String(); err == nil {
				enumValues = append(enumValues, s)
			}
		case cue.IntKind:
			if n, err := arg.Int64(); err == nil {
				enumValues = append(enumValues, fmt.Sprintf("%d", n))
			}
		default:
			allConcrete = false
		}
	}

	if allConcrete && len(enumValues) > 0 {
		// It's an enum
		schema := &apiextensionsv1.JSONSchemaProps{}
		switch firstKind {
		case cue.StringKind:
			schema.Type = "string"
		case cue.IntKind:
			schema.Type = "integer"
		default:
			schema.Type = "string"
		}

		// Convert to JSON enum format
		for _, val := range enumValues {
			var jsonVal []byte
			if firstKind == cue.StringKind {
				jsonVal = []byte(fmt.Sprintf("%q", val))
			} else {
				jsonVal = []byte(val)
			}
			schema.Enum = append(schema.Enum, apiextensionsv1.JSON{Raw: jsonVal})
		}

		return schema, nil
	}

	// Not a simple enum - use oneOf
	schema := &apiextensionsv1.JSONSchemaProps{
		OneOf: make([]apiextensionsv1.JSONSchemaProps, 0, len(args)),
	}

	for _, arg := range args {
		argSchema, err := e.CueToJSONSchema(arg)
		if err != nil {
			continue // Skip values we can't convert
		}
		schema.OneOf = append(schema.OneOf, *argSchema)
	}

	if len(schema.OneOf) == 0 {
		// Fallback
		return &apiextensionsv1.JSONSchemaProps{
			XPreserveUnknownFields: boolPtr(true),
		}, nil
	}

	return schema, nil
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
