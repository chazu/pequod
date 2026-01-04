# Pequod User Guide

This guide covers how to use Pequod to deploy and manage applications as a developer or platform user.

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Platform Instance API](#platform-instance-api)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

Pequod provides a simplified interface for deploying applications on Kubernetes. Your platform engineering team has created custom CRDs (Custom Resource Definitions) that are tailored to your organization's needs.

Instead of writing complex Kubernetes YAML, you create instances of these platform CRDs (e.g., `WebService`, `Database`) with a simple spec:

1. **Find available platform types** - Your platform team has defined what's available
2. **Create an instance** with the configuration you need (e.g., image, replicas, port)

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

# List available platform types (generated CRDs)
kubectl get crd -l pequod.io/managed=true
```

## Quick Start

### 1. Discover Available Platform Types

Your platform team creates `Transform` resources that generate CRDs for you to use:

```bash
# List available platform types
kubectl get transforms

# Example output:
# NAME          PHASE   CRD                                AGE
# webservice    Ready   webservices.apps.mycompany.com     5d
# database      Ready   databases.apps.mycompany.com       3d
```

### 2. Create a Platform Instance

Once you know what platform types are available, create an instance. For example, if `webservice` is available:

Create a file called `my-app.yaml`:

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: my-app
  namespace: default
spec:
  image: nginx:latest
  port: 80
  replicas: 2
```

Apply it:

```bash
kubectl apply -f my-app.yaml
```

### 3. Check Status

```bash
# View instance status
kubectl get webservice my-app -o yaml

# View the generated ResourceGraph
kubectl get resourcegraph -l pequod.io/instance=my-app

# View created resources
kubectl get deployment,service -l app.kubernetes.io/instance=my-app
```

### 4. Update the Application

Edit `my-app.yaml` to change replicas:

```yaml
spec:
  replicas: 3  # Changed from 2
```

Apply the changes:

```bash
kubectl apply -f my-app.yaml
```

### 5. Delete the Application

```bash
kubectl delete webservice my-app
```

This will automatically clean up all created resources.

## Platform Instance API

Platform instances are created using the CRDs generated from Transform resources. The schema for each platform type is defined by your platform engineering team.

### Discovering Available Fields

To see what fields are available for a platform type:

```bash
# Get the CRD schema
kubectl explain webservice.spec

# Get detailed field information
kubectl explain webservice.spec.replicas
```

### Common Platform Instance Fields

Most platform instances will have a `spec` section with fields defined by the CUE module's `#Input` definition. Common patterns include:

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | Container image to deploy |
| `port` | integer | Service port |
| `replicas` | integer | Number of replicas |

The exact fields depend on your platform team's CUE module definitions.

### WebService Platform Example

If your platform team has created a `webservice` Transform, the generated CRD might accept:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image` | string | Required | Container image |
| `port` | integer | Required | Service port (1-65535) |
| `replicas` | integer | 1 | Number of replicas |

### Instance Status Fields

Platform instances have status fields that show the current state:

| Field | Description |
|-------|-------------|
| `phase` | Current phase: Pending, Rendering, Ready, Failed |
| `resourceGraphRef` | Reference to the created ResourceGraph |
| `conditions` | Detailed condition statuses |

### Viewing Instance Status

```bash
# Get instance status
kubectl get webservice my-app -o yaml

# Check conditions
kubectl get webservice my-app -o jsonpath='{.status.conditions}'
```

## Examples

### Basic WebService

Assuming your platform team has created a `webservice` Transform that generates a `WebService` CRD:

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: nginx-app
  namespace: default
spec:
  image: nginx:1.25
  port: 80
  replicas: 3
```

### WebService with Minimal Configuration

Using defaults defined by your platform team:

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: simple-app
spec:
  image: my-registry/my-app:v1.0.0
  port: 8080
  # replicas defaults to value defined in CUE module
```

### Multiple Instances

You can create multiple instances of the same platform type:

```yaml
---
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: frontend
  namespace: production
spec:
  image: my-registry/frontend:v2.0.0
  port: 3000
  replicas: 3
---
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: backend
  namespace: production
spec:
  image: my-registry/backend:v2.0.0
  port: 8080
  replicas: 5
```

### Pausing Reconciliation

To temporarily stop reconciliation on an instance:

```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: my-app
  labels:
    pequod.io/paused: "true"  # Pauses reconciliation
spec:
  image: nginx:latest
  port: 80
```

## Troubleshooting

### Instance stuck in Pending

**Symptoms**: Instance status shows `phase: Pending` for a long time.

**Causes and Solutions**:

1. **Controller not running**
   ```bash
   kubectl get pods -n pequod-system
   # If no pods, reinstall the operator
   ```

2. **Transform not ready**
   The Transform that generates your CRD might not be ready:
   ```bash
   kubectl get transforms
   # Check that the Transform for your platform type shows Phase: Ready
   ```

### Instance shows Failed

**Symptoms**: Instance status shows `phase: Failed`.

**Causes and Solutions**:

1. **Invalid input**
   ```bash
   kubectl describe webservice my-app
   ```
   Check the conditions and events for validation errors.

2. **Policy violations**
   Your platform team may have defined policies that your input violates.
   Check the status conditions for violation messages.

### Resources not created

**Symptoms**: Instance shows Ready but resources don't exist.

**Causes and Solutions**:

1. **Check ResourceGraph status**
   ```bash
   kubectl get resourcegraph -l pequod.io/instance=my-app -o yaml
   ```
   Look at `status.phase` and `status.nodeStates` for errors.

2. **RBAC issues**
   The controller might lack permissions. Check controller logs:
   ```bash
   kubectl logs -n pequod-system -l control-plane=controller-manager
   ```

### Resources not cleaning up on delete

**Symptoms**: After deleting instance, resources remain.

**Causes and Solutions**:

1. **Check finalizers**
   ```bash
   kubectl get webservice my-app -o jsonpath='{.metadata.finalizers}'
   ```
   If stuck, the controller might have issues. Check logs.

2. **Orphaned resources**
   Resources might have been orphaned intentionally. Check if they have owner references:
   ```bash
   kubectl get deployment my-app -o jsonpath='{.metadata.ownerReferences}'
   ```

### Platform type not available

**Symptoms**: `error: the server doesn't have a resource type "webservice"`

**Causes and Solutions**:

1. **Transform not created**
   Your platform team needs to create a Transform first:
   ```bash
   kubectl get transforms
   ```

2. **Transform not ready**
   The Transform might still be generating the CRD:
   ```bash
   kubectl get transform webservice -o yaml
   ```
   Wait for `phase: Ready`.

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
| `#Render definition not found` | CUE module missing #Render | Contact platform team |
| `field manager conflict` | Another controller manages field | Contact platform team |
| `resource not found` | Referenced resource doesn't exist | Check your input values |
| `validation failed` | Input doesn't match schema | Check field types and constraints |

## Getting Help

- Check the [operations guide](operations.md) for deployment and monitoring details
- Contact your platform engineering team for platform-specific questions
- File issues at https://github.com/chazu/pequod/issues
