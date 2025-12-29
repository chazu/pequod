package platformloader

import (
	"context"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

// Render converts a WebService spec to a Graph artifact using CUE evaluation
func (r *Renderer) Render(ctx context.Context, name, namespace, image string, port int32, replicas *int32, platformRef string) (*graph.Graph, error) {
	// Load the CUE module
	cueValue, err := r.loader.LoadEmbedded(platformRef)
	if err != nil {
		return nil, fmt.Errorf("failed to load CUE module: %w", err)
	}

	// Build the input structure
	input := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"image": image,
			"port":  port,
		},
	}

	if replicas != nil {
		input["spec"].(map[string]interface{})["replicas"] = *replicas
	}

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
