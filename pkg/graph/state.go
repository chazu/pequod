package graph

import (
	"fmt"
	"sync"
	"time"
)

// NodeState represents the execution state of a node in the DAG
type NodeState string

const (
	// NodeStatePending indicates the node is waiting for dependencies
	NodeStatePending NodeState = "Pending"

	// NodeStateApplying indicates the node is being applied
	NodeStateApplying NodeState = "Applying"

	// NodeStateWaitingReady indicates the node has been applied and is waiting for readiness
	NodeStateWaitingReady NodeState = "WaitingReady"

	// NodeStateReady indicates the node is ready (all predicates satisfied)
	NodeStateReady NodeState = "Ready"

	// NodeStateError indicates the node encountered an error
	NodeStateError NodeState = "Error"
)

// NodeStatus contains the execution status of a single node
type NodeStatus struct {
	// State is the current state of the node
	State NodeState

	// Error contains the error message if State is NodeStateError
	Error string

	// StartTime is when the node started applying
	StartTime *time.Time

	// ReadyTime is when the node became ready
	ReadyTime *time.Time

	// RetryCount is the number of times this node has been retried
	RetryCount int

	// LastRetryTime is the time of the last retry attempt
	LastRetryTime *time.Time
}

// ExecutionState tracks the execution state of all nodes in a DAG
type ExecutionState struct {
	mu sync.RWMutex

	// nodeStates maps node ID to its current status
	nodeStates map[string]*NodeStatus

	// startTime is when execution started
	startTime time.Time

	// endTime is when execution completed (or failed)
	endTime *time.Time
}

// NewExecutionState creates a new execution state tracker
func NewExecutionState(nodeIDs []string) *ExecutionState {
	states := make(map[string]*NodeStatus, len(nodeIDs))
	for _, id := range nodeIDs {
		states[id] = &NodeStatus{
			State: NodeStatePending,
		}
	}

	return &ExecutionState{
		nodeStates: states,
		startTime:  time.Now(),
	}
}

// GetState returns the current state of a node
func (es *ExecutionState) GetState(nodeID string) (NodeState, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	status, found := es.nodeStates[nodeID]
	if !found {
		return "", fmt.Errorf("node %s not found", nodeID)
	}
	return status.State, nil
}

// GetStatus returns the full status of a node
func (es *ExecutionState) GetStatus(nodeID string) (*NodeStatus, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	status, found := es.nodeStates[nodeID]
	if !found {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	// Return a copy to prevent external modification
	statusCopy := *status
	return &statusCopy, nil
}

// SetState updates the state of a node with validation
func (es *ExecutionState) SetState(nodeID string, newState NodeState) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	status, found := es.nodeStates[nodeID]
	if !found {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Validate state transition
	if err := validateStateTransition(status.State, newState); err != nil {
		return fmt.Errorf("invalid state transition for node %s: %w", nodeID, err)
	}

	// Update state and timestamps
	status.State = newState

	now := time.Now()
	switch newState {
	case NodeStateApplying:
		if status.StartTime == nil {
			status.StartTime = &now
		}
	case NodeStateReady:
		status.ReadyTime = &now
	}

	return nil
}

// SetError sets a node to error state with an error message
func (es *ExecutionState) SetError(nodeID string, err error) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	status, found := es.nodeStates[nodeID]
	if !found {
		return fmt.Errorf("node %s not found", nodeID)
	}

	status.State = NodeStateError
	status.Error = err.Error()

	return nil
}

// IncrementRetry increments the retry count for a node
func (es *ExecutionState) IncrementRetry(nodeID string) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	status, found := es.nodeStates[nodeID]
	if !found {
		return fmt.Errorf("node %s not found", nodeID)
	}

	status.RetryCount++
	now := time.Now()
	status.LastRetryTime = &now

	return nil
}

// GetNodesInState returns all node IDs in a given state
func (es *ExecutionState) GetNodesInState(state NodeState) []string {
	es.mu.RLock()
	defer es.mu.RUnlock()

	var nodes []string
	for id, status := range es.nodeStates {
		if status.State == state {
			nodes = append(nodes, id)
		}
	}
	return nodes
}

// GetAllStates returns a copy of all node states
func (es *ExecutionState) GetAllStates() map[string]NodeState {
	es.mu.RLock()
	defer es.mu.RUnlock()

	states := make(map[string]NodeState, len(es.nodeStates))
	for id, status := range es.nodeStates {
		states[id] = status.State
	}
	return states
}

// IsComplete returns true if all nodes are in a terminal state (Ready or Error)
func (es *ExecutionState) IsComplete() bool {
	es.mu.RLock()
	defer es.mu.RUnlock()

	for _, status := range es.nodeStates {
		if status.State != NodeStateReady && status.State != NodeStateError {
			return false
		}
	}
	return true
}

// HasErrors returns true if any node is in error state
func (es *ExecutionState) HasErrors() bool {
	es.mu.RLock()
	defer es.mu.RUnlock()

	for _, status := range es.nodeStates {
		if status.State == NodeStateError {
			return true
		}
	}
	return false
}

// GetSummary returns a summary of execution state
func (es *ExecutionState) GetSummary() ExecutionSummary {
	es.mu.RLock()
	defer es.mu.RUnlock()

	summary := ExecutionSummary{
		Total:     len(es.nodeStates),
		StartTime: es.startTime,
		EndTime:   es.endTime,
	}

	for _, status := range es.nodeStates {
		switch status.State {
		case NodeStatePending:
			summary.Pending++
		case NodeStateApplying:
			summary.Applying++
		case NodeStateWaitingReady:
			summary.WaitingReady++
		case NodeStateReady:
			summary.Ready++
		case NodeStateError:
			summary.Error++
		}
	}

	return summary
}

// MarkComplete marks the execution as complete
func (es *ExecutionState) MarkComplete() {
	es.mu.Lock()
	defer es.mu.Unlock()

	now := time.Now()
	es.endTime = &now
}

// ExecutionSummary provides a summary of execution state
type ExecutionSummary struct {
	Total        int
	Pending      int
	Applying     int
	WaitingReady int
	Ready        int
	Error        int
	StartTime    time.Time
	EndTime      *time.Time
}

// validateStateTransition checks if a state transition is valid
func validateStateTransition(from, to NodeState) error {
	// Define valid transitions
	validTransitions := map[NodeState][]NodeState{
		NodeStatePending: {
			NodeStateApplying,
			NodeStateError,
		},
		NodeStateApplying: {
			NodeStateWaitingReady,
			NodeStateReady, // If no readiness predicates
			NodeStateError,
		},
		NodeStateWaitingReady: {
			NodeStateReady,
			NodeStateError,
		},
		NodeStateReady: {
			// Terminal state - no transitions
		},
		NodeStateError: {
			NodeStatePending, // Allow retry
		},
	}

	allowed, found := validTransitions[from]
	if !found {
		return fmt.Errorf("unknown state: %s", from)
	}

	for _, allowedState := range allowed {
		if allowedState == to {
			return nil
		}
	}

	return fmt.Errorf("cannot transition from %s to %s", from, to)
}
