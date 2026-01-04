# Pequod Tutorial: Getting Started

This tutorial walks you through deploying your first application with Pequod, from installation to cleanup. You'll see how platform engineers create platform types and how developers use them.

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

## Step 3: Create a Platform Type (Platform Engineer Role)

First, we'll act as a platform engineer and create a `Transform` that generates a `WebService` CRD.

Create a file called `webservice-transform.yaml`:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.tutorial.io
  shortNames: [ws]
```

Apply it:

```bash
kubectl apply -f webservice-transform.yaml
```

Expected output:
```
transform.platform.platform.example.com/webservice created
```

## Step 4: Watch the CRD Generation

Watch the Transform status:

```bash
kubectl get transform webservice -w
```

You'll see the status progress:
```
NAME         PHASE        CRD                                 AGE
webservice   Pending      <none>                              0s
webservice   Fetching     <none>                              1s
webservice   Generating   <none>                              2s
webservice   Ready        webservices.apps.tutorial.io        3s
```

Press `Ctrl+C` to stop watching.

Verify the CRD was created:

```bash
kubectl get crd webservices.apps.tutorial.io
```

## Step 5: Create Your First Application (Developer Role)

Now we'll act as a developer and create an instance of the `WebService` platform type.

Create a file called `my-first-app.yaml`:

```yaml
apiVersion: apps.tutorial.io/v1alpha1
kind: WebService
metadata:
  name: my-first-app
  namespace: default
spec:
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
webservice.apps.tutorial.io/my-first-app created
```

## Step 6: Watch the Reconciliation

Watch the WebService status:

```bash
kubectl get webservice my-first-app -w
```

You'll see the status progress as resources are created.

Press `Ctrl+C` to stop watching.

## Step 7: Explore What Was Created

### View the ResourceGraph

Pequod creates a ResourceGraph that contains the rendered resources:

```bash
kubectl get resourcegraph -l pequod.io/instance=my-first-app
```

Expected output:
```
NAME                     PHASE       NODES   AGE
my-first-app-abc123xyz   Completed   2       30s
```

View the full ResourceGraph:

```bash
kubectl get resourcegraph -l pequod.io/instance=my-first-app -o yaml
```

### View the Created Resources

Pequod created a Deployment and a Service:

```bash
kubectl get deployment,service -l app.kubernetes.io/instance=my-first-app
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
kubectl get pods -l app.kubernetes.io/instance=my-first-app
```

Expected output:
```
NAME                            READY   STATUS    RESTARTS   AGE
my-first-app-xxxxxxxxx-xxxxx    1/1     Running   0          1m
my-first-app-xxxxxxxxx-yyyyy    1/1     Running   0          1m
```

## Step 8: Access Your Application

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

## Step 9: Update the Application

Let's scale up to 3 replicas. Edit `my-first-app.yaml`:

```yaml
apiVersion: apps.tutorial.io/v1alpha1
kind: WebService
metadata:
  name: my-first-app
  namespace: default
spec:
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
kubectl get pods -l app.kubernetes.io/instance=my-first-app -w
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

## Step 10: View Status and Debug

### WebService Instance Status

Get detailed instance status:

```bash
kubectl get webservice my-first-app -o yaml
```

Key status fields will show phase, conditions, and resource references.

### ResourceGraph Status

Check the node execution states:

```bash
kubectl get resourcegraph -l pequod.io/instance=my-first-app \
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

## Step 11: Pause Reconciliation (Optional)

You can pause reconciliation to make manual changes:

```bash
kubectl label webservice my-first-app pequod.io/paused=true
```

Verify it's paused:
```bash
kubectl get webservice my-first-app -o jsonpath='{.metadata.labels}'
```

Resume reconciliation:
```bash
kubectl label webservice my-first-app pequod.io/paused-
```

## Step 12: Delete the Application

Clean up by deleting the WebService instance:

```bash
kubectl delete webservice my-first-app
```

Verify everything is deleted:

```bash
# Instance should be gone
kubectl get webservice my-first-app
# Expected: Error from server (NotFound)

# ResourceGraph should be gone
kubectl get resourcegraph -l pequod.io/instance=my-first-app
# Expected: No resources found

# Resources should be gone
kubectl get deployment,service my-first-app
# Expected: Error from server (NotFound)
```

(Optional) Delete the Transform and generated CRD:

```bash
kubectl delete transform webservice
# This will also delete the WebService CRD
```

## Step 13: Clean Up

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
2. Created a Transform (platform engineer role) that generated a WebService CRD
3. Created a WebService instance (developer role)
4. Observed the rendered ResourceGraph
5. Updated the application
6. Viewed status and debugged
7. Cleaned up

### Learn More

- [User Guide](user-guide.md) - Complete API reference and examples for developers
- [Platform Engineer Guide](platform-engineer-guide.md) - Create custom platform modules
- [Operations Guide](operations.md) - Production deployment and monitoring

### Key Takeaways

- **Platform Engineers** create `Transform` resources that generate CRDs
- **Developers** create instances of generated CRDs (e.g., `WebService`)
- Pequod handles CUE evaluation, resource rendering, and DAG execution
- Each platform instance creates a `ResourceGraph` that tracks managed resources

## Troubleshooting This Tutorial

### Transform stuck in Pending

```bash
# Check controller is running
kubectl get pods -n pequod-system

# If not running, check events
kubectl describe deployment -n pequod-system pequod-controller-manager
```

### CRD not generated

```bash
# Check Transform status
kubectl get transform webservice -o yaml

# Look at conditions for errors
kubectl describe transform webservice
```

### Resources not created

```bash
# Check ResourceGraph status
kubectl get resourcegraph -l pequod.io/instance=my-first-app -o yaml

# Check for node errors in nodeStates
```

### Permission errors

```bash
# Check controller logs
kubectl logs -n pequod-system -l control-plane=controller-manager | grep -i "forbidden\|permission"

# Verify RBAC
kubectl auth can-i create deployments --as=system:serviceaccount:pequod-system:pequod-controller-manager
```
