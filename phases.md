# Plan: Dynamic CRD Generation Architecture

## Overview

Transform Pequod from a single-CRD architecture to a dynamic CRD generation architecture where:
1. **Transform** becomes a "Platform Definition" created by platform engineers
2. **Transform controller** generates a new CRD from the CUE schema (e.g., WebService CRD)
3. **Users** create instances of the generated CRD (e.g., a WebService CR)
4. **Instance controller** watches those instances and creates ResourceGraphs
5. **ResourceGraph controller** executes the DAG (unchanged)

## Current vs Target Architecture

### Current (Single CRD)
```
User creates Transform (with cueRef + input)
    -> Transform Controller renders CUE
        -> Creates ResourceGraph
            -> ResourceGraph Controller applies resources
```

### Target (Dynamic CRD)
```
Platform Engineer creates Transform (platform definition with CUE module)
    -> Transform Controller extracts schema from CUE
        -> Generates and applies new CRD (e.g., WebService)
            -> User creates WebService instance
                -> Instance Controller renders CUE with instance spec
                    -> Creates ResourceGraph
                        -> ResourceGraph Controller applies resources (unchanged)
```

## Implementation Phases

### Phase 1: Schema Extraction from CUE

**Goal**: Extract JSONSchema from CUE `#Input` or `#Spec` definitions

**Files to create**:
- `pkg/schema/extractor.go` - CUE to JSONSchema conversion
- `pkg/schema/extractor_test.go` - Tests

**Key functions**:
```go
// ExtractInputSchema extracts the input schema from a CUE module
func ExtractInputSchema(cueValue cue.Value) (*apiextensionsv1.JSONSchemaProps, error)

// cueToJSONSchema converts a CUE value to JSONSchema
func cueToJSONSchema(v cue.Value) (*apiextensionsv1.JSONSchemaProps, error)
```

**Handles**:
- Scalar types (string, int, bool, float)
- Required vs optional fields (CUE `?` operator)
- Numeric constraints (>=, <=)
- String constraints (non-empty)
- Enumerations (disjunctions)
- Nested objects
- Arrays
- Default values

### Phase 2: CRD Generator

**Goal**: Generate Kubernetes CRDs from extracted schemas

**Files to create**:
- `pkg/crd/generator.go` - CRD generation logic
- `pkg/crd/generator_test.go` - Tests

**Key functions**:
```go
// GenerateCRD creates a CRD from a platform definition
func GenerateCRD(platformName string, group string, schema *apiextensionsv1.JSONSchemaProps) *apiextensionsv1.CustomResourceDefinition

// ApplyCRD applies the CRD to the cluster
func ApplyCRD(ctx context.Context, client client.Client, crd *apiextensionsv1.CustomResourceDefinition) error
```

**CRD Structure**:
- Group: `platform.pequod.io` (or configurable)
- Version: `v1alpha1`
- Kind: Derived from Transform name (e.g., `webservice` -> `WebService`)
- Scope: Namespaced
- Schema: From CUE extraction

### Phase 3: Transform Controller Refactoring

**Goal**: Transform controller generates CRDs instead of ResourceGraphs

**Files to modify**:
- `api/v1alpha1/transform_types.go` - Update Transform spec/status
- `pkg/reconcile/transform_handlers.go` - New reconciliation logic
- `internal/controller/transform_controller.go` - Setup changes

**New Transform Spec**:
```go
type TransformSpec struct {
    // CueRef specifies how to load the CUE platform module
    CueRef CueReference `json:"cueRef"`

    // Group is the API group for generated CRD (default: platform.pequod.io)
    Group string `json:"group,omitempty"`

    // Version is the API version (default: v1alpha1)
    Version string `json:"version,omitempty"`

    // ShortNames for the generated CRD
    ShortNames []string `json:"shortNames,omitempty"`
}
```

**New Transform Status**:
```go
type TransformStatus struct {
    // GeneratedCRD contains info about the generated CRD
    GeneratedCRD *GeneratedCRDReference `json:"generatedCRD,omitempty"`

    // Phase: Pending, Generating, Ready, Failed
    Phase string `json:"phase,omitempty"`

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type GeneratedCRDReference struct {
    APIVersion string `json:"apiVersion"`
    Kind       string `json:"kind"`
    Name       string `json:"name"` // CRD name
}
```

**New Reconciliation Flow**:
1. Fetch Transform
2. Load CUE module
3. Extract input schema from CUE
4. Generate CRD specification
5. Apply CRD to cluster (create or update)
6. Update Transform status with CRD reference

### Phase 4: Platform Instance Controller

**Goal**: New controller watches generated CRDs and creates ResourceGraphs

**Files to create**:
- `internal/controller/platforminstance_controller.go` - Dynamic instance controller
- `pkg/reconcile/instance_handlers.go` - Instance reconciliation logic

**Key challenges**:
1. **Dynamic watching**: Controller must watch CRDs that don't exist at startup
2. **GVK discovery**: Must discover which GVKs to watch from Transform resources
3. **Rendering**: Must render CUE with instance spec to produce ResourceGraph

**Implementation approach**:
```go
type PlatformInstanceReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    DynamicClient  dynamic.Interface
    Renderer       *platformloader.Renderer

    // Track watched GVKs
    watchedGVKs    map[schema.GroupVersionKind]bool
    watchMutex     sync.RWMutex
}

// SetupWithManager sets up initial watches
func (r *PlatformInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Watch Transform to discover new platform types
    // Dynamically add watches for generated CRDs
}

// Reconcile handles platform instance creation
func (r *PlatformInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Get the unstructured instance
    // 2. Find the corresponding Transform (platform definition)
    // 3. Load CUE module from Transform
    // 4. Render graph with instance spec
    // 5. Create/update ResourceGraph
}
```

### Phase 5: Dynamic Watch Management

**Goal**: Dynamically add/remove watches as Transforms are created/deleted

**Approach**: Use controller-runtime's dynamic source or informer factory

**Option A: Dynamic Informer (Recommended)**
```go
// Watch for new platform types by monitoring Transforms
func (r *PlatformInstanceReconciler) watchPlatformType(gvk schema.GroupVersionKind) error {
    // Create informer for the new GVK
    // Add to controller's watch list
}
```

**Option B: Polling-based discovery**
- Periodically list Transforms
- Check for new generated CRDs
- Add watches as needed

### Phase 6: ResourceGraph Ownership Changes

**Goal**: ResourceGraphs are now owned by platform instances, not Transforms

**Files to modify**:
- `api/v1alpha1/resourcegraph_types.go` - Update SourceRef
- `pkg/reconcile/instance_handlers.go` - Set correct owner references

**Changes**:
- SourceRef points to platform instance (e.g., WebService/my-app)
- Owner reference on ResourceGraph points to instance
- Instance deletion cascades to ResourceGraph deletion

### Phase 7: RBAC Updates

**Goal**: Add permissions for CRD management

**Files to modify**:
- `config/rbac/role.yaml`

**New permissions**:
```yaml
- apiGroups:
  - apiextensions.k8s.io
  resources:
  - customresourcedefinitions
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
```

### Phase 8: Migration and Cleanup

**Goal**: Clean up old code paths

**Tasks**:
1. Remove `Input` field from Transform spec
2. Update all tests
3. Update documentation
4. Update sample manifests
5. Update e2e tests

## CUE Module Requirements

CUE modules must define:
```cue
// Input schema - REQUIRED for CRD generation
#Input: {
    // Fields that become the CRD spec
    image: string
    port: int
    replicas?: int
}

// Render template - REQUIRED for ResourceGraph generation
#Render: {
    input: {
        metadata: { name: string, namespace: string }
        spec: #Input
    }
    output: #Graph
}
```

## File Changes Summary

### New Files
- `pkg/schema/extractor.go` - CUE -> JSONSchema
- `pkg/schema/extractor_test.go`
- `pkg/crd/generator.go` - CRD generation
- `pkg/crd/generator_test.go`
- `internal/controller/platforminstance_controller.go`
- `pkg/reconcile/instance_handlers.go`

### Modified Files
- `api/v1alpha1/transform_types.go` - New spec/status fields
- `api/v1alpha1/resourcegraph_types.go` - SourceRef changes
- `pkg/reconcile/transform_handlers.go` - CRD generation logic
- `internal/controller/transform_controller.go` - Setup changes
- `config/rbac/role.yaml` - CRD permissions
- `cmd/main.go` - Register new controller

### Deleted/Deprecated
- Transform `Input` field (replaced by instance spec)
- Direct Transform -> ResourceGraph flow

## Example User Flow

### Platform Engineer
```yaml
apiVersion: platform.pequod.io/v1alpha1
kind: Transform
metadata:
  name: webservice
spec:
  cueRef:
    type: embedded
    ref: webservice
  group: apps.mycompany.com  # Optional, defaults to platform.pequod.io
  shortNames: [ws]
```

### Generated CRD
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: webservices.apps.mycompany.com
spec:
  group: apps.mycompany.com
  names:
    kind: WebService
    plural: webservices
    shortNames: [ws]
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              image:
                type: string
              port:
                type: integer
              replicas:
                type: integer
            required: [image, port]
```

### Developer
```yaml
apiVersion: apps.mycompany.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  image: nginx:latest
  port: 80
  replicas: 3
```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Dynamic watching complexity | Use controller-runtime's built-in dynamic source support |
| CRD update conflicts | Use SSA with proper field managers |
| Schema extraction limitations | Start with basic types, iterate on complex constraints |
| Breaking change for existing users | Provide migration guide, deprecation period |

## Testing Strategy

1. **Unit tests**: Schema extraction, CRD generation
2. **Integration tests**: Transform -> CRD flow with envtest
3. **E2E tests**: Full flow from Transform to deployed resources
