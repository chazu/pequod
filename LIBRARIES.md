# Key Libraries and Dependencies

This document provides detailed information about the key libraries used in Pequod and their specific purposes.

## Core Kubernetes Libraries

### controller-runtime
- **Package**: `sigs.k8s.io/controller-runtime`
- **Purpose**: High-level framework for building Kubernetes controllers
- **Key Features**:
  - Manager for running controllers
  - Client with caching and Server-Side Apply support
  - Predicate filtering for watch events
  - Webhook support
- **Usage in Pequod**: Core controller framework, reconciliation loop, client operations

### client-go
- **Package**: `k8s.io/client-go`
- **Purpose**: Official Kubernetes Go client
- **Key Features**:
  - REST client for Kubernetes API
  - Server-Side Apply support (Patch with Apply patch type)
  - Dynamic client for unstructured resources
  - Apply configurations for type-safe SSA
- **Usage in Pequod**: Server-Side Apply operations, dynamic resource handling

### apimachinery
- **Package**: `k8s.io/apimachinery`
- **Purpose**: Kubernetes API machinery and types
- **Key Features**:
  - `unstructured.Unstructured` for dynamic resources
  - `schema.GroupVersionKind` for resource identification
  - `meta/v1` for common metadata types
  - JSON/YAML serialization
- **Usage in Pequod**: Graph artifact nodes (unstructured resources), GVK handling

## DAG Management Libraries

### dominikbraun/graph (Recommended)
- **Package**: `github.com/dominikbraun/graph`
- **GitHub**: https://github.com/dominikbraun/graph
- **Purpose**: Generic graph data structures with algorithms
- **Key Features**:
  - Directed and undirected graphs
  - Cycle detection
  - Topological sort (Kahn's algorithm)
  - Thread-safe operations
  - DOT format export for visualization
  - Transitive reduction
- **Usage in Pequod**: 
  - Build DAG from Graph artifact
  - Topological sort for execution order
  - Cycle detection for validation
  - Visualization for debugging
- **Example**:
  ```go
  g := graph.New(graph.StringHash, graph.Directed(), graph.PreventCycles())
  g.AddVertex("deployment")
  g.AddVertex("service")
  g.AddEdge("service", "deployment") // service depends on deployment
  order, _ := graph.TopologicalSort(g)
  ```

### heimdalr/dag (Alternative)
- **Package**: `github.com/heimdalr/dag`
- **GitHub**: https://github.com/heimdalr/dag
- **Purpose**: Specialized DAG implementation
- **Key Features**:
  - Fast, thread-safe DAG operations
  - Automatic cycle and duplicate prevention
  - Ordered walk (topological traversal)
  - Optimized for concurrent scenarios
- **Usage in Pequod**: Consider if `dominikbraun/graph` has performance issues
- **When to Use**: High-frequency DAG operations, large graphs (100+ nodes)

## Concurrency Libraries

### sourcegraph/conc
- **Package**: `github.com/sourcegraph/conc`
- **GitHub**: https://github.com/sourcegraph/conc
- **Purpose**: Better structured concurrency for Go
- **Key Features**:
  - Worker pools with bounded concurrency (`pool.Pool`)
  - Automatic panic recovery and propagation
  - Error aggregation for concurrent tasks (`pool.ErrorPool`)
  - Result collection from parallel tasks (`pool.ResultPool`)
  - Prevents goroutine leaks with scoped concurrency
  - Cleaner API than manual sync.WaitGroup + channels
- **Usage in Pequod**:
  - DAG executor worker pool for parallel node application
  - Bounded concurrency (configurable max goroutines)
  - Panic-safe goroutine execution in long-running operator
  - Error collection from parallel apply operations
- **Example**:
  ```go
  p := pool.New().WithMaxGoroutines(10).WithErrors()
  for _, node := range nodes {
      node := node
      p.Go(func() error {
          return applyNode(ctx, node)
      })
  }
  if err := p.Wait(); err != nil {
      // Handle aggregated errors
  }
  ```
- **Why Not Standard Library**:
  - Manual worker pools require significant boilerplate
  - Easy to leak goroutines or mishandle panics
  - `conc` provides safety guarantees critical for operators

## CUE Integration

### cuelang.org/go/cue
- **Package**: `cuelang.org/go/cue`
- **Docs**: https://pkg.go.dev/cuelang.org/go/cue
- **Purpose**: Official CUE Go API for evaluation and validation
- **Key Features**:
  - Value evaluation and unification
  - Schema validation
  - JSON/YAML encoding/decoding
  - Error diagnostics
  - Path-based value extraction
- **Usage in Pequod**:
  - Load and evaluate CUE platform modules
  - Validate WebService specs against CUE schemas
  - Extract rendered resources from CUE output
  - Policy evaluation and violation reporting
- **Example**:
  ```go
  ctx := cuecontext.New()
  val := ctx.CompileString(`
    #WebService: {
      image: string
      replicas: int | *3
    }
  `)
  ```

### cuelang.org/go/cue/load
- **Package**: `cuelang.org/go/cue/load`
- **Purpose**: Load CUE packages from filesystem
- **Usage in Pequod**: Load embedded CUE modules, parse CUE files

### cuelang.org/go/cue/cuecontext
- **Package**: `cuelang.org/go/cue/cuecontext`
- **Purpose**: CUE evaluation context
- **Usage in Pequod**: Create evaluation contexts for CUE operations

## Observability

### prometheus/client_golang
- **Package**: `github.com/prometheus/client_golang/prometheus`
- **Purpose**: Prometheus metrics instrumentation
- **Key Features**:
  - Counter, Gauge, Histogram, Summary metrics
  - Metric registration and collection
  - HTTP handler for `/metrics` endpoint
- **Usage in Pequod**:
  - Reconciliation metrics (duration, errors)
  - CUE evaluation metrics (cache hits, duration)
  - DAG execution metrics (nodes, duration)
  - Resource metrics (managed resources, inventory size)

### go-logr/logr
- **Package**: `github.com/go-logr/logr`
- **Purpose**: Structured logging interface (standard in controller-runtime)
- **Key Features**:
  - Structured key-value logging
  - Log levels (info, error)
  - Context-aware logging
- **Usage in Pequod**: All logging throughout the operator

## Testing

### onsi/ginkgo
- **Package**: `github.com/onsi/ginkgo/v2`
- **Purpose**: BDD-style testing framework
- **Usage in Pequod**: Controller integration tests, E2E tests

### onsi/gomega
- **Package**: `github.com/onsi/gomega`
- **Purpose**: Matcher library for assertions
- **Usage in Pequod**: Test assertions and expectations

### controller-runtime/pkg/envtest
- **Package**: `sigs.k8s.io/controller-runtime/pkg/envtest`
- **Purpose**: Integration testing with real Kubernetes API
- **Key Features**:
  - Starts etcd and kube-apiserver
  - No kubelet (no pod execution)
  - Fast test setup/teardown
- **Usage in Pequod**: Controller integration tests

## Additional Utilities

### go-containerregistry
- **Package**: `github.com/google/go-containerregistry`
- **Purpose**: OCI registry operations
- **Usage in Pequod**: Fetch remote CUE modules from OCI registries (Phase 9)

### go-git
- **Package**: `github.com/go-git/go-git/v5`
- **Purpose**: Git operations in Go
- **Usage in Pequod**: Fetch remote CUE modules from Git repositories (Phase 9)

## Dependency Management Strategy

### Version Pinning
- Pin all dependencies to specific versions in `go.mod`
- Use `go mod tidy` to clean up unused dependencies
- Regularly update dependencies for security patches

### Compatibility
- Ensure Kubernetes library versions are compatible
- Match controller-runtime version with Kubebuilder version
- Test with multiple Kubernetes versions (1.27+)

### Vendoring (Optional)
- Consider vendoring for reproducible builds
- Use `go mod vendor` if needed
- Commit vendor directory for air-gapped environments

## Library Selection Rationale

### Why dominikbraun/graph over heimdalr/dag?
- More flexible (supports multiple graph types)
- Better documentation and examples
- Visualization support (DOT export)
- Active maintenance
- Can switch to heimdalr/dag if performance becomes critical

### Why CUE over alternatives (Jsonnet, Dhall)?
- Native Go integration (cuelang.org/go)
- Strong type system with constraints
- Designed for configuration and validation
- Growing ecosystem and community
- Used by KubeVela and other projects

### Why Kubebuilder over Operator SDK?
- More focused on Go operators
- Better integration with controller-runtime
- Simpler scaffolding
- Active development by Kubernetes SIG
- Both use controller-runtime underneath

## Performance Considerations

### CUE Evaluation
- Cache evaluated CUE values by digest
- Use incremental evaluation where possible
- Set memory limits for large schemas
- Profile with `pprof` if performance issues arise

### DAG Execution
- Parallelize independent nodes
- Use goroutines with semaphore for concurrency control
- Monitor execution time per node
- Add circuit breakers for slow nodes

### Kubernetes API Calls
- Use controller-runtime's caching client
- Batch operations where possible
- Use Server-Side Apply to reduce conflicts
- Monitor API request rate and latency

