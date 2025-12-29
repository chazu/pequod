package graph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestComputeHash(t *testing.T) {
	g := &Graph{
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
							"name": "test",
						},
					},
				},
			},
		},
	}

	hash1 := g.ComputeHash()
	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}

	// Same graph should produce same hash
	hash2 := g.ComputeHash()
	if hash1 != hash2 {
		t.Errorf("Expected same hash for same graph, got %s and %s", hash1, hash2)
	}

	// Different graph should produce different hash
	g.Nodes[0].Object.Object["metadata"].(map[string]interface{})["name"] = "different"
	hash3 := g.ComputeHash()
	if hash1 == hash3 {
		t.Error("Expected different hash for different graph")
	}
}

func TestSetHash(t *testing.T) {
	g := &Graph{
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
					},
				},
			},
		},
	}

	// Initially no hash
	if g.Metadata.RenderHash != "" {
		t.Error("Expected empty hash initially")
	}

	// Set hash
	g.SetHash()
	if g.Metadata.RenderHash == "" {
		t.Error("Expected non-empty hash after SetHash()")
	}

	// Hash should match computed hash
	if g.Metadata.RenderHash != g.ComputeHash() {
		t.Error("RenderHash should match ComputeHash()")
	}
}

func TestHasChanged(t *testing.T) {
	g := &Graph{
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
							"name": "test",
						},
					},
				},
			},
		},
	}

	// No previous hash means changed
	if !g.HasChanged("") {
		t.Error("Expected HasChanged(true) with empty previous hash")
	}

	// Same hash means not changed
	currentHash := g.ComputeHash()
	if g.HasChanged(currentHash) {
		t.Error("Expected HasChanged(false) with same hash")
	}

	// Different hash means changed
	if !g.HasChanged("different-hash") {
		t.Error("Expected HasChanged(true) with different hash")
	}

	// Modify graph
	g.Nodes[0].Object.Object["metadata"].(map[string]interface{})["name"] = "modified"
	if !g.HasChanged(currentHash) {
		t.Error("Expected HasChanged(true) after modification")
	}
}

func TestHashIgnoresMetadata(t *testing.T) {
	g1 := &Graph{
		Metadata: GraphMetadata{
			Name:        "graph1",
			Version:     "v1",
			PlatformRef: "ref1",
		},
		Nodes: []Node{
			{
				ID: "node1",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
				},
			},
		},
	}

	g2 := &Graph{
		Metadata: GraphMetadata{
			Name:        "graph2", // Different name
			Version:     "v2",     // Different version
			PlatformRef: "ref2",   // Different ref
		},
		Nodes: []Node{
			{
				ID: "node1",
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
					},
				},
			},
		},
	}

	// Hashes should be the same because nodes are the same
	hash1 := g1.ComputeHash()
	hash2 := g2.ComputeHash()

	if hash1 != hash2 {
		t.Errorf("Expected same hash when only metadata differs, got %s and %s", hash1, hash2)
	}
}
