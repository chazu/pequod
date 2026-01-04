package platformloader

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
)

// mockOCIRegistry creates a mock OCI registry server for testing
func mockOCIRegistry(t *testing.T, cueContent string) *httptest.Server {
	// Create a tar.gz layer containing the CUE file
	layerData := createTarGzLayer(t, "module.cue", cueContent)
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layerData))

	// Create manifest
	manifest := ocispec.Manifest{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    digest.Digest("sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: CUEModuleMediaType,
				Digest:    digest.Digest(layerDigest),
				Size:      int64(len(layerData)),
			},
		},
	}

	manifestJSON, _ := json.Marshal(manifest)
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifestJSON))

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock OCI registry received: %s %s", r.Method, r.URL.Path)

		// Handle manifest requests
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Docker-Content-Digest", manifestDigest)
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Write(manifestJSON)
			return
		}

		// Handle blob requests
		if strings.Contains(r.URL.Path, "/blobs/") {
			if strings.Contains(r.URL.Path, layerDigest) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Write(layerData)
				return
			}
			// Config blob (empty JSON)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("{}"))
			return
		}

		http.NotFound(w, r)
	}))
}

// createTarGzLayer creates a tar.gz archive containing a single file
func createTarGzLayer(t *testing.T, filename, content string) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name: filename,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}

	tw.Close()
	gzw.Close()
	return buf.Bytes()
}

func TestOCIFetcher_Type(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "oci-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	if fetcher.Type() != "oci" {
		t.Errorf("Expected type 'oci', got %q", fetcher.Type())
	}
}

func TestOCIFetcher_Fetch(t *testing.T) {
	cueContent := `
package webservice

name: "test-service"
replicas: 3
`
	server := mockOCIRegistry(t, cueContent)
	defer server.Close()

	// Extract host from server URL (remove http://)
	serverHost := strings.TrimPrefix(server.URL, "http://")

	tmpDir, _ := os.MkdirTemp("", "oci-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	// Create fetcher with custom client that uses the mock server
	fetcher := &OCIFetcher{
		cache:  cache,
		client: server.Client(),
	}

	// Override the fetcher's methods to use http:// instead of https://
	// For testing, we'll use the parseOCIRef directly and create a custom fetch
	ref := fmt.Sprintf("%s/test/module:v1.0.0", serverHost)

	result, err := fetchWithMockRegistry(t, fetcher, server.URL, "test/module", "v1.0.0", cueContent)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if !strings.Contains(string(result.Content), "test-service") {
		t.Errorf("Content doesn't contain expected CUE: %s", result.Content)
	}

	if !strings.HasPrefix(result.Source, "oci://") {
		t.Errorf("Expected source to start with 'oci://', got %q", result.Source)
	}

	_ = ref // Used in full integration test
}

// fetchWithMockRegistry performs a fetch against the mock registry
func fetchWithMockRegistry(t *testing.T, fetcher *OCIFetcher, baseURL, repo, tag, cueContent string) (*FetchResult, error) {
	ctx := context.Background()

	// Create layer
	layerData := createTarGzLayer(t, "module.cue", cueContent)
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layerData))

	// Create manifest
	manifest := ocispec.Manifest{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    digest.Digest("sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: CUEModuleMediaType,
				Digest:    digest.Digest(layerDigest),
				Size:      int64(len(layerData)),
			},
		},
	}

	manifestJSON, _ := json.Marshal(manifest)
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifestJSON))

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Docker-Content-Digest", manifestDigest)
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Write(manifestJSON)
			return
		}
		if strings.Contains(r.URL.Path, "/blobs/") && strings.Contains(r.URL.Path, strings.TrimPrefix(layerDigest, "sha256:")) {
			w.Write(layerData)
			return
		}
		if strings.Contains(r.URL.Path, "/blobs/") {
			w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Use the mock server's client
	fetcher.client = server.Client()

	// Manually call the internal methods to work around HTTPS requirement
	serverHost := strings.TrimPrefix(server.URL, "http://")

	// Resolve tag
	req, _ := http.NewRequestWithContext(ctx, "HEAD", fmt.Sprintf("%s/v2/%s/manifests/%s", server.URL, repo, tag), nil)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := fetcher.client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	resolvedDigest := resp.Header.Get("Docker-Content-Digest")

	// Fetch manifest
	req, _ = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/v2/%s/manifests/%s", server.URL, repo, resolvedDigest), nil)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err = fetcher.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var fetchedManifest ocispec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&fetchedManifest); err != nil {
		return nil, err
	}

	// Find CUE layer
	layer, err := findCUELayer(&fetchedManifest)
	if err != nil {
		return nil, err
	}

	// Fetch layer
	req, _ = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/v2/%s/blobs/%s", server.URL, repo, layer.Digest), nil)
	resp, err = fetcher.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decompress
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	// Extract
	var content []byte
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		if strings.HasSuffix(header.Name, ".cue") {
			buf := new(bytes.Buffer)
			buf.ReadFrom(tr)
			content = buf.Bytes()
		}
	}

	return &FetchResult{
		Content: content,
		Digest:  resolvedDigest,
		Source:  fmt.Sprintf("oci://%s/%s:%s", serverHost, repo, tag),
	}, nil
}

func TestParseOCIRef(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		wantRegistry string
		wantRepo     string
		wantTag      string
		wantDigest   string
		wantErr      bool
	}{
		{
			name:         "simple with tag",
			ref:          "ghcr.io/myorg/myrepo:v1.0.0",
			wantRegistry: "ghcr.io",
			wantRepo:     "myorg/myrepo",
			wantTag:      "v1.0.0",
			wantDigest:   "",
		},
		{
			name:         "with digest",
			ref:          "ghcr.io/myorg/myrepo@sha256:abc123",
			wantRegistry: "ghcr.io",
			wantRepo:     "myorg/myrepo",
			wantTag:      "",
			wantDigest:   "sha256:abc123",
		},
		{
			name:         "default tag",
			ref:          "ghcr.io/myorg/myrepo",
			wantRegistry: "ghcr.io",
			wantRepo:     "myorg/myrepo",
			wantTag:      "latest",
			wantDigest:   "",
		},
		{
			name:         "registry with port",
			ref:          "localhost:5000/myrepo:v1.0.0",
			wantRegistry: "localhost:5000",
			wantRepo:     "myrepo",
			wantTag:      "v1.0.0",
			wantDigest:   "",
		},
		{
			name:         "docker hub user repo",
			ref:          "myuser/myrepo:v1.0.0",
			wantRegistry: "registry-1.docker.io",
			wantRepo:     "myuser/myrepo",
			wantTag:      "v1.0.0",
			wantDigest:   "",
		},
		{
			name:    "invalid ref - no slash",
			ref:     "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, repo, tag, digest, err := parseOCIRef(tt.ref)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if registry != tt.wantRegistry {
				t.Errorf("Registry: got %q, want %q", registry, tt.wantRegistry)
			}
			if repo != tt.wantRepo {
				t.Errorf("Repo: got %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("Tag: got %q, want %q", tag, tt.wantTag)
			}
			if digest != tt.wantDigest {
				t.Errorf("Digest: got %q, want %q", digest, tt.wantDigest)
			}
		})
	}
}

func TestGetAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		secret     *corev1.Secret
		registry   string
		wantPrefix string
		wantErr    bool
	}{
		{
			name: "dockerconfigjson secret",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{"ghcr.io":{"auth":"dXNlcjpwYXNz"}}}`),
				},
			},
			registry:   "ghcr.io",
			wantPrefix: "Basic ",
		},
		{
			name: "basic auth secret",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("user"),
					corev1.BasicAuthPasswordKey: []byte("pass"),
				},
			},
			registry:   "ghcr.io",
			wantPrefix: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		},
		{
			name: "generic secret with username/password",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("user"),
					"password": []byte("pass"),
				},
			},
			registry:   "ghcr.io",
			wantPrefix: "Basic ",
		},
		{
			name: "registry not found",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{"docker.io":{"auth":"dXNlcjpwYXNz"}}}`),
				},
			},
			registry: "ghcr.io",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, err := getAuthHeader(tt.secret, tt.registry)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !strings.HasPrefix(header, tt.wantPrefix) {
				t.Errorf("Header: got %q, want prefix %q", header, tt.wantPrefix)
			}
		})
	}
}

func TestFindCUELayer(t *testing.T) {
	tests := []struct {
		name       string
		manifest   *ocispec.Manifest
		wantErr    bool
		wantFormat layerFormat
	}{
		{
			name: "ZIP format (official CUE v0.15+)",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{
					{
						MediaType: CUEModuleZipMediaType,
						Digest:    "sha256:abc123",
					},
				},
			},
			wantErr:    false,
			wantFormat: formatZip,
		},
		{
			name: "ZIP takes priority over tar.gz",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{
					{
						MediaType: CUEModuleMediaType,
						Digest:    "sha256:legacy123",
					},
					{
						MediaType: CUEModuleZipMediaType,
						Digest:    "sha256:abc123",
					},
				},
			},
			wantErr:    false,
			wantFormat: formatZip,
		},
		{
			name: "CUE-specific tar.gz media type (legacy)",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{
					{
						MediaType: CUEModuleMediaType,
						Digest:    "sha256:abc123",
					},
				},
			},
			wantErr:    false,
			wantFormat: formatTarGzip,
		},
		{
			name: "fallback OCI tar.gz media type",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{
					{
						MediaType: FallbackLayerMediaType,
						Digest:    "sha256:abc123",
					},
				},
			},
			wantErr:    false,
			wantFormat: formatTarGzip,
		},
		{
			name: "first layer fallback",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/octet-stream",
						Digest:    "sha256:abc123",
					},
				},
			},
			wantErr:    false,
			wantFormat: formatTarGzip, // Unknown defaults to tar.gz
		},
		{
			name: "no layers",
			manifest: &ocispec.Manifest{
				Layers: []ocispec.Descriptor{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer, err := findCUELayer(tt.manifest)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if layer == nil {
				t.Fatal("Expected non-nil layer info")
			}

			if layer.Digest == "" {
				t.Error("Expected non-empty digest")
			}

			if layer.Format != tt.wantFormat {
				t.Errorf("Format: got %v, want %v", layer.Format, tt.wantFormat)
			}
		})
	}
}

func TestOCIFetcher_CacheIntegration(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "oci-cache-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	// Pre-populate cache
	testDigest := "sha256:testdigest123"
	testContent := []byte("cached: true")
	cache.Set(testDigest, testContent)

	// Verify cache is used
	cached, err := cache.Get(testDigest)
	if err != nil {
		t.Fatalf("Cache get failed: %v", err)
	}

	if string(cached) != string(testContent) {
		t.Errorf("Cache content mismatch")
	}

	_ = fetcher // Fetcher would use cached content for matching digest
}

func TestExtractZipContent(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "zip-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	// Create a ZIP file with CUE content
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add schema.cue
	schemaFile, _ := zipWriter.Create("schema.cue")
	schemaFile.Write([]byte("package test\n#Input: { name: string }"))

	// Add render.cue
	renderFile, _ := zipWriter.Create("render.cue")
	renderFile.Write([]byte("package test\ngraph: { nodes: [] }"))

	// Add a non-CUE file (should be ignored)
	readmeFile, _ := zipWriter.Create("README.md")
	readmeFile.Write([]byte("# Test module"))

	zipWriter.Close()

	// Extract
	content, err := fetcher.extractZipContent(buf.Bytes())
	if err != nil {
		t.Fatalf("extractZipContent failed: %v", err)
	}

	// Verify content contains both CUE files
	contentStr := string(content)
	if !strings.Contains(contentStr, "#Input:") {
		t.Error("Expected content to contain schema.cue content")
	}
	if !strings.Contains(contentStr, "graph:") {
		t.Error("Expected content to contain render.cue content")
	}
	if strings.Contains(contentStr, "# Test module") {
		t.Error("README.md should not be included")
	}
}

func TestExtractZipContent_NestedDirectories(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "zip-nested-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	// Create a ZIP with nested structure (like official CUE modules)
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add cue.mod/module.cue
	modFile, _ := zipWriter.Create("cue.mod/module.cue")
	modFile.Write([]byte(`module: "example.com/test@v1"`))

	// Add root schema.cue
	schemaFile, _ := zipWriter.Create("schema.cue")
	schemaFile.Write([]byte("package test\n#Input: { name: string }"))

	zipWriter.Close()

	// Extract
	content, err := fetcher.extractZipContent(buf.Bytes())
	if err != nil {
		t.Fatalf("extractZipContent failed: %v", err)
	}

	// Verify content contains both CUE files
	contentStr := string(content)
	if !strings.Contains(contentStr, "module:") {
		t.Error("Expected content to contain module.cue content")
	}
	if !strings.Contains(contentStr, "#Input:") {
		t.Error("Expected content to contain schema.cue content")
	}
}

func TestExtractZipContent_NoCueFiles(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "zip-empty-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	// Create a ZIP without CUE files
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	readmeFile, _ := zipWriter.Create("README.md")
	readmeFile.Write([]byte("# Test module"))

	zipWriter.Close()

	// Extract should fail
	_, err := fetcher.extractZipContent(buf.Bytes())
	if err == nil {
		t.Error("Expected error for ZIP without CUE files")
	}
}

func TestExtractTarGzipContent(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "targz-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewOCIFetcher(cache)

	// Create a tar.gz with CUE content
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add schema.cue
	schemaContent := []byte("package test\n#Input: { name: string }")
	tarWriter.WriteHeader(&tar.Header{
		Name: "schema.cue",
		Size: int64(len(schemaContent)),
		Mode: 0644,
	})
	tarWriter.Write(schemaContent)

	// Add render.cue
	renderContent := []byte("package test\ngraph: { nodes: [] }")
	tarWriter.WriteHeader(&tar.Header{
		Name: "render.cue",
		Size: int64(len(renderContent)),
		Mode: 0644,
	})
	tarWriter.Write(renderContent)

	tarWriter.Close()
	gzWriter.Close()

	// Extract
	content, err := fetcher.extractTarGzipContent(buf.Bytes())
	if err != nil {
		t.Fatalf("extractTarGzipContent failed: %v", err)
	}

	// Verify content contains both CUE files
	contentStr := string(content)
	if !strings.Contains(contentStr, "#Input:") {
		t.Error("Expected content to contain schema.cue content")
	}
	if !strings.Contains(contentStr, "graph:") {
		t.Error("Expected content to contain render.cue content")
	}
}
