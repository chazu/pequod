package platformloader

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CUEContentKey is the default key for CUE content in a ConfigMap
	CUEContentKey = "module.cue"
)

// ConfigMapFetcher fetches CUE modules from Kubernetes ConfigMaps
type ConfigMapFetcher struct {
	client client.Client
}

// NewConfigMapFetcher creates a new ConfigMap fetcher
func NewConfigMapFetcher(k8sClient client.Client) *ConfigMapFetcher {
	return &ConfigMapFetcher{
		client: k8sClient,
	}
}

// Type returns the fetcher type
func (f *ConfigMapFetcher) Type() string {
	return "configmap"
}

// Fetch retrieves a CUE module from a ConfigMap
// ref format: configmap-name or namespace/configmap-name
// pullSecret is ignored for ConfigMap fetches (used only for API consistency)
func (f *ConfigMapFetcher) Fetch(ctx context.Context, ref string, _ *corev1.Secret) (*FetchResult, error) {
	// Parse the reference
	namespace, name, err := parseConfigMapRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid ConfigMap reference: %w", err)
	}

	// Fetch the ConfigMap
	cm := &corev1.ConfigMap{}
	if err := f.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, cm); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, name, err)
	}

	// Extract CUE content from the ConfigMap
	content, err := extractCUEFromConfigMap(cm)
	if err != nil {
		return nil, fmt.Errorf("failed to extract CUE from ConfigMap: %w", err)
	}

	// Use resourceVersion as the digest for change detection
	digest := string(cm.UID) + ":" + cm.ResourceVersion

	return &FetchResult{
		Content: content,
		Digest:  digest,
		Source:  fmt.Sprintf("configmap://%s/%s", namespace, name),
	}, nil
}

// parseConfigMapRef parses a ConfigMap reference
// Supports formats:
//   - name (uses default namespace from context)
//   - namespace/name
func parseConfigMapRef(ref string) (namespace, name string, err error) {
	parts := strings.SplitN(ref, "/", 2)

	if len(parts) == 1 {
		// Just name, namespace must be set from context
		return "", parts[0], nil
	}

	return parts[0], parts[1], nil
}

// extractCUEFromConfigMap extracts CUE content from a ConfigMap
// It looks for:
// 1. A key named "module.cue"
// 2. Any keys ending in ".cue"
// 3. Falls back to concatenating all data keys
func extractCUEFromConfigMap(cm *corev1.ConfigMap) ([]byte, error) {
	if len(cm.Data) == 0 {
		return nil, fmt.Errorf("ConfigMap has no data")
	}

	// First, look for the default key
	if content, ok := cm.Data[CUEContentKey]; ok {
		return []byte(content), nil
	}

	// Look for any .cue files
	var cueFiles []string
	for key := range cm.Data {
		if strings.HasSuffix(key, ".cue") {
			cueFiles = append(cueFiles, key)
		}
	}

	if len(cueFiles) > 0 {
		// Sort for deterministic ordering
		sort.Strings(cueFiles)

		var content []byte
		for _, key := range cueFiles {
			if len(content) > 0 {
				content = append(content, '\n')
			}
			content = append(content, []byte(cm.Data[key])...)
		}
		return content, nil
	}

	// Fall back to concatenating all data
	// This allows simple ConfigMaps with a single key containing CUE
	var keys []string
	for key := range cm.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var content []byte
	for _, key := range keys {
		if len(content) > 0 {
			content = append(content, '\n')
		}
		content = append(content, []byte(cm.Data[key])...)
	}

	return content, nil
}

// ConfigMapFetcherWithNamespace wraps ConfigMapFetcher with a default namespace
type ConfigMapFetcherWithNamespace struct {
	*ConfigMapFetcher
	namespace string
}

// NewConfigMapFetcherWithNamespace creates a ConfigMap fetcher with a default namespace
func NewConfigMapFetcherWithNamespace(k8sClient client.Client, namespace string) *ConfigMapFetcherWithNamespace {
	return &ConfigMapFetcherWithNamespace{
		ConfigMapFetcher: NewConfigMapFetcher(k8sClient),
		namespace:        namespace,
	}
}

// Fetch retrieves a CUE module from a ConfigMap, using the default namespace if not specified
func (f *ConfigMapFetcherWithNamespace) Fetch(ctx context.Context, ref string, pullSecret *corev1.Secret) (*FetchResult, error) {
	// Parse the reference
	namespace, name, err := parseConfigMapRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid ConfigMap reference: %w", err)
	}

	// Use default namespace if not specified
	if namespace == "" {
		namespace = f.namespace
	}

	// Rebuild the reference with namespace
	fullRef := namespace + "/" + name

	return f.ConfigMapFetcher.Fetch(ctx, fullRef, pullSecret)
}
