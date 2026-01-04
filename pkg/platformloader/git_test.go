package platformloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestGitFetcher_Type(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "git-test-*")
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)
	fetcher := NewGitFetcher(cache)

	if fetcher.Type() != "git" {
		t.Errorf("Expected type 'git', got %q", fetcher.Type())
	}
}

func TestParseGitRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantURL  string
		wantRef  string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "simple URL",
			ref:      "https://github.com/myorg/myrepo",
			wantURL:  "https://github.com/myorg/myrepo.git",
			wantRef:  "",
			wantPath: "",
		},
		{
			name:     "URL with .git suffix",
			ref:      "https://github.com/myorg/myrepo.git",
			wantURL:  "https://github.com/myorg/myrepo.git",
			wantRef:  "",
			wantPath: "",
		},
		{
			name:     "URL with ref parameter",
			ref:      "https://github.com/myorg/myrepo?ref=v1.0.0",
			wantURL:  "https://github.com/myorg/myrepo.git",
			wantRef:  "v1.0.0",
			wantPath: "",
		},
		{
			name:     "URL with path parameter",
			ref:      "https://github.com/myorg/myrepo?path=modules/webservice",
			wantURL:  "https://github.com/myorg/myrepo.git",
			wantRef:  "",
			wantPath: "modules/webservice",
		},
		{
			name:     "URL with ref and path",
			ref:      "https://github.com/myorg/myrepo?ref=main&path=platforms/database",
			wantURL:  "https://github.com/myorg/myrepo.git",
			wantRef:  "main",
			wantPath: "platforms/database",
		},
		{
			name:     "GitLab URL",
			ref:      "https://gitlab.com/group/project.git?ref=develop",
			wantURL:  "https://gitlab.com/group/project.git",
			wantRef:  "develop",
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRef, err := parseGitRef(tt.ref)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if gitRef.URL != tt.wantURL {
				t.Errorf("URL: got %q, want %q", gitRef.URL, tt.wantURL)
			}
			if gitRef.Ref != tt.wantRef {
				t.Errorf("Ref: got %q, want %q", gitRef.Ref, tt.wantRef)
			}
			if gitRef.Path != tt.wantPath {
				t.Errorf("Path: got %q, want %q", gitRef.Path, tt.wantPath)
			}
		})
	}
}

func TestGetGitAuth(t *testing.T) {
	tests := []struct {
		name    string
		secret  *corev1.Secret
		wantNil bool
		wantErr bool
	}{
		{
			name:    "nil secret",
			secret:  nil,
			wantNil: true,
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
			wantNil: false,
		},
		{
			name: "generic secret with token",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"token": []byte("ghp_token123"),
				},
			},
			wantNil: false,
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
			wantNil: false,
		},
		{
			name: "unsupported secret type",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"other": []byte("value"),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := getGitAuth(tt.secret)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.wantNil && auth != nil {
				t.Error("Expected nil auth")
			}
			if !tt.wantNil && auth == nil {
				t.Error("Expected non-nil auth")
			}
		})
	}
}

func TestReadCUEFiles(t *testing.T) {
	// Create a temp directory with CUE files
	tmpDir, err := os.MkdirTemp("", "cue-read-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some CUE files
	os.WriteFile(filepath.Join(tmpDir, "schema.cue"), []byte("schema: true\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "render.cue"), []byte("render: true\n"), 0644)

	// Create a .git directory (should be skipped)
	gitDir := filepath.Join(tmpDir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "config.cue"), []byte("gitconfig: true\n"), 0644)

	// Create a non-CUE file (should be skipped)
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# readme"), 0644)

	content, err := readCUEFiles(tmpDir)
	if err != nil {
		t.Fatalf("readCUEFiles failed: %v", err)
	}

	// Should contain schema and render files
	if !strings.Contains(string(content), "schema:") {
		t.Error("Content should contain schema.cue")
	}
	if !strings.Contains(string(content), "render:") {
		t.Error("Content should contain render.cue")
	}

	// Should NOT contain .git files or README
	if strings.Contains(string(content), "gitconfig:") {
		t.Error("Content should not contain .git directory files")
	}
	if strings.Contains(string(content), "readme") {
		t.Error("Content should not contain non-CUE files")
	}
}

func TestReadCUEFiles_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cue-read-empty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = readCUEFiles(tmpDir)
	if err == nil {
		t.Error("Expected error for empty directory, got nil")
	}
}

func TestReadCUEFiles_SubDirectories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cue-read-subdir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested structure
	subDir := filepath.Join(tmpDir, "submodule")
	os.MkdirAll(subDir, 0755)

	os.WriteFile(filepath.Join(tmpDir, "main.cue"), []byte("main: true\n"), 0644)
	os.WriteFile(filepath.Join(subDir, "sub.cue"), []byte("sub: true\n"), 0644)

	content, err := readCUEFiles(tmpDir)
	if err != nil {
		t.Fatalf("readCUEFiles failed: %v", err)
	}

	// Should contain both files
	if !strings.Contains(string(content), "main:") {
		t.Error("Content should contain main.cue")
	}
	if !strings.Contains(string(content), "sub:") {
		t.Error("Content should contain sub.cue from subdirectory")
	}
}

// TestGitFetcher_FetchLocalRepo tests fetching from a local git repository
// This test creates a real local git repo to test the full flow
func TestGitFetcher_FetchLocalRepo(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping local repo test")
	}

	// Create a temp directory for the repo
	repoDir, err := os.MkdirTemp("", "git-repo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp repo dir: %v", err)
	}
	defer os.RemoveAll(repoDir)

	// Create a temp directory for the cache
	cacheDir, err := os.MkdirTemp("", "git-cache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = repoDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = repoDir
	cmd.Run()

	// Create CUE file
	cueContent := `package test

name: "local-test"
value: 42
`
	if err := os.WriteFile(filepath.Join(repoDir, "module.cue"), []byte(cueContent), 0644); err != nil {
		t.Fatalf("Failed to write CUE file: %v", err)
	}

	// Add and commit
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Create fetcher
	cache := NewDiskCache(cacheDir)
	fetcher := NewGitFetcher(cache)

	// Fetch from local repo
	ctx := context.Background()
	// Use file:// protocol with proper format for local paths
	localURL := fmt.Sprintf("file://%s", repoDir)
	result, err := fetcher.Fetch(ctx, localURL, nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// Verify content
	if !strings.Contains(string(result.Content), "local-test") {
		t.Errorf("Content doesn't contain expected CUE: %s", result.Content)
	}

	// Verify digest (should be a git commit SHA)
	if len(result.Digest) != 40 {
		t.Errorf("Expected 40-char commit SHA, got %q", result.Digest)
	}

	if !strings.HasPrefix(result.Source, "git://") {
		t.Errorf("Expected source to start with 'git://', got %q", result.Source)
	}
}

// TestGitFetcher_FetchWithPath tests fetching a specific path within a repo
func TestGitFetcher_FetchWithPath(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping path test")
	}

	// Create a temp directory for the repo
	repoDir, err := os.MkdirTemp("", "git-repo-path-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp repo dir: %v", err)
	}
	defer os.RemoveAll(repoDir)

	// Create a temp directory for the cache
	cacheDir, err := os.MkdirTemp("", "git-cache-path-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = repoDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = repoDir
	cmd.Run()

	// Create nested module structure
	modulesDir := filepath.Join(repoDir, "modules", "webservice")
	os.MkdirAll(modulesDir, 0755)

	cueContent := `package webservice

name: "nested-module"
`
	os.WriteFile(filepath.Join(modulesDir, "main.cue"), []byte(cueContent), 0644)

	// Also create a root-level file (should not be included)
	os.WriteFile(filepath.Join(repoDir, "root.cue"), []byte("root: true\n"), 0644)

	// Add and commit
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Add modules")
	cmd.Dir = repoDir
	cmd.Run()

	// Create fetcher
	cache := NewDiskCache(cacheDir)
	fetcher := NewGitFetcher(cache)

	// Fetch with path
	ctx := context.Background()
	ref := fmt.Sprintf("file://%s?path=modules/webservice", repoDir)
	result, err := fetcher.Fetch(ctx, ref, nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// Should contain nested module content
	if !strings.Contains(string(result.Content), "nested-module") {
		t.Errorf("Content doesn't contain nested module: %s", result.Content)
	}

	// Should NOT contain root-level file
	if strings.Contains(string(result.Content), "root:") {
		t.Errorf("Content should not contain root-level files")
	}
}

func TestGitFetcher_CacheHit(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "git-cache-hit-*")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	cache := NewDiskCache(cacheDir)

	// Pre-populate cache
	testKey := "git:file:///test/repo.git:abc123def456"
	testContent := []byte("cached: true")
	cache.Set(testKey, testContent)

	// Verify cache works
	cached, err := cache.Get(testKey)
	if err != nil {
		t.Fatalf("Cache get failed: %v", err)
	}

	if string(cached) != string(testContent) {
		t.Errorf("Cache content mismatch")
	}
}
