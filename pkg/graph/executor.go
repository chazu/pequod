package graph

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/sourcegraph/conc/pool"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Applier is the interface for applying Kubernetes resources
type Applier interface {
	// Apply applies a resource according to its ApplyPolicy
	Apply(ctx context.Context, obj *unstructured.Unstructured, policy ApplyPolicy) error
}

// ReadinessChecker is the interface for checking resource readiness
type ReadinessChecker interface {
	// Check evaluates readiness predicates for a resource
	Check(ctx context.Context, obj *unstructured.Unstructured, predicates []ReadinessPredicate) (bool, error)
}

// ExecutorConfig contains configuration for the DAG executor
type ExecutorConfig struct {
	// MaxConcurrency is the maximum number of nodes to apply concurrently
	// Default: 10
	MaxConcurrency int

	// RetryBackoffBase is the base duration for exponential backoff
	// Default: 1 second
	RetryBackoffBase time.Duration

	// RetryBackoffMax is the maximum backoff duration
	// Default: 5 minutes
	RetryBackoffMax time.Duration

	// MaxRetries is the maximum number of retries per node
	// Default: 3
	MaxRetries int
}

// DefaultExecutorConfig returns the default executor configuration
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		MaxConcurrency:   10,
		RetryBackoffBase: 1 * time.Second,
		RetryBackoffMax:  5 * time.Minute,
		MaxRetries:       3,
	}
}

// Executor executes a DAG with dependency-aware parallel execution
type Executor struct {
	config           ExecutorConfig
	applier          Applier
	readinessChecker ReadinessChecker
	client           client.Client
}

// NewExecutor creates a new DAG executor
func NewExecutor(applier Applier, readinessChecker ReadinessChecker, client client.Client, config ExecutorConfig) *Executor {
	return &Executor{
		config:           config,
		applier:          applier,
		readinessChecker: readinessChecker,
		client:           client,
	}
}

// Execute executes the DAG with dependency-aware parallel execution
func (e *Executor) Execute(ctx context.Context, dag *DAG) (*ExecutionState, error) {
	if dag == nil {
		return nil, fmt.Errorf("DAG cannot be nil")
	}

	// Initialize execution state
	nodeIDs := make([]string, 0, dag.Size())
	for _, id := range dag.GetOrder() {
		nodeIDs = append(nodeIDs, id)
	}
	state := NewExecutionState(nodeIDs)

	// Execute nodes in waves based on dependencies
	for !state.IsComplete() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		// Find nodes ready to execute
		readyNodes := e.findReadyNodes(dag, state)
		if len(readyNodes) == 0 {
			// No nodes ready - execution is complete (either all done or stuck)
			break
		}

		// Execute ready nodes in parallel using conc
		if err := e.executeNodes(ctx, dag, state, readyNodes); err != nil {
			return state, err
		}
	}

	state.MarkComplete()
	return state, nil
}

// findReadyNodes identifies nodes that are ready to execute
// A node is ready if:
// - It's in Pending or Error state (for retry)
// - All its dependencies are in Ready state
func (e *Executor) findReadyNodes(dag *DAG, state *ExecutionState) []string {
	var ready []string

	for _, nodeID := range dag.GetOrder() {
		nodeState, _ := state.GetState(nodeID)

		// Only consider Pending or Error (for retry) nodes
		if nodeState != NodeStatePending && nodeState != NodeStateError {
			continue
		}

		// Check if this is a retry and we've exceeded max retries
		if nodeState == NodeStateError {
			status, _ := state.GetStatus(nodeID)
			if status.RetryCount >= e.config.MaxRetries {
				continue
			}
		}

		// Check if all dependencies are ready
		deps, _ := dag.GetDependencies(nodeID)
		allDepsReady := true
		for _, depID := range deps {
			depState, _ := state.GetState(depID)
			if depState != NodeStateReady {
				allDepsReady = false
				break
			}
		}

		if allDepsReady {
			ready = append(ready, nodeID)
		}
	}

	return ready
}

// executeNodes executes a batch of nodes in parallel using conc
func (e *Executor) executeNodes(ctx context.Context, dag *DAG, state *ExecutionState, nodeIDs []string) error {
	// Create a worker pool with bounded concurrency
	p := pool.New().WithMaxGoroutines(e.config.MaxConcurrency).WithErrors()

	for _, nodeID := range nodeIDs {
		nodeID := nodeID // Capture for goroutine

		p.Go(func() error {
			return e.executeNode(ctx, dag, state, nodeID)
		})
	}

	// Wait for all nodes to complete
	// Note: We don't fail fast - we want to continue with independent nodes
	if err := p.Wait(); err != nil {
		// Errors are already recorded in state, just log that some failed
		return nil // Don't stop execution
	}

	return nil
}

// executeNode executes a single node: apply, wait for readiness
func (e *Executor) executeNode(ctx context.Context, dag *DAG, state *ExecutionState, nodeID string) error {
	node, found := dag.GetNode(nodeID)
	if !found {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Check if this is a retry
	currentState, _ := state.GetState(nodeID)
	if currentState == NodeStateError {
		// Calculate backoff delay
		status, _ := state.GetStatus(nodeID)
		delay := e.calculateBackoff(status.RetryCount)

		// Wait for backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		// Increment retry count
		state.IncrementRetry(nodeID)
	}

	// Transition to Applying state
	if err := state.SetState(nodeID, NodeStateApplying); err != nil {
		state.SetError(nodeID, err)
		return err
	}

	// Apply the resource with its policy
	if err := e.applier.Apply(ctx, &node.Object, node.ApplyPolicy); err != nil {
		state.SetError(nodeID, fmt.Errorf("failed to apply: %w", err))
		return err
	}

	// Check if we need to wait for readiness
	if len(node.ReadyWhen) == 0 {
		// No readiness predicates - mark as ready immediately
		if err := state.SetState(nodeID, NodeStateReady); err != nil {
			state.SetError(nodeID, err)
			return err
		}
		return nil
	}

	// Transition to WaitingReady state
	if err := state.SetState(nodeID, NodeStateWaitingReady); err != nil {
		state.SetError(nodeID, err)
		return err
	}

	// Wait for readiness
	if err := e.waitForReadiness(ctx, node, state, nodeID); err != nil {
		state.SetError(nodeID, fmt.Errorf("readiness check failed: %w", err))
		return err
	}

	// Mark as ready
	if err := state.SetState(nodeID, NodeStateReady); err != nil {
		state.SetError(nodeID, err)
		return err
	}

	return nil
}

// waitForReadiness polls the resource until all readiness predicates are satisfied
func (e *Executor) waitForReadiness(ctx context.Context, node *Node, state *ExecutionState, nodeID string) error {
	// Determine timeout - use the maximum timeout from all predicates, or default to 5 minutes
	timeout := 5 * time.Minute
	for _, pred := range node.ReadyWhen {
		if pred.Timeout > 0 {
			predTimeout := time.Duration(pred.Timeout) * time.Second
			if predTimeout > timeout {
				timeout = predTimeout
			}
		}
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll with exponential backoff
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		// Check readiness
		ready, err := e.readinessChecker.Check(timeoutCtx, &node.Object, node.ReadyWhen)
		if err != nil {
			return fmt.Errorf("readiness check error: %w", err)
		}

		if ready {
			return nil
		}

		// Wait with backoff
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("readiness timeout after %v", timeout)
		case <-time.After(backoff):
			// Increase backoff exponentially with jitter
			backoff = time.Duration(float64(backoff) * 1.5)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// calculateBackoff calculates the backoff duration for a retry attempt
func (e *Executor) calculateBackoff(retryCount int) time.Duration {
	// Exponential backoff: base * 2^retryCount
	backoff := time.Duration(float64(e.config.RetryBackoffBase) * math.Pow(2, float64(retryCount)))

	// Cap at max backoff
	if backoff > e.config.RetryBackoffMax {
		backoff = e.config.RetryBackoffMax
	}

	return backoff
}
