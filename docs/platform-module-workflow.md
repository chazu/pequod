# Platform Module Workflow: Visual Guide

This document provides visual representations of how platform modules flow through the system.

## Embedded Module Workflow (Phase 1)

```
┌─────────────────────────────────────────────────────────────────┐
│ Platform Team                                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ 1. Create CUE module
                              ▼
                    ┌──────────────────┐
                    │  cue/platform/   │
                    │    v1.0.0/       │
                    │  - schema.cue    │
                    │  - render.cue    │
                    │  - policy/*.cue  │
                    └──────────────────┘
                              │
                              │ 2. Commit to operator repo
                              ▼
                    ┌──────────────────┐
                    │   Git Commit     │
                    │   //go:embed     │
                    └──────────────────┘
                              │
                              │ 3. Build operator image
                              ▼
                    ┌──────────────────┐
                    │ Operator Image   │
                    │ pequod:v0.1.0    │
                    │ [CUE embedded]   │
                    └──────────────────┘
                              │
                              │ 4. Deploy to cluster
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                              │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Pequod Operator Pod                                      │  │
│  │                                                          │  │
│  │  ┌────────────────────────────────────────────────┐     │  │
│  │  │ Embedded Modules                               │     │  │
│  │  │  - v1.0.0 (CUE files in memory)               │     │  │
│  │  │  - v1.1.0 (CUE files in memory)               │     │  │
│  │  └────────────────────────────────────────────────┘     │  │
│  │                      ▲                                   │  │
│  │                      │ 5. Load embedded module           │  │
│  │                      │                                   │  │
│  │  ┌────────────────────────────────────────────────┐     │  │
│  │  │ Controller                                     │     │  │
│  │  │  - Watches WebService CRs                     │     │  │
│  │  │  - Loads module by version                    │     │  │
│  │  │  - Evaluates CUE → Graph                      │     │  │
│  │  └────────────────────────────────────────────────┘     │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ WebService CR                                            │  │
│  │                                                          │  │
│  │  spec:                                                   │  │
│  │    platformRef:                                          │  │
│  │      embedded: v1.0.0  ◄─── 6. Developer specifies      │  │
│  │    image: myapp:latest                                   │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**Key Points:**
- Module is part of operator binary
- No network calls at runtime
- Update requires operator rebuild
- Fast and reliable

---

## OCI Registry Workflow (Phase 9)

```
┌─────────────────────────────────────────────────────────────────┐
│ Platform Team                                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ 1. Create CUE module
                              ▼
                    ┌──────────────────┐
                    │  Module Files    │
                    │  - schema.cue    │
                    │  - render.cue    │
                    │  - policy/*.cue  │
                    └──────────────────┘
                              │
                              │ 2. Package as OCI artifact
                              ▼
                    ┌──────────────────┐
                    │  docker build    │
                    │  docker push     │
                    └──────────────────┘
                              │
                              │ 3. Push to registry
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ OCI Registry (e.g., Docker Hub, ECR, GCR)                       │
│                                                                 │
│  myregistry.com/platform-modules/webservice                    │
│    ├── v1.0.0 → sha256:abc123...                              │
│    ├── v1.1.0 → sha256:def456...                              │
│    └── v1.2.0 → sha256:xyz789...  ◄─── 4. Published           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ 5. Operator fetches on-demand
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                              │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Pequod Operator Pod                                      │  │
│  │                                                          │  │
│  │  ┌────────────────────────────────────────────────┐     │  │
│  │  │ Module Cache (Disk)                            │     │  │
│  │  │  /cache/sha256:abc123... (v1.0.0)             │     │  │
│  │  │  /cache/sha256:def456... (v1.1.0)             │     │  │
│  │  │  /cache/sha256:xyz789... (v1.2.0) ◄─ 7. Cache │     │  │
│  │  └────────────────────────────────────────────────┘     │  │
│  │                      ▲                                   │  │
│  │                      │ 6. Fetch if not cached            │  │
│  │                      │                                   │  │
│  │  ┌────────────────────────────────────────────────┐     │  │
│  │  │ Platform Loader                                │     │  │
│  │  │  - Check cache                                 │     │  │
│  │  │  - Fetch from OCI if needed                   │     │  │
│  │  │  - Verify digest                              │     │  │
│  │  │  - Store in cache                             │     │  │
│  │  └────────────────────────────────────────────────┘     │  │
│  │                      │                                   │  │
│  │                      │ 8. Load module                    │  │
│  │                      ▼                                   │  │
│  │  ┌────────────────────────────────────────────────┐     │  │
│  │  │ Controller                                     │     │  │
│  │  │  - Evaluates CUE → Graph                      │     │  │
│  │  └────────────────────────────────────────────────┘     │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ WebService CR                                            │  │
│  │                                                          │  │
│  │  spec:                                                   │  │
│  │    platformRef:                                          │  │
│  │      oci: "myregistry.com/platform-modules/             │  │
│  │            webservice@sha256:xyz789..."                  │  │
│  │    image: myapp:latest                                   │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**Key Points:**
- Module stored in OCI registry
- Fetched on first use, then cached
- Digest ensures immutability
- Independent from operator version

---

## Module Resolution Flow

```
Developer creates WebService
         │
         ▼
┌─────────────────────┐
│ Parse platformRef   │
└─────────────────────┘
         │
         ├─────────────────┬─────────────────┬─────────────────┐
         ▼                 ▼                 ▼                 ▼
    embedded:v1.0.0    oci:registry/...  git:github.com/...  (future)
         │                 │                 │
         ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ Load from       │ │ Check cache     │ │ Clone repo      │
│ embedded map    │ │ If miss: fetch  │ │ Checkout ref    │
└─────────────────┘ └─────────────────┘ └─────────────────┘
         │                 │                 │
         └─────────────────┴─────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Resolve digest  │
                  │ (for audit)     │
                  └─────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Load CUE files  │
                  └─────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Compile CUE     │
                  │ context         │
                  └─────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Evaluate with   │
                  │ WebService spec │
                  └─────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Produce Graph   │
                  │ artifact        │
                  └─────────────────┘
```

---

## Status Tracking

The operator tracks which module version was used:

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    oci: "myregistry.com/platform-modules/webservice@sha256:xyz789..."
  image: myapp:latest
status:
  conditions:
  - type: Rendered
    status: "True"
    reason: GraphRendered
    message: "Graph rendered successfully"
  
  # Operator records resolved module
  platformRefResolved: "sha256:xyz789..."  # Immutable digest
  renderHash: "abc123..."                   # Hash of (module + input)
  
  # Optional: Reference to stored Graph artifact
  graphArtifact:
    configMapRef: my-app-graph-abc123
```

This enables:
- **Auditability**: Know exactly which module version was used
- **Reproducibility**: Same input + same module = same output
- **Debugging**: Inspect the exact Graph that was applied

---

## Migration Path

### Step 1: Start with Embedded (v0.1.0)

```yaml
# All WebServices use embedded modules
spec:
  platformRef:
    embedded: v1.0.0
```

### Step 2: Introduce OCI (v0.2.0)

```yaml
# New WebServices can use OCI
spec:
  platformRef:
    oci: "myregistry.com/platform-modules/webservice@sha256:..."
    
# Old WebServices still work with embedded
spec:
  platformRef:
    embedded: v1.0.0
```

### Step 3: Migrate Gradually

```bash
# Platform team publishes to OCI
docker push myregistry.com/platform-modules/webservice:v1.1.0

# Developers update at their own pace
kubectl patch webservice my-app --type=merge -p '
spec:
  platformRef:
    oci: "myregistry.com/platform-modules/webservice@sha256:new..."
'
```

### Step 4: Deprecate Embedded (v1.0.0)

```yaml
# Eventually, remove embedded modules from operator
# All WebServices must use OCI or Git
spec:
  platformRef:
    oci: "myregistry.com/platform-modules/webservice@sha256:..."
```

---

## Best Practices

### For Platform Teams

1. **Version Semantically**: Use semver (v1.2.3)
2. **Test Thoroughly**: Validate CUE before publishing
3. **Document Changes**: Maintain CHANGELOG for modules
4. **Use Digests**: Always publish with immutable digests
5. **Communicate**: Announce new versions to developers

### For Developers

1. **Pin Digests**: Use `@sha256:...` not `:tag`
2. **Test Updates**: Validate new modules in dev first
3. **Monitor Status**: Check `status.platformRefResolved`
4. **Report Issues**: Feedback to platform team

### For Operators

1. **Cache Aggressively**: Reduce registry calls
2. **Monitor Metrics**: Track cache hit rate
3. **Set Timeouts**: Fail fast on network issues
4. **Verify Signatures**: Use cosign/sigstore (future)
5. **Audit Logs**: Track all module loads

