# Platform Module Delivery: How Platform Teams Deploy CUE Modules

This document explains how platform engineering teams create, version, and deliver CUE modules to the Pequod operator.

## Overview

Platform teams have **three options** for delivering CUE modules:

1. **Embedded Modules** (Phase 1): Bundled with operator image
2. **OCI Registry** (Phase 9): Published as OCI artifacts
3. **Git Repository** (Phase 9): Stored in Git repos

## Option 1: Embedded Modules (Recommended for v0.1.0)

### How It Works

Platform modules are embedded directly into the operator binary using Go's `//go:embed` directive.

### Directory Structure

```
pequod/
├── cue/
│   └── platform/
│       ├── v1.0.0/              # Semantic version
│       │   ├── webservice/
│       │   │   ├── schema.cue   # Input schema
│       │   │   └── render.cue   # Resource templates
│       │   ├── policy/
│       │   │   ├── input.cue    # Input validation
│       │   │   └── output.cue   # Output validation
│       │   └── lib/
│       │       └── k8s/
│       │           └── helpers.cue
│       └── v1.1.0/              # New version
│           └── ...
```

### Platform Team Workflow

#### Step 1: Create CUE Module

```bash
# Platform team creates new module version
cd cue/platform/v1.0.0/

# Create schema
cat > webservice/schema.cue << 'EOF'
package webservice

#WebServiceSpec: {
    image: string & =~"^[a-z0-9-./]+:[a-z0-9.-]+$"
    replicas?: int & >=1 & <=100 | *3
    port: int & >0 & <65536
    resources?: {
        cpu?: string
        memory?: string
    }
}
EOF

# Create rendering logic
cat > webservice/render.cue << 'EOF'
package webservice

import "encoding/yaml"

// Input from WebService CR
input: #WebServiceSpec

// Rendered graph
graph: {
    metadata: {
        version: "v1.0.0"
    }
    
    nodes: [
        {
            id: "deployment"
            object: {
                apiVersion: "apps/v1"
                kind: "Deployment"
                metadata: {
                    name: input.name
                    namespace: input.namespace
                }
                spec: {
                    replicas: input.replicas
                    selector: matchLabels: app: input.name
                    template: {
                        metadata: labels: app: input.name
                        spec: containers: [{
                            name: input.name
                            image: input.image
                            ports: [{containerPort: input.port}]
                        }]
                    }
                }
            }
            readyWhen: [{
                type: "DeploymentAvailable"
            }]
            dependsOn: []
        },
        {
            id: "service"
            object: {
                apiVersion: "v1"
                kind: "Service"
                metadata: {
                    name: input.name
                    namespace: input.namespace
                }
                spec: {
                    selector: app: input.name
                    ports: [{
                        port: input.port
                        targetPort: input.port
                    }]
                }
            }
            readyWhen: [{type: "Exists"}]
            dependsOn: ["deployment"]
        }
    ]
}
EOF

# Create policies
cat > policy/input.cue << 'EOF'
package policy

import "strings"

// Input validation
violations: [
    if !strings.HasPrefix(input.image, "myregistry.com/") {
        {
            path: "spec.image"
            message: "Image must be from myregistry.com"
            severity: "error"
        }
    },
    if input.replicas != _|_ && input.replicas > 10 {
        {
            path: "spec.replicas"
            message: "Replicas cannot exceed 10 for cost control"
            severity: "warning"
        }
    }
]
EOF
```

#### Step 2: Test CUE Module Locally

```bash
# Test CUE evaluation
cue eval ./cue/platform/v1.0.0/...

# Test with sample input
cat > test-input.cue << 'EOF'
package webservice

input: {
    name: "my-app"
    namespace: "default"
    image: "myregistry.com/app:v1.0.0"
    port: 8080
}
EOF

cue eval -c ./cue/platform/v1.0.0/... test-input.cue
```

#### Step 3: Embed in Operator

The operator code embeds all platform modules:

```go
// pkg/platformloader/embedded.go
package platformloader

import (
    _ "embed"
    "fmt"
)

//go:embed ../../cue/platform/v1.0.0
var platformV1_0_0 string

//go:embed ../../cue/platform/v1.1.0
var platformV1_1_0 string

var embeddedModules = map[string]string{
    "v1.0.0": platformV1_0_0,
    "v1.1.0": platformV1_1_0,
}

func LoadEmbedded(version string) (string, error) {
    content, ok := embeddedModules[version]
    if !ok {
        return "", fmt.Errorf("embedded module version %s not found", version)
    }
    return content, nil
}
```

#### Step 4: Build and Deploy Operator

```bash
# Build operator with embedded modules
make docker-build IMG=myregistry.com/pequod:v0.1.0

# Push to registry
make docker-push IMG=myregistry.com/pequod:v0.1.0

# Deploy to cluster
kubectl apply -f config/deploy/operator.yaml
```

### Developer Usage

Developers reference embedded versions in their WebService:

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    embedded: v1.0.0  # References embedded module
  image: myregistry.com/my-app:latest
  port: 8080
  replicas: 3
```

### Pros and Cons

**Pros:**
- ✅ Simple: No external dependencies
- ✅ Fast: No network calls
- ✅ Reliable: Always available
- ✅ Versioned: Tied to operator version
- ✅ Auditable: Module version in operator image

**Cons:**
- ❌ Requires operator rebuild for module updates
- ❌ All versions bundled (image size)
- ❌ No independent module versioning

**Best For:** Initial releases, stable platforms, air-gapped environments

---

## Option 2: OCI Registry (Recommended for Production)

### How It Works

Platform modules are packaged as OCI artifacts and published to a container registry. The operator fetches them at runtime.

### Platform Team Workflow

#### Step 1: Create CUE Module (Same as Embedded)

```bash
# Create module in separate repo
mkdir -p platform-modules/webservice/v1.2.0
cd platform-modules/webservice/v1.2.0

# Create CUE files (schema.cue, render.cue, policy/*.cue)
# ... same structure as embedded ...
```

#### Step 2: Package as OCI Artifact

```bash
# Create OCI artifact using ORAS (OCI Registry As Storage)
# Install ORAS: https://oras.land/

# Package the module
oras push myregistry.com/platform-modules/webservice:v1.2.0 \
    --artifact-type application/vnd.cue.module.v1 \
    schema.cue:application/vnd.cue.source \
    render.cue:application/vnd.cue.source \
    policy/:application/vnd.cue.source

# Or use a Dockerfile approach
cat > Dockerfile << 'EOF'
FROM scratch
COPY . /module
EOF

docker build -t myregistry.com/platform-modules/webservice:v1.2.0 .
docker push myregistry.com/platform-modules/webservice:v1.2.0

# Get the digest
DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' \
    myregistry.com/platform-modules/webservice:v1.2.0)
echo "Module digest: $DIGEST"
# Output: myregistry.com/platform-modules/webservice@sha256:abc123...
```

#### Step 3: Publish and Communicate

```bash
# Tag for semantic versioning
docker tag myregistry.com/platform-modules/webservice:v1.2.0 \
    myregistry.com/platform-modules/webservice:v1.2
docker tag myregistry.com/platform-modules/webservice:v1.2.0 \
    myregistry.com/platform-modules/webservice:v1
docker tag myregistry.com/platform-modules/webservice:v1.2.0 \
    myregistry.com/platform-modules/webservice:latest

docker push myregistry.com/platform-modules/webservice:v1.2
docker push myregistry.com/platform-modules/webservice:v1
docker push myregistry.com/platform-modules/webservice:latest

# Communicate digest to developers
echo "New platform module available:"
echo "  Version: v1.2.0"
echo "  Digest: sha256:abc123..."
echo "  Features: Added HPA support, improved policies"
```

### Developer Usage

Developers reference OCI modules by digest (recommended) or tag:

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    # Option 1: By digest (immutable, recommended)
    oci: "myregistry.com/platform-modules/webservice@sha256:abc123..."

    # Option 2: By tag (mutable, not recommended for production)
    # oci: "myregistry.com/platform-modules/webservice:v1.2.0"

  image: myregistry.com/my-app:latest
  port: 8080
```

### Operator Implementation

```go
// pkg/platformloader/oci.go
package platformloader

import (
    "context"
    "fmt"

    "github.com/google/go-containerregistry/pkg/crane"
    "github.com/google/go-containerregistry/pkg/v1/remote"
)

func FetchOCI(ctx context.Context, ref string) ([]byte, string, error) {
    // Parse reference
    // ref format: "registry.com/repo/module@sha256:..." or "registry.com/repo/module:tag"

    // Pull the image
    img, err := crane.Pull(ref)
    if err != nil {
        return nil, "", fmt.Errorf("failed to pull OCI artifact: %w", err)
    }

    // Get digest
    digest, err := img.Digest()
    if err != nil {
        return nil, "", fmt.Errorf("failed to get digest: %w", err)
    }

    // Extract layers (CUE files)
    layers, err := img.Layers()
    if err != nil {
        return nil, "", fmt.Errorf("failed to get layers: %w", err)
    }

    // Read CUE content from layers
    var content []byte
    for _, layer := range layers {
        rc, err := layer.Uncompressed()
        if err != nil {
            return nil, "", err
        }
        defer rc.Close()

        // Read and concatenate CUE files
        // ... implementation details ...
    }

    return content, digest.String(), nil
}
```

### Caching Strategy

```go
// pkg/platformloader/cache.go
package platformloader

import (
    "crypto/sha256"
    "fmt"
    "os"
    "path/filepath"
    "sync"
)

type ModuleCache struct {
    cacheDir string
    mu       sync.RWMutex
}

func NewModuleCache(cacheDir string) *ModuleCache {
    return &ModuleCache{cacheDir: cacheDir}
}

func (c *ModuleCache) Get(digest string) ([]byte, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    path := filepath.Join(c.cacheDir, digest)
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, false
    }
    return content, true
}

func (c *ModuleCache) Put(digest string, content []byte) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    path := filepath.Join(c.cacheDir, digest)
    return os.WriteFile(path, content, 0644)
}
```

### Pros and Cons

**Pros:**
- ✅ Independent versioning: Update modules without operator rebuild
- ✅ Centralized: Single source of truth
- ✅ Immutable: Digest-based references
- ✅ Reusable: Standard OCI tooling
- ✅ Auditable: Registry tracks all versions

**Cons:**
- ❌ Network dependency: Requires registry access
- ❌ Complexity: More moving parts
- ❌ Security: Need to verify signatures
- ❌ Latency: First fetch is slower

**Best For:** Production environments, large organizations, frequent module updates

---

## Option 3: Git Repository

### How It Works

Platform modules are stored in Git repositories. The operator clones/fetches them at runtime.

### Platform Team Workflow

#### Step 1: Create Git Repository

```bash
# Create dedicated repo for platform modules
git init platform-modules
cd platform-modules

# Create module structure
mkdir -p webservice/v1.3.0
cd webservice/v1.3.0

# Add CUE files
# ... same structure as before ...

git add .
git commit -m "Add webservice module v1.3.0"
git tag v1.3.0
git push origin main --tags
```

#### Step 2: Publish Module

```bash
# Tag the commit
git tag -a webservice/v1.3.0 -m "WebService module v1.3.0"
git push origin webservice/v1.3.0

# Get commit SHA
COMMIT_SHA=$(git rev-parse HEAD)
echo "Module commit: $COMMIT_SHA"
```

### Developer Usage

```yaml
apiVersion: platform.example.com/v1alpha1
kind: WebService
metadata:
  name: my-app
spec:
  platformRef:
    # Option 1: By commit SHA (immutable, recommended)
    git: "https://github.com/myorg/platform-modules.git?ref=abc123def456&path=webservice/v1.3.0"

    # Option 2: By tag (semi-mutable)
    # git: "https://github.com/myorg/platform-modules.git?ref=webservice/v1.3.0&path=webservice/v1.3.0"

    # Option 3: By branch (mutable, not recommended)
    # git: "https://github.com/myorg/platform-modules.git?ref=main&path=webservice/v1.3.0"

  image: myregistry.com/my-app:latest
  port: 8080
```

### Operator Implementation

```go
// pkg/platformloader/git.go
package platformloader

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing"
)

func FetchGit(ctx context.Context, url, ref, path string) ([]byte, string, error) {
    // Create temp directory
    tmpDir, err := os.MkdirTemp("", "platform-module-*")
    if err != nil {
        return nil, "", err
    }
    defer os.RemoveAll(tmpDir)

    // Clone repository
    repo, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
        URL:           url,
        ReferenceName: plumbing.ReferenceName(ref),
        SingleBranch:  true,
        Depth:         1,
    })
    if err != nil {
        return nil, "", fmt.Errorf("failed to clone: %w", err)
    }

    // Get commit SHA
    head, err := repo.Head()
    if err != nil {
        return nil, "", err
    }
    commitSHA := head.Hash().String()

    // Read CUE files from path
    modulePath := filepath.Join(tmpDir, path)
    content, err := readCUEFiles(modulePath)
    if err != nil {
        return nil, "", err
    }

    return content, commitSHA, nil
}
```

### Pros and Cons

**Pros:**
- ✅ Version control: Full Git history
- ✅ Familiar: Developers know Git
- ✅ Code review: PR workflow for changes
- ✅ Free: GitHub/GitLab hosting

**Cons:**
- ❌ Slower: Clone operation overhead
- ❌ Network dependency: Requires Git access
- ❌ Authentication: Need SSH keys or tokens
- ❌ Less efficient: Not designed for artifacts

**Best For:** Small teams, GitOps workflows, development environments

---

## Comparison Matrix

| Feature | Embedded | OCI Registry | Git Repository |
|---------|----------|--------------|----------------|
| **Speed** | ⚡ Instant | 🚀 Fast (cached) | 🐌 Slower |
| **Network Required** | ❌ No | ✅ Yes | ✅ Yes |
| **Independent Updates** | ❌ No | ✅ Yes | ✅ Yes |
| **Immutability** | ✅ Yes | ✅ Yes (digest) | ✅ Yes (SHA) |
| **Tooling** | Go embed | OCI/Docker | Git |
| **Storage** | Operator image | Registry | Git repo |
| **Best For** | v0.1.0, air-gap | Production | Small teams |

---

## Recommended Approach

### Phase 1 (v0.1.0): Embedded Only
- Start simple with embedded modules
- Validate the CUE module structure
- Get feedback from early users
- No external dependencies

### Phase 2 (v0.2.0): Add OCI Support
- Implement OCI fetching and caching
- Migrate to OCI for production
- Keep embedded as fallback
- Enable independent module updates

### Phase 3 (v0.3.0): Add Git Support (Optional)
- Add Git fetching for GitOps workflows
- Support private repositories
- Enable PR-based module reviews

---

## Security Considerations

### Embedded Modules
- ✅ No runtime security concerns
- ✅ Scanned with operator image
- ✅ No supply chain attacks

### OCI Modules
- ⚠️ Verify signatures (Sigstore/cosign)
- ⚠️ Use digest-based references
- ⚠️ Scan artifacts for vulnerabilities
- ⚠️ Implement allowlist for registries

### Git Modules
- ⚠️ Verify commit signatures (GPG)
- ⚠️ Use commit SHAs, not branches
- ⚠️ Secure authentication (SSH keys, tokens)
- ⚠️ Implement allowlist for repositories

---

## Example: Complete Platform Team Workflow

### Scenario: Platform team wants to add HPA support to WebService

#### 1. Develop Module Locally

```bash
cd cue/platform/v1.4.0
# Add HPA to render.cue
# Test with cue eval
```

#### 2. Embedded Deployment (v0.1.0)

```bash
# Commit to operator repo
git add cue/platform/v1.4.0
git commit -m "Add platform module v1.4.0 with HPA support"

# Build operator
make docker-build IMG=myregistry.com/pequod:v0.2.0

# Deploy
kubectl set image deployment/pequod-controller-manager \
    manager=myregistry.com/pequod:v0.2.0
```

#### 3. OCI Deployment (v0.2.0+)

```bash
# Package and push
docker build -t myregistry.com/platform-modules/webservice:v1.4.0 \
    cue/platform/v1.4.0
docker push myregistry.com/platform-modules/webservice:v1.4.0

# Get digest
DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' \
    myregistry.com/platform-modules/webservice:v1.4.0)

# Announce to developers
echo "New module: $DIGEST"
```

#### 4. Developers Update

```yaml
# Old
spec:
  platformRef:
    embedded: v1.3.0

# New
spec:
  platformRef:
    oci: "myregistry.com/platform-modules/webservice@sha256:xyz789..."
```

---

## Summary

**For v0.1.0 (Phase 1):**
- Use **embedded modules** exclusively
- Platform team updates modules by rebuilding operator
- Simple, reliable, no external dependencies

**For v0.2.0+ (Phase 9):**
- Add **OCI registry** support for production
- Platform team publishes modules independently
- Developers reference by immutable digest
- Operator caches modules for performance

**Optional (Phase 9+):**
- Add **Git repository** support for GitOps workflows
- Useful for teams already using Git-based workflows


