package platformloader

import (
	"context"
	"fmt"

	"github.com/cespare/xxhash/v2"
	corev1 "k8s.io/api/core/v1"
)

// InlineFetcher handles inline CUE content embedded directly in the Transform spec
type InlineFetcher struct{}

// NewInlineFetcher creates a new inline fetcher
func NewInlineFetcher() *InlineFetcher {
	return &InlineFetcher{}
}

// Type returns the fetcher type
func (f *InlineFetcher) Type() string {
	return "inline"
}

// Fetch returns the inline CUE content directly
// The ref parameter IS the CUE content itself
// pullSecret is ignored for inline fetches
func (f *InlineFetcher) Fetch(ctx context.Context, ref string, _ *corev1.Secret) (*FetchResult, error) {
	if ref == "" {
		return nil, fmt.Errorf("inline CUE content is empty")
	}

	content := []byte(ref)

	// Compute a content-based digest
	digest := fmt.Sprintf("inline:%x", xxhash.Sum64(content))

	return &FetchResult{
		Content: content,
		Digest:  digest,
		Source:  "inline",
	}, nil
}
