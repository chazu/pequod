package platformloader

import (
	"testing"

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

func TestLoadEmbedded(t *testing.T) {
	loader := NewLoader()

	// Load embedded module
	value, err := loader.LoadEmbedded("embedded")
	if err != nil {
		t.Fatalf("failed to load embedded module: %v", err)
	}

	if value.Err() != nil {
		t.Fatalf("loaded value has error: %v", value.Err())
	}

	// Verify the value contains expected definitions
	// Check for #WebServiceSpec
	specDef := value.LookupPath(cuePathFromString("#WebServiceSpec"))
	if !specDef.Exists() {
		t.Error("expected #WebServiceSpec definition to exist")
	}

	// Check for #Render
	renderDef := value.LookupPath(cuePathFromString("#Render"))
	if !renderDef.Exists() {
		t.Error("expected #Render definition to exist")
	}
}

func TestLoadEmbeddedCaching(t *testing.T) {
	loader := NewLoader()

	// First load
	value1, err := loader.LoadEmbedded("embedded")
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	// Verify cache is populated
	if loader.cache.Size() != 1 {
		t.Errorf("expected cache size 1, got %d", loader.cache.Size())
	}

	// Second load should come from cache
	value2, err := loader.LoadEmbedded("embedded")
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}

	// Both values should be valid
	if value1.Err() != nil {
		t.Errorf("value1 has error: %v", value1.Err())
	}
	if value2.Err() != nil {
		t.Errorf("value2 has error: %v", value2.Err())
	}

	// Cache should still have only one entry
	if loader.cache.Size() != 1 {
		t.Errorf("expected cache size 1 after second load, got %d", loader.cache.Size())
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
	specDef := value.LookupPath(cuePathFromString("#WebServiceSpec"))
	if !specDef.Exists() {
		t.Error("expected #WebServiceSpec definition to exist")
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

// Helper function to create a CUE path from a string
func cuePathFromString(s string) cue.Path {
	return cue.ParsePath(s)
}
