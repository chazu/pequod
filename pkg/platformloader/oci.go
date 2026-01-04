package platformloader

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Official CUE module format (v0.15+)
	// See: https://pkg.go.dev/cuelang.org/go/mod/modregistry

	// CUEModuleArtifactType is the artifact type for CUE modules
	CUEModuleArtifactType = "application/vnd.cue.module.v1+json"

	// CUEModuleZipMediaType is the media type for CUE module ZIP archives
	CUEModuleZipMediaType = "application/zip"

	// CUEModuleFileMediaType is the media type for the module.cue file
	CUEModuleFileMediaType = "application/vnd.cue.modulefile.v1"

	// CUEModuleAnnotation is the annotation key for module path@version
	CUEModuleAnnotation = "works.cue.module"

	// Legacy formats (for backwards compatibility)

	// CUEModuleMediaType is the legacy media type for CUE module layers (tar+gzip)
	CUEModuleMediaType = "application/vnd.cue.module.layer.v1+tar+gzip"

	// FallbackLayerMediaType is used when CUE-specific media type is not found
	FallbackLayerMediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
)

// OCIFetcher fetches CUE modules from OCI registries
type OCIFetcher struct {
	cache  *DiskCache
	client *http.Client
}

// NewOCIFetcher creates a new OCI fetcher
func NewOCIFetcher(cache *DiskCache) *OCIFetcher {
	return &OCIFetcher{
		cache:  cache,
		client: &http.Client{},
	}
}

// Type returns the fetcher type
func (f *OCIFetcher) Type() string {
	return "oci"
}

// Fetch retrieves a CUE module from an OCI registry
// ref format: registry/repo:tag or registry/repo@sha256:...
func (f *OCIFetcher) Fetch(ctx context.Context, ref string, pullSecret *corev1.Secret) (*FetchResult, error) {
	// Parse the OCI reference
	registry, repo, tag, dgst, err := parseOCIRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI reference: %w", err)
	}

	// Build auth header if pull secret provided
	authHeader := ""
	if pullSecret != nil {
		authHeader, err = getAuthHeader(pullSecret, registry)
		if err != nil {
			return nil, fmt.Errorf("failed to get auth header: %w", err)
		}
	}

	// If we have a digest, check cache first
	if dgst != "" {
		if cached, err := f.cache.Get(dgst); err == nil {
			return &FetchResult{
				Content: cached,
				Digest:  dgst,
				Source:  fmt.Sprintf("oci://%s (cached)", ref),
			}, nil
		}
	}

	// Resolve tag to digest if needed
	manifestDigest := dgst
	if manifestDigest == "" {
		manifestDigest, err = f.resolveTag(ctx, registry, repo, tag, authHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tag: %w", err)
		}

		// Check cache with resolved digest
		if cached, err := f.cache.Get(manifestDigest); err == nil {
			return &FetchResult{
				Content: cached,
				Digest:  manifestDigest,
				Source:  fmt.Sprintf("oci://%s (cached)", ref),
			}, nil
		}
	}

	// Fetch the manifest
	manifest, err := f.fetchManifest(ctx, registry, repo, manifestDigest, authHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Find the CUE module layer
	layer, err := findCUELayer(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to find CUE layer: %w", err)
	}

	// Fetch and extract the layer (supports both ZIP and tar.gz formats)
	content, err := f.fetchAndExtractLayer(ctx, registry, repo, layer, authHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch layer: %w", err)
	}

	// Cache the content
	if err := f.cache.Set(manifestDigest, content); err != nil {
		// Log but don't fail
		fmt.Printf("warning: failed to cache OCI module: %v\n", err)
	}

	return &FetchResult{
		Content: content,
		Digest:  manifestDigest,
		Source:  fmt.Sprintf("oci://%s", ref),
	}, nil
}

// parseOCIRef parses an OCI reference into its components
// Supports formats:
//   - registry/repo:tag
//   - registry/repo@sha256:...
//   - registry:port/repo:tag
func parseOCIRef(ref string) (registry, repo, tag, digest string, err error) {
	// Handle digest reference
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		digest = ref[idx+1:]
		ref = ref[:idx]
	}

	// Handle tag
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Check if this is a port or tag
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			// This is a tag (or digest already handled)
			if digest == "" {
				tag = afterColon
			}
			ref = ref[:idx]
		}
	}

	// Default tag
	if tag == "" && digest == "" {
		tag = "latest"
	}

	// Split registry and repo
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return "", "", "", "", fmt.Errorf("invalid OCI reference format: %s", ref)
	}

	// First part is registry if it contains a dot, colon, or is "localhost"
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		registry = parts[0]
		repo = parts[1]
	} else {
		// Default to docker.io
		registry = "registry-1.docker.io"
		repo = ref
		// Docker Hub requires library/ prefix for official images
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	}

	return registry, repo, tag, digest, nil
}

// getAuthHeader builds an authorization header from a Kubernetes secret
func getAuthHeader(secret *corev1.Secret, registry string) (string, error) {
	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		// Parse .dockerconfigjson
		configJSON, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			return "", fmt.Errorf("secret missing %s key", corev1.DockerConfigJsonKey)
		}

		var config struct {
			Auths map[string]struct {
				Auth string `json:"auth"`
			} `json:"auths"`
		}
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return "", fmt.Errorf("failed to parse docker config: %w", err)
		}

		// Look for matching registry
		for reg, auth := range config.Auths {
			if strings.Contains(reg, registry) || strings.Contains(registry, reg) {
				return "Basic " + auth.Auth, nil
			}
		}

		// Try without https://
		for reg, auth := range config.Auths {
			cleanReg := strings.TrimPrefix(strings.TrimPrefix(reg, "https://"), "http://")
			if cleanReg == registry {
				return "Basic " + auth.Auth, nil
			}
		}

		return "", fmt.Errorf("no auth found for registry %s", registry)

	case corev1.SecretTypeBasicAuth:
		username := string(secret.Data[corev1.BasicAuthUsernameKey])
		password := string(secret.Data[corev1.BasicAuthPasswordKey])
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		return "Basic " + auth, nil

	default:
		// Try to use username/password if present
		if username, ok := secret.Data["username"]; ok {
			password := secret.Data["password"]
			auth := base64.StdEncoding.EncodeToString([]byte(string(username) + ":" + string(password)))
			return "Basic " + auth, nil
		}
		return "", fmt.Errorf("unsupported secret type: %s", secret.Type)
	}
}

// resolveTag resolves a tag to a digest
func (f *OCIFetcher) resolveTag(ctx context.Context, registry, repo, tag, authHeader string) (string, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to resolve tag: status %d", resp.StatusCode)
	}

	dgst := resp.Header.Get("Docker-Content-Digest")
	if dgst == "" {
		return "", fmt.Errorf("no digest in response headers")
	}

	return dgst, nil
}

// fetchManifest fetches and parses an OCI manifest
func (f *OCIFetcher) fetchManifest(ctx context.Context, registry, repo, dgst, authHeader string) (*ocispec.Manifest, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, dgst)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch manifest: status %d, body: %s", resp.StatusCode, string(body))
	}

	var manifest ocispec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &manifest, nil
}

// layerFormat indicates the archive format of a CUE module layer
type layerFormat int

const (
	formatUnknown layerFormat = iota
	formatZip                 // Official CUE v0.15+ format (application/zip)
	formatTarGzip             // Legacy format (application/vnd.oci.image.layer.v1.tar+gzip)
)

// layerInfo contains information about a CUE module layer
type layerInfo struct {
	Digest string
	Format layerFormat
}

// findCUELayer finds the CUE module layer in the manifest
// Returns layer digest and format type for proper extraction
func findCUELayer(manifest *ocispec.Manifest) (*layerInfo, error) {
	// Priority 1: Official CUE module ZIP format (v0.15+)
	for _, layer := range manifest.Layers {
		if layer.MediaType == CUEModuleZipMediaType {
			return &layerInfo{
				Digest: layer.Digest.String(),
				Format: formatZip,
			}, nil
		}
	}

	// Priority 2: Legacy CUE-specific tar+gzip media type
	for _, layer := range manifest.Layers {
		if layer.MediaType == CUEModuleMediaType {
			return &layerInfo{
				Digest: layer.Digest.String(),
				Format: formatTarGzip,
			}, nil
		}
	}

	// Priority 3: Generic OCI tar+gzip layer
	for _, layer := range manifest.Layers {
		if layer.MediaType == FallbackLayerMediaType {
			return &layerInfo{
				Digest: layer.Digest.String(),
				Format: formatTarGzip,
			}, nil
		}
	}

	// Priority 4: Take the first layer and try to detect format
	if len(manifest.Layers) > 0 {
		layer := manifest.Layers[0]
		format := formatTarGzip // Default to tar.gz
		if layer.MediaType == CUEModuleZipMediaType || strings.HasSuffix(layer.MediaType, "zip") {
			format = formatZip
		}
		return &layerInfo{
			Digest: layer.Digest.String(),
			Format: format,
		}, nil
	}

	return nil, fmt.Errorf("no suitable layer found in manifest")
}

// fetchAndExtractLayer fetches a layer and extracts its content
// Supports both ZIP (official CUE v0.15+) and tar.gz (legacy) formats
func (f *OCIFetcher) fetchAndExtractLayer(ctx context.Context, registry string, repo string, layer *layerInfo, authHeader string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repo, layer.Digest)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch layer: status %d", resp.StatusCode)
	}

	// Read entire body for digest verification and format-specific extraction
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer body: %w", err)
	}

	// Verify digest
	verifier := digest.Digest(layer.Digest).Verifier()
	verifier.Write(body)
	if !verifier.Verified() {
		return nil, fmt.Errorf("layer digest verification failed")
	}

	// Extract based on format
	switch layer.Format {
	case formatZip:
		return f.extractZipContent(body)
	case formatTarGzip:
		return f.extractTarGzipContent(body)
	default:
		// Try ZIP first (official format), fall back to tar.gz
		if content, err := f.extractZipContent(body); err == nil {
			return content, nil
		}
		return f.extractTarGzipContent(body)
	}
}

// extractZipContent extracts .cue files from a ZIP archive
// This is the official CUE module format (v0.15+)
func (f *OCIFetcher) extractZipContent(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP archive: %w", err)
	}

	var content []byte
	for _, file := range zipReader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Read .cue files
		if strings.HasSuffix(file.Name, ".cue") {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open %s: %w", file.Name, err)
			}

			fileData, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", file.Name, err)
			}

			// Add a newline separator between files
			if len(content) > 0 {
				content = append(content, '\n')
			}
			content = append(content, fileData...)
		}
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("no .cue files found in ZIP archive")
	}

	return content, nil
}

// extractTarGzipContent extracts .cue files from a tar.gz archive
// This is the legacy format for backwards compatibility
func (f *OCIFetcher) extractTarGzipContent(data []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	var content []byte
	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Read .cue files
		if strings.HasSuffix(header.Name, ".cue") {
			fileData, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", header.Name, err)
			}
			// Add a newline separator between files
			if len(content) > 0 {
				content = append(content, '\n')
			}
			content = append(content, fileData...)
		}
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("no .cue files found in tar.gz archive")
	}

	return content, nil
}
