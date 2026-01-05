# Pequod Kubernetes Operator Code Review Report

**Date:** 2026-01-05
**Reviewer:** k8s-operator-expert agent

## Executive Summary

The Pequod operator is a platform engineering tool that dynamically generates CRDs from CUE modules and manages resources via ResourceGraph execution. The codebase demonstrates solid understanding of Kubernetes operator patterns but has several areas requiring attention before production deployment.

---

## 1. Technical Debt

### 1.1 TODO Comments and Incomplete Implementations

**File: `pkg/platformloader/loader.go:105-106`**
```go
// TODO: In production, this should use go:embed with proper paths
func (l *Loader) LoadEmbedded(version string) (cue.Value, error) {
```
**Issue:** The embedded CUE module loader uses relative path detection (`findCuePlatformPath`) instead of Go's `embed` package, which is brittle and will fail in containerized deployments.

**File: `pkg/platformloader/loader.go:134-150`**
```go
func (l *Loader) findCuePlatformPath() (string, error) {
    // Try current directory first
    if _, err := os.Stat("cue/platform"); err == nil {
        return "cue/platform", nil
    }
    // Try going up one level...
```
**Issue:** This approach is non-deterministic and environment-dependent. The Docker image may not have these paths available.

**File: `config/manager/manager.yaml:87-88`**
```yaml
# TODO(user): Configure the resources accordingly based on the project requirements.
resources:
  limits:
    cpu: 500m
    memory: 128Mi
```
**Issue:** 128Mi memory limit is very low for an operator that processes CUE templates, generates CRDs, and manages multiple controllers. This will likely cause OOM issues in production.

### 1.2 LabelSelector Mode Not Implemented

**File: `pkg/apply/adopter.go:123`**
```go
case platformv1alpha1.AdoptModeLabelSelector:
    return nil, fmt.Errorf("LabelSelector mode not yet implemented")
```
**Issue:** The API exposes `LabelSelector` as a valid mode but it's not implemented. This should either be implemented or removed from the API enum.

### 1.3 Mirror Strategy Not Implemented

**File: `pkg/apply/adopter.go:227-229`**
```go
case platformv1alpha1.AdoptStrategyMirror:
    // Mirror mode: don't modify the resource, just track it
    // Future: implement mirror tracking
    result.Error = nil
```
**Issue:** `Mirror` strategy is a no-op but exposed in the API.

---

## 2. Kubernetes Operator Best Practices Issues

### 2.1 RBAC Configuration - Overly Permissive (CRITICAL)

**File: `config/rbac/role.yaml:14-28`**
```yaml
- apiGroups:
  - ""
  - '*'
  - apps
  - batch
  resources:
  - '*'
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
```
**CRITICAL:** The RBAC role grants `*` verbs on `*` resources across all API groups. This is a major security vulnerability. The operator should request only the specific resources it manages.

**File: `internal/controller/platforminstance_controller.go:63`**
```go
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete
```
**Issue:** The RBAC markers also request wildcard permissions.

### 2.2 Status Updates Without Retry-On-Conflict Pattern

**File: `pkg/reconcile/transform_handlers.go:134-137`**
```go
tf.Status.Phase = platformv1alpha1.TransformPhaseFetching
if err := h.client.Status().Update(ctx, tf); err != nil {
    logger.Error(err, "failed to update phase to Fetching")
}
```
**Issue:** Multiple status updates throughout the reconciliation do not use retry-on-conflict and silently swallow errors. Status updates should use `retry.RetryOnConflict` or `meta.SetStatusCondition` patterns.

**Similar issues in:**
- `pkg/reconcile/transform_handlers.go:149-151`
- `pkg/reconcile/transform_handlers.go:164-166`
- `pkg/reconcile/transform_handlers.go:179-181`

### 2.3 Missing Rate Limiting on Controllers

**File: `internal/controller/transform_controller.go:70-73`**
```go
return ctrl.NewControllerManagedBy(mgr).
    For(&platformv1alpha1.Transform{}).
    Named("transform").
    Complete(r)
```
**Issue:** No rate limiter configured. Under load or with many failing resources, the controller may overwhelm the API server.

**Same issue in:**
- `internal/controller/resourcegraph_controller.go:380-394`
- `internal/controller/platforminstance_controller.go:136-147`

### 2.4 PlatformInstanceReconciler Inefficient GVK Lookup

**File: `internal/controller/platforminstance_controller.go:76-96`**
```go
r.watchMutex.RLock()
gvks := make([]schema.GroupVersionKind, 0, len(r.watchedGVKs))
for gvk := range r.watchedGVKs {
    gvks = append(gvks, gvk)
}
r.watchMutex.RUnlock()

// Try to get the instance using each watched GVK
for _, gvk := range gvks {
    u := &unstructured.Unstructured{}
    u.SetGroupVersionKind(gvk)
    if err := r.Get(ctx, req.NamespacedName, u); err == nil {
        instance = u
        instanceGVK = gvk
        break
    }
}
```
**Issue:** On every reconcile, the controller iterates through ALL watched GVKs and performs API server GETs until it finds the right one. This is O(n) API calls where n is the number of platform types. Should use an index to map namespace/name to GVK.

### 2.5 Watch Discovery Loop Polling

**File: `internal/controller/platforminstance_controller.go:183-198`**
```go
ticker := time.NewTicker(10 * time.Second)
defer ticker.Stop()

// Initial discovery
r.discoverAndWatchPlatformTypes(ctx)

for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        r.discoverAndWatchPlatformTypes(ctx)
    }
}
```
**Issue:** Polling every 10 seconds is inefficient. Should use a Watch on Transforms instead and react to Transform changes. The Transform watch in `SetupWithManager` returns empty requests, defeating its purpose.

### 2.6 Watch Removal Not Actually Working

**File: `internal/controller/platforminstance_controller.go:278-284`**
```go
func (r *PlatformInstanceReconciler) RemoveWatch(gvk schema.GroupVersionKind) {
    r.watchMutex.Lock()
    defer r.watchMutex.Unlock()
    delete(r.watchedGVKs, gvk)
    // Note: controller-runtime doesn't support removing watches dynamically
    // The watch will remain but instances will 404 since the CRD is deleted
}
```
**Issue:** The comment acknowledges that watches cannot be removed, but `RemoveWatch` is still exposed in the API, misleading callers.

### 2.7 Missing Condition Management with ObservedGeneration

**File: `api/v1alpha1/transform_types.go:276-299`**
```go
func (t *Transform) SetCondition(condType string, status metav1.ConditionStatus, reason, message string) {
    // ... custom condition management
}
```
**Issue:** Custom condition management instead of using `meta.SetStatusCondition`. Also, the conditions in ResourceGraphStatus at `internal/controller/resourcegraph_controller.go:529-537` replace the entire condition slice rather than updating individual conditions:
```go
latest.Status.Conditions = []metav1.Condition{
    {
        Type: ConditionTypeFailed,
        // ...
    },
}
```
This loses history of other conditions.

### 2.8 No MaxConcurrentReconciles Configuration

All three controllers lack `WithOptions(controller.Options{MaxConcurrentReconciles: n})` configuration, defaulting to 1 concurrent reconcile which may be a bottleneck.

---

## 3. Code Quality Issues

### 3.1 Race Condition in Executor

**File: `pkg/graph/executor.go:152-169`**
```go
func (e *Executor) executeNodes(ctx context.Context, dag *DAG, state *ExecutionState, nodeIDs []string) error {
    p := pool.New().WithMaxGoroutines(e.config.MaxConcurrency).WithErrors()

    for _, nodeID := range nodeIDs {
        p.Go(func() error {
            return e.executeNode(ctx, dag, state, nodeID)  // nodeID captured by closure
        })
    }
```
**Issue:** Go loop variable capture issue (fixed in Go 1.22, but should be explicit for older versions):
```go
for _, nodeID := range nodeIDs {
    nodeID := nodeID  // Shadow the loop variable
    p.Go(func() error {
        return e.executeNode(ctx, dag, state, nodeID)
    })
}
```

### 3.2 Error Handling Gaps

**File: `pkg/graph/executor.go:164-168`**
```go
if err := p.Wait(); err != nil {
    // Errors are already recorded in state, just log that some failed
    return nil // Don't stop execution
}
```
**Issue:** Errors from parallel execution are silently dropped. While the intent is to continue with independent nodes, there's no logging or tracking of which specific errors occurred.

**File: `pkg/graph/executor.go:193-194`**
```go
// Increment retry count (error ignored: retry proceeds regardless)
_ = state.IncrementRetry(nodeID)
```
**Issue:** Error explicitly ignored. If the node doesn't exist in state, this could indicate a bug that goes undetected.

### 3.3 Status Update Timing in ResourceGraph

**File: `internal/controller/resourcegraph_controller.go:186-191`**
```go
// Update status to Executing
if err := r.updateStatusExecuting(ctx, rg); err != nil {
    logger.Error(err, "Failed to update status to Executing")
    // Requeue to retry status update
    return ctrl.Result{Requeue: true}, err
}
```
Then later:
```go
executionState, err := r.Executor.Execute(ctx, dag)
```
**Issue:** If the executor starts executing but then we can't update status at the end (conflict), we'll re-run the entire execution. While idempotent, this wastes resources.

### 3.4 Missing Input Validation

**File: `pkg/apply/applier.go:50-58`**
```go
func (a *Applier) Apply(ctx context.Context, obj *unstructured.Unstructured, policy graph.ApplyPolicy) error {
    if obj == nil {
        return fmt.Errorf("object cannot be nil")
    }
```
Good nil check, but no validation that obj has required fields (GVK, name, etc.) before the API call.

### 3.5 Inconsistent API Group Names

**File: `config/rbac/role.yaml`**
Multiple different API groups referenced:
- `platform.platform.example.com` (CRDs)
- `platform.pequod.io` (some RBAC)

**File: `internal/controller/resourcegraph_controller.go:75-77`**
```go
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=resourcegraphs
```
But `pkg/crd/generator.go:19-20`:
```go
DefaultGroup = "platform.pequod.io"
```
**Issue:** Inconsistent group naming (`platform.platform.example.com` vs `platform.pequod.io`). This could cause RBAC issues.

---

## 4. Testing Issues

### 4.1 Skipped Tests

**File: `internal/controller/transform_controller_test.go:117-166`**
```go
PIt("should update the CRD when Transform spec changes", func() {
    // Skip: This test has race conditions due to concurrent status updates
```
Three tests are marked as `PIt` (pending):
- Line 117: "should update the CRD when Transform spec changes"
- Line 169: "should handle paused transforms"
- Line 203: "should delete the CRD when Transform is deleted"

**Issue:** Critical functionality (CRD updates, paused handling, deletion cleanup) has skipped tests, indicating known race conditions.

### 4.2 No Integration Tests for PlatformInstanceReconciler

The test suite in `internal/controller/suite_test.go:105-127` sets up `ResourceGraphReconciler` and `TransformReconciler` but NOT `PlatformInstanceReconciler`:
```go
// Setup ResourceGraph controller
err = (&ResourceGraphReconciler{...}).SetupWithManager(k8sManager)
// Setup Transform controller
err = (&TransformReconciler{...}).SetupWithManager(k8sManager)
// Missing: PlatformInstanceReconciler
```

### 4.3 Test Flakiness Risk

**File: `internal/controller/resourcegraph_controller_test.go:38-41`**
```go
const (
    timeout  = time.Second * 60
    interval = time.Millisecond * 250
)
```
60-second timeout with 250ms polling is reasonable, but some tests may be flaky under CI load.

### 4.4 E2E Tests Don't Test Adoption

**File: `test/e2e/e2e_test.go`**
The E2E tests cover basic Transform/Instance flow but don't test:
- Resource adoption
- DAG execution ordering
- Readiness checking
- Error recovery
- Re-execution after spec changes

---

## 5. Production Readiness Issues

### 5.1 Insufficient Resource Limits

**File: `config/manager/manager.yaml:89-95`**
```yaml
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi
```
**Issue:**
- 128Mi memory is too low for CUE processing (CUE can be memory-intensive)
- 10m CPU request is extremely low
- No resource quotas consideration for managed resources

### 5.2 No Webhook Validation

The CRDs don't have admission webhooks for validation. All validation happens at runtime during reconciliation:

**File: `pkg/graph/validation.go`**
Validation exists but only executes during reconciliation, not at admission time.

### 5.3 Missing PodDisruptionBudget

No PDB defined in `config/` which could cause unavailability during cluster upgrades.

### 5.4 Metrics Cardinality Risk

**File: `internal/controller/metrics.go:43-52`**
```go
dagNodesTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Name: "pequod_dag_nodes_total",
}, []string{"resourcegraph"})

dagExecutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "pequod_dag_execution_duration_seconds",
}, []string{"resourcegraph", "result"})

dagNodeExecutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "pequod_dag_node_execution_duration_seconds",
}, []string{"node_id", "result"})
```
**Issue:** Using `resourcegraph` name and especially `node_id` as labels will create unbounded cardinality. With many ResourceGraphs and nodes, this will blow up Prometheus memory.

### 5.5 No Context Timeout on CUE Evaluation

**File: `pkg/platformloader/renderer.go:39-94`**
CUE compilation/evaluation has no timeout. Malformed or complex CUE could hang the controller.

### 5.6 No Health Check Beyond Ping

**File: `cmd/main.go:210-216`**
```go
if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
    setupLog.Error(err, "unable to set up health check")
    os.Exit(1)
}
if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
```
**Issue:** Both health and readiness use simple ping. Should check cache sync status for readiness.

### 5.7 No Graceful Shutdown for Long-Running Operations

**File: `pkg/graph/executor.go:84-89`**
```go
for !state.IsComplete() {
    select {
    case <-ctx.Done():
        return state, ctx.Err()
    default:
    }
```
Good context cancellation, but:
- Active node executions may not be cancelled
- No drain logic to finish in-progress work before shutdown

### 5.8 Missing Network Policies

While `config/network-policy/` exists with metrics access, there's no policy restricting egress (the operator can reach any external OCI registry, Git repo, etc.).

---

## 6. Security Concerns

### 6.1 Wildcard RBAC (CRITICAL)

As noted in section 2.1, the RBAC configuration grants full cluster access.

### 6.2 CUE Code Execution Risk

**File: `pkg/platformloader/renderer.go:60-67`**
```go
case InlineType:
    // Compile inline CUE directly
    cueValue = r.loader.ctx.CompileString(cueRef.Ref)
```
Inline CUE is user-provided and executed directly. While CUE is not a full programming language, complex expressions could cause resource exhaustion.

### 6.3 Secret Access Without Audit

**File: `pkg/platformloader/loader.go:64-74`**
```go
if pullSecretRef != nil && *pullSecretRef != "" {
    pullSecret = &corev1.Secret{}
    if err := l.fetchers.client.Get(ctx, client.ObjectKey{
        Namespace: namespace,
        Name:      *pullSecretRef,
    }, pullSecret); err != nil {
```
Secrets are accessed but there's no logging/auditing of which secrets are accessed.

### 6.4 No Input Sanitization for CRD Names

**File: `pkg/crd/generator.go:80-82`**
```go
kind := toKind(platformName)
plural := toPlural(platformName)
singular := strings.ToLower(platformName)
```
Platform names directly become CRD names without validation. Malicious names could cause issues.

---

## 7. Summary of Critical Issues

| Priority | Issue | Location |
|----------|-------|----------|
| **CRITICAL** | RBAC Wildcard Permissions | `config/rbac/role.yaml` |
| **HIGH** | Embedded Loader Path Detection | `pkg/platformloader/loader.go` |
| **HIGH** | Metrics Cardinality | `internal/controller/metrics.go` |
| **HIGH** | Status Update Race Conditions | Multiple skipped tests confirm |
| **MEDIUM** | Inefficient GVK Lookup | `internal/controller/platforminstance_controller.go` |
| **MEDIUM** | Memory Limits | `config/manager/manager.yaml` |
| **MEDIUM** | Missing PlatformInstanceReconciler Tests | `internal/controller/suite_test.go` |
| **MEDIUM** | Polling Watch Discovery | `internal/controller/platforminstance_controller.go` |
| **LOW** | Inconsistent API Groups | Multiple files |

---

## 8. Recommendations

### Immediate (Before Production)

1. **Fix RBAC** to request only specific resources the operator needs to manage
2. **Implement go:embed** for CUE modules instead of path detection
3. **Increase memory limits** to at least 512Mi
4. **Fix metrics labels** to avoid unbounded cardinality (use hashed identifiers or remove high-cardinality labels)

### Short-term

1. Add `retry.RetryOnConflict` to all status updates
2. Implement webhook validation for Transform and ResourceGraph CRDs
3. Add rate limiting to controllers with `WithOptions(controller.Options{RateLimiter: ...})`
4. Fix or remove skipped tests
5. Add `MaxConcurrentReconciles` configuration

### Medium-term

1. Replace polling with watch-based Transform discovery
2. Add GVK index for O(1) instance lookups
3. Implement comprehensive E2E test coverage for:
   - Resource adoption
   - DAG execution ordering
   - Error recovery
   - Spec change re-execution
4. Add PodDisruptionBudget
5. Add CUE evaluation timeout
6. Implement or remove unimplemented API features (LabelSelector mode, Mirror strategy)

---

## Appendix: Files Reviewed

- `api/v1alpha1/*.go` - API types
- `cmd/main.go` - Entry point
- `config/` - Kubernetes manifests
- `internal/controller/*.go` - Reconcilers
- `pkg/apply/*.go` - Resource application
- `pkg/crd/*.go` - CRD generation
- `pkg/graph/*.go` - DAG execution
- `pkg/platformloader/*.go` - CUE module loading
- `pkg/reconcile/*.go` - Reconciliation handlers
- `test/e2e/*.go` - E2E tests
