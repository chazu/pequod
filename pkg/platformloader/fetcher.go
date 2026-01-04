package platformloader

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FetchResult contains the result of fetching a CUE module
type FetchResult struct {
	// Content is the raw CUE module content (file bytes or directory structure)
	Content []byte

	// Digest is a content-addressable identifier for the module
	// For OCI: manifest digest (sha256:...)
	// For Git: commit SHA
	// For ConfigMap: resourceVersion or content hash
	// For Inline: content hash
	Digest string

	// Source describes where the module was fetched from (for logging/debugging)
	Source string
}

// Fetcher defines the interface for fetching CUE modules from various sources
type Fetcher interface {
	// Fetch retrieves a CUE module from the source
	// ctx: context for cancellation and timeouts
	// ref: the reference string (OCI image, git URL, etc.)
	// pullSecret: optional secret for authentication (may be nil)
	Fetch(ctx context.Context, ref string, pullSecret *corev1.Secret) (*FetchResult, error)

	// Type returns the type of fetcher (for logging and metrics)
	Type() string
}

// FetcherRegistry manages all available fetchers
type FetcherRegistry struct {
	fetchers map[string]Fetcher
	client   client.Client
}

// NewFetcherRegistry creates a new fetcher registry with all supported fetchers
func NewFetcherRegistry(k8sClient client.Client, cacheDir string) *FetcherRegistry {
	diskCache := NewDiskCache(cacheDir)

	registry := &FetcherRegistry{
		fetchers: make(map[string]Fetcher),
		client:   k8sClient,
	}

	// Register all fetchers
	registry.fetchers["oci"] = NewOCIFetcher(diskCache)
	registry.fetchers["git"] = NewGitFetcher(diskCache)
	registry.fetchers["configmap"] = NewConfigMapFetcher(k8sClient)
	registry.fetchers["inline"] = NewInlineFetcher()
	registry.fetchers["embedded"] = NewEmbeddedFetcher()

	return registry
}

// GetFetcher returns the fetcher for the given type
func (r *FetcherRegistry) GetFetcher(fetcherType string) (Fetcher, error) {
	fetcher, ok := r.fetchers[fetcherType]
	if !ok {
		return nil, fmt.Errorf("unsupported fetcher type: %s", fetcherType)
	}
	return fetcher, nil
}

// Fetch fetches a CUE module using the appropriate fetcher
func (r *FetcherRegistry) Fetch(ctx context.Context, fetcherType, ref string, pullSecret *corev1.Secret) (*FetchResult, error) {
	fetcher, err := r.GetFetcher(fetcherType)
	if err != nil {
		return nil, err
	}
	return fetcher.Fetch(ctx, ref, pullSecret)
}

// FetchWithSecretRef fetches a CUE module, resolving the secret reference if provided
func (r *FetcherRegistry) FetchWithSecretRef(ctx context.Context, fetcherType, ref, namespace string, secretRef *string) (*FetchResult, error) {
	var pullSecret *corev1.Secret

	if secretRef != nil && *secretRef != "" {
		pullSecret = &corev1.Secret{}
		if err := r.client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      *secretRef,
		}, pullSecret); err != nil {
			return nil, fmt.Errorf("failed to get pull secret %s: %w", *secretRef, err)
		}
	}

	return r.Fetch(ctx, fetcherType, ref, pullSecret)
}
