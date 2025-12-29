package graph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildDAG(t *testing.T) {
	tests := []struct {
		name    string
		graph   *Graph
		wantErr bool
	}{
		{
			name: "simple linear dependency",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "a",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "a"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
					},
					{
						ID: "b",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "b"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"a"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "diamond dependency",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "a",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "a"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
					},
					{
						ID: "b",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "b"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"a"},
					},
					{
						ID: "c",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "c"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"a"},
					},
					{
						ID: "d",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "d"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"b", "c"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cycle detection",
			graph: &Graph{
				Metadata: GraphMetadata{
					Name:    "test",
					Version: "v1",
				},
				Nodes: []Node{
					{
						ID: "a",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "a"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"b"},
					},
					{
						ID: "b",
						Object: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata":   map[string]interface{}{"name": "b"},
							},
						},
						ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
						DependsOn:   []string{"a"},
					},
				},
			},
			wantErr: true,
		},
		{
			name:    "nil graph",
			graph:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag, err := BuildDAG(tt.graph)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildDAG() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && dag == nil {
				t.Error("BuildDAG() returned nil DAG without error")
			}
		})
	}
}

func TestDAGOperations(t *testing.T) {
	// Create a test graph with known structure
	// a -> b -> d
	// a -> c -> d
	g := &Graph{
		Metadata: GraphMetadata{
			Name:    "test",
			Version: "v1",
		},
		Nodes: []Node{
			{
				ID: "a",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "a"},
					},
				},
				ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
			},
			{
				ID: "b",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "b"},
					},
				},
				ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
				DependsOn:   []string{"a"},
			},
			{
				ID: "c",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "c"},
					},
				},
				ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
				DependsOn:   []string{"a"},
			},
			{
				ID: "d",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "d"},
					},
				},
				ApplyPolicy: ApplyPolicy{Mode: ApplyModeApply},
				DependsOn:   []string{"b", "c"},
			},
		},
	}

	dag, err := BuildDAG(g)
	if err != nil {
		t.Fatalf("BuildDAG() failed: %v", err)
	}

	t.Run("GetNode", func(t *testing.T) {
		node, found := dag.GetNode("a")
		if !found {
			t.Error("GetNode('a') not found")
		}
		if node.ID != "a" {
			t.Errorf("GetNode('a') returned wrong node: %s", node.ID)
		}

		_, found = dag.GetNode("nonexistent")
		if found {
			t.Error("GetNode('nonexistent') should not be found")
		}
	})

	t.Run("GetOrder", func(t *testing.T) {
		order := dag.GetOrder()
		if len(order) != 4 {
			t.Errorf("GetOrder() returned %d nodes, want 4", len(order))
		}

		// Verify topological order: a must come before b, c, d
		aIndex := -1
		bIndex := -1
		cIndex := -1
		dIndex := -1
		for i, id := range order {
			switch id {
			case "a":
				aIndex = i
			case "b":
				bIndex = i
			case "c":
				cIndex = i
			case "d":
				dIndex = i
			}
		}

		if aIndex > bIndex || aIndex > cIndex || aIndex > dIndex {
			t.Error("'a' must come before 'b', 'c', and 'd' in topological order")
		}
		if bIndex > dIndex || cIndex > dIndex {
			t.Error("'b' and 'c' must come before 'd' in topological order")
		}
	})

	t.Run("GetDependencies", func(t *testing.T) {
		deps, err := dag.GetDependencies("d")
		if err != nil {
			t.Errorf("GetDependencies('d') failed: %v", err)
		}
		if len(deps) != 2 {
			t.Errorf("GetDependencies('d') returned %d deps, want 2", len(deps))
		}

		_, err = dag.GetDependencies("nonexistent")
		if err == nil {
			t.Error("GetDependencies('nonexistent') should return error")
		}
	})

	t.Run("GetDependents", func(t *testing.T) {
		dependents, err := dag.GetDependents("a")
		if err != nil {
			t.Errorf("GetDependents('a') failed: %v", err)
		}
		if len(dependents) != 2 {
			t.Errorf("GetDependents('a') returned %d dependents, want 2", len(dependents))
		}

		dependents, err = dag.GetDependents("d")
		if err != nil {
			t.Errorf("GetDependents('d') failed: %v", err)
		}
		if len(dependents) != 0 {
			t.Errorf("GetDependents('d') returned %d dependents, want 0", len(dependents))
		}
	})

	t.Run("Size", func(t *testing.T) {
		if dag.Size() != 4 {
			t.Errorf("Size() = %d, want 4", dag.Size())
		}
	})

	t.Run("HasCycles", func(t *testing.T) {
		if dag.HasCycles() {
			t.Error("HasCycles() = true, want false")
		}
	})

	t.Run("GetRootNodes", func(t *testing.T) {
		roots := dag.GetRootNodes()
		if len(roots) != 1 {
			t.Errorf("GetRootNodes() returned %d roots, want 1", len(roots))
		}
		if len(roots) > 0 && roots[0] != "a" {
			t.Errorf("GetRootNodes() = %v, want ['a']", roots)
		}
	})

	t.Run("GetLeafNodes", func(t *testing.T) {
		leaves := dag.GetLeafNodes()
		if len(leaves) != 1 {
			t.Errorf("GetLeafNodes() returned %d leaves, want 1", len(leaves))
		}
		if len(leaves) > 0 && leaves[0] != "d" {
			t.Errorf("GetLeafNodes() = %v, want ['d']", leaves)
		}
	})
}
