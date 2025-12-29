# Pequod Dagger CI/CD

This directory contains the Dagger module for Pequod's CI/CD pipelines.

## What is Dagger?

Dagger is a programmable CI/CD engine that runs your pipelines in containers. The same pipeline runs identically on your local machine and in CI.

## Installation

### macOS
```bash
brew install dagger/tap/dagger
```

### Linux
```bash
curl -L https://dl.dagger.io/dagger/install.sh | sh
```

### Verify Installation
```bash
dagger version
```

## Available Commands

### Run Tests
```bash
dagger call test --source=.
```

### Run Linter
```bash
dagger call lint --source=.
```

### Build Binary
```bash
dagger call build --source=. export --path=./bin/manager
```

### Build Docker Image
```bash
dagger call build-image --source=. --name=pequod-controller --tag=latest
```

### Run E2E Tests
```bash
# Requires Docker to be running
dagger call e2e --source=.
```

### Run Full CI Pipeline
```bash
dagger call ci --source=.
```

## Using the Helper Script

A convenience script is provided at `scripts/dagger-ci.sh`:

```bash
# Run tests
./scripts/dagger-ci.sh test

# Run linter
./scripts/dagger-ci.sh lint

# Build binary
./scripts/dagger-ci.sh build

# Build Docker image
./scripts/dagger-ci.sh image

# Run E2E tests
./scripts/dagger-ci.sh e2e

# Run full CI
./scripts/dagger-ci.sh ci
```

## GitHub Actions Integration

The Dagger pipeline is integrated with GitHub Actions in `.github/workflows/dagger-ci.yml`.

This workflow:
1. Runs on every push and pull request
2. Executes tests and linting in parallel
3. Builds the controller binary
4. Runs E2E tests in a separate job

## Benefits of Dagger

1. **Local Development**: Run the exact same CI pipeline locally
2. **Fast Feedback**: No need to push to see CI results
3. **Reproducible**: Containers ensure consistency across environments
4. **Cacheable**: Dagger caches layers for faster subsequent runs
5. **Debuggable**: Easy to debug failures locally

## Module Structure

The Dagger module is defined in `main.go` and provides these functions:

- `Test`: Runs `make test` in a Go container
- `Lint`: Runs `golangci-lint` in a linter container
- `Build`: Compiles the controller binary
- `BuildImage`: Builds the Docker image
- `E2E`: Runs end-to-end tests with Kind
- `CI`: Runs the full CI pipeline (test + lint)

## Customization

To modify the CI pipeline, edit `dagger/main.go`. The module is written in Go and uses the Dagger SDK.

Example: Adding a new step
```go
// Format runs go fmt
func (p *Pequod) Format(ctx context.Context, source *Directory) (string, error) {
    return dag.Container().
        From("golang:1.23").
        WithDirectory("/src", source).
        WithWorkdir("/src").
        WithExec([]string{"go", "fmt", "./..."}).
        Stdout(ctx)
}
```

Then call it:
```bash
dagger call format --source=.
```

## Troubleshooting

### "Docker daemon not running"
Make sure Docker Desktop is running before executing Dagger commands.

### "Permission denied"
On Linux, you may need to add your user to the `docker` group:
```bash
sudo usermod -aG docker $USER
newgrp docker
```

### Slow first run
The first run downloads container images and builds caches. Subsequent runs are much faster.

### E2E tests fail
E2E tests require:
- Docker running
- Sufficient resources (4GB+ RAM recommended)
- Network access to pull images

## Learn More

- [Dagger Documentation](https://docs.dagger.io)
- [Dagger Go SDK](https://docs.dagger.io/sdk/go)
- [Dagger Examples](https://github.com/dagger/dagger/tree/main/docs/current_docs/guides)

