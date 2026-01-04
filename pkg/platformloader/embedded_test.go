package platformloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedFetcher_Fetch(t *testing.T) {
	// Create a temporary directory structure with CUE files
	tmpDir, err := os.MkdirTemp("", "embedded-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test module
	moduleDir := filepath.Join(tmpDir, "testmodule")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("Failed to create module dir: %v", err)
	}

	// Create CUE files
	cueContent := `
package testmodule

name: "test"
value: 42
`
	if err := os.WriteFile(filepath.Join(moduleDir, "main.cue"), []byte(cueContent), 0644); err != nil {
		t.Fatalf("Failed to create CUE file: %v", err)
	}

	// Create fetcher with custom paths
	fetcher := NewEmbeddedFetcherWithPaths([]string{tmpDir})

	// Test fetch
	result, err := fetcher.Fetch(context.Background(), "testmodule", nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if !strings.Contains(string(result.Content), "name:") {
		t.Errorf("Content doesn't contain expected CUE: %s", result.Content)
	}

	if !strings.HasPrefix(result.Source, "embedded://") {
		t.Errorf("Expected source to start with 'embedded://', got %q", result.Source)
	}

	if !strings.HasPrefix(result.Digest, "embedded:") {
		t.Errorf("Expected digest to start with 'embedded:', got %q", result.Digest)
	}
}

func TestEmbeddedFetcher_FetchNonexistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "embedded-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher := NewEmbeddedFetcherWithPaths([]string{tmpDir})

	_, err = fetcher.Fetch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("Expected error for nonexistent module, got nil")
	}
}

func TestEmbeddedFetcher_FetchEmptyRef(t *testing.T) {
	fetcher := NewEmbeddedFetcher()

	_, err := fetcher.Fetch(context.Background(), "", nil)
	if err == nil {
		t.Error("Expected error for empty ref, got nil")
	}
}

func TestEmbeddedFetcher_Type(t *testing.T) {
	fetcher := NewEmbeddedFetcher()
	if fetcher.Type() != "embedded" {
		t.Errorf("Expected type 'embedded', got %q", fetcher.Type())
	}
}

func TestEmbeddedFetcher_ListModules(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "embedded-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module directories
	os.MkdirAll(filepath.Join(tmpDir, "module1"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "module2"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".hidden"), 0755) // Hidden dir should be excluded

	fetcher := NewEmbeddedFetcherWithPaths([]string{tmpDir})

	modules, err := fetcher.ListEmbeddedModules()
	if err != nil {
		t.Fatalf("ListEmbeddedModules failed: %v", err)
	}

	// Should find module1 and module2 but not .hidden
	if len(modules) != 2 {
		t.Errorf("Expected 2 modules, got %d: %v", len(modules), modules)
	}

	found := make(map[string]bool)
	for _, m := range modules {
		found[m] = true
	}

	if !found["module1"] {
		t.Error("Expected to find module1")
	}
	if !found["module2"] {
		t.Error("Expected to find module2")
	}
	if found[".hidden"] {
		t.Error("Should not find hidden directory")
	}
}

func TestEmbeddedFetcher_MultipleCUEFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "embedded-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	moduleDir := filepath.Join(tmpDir, "multi")
	os.MkdirAll(moduleDir, 0755)

	// Create multiple CUE files
	os.WriteFile(filepath.Join(moduleDir, "schema.cue"), []byte("schema: true"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "render.cue"), []byte("render: true"), 0644)
	os.WriteFile(filepath.Join(moduleDir, "policy.cue"), []byte("policy: true"), 0644)

	fetcher := NewEmbeddedFetcherWithPaths([]string{tmpDir})

	result, err := fetcher.Fetch(context.Background(), "multi", nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// All files should be included
	content := string(result.Content)
	if !strings.Contains(content, "schema:") {
		t.Error("Missing schema.cue content")
	}
	if !strings.Contains(content, "render:") {
		t.Error("Missing render.cue content")
	}
	if !strings.Contains(content, "policy:") {
		t.Error("Missing policy.cue content")
	}
}

func TestReadCUEFilesFromDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cue-read-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create CUE file
	os.WriteFile(filepath.Join(tmpDir, "test.cue"), []byte("test: 1"), 0644)

	// Create non-CUE file (should be ignored)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644)

	// Create hidden directory with CUE file (should be skipped)
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)
	os.WriteFile(filepath.Join(hiddenDir, "hidden.cue"), []byte("hidden: true"), 0644)

	content, err := readCUEFilesFromDir(tmpDir)
	if err != nil {
		t.Fatalf("readCUEFilesFromDir failed: %v", err)
	}

	if !strings.Contains(string(content), "test:") {
		t.Error("Should contain test.cue content")
	}
	if strings.Contains(string(content), "hidden:") {
		t.Error("Should not contain hidden directory content")
	}
	if strings.Contains(string(content), "readme") {
		t.Error("Should not contain non-CUE files")
	}
}

func TestReadCUEFilesFromDir_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cue-read-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = readCUEFilesFromDir(tmpDir)
	if err == nil {
		t.Error("Expected error for empty directory, got nil")
	}
}
