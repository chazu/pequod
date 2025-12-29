package graph

import (
	"fmt"

	"github.com/dominikbraun/graph"
)

// DAG represents an executable directed acyclic graph built from a Graph artifact
type DAG struct {
	// graph is the underlying graph structure from dominikbraun/graph
	graph graph.Graph[string, string]

	// nodeMap provides quick lookup of nodes by ID
	nodeMap map[string]*Node

	// order contains the topologically sorted node IDs
	order []string
}

// BuildDAG converts a Graph artifact into an executable DAG
// It validates the graph structure, detects cycles, and computes topological order
func BuildDAG(g *Graph) (*DAG, error) {
	if g == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	// Validate the graph first
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %w", err)
	}

	// Create a directed graph with cycle prevention
	dg := graph.New(graph.StringHash, graph.Directed(), graph.PreventCycles())

	// Build node map for quick lookup
	nodeMap := make(map[string]*Node, len(g.Nodes))
	for i := range g.Nodes {
		node := &g.Nodes[i]
		nodeMap[node.ID] = node
	}

	// Add all vertices first
	for id := range nodeMap {
		if err := dg.AddVertex(id); err != nil {
			return nil, fmt.Errorf("failed to add vertex %s: %w", id, err)
		}
	}

	// Add edges based on dependencies
	// Note: In dominikbraun/graph, AddEdge(source, target) means source -> target
	// In our model, if node B depends on node A, we want A -> B (A must complete before B)
	for id, node := range nodeMap {
		for _, depID := range node.DependsOn {
			// depID must complete before id can start
			// So we add edge: depID -> id
			if err := dg.AddEdge(depID, id); err != nil {
				return nil, fmt.Errorf("failed to add edge %s -> %s: %w", depID, id, err)
			}
		}
	}

	// Compute topological order
	order, err := graph.TopologicalSort(dg)
	if err != nil {
		return nil, fmt.Errorf("failed to compute topological sort (possible cycle): %w", err)
	}

	return &DAG{
		graph:   dg,
		nodeMap: nodeMap,
		order:   order,
	}, nil
}

// GetNode retrieves a node by ID
func (d *DAG) GetNode(id string) (*Node, bool) {
	node, found := d.nodeMap[id]
	return node, found
}

// GetOrder returns the topologically sorted node IDs
// Nodes earlier in the list have no dependencies on nodes later in the list
func (d *DAG) GetOrder() []string {
	return d.order
}

// GetDependencies returns the IDs of nodes that the given node depends on
func (d *DAG) GetDependencies(id string) ([]string, error) {
	node, found := d.nodeMap[id]
	if !found {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return node.DependsOn, nil
}

// GetDependents returns the IDs of nodes that depend on the given node
func (d *DAG) GetDependents(id string) ([]string, error) {
	if _, found := d.nodeMap[id]; !found {
		return nil, fmt.Errorf("node %s not found", id)
	}

	var dependents []string
	for nodeID, node := range d.nodeMap {
		for _, depID := range node.DependsOn {
			if depID == id {
				dependents = append(dependents, nodeID)
				break
			}
		}
	}
	return dependents, nil
}

// Size returns the number of nodes in the DAG
func (d *DAG) Size() int {
	return len(d.nodeMap)
}

// HasCycles checks if the graph has any cycles
// This should always return false if BuildDAG succeeded, but is provided for completeness
func (d *DAG) HasCycles() bool {
	_, err := graph.TopologicalSort(d.graph)
	return err != nil
}

// GetRootNodes returns nodes that have no dependencies
func (d *DAG) GetRootNodes() []string {
	var roots []string
	for id, node := range d.nodeMap {
		if len(node.DependsOn) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}

// GetLeafNodes returns nodes that no other nodes depend on
func (d *DAG) GetLeafNodes() []string {
	// Build a set of all nodes that are dependencies
	hasDependents := make(map[string]bool)
	for _, node := range d.nodeMap {
		for _, depID := range node.DependsOn {
			hasDependents[depID] = true
		}
	}

	// Nodes not in the set are leaves
	var leaves []string
	for id := range d.nodeMap {
		if !hasDependents[id] {
			leaves = append(leaves, id)
		}
	}
	return leaves
}
