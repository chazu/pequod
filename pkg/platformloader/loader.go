package platformloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Loader handles loading and caching of CUE platform modules
type Loader struct {
	ctx      *cue.Context
	cache    *Cache
	fetchers *FetcherRegistry
}

// LoaderConfig contains configuration for the Loader
type LoaderConfig struct {
	// CacheDir is the directory for the disk cache
	CacheDir string

	// K8sClient is the Kubernetes client for ConfigMap fetching
	K8sClient client.Client
}

// NewLoader creates a new platform loader with caching
func NewLoader() *Loader {
	return &Loader{
		ctx:   cuecontext.New(),
		cache: NewCache(),
	}
}

// NewLoaderWithConfig creates a platform loader with fetcher support
func NewLoaderWithConfig(config LoaderConfig) *Loader {
	loader := &Loader{
		ctx:   cuecontext.New(),
		cache: NewCache(),
	}

	if config.K8sClient != nil {
		cacheDir := config.CacheDir
		if cacheDir == "" {
			cacheDir = DefaultCacheDir
		}
		loader.fetchers = NewFetcherRegistry(config.K8sClient, cacheDir)
	}

	return loader
}

// FetchModule fetches a CUE module using the appropriate fetcher
func (l *Loader) FetchModule(ctx context.Context, fetcherType, ref, namespace string, pullSecretRef *string) (*FetchResult, error) {
	if l.fetchers == nil {
		return nil, fmt.Errorf("fetchers not initialized; use NewLoaderWithConfig")
	}

	// Get the pull secret if specified
	var pullSecret *corev1.Secret
	if pullSecretRef != nil && *pullSecretRef != "" {
		pullSecret = &corev1.Secret{}
		if err := l.fetchers.client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      *pullSecretRef,
		}, pullSecret); err != nil {
			return nil, fmt.Errorf("failed to get pull secret %s: %w", *pullSecretRef, err)
		}
	}

	// Handle ConfigMap fetcher with namespace
	if fetcherType == "configmap" && namespace != "" && !containsSlash(ref) {
		ref = namespace + "/" + ref
	}

	return l.fetchers.Fetch(ctx, fetcherType, ref, pullSecret)
}

// containsSlash checks if the string contains a forward slash
func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

// LoadFromContent loads a CUE module from raw content bytes
func (l *Loader) LoadFromContent(content []byte) (cue.Value, error) {
	value := l.ctx.CompileBytes(content)
	if value.Err() != nil {
		return cue.Value{}, fmt.Errorf("failed to compile CUE content: %w", value.Err())
	}
	return value, nil
}

// LoadEmbedded loads an embedded CUE module by version
// For now, version is ignored and we load from the cue/platform directory
// TODO: In production, this should use go:embed with proper paths
func (l *Loader) LoadEmbedded(version string) (cue.Value, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("embedded:%s", version)
	if cached, found := l.cache.Get(cacheKey); found {
		return cached, nil
	}

	// Find the cue/platform directory relative to the current working directory
	// This works for both development and testing
	cuePath, err := l.findCuePlatformPath()
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to find CUE platform path: %w", err)
	}

	// Load from the webservice package
	webservicePath := filepath.Join(cuePath, "webservice")
	value, err := l.LoadFromPath(webservicePath)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to load embedded module: %w", err)
	}

	// Cache the loaded value
	l.cache.Set(cacheKey, value)

	return value, nil
}

// findCuePlatformPath locates the cue/platform directory
func (l *Loader) findCuePlatformPath() (string, error) {
	// Try current directory first
	if _, err := os.Stat("cue/platform"); err == nil {
		return "cue/platform", nil
	}

	// Try going up one level (for tests in pkg/platformloader)
	if _, err := os.Stat("../../cue/platform"); err == nil {
		return "../../cue/platform", nil
	}

	// Try going up two levels (for deeply nested tests)
	if _, err := os.Stat("../../../cue/platform"); err == nil {
		return "../../../cue/platform", nil
	}

	return "", fmt.Errorf("could not find cue/platform directory")
}

// LoadFromPath loads a CUE module from a filesystem path
// This is useful for development and testing
func (l *Loader) LoadFromPath(path string) (cue.Value, error) {
	// Use CUE's load package to load from filesystem
	buildInstances := load.Instances([]string{path}, nil)
	if len(buildInstances) == 0 {
		return cue.Value{}, fmt.Errorf("no CUE instances found at %s", path)
	}

	inst := buildInstances[0]
	if inst.Err != nil {
		return cue.Value{}, fmt.Errorf("failed to load CUE instance: %w", inst.Err)
	}

	value := l.ctx.BuildInstance(inst)
	if value.Err() != nil {
		return cue.Value{}, fmt.Errorf("failed to build CUE value: %w", value.Err())
	}

	return value, nil
}

// Context returns the CUE context used by this loader
func (l *Loader) Context() *cue.Context {
	return l.ctx
}

// GetFetcher returns a fetcher by type (for direct access if needed)
func (l *Loader) GetFetcher(fetcherType string) (Fetcher, error) {
	if l.fetchers == nil {
		return nil, fmt.Errorf("fetchers not initialized")
	}
	return l.fetchers.GetFetcher(fetcherType)
}
