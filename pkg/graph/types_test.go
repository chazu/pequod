package graph

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGraphValidation(t *testing.T) {
	tests := []struct {
		name    string
		graph   *Graph
		wantErr bool
	}{
		{
			name: "valid graph",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test-graph",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "node1",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name": "test-cm",
								},
							},
						},
						ApplyPolicy: ApplyPolicy{
							Mode: ApplyModeApply,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing metadata name",
			graph: &Graph{
				Metadata: GraphMetadata{
					Version: "v1",
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate node IDs",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test-graph",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "node1",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name": "test-cm",
								},
							},
						},
					},
					{
						ID: "node1",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name": "test-secret",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "non-existent dependency",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test-graph",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "node1",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name": "test-cm",
								},
							},
						},
						DependsOn: []string{"non-existent"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.graph.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Graph.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGraphSerialization(t *testing.T) {
	graph := &Graph{
		Metadata: GraphMetadata{
			Name:    "test-graph",
			Version: "v1",
		},
		Nodes: []Node{
			{
				ID: "node1",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "test-cm",
						},
					},
				},
				ApplyPolicy: ApplyPolicy{
					Mode: ApplyModeApply,
				},
			},
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("Failed to marshal graph: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled Graph
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal graph: %v", err)
	}

	if unmarshaled.Metadata.Name != graph.Metadata.Name {
		t.Errorf("Unmarshaled graph name = %v, want %v", unmarshaled.Metadata.Name, graph.Metadata.Name)
	}
}
