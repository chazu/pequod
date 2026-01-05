# Dynamic RBAC Management for Transforms

Pequod supports dynamic RBAC (Role-Based Access Control) management for Transforms. When a Transform declares the Kubernetes resources it will manage, Pequod automatically generates scoped ClusterRoles or Roles with the appropriate permissions.

## Overview

The dynamic RBAC system allows platform engineers to:

1. Declare which Kubernetes resources a Transform's CUE template will create
2. Automatically generate RBAC resources with scoped permissions
3. Choose between cluster-wide or namespace-scoped RBAC

This follows the principle of least privilege - the controller only gets permissions for the specific resources each Transform needs to manage.

## Architecture

```
Transform with managedResources
          │
          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Transform Reconciliation                    │
├─────────────────────────────────────────────────────────────────┤
│  1. Fetch CUE module                                            │
│  2. Extract schema → Generate CRD                               │
│  3. Generate RBAC from managedResources                         │
│     - If scope=Cluster → ClusterRole (aggregated)               │
│     - If scope=Namespace → Role + RoleBinding                   │
│  4. Update status                                               │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────────┐ ┌─────────────────────────────────┐
│  Cluster-Scoped Transform   │ │  Namespace-Scoped Transform     │
├─────────────────────────────┤ ├─────────────────────────────────┤
│  ClusterRole:               │ │  Role (in Transform namespace): │
│    pequod:transform:ns.name │ │    pequod:transform:ns.name     │
│  ├── labels:                │ │  ├── labels:                    │
│  │   pequod.io/aggregate-   │ │  │   pequod.io/                 │
│  │     to-manager: "true"   │ │  │     transform: <name>        │
│  └── rules: [...]           │ │  └── rules: [...]               │
│                             │ │                                 │
│  (Aggregated into manager)  │ │  RoleBinding:                   │
└─────────────────────────────┘ │    pequod:transform:ns.name     │
                                │  ├── roleRef: → Role            │
                                │  └── subjects: → ServiceAccount │
                                └─────────────────────────────────┘
```

## Usage

### Declaring Managed Resources

Add the `managedResources` field to your Transform spec to declare which Kubernetes resources your platform template creates:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
  namespace: default
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.example.com

  # Declare resources this Transform manages
  managedResources:
    - apiGroup: apps
      resources:
        - deployments
    - apiGroup: ""
      resources:
        - services
        - configmaps
```

Each managed resource entry specifies:
- `apiGroup`: The API group of the resource (empty string `""` for core resources)
- `resources`: A list of resource types in that API group

### RBAC Scope

Control where RBAC resources are created using the `rbacScope` field:

```yaml
spec:
  # ... other fields ...

  # Cluster scope (default): Creates a ClusterRole aggregated into manager
  rbacScope: Cluster

  # OR Namespace scope: Creates Role + RoleBinding in Transform's namespace
  rbacScope: Namespace
```

#### Cluster Scope (Default)

When `rbacScope: Cluster`:
- A ClusterRole is created with the name `pequod:transform:<namespace>.<name>`
- The ClusterRole has the label `pequod.io/aggregate-to-manager: "true"`
- Kubernetes automatically aggregates this into the `pequod-manager-aggregate` ClusterRole
- The controller gains cluster-wide permissions for the declared resources

Best for:
- Transforms that manage resources across multiple namespaces
- Platform-wide services that need broad access

#### Namespace Scope

When `rbacScope: Namespace`:
- A Role is created in the Transform's namespace
- A RoleBinding is created binding the Role to the controller's ServiceAccount
- The controller only gains permissions in that specific namespace

Best for:
- Multi-tenant environments
- Transforms that only manage resources in their own namespace
- Tighter security requirements

## ClusterRole Aggregation

For cluster-scoped Transforms, Pequod uses Kubernetes' ClusterRole aggregation feature. This allows dynamic permission expansion without modifying the base manager role.

The aggregate ClusterRole is defined as:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pequod-manager-aggregate
aggregationRule:
  clusterRoleSelectors:
    - matchLabels:
        pequod.io/aggregate-to-manager: "true"
rules: []  # Auto-populated by Kubernetes
```

When you create a Transform with `managedResources`, its generated ClusterRole includes this label and is automatically aggregated.

## Generated RBAC Structure

### ClusterRole Example (scope=Cluster)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pequod:transform:default.webservice
  labels:
    pequod.io/aggregate-to-manager: "true"
    pequod.io/transform: webservice
    pequod.io/transform-namespace: default
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services", "configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Role + RoleBinding Example (scope=Namespace)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pequod:transform:team-a.myapp
  namespace: team-a
  labels:
    pequod.io/transform: myapp
    pequod.io/transform-namespace: team-a
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: pequod:transform:team-a.myapp
  namespace: team-a
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: pequod:transform:team-a.myapp
subjects:
  - kind: ServiceAccount
    name: pequod-controller-manager
    namespace: pequod-system
```

## Status

Transform status includes information about generated RBAC:

```yaml
status:
  phase: Ready
  generatedCRD:
    name: webservices.apps.example.com
    # ...
  generatedRBAC:
    clusterRoleName: "pequod:transform:default.webservice"  # If scope=Cluster
    roleName: ""                                            # If scope=Namespace
    roleBindingName: ""                                     # If scope=Namespace
    ruleCount: 2
```

## Conditions

A new condition `RBACConfigured` indicates RBAC status:

```yaml
conditions:
  - type: RBACConfigured
    status: "True"
    reason: RBACApplied
    message: "ClusterRole pequod:transform:default.webservice configured with 2 rules"
```

## Backwards Compatibility

- Transforms without `managedResources` continue to work as before
- No RBAC resources are generated for these Transforms
- The controller must have pre-existing permissions to manage their resources

## Security Considerations

1. **Principle of Least Privilege**: Only declare resources your Transform actually creates
2. **Scope Selection**: Use `Namespace` scope when possible for tighter security
3. **Review Generated RBAC**: The generated ClusterRoles/Roles are visible in the cluster
4. **Aggregation Security**: Be aware that cluster-scoped Transforms expand the controller's permissions

## Troubleshooting

### RBAC Not Generated

Check that:
1. `managedResources` is defined in the Transform spec
2. The Transform has reconciled successfully (check `status.phase`)
3. Look for errors in the Transform conditions

### Permission Denied Errors

If the controller can't manage resources:
1. Check the generated ClusterRole/Role exists: `kubectl get clusterrole -l pequod.io/transform=<name>`
2. Verify aggregation is working: `kubectl get clusterrole pequod-manager-aggregate -o yaml`
3. Check RoleBinding subjects for namespace-scoped Transforms

### Cleanup

RBAC resources are automatically deleted when the Transform is deleted.
