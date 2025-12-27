# Development Phases for CUE-Powered Platform Operator

This document breaks down the development of the platform operator into small, shippable chunks with clear goals for each task.

## Technology Stack

### Core Framework
- **Kubebuilder**: Scaffolding and controller framework
- **controller-runtime**: Kubernetes controller library
- **client-go**: Kubernetes API client with Server-Side Apply support

### Key Libraries

#### DAG Management
- **[github.com/dominikbraun/graph](https://github.com/dominikbraun/graph)**: Generic graph library with topological sort
  - Supports directed graphs with cycle detection
  - Built-in topological sort for dependency ordering
  - Thread-safe operations
  - Visualization support (DOT format)
  
- **[github.com/heimdalr/dag](https://github.com/heimdalr/dag)**: Specialized DAG implementation
  - Fast, thread-safe DAG operations
  - Prevents cycles and duplicates automatically
  - Ordered walk (topological traversal)
  - Good for concurrent execution scenarios

**Recommendation**: Start with `dominikbraun/graph` for flexibility, consider `heimdalr/dag` if performance becomes critical.

#### CUE Integration
- **[cuelang.org/go/cue](https://pkg.go.dev/cuelang.org/go/cue)**: Official CUE Go API
  - Value evaluation and validation
  - Schema compilation
  - JSON/YAML encoding/decoding
  - Error handling and diagnostics

#### Additional Utilities
- **k8s.io/apimachinery/pkg/apis/meta/v1/unstructured**: For dynamic resource handling
- **k8s.io/apimachinery/pkg/runtime/schema**: GVK handling
- **sigs.k8s.io/controller-runtime/pkg/client**: Enhanced Kubernetes client with SSA
- **github.com/go-logr/logr**: Structured logging (standard in controller-runtime)
- **github.com/prometheus/client_golang**: Metrics and observability

---

## Phase 0: Project Setup and Foundation (Week 1)

**Goal**: Establish project structure, dependencies, and development environment.

### Task 0.1: Initialize Kubebuilder Project
**Deliverable**: Working Kubebuilder scaffold

- [ ] Install Kubebuilder v3.x+
- [ ] Initialize project: `kubebuilder init --domain platform.example.com --repo github.com/yourorg/pequod`
- [ ] Verify scaffold builds: `make build`
- [ ] Set up Go module with required dependencies
- [ ] Create initial `.gitignore` and project documentation

**Acceptance Criteria**:
- Project builds successfully
- `make test` passes
- README documents setup steps

### Task 0.2: Add Core Dependencies
**Deliverable**: All required libraries integrated

- [ ] Add `github.com/dominikbraun/graph` for DAG management
- [ ] Add `cuelang.org/go` for CUE evaluation
- [ ] Add `github.com/prometheus/client_golang` for metrics
- [ ] Add testing dependencies: `github.com/onsi/ginkgo/v2`, `github.com/onsi/gomega`
- [ ] Run `go mod tidy` and verify all dependencies resolve

**Acceptance Criteria**:
- All dependencies in `go.mod`
- No version conflicts
- `go mod verify` passes

### Task 0.3: Set Up Development Environment
**Deliverable**: Local development tooling configured

- [ ] Create `Makefile` targets for common tasks (build, test, lint, run)
- [ ] Set up `golangci-lint` configuration
- [ ] Configure `envtest` for controller testing
- [ ] Create `kind` or `k3d` cluster configuration for local testing
- [ ] Document development workflow in `CONTRIBUTING.md`

**Acceptance Criteria**:
- `make lint` runs successfully
- `make test` runs unit tests
- Local cluster can be created with one command

### Task 0.4: Define Project Structure
**Deliverable**: Package layout following best practices

```
/cmd/operator          # Main entry point
/pkg/graph            # Graph types and executor
/pkg/apply            # SSA applier
/pkg/readiness        # Readiness predicate evaluation
/pkg/platformloader   # CUE module loading and caching
/pkg/inventory        # Inventory tracking
/cue/platform         # Embedded CUE modules
/config/crd           # CRD manifests
/config/samples       # Example resources
/test/e2e             # End-to-end tests
```

- [ ] Create directory structure
- [ ] Add package documentation (doc.go) for each package
- [ ] Create placeholder files to establish structure

**Acceptance Criteria**:
- All directories exist
- Each package has a `doc.go` with package description
- Structure documented in README

---

## Phase 1: Core Types and CRD (Week 2)

**Goal**: Define WebService CRD and Graph artifact types.

### Task 1.1: Create WebService CRD
**Deliverable**: WebService API definition

- [ ] Run: `kubebuilder create api --group platform --version v1alpha1 --kind WebService`
- [ ] Define `WebServiceSpec` with initial fields:
  - `image`: Container image
  - `replicas`: Number of replicas (optional, default from policy)
  - `port`: Service port
  - `platformRef`: Reference to platform module (embedded version only for now)
- [ ] Define `WebServiceStatus` with conditions:
  - `Rendered`: Graph artifact created
  - `PolicyPassed`: Policy validation succeeded
  - `Applying`: Resources being applied
  - `Ready`: All resources ready
- [ ] Add status subresource
- [ ] Generate CRD manifests: `make manifests`

**Acceptance Criteria**:
- CRD YAML generated in `config/crd/bases/`
- API types compile without errors
- `make manifests` succeeds

### Task 1.2: Define Graph Artifact Types
**Deliverable**: Internal types for Graph representation

- [ ] Create `pkg/graph/types.go` with:
  - `Graph` struct (metadata, nodes, violations)
  - `Node` struct (id, object, applyPolicy, dependsOn, readyWhen)
  - `ApplyPolicy` struct (mode, conflictPolicy)
  - `ReadinessPredicate` interface and implementations
- [ ] Add JSON/YAML serialization tags
- [ ] Create validation functions for Graph integrity
- [ ] Add unit tests for type validation

**Acceptance Criteria**:
- All types compile
- JSON marshaling/unmarshaling works
- Unit tests pass with >80% coverage

### Task 1.3: Define Readiness Predicate Types
**Deliverable**: Readiness predicate implementations

- [ ] Create `pkg/readiness/predicates.go` with:
  - `ConditionMatch` predicate (status.conditions check)
  - `DeploymentAvailable` predicate
  - `Exists` predicate (object exists)
- [ ] Implement `Evaluate(ctx, client, object) (bool, error)` for each
- [ ] Add unit tests with mock Kubernetes objects

**Acceptance Criteria**:
- All predicates implement common interface
- Unit tests cover success and failure cases
- Predicates work with unstructured.Unstructured objects

### Task 1.4: Add Inventory Types to Status
**Deliverable**: Inventory tracking in WebService status

- [ ] Add `Inventory` field to `WebServiceStatus`:
  - `nodeId`: string
  - `gvk`: GroupVersionKind
  - `namespace`, `name`, `uid`: string
  - `mode`: Managed | Adopted | Orphaned
  - `lastAppliedHash`: string
- [ ] Add helper functions for inventory management
- [ ] Update CRD manifests: `make manifests`

**Acceptance Criteria**:
- Status includes inventory array
- CRD updated with new fields
- Helper functions tested

---

## Phase 2: CUE Integration (Week 3)

**Goal**: Implement CUE module loading and evaluation to produce Graph artifacts.

### Task 2.1: Create Embedded CUE Module Structure
**Deliverable**: Initial platform module for WebService

- [ ] Create `cue/platform/webservice/schema.cue`:
  - Define `#WebServiceSpec` schema matching Go types
  - Add basic constraints (required fields, validation)
- [ ] Create `cue/platform/webservice/render.cue`:
  - Template for Deployment
  - Template for Service
  - Basic graph structure with nodes and dependencies
- [ ] Create `cue/platform/policy/input.cue`:
  - Example input policies (image registry, resource limits)
- [ ] Test CUE evaluation manually: `cue eval ./cue/platform/...`

**Acceptance Criteria**:
- CUE files are valid and evaluate without errors
- Schema matches Go WebServiceSpec
- Can produce a simple Deployment + Service

### Task 2.2: Implement CUE Loader
**Deliverable**: Go package to load and evaluate CUE modules

- [ ] Create `pkg/platformloader/loader.go`:
  - `LoadEmbedded(version string) (*cue.Value, error)`
  - Embed CUE files using `//go:embed`
  - Use `cuelang.org/go/cue/cuecontext` for evaluation
- [ ] Create `pkg/platformloader/cache.go`:
  - Simple in-memory cache keyed by version
  - Thread-safe access with sync.RWMutex
- [ ] Add unit tests with sample CUE content

**Acceptance Criteria**:
- Can load embedded CUE modules
- Cache prevents redundant evaluations
- Unit tests pass

### Task 2.3: Implement Graph Renderer
**Deliverable**: Convert CUE evaluation to Graph artifact

- [ ] Create `pkg/platformloader/renderer.go`:
  - `Render(ctx, cueValue, webServiceSpec) (*graph.Graph, error)`
  - Extract nodes from CUE output
  - Parse dependencies and readiness predicates
  - Convert CUE objects to unstructured.Unstructured
- [ ] Add validation for rendered Graph
- [ ] Add unit tests with golden files (expected Graph outputs)

**Acceptance Criteria**:
- CUE evaluation produces valid Graph
- All node fields populated correctly
- Golden file tests pass

### Task 2.4: Implement Policy Evaluation
**Deliverable**: Validate inputs and outputs against CUE policies

- [ ] Create `pkg/platformloader/policy.go`:
  - `ValidateInput(cueValue, spec) ([]Violation, error)`
  - `ValidateOutput(cueValue, graph) ([]Violation, error)`
  - Extract violations from CUE unification errors
- [ ] Add structured `Violation` type with path, message, severity
- [ ] Add unit tests with policy violations

**Acceptance Criteria**:
- Policy violations are detected and structured
- Both input and output validation work
- Clear error messages for developers

---

## Phase 3: DAG Executor (Week 4-5)

**Goal**: Implement dependency-aware resource application with readiness gates.

### Task 3.1: Implement DAG Builder
**Deliverable**: Convert Graph to executable DAG

- [ ] Create `pkg/graph/dag.go`:
  - `BuildDAG(graph *Graph) (*dominikbraun.Graph, error)`
  - Add nodes to graph library
  - Add edges based on `dependsOn`
  - Detect cycles and return error
- [ ] Add topological sort wrapper
- [ ] Add unit tests with various DAG structures

**Acceptance Criteria**:
- DAG built from Graph artifact
- Cycle detection works
- Topological sort produces correct order

### Task 3.2: Implement Node State Machine
**Deliverable**: Track per-node execution state

- [ ] Create `pkg/graph/state.go`:
  - `NodeState` enum: Pending, Applied, Ready, Error
  - `ExecutionState` struct tracking all node states
  - State transition functions with validation
- [ ] Add thread-safe state updates
- [ ] Add unit tests for state transitions

**Acceptance Criteria**:
- All state transitions are valid
- Thread-safe concurrent access
- State can be serialized to status

### Task 3.3: Implement DAG Executor Core
**Deliverable**: Execute DAG with dependency ordering

- [ ] Create `pkg/graph/executor.go`:
  - `Execute(ctx, dag, applier, readinessChecker) error`
  - Identify nodes with satisfied dependencies
  - Apply nodes in parallel where possible
  - Wait for readiness before marking complete
  - Handle errors and propagate to dependents
- [ ] Add execution metrics (nodes applied, time per node)
- [ ] Add comprehensive unit tests

**Acceptance Criteria**:
- Nodes applied in correct order
- Parallel execution where possible
- Errors handled gracefully
- Metrics collected

### Task 3.4: Implement Readiness Checker
**Deliverable**: Evaluate readiness predicates for applied resources

- [ ] Create `pkg/readiness/checker.go`:
  - `Check(ctx, client, object, predicates) (bool, error)`
  - Poll with exponential backoff
  - Configurable timeout per predicate
  - Return detailed status on failure
- [ ] Add retry logic with jitter
- [ ] Add unit tests with mock client

**Acceptance Criteria**:
- All predicate types supported
- Timeout and retry logic works
- Clear error messages on failure

---

## Phase 4: Server-Side Apply Integration (Week 5-6)

**Goal**: Implement authoritative resource management with SSA.

### Task 4.1: Implement SSA Applier
**Deliverable**: Apply resources using Server-Side Apply

- [ ] Create `pkg/apply/applier.go`:
  - `Apply(ctx, client, object, fieldManager) error`
  - Use `client.Patch()` with `client.Apply` patch type
  - Set field manager name (e.g., "pequod-operator")
  - Handle conflicts gracefully
- [ ] Add dry-run support
- [ ] Add unit tests with envtest

**Acceptance Criteria**:
- Resources applied with SSA
- Field manager set correctly
- Conflicts detected and reported
- Dry-run mode works

### Task 4.2: Implement Inventory Tracker
**Deliverable**: Track applied resources in status

- [ ] Create `pkg/inventory/tracker.go`:
  - `RecordApplied(nodeId, object, hash) InventoryItem`
  - `GetInventory() []InventoryItem`
  - `FindOrphaned(currentGraph) []InventoryItem`
  - Calculate resource hash for drift detection
- [ ] Add inventory comparison logic
- [ ] Add unit tests

**Acceptance Criteria**:
- Inventory items created for applied resources
- Orphaned resources detected correctly
- Hash calculation is deterministic

### Task 4.3: Implement Pruning Logic
**Deliverable**: Delete or orphan resources no longer in Graph

- [ ] Create `pkg/apply/pruner.go`:
  - `Prune(ctx, client, inventory, currentGraph, policy) error`
  - Identify resources to prune
  - Respect deletion policy (Delete vs Orphan)
  - Add safety checks (grace period, protection annotations)
- [ ] Add dry-run mode for pruning
- [ ] Add unit tests with various scenarios

**Acceptance Criteria**:
- Orphaned resources identified
- Deletion policy respected
- Safety checks prevent accidental deletion
- Dry-run shows what would be pruned

### Task 4.4: Add Apply Metrics and Logging
**Deliverable**: Observability for apply operations

- [ ] Add Prometheus metrics:
  - `pequod_apply_total` (counter by result: success/failure)
  - `pequod_apply_duration_seconds` (histogram)
  - `pequod_resources_managed` (gauge)
- [ ] Add structured logging for all apply operations
- [ ] Add tracing spans for apply operations (optional)

**Acceptance Criteria**:
- Metrics exposed on `/metrics` endpoint
- Logs include resource GVK, namespace, name
- Metrics can be scraped by Prometheus

---

## Phase 5: Controller Implementation (Week 6-7)

**Goal**: Implement WebService controller with full reconciliation loop.

### Task 5.1: Implement Basic Reconciler
**Deliverable**: WebService controller scaffold

- [ ] Create controller: `kubebuilder create controller --group platform --version v1alpha1 --kind WebService`
- [ ] Implement basic reconciliation loop:
  - Fetch WebService resource
  - Update status conditions
  - Handle deletion (finalizer)
- [ ] Add RBAC markers for required permissions
- [ ] Generate RBAC manifests: `make manifests`

**Acceptance Criteria**:
- Controller reconciles WebService resources
- Status conditions updated
- RBAC manifests generated

### Task 5.2: Integrate CUE Rendering
**Deliverable**: Controller renders Graph from WebService

- [ ] Add platform loader to reconciler
- [ ] Load embedded CUE module based on `spec.platformRef`
- [ ] Render Graph artifact from WebService spec
- [ ] Update status with render hash and platform ref
- [ ] Set `Rendered` condition

**Acceptance Criteria**:
- Graph rendered successfully
- Status includes render hash
- `Rendered` condition set to True on success

### Task 5.3: Integrate Policy Validation
**Deliverable**: Controller validates policies

- [ ] Add policy evaluation to reconciliation
- [ ] Validate input (WebService spec)
- [ ] Validate output (rendered Graph)
- [ ] Update status with violations
- [ ] Set `PolicyPassed` condition
- [ ] Stop reconciliation if policy fails

**Acceptance Criteria**:
- Policy violations detected
- Status includes structured violations
- Reconciliation stops on policy failure

### Task 5.4: Integrate DAG Execution
**Deliverable**: Controller applies resources via DAG executor

- [ ] Add DAG executor to reconciler
- [ ] Build DAG from Graph artifact
- [ ] Execute DAG with SSA applier
- [ ] Update inventory in status
- [ ] Set `Applying` and `Ready` conditions
- [ ] Handle partial failures gracefully

**Acceptance Criteria**:
- Resources applied in dependency order
- Inventory tracked in status
- Conditions reflect execution state
- Partial failures don't crash controller

### Task 5.5: Implement Finalizer Logic
**Deliverable**: Clean up resources on WebService deletion

- [ ] Add finalizer to WebService on creation
- [ ] Implement deletion logic:
  - Respect deletion policy (Delete vs Orphan)
  - Delete managed resources or mark orphaned
  - Remove finalizer when cleanup complete
- [ ] Add unit tests for deletion scenarios

**Acceptance Criteria**:
- Finalizer added on creation
- Resources cleaned up on deletion
- Finalizer removed after cleanup
- Orphan mode works correctly

---

## Phase 6: Testing and Validation (Week 7-8)

**Goal**: Comprehensive testing at all levels.

### Task 6.1: Unit Tests for All Packages
**Deliverable**: >80% unit test coverage

- [ ] Write unit tests for `pkg/graph/*`
- [ ] Write unit tests for `pkg/apply/*`
- [ ] Write unit tests for `pkg/readiness/*`
- [ ] Write unit tests for `pkg/platformloader/*`
- [ ] Write unit tests for `pkg/inventory/*`
- [ ] Achieve >80% coverage: `make test-coverage`

**Acceptance Criteria**:
- All packages have unit tests
- Coverage >80%
- All tests pass

### Task 6.2: Controller Integration Tests
**Deliverable**: envtest-based controller tests

- [ ] Create `controllers/webservice_controller_test.go`
- [ ] Test basic reconciliation flow
- [ ] Test policy validation (pass and fail)
- [ ] Test resource application
- [ ] Test deletion and cleanup
- [ ] Test error handling and retries

**Acceptance Criteria**:
- Integration tests run with envtest
- All reconciliation paths tested
- Tests pass consistently

### Task 6.3: End-to-End Tests
**Deliverable**: E2E tests on real cluster

- [ ] Create `test/e2e/webservice_test.go`
- [ ] Set up kind cluster in CI
- [ ] Test complete WebService lifecycle:
  - Create WebService
  - Verify Deployment and Service created
  - Verify readiness
  - Update WebService
  - Verify update applied
  - Delete WebService
  - Verify cleanup
- [ ] Add test for policy violations

**Acceptance Criteria**:
- E2E tests run on kind cluster
- Full lifecycle tested
- Tests pass in CI

### Task 6.4: Create Sample Resources
**Deliverable**: Example WebService manifests

- [ ] Create `config/samples/platform_v1alpha1_webservice.yaml`:
  - Simple web service example
  - Documented with comments
- [ ] Create `config/samples/webservice-with-policy-violation.yaml`:
  - Example that fails policy
- [ ] Test samples against running operator

**Acceptance Criteria**:
- Samples are valid YAML
- Samples work with operator
- Samples documented

---

## Phase 7: Observability and DX (Week 8-9)

**Goal**: Excellent developer experience and operational visibility.

### Task 7.1: Enhance Status Reporting
**Deliverable**: Rich, actionable status information

- [ ] Add node-level status to WebService:
  - `status.nodes[id].phase`
  - `status.nodes[id].lastError`
  - `status.nodes[id].readyDetails`
- [ ] Add human-readable messages to conditions
- [ ] Add observed generation tracking
- [ ] Update CRD manifests

**Acceptance Criteria**:
- Status shows per-node state
- Conditions have clear messages
- Status helps debug issues

### Task 7.2: Implement Graph Artifact Storage
**Deliverable**: Store rendered Graph for debugging

- [ ] Create ConfigMap for Graph artifact
- [ ] Store Graph as JSON in ConfigMap
- [ ] Reference ConfigMap name in status
- [ ] Add cleanup logic for old ConfigMaps
- [ ] Add size limits and warnings

**Acceptance Criteria**:
- Graph stored in ConfigMap
- ConfigMap referenced in status
- Old ConfigMaps cleaned up
- Size limits enforced

### Task 7.3: Add Comprehensive Metrics
**Deliverable**: Prometheus metrics for all operations

- [ ] Add reconciliation metrics:
  - `pequod_reconcile_total` (counter)
  - `pequod_reconcile_duration_seconds` (histogram)
  - `pequod_reconcile_errors_total` (counter)
- [ ] Add CUE evaluation metrics:
  - `pequod_cue_eval_duration_seconds` (histogram)
  - `pequod_cue_cache_hits_total` (counter)
- [ ] Add DAG execution metrics:
  - `pequod_dag_nodes_total` (gauge)
  - `pequod_dag_execution_duration_seconds` (histogram)
- [ ] Document metrics in README

**Acceptance Criteria**:
- All metrics exposed
- Metrics documented
- Metrics can be visualized in Grafana

### Task 7.4: Add Events
**Deliverable**: Kubernetes events for important operations

- [ ] Emit events for:
  - Graph rendered successfully
  - Policy validation failed
  - Resource applied
  - Resource ready
  - Pruning occurred
  - Errors
- [ ] Use appropriate event types (Normal, Warning)
- [ ] Include helpful messages

**Acceptance Criteria**:
- Events visible with `kubectl describe`
- Events help debug issues
- Event messages are clear

---

## Phase 8: Advanced Features - Adoption (Week 9-10)

**Goal**: Support adopting existing resources.

### Task 8.1: Add Adoption API
**Deliverable**: API for specifying resources to adopt

- [ ] Add to `WebServiceSpec`:
  - `adopt.resources[]` with GVK, namespace, name
  - `adopt.mode`: Explicit (default) or LabelSelector (future)
- [ ] Add validation for adoption spec
- [ ] Update CRD manifests
- [ ] Document adoption in API comments

**Acceptance Criteria**:
- Adoption spec in CRD
- Validation prevents invalid adoption specs
- API documented

### Task 8.2: Implement Adoption Logic
**Deliverable**: Adopt existing resources into management

- [ ] Create `pkg/apply/adopter.go`:
  - `Adopt(ctx, client, adoptSpec, node) error`
  - Fetch existing resource
  - Verify identity matches node target
  - SSA apply with field manager
  - Mark inventory item as Adopted
- [ ] Add safety checks (field manager conflicts)
- [ ] Add dry-run mode
- [ ] Add unit tests

**Acceptance Criteria**:
- Existing resources can be adopted
- Field manager conflicts detected
- Inventory marked as Adopted
- Dry-run shows what would be adopted

### Task 8.3: Integrate Adoption into Reconciler
**Deliverable**: Controller supports adoption workflow

- [ ] Add adoption phase to reconciliation
- [ ] Run adoption before normal DAG execution
- [ ] Update status with adoption results
- [ ] Add events for adopted resources
- [ ] Handle adoption failures gracefully

**Acceptance Criteria**:
- Adoption runs before apply
- Status shows adopted resources
- Events emitted for adoptions
- Failures don't block non-adopted resources

### Task 8.4: Add Adoption E2E Tests
**Deliverable**: Test adoption scenarios

- [ ] Create E2E test for adoption:
  - Create resources manually
  - Create WebService with adoption spec
  - Verify resources adopted
  - Verify resources managed after adoption
- [ ] Test adoption failure scenarios
- [ ] Test mixed adoption (some resources exist, some don't)

**Acceptance Criteria**:
- E2E tests cover adoption
- All scenarios tested
- Tests pass consistently

---

## Phase 9: Advanced Features - Remote Modules (Week 10-11)

**Goal**: Support loading CUE modules from remote sources.

### Task 9.1: Implement OCI Module Fetcher
**Deliverable**: Fetch CUE modules from OCI registries

- [ ] Create `pkg/platformloader/oci.go`:
  - `FetchOCI(ctx, ref string) ([]byte, string, error)`
  - Use `github.com/google/go-containerregistry` for OCI operations
  - Verify digest matches reference
  - Return module content and resolved digest
- [ ] Add authentication support (pull secrets)
- [ ] Add unit tests with mock registry

**Acceptance Criteria**:
- Can fetch from OCI registry
- Digest verification works
- Authentication supported
- Unit tests pass

### Task 9.2: Implement Git Module Fetcher
**Deliverable**: Fetch CUE modules from Git repositories

- [ ] Create `pkg/platformloader/git.go`:
  - `FetchGit(ctx, url, ref string) ([]byte, string, error)`
  - Use `github.com/go-git/go-git/v5` for Git operations
  - Resolve ref to commit SHA
  - Return module content and commit SHA
- [ ] Support authentication (SSH keys, tokens)
- [ ] Add unit tests with mock Git server

**Acceptance Criteria**:
- Can fetch from Git repository
- Ref resolved to commit SHA
- Authentication supported
- Unit tests pass

### Task 9.3: Implement Module Cache
**Deliverable**: Persistent cache for remote modules

- [ ] Create `pkg/platformloader/cache.go`:
  - Disk-based cache keyed by digest
  - LRU eviction policy
  - Thread-safe access
  - Cache size limits
- [ ] Add cache metrics
- [ ] Add unit tests

**Acceptance Criteria**:
- Modules cached on disk
- Cache hits avoid network calls
- LRU eviction works
- Metrics track cache performance

### Task 9.4: Integrate Remote Modules into Controller
**Deliverable**: Controller supports remote platform refs

- [ ] Update reconciler to handle remote refs:
  - Parse `spec.platformRef` (embedded vs OCI vs Git)
  - Fetch remote modules
  - Cache modules by digest
  - Update `status.platformRefResolved` with digest
- [ ] Add timeout for remote fetches
- [ ] Add retry logic with backoff
- [ ] Add E2E tests with remote modules

**Acceptance Criteria**:
- Remote modules loaded successfully
- Status shows resolved digest
- Timeouts and retries work
- E2E tests pass

---

## Phase 10: EKS/ACK Integration (Week 11-12)

**Goal**: Support AWS resources via ACK.

### Task 10.1: Add ACK IAM Role to CUE Module
**Deliverable**: CUE templates for ACK IAM resources

- [ ] Create `cue/platform/aws/iam.cue`:
  - Template for ACK IAM Role CR
  - Template for IAM Policy attachment
  - IRSA annotation for ServiceAccount
- [ ] Add dependencies: iamRole → serviceAccount → deployment
- [ ] Add readiness predicates for ACK conditions
- [ ] Test CUE evaluation

**Acceptance Criteria**:
- CUE templates for ACK resources
- Dependencies defined correctly
- Readiness predicates for ACK

### Task 10.2: Add Capability Detection
**Deliverable**: Detect ACK CRDs in cluster

- [ ] Create `pkg/capabilities/detector.go`:
  - `DetectACK(ctx, client) (bool, error)`
  - Check for ACK IAM CRDs
  - Cache detection results
- [ ] Add to controller startup
- [ ] Add metrics for capabilities

**Acceptance Criteria**:
- ACK detection works
- Results cached
- Metrics track capabilities

### Task 10.3: Conditional ACK Resource Rendering
**Deliverable**: Render ACK resources only when available

- [ ] Update CUE module to check capabilities
- [ ] Conditionally include ACK resources in Graph
- [ ] Fail with clear error if ACK required but missing
- [ ] Add unit tests

**Acceptance Criteria**:
- ACK resources only rendered when available
- Clear error when ACK missing
- Tests cover both scenarios

### Task 10.4: E2E Tests with ACK
**Deliverable**: Test WebService with IRSA

- [ ] Set up kind cluster with ACK controllers (mocked)
- [ ] Create E2E test:
  - WebService with IAM requirements
  - Verify IAM Role CR created
  - Verify ServiceAccount annotated
  - Verify Deployment uses ServiceAccount
- [ ] Test without ACK (should fail gracefully)

**Acceptance Criteria**:
- E2E test with ACK passes
- Test without ACK shows clear error
- All resources created in correct order

---

## Phase 11: Production Readiness (Week 12-13)

**Goal**: Prepare for production deployment.

### Task 11.1: Add Leader Election
**Deliverable**: Support multiple operator replicas

- [ ] Enable leader election in main.go
- [ ] Configure lease duration and renewal
- [ ] Add metrics for leader election
- [ ] Test with multiple replicas

**Acceptance Criteria**:
- Leader election works
- Only one replica reconciles
- Failover works correctly
- Metrics track leader status

### Task 11.2: Add Health Checks
**Deliverable**: Liveness and readiness probes

- [ ] Implement `/healthz` endpoint
- [ ] Implement `/readyz` endpoint
- [ ] Add checks for:
  - Controller manager running
  - Kubernetes API accessible
  - CUE module cache accessible
- [ ] Configure probes in deployment manifest

**Acceptance Criteria**:
- Health endpoints respond correctly
- Probes configured in manifest
- Unhealthy pods restarted

### Task 11.3: Add Resource Limits and Requests
**Deliverable**: Proper resource configuration

- [ ] Profile operator resource usage
- [ ] Set appropriate requests and limits
- [ ] Configure memory limits for CUE evaluation
- [ ] Add resource metrics
- [ ] Document resource requirements

**Acceptance Criteria**:
- Resources set based on profiling
- Operator doesn't OOM
- Resource usage documented

### Task 11.4: Security Hardening
**Deliverable**: Secure operator deployment

- [ ] Run as non-root user
- [ ] Drop all capabilities
- [ ] Set read-only root filesystem
- [ ] Add security context to deployment
- [ ] Scan container image for vulnerabilities
- [ ] Document security considerations

**Acceptance Criteria**:
- Security context configured
- Container runs as non-root
- No high/critical vulnerabilities
- Security documented

### Task 11.5: Create Helm Chart
**Deliverable**: Easy installation via Helm

- [ ] Create Helm chart structure
- [ ] Parameterize common values:
  - Image repository and tag
  - Resource limits
  - RBAC settings
  - Metrics configuration
- [ ] Add chart documentation
- [ ] Test installation with Helm

**Acceptance Criteria**:
- Helm chart installs successfully
- Values documented
- Chart follows best practices

---

## Phase 12: Documentation and Release (Week 13-14)

**Goal**: Complete documentation and prepare first release.

### Task 12.1: Write User Documentation
**Deliverable**: Comprehensive user guide

- [ ] Create `docs/user-guide.md`:
  - Installation instructions
  - WebService API reference
  - Examples (simple, with policies, with ACK)
  - Troubleshooting guide
- [ ] Create `docs/platform-engineer-guide.md`:
  - CUE module development
  - Policy authoring
  - Platform module versioning
- [ ] Add architecture diagrams

**Acceptance Criteria**:
- Documentation covers all features
- Examples work as documented
- Diagrams illustrate architecture

### Task 12.2: Write Operator Documentation
**Deliverable**: Operational guide

- [ ] Create `docs/operations.md`:
  - Deployment guide
  - Monitoring and alerting
  - Backup and recovery
  - Upgrade procedures
  - Troubleshooting runbook
- [ ] Document all metrics
- [ ] Document all events

**Acceptance Criteria**:
- Operations guide complete
- Metrics documented
- Runbook covers common issues

### Task 12.3: Create Tutorial
**Deliverable**: Getting started tutorial

- [ ] Create `docs/tutorial.md`:
  - Step-by-step walkthrough
  - Deploy operator to kind
  - Create first WebService
  - Update WebService
  - View status and debug
  - Clean up
- [ ] Test tutorial on fresh environment

**Acceptance Criteria**:
- Tutorial works end-to-end
- Tested on clean environment
- Clear for beginners

### Task 12.4: Prepare Release
**Deliverable**: v0.1.0 release

- [ ] Tag release: `v0.1.0`
- [ ] Build and push container images
- [ ] Create GitHub release with:
  - Release notes
  - Installation instructions
  - Known limitations
  - Upgrade notes (N/A for first release)
- [ ] Publish Helm chart
- [ ] Announce release

**Acceptance Criteria**:
- Release tagged
- Images published
- Release notes complete
- Helm chart available

---

## Future Phases (Post v0.1.0)

### Phase 13: Advanced Readiness Predicates
- Custom readiness webhooks
- CEL expression support
- External dependency checks
- Timeout configuration per predicate

### Phase 14: Rollback and Progressive Delivery
- Store previous Graph artifacts
- `spec.rollbackTo` field
- Progressive rollout (canary)
- Automatic rollback on failures

### Phase 15: Multi-Tenancy
- Namespace scoping
- Tenant-specific policies
- RBAC validation
- Resource quotas

### Phase 16: Enhanced Observability
- OpenTelemetry tracing
- Backstage plugin
- Web UI for Graph visualization
- Cost attribution

### Phase 17: Additional Cloud Providers
- GCP integration (Config Connector)
- Azure integration (ASO)
- Multi-cloud abstractions

---

## Success Metrics

### Phase 1-6 (Core Functionality)
- [ ] WebService CRD deployed
- [ ] Simple Deployment + Service rendered
- [ ] Resources applied with SSA
- [ ] >80% test coverage
- [ ] E2E tests pass

### Phase 7-9 (Advanced Features)
- [ ] Adoption works
- [ ] Remote modules supported
- [ ] Comprehensive metrics
- [ ] Graph artifacts stored

### Phase 10-12 (Production Ready)
- [ ] ACK integration works
- [ ] Leader election enabled
- [ ] Security hardened
- [ ] Documentation complete
- [ ] v0.1.0 released

---

## Risk Mitigation

### High-Risk Items
1. **DAG executor correctness**: Extensive unit and integration tests, chaos testing
2. **CUE evaluation performance**: Aggressive caching, profiling, optimization
3. **SSA conflict handling**: Clear error messages, dry-run mode, documentation

### Mitigation Strategies
- **Weekly demos**: Show progress to stakeholders
- **Incremental delivery**: Each phase produces working software
- **Continuous testing**: Tests run on every commit
- **Early feedback**: Get user feedback starting Phase 6
- **Documentation-driven**: Write docs before code when possible

---

## Timeline Summary

- **Weeks 1-2**: Foundation and CRDs
- **Weeks 3-5**: CUE integration and DAG execution
- **Weeks 6-8**: Controller and testing
- **Weeks 9-11**: Advanced features (adoption, remote modules, ACK)
- **Weeks 12-14**: Production readiness and release

**Total**: ~14 weeks to v0.1.0

**Team Size**: 2-3 engineers recommended for this timeline


