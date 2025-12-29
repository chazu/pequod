package graph

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewExecutionState(t *testing.T) {
	nodeIDs := []string{"a", "b", "c"}
	es := NewExecutionState(nodeIDs)

	if es == nil {
		t.Fatal("NewExecutionState returned nil")
	}

	// All nodes should start in Pending state
	for _, id := range nodeIDs {
		state, err := es.GetState(id)
		if err != nil {
			t.Errorf("GetState(%s) failed: %v", id, err)
		}
		if state != NodeStatePending {
			t.Errorf("Node %s state = %s, want %s", id, state, NodeStatePending)
		}
	}
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      NodeState
		to        NodeState
		wantError bool
	}{
		{"Pending to Applying", NodeStatePending, NodeStateApplying, false},
		{"Pending to Error", NodeStatePending, NodeStateError, false},
		{"Applying to WaitingReady", NodeStateApplying, NodeStateWaitingReady, false},
		{"Applying to Ready", NodeStateApplying, NodeStateReady, false},
		{"Applying to Error", NodeStateApplying, NodeStateError, false},
		{"WaitingReady to Ready", NodeStateWaitingReady, NodeStateReady, false},
		{"WaitingReady to Error", NodeStateWaitingReady, NodeStateError, false},
		{"Error to Pending", NodeStateError, NodeStatePending, false},
		{"Ready to Pending", NodeStateReady, NodeStatePending, true}, // Invalid
		{"Pending to Ready", NodeStatePending, NodeStateReady, true}, // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := NewExecutionState([]string{"test"})

			// Set initial state (bypass validation for setup)
			es.mu.Lock()
			es.nodeStates["test"].State = tt.from
			es.mu.Unlock()

			err := es.SetState("test", tt.to)
			if (err != nil) != tt.wantError {
				t.Errorf("SetState() error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError {
				state, _ := es.GetState("test")
				if state != tt.to {
					t.Errorf("State = %s, want %s", state, tt.to)
				}
			}
		})
	}
}

func TestSetError(t *testing.T) {
	es := NewExecutionState([]string{"test"})
	testErr := errors.New("test error")

	err := es.SetError("test", testErr)
	if err != nil {
		t.Fatalf("SetError() failed: %v", err)
	}

	state, _ := es.GetState("test")
	if state != NodeStateError {
		t.Errorf("State = %s, want %s", state, NodeStateError)
	}

	status, _ := es.GetStatus("test")
	if status.Error != testErr.Error() {
		t.Errorf("Error = %s, want %s", status.Error, testErr.Error())
	}
}

func TestIncrementRetry(t *testing.T) {
	es := NewExecutionState([]string{"test"})

	// Increment retry count
	err := es.IncrementRetry("test")
	if err != nil {
		t.Fatalf("IncrementRetry() failed: %v", err)
	}

	status, _ := es.GetStatus("test")
	if status.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", status.RetryCount)
	}
	if status.LastRetryTime == nil {
		t.Error("LastRetryTime should be set")
	}

	// Increment again
	time.Sleep(10 * time.Millisecond)
	err = es.IncrementRetry("test")
	if err != nil {
		t.Fatalf("IncrementRetry() failed: %v", err)
	}

	status, _ = es.GetStatus("test")
	if status.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", status.RetryCount)
	}
}

func TestGetNodesInState(t *testing.T) {
	es := NewExecutionState([]string{"a", "b", "c", "d"})

	// Set different states
	es.SetState("a", NodeStateApplying)
	es.SetState("b", NodeStateApplying)
	es.SetState("c", NodeStateApplying)
	es.SetState("c", NodeStateReady)

	applying := es.GetNodesInState(NodeStateApplying)
	if len(applying) != 2 {
		t.Errorf("GetNodesInState(Applying) returned %d nodes, want 2", len(applying))
	}

	pending := es.GetNodesInState(NodeStatePending)
	if len(pending) != 1 {
		t.Errorf("GetNodesInState(Pending) returned %d nodes, want 1", len(pending))
	}

	ready := es.GetNodesInState(NodeStateReady)
	if len(ready) != 1 {
		t.Errorf("GetNodesInState(Ready) returned %d nodes, want 1", len(ready))
	}
}

func TestIsComplete(t *testing.T) {
	es := NewExecutionState([]string{"a", "b", "c"})

	// Not complete initially
	if es.IsComplete() {
		t.Error("IsComplete() = true, want false (all pending)")
	}

	// Set some to ready
	es.SetState("a", NodeStateApplying)
	es.SetState("a", NodeStateReady)
	if es.IsComplete() {
		t.Error("IsComplete() = true, want false (some still pending)")
	}

	// Set all to terminal states
	es.SetState("b", NodeStateApplying)
	es.SetState("b", NodeStateReady)
	es.SetError("c", errors.New("test"))
	if !es.IsComplete() {
		t.Error("IsComplete() = false, want true (all terminal)")
	}
}

func TestHasErrors(t *testing.T) {
	es := NewExecutionState([]string{"a", "b"})

	if es.HasErrors() {
		t.Error("HasErrors() = true, want false (no errors)")
	}

	es.SetError("a", errors.New("test"))
	if !es.HasErrors() {
		t.Error("HasErrors() = false, want true (has error)")
	}
}

func TestGetSummary(t *testing.T) {
	es := NewExecutionState([]string{"a", "b", "c", "d", "e"})

	// Set various states
	es.SetState("a", NodeStateApplying)
	es.SetState("b", NodeStateApplying)
	es.SetState("b", NodeStateWaitingReady)
	es.SetState("c", NodeStateApplying)
	es.SetState("c", NodeStateReady)
	es.SetError("d", errors.New("test"))
	// e remains pending

	summary := es.GetSummary()
	if summary.Total != 5 {
		t.Errorf("Summary.Total = %d, want 5", summary.Total)
	}
	if summary.Pending != 1 {
		t.Errorf("Summary.Pending = %d, want 1", summary.Pending)
	}
	if summary.Applying != 1 {
		t.Errorf("Summary.Applying = %d, want 1", summary.Applying)
	}
	if summary.WaitingReady != 1 {
		t.Errorf("Summary.WaitingReady = %d, want 1", summary.WaitingReady)
	}
	if summary.Ready != 1 {
		t.Errorf("Summary.Ready = %d, want 1", summary.Ready)
	}
	if summary.Error != 1 {
		t.Errorf("Summary.Error = %d, want 1", summary.Error)
	}
}

func TestConcurrentAccess(t *testing.T) {
	es := NewExecutionState([]string{"a", "b", "c", "d", "e"})

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent state updates
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nodeID := []string{"a", "b", "c", "d", "e"}[idx%5]
			es.SetState(nodeID, NodeStateApplying)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nodeID := []string{"a", "b", "c", "d", "e"}[idx%5]
			es.GetState(nodeID)
			es.GetStatus(nodeID)
		}(i)
	}

	// Concurrent summary reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			es.GetSummary()
			es.IsComplete()
			es.HasErrors()
		}()
	}

	wg.Wait()
}

func TestMarkComplete(t *testing.T) {
	es := NewExecutionState([]string{"a"})

	summary := es.GetSummary()
	if summary.EndTime != nil {
		t.Error("EndTime should be nil before MarkComplete")
	}

	es.MarkComplete()

	summary = es.GetSummary()
	if summary.EndTime == nil {
		t.Error("EndTime should be set after MarkComplete")
	}
}

func TestTimestamps(t *testing.T) {
	es := NewExecutionState([]string{"test"})

	// StartTime should be nil initially
	status, _ := es.GetStatus("test")
	if status.StartTime != nil {
		t.Error("StartTime should be nil for pending node")
	}

	// StartTime should be set when transitioning to Applying
	es.SetState("test", NodeStateApplying)
	status, _ = es.GetStatus("test")
	if status.StartTime == nil {
		t.Error("StartTime should be set when applying")
	}

	// ReadyTime should be set when transitioning to Ready
	time.Sleep(10 * time.Millisecond)
	es.SetState("test", NodeStateReady)
	status, _ = es.GetStatus("test")
	if status.ReadyTime == nil {
		t.Error("ReadyTime should be set when ready")
	}

	if !status.ReadyTime.After(*status.StartTime) {
		t.Error("ReadyTime should be after StartTime")
	}
}
