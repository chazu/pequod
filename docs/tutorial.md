# Pequod Tutorial: Getting Started

This tutorial walks you through deploying your first application with Pequod, from installation to cleanup.

## Prerequisites

Before you begin, ensure you have:

- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) or another Kubernetes cluster
- [kubectl](https://kubernetes.io/docs/tasks/tools/) configured for your cluster

## Step 1: Create a Kubernetes Cluster

If you don't have a cluster, create one with kind:

```bash
# Create a cluster
kind create cluster --name pequod-tutorial

# Verify it's working
kubectl cluster-info
kubectl get nodes
```

Expected output:
```
Kubernetes control plane is running at https://127.0.0.1:xxxxx
...

NAME                           STATUS   ROLES           AGE   VERSION
pequod-tutorial-control-plane  Ready    control-plane   1m    v1.28.0
```

## Step 2: Install Pequod

Install the Pequod operator:

```bash
# Install CRDs and controller
kubectl apply -k github.com/chazu/pequod/config/default?ref=main

# Wait for the controller to be ready
kubectl wait --for=condition=available deployment/pequod-controller-manager \
  -n pequod-system --timeout=60s
```

Verify the installation:

```bash
# Check the controller is running
kubectl get pods -n pequod-system

# Check CRDs are installed
kubectl get crd transforms.platform.platform.example.com
```

Expected output:
```
NAME                                       READY   STATUS    RESTARTS   AGE
pequod-controller-manager-xxxxxxx-xxxxx    1/1     Running   0          30s

NAME                                            CREATED AT
transforms.platform.platform.example.com        2024-01-01T00:00:00Z
```

## Step 3: Create Your First Transform

Create a file called `my-first-app.yaml`:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: my-first-app
  namespace: default
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    image: nginx:1.25-alpine
    port: 80
    replicas: 2
```

Apply it:

```bash
kubectl apply -f my-first-app.yaml
```

Expected output:
```
transform.platform.platform.example.com/my-first-app created
```

## Step 4: Watch the Reconciliation

Watch the Transform status:

```bash
kubectl get transform my-first-app -w
```

You'll see the status progress:
```
NAME           PHASE      AGE
my-first-app   Pending    0s
my-first-app   Rendering  1s
my-first-app   Rendered   2s
```

Press `Ctrl+C` to stop watching.

## Step 5: Explore What Was Created

### View the ResourceGraph

Pequod creates a ResourceGraph that contains the rendered resources:

```bash
kubectl get resourcegraph -l pequod.io/transform=my-first-app
```

Expected output:
```
NAME                     PHASE       NODES   AGE
my-first-app-abc123xyz   Completed   2       30s
```

View the full ResourceGraph:

```bash
kubectl get resourcegraph -l pequod.io/transform=my-first-app -o yaml
```

### View the Created Resources

Pequod created a Deployment and a Service:

```bash
kubectl get deployment,service -l app.kubernetes.io/name=my-first-app
```

Expected output:
```
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/my-first-app   2/2     2            2           1m

NAME                   TYPE        CLUSTER-IP      PORT(S)   AGE
service/my-first-app   ClusterIP   10.96.xxx.xxx   80/TCP    1m
```

### Check the Pods

```bash
kubectl get pods -l app.kubernetes.io/name=my-first-app
```

Expected output:
```
NAME                            READY   STATUS    RESTARTS   AGE
my-first-app-xxxxxxxxx-xxxxx    1/1     Running   0          1m
my-first-app-xxxxxxxxx-yyyyy    1/1     Running   0          1m
```

## Step 6: Access Your Application

Port-forward to test the application:

```bash
kubectl port-forward svc/my-first-app 8080:80 &
curl http://localhost:8080
```

You should see the nginx welcome page HTML.

Stop the port-forward:
```bash
kill %1
```

## Step 7: Update the Application

Let's scale up to 3 replicas. Edit `my-first-app.yaml`:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: my-first-app
  namespace: default
spec:
  cueRef:
    type: Embedded
    ref: webservice
  input:
    image: nginx:1.25-alpine
    port: 80
    replicas: 3  # Changed from 2 to 3
```

Apply the change:

```bash
kubectl apply -f my-first-app.yaml
```

Watch the update:

```bash
kubectl get pods -l app.kubernetes.io/name=my-first-app -w
```

You'll see a new pod being created:
```
NAME                            READY   STATUS    RESTARTS   AGE
my-first-app-xxxxxxxxx-xxxxx    1/1     Running   0          2m
my-first-app-xxxxxxxxx-yyyyy    1/1     Running   0          2m
my-first-app-xxxxxxxxx-zzzzz    0/1     Pending   0          0s
my-first-app-xxxxxxxxx-zzzzz    1/1     Running   0          5s
```

Press `Ctrl+C` to stop watching.

## Step 8: View Status and Debug

### Transform Status

Get detailed Transform status:

```bash
kubectl get transform my-first-app -o yaml
```

Key status fields:
```yaml
status:
  phase: Rendered
  observedGeneration: 2
  resourceGraphRef:
    name: my-first-app-abc123xyz
    namespace: default
  resolvedCueRef:
    digest: embedded:webservice
    fetchedAt: "2024-01-01T00:00:00Z"
  conditions:
  - type: Ready
    status: "True"
    reason: Reconciled
    message: Successfully reconciled
```

### ResourceGraph Status

Check the node execution states:

```bash
kubectl get resourcegraph -l pequod.io/transform=my-first-app \
  -o jsonpath='{.items[0].status.nodeStates}' | jq .
```

Expected output:
```json
{
  "deployment": {
    "phase": "Ready",
    "message": "Deployment available",
    "appliedAt": "2024-01-01T00:00:00Z",
    "readyAt": "2024-01-01T00:00:10Z"
  },
  "service": {
    "phase": "Ready",
    "message": "Resource exists",
    "appliedAt": "2024-01-01T00:00:01Z",
    "readyAt": "2024-01-01T00:00:01Z"
  }
}
```

### Controller Logs

View the controller logs for debugging:

```bash
kubectl logs -n pequod-system -l control-plane=controller-manager --tail=50
```

## Step 9: Pause Reconciliation (Optional)

You can pause reconciliation to make manual changes:

```bash
kubectl label transform my-first-app pequod.io/paused=true
```

Verify it's paused:
```bash
kubectl get transform my-first-app -o jsonpath='{.metadata.labels}'
```

Resume reconciliation:
```bash
kubectl label transform my-first-app pequod.io/paused-
```

## Step 10: Delete the Application

Clean up by deleting the Transform:

```bash
kubectl delete transform my-first-app
```

Verify everything is deleted:

```bash
# Transform should be gone
kubectl get transform my-first-app
# Expected: Error from server (NotFound)

# ResourceGraph should be gone
kubectl get resourcegraph -l pequod.io/transform=my-first-app
# Expected: No resources found

# Resources should be gone
kubectl get deployment,service my-first-app
# Expected: Error from server (NotFound)
```

## Step 11: Clean Up

Remove Pequod from your cluster:

```bash
# Remove the controller and CRDs
kubectl delete -k github.com/chazu/pequod/config/default?ref=main
```

If using kind, delete the cluster:

```bash
kind delete cluster --name pequod-tutorial
```

## Next Steps

Congratulations! You've successfully:

1. Installed Pequod
2. Created a Transform
3. Observed the rendered ResourceGraph
4. Updated the application
5. Viewed status and debugged
6. Cleaned up

### Learn More

- [User Guide](user-guide.md) - Complete API reference and examples
- [Platform Engineer Guide](platform-engineer-guide.md) - Create custom platform modules
- [Operations Guide](operations.md) - Production deployment and monitoring

### Try More Examples

Create a Transform with inline CUE:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: inline-example
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
            applyPolicy: { mode: "Apply" }
            dependsOn: []
            readyWhen: [{ type: "Exists" }]
          }]
          violations: []
        }
      }
  input:
    message: "Hello from Pequod!"
```

Apply and verify:

```bash
kubectl apply -f inline-example.yaml
kubectl get configmap inline-example -o yaml
```

## Troubleshooting This Tutorial

### Transform stuck in Pending

```bash
# Check controller is running
kubectl get pods -n pequod-system

# If not running, check events
kubectl describe deployment -n pequod-system pequod-controller-manager
```

### Resources not created

```bash
# Check ResourceGraph status
kubectl get resourcegraph -l pequod.io/transform=my-first-app -o yaml

# Check for node errors in nodeStates
```

### Permission errors

```bash
# Check controller logs
kubectl logs -n pequod-system -l control-plane=controller-manager | grep -i "forbidden\|permission"

# Verify RBAC
kubectl auth can-i create deployments --as=system:serviceaccount:pequod-system:pequod-controller-manager
```
