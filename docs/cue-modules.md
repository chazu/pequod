# CUE Module Formats

This document describes the supported formats for CUE platform modules in Pequod.

## Overview

Pequod supports loading CUE platform definitions from multiple sources:

| Type | Description | Use Case |
|------|-------------|----------|
| `oci` | OCI registry artifact | Production deployments |
| `git` | Git repository | Development, CI/CD |
| `configmap` | Kubernetes ConfigMap | Testing, quick iterations |
| `inline` | Inline CUE in Transform spec | Simple cases, debugging |
| `embedded` | Bundled with operator | Built-in platform types |

---

## OCI Registry Format

Pequod supports the **official CUE module OCI format** as specified by [CUE v0.15+](https://cuelang.org/docs/reference/modules/). This ensures compatibility with the CUE Central Registry (`registry.cue.works`) and the `cue` CLI tooling.

### Reference Format

```yaml
spec:
  cueRef:
    type: oci
    ref: "ghcr.io/myorg/platforms/webservice:v1.0.0"
    pullSecretRef:
      name: ghcr-pull-secret  # Optional
```

### Reference Patterns

| Pattern | Example |
|---------|---------|
| Registry/repo:tag | `ghcr.io/myorg/mymodule:v1.0.0` |
| Registry/repo@digest | `ghcr.io/myorg/mymodule@sha256:abc123...` |
| Registry with port | `localhost:5000/mymodule:latest` |

### OCI Artifact Structure (CUE v0.15+ Compliant)

CUE modules are stored as OCI artifacts following the [official CUE specification](https://pkg.go.dev/cuelang.org/go/mod/modregistry):

```
OCI Manifest (application/vnd.oci.image.manifest.v1+json)
├── ArtifactType: application/vnd.cue.module.v1+json
├── Config: application/vnd.oci.image.config.v1+json
│   └── {} (empty scratch config)
├── Annotations:
│   ├── works.cue.module: "<module-path>@<version>"
│   ├── org.cuelang.vcs-type: "git" (optional)
│   ├── org.cuelang.vcs-commit: "<commit-sha>" (optional)
│   └── org.cuelang.vcs-commit-time: "<RFC3339>" (optional)
└── Layers:
    ├── Layer 0: application/zip
    │   └── ZIP archive containing:
    │       ├── cue.mod/
    │       │   └── module.cue
    │       ├── schema.cue
    │       ├── render.cue
    │       ├── policy.cue
    │       └── ... (other .cue files)
    └── Layer 1: application/vnd.cue.modulefile.v1
        └── Exact copy of cue.mod/module.cue (for fast dependency resolution)
```

### Media Types

| Component | Media Type |
|-----------|------------|
| Manifest | `application/vnd.oci.image.manifest.v1+json` |
| Artifact Type | `application/vnd.cue.module.v1+json` |
| Config | `application/vnd.oci.image.config.v1+json` |
| Module Archive | `application/zip` |
| Module File | `application/vnd.cue.modulefile.v1` |

### Size Limits

| Component | Maximum Size |
|-----------|--------------|
| ZIP archive (compressed) | 500 MiB |
| ZIP archive (uncompressed) | 500 MiB |
| Module file (cue.mod/module.cue) | 16 MiB |
| LICENSE file | 16 MiB |

### Module Path Requirements

As per the [CUE module specification](https://cuelang.org/docs/reference/modules/):

- Path must consist of one or more elements separated by `/`
- Must not begin or end with `/`
- Allowed characters: lowercase ASCII letters, digits, `-`, `_`, `.`
- All paths in the ZIP must be case-folding unique

### Building OCI Artifacts

**Using the `cue` CLI (Recommended):**

```bash
# Initialize a module (if not already done)
cue mod init example.com/myplatform@v0

# Publish to a registry
cue mod publish v1.0.0
```

**Using `oras` CLI (Manual):**

```bash
# Create the module.cue file
cat > cue.mod/module.cue << 'EOF'
module: "example.com/platforms/webservice@v1"
language: version: "v0.15.0"
EOF

# Create the ZIP archive
zip -r module.zip . -x "*.git*"

# Create the module file blob
cp cue.mod/module.cue module.cue.blob

# Push to registry with proper media types
oras push ghcr.io/myorg/platforms/webservice:v1.0.0 \
  --artifact-type "application/vnd.cue.module.v1+json" \
  --annotation "works.cue.module=example.com/platforms/webservice@v1.0.0" \
  module.zip:application/zip \
  module.cue.blob:application/vnd.cue.modulefile.v1
```

**Using Go (Programmatic):**

```go
import "cuelang.org/go/mod/modregistry"

client := modregistry.NewClient(ociRegistry)
err := client.PutModuleWithMetadata(ctx, zipReader, moduleVersion, modregistry.Metadata{
    VCSType:       "git",
    VCSCommit:     commitSHA,
    VCSCommitTime: commitTime,
})
```

### Authentication

Create a Kubernetes secret for private registries:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-pull-secret
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>
```

Or using `kubectl`:

```bash
kubectl create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=myuser \
  --docker-password=mytoken
```

### Compatibility Notes

Pequod supports both:
1. **Standard CUE modules** - Full compatibility with `cue` CLI and Central Registry
2. **Legacy tar.gz format** - For backwards compatibility, Pequod also accepts:
   - `application/vnd.cue.module.layer.v1+tar+gzip`
   - `application/vnd.oci.image.layer.v1.tar+gzip`

When using the standard CUE format, modules can be:
- Published using `cue mod publish`
- Fetched using `cue mod download`
- Mirrored between registries
- Verified using standard CUE tooling

---

## Git Repository Format

### Reference Format

```yaml
spec:
  cueRef:
    type: git
    ref: "https://github.com/myorg/platforms?ref=v1.0.0&path=webservice"
    pullSecretRef:
      name: git-auth-secret  # Optional
```

### Reference Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| Base URL | Git repository URL | `https://github.com/myorg/platforms` |
| `ref` | Branch, tag, or commit SHA | `ref=main`, `ref=v1.0.0`, `ref=abc123` |
| `path` | Path within repository | `path=modules/webservice` |

### Repository Structure

For standard CUE modules:

```
platforms/
├── webservice/
│   ├── cue.mod/
│   │   └── module.cue      # Module metadata
│   ├── schema.cue          # Input schema definition
│   ├── render.cue          # Resource templates
│   └── policy.cue          # Optional policies
├── database/
│   ├── cue.mod/
│   │   └── module.cue
│   ├── schema.cue
│   └── render.cue
└── README.md
```

### Authentication

**Using Personal Access Token (GitHub, GitLab):**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-auth-secret
type: Opaque
data:
  token: <base64-encoded-token>
```

**Using Basic Auth:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-auth-secret
type: kubernetes.io/basic-auth
data:
  username: <base64-encoded-username>
  password: <base64-encoded-password>
```

**Using SSH Key:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-ssh-secret
type: kubernetes.io/ssh-auth
data:
  ssh-privatekey: <base64-encoded-private-key>
  # Optional passphrase
  passphrase: <base64-encoded-passphrase>
```

---

## ConfigMap Format

### Reference Format

```yaml
spec:
  cueRef:
    type: configmap
    ref: "my-platform-module"  # ConfigMap name in same namespace
```

### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-platform-module
data:
  schema.cue: |
    package webservice

    #Input: {
      name:     string
      image:    string
      port:     int | *8080
      replicas: int | *1
    }

  render.cue: |
    package webservice

    import "encoding/yaml"

    graph: {
      metadata: {
        name: input.name
      }
      nodes: [
        {
          id: "deployment"
          object: yaml.Marshal(_deployment)
        }
      ]
    }

    _deployment: {
      apiVersion: "apps/v1"
      kind: "Deployment"
      // ...
    }
```

### Multiple Files

All keys ending in `.cue` are concatenated:

```yaml
data:
  schema.cue: |
    // Schema definitions
  render.cue: |
    // Rendering logic
  policy.cue: |
    // Policy rules
```

---

## Inline Format

### Reference Format

```yaml
spec:
  cueRef:
    type: inline
    ref: |
      package simple

      #Input: {
        name: string
      }

      graph: {
        metadata: name: input.name
        nodes: []
      }
```

### Use Cases

- Quick prototyping
- Debugging
- Simple single-resource transforms
- CI/CD testing

### Limitations

- No file organization (all in one block)
- Limited size (YAML document limits)
- Harder to version control

---

## CUE Module Structure

### Module Metadata (cue.mod/module.cue)

Every CUE module should have a `cue.mod/module.cue` file:

```cue
module: "example.com/platforms/webservice@v1"
language: version: "v0.15.0"

// Optional dependencies
deps: {
    "example.com/lib/k8s@v0": v: "v0.3.0"
}
```

### Pequod Platform Module Structure

Regardless of source, all Pequod platform modules must follow this structure:

```cue
package myplatform

// Input schema - defines what users can specify
#Input: {
  name:      string
  namespace: string | *"default"
  // ... other fields
}

// Input instance - bound to Transform.spec.input
input: #Input

// Graph output - defines resources to create
graph: {
  metadata: {
    name:    string
    version: string | *"v1"
  }
  nodes: [...#Node]
}

#Node: {
  id:        string
  object:    string  // YAML-encoded Kubernetes resource
  dependsOn: [...string] | *[]
  readyWhen: #ReadinessPredicate | *{type: "exists"}
  applyPolicy: #ApplyPolicy | *{mode: "ssa"}
}
```

### Example: WebService Platform

```cue
package webservice

#Input: {
  name:     string
  image:    string
  port:     int | *8080
  replicas: int | *1
}

input: #Input

import "encoding/yaml"

graph: {
  metadata: {
    name:    input.name
    version: "v1"
  }
  nodes: [
    {
      id: "deployment"
      object: yaml.Marshal({
        apiVersion: "apps/v1"
        kind:       "Deployment"
        metadata: {
          name:      input.name
          namespace: "default"
        }
        spec: {
          replicas: input.replicas
          selector: matchLabels: app: input.name
          template: {
            metadata: labels: app: input.name
            spec: containers: [{
              name:  input.name
              image: input.image
              ports: [{containerPort: input.port}]
            }]
          }
        }
      })
      readyWhen: {
        type: "deploymentAvailable"
      }
    },
    {
      id: "service"
      object: yaml.Marshal({
        apiVersion: "v1"
        kind:       "Service"
        metadata: {
          name:      input.name
          namespace: "default"
        }
        spec: {
          selector: app: input.name
          ports: [{
            port:       input.port
            targetPort: input.port
          }]
        }
      })
      dependsOn: ["deployment"]
    },
  ]
}
```

---

## Caching

All fetched modules are cached on disk with:

- **LRU eviction**: Oldest unused entries removed when cache is full
- **TTL expiration**: Entries expire after configurable duration
- **Digest-based keys**: Same content = same cache entry

Configure cache in operator deployment:

```yaml
env:
  - name: PEQUOD_CACHE_DIR
    value: /var/cache/pequod
  - name: PEQUOD_CACHE_MAX_ENTRIES
    value: "100"
  - name: PEQUOD_CACHE_TTL
    value: "24h"
```

---

## Security Considerations

1. **OCI registries**: Use digest references (`@sha256:...`) for immutable deployments
2. **Git repositories**: Pin to specific commits or tags, not branches
3. **ConfigMaps**: Use RBAC to control who can modify platform definitions
4. **Inline CUE**: Validate Transform specs before applying
5. **Pull secrets**: Use Kubernetes secrets, never inline credentials

---

## References

- [CUE Modules Reference](https://cuelang.org/docs/reference/modules/)
- [CUE Registry Configuration](https://cuelang.org/docs/reference/command/cue-help-registryconfig/)
- [modregistry Package](https://pkg.go.dev/cuelang.org/go/mod/modregistry)
- [Working with Custom Registries](https://cuelang.org/docs/tutorial/working-with-a-custom-module-registry/)
