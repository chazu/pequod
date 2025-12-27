# Getting Started with Pequod Development

This guide walks you through setting up your development environment and starting Phase 0 of the project.

## Prerequisites

### Required Tools
- **Go 1.21+**: [Download](https://go.dev/dl/)
- **Kubebuilder 3.x+**: [Installation](https://book.kubebuilder.io/quick-start.html#installation)
- **Docker**: For building container images
- **kubectl**: Kubernetes CLI
- **kind** or **k3d**: Local Kubernetes cluster

### Optional Tools
- **golangci-lint**: Code linting
- **make**: Build automation (usually pre-installed on macOS/Linux)
- **git**: Version control

## Installation Steps

### 1. Install Go
```bash
# macOS (using Homebrew)
brew install go

# Linux
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Verify
go version
```

### 2. Install Kubebuilder
```bash
# macOS
brew install kubebuilder

# Linux
curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/

# Verify
kubebuilder version
```

### 3. Install Docker
```bash
# macOS
brew install --cask docker

# Linux - follow official docs
# https://docs.docker.com/engine/install/

# Verify
docker --version
```

### 4. Install kind (Kubernetes in Docker)
```bash
# macOS/Linux
go install sigs.k8s.io/kind@latest

# Or using Homebrew (macOS)
brew install kind

# Verify
kind version
```

### 5. Install kubectl
```bash
# macOS
brew install kubectl

# Linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/

# Verify
kubectl version --client
```

### 6. Install golangci-lint (Optional but Recommended)
```bash
# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Verify
golangci-lint --version
```

## Phase 0: Project Setup

Follow these steps to complete Phase 0 from [phases.md](phases.md).

### Task 0.1: Initialize Kubebuilder Project

```bash
# Navigate to project directory
cd /Users/chazu/dev/go/pequod

# Initialize Kubebuilder project
kubebuilder init --domain platform.example.com --repo github.com/yourorg/pequod

# Verify build
make build

# Run tests
make test
```

**Expected Output**: Project scaffolded with controller-runtime, builds successfully.

### Task 0.2: Add Core Dependencies

```bash
# Add DAG library
go get github.com/dominikbraun/graph@latest

# Add CUE library
go get cuelang.org/go@latest

# Add Prometheus client
go get github.com/prometheus/client_golang@latest

# Add testing libraries (if not already present)
go get github.com/onsi/ginkgo/v2@latest
go get github.com/onsi/gomega@latest

# Clean up and verify
go mod tidy
go mod verify

# Verify all dependencies
go list -m all
```

**Expected Output**: All dependencies added to `go.mod`, no conflicts.

### Task 0.3: Set Up Development Environment

```bash
# Create enhanced Makefile targets
cat >> Makefile << 'EOF'

# Lint code
.PHONY: lint
lint:
	golangci-lint run ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Create local kind cluster
.PHONY: cluster-up
cluster-up:
	kind create cluster --name pequod

# Delete local kind cluster
.PHONY: cluster-down
cluster-down:
	kind delete cluster --name pequod

# Install CRDs into cluster
.PHONY: install-crds
install-crds: manifests
	kubectl apply -f config/crd/bases/
EOF

# Create golangci-lint configuration
cat > .golangci.yml << 'EOF'
linters:
  enable:
    - gofmt
    - govet
    - errcheck
    - staticcheck
    - unused
    - gosimple
    - ineffassign
    - typecheck

linters-settings:
  govet:
    check-shadowing: true

run:
  timeout: 5m
  tests: true
EOF

# Create kind cluster configuration
cat > kind-config.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30000
    hostPort: 30000
    protocol: TCP
EOF

# Test the setup
make lint
make test
make cluster-up
```

**Expected Output**: Makefile targets work, cluster created successfully.

### Task 0.4: Define Project Structure

```bash
# Create package directories
mkdir -p pkg/graph
mkdir -p pkg/apply
mkdir -p pkg/readiness
mkdir -p pkg/platformloader
mkdir -p pkg/inventory
mkdir -p cue/platform
mkdir -p test/e2e

# Create package documentation files
cat > pkg/graph/doc.go << 'EOF'
// Package graph provides types and execution logic for the Graph artifact.
// It includes the DAG executor that applies resources in dependency order
// with readiness gates.
package graph
EOF

cat > pkg/apply/doc.go << 'EOF'
// Package apply provides Server-Side Apply operations for Kubernetes resources.
// It includes the SSA applier and pruning logic for authoritative reconciliation.
package apply
EOF

cat > pkg/readiness/doc.go << 'EOF'
// Package readiness provides readiness predicate evaluation for resources.
// It supports various predicate types like ConditionMatch, DeploymentAvailable, etc.
package readiness
EOF

cat > pkg/platformloader/doc.go << 'EOF'
// Package platformloader handles loading and evaluating CUE platform modules.
// It supports embedded modules and remote modules (OCI/Git) with caching.
package platformloader
EOF

cat > pkg/inventory/doc.go << 'EOF'
// Package inventory provides inventory tracking for managed resources.
// It tracks applied resources, detects drift, and identifies orphaned resources.
package inventory
EOF

# Verify structure
tree -L 2 pkg/
```

**Expected Output**: All directories and doc.go files created.

## Verify Phase 0 Completion

Run this checklist to ensure Phase 0 is complete:

```bash
# 1. Project builds
make build
echo "✓ Project builds successfully"

# 2. Tests pass
make test
echo "✓ Tests pass"

# 3. Linting works
make lint
echo "✓ Linting passes"

# 4. Dependencies verified
go mod verify
echo "✓ Dependencies verified"

# 5. Cluster can be created
make cluster-up
kubectl cluster-info
make cluster-down
echo "✓ Local cluster works"

# 6. Structure exists
ls -la pkg/
echo "✓ Package structure created"
```

## Next Steps

Once Phase 0 is complete, proceed to **Phase 1: Core Types and CRD** in [phases.md](phases.md).

### Phase 1 Preview

You'll be creating:
1. WebService CRD with Kubebuilder
2. Graph artifact types
3. Readiness predicate types
4. Inventory types

Command to start Phase 1:
```bash
kubebuilder create api --group platform --version v1alpha1 --kind WebService
```

## Troubleshooting

### Kubebuilder init fails
- Ensure Go is in PATH: `echo $PATH | grep go`
- Verify Go version: `go version` (need 1.21+)
- Check Kubebuilder version: `kubebuilder version`

### Dependencies fail to download
- Check network connectivity
- Try with Go proxy: `export GOPROXY=https://proxy.golang.org,direct`
- Clear module cache: `go clean -modcache`

### kind cluster fails to start
- Ensure Docker is running: `docker ps`
- Check Docker resources (need 2GB+ RAM)
- Try deleting existing cluster: `kind delete cluster --name pequod`

### Tests fail
- Ensure envtest binaries are installed: `make envtest`
- Check Go version compatibility
- Review test output for specific errors

## Resources

- **Kubebuilder Book**: https://book.kubebuilder.io/
- **controller-runtime Docs**: https://pkg.go.dev/sigs.k8s.io/controller-runtime
- **CUE Documentation**: https://cuelang.org/docs/
- **dominikbraun/graph**: https://github.com/dominikbraun/graph
- **Project Phases**: [phases.md](phases.md)
- **Architecture**: [plan.md](plan.md)

## Getting Help

- Review [phases.md](phases.md) for detailed task descriptions
- Check [LIBRARIES.md](LIBRARIES.md) for library usage examples
- Consult [plan.md](plan.md) for architectural decisions

