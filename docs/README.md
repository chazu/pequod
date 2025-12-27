# Pequod Documentation

Welcome to the Pequod documentation! This directory contains detailed guides for platform teams and developers.

## Quick Answer: How Do Platform Teams Deploy CUE Modules?

Platform teams have **three options**:

### 1. Embedded Modules (v0.1.0 - Recommended for Start)
- **How**: CUE files bundled in operator image using `//go:embed`
- **Update**: Rebuild and redeploy operator
- **Best for**: Initial releases, stable platforms, air-gapped environments
- **Speed**: ⚡ Instant (no network)

### 2. OCI Registry (v0.2.0+ - Recommended for Production)
- **How**: Package CUE as OCI artifact, push to registry
- **Update**: Push new version, developers update `platformRef`
- **Best for**: Production, frequent updates, large organizations
- **Speed**: 🚀 Fast (cached after first fetch)

### 3. Git Repository (v0.3.0+ - Optional)
- **How**: Store CUE in Git repo, operator clones at runtime
- **Update**: Git commit/tag, developers update `platformRef`
- **Best for**: Small teams, GitOps workflows
- **Speed**: 🐌 Slower (clone overhead)

## Documentation Index

### For Platform Engineers

- **[Platform Module Delivery](platform-module-delivery.md)** - Complete guide to all three delivery methods
  - Embedded modules workflow
  - OCI registry workflow
  - Git repository workflow
  - Comparison and recommendations

- **[Platform Module Workflow](platform-module-workflow.md)** - Visual diagrams and flows
  - Workflow diagrams for each method
  - Module resolution flow
  - Status tracking
  - Migration path

### For Developers

- **[WebService API Reference](webservice-api.md)** *(Coming in Phase 6)*
  - WebService CRD specification
  - Field descriptions
  - Examples

- **[Tutorial](tutorial.md)** *(Coming in Phase 12)*
  - Getting started guide
  - Step-by-step walkthrough
  - Common patterns

### For Operators

- **[Operations Guide](operations.md)** *(Coming in Phase 12)*
  - Installation and deployment
  - Monitoring and alerting
  - Troubleshooting
  - Upgrade procedures

## Quick Examples

### Embedded Module (Phase 1)

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    embedded: v1.0.0  # References embedded module
  image: myregistry.com/my-app:latest
  port: 8080
  replicas: 3
```

### OCI Module (Phase 9)

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    # Use digest for immutability
    oci: "myregistry.com/platform-modules/webservice@sha256:abc123..."
  image: myregistry.com/my-app:latest
  port: 8080
  replicas: 3
```

### Git Module (Phase 9)

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    # Use commit SHA for immutability
    git: "https://github.com/myorg/platform-modules.git?ref=abc123&path=webservice/v1.0.0"
  image: myregistry.com/my-app:latest
  port: 8080
  replicas: 3
```

## Platform Team Workflow Summary

### Embedded (Simple)

```bash
# 1. Create CUE module
cd cue/platform/v1.0.0/
# ... create schema.cue, render.cue, policy/*.cue ...

# 2. Commit to operator repo
git add cue/platform/v1.0.0
git commit -m "Add platform module v1.0.0"

# 3. Rebuild operator
make docker-build IMG=myregistry.com/pequod:v0.1.0
make docker-push IMG=myregistry.com/pequod:v0.1.0

# 4. Deploy operator
kubectl set image deployment/pequod-controller-manager \
    manager=myregistry.com/pequod:v0.1.0
```

### OCI (Production)

```bash
# 1. Create CUE module (in separate repo)
cd platform-modules/webservice/v1.2.0/
# ... create schema.cue, render.cue, policy/*.cue ...

# 2. Package and push to registry
docker build -t myregistry.com/platform-modules/webservice:v1.2.0 .
docker push myregistry.com/platform-modules/webservice:v1.2.0

# 3. Get digest
DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' \
    myregistry.com/platform-modules/webservice:v1.2.0)

# 4. Announce to developers
echo "New module available: $DIGEST"
```

Developers update their WebService CRs independently - no operator restart needed!

## Key Concepts

### Platform Module
A versioned CUE package that defines:
- **Schema**: Input validation for WebService
- **Rendering**: How to convert WebService → Kubernetes resources
- **Policy**: Constraints on inputs and outputs
- **Graph**: Dependencies and readiness rules

### Graph Artifact
The intermediate representation produced by evaluating CUE:
- **Nodes**: Kubernetes resources to create
- **Dependencies**: Ordering constraints (DAG)
- **Readiness**: When each resource is considered ready
- **Metadata**: Version, hash, timestamp

### Platform Reference
How developers specify which module version to use:
- `embedded: v1.0.0` - Bundled with operator
- `oci: registry/module@sha256:...` - From OCI registry
- `git: github.com/org/repo?ref=...` - From Git repo

## Architecture Overview

```
Developer → WebService CR → Operator → CUE Module → Graph → Resources
                                ↓
                          Platform Team
                          (creates modules)
```

1. **Platform Team** creates CUE modules defining platform abstractions
2. **Developer** creates WebService CR with simple spec
3. **Operator** loads CUE module and evaluates it with WebService spec
4. **CUE** produces Graph artifact with resources and dependencies
5. **Operator** applies resources in dependency order with readiness gates

## Next Steps

- **Platform Engineers**: Read [platform-module-delivery.md](platform-module-delivery.md)
- **Developers**: Wait for Phase 6, then read tutorial
- **Operators**: Wait for Phase 12, then read operations guide

## Contributing

See the main [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.

## Questions?

- Check [platform-module-delivery.md](platform-module-delivery.md) for detailed workflows
- Review [platform-module-workflow.md](platform-module-workflow.md) for visual diagrams
- See [../phases.md](../phases.md) for implementation timeline

