# Pequod Operations Guide

This guide covers deployment, monitoring, maintenance, and troubleshooting for Pequod operators.

## Table of Contents

- [Deployment](#deployment)
- [Configuration](#configuration)
- [Monitoring and Alerting](#monitoring-and-alerting)
- [Backup and Recovery](#backup-and-recovery)
- [Upgrade Procedures](#upgrade-procedures)
- [Troubleshooting Runbook](#troubleshooting-runbook)

## Deployment

### Prerequisites

- Kubernetes cluster 1.24+
- kubectl with cluster-admin access
- Container registry access (for custom images)

### Installation Methods

#### Kustomize (Recommended)

```bash
# Standard installation
kubectl apply -k github.com/chazu/pequod/config/default?ref=v0.1.0

# Or reference a specific version in kustomization.yaml
cat > kustomization.yaml << 'EOF'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - github.com/chazu/pequod/config/default?ref=v0.1.0
EOF
kubectl apply -k .
```

#### Customized Deployment

Create a kustomization with patches:

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - github.com/chazu/pequod/config/default?ref=v0.1.0

namespace: pequod-system

patches:
  # Custom resource limits
  - patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: pequod-controller-manager
      spec:
        template:
          spec:
            containers:
            - name: manager
              resources:
                limits:
                  cpu: "1"
                  memory: 512Mi
                requests:
                  cpu: 100m
                  memory: 128Mi

  # Custom replicas
  - patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: pequod-controller-manager
      spec:
        replicas: 2
```

### Namespace Configuration

By default, Pequod installs into `pequod-system`. To use a different namespace:

```yaml
# kustomization.yaml
namespace: my-operators
resources:
  - github.com/chazu/pequod/config/default?ref=v0.1.0
```

### RBAC Requirements

Pequod requires broad RBAC permissions because it creates arbitrary Kubernetes resources. The default RBAC includes:

```yaml
# ResourceGraph controller needs broad permissions
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["create", "update", "patch", "delete", "get", "list", "watch"]
```

For restricted environments, you can scope permissions to specific resource types:

```yaml
# Restrict to specific API groups
- apiGroups: ["", "apps", "networking.k8s.io"]
  resources: ["deployments", "services", "configmaps", "secrets", "ingresses"]
  verbs: ["create", "update", "patch", "delete", "get", "list", "watch"]
```

## Configuration

### Controller Arguments

The controller accepts these command-line arguments:

| Argument | Default | Description |
|----------|---------|-------------|
| `--metrics-bind-address` | `:8443` | Metrics endpoint address |
| `--health-probe-bind-address` | `:8081` | Health probe address |
| `--leader-elect` | `false` | Enable leader election |
| `--zap-log-level` | `info` | Log level (debug, info, error) |
| `--zap-encoder` | `json` | Log format (json, console) |

To modify, patch the Deployment:

```yaml
- patch: |-
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: pequod-controller-manager
    spec:
      template:
        spec:
          containers:
          - name: manager
            args:
            - --leader-elect=true
            - --zap-log-level=debug
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `PEQUOD_CACHE_DIR` | CUE module cache directory (default: `/tmp/pequod-cache`) |
| `PEQUOD_CACHE_SIZE_MB` | Maximum cache size in MB (default: 100) |
| `PEQUOD_CACHE_TTL_HOURS` | Cache entry TTL in hours (default: 24) |

## Monitoring and Alerting

### Metrics Endpoint

Metrics are exposed at `:8443/metrics` in Prometheus format.

### Key Metrics

#### Reconciliation Health

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `pequod_reconcile_total{result="error"}` | Counter | Failed reconciliations | >10/min |
| `pequod_reconcile_duration_seconds` | Histogram | Reconcile latency | p99 > 30s |

#### DAG Execution

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `pequod_dag_execution_duration_seconds` | Histogram | DAG execution time | p99 > 5min |
| `pequod_dag_nodes_total` | Gauge | Nodes per ResourceGraph | >100 (warning) |

#### Apply Operations

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `pequod_apply_total{result="failure"}` | Counter | Failed applies | >5/min |
| `pequod_apply_duration_seconds` | Histogram | Apply latency | p99 > 10s |

#### CUE Module Cache

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `pequod_cue_cache_hits_total` | Counter | Cache hits | - |
| `pequod_cue_cache_misses_total` | Counter | Cache misses | miss rate >50% |
| `pequod_cue_fetch_duration_seconds` | Histogram | Module fetch time | p99 > 30s |
| `pequod_cue_render_errors_total` | Counter | CUE render errors | >1/min |

### Prometheus Alerting Rules

```yaml
groups:
  - name: pequod
    rules:
      - alert: PequodReconcileErrors
        expr: rate(pequod_reconcile_total{result="error"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High rate of reconcile errors
          description: Pequod is experiencing {{ $value }} errors/sec

      - alert: PequodApplyFailures
        expr: rate(pequod_apply_total{result="failure"}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High rate of apply failures

      - alert: PequodSlowReconcile
        expr: histogram_quantile(0.99, rate(pequod_reconcile_duration_seconds_bucket[5m])) > 60
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Slow reconciliation detected

      - alert: PequodCacheMissRate
        expr: >
          rate(pequod_cue_cache_misses_total[5m]) /
          (rate(pequod_cue_cache_hits_total[5m]) + rate(pequod_cue_cache_misses_total[5m])) > 0.5
        for: 15m
        labels:
          severity: info
        annotations:
          summary: High cache miss rate

      - alert: PequodControllerDown
        expr: up{job="pequod-controller-manager"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Pequod controller is down
```

### Grafana Dashboard

Key panels for a Pequod dashboard:

1. **Reconciliation Rate**: `rate(pequod_reconcile_total[5m])`
2. **Error Rate**: `rate(pequod_reconcile_total{result="error"}[5m])`
3. **P99 Latency**: `histogram_quantile(0.99, rate(pequod_reconcile_duration_seconds_bucket[5m]))`
4. **Active ResourceGraphs**: `count(pequod_dag_nodes_total)`
5. **Cache Hit Rate**: `rate(pequod_cue_cache_hits_total[5m]) / (rate(pequod_cue_cache_hits_total[5m]) + rate(pequod_cue_cache_misses_total[5m]))`

### Health Endpoints

| Endpoint | Purpose | Expected Response |
|----------|---------|-------------------|
| `:8081/healthz` | Liveness probe | `200 OK` |
| `:8081/readyz` | Readiness probe | `200 OK` |

Configure in Deployment:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 15
  periodSeconds: 20

readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Backup and Recovery

### What to Backup

1. **CRDs**: Transform and ResourceGraph definitions
2. **Custom Resources**: All Transform and ResourceGraph objects
3. **ConfigMaps**: Any CUE modules stored in ConfigMaps

### Backup Commands

```bash
# Backup all Transforms
kubectl get transform -A -o yaml > transforms-backup.yaml

# Backup all ResourceGraphs
kubectl get resourcegraph -A -o yaml > resourcegraphs-backup.yaml

# Backup CUE ConfigMaps
kubectl get configmap -l pequod.io/cue-module=true -A -o yaml > cue-modules-backup.yaml
```

### Recovery Procedure

1. **Install CRDs first**:
   ```bash
   kubectl apply -k github.com/chazu/pequod/config/crd?ref=v0.1.0
   ```

2. **Restore Transforms** (controller not required):
   ```bash
   kubectl apply -f transforms-backup.yaml
   ```

3. **Install Controller**:
   ```bash
   kubectl apply -k github.com/chazu/pequod/config/default?ref=v0.1.0
   ```

4. **Verify Reconciliation**:
   ```bash
   kubectl get transform -A
   kubectl get resourcegraph -A
   ```

### Disaster Recovery

If the controller is deleted but CRDs remain:

1. Transforms and ResourceGraphs persist
2. Managed resources continue running (no drift correction)
3. Reinstall controller to resume management

## Upgrade Procedures

### Pre-Upgrade Checklist

1. [ ] Review release notes for breaking changes
2. [ ] Backup all Transforms and ResourceGraphs
3. [ ] Test upgrade in non-production environment
4. [ ] Notify users of potential reconciliation pause

### Upgrade Steps

```bash
# 1. Check current version
kubectl get deployment -n pequod-system pequod-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].image}'

# 2. Apply new version
kubectl apply -k github.com/chazu/pequod/config/default?ref=v0.2.0

# 3. Wait for rollout
kubectl rollout status deployment -n pequod-system pequod-controller-manager

# 4. Verify health
kubectl get pods -n pequod-system
kubectl logs -n pequod-system -l control-plane=controller-manager --tail=50

# 5. Verify reconciliation
kubectl get transform -A -o wide
```

### Rollback

```bash
# Rollback to previous version
kubectl apply -k github.com/chazu/pequod/config/default?ref=v0.1.0

# Or use kubectl rollout
kubectl rollout undo deployment -n pequod-system pequod-controller-manager
```

### CRD Upgrades

CRD changes require special handling:

```bash
# Check for CRD changes in release notes

# Apply CRD updates first
kubectl apply -k github.com/chazu/pequod/config/crd?ref=v0.2.0

# Then upgrade controller
kubectl apply -k github.com/chazu/pequod/config/default?ref=v0.2.0
```

## Troubleshooting Runbook

### Controller Not Starting

**Symptoms**: Pod in CrashLoopBackOff or Pending

**Diagnosis**:
```bash
kubectl describe pod -n pequod-system -l control-plane=controller-manager
kubectl logs -n pequod-system -l control-plane=controller-manager --previous
```

**Common Causes**:
1. **Resource limits too low**: Increase memory/CPU limits
2. **RBAC issues**: Check ClusterRole and ClusterRoleBinding
3. **CRD not installed**: Install CRDs first
4. **Port conflict**: Check if ports 8081/8443 are in use

### Transforms Stuck in Pending

**Symptoms**: Transform status shows Pending indefinitely

**Diagnosis**:
```bash
kubectl get transform <name> -o yaml
kubectl describe transform <name>
kubectl logs -n pequod-system -l control-plane=controller-manager | grep <name>
```

**Common Causes**:
1. **Controller not running**: Check controller pod status
2. **CUE module not found**: Verify cueRef type and ref
3. **Rate limiting**: Check for high reconcile queue depth

### Resources Not Being Created

**Symptoms**: Transform shows Rendered but no resources exist

**Diagnosis**:
```bash
kubectl get resourcegraph -l pequod.io/transform=<name> -o yaml
kubectl describe resourcegraph <resourcegraph-name>
```

**Common Causes**:
1. **RBAC insufficient**: Controller lacks permission to create resource type
2. **Node errors**: Check `status.nodeStates` for individual node errors
3. **Dependency cycle**: Check for circular dependencies in graph

### High Memory Usage

**Symptoms**: Controller OOMKilled or high memory metrics

**Diagnosis**:
```bash
kubectl top pod -n pequod-system
kubectl get events -n pequod-system --sort-by='.lastTimestamp'
```

**Solutions**:
1. Increase memory limits
2. Check for large CUE modules (>1MB)
3. Reduce cache size via `PEQUOD_CACHE_SIZE_MB`
4. Check for ResourceGraphs with many nodes (>50)

### Slow Reconciliation

**Symptoms**: High `pequod_reconcile_duration_seconds` latency

**Diagnosis**:
```bash
# Check reconcile queue depth
kubectl logs -n pequod-system -l control-plane=controller-manager | grep "queue depth"

# Check for slow operations
kubectl logs -n pequod-system -l control-plane=controller-manager | grep -E "(slow|timeout)"
```

**Solutions**:
1. Check for external dependencies (OCI registry, Git)
2. Enable debug logging to identify slow steps
3. Check Kubernetes API server latency
4. Consider horizontal scaling with leader election

### CUE Module Fetch Failures

**Symptoms**: `pequod_cue_fetch_total{result="failure"}` increasing

**Diagnosis**:
```bash
kubectl describe transform <name>  # Check conditions
kubectl logs -n pequod-system -l control-plane=controller-manager | grep "fetch"
```

**Common Causes**:
1. **Network issues**: Check egress to registry/Git
2. **Authentication**: Verify pull secret exists and is valid
3. **Module not found**: Verify ref is correct
4. **Rate limiting**: Registry may be rate limiting

### Events Reference

| Event | Meaning | Action |
|-------|---------|--------|
| `CueFetchFailed` | Failed to fetch CUE module | Check ref and credentials |
| `CueRenderFailed` | CUE evaluation error | Check CUE module for errors |
| `PolicyViolation` | Input failed policy check | Fix input or update policy |
| `ApplyFailed` | Failed to apply resource | Check RBAC and resource spec |
| `ReadinessTimeout` | Resource didn't become ready | Check resource status |
| `AdoptionFailed` | Failed to adopt resource | Check resource exists and permissions |

## Support

For issues not covered here:

1. Enable debug logging: `--zap-log-level=debug`
2. Collect logs: `kubectl logs -n pequod-system -l control-plane=controller-manager > pequod.log`
3. File issue: https://github.com/chazu/pequod/issues
