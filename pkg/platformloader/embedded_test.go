package platformloader

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedFetcher_Fetch(t *testing.T) {
	// Create an in-memory filesystem using fstest.MapFS
	testFS := fstest.MapFS{
		"platform/testmodule/main.cue": &fstest.MapFile{
			Data: []byte(`
package testmodule

name: "test"
value: 42
`),
		},
	}

	// Create fetcher with the test filesystem
	fetcher := NewEmbeddedFetcher(testFS, "platform")

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
	testFS := fstest.MapFS{
		"platform/existing/main.cue": &fstest.MapFile{
			Data: []byte("test: true"),
		},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	_, err := fetcher.Fetch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("Expected error for nonexistent module, got nil")
	}
}

func TestEmbeddedFetcher_FetchEmptyRef(t *testing.T) {
	testFS := fstest.MapFS{}
	fetcher := NewEmbeddedFetcher(testFS, "platform")

	_, err := fetcher.Fetch(context.Background(), "", nil)
	if err == nil {
		t.Error("Expected error for empty ref, got nil")
	}
}

func TestEmbeddedFetcher_NilFS(t *testing.T) {
	fetcher := NewEmbeddedFetcher(nil, "platform")

	_, err := fetcher.Fetch(context.Background(), "testmodule", nil)
	if err == nil {
		t.Error("Expected error for nil filesystem, got nil")
	}
}

func TestEmbeddedFetcher_Type(t *testing.T) {
	fetcher := NewEmbeddedFetcher(nil, "platform")
	if fetcher.Type() != "embedded" {
		t.Errorf("Expected type 'embedded', got %q", fetcher.Type())
	}
}

func TestEmbeddedFetcher_ListModules(t *testing.T) {
	testFS := fstest.MapFS{
		"platform/module1/main.cue": &fstest.MapFile{Data: []byte("test: 1")},
		"platform/module2/main.cue": &fstest.MapFile{Data: []byte("test: 2")},
		"platform/.hidden/main.cue": &fstest.MapFile{Data: []byte("hidden: true")},
		"platform/notamodule.txt":   &fstest.MapFile{Data: []byte("not a module")},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	modules, err := fetcher.ListEmbeddedModules()
	if err != nil {
		t.Fatalf("ListEmbeddedModules failed: %v", err)
	}

	// Should find module1 and module2 but not .hidden (hidden) or notamodule.txt (file)
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
	testFS := fstest.MapFS{
		"platform/multi/schema.cue": &fstest.MapFile{Data: []byte("schema: true")},
		"platform/multi/render.cue": &fstest.MapFile{Data: []byte("render: true")},
		"platform/multi/policy.cue": &fstest.MapFile{Data: []byte("policy: true")},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

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

func TestEmbeddedFetcher_NestedDirectories(t *testing.T) {
	testFS := fstest.MapFS{
		"platform/nested/main.cue":           &fstest.MapFile{Data: []byte("main: true")},
		"platform/nested/subdir/nested.cue":  &fstest.MapFile{Data: []byte("nested: true")},
		"platform/nested/.hidden/hidden.cue": &fstest.MapFile{Data: []byte("hidden: true")},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	result, err := fetcher.Fetch(context.Background(), "nested", nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	content := string(result.Content)
	if !strings.Contains(content, "main:") {
		t.Error("Missing main.cue content")
	}
	if !strings.Contains(content, "nested:") {
		t.Error("Missing nested subdirectory content")
	}
	if strings.Contains(content, "hidden:") {
		t.Error("Should not include hidden directory content")
	}
}

func TestEmbeddedFetcher_NonCUEFilesIgnored(t *testing.T) {
	testFS := fstest.MapFS{
		"platform/mixed/main.cue":    &fstest.MapFile{Data: []byte("cue: true")},
		"platform/mixed/readme.txt":  &fstest.MapFile{Data: []byte("readme content")},
		"platform/mixed/config.json": &fstest.MapFile{Data: []byte(`{"json": true}`)},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	result, err := fetcher.Fetch(context.Background(), "mixed", nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	content := string(result.Content)
	if !strings.Contains(content, "cue:") {
		t.Error("Should contain CUE file content")
	}
	if strings.Contains(content, "readme") {
		t.Error("Should not contain txt file content")
	}
	if strings.Contains(content, "json") {
		t.Error("Should not contain json file content")
	}
}

func TestEmbeddedFetcher_EmptyModule(t *testing.T) {
	// Module directory exists but has no .cue files
	testFS := fstest.MapFS{
		"platform/empty/readme.txt": &fstest.MapFile{Data: []byte("no cue files")},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	_, err := fetcher.Fetch(context.Background(), "empty", nil)
	if err == nil {
		t.Error("Expected error for empty module (no .cue files), got nil")
	}
}

func TestEmbeddedFetcher_DigestConsistency(t *testing.T) {
	testFS := fstest.MapFS{
		"platform/consistent/main.cue": &fstest.MapFile{Data: []byte("test: true")},
	}

	fetcher := NewEmbeddedFetcher(testFS, "platform")

	// Fetch twice and verify digest is consistent
	result1, err := fetcher.Fetch(context.Background(), "consistent", nil)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}

	result2, err := fetcher.Fetch(context.Background(), "consistent", nil)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}

	if result1.Digest != result2.Digest {
		t.Errorf("Digests should be consistent: %q != %q", result1.Digest, result2.Digest)
	}
}
