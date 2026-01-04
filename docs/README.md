# Pequod Documentation

Welcome to the Pequod documentation! This directory contains detailed guides for platform teams and developers.

## Architecture Overview

Pequod uses a **dynamic CRD generation architecture**:

1. **Platform Engineers** create `Transform` resources that define platform types
2. **Pequod** extracts schemas from CUE modules and generates CRDs (e.g., `WebService`)
3. **Developers** create instances of the generated CRDs
4. **Pequod** renders CUE templates and creates ResourceGraphs
5. **ResourceGraph Controller** applies resources in dependency order

```
Transform (Platform Definition) → Generated CRD → Instance → ResourceGraph → Resources
```

## Quick Answer: How Do Platform Teams Deploy CUE Modules?

Platform teams have **three options** for deploying CUE modules referenced by Transforms:

### 1. Embedded Modules (Recommended for Start)
- **How**: CUE files bundled in operator image using `//go:embed`
- **Update**: Rebuild and redeploy operator
- **Best for**: Initial releases, stable platforms, air-gapped environments
- **Speed**: Instant (no network)

### 2. OCI Registry (Recommended for Production)
- **How**: Package CUE as OCI artifact, push to registry
- **Update**: Push new version, update Transform's `cueRef`
- **Best for**: Production, frequent updates, large organizations
- **Speed**: Fast (cached after first fetch)

### 3. Git Repository (Optional)
- **How**: Store CUE in Git repo, operator clones at runtime
- **Update**: Git commit/tag, update Transform's `cueRef`
- **Best for**: Small teams, GitOps workflows
- **Speed**: Slower (clone overhead)

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

### Step 1: Platform Engineer Creates Transform

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.mycompany.com
  shortNames: [ws]
```

This generates a `WebService` CRD in the `apps.mycompany.com` group.

### Step 2: Developer Creates Platform Instance

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: my-app
  namespace: default
spec:
  image: myregistry.com/my-app:latest
  port: 8080
  replicas: 3
```

The schema for the `spec` is automatically derived from the CUE module's `#Input` definition.

### Using OCI Module

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: oci
    ref: "myregistry.com/platform-modules/webservice@sha256:abc123..."
  group: apps.mycompany.com
```

### Using Git Module

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: git
    ref: "https://github.com/myorg/platform-modules.git?ref=v1.0.0&path=webservice"
  group: apps.mycompany.com
```

## Platform Team Workflow Summary

### Embedded (Simple)

```bash
# 1. Create CUE module
cd cue/platform/webservice/
# ... create schema.cue, render.cue, policy/*.cue ...

# 2. Commit to operator repo
git add cue/platform/webservice
git commit -m "Add webservice platform module"

# 3. Rebuild operator
make docker-build IMG=myregistry.com/pequod:v0.1.0
make docker-push IMG=myregistry.com/pequod:v0.1.0

# 4. Deploy operator
kubectl apply -k config/default

# 5. Create Transform to generate CRD
kubectl apply -f - <<EOF
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.mycompany.com
EOF
```

### OCI (Production)

```bash
# 1. Create CUE module (in separate repo)
cd platform-modules/webservice/
# ... create schema.cue, render.cue with #Input and #Render ...

# 2. Package and push to registry
cue mod publish v1.2.0
# Or use oras for custom packaging

# 3. Create/update Transform
kubectl apply -f - <<EOF
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: oci
    ref: "myregistry.com/platform-modules/webservice:v1.2.0"
  group: apps.mycompany.com
EOF
```

Transform updates automatically regenerate the CRD with the new schema!

## Key Concepts

### Transform
A platform definition created by platform engineers. A Transform:
- References a CUE module containing `#Input` and `#Render` definitions
- Specifies the API group and version for the generated CRD
- Generates a new CRD dynamically from the CUE schema

### Platform Module
A versioned CUE package that defines:
- **#Input**: Schema for the generated CRD's spec (becomes JSONSchema)
- **#Render**: Template that converts instance spec → ResourceGraph
- **Policy**: Constraints on inputs and outputs

### Generated CRD
A Kubernetes CRD dynamically created from a Transform:
- Schema extracted from CUE `#Input` definition
- Instances are watched by the platform instance controller
- Each instance creates a ResourceGraph

### ResourceGraph
The intermediate representation produced by evaluating CUE with instance data:
- **Nodes**: Kubernetes resources to create
- **Dependencies**: Ordering constraints (DAG)
- **Readiness**: When each resource is considered ready
- **Metadata**: Version, hash, timestamp

## Architecture Overview (Detailed)

```
Platform Engineer → Transform → CRD Generated
                                     ↓
Developer → Platform Instance (e.g., WebService CR)
                                     ↓
                        Instance Controller → CUE Module → ResourceGraph
                                                                ↓
                                          ResourceGraph Controller → Resources
```

1. **Platform Engineer** creates Transform referencing a CUE module
2. **Transform Controller** extracts schema from CUE and generates CRD
3. **Developer** creates an instance of the generated CRD
4. **Instance Controller** loads CUE module and renders with instance spec
5. **CUE** produces ResourceGraph with resources and dependencies
6. **ResourceGraph Controller** applies resources in dependency order

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

