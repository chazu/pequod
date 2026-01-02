package platformloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/chazu/pequod/pkg/graph"
)

// Renderer converts CUE evaluation results to Graph artifacts
type Renderer struct {
	loader *Loader
}

// NewRenderer creates a new renderer with the given loader
func NewRenderer(loader *Loader) *Renderer {
	return &Renderer{
		loader: loader,
	}
}

// RenderTransform renders a Transform's input through a CUE platform module
// This is the generic rendering method that works with any platform type.
// The input is expected to contain the platform-specific "spec" fields.
// Metadata (name, namespace) is provided separately and injected into the CUE input.
func (r *Renderer) RenderTransform(ctx context.Context, name, namespace string, rawInput runtime.RawExtension, platformRef string) (*graph.Graph, error) {
	// Load the CUE module based on platformRef
	// For now, we only support embedded modules
	cueValue, err := r.loader.LoadEmbedded(platformRef)
	if err != nil {
		return nil, fmt.Errorf("failed to load CUE module: %w", err)
	}

	// Parse the raw input using json.Decoder with UseNumber() to preserve integer types
	// Standard json.Unmarshal converts all numbers to float64, which causes CUE type errors
	var specInput map[string]interface{}
	if len(rawInput.Raw) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(rawInput.Raw))
		decoder.UseNumber()
		if err := decoder.Decode(&specInput); err != nil {
			return nil, fmt.Errorf("failed to parse input: %w", err)
		}
		// Convert json.Number to native types for CUE compatibility
		specInput = convertJSONNumbers(specInput).(map[string]interface{})
	} else {
		specInput = make(map[string]interface{})
	}

	// Inject platformRef into spec for CUE template to use
	specInput["platformRef"] = platformRef

	// Build the CUE input structure
	// CUE expects: { metadata: { name, namespace }, spec: { ...platform-specific... } }
	input := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": specInput,
	}

	return r.renderWithInput(cueValue, input)
}

// renderWithInput is the internal rendering method that takes a loaded CUE value and input
func (r *Renderer) renderWithInput(cueValue cue.Value, input map[string]interface{}) (*graph.Graph, error) {
	// Fill the #Render template with our input
	renderDef := cueValue.LookupPath(cue.ParsePath("#Render"))
	if !renderDef.Exists() {
		return nil, fmt.Errorf("#Render definition not found in CUE module")
	}

	// Unify the input with the render definition
	inputValue := r.loader.ctx.Encode(input)
	filled := renderDef.FillPath(cue.ParsePath("input"), inputValue)
	if filled.Err() != nil {
		return nil, fmt.Errorf("failed to fill input: %w", filled.Err())
	}

	// Extract the output
	output := filled.LookupPath(cue.ParsePath("output"))
	if !output.Exists() {
		return nil, fmt.Errorf("output not found in rendered CUE")
	}

	if output.Err() != nil {
		return nil, fmt.Errorf("output has errors: %w", output.Err())
	}

	// Convert CUE output to Graph
	g, err := r.cueValueToGraph(output)
	if err != nil {
		return nil, fmt.Errorf("failed to convert CUE to Graph: %w", err)
	}

	// Validate the graph
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("rendered graph is invalid: %w", err)
	}

	return g, nil
}

// cueValueToGraph converts a CUE value to a Graph struct
func (r *Renderer) cueValueToGraph(v cue.Value) (*graph.Graph, error) {
	// Convert CUE value to JSON
	jsonBytes, err := v.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CUE to JSON: %w", err)
	}

	// Unmarshal into a temporary structure
	var temp struct {
		Metadata   graph.GraphMetadata `json:"metadata"`
		Nodes      []json.RawMessage   `json:"nodes"`
		Violations []graph.Violation   `json:"violations"`
	}

	if err := json.Unmarshal(jsonBytes, &temp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal graph structure: %w", err)
	}

	// Convert nodes
	nodes := make([]graph.Node, len(temp.Nodes))
	for i, nodeJSON := range temp.Nodes {
		node, err := r.parseNode(nodeJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node %d: %w", i, err)
		}
		nodes[i] = node
	}

	return &graph.Graph{
		Metadata:   temp.Metadata,
		Nodes:      nodes,
		Violations: temp.Violations,
	}, nil
}

// convertJSONNumbers recursively converts json.Number values to native Go types
// This preserves integer types when possible, which is required for CUE type checking
func convertJSONNumbers(v interface{}) interface{} {
	switch val := v.(type) {
	case json.Number:
		// Try to parse as int first
		if i, err := strconv.ParseInt(string(val), 10, 64); err == nil {
			return i
		}
		// Fall back to float
		if f, err := strconv.ParseFloat(string(val), 64); err == nil {
			return f
		}
		return val
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v := range val {
			result[k] = convertJSONNumbers(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertJSONNumbers(v)
		}
		return result
	default:
		return v
	}
}

// parseNode converts a JSON node to a graph.Node
func (r *Renderer) parseNode(nodeJSON json.RawMessage) (graph.Node, error) {
	var temp struct {
		ID          string                     `json:"id"`
		Object      map[string]interface{}     `json:"object"`
		ApplyPolicy graph.ApplyPolicy          `json:"applyPolicy"`
		DependsOn   []string                   `json:"dependsOn"`
		ReadyWhen   []graph.ReadinessPredicate `json:"readyWhen"`
	}

	if err := json.Unmarshal(nodeJSON, &temp); err != nil {
		return graph.Node{}, fmt.Errorf("failed to unmarshal node: %w", err)
	}

	// Convert object map to unstructured.Unstructured
	obj := unstructured.Unstructured{Object: temp.Object}

	return graph.Node{
		ID:          temp.ID,
		Object:      obj,
		ApplyPolicy: temp.ApplyPolicy,
		DependsOn:   temp.DependsOn,
		ReadyWhen:   temp.ReadyWhen,
	}, nil
}
