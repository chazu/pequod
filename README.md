# Pequod - CUE-Powered Platform Operator

A Kubernetes operator that enables platform engineering teams to create high-level abstractions using CUE language, with dependency-aware orchestration and policy enforcement.

## Project Status

🚧 **In Planning Phase** - See [phases.md](phases.md) for detailed development roadmap.

## Overview

Pequod provides a platform engineering tool that:

- **Abstracts complexity**: Developers interact with simple CRDs (like `WebService`), not raw Kubernetes YAML
- **Enforces policy**: CUE-based policies validate both inputs and rendered outputs
- **Manages dependencies**: DAG-based execution ensures resources are created in the correct order with readiness gates
- **Supports brownfield**: Adopt existing resources and safely abandon/orphan when needed
- **Multi-cloud ready**: EKS-first with ACK integration, portable to GKE/AKS

## Key Features

### Developer Experience
- Simple, stable CRDs (starting with `WebService`)
- Developers never write CUE - platform teams own the complexity
- Rich status reporting with per-resource state
- Clear error messages and policy violation feedback

### Platform Engineering
- CUE-based platform modules for schema, composition, and policy
- Versioned platform modules (embedded or remote via OCI/Git)
- Authoritative reconciliation using Server-Side Apply
- Comprehensive inventory tracking and drift detection

### Orchestration
- Dependency-aware resource application (DAG execution)
- Readiness gates ensure proper ordering (e.g., IAM role before deployment)
- Parallel execution where dependencies allow
- Pluggable readiness predicates for different resource types

### Cloud Integration
- EKS/ACK integration for AWS resources (IAM, etc.)
- Capability detection for graceful degradation
- Portable across managed Kubernetes (GKE, AKS)

## Architecture

```
┌─────────────────┐
│  WebService CRD │  ← Developer API (simple, stable)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  CUE Platform   │  ← Platform logic (schema, policy, composition)
│     Module      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Graph Artifact │  ← Intermediate representation (nodes + DAG)
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

- **[plan.md](plan.md)**: Detailed architectural specification and technical assessment
- **[phases.md](phases.md)**: Development roadmap broken into shippable phases
- **docs/** (coming soon): User guides, tutorials, and operational documentation

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

See [phases.md](phases.md) for complete technology stack details.

## Development Roadmap

### Phase 0-1: Foundation (Weeks 1-2)
- Project setup with Kubebuilder
- WebService CRD definition
- Core types and Graph artifact

### Phase 2-4: Core Engine (Weeks 3-6)
- CUE integration and rendering
- DAG executor with readiness gates
- Server-Side Apply integration
- Basic controller implementation

### Phase 5-7: Testing & DX (Weeks 6-9)
- Comprehensive testing (unit, integration, E2E)
- Observability (metrics, events, status)
- Graph artifact storage

### Phase 8-10: Advanced Features (Weeks 9-12)
- Resource adoption
- Remote platform modules (OCI/Git)
- EKS/ACK integration

### Phase 11-12: Production Ready (Weeks 12-14)
- Leader election and HA
- Security hardening
- Documentation and release

**Target**: v0.1.0 in ~14 weeks with 2-3 engineers

## Quick Start (Coming Soon)

Installation and usage instructions will be available after Phase 6.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (coming soon) for development setup and guidelines.

## License

TBD

## Acknowledgments

This project draws inspiration from:
- **[kro](https://kro.run/)**: Resource graph orchestration patterns
- **[ACK](https://aws-controllers-k8s.github.io/community/)**: AWS resource management in Kubernetes
- **[CUE](https://cuelang.org/)**: Configuration and policy language
- **[Kubebuilder](https://kubebuilder.io/)**: Kubernetes operator framework

