# Pequod - CUE-Powered Platform Operator

A Kubernetes operator that enables platform engineering teams to create high-level abstractions using CUE language, with dynamic CRD generation, dependency-aware orchestration, and policy enforcement.

## Project Status

**Active Development** - Core functionality implemented. See [phases.md](phases.md) for detailed development roadmap.

## Overview

Pequod provides a platform engineering tool that:

- **Generates custom CRDs**: Platform engineers define platforms using CUE, Pequod generates CRDs for developers to use
- **Abstracts complexity**: Developers interact with simple, auto-generated CRDs (like `WebService`), not raw Kubernetes YAML
- **Enforces policy**: CUE-based policies validate both inputs and rendered outputs
- **Manages dependencies**: DAG-based execution ensures resources are created in the correct order with readiness gates
- **Supports brownfield**: Adopt existing resources and safely abandon/orphan when needed
- **Multi-cloud ready**: EKS-first with ACK integration, portable to GKE/AKS

## Key Features

### Developer Experience
- Simple, auto-generated CRDs (e.g., `WebService`, `Database`) tailored to your organization
- Developers never write CUE - platform teams own the complexity
- Rich status reporting with per-resource state
- Clear error messages and policy violation feedback

### Platform Engineering
- Define platforms using CUE modules with schema, composition, and policy
- `Transform` resources generate CRDs dynamically from CUE schemas
- Versioned platform modules (embedded or remote via OCI)
- Authoritative reconciliation using Server-Side Apply
- Comprehensive inventory tracking and drift detection

### Orchestration
- Dependency-aware resource application (DAG execution)
- Readiness gates ensure proper ordering (e.g., IAM role before deployment)
- Parallel execution where dependencies allow
- Pluggable readiness predicates for different resource types

## Installation

### Using Kustomize (Recommended)

```bash
# Install CRDs only
kubectl apply -k github.com/chazu/pequod/config/crd?ref=main

# Install full controller (includes CRDs, RBAC, controller)
kubectl apply -k github.com/chazu/pequod/config/default?ref=main
```

Or reference remotely in your kustomization.yaml:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - github.com/chazu/pequod/config/default?ref=main
```

### From Source

```bash
# Install CRDs
make install

# Run controller locally (for development)
make run

# Deploy to cluster
make deploy
```

## Development Setup

### Prerequisites

- Go 1.21+
- kubectl configured with cluster access
- Docker (for building images)

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Run linter
make lint

# Generate CRD manifests and code
make manifests generate
```

### Running Locally

```bash
# Install CRDs to your cluster
make install

# Run the controller locally
make run
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

## Project Structure

```
pequod/
├── api/v1alpha1/          # CRD type definitions (Transform, ResourceGraph)
├── cmd/                   # Main entrypoint
├── config/
│   ├── crd/              # CRD manifests (kustomize base)
│   ├── default/          # Full deployment (kustomize base)
│   ├── manager/          # Controller deployment
│   ├── rbac/             # RBAC configuration
│   └── samples/          # Example resources
├── cue/                   # Example CUE platform modules
├── docs/                  # Additional documentation
├── internal/
│   └── controller/       # Controller implementations
│       ├── transform_controller.go      # Transform → CRD generation
│       └── platforminstance_controller.go  # Instance → ResourceGraph
├── pkg/
│   ├── apply/            # SSA applier and resource adoption
│   ├── crd/              # CRD generation from schemas
│   ├── graph/            # Graph types and DAG executor
│   ├── inventory/        # Resource inventory tracking
│   ├── metrics/          # Prometheus metrics (apply operations)
│   ├── platformloader/   # CUE module loading (embedded, OCI)
│   ├── readiness/        # Readiness predicate evaluation
│   ├── reconcile/        # Reconciliation handlers
│   └── schema/           # CUE to JSONSchema extraction
└── test/                  # Test utilities
```

## Custom Resource Definitions

### Transform (Platform Definition)

Created by **platform engineers** to define a platform type. A Transform references a CUE platform module and generates a CRD:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.mycompany.com  # Optional, defaults to pequod.io
  shortNames: [ws]
```

When applied, Pequod extracts the schema from the CUE module and generates a new CRD (e.g., `WebService`).

### Generated CRDs (Platform Instances)

Created by **developers** using the CRDs generated from Transforms:

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: my-app
  namespace: default
spec:
  image: nginx:latest
  port: 80
  replicas: 3
```

The schema for this CRD is automatically extracted from the CUE module's `#Input` definition.

### ResourceGraph

An intermediate representation created by the platform instance controller. Contains the rendered graph of Kubernetes resources with dependencies:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: ResourceGraph
metadata:
  name: my-app-abc123
spec:
  sourceRef:
    apiVersion: apps.mycompany.com/v1alpha1
    kind: WebService
    name: my-app
  graph:
    nodes:
      - id: deployment-my-app
        resource: {...}
        dependencies: []
```

## Metrics

Pequod exposes Prometheus metrics on `:8443/metrics`. All metrics are prefixed with `pequod_`.

### Reconciliation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pequod_reconcile_total` | Counter | controller, result | Total reconciliations |
| `pequod_reconcile_duration_seconds` | Histogram | controller | Reconciliation duration |
| `pequod_reconcile_errors_total` | Counter | controller, error_type | Reconciliation errors |

### DAG Execution Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pequod_dag_nodes_total` | Gauge | resourcegraph | Nodes in current DAG |
| `pequod_dag_execution_duration_seconds` | Histogram | resourcegraph, result | DAG execution duration |
| `pequod_dag_node_execution_duration_seconds` | Histogram | node_id, result | Per-node execution duration |

### Apply Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pequod_apply_total` | Counter | result, mode, gvk | Apply operations |
| `pequod_apply_duration_seconds` | Histogram | mode, gvk | Apply duration |
| `pequod_resources_managed` | Gauge | gvk, namespace | Managed resource count |

### Adoption Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pequod_adoption_total` | Counter | result | Adoption operations |
| `pequod_adoption_duration_seconds` | Histogram | - | Adoption duration |

### CUE/Platform Loader Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pequod_cue_cache_hits_total` | Counter | - | Cache hits |
| `pequod_cue_cache_misses_total` | Counter | - | Cache misses |
| `pequod_cue_cache_evictions_total` | Counter | - | Cache evictions |
| `pequod_cue_cache_entries` | Gauge | - | Current cache entries |
| `pequod_cue_cache_size_bytes` | Gauge | - | Cache size in bytes |
| `pequod_cue_fetch_duration_seconds` | Histogram | source | Module fetch duration |
| `pequod_cue_fetch_total` | Counter | source, result | Module fetch operations |
| `pequod_cue_render_duration_seconds` | Histogram | platform | CUE render duration |
| `pequod_cue_render_errors_total` | Counter | platform, error_type | Render errors |
| `pequod_policy_violations_total` | Counter | severity | Policy violations |

## Health Endpoints

The controller exposes health endpoints on `:8081`:

- `/healthz` - Liveness probe
- `/readyz` - Readiness probe

## Architecture

```
┌─────────────────┐
│    Transform    │  ← Platform Definition (created by platform engineers)
└────────┬────────┘
         │ extracts schema
         ▼
┌─────────────────┐
│  CUE Platform   │  ← Platform logic (schema, policy, composition)
│     Module      │
└────────┬────────┘
         │ generates
         ▼
┌─────────────────┐
│  Generated CRD  │  ← e.g., WebService, Database CRD
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Platform        │  ← Developer API (instances of generated CRDs)
│ Instance        │
└────────┬────────┘
         │ renders
         ▼
┌─────────────────┐
│  ResourceGraph  │  ← Intermediate representation (nodes + DAG)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  DAG Executor   │  ← Dependency-aware orchestration
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Server-Side     │  ← Authoritative resource management
│     Apply       │
└─────────────────┘
```

## Documentation

- **[docs/tutorial.md](docs/tutorial.md)**: Step-by-step getting started guide
- **[docs/user-guide.md](docs/user-guide.md)**: Complete user documentation and API reference
- **[docs/platform-engineer-guide.md](docs/platform-engineer-guide.md)**: Creating custom CUE platform modules
- **[docs/operations.md](docs/operations.md)**: Deployment, monitoring, and troubleshooting
- **[docs/cue-modules.md](docs/cue-modules.md)**: CUE module format and OCI specification
- **[phases.md](phases.md)**: Development roadmap broken into shippable phases

## Technology Stack

### Core
- **Kubebuilder**: Controller framework and scaffolding
- **controller-runtime**: Kubernetes controller library
- **CUE**: Configuration and policy language

### Key Libraries
- **[github.com/dominikbraun/graph](https://github.com/dominikbraun/graph)**: DAG management with topological sort
- **[cuelang.org/go/cue](https://pkg.go.dev/cuelang.org/go/cue)**: CUE evaluation and validation
- **k8s.io/client-go**: Kubernetes API client with Server-Side Apply
- **github.com/prometheus/client_golang**: Metrics and observability

## Contributing

```bash
# Fork and clone the repository
git clone https://github.com/yourusername/pequod.git
cd pequod

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint
```

## License

Apache License 2.0

## Acknowledgments

This project draws inspiration from:
- **[kro](https://kro.run/)**: Resource graph orchestration patterns
- **[ACK](https://aws-controllers-k8s.github.io/community/)**: AWS resource management in Kubernetes
- **[CUE](https://cuelang.org/)**: Configuration and policy language
- **[Kubebuilder](https://kubebuilder.io/)**: Kubernetes operator framework
