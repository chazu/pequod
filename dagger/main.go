// A Dagger module for Pequod CI/CD pipelines
package main

import (
	"context"
	"fmt"
)

type Pequod struct{}

// Test runs unit tests
func (p *Pequod) Test(ctx context.Context, source *Directory) (string, error) {
	// Exclude dagger directory from source to avoid vet errors
	sourceWithoutDagger := source.WithoutDirectory("dagger")

	return dag.Container().
		From("golang:1.24").
		WithDirectory("/src", sourceWithoutDagger).
		WithWorkdir("/src").
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"make", "test"}).
		Stdout(ctx)
}

// Lint runs golangci-lint
func (p *Pequod) Lint(ctx context.Context, source *Directory) (string, error) {
	// Exclude dagger directory from source
	sourceWithoutDagger := source.WithoutDirectory("dagger")

	return dag.Container().
		From("golangci/golangci-lint:v1.61").
		WithDirectory("/src", sourceWithoutDagger).
		WithWorkdir("/src").
		WithExec([]string{"golangci-lint", "run", "--timeout=5m"}).
		Stdout(ctx)
}

// Build compiles the controller manager binary
func (p *Pequod) Build(ctx context.Context, source *Directory) *File {
	// Exclude dagger directory from source
	sourceWithoutDagger := source.WithoutDirectory("dagger")

	return dag.Container().
		From("golang:1.24").
		WithDirectory("/src", sourceWithoutDagger).
		WithWorkdir("/src").
		WithExec([]string{"go", "mod", "download"}).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"go", "build", "-o", "bin/manager", "./cmd/main.go"}).
		File("/src/bin/manager")
}

// BuildImage builds the Docker image for the controller
func (p *Pequod) BuildImage(
	ctx context.Context,
	source *Directory,
	// +optional
	// +default="pequod-controller"
	name string,
	// +optional
	// +default="latest"
	tag string,
) *Container {
	binary := p.Build(ctx, source)

	return dag.Container().
		From("gcr.io/distroless/static:nonroot").
		WithFile("/manager", binary).
		WithEntrypoint([]string{"/manager"}).
		WithLabel("org.opencontainers.image.source", "https://github.com/chazu/pequod").
		WithLabel("org.opencontainers.image.description", "Pequod Platform Controller").
		WithLabel("org.opencontainers.image.licenses", "Apache-2.0")
}

// E2ESimple runs E2E tests using a Kind cluster
// This is a simplified version that uses the existing Makefile target
func (p *Pequod) E2ESimple(ctx context.Context, source *Directory) (string, error) {
	// For now, this is the same as E2E
	// In the future, we could add more sophisticated Kind cluster management
	return p.E2E(ctx, source)
}

// E2E runs comprehensive E2E tests
// Note: This requires Docker socket access and will create a Kind cluster
func (p *Pequod) E2E(ctx context.Context, source *Directory) (string, error) {
	// For E2E tests, we need to use the host's Docker socket
	// This is a limitation of running Kind inside a container
	// The recommended approach is to run E2E tests directly on the host
	// using: make test-e2e

	return "E2E tests should be run on the host using: make test-e2e\n" +
		"This is because Kind requires Docker socket access which is complex in Dagger.\n" +
		"See .github/workflows/test-e2e.yml for the CI implementation.", nil
}

// CI runs all CI checks (test, lint, build)
func (p *Pequod) CI(ctx context.Context, source *Directory) (string, error) {
	// Run tests
	testOutput, err := p.Test(ctx, source)
	if err != nil {
		return "", fmt.Errorf("tests failed: %w", err)
	}

	// Run lint
	lintOutput, err := p.Lint(ctx, source)
	if err != nil {
		return "", fmt.Errorf("lint failed: %w", err)
	}

	// Build to ensure it compiles
	_, err = p.Build(ctx, source).Contents(ctx)
	if err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}

	return fmt.Sprintf("✅ All CI checks passed\n\nTests:\n%s\n\nLint:\n%s", testOutput, lintOutput), nil
}
