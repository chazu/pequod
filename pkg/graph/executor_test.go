package graph

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// mockApplier is a mock implementation of Applier for testing
type mockApplier struct {
	mu           sync.Mutex
	appliedNodes []string
	failNodes    map[string]error
	applyDelay   time.Duration
}

func newMockApplier() *mockApplier {
	return &mockApplier{
		appliedNodes: make([]string, 0),
		failNodes:    make(map[string]error),
	}
}

func (m *mockApplier) Apply(ctx context.Context, obj *unstructured.Unstructured, policy ApplyPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate apply delay
	if m.applyDelay > 0 {
		time.Sleep(m.applyDelay)
	}

	name := obj.GetName()

	// Check if this node should fail
	if err, shouldFail := m.failNodes[name]; shouldFail {
		return err
	}

	m.appliedNodes = append(m.appliedNodes, name)
	return nil
}

func (m *mockApplier) getAppliedNodes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.appliedNodes))
	copy(result, m.appliedNodes)
	return result
}

func (m *mockApplier) setFailNode(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNodes[name] = err
}

// mockReadinessChecker is a mock implementation of ReadinessChecker for testing
type mockReadinessChecker struct {
	mu         sync.Mutex
	readyNodes map[string]bool
	checkDelay time.Duration
	failNodes  map[string]error
}

func newMockReadinessChecker() *mockReadinessChecker {
	return &mockReadinessChecker{
		readyNodes: make(map[string]bool),
		failNodes:  make(map[string]error),
	}
}

func (m *mockReadinessChecker) Check(ctx context.Context, obj *unstructured.Unstructured, predicates []ReadinessPredicate) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate check delay
	if m.checkDelay > 0 {
		time.Sleep(m.checkDelay)
	}

	name := obj.GetName()

	// Check if this node should fail
	if err, shouldFail := m.failNodes[name]; shouldFail {
		return false, err
	}

	// Check if ready
	ready, exists := m.readyNodes[name]
	if !exists {
		return false, nil
	}

	return ready, nil
}

func (m *mockReadinessChecker) setReady(name string, ready bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyNodes[name] = ready
}

func (m *mockReadinessChecker) setFailNode(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNodes[name] = err
}

func TestExecutor_SimpleLinearDAG(t *testing.T) {
	// Create a simple linear DAG: a -> b -> c
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
				DependsOn:   []string{"b"},
			},
		},
	}

	dag, err := BuildDAG(g)
	if err != nil {
		t.Fatalf("BuildDAG() failed: %v", err)
	}

	applier := newMockApplier()
	checker := newMockReadinessChecker()

	// All nodes are ready immediately (no readiness predicates)
	executor := NewExecutor(applier, checker, nil, DefaultExecutorConfig())

	ctx := context.Background()
	state, err := executor.Execute(ctx, dag)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Check that all nodes were applied
	applied := applier.getAppliedNodes()
	if len(applied) != 3 {
		t.Errorf("Expected 3 nodes applied, got %d", len(applied))
	}

	// Check execution state
	if !state.IsComplete() {
		t.Error("Execution should be complete")
	}

	if state.HasErrors() {
		t.Error("Execution should not have errors")
	}

	summary := state.GetSummary()
	if summary.Ready != 3 {
		t.Errorf("Expected 3 ready nodes, got %d", summary.Ready)
	}
}

func TestExecutor_ParallelExecution(t *testing.T) {
	// Create a diamond DAG: a -> b, a -> c, b -> d, c -> d
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

	applier := newMockApplier()
	applier.applyDelay = 50 * time.Millisecond // Add delay to test parallelism
	checker := newMockReadinessChecker()

	executor := NewExecutor(applier, checker, nil, DefaultExecutorConfig())

	ctx := context.Background()
	start := time.Now()
	state, err := executor.Execute(ctx, dag)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Check that all nodes were applied
	applied := applier.getAppliedNodes()
	if len(applied) != 4 {
		t.Errorf("Expected 4 nodes applied, got %d", len(applied))
	}

	// With parallelism, b and c should execute concurrently
	// Total time should be less than sequential (4 * 50ms = 200ms)
	// With parallelism: a (50ms) + b,c parallel (50ms) + d (50ms) = ~150ms
	// Allow some overhead for goroutine scheduling and wave coordination
	if duration > 250*time.Millisecond {
		t.Errorf("Execution took too long (%v), parallelism may not be working", duration)
	}

	if !state.IsComplete() || state.HasErrors() {
		t.Error("Execution should be complete without errors")
	}
}

func TestExecutor_ErrorHandling(t *testing.T) {
	// Create a DAG where node b fails: a -> b -> c, a -> d
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
				DependsOn:   []string{"b"},
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
				DependsOn:   []string{"a"},
			},
		},
	}

	dag, err := BuildDAG(g)
	if err != nil {
		t.Fatalf("BuildDAG() failed: %v", err)
	}

	applier := newMockApplier()
	applier.setFailNode("b", errors.New("apply failed"))
	checker := newMockReadinessChecker()

	// Disable retries for this test
	config := DefaultExecutorConfig()
	config.MaxRetries = 0
	executor := NewExecutor(applier, checker, nil, config)

	ctx := context.Background()
	state, err := executor.Execute(ctx, dag)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Check that a and d succeeded, b failed, c blocked
	applied := applier.getAppliedNodes()
	if len(applied) != 2 { // a and d
		t.Errorf("Expected 2 nodes applied (a, d), got %d", len(applied))
	}

	// Check states
	stateA, _ := state.GetState("a")
	if stateA != NodeStateReady {
		t.Errorf("Node a should be Ready, got %s", stateA)
	}

	stateB, _ := state.GetState("b")
	if stateB != NodeStateError {
		t.Errorf("Node b should be Error, got %s", stateB)
	}

	stateC, _ := state.GetState("c")
	if stateC != NodeStatePending {
		t.Errorf("Node c should be Pending (blocked), got %s", stateC)
	}

	stateD, _ := state.GetState("d")
	if stateD != NodeStateReady {
		t.Errorf("Node d should be Ready, got %s", stateD)
	}

	if !state.HasErrors() {
		t.Error("Execution should have errors")
	}
}
