package platformloader

import (
	"context"
	"strings"
	"testing"
)

func TestInlineFetcher_Fetch(t *testing.T) {
	fetcher := NewInlineFetcher()

	tests := []struct {
		name        string
		ref         string
		wantErr     bool
		wantContent string
	}{
		{
			name: "simple CUE",
			ref: `
				name: "test"
				value: 42
			`,
			wantErr:     false,
			wantContent: "name:",
		},
		{
			name:        "empty ref",
			ref:         "",
			wantErr:     true,
			wantContent: "",
		},
		{
			name: "complex CUE",
			ref: `
				#Input: {
					name: string
					replicas: int | *1
				}

				graph: {
					nodes: []
				}
			`,
			wantErr:     false,
			wantContent: "#Input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fetcher.Fetch(context.Background(), tt.ref, nil)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !strings.Contains(string(result.Content), tt.wantContent) {
				t.Errorf("Content doesn't contain %q: %s", tt.wantContent, result.Content)
			}

			if result.Source != "inline" {
				t.Errorf("Expected source 'inline', got %q", result.Source)
			}

			if !strings.HasPrefix(result.Digest, "inline:") {
				t.Errorf("Expected digest to start with 'inline:', got %q", result.Digest)
			}
		})
	}
}

func TestInlineFetcher_Type(t *testing.T) {
	fetcher := NewInlineFetcher()
	if fetcher.Type() != "inline" {
		t.Errorf("Expected type 'inline', got %q", fetcher.Type())
	}
}

func TestInlineFetcher_DigestConsistency(t *testing.T) {
	fetcher := NewInlineFetcher()
	ref := "test: 123"

	result1, err := fetcher.Fetch(context.Background(), ref, nil)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}

	result2, err := fetcher.Fetch(context.Background(), ref, nil)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}

	if result1.Digest != result2.Digest {
		t.Errorf("Digests should be consistent: %q != %q", result1.Digest, result2.Digest)
	}
}

func TestInlineFetcher_DifferentContentDifferentDigest(t *testing.T) {
	fetcher := NewInlineFetcher()

	result1, _ := fetcher.Fetch(context.Background(), "content1", nil)
	result2, _ := fetcher.Fetch(context.Background(), "content2", nil)

	if result1.Digest == result2.Digest {
		t.Error("Different content should produce different digests")
	}
}
