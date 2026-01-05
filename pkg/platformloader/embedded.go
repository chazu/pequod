package platformloader

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/cespare/xxhash/v2"
	corev1 "k8s.io/api/core/v1"
)

// EmbeddedFetcher handles CUE modules embedded with the operator using go:embed.
// It reads from an fs.FS interface, which can be either an embed.FS for production
// or an os.DirFS/fstest.MapFS for testing.
type EmbeddedFetcher struct {
	// fs is the filesystem containing embedded modules
	fs fs.FS
	// rootDir is the directory within fs that contains the platform modules
	rootDir string
}

// NewEmbeddedFetcher creates a new embedded fetcher with the given filesystem.
// fs should be an embed.FS or compatible fs.FS implementation.
// rootDir is the directory within fs containing platform modules (e.g., "platform").
func NewEmbeddedFetcher(filesystem fs.FS, rootDir string) *EmbeddedFetcher {
	return &EmbeddedFetcher{
		fs:      filesystem,
		rootDir: rootDir,
	}
}

// Type returns the fetcher type
func (f *EmbeddedFetcher) Type() string {
	return "embedded"
}

// Fetch retrieves a CUE module from the embedded filesystem.
// ref is the module name (e.g., "webservice", "database")
// pullSecret is ignored for embedded fetches
func (f *EmbeddedFetcher) Fetch(ctx context.Context, ref string, _ *corev1.Secret) (*FetchResult, error) {
	if ref == "" {
		return nil, fmt.Errorf("embedded module reference is empty")
	}

	if f.fs == nil {
		return nil, fmt.Errorf("embedded filesystem not initialized")
	}

	// Build the module path within the embedded filesystem
	modulePath := f.rootDir + "/" + ref

	// Check if the module directory exists
	entries, err := fs.ReadDir(f.fs, modulePath)
	if err != nil {
		return nil, fmt.Errorf("embedded module %q not found: %w", ref, err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("embedded module %q is empty", ref)
	}

	// Read all .cue files from the module directory (recursively)
	content, err := f.readCUEFiles(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded module %q: %w", ref, err)
	}

	// Compute a content-based digest
	digest := fmt.Sprintf("embedded:%s:%x", ref, xxhash.Sum64(content))

	return &FetchResult{
		Content: content,
		Digest:  digest,
		Source:  fmt.Sprintf("embedded://%s", ref),
	}, nil
}

// readCUEFiles reads all .cue files from the given directory recursively
// It merges them into a single CUE source, handling package declarations
func (f *EmbeddedFetcher) readCUEFiles(dir string) ([]byte, error) {
	var files [][]byte
	var packageName string

	err := fs.WalkDir(f.fs, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (like .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
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
		data, err := fs.ReadFile(f.fs, path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		files = append(files, data)
		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .cue files found in %s", dir)
	}

	// Merge files, handling package declarations
	// CUE allows one package declaration per compilation unit
	// We extract and remove package declarations from all files, then add one at the top
	var mergedContent []byte
	for _, fileData := range files {
		// Extract package name if present, and remove the package line
		cleaned, pkg := stripPackageDeclaration(fileData)
		if pkg != "" && packageName == "" {
			packageName = pkg
		}
		if len(mergedContent) > 0 {
			mergedContent = append(mergedContent, '\n')
		}
		mergedContent = append(mergedContent, cleaned...)
	}

	// Prepend the package declaration if found
	if packageName != "" {
		header := []byte("package " + packageName + "\n\n")
		mergedContent = append(header, mergedContent...)
	}

	return mergedContent, nil
}

// stripPackageDeclaration removes the package declaration from CUE content
// and returns the cleaned content and the package name (if found)
func stripPackageDeclaration(content []byte) ([]byte, string) {
	lines := strings.Split(string(content), "\n")
	var result []string
	var packageName string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			// Extract package name
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				packageName = parts[1]
			}
			// Skip this line (don't add to result)
			continue
		}
		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n")), packageName
}

// ListEmbeddedModules returns a list of available embedded modules
func (f *EmbeddedFetcher) ListEmbeddedModules() ([]string, error) {
	if f.fs == nil {
		return nil, fmt.Errorf("embedded filesystem not initialized")
	}

	entries, err := fs.ReadDir(f.fs, f.rootDir)
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
