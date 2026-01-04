package platformloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	corev1 "k8s.io/api/core/v1"
)

// EmbeddedFetcher handles CUE modules embedded/bundled with the operator
// In development, it reads from the filesystem
// In production, this would use go:embed with files copied during build
type EmbeddedFetcher struct {
	// searchPaths are paths to search for the cue/platform directory
	searchPaths []string
}

// NewEmbeddedFetcher creates a new embedded fetcher
func NewEmbeddedFetcher() *EmbeddedFetcher {
	return &EmbeddedFetcher{
		searchPaths: []string{
			"cue/platform",
			"../../cue/platform",
			"../../../cue/platform",
			"/app/cue/platform", // Common container path
		},
	}
}

// NewEmbeddedFetcherWithPaths creates an embedded fetcher with custom search paths
func NewEmbeddedFetcherWithPaths(paths []string) *EmbeddedFetcher {
	return &EmbeddedFetcher{
		searchPaths: paths,
	}
}

// Type returns the fetcher type
func (f *EmbeddedFetcher) Type() string {
	return "embedded"
}

// Fetch retrieves a CUE module from the embedded/bundled filesystem
// ref is the module name (e.g., "webservice", "database")
// pullSecret is ignored for embedded fetches
func (f *EmbeddedFetcher) Fetch(ctx context.Context, ref string, _ *corev1.Secret) (*FetchResult, error) {
	if ref == "" {
		return nil, fmt.Errorf("embedded module reference is empty")
	}

	// Find the platform directory
	basePath, err := f.findPlatformPath()
	if err != nil {
		return nil, fmt.Errorf("failed to find embedded modules: %w", err)
	}

	// Build the module path
	modulePath := filepath.Join(basePath, ref)

	// Check if the module exists
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("embedded module %s not found at %s", ref, modulePath)
	}

	// Read all .cue files from the module directory
	content, err := readCUEFilesFromDir(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded module %s: %w", ref, err)
	}

	// Compute a content-based digest
	digest := fmt.Sprintf("embedded:%s:%x", ref, xxhash.Sum64(content))

	return &FetchResult{
		Content: content,
		Digest:  digest,
		Source:  fmt.Sprintf("embedded://%s", ref),
	}, nil
}

// findPlatformPath locates the cue/platform directory
func (f *EmbeddedFetcher) findPlatformPath() (string, error) {
	for _, path := range f.searchPaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find cue/platform directory in any of: %v", f.searchPaths)
}

// readCUEFilesFromDir reads all .cue files from a directory
func readCUEFilesFromDir(dir string) ([]byte, error) {
	var content []byte

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (like .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .cue files
		if !strings.HasSuffix(d.Name(), ".cue") {
			return nil
		}

		// Read the file
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		if len(content) > 0 {
			content = append(content, '\n')
		}
		content = append(content, data...)

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("no .cue files found in %s", dir)
	}

	return content, nil
}

// ListEmbeddedModules returns a list of available embedded modules
func (f *EmbeddedFetcher) ListEmbeddedModules() ([]string, error) {
	basePath, err := f.findPlatformPath()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded modules: %w", err)
	}

	var modules []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			modules = append(modules, entry.Name())
		}
	}

	return modules, nil
}
