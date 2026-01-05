package platformloader

import (
	"context"
	"testing"
	"testing/fstest"

	"cuelang.org/go/cue"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader()
	if loader == nil {
		t.Fatal("expected non-nil loader")
	}

	if loader.ctx == nil {
		t.Error("expected non-nil CUE context")
	}

	if loader.cache == nil {
		t.Error("expected non-nil cache")
	}
}

func TestNewLoaderWithConfig_EmbeddedFS(t *testing.T) {
	// Create a mock filesystem with a test module
	testFS := fstest.MapFS{
		"platform/testmodule/schema.cue": &fstest.MapFile{
			Data: []byte(`
package testmodule

#TestSpec: {
	name: string
	value: int
}

#Render: {
	input: #TestSpec
	output: {
		metadata: {
			name: input.name
		}
	}
}
`),
		},
	}

	// Create loader with embedded filesystem (needs a fake k8s client to init fetchers)
	// For this test, we'll test the fetcher directly instead
	fetcher := NewEmbeddedFetcher(testFS, "platform")

	result, err := fetcher.Fetch(context.Background(), "testmodule", nil)
	if err != nil {
		t.Fatalf("failed to fetch embedded module: %v", err)
	}

	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}

	// Verify the content can be compiled
	loader := NewLoader()
	value, err := loader.LoadFromContent(result.Content)
	if err != nil {
		t.Fatalf("failed to compile fetched content: %v", err)
	}

	// Verify the value contains expected definitions
	specDef := value.LookupPath(cue.ParsePath("#TestSpec"))
	if !specDef.Exists() {
		t.Error("expected #TestSpec definition to exist")
	}

	renderDef := value.LookupPath(cue.ParsePath("#Render"))
	if !renderDef.Exists() {
		t.Error("expected #Render definition to exist")
	}
}

func TestLoadFromPath(t *testing.T) {
	loader := NewLoader()

	// Load from the actual cue/platform/webservice directory
	value, err := loader.LoadFromPath("../../cue/platform/webservice")
	if err != nil {
		t.Fatalf("failed to load from path: %v", err)
	}

	if value.Err() != nil {
		t.Fatalf("loaded value has error: %v", value.Err())
	}

	// Verify the value contains expected definitions
	specDef := value.LookupPath(cue.ParsePath("#WebServiceSpec"))
	if !specDef.Exists() {
		t.Error("expected #WebServiceSpec definition to exist")
	}
}

func TestLoadFromContent(t *testing.T) {
	loader := NewLoader()

	content := []byte(`
package test

#MySpec: {
	name: string
	count: int | *1
}

result: #MySpec & {
	name: "test"
	count: 5
}
`)

	value, err := loader.LoadFromContent(content)
	if err != nil {
		t.Fatalf("failed to load from content: %v", err)
	}

	if value.Err() != nil {
		t.Fatalf("loaded value has error: %v", value.Err())
	}

	// Verify the value contains expected definitions
	specDef := value.LookupPath(cue.ParsePath("#MySpec"))
	if !specDef.Exists() {
		t.Error("expected #MySpec definition to exist")
	}

	// Verify we can read the result
	resultVal := value.LookupPath(cue.ParsePath("result.name"))
	if !resultVal.Exists() {
		t.Error("expected result.name to exist")
	}

	name, err := resultVal.String()
	if err != nil {
		t.Fatalf("failed to get result.name: %v", err)
	}
	if name != "test" {
		t.Errorf("expected name 'test', got %q", name)
	}
}

func TestContext(t *testing.T) {
	loader := NewLoader()

	ctx := loader.Context()
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	// Verify we can use the context
	value := ctx.CompileString(`x: 1`)
	if value.Err() != nil {
		t.Errorf("failed to compile with context: %v", value.Err())
	}
}
