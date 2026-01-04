# Pequod User Guide

This guide covers how to use Pequod to deploy and manage applications as a developer or platform user.

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Transform API Reference](#transform-api-reference)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

Pequod provides a simplified interface for deploying applications on Kubernetes. Instead of writing complex Kubernetes YAML, you create a `Transform` resource that specifies:

1. **What platform type** you want (e.g., `webservice`, `database`)
2. **What configuration** you need (e.g., image, replicas, port)

Pequod then:
- Renders the appropriate Kubernetes resources (Deployments, Services, etc.)
- Applies them in the correct dependency order
- Monitors readiness and reports status

## Installation

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured with cluster access

### Install via Kustomize

```bash
# Install the full operator (CRDs + controller)
kubectl apply -k github.com/chazu/pequod/config/default?ref=main

# Verify installation
kubectl get pods -n pequod-system
```

### Install CRDs Only

If you want to install just the CRDs (for development or GitOps workflows):

```bash
kubectl apply -k github.com/chazu/pequod/config/crd?ref=main
```

### Verify Installation

```bash
# Check CRDs are installed
kubectl get crd transforms.platform.platform.example.com
kubectl get crd resourcegraphs.platform.platform.example.com

# Check controller is running
kubectl get pods -n pequod-system -l control-plane=controller-manager
```

## Quick Start

### 1. Create a WebService

Create a file called `my-app.yaml`:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: my-app
  namespace: default
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    image: nginx:latest
    port: 80
    replicas: 2
```

Apply it:

```bash
kubectl apply -f my-app.yaml
```

### 2. Check Status

```bash
# View Transform status
kubectl get transform my-app -o yaml

# View the generated ResourceGraph
kubectl get resourcegraph -l pequod.io/transform=my-app

# View created resources
kubectl get deployment,service -l app=my-app
```

### 3. Update the Application

Edit `my-app.yaml` to change replicas:

```yaml
spec:
  input:
    replicas: 3  # Changed from 2
```

Apply the changes:

```bash
kubectl apply -f my-app.yaml
```

### 4. Delete the Application

```bash
kubectl delete transform my-app
```

This will automatically clean up all created resources.

## Transform API Reference

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cueRef` | CueReference | Yes | Reference to the CUE platform module |
| `input` | object | Yes | Platform-specific configuration values |
| `adopt` | AdoptSpec | No | Configuration for adopting existing resources |
| `paused` | boolean | No | When true, reconciliation is paused |

### CueReference Types

#### Embedded (Built-in Platforms)

```yaml
spec:
  cueRef:
    type: Embedded
    ref: webservice  # Built-in platform name
```

Available embedded platforms:
- `webservice` - Deployment + Service

#### OCI Registry

```yaml
spec:
  cueRef:
    type: oci
    ref: ghcr.io/myorg/platforms/postgres:v1.0.0
    pullSecretRef:
      name: ghcr-credentials  # Optional for private registries
```

#### Git Repository

```yaml
spec:
  cueRef:
    type: git
    ref: https://github.com/myorg/platforms.git?ref=v1.0.0&path=postgres
    pullSecretRef:
      name: git-credentials  # Optional for private repos
```

#### ConfigMap

```yaml
spec:
  cueRef:
    type: configmap
    ref: my-platform-configmap  # ConfigMap name in same namespace
```

#### Inline CUE

```yaml
spec:
  cueRef:
    type: inline
    ref: |
      #Render: {
        input: {
          metadata: { name: string, namespace: string }
          spec: { message: string }
        }
        output: {
          metadata: { name: input.metadata.name, version: "v1" }
          nodes: [{
            id: "configmap"
            object: {
              apiVersion: "v1"
              kind: "ConfigMap"
              metadata: {
                name: input.metadata.name
                namespace: input.metadata.namespace
              }
              data: { message: input.spec.message }
            }
          }]
        }
      }
```

### WebService Platform Input

The built-in `webservice` platform accepts these inputs:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image` | string | Required | Container image |
| `port` | integer | Required | Service port |
| `replicas` | integer | 1 | Number of replicas |
| `name` | string | Transform name | Resource name prefix |

### Status Fields

| Field | Description |
|-------|-------------|
| `phase` | Current phase: Pending, Rendering, Rendered, Failed |
| `resourceGraphRef` | Reference to the created ResourceGraph |
| `resolvedCueRef` | Resolved CUE module digest and fetch time |
| `conditions` | Detailed condition statuses |
| `violations` | Any policy violations |

## Examples

### Basic WebService

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: nginx-app
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    image: nginx:1.25
    port: 80
    replicas: 3
```

### WebService with Custom Name

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: my-frontend
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    name: frontend-app
    image: my-registry/frontend:v2.0.0
    port: 8080
    replicas: 2
```

### Adopting Existing Resources

If you have existing resources you want Pequod to manage:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: adopt-existing
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    image: nginx:latest
    port: 80
  adopt:
    mode: Explicit
    strategy: TakeOwnership
    resources:
      - apiVersion: apps/v1
        kind: Deployment
        name: existing-deployment
        namespace: default
```

### Pausing Reconciliation

To temporarily stop reconciliation:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: my-app
  labels:
    pequod.io/paused: "true"  # Pauses reconciliation
spec:
  # ... rest of spec
```

## Troubleshooting

### Transform stuck in Pending

**Symptoms**: Transform status shows `phase: Pending` for a long time.

**Causes and Solutions**:

1. **Controller not running**
   ```bash
   kubectl get pods -n pequod-system
   # If no pods, reinstall the operator
   ```

2. **CRD not installed**
   ```bash
   kubectl get crd transforms.platform.platform.example.com
   # If not found, install CRDs
   ```

### Transform shows Failed

**Symptoms**: Transform status shows `phase: Failed` with violations.

**Causes and Solutions**:

1. **Invalid input**
   ```bash
   kubectl get transform my-app -o jsonpath='{.status.violations}'
   ```
   Fix the input values according to the violation messages.

2. **CUE module not found**
   Check the `cueRef` type and ref are correct. For embedded modules, ensure the platform name exists.

### Resources not created

**Symptoms**: Transform shows Rendered but resources don't exist.

**Causes and Solutions**:

1. **Check ResourceGraph status**
   ```bash
   kubectl get resourcegraph -l pequod.io/transform=my-app -o yaml
   ```
   Look at `status.phase` and `status.nodeStates` for errors.

2. **RBAC issues**
   The controller might lack permissions. Check controller logs:
   ```bash
   kubectl logs -n pequod-system -l control-plane=controller-manager
   ```

### Resources not cleaning up on delete

**Symptoms**: After deleting Transform, resources remain.

**Causes and Solutions**:

1. **Check finalizers**
   ```bash
   kubectl get transform my-app -o jsonpath='{.metadata.finalizers}'
   ```
   If stuck, the controller might have issues. Check logs.

2. **Orphaned resources**
   Resources might have been orphaned intentionally. Check if they have owner references:
   ```bash
   kubectl get deployment my-app -o jsonpath='{.metadata.ownerReferences}'
   ```

### Viewing Detailed Logs

```bash
# Controller logs
kubectl logs -n pequod-system -l control-plane=controller-manager -f

# Increase verbosity (edit deployment)
kubectl edit deployment -n pequod-system pequod-controller-manager
# Add to container args: --zap-log-level=2
```

### Common Error Messages

| Error | Meaning | Solution |
|-------|---------|----------|
| `#Render definition not found` | CUE module missing #Render | Check CUE module structure |
| `failed to load embedded CUE module` | Unknown embedded platform | Use valid platform name |
| `field manager conflict` | Another controller manages field | Use adoption to take ownership |
| `resource not found` | Adoption target doesn't exist | Verify resource exists |

## Getting Help

- Check the [operations guide](operations.md) for deployment and monitoring details
- Check the [platform engineer guide](platform-engineer-guide.md) for creating custom platforms
- File issues at https://github.com/chazu/pequod/issues
