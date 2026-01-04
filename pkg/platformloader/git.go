package platformloader

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	corev1 "k8s.io/api/core/v1"
)

// GitFetcher fetches CUE modules from Git repositories
type GitFetcher struct {
	cache   *DiskCache
	tempDir string
}

// NewGitFetcher creates a new Git fetcher
func NewGitFetcher(cache *DiskCache) *GitFetcher {
	return &GitFetcher{
		cache:   cache,
		tempDir: os.TempDir(),
	}
}

// Type returns the fetcher type
func (f *GitFetcher) Type() string {
	return "git"
}

// GitRef contains parsed Git reference information
type GitRef struct {
	URL  string
	Ref  string // branch, tag, or commit SHA
	Path string // path within the repository
}

// Fetch retrieves a CUE module from a Git repository
// ref format: https://github.com/org/repo.git?ref=v1.0.0&path=modules/webservice
func (f *GitFetcher) Fetch(ctx context.Context, ref string, pullSecret *corev1.Secret) (*FetchResult, error) {
	// Parse the Git reference
	gitRef, err := parseGitRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid Git reference: %w", err)
	}

	// Build authentication
	auth, err := getGitAuth(pullSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Git auth: %w", err)
	}

	// Clone to a temporary directory
	tmpDir, err := os.MkdirTemp(f.tempDir, "pequod-git-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone options
	cloneOpts := &git.CloneOptions{
		URL:      gitRef.URL,
		Auth:     auth,
		Depth:    1, // Shallow clone for speed
		Progress: io.Discard,
	}

	// If ref is specified, try to set it as reference
	if gitRef.Ref != "" {
		// Check if it looks like a SHA
		if len(gitRef.Ref) == 40 || len(gitRef.Ref) == 7 {
			// Full clone needed for specific commit
			cloneOpts.Depth = 0
		} else {
			// It's a branch or tag
			cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(gitRef.Ref)
			cloneOpts.SingleBranch = true
		}
	}

	// Clone the repository
	repo, err := git.PlainCloneContext(ctx, tmpDir, false, cloneOpts)
	if err != nil {
		// Try as tag if branch clone failed
		if gitRef.Ref != "" && !strings.HasPrefix(err.Error(), "couldn't find") {
			cloneOpts.ReferenceName = plumbing.NewTagReferenceName(gitRef.Ref)
			repo, err = git.PlainCloneContext(ctx, tmpDir, false, cloneOpts)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	// If we have a specific commit SHA, checkout that commit
	if gitRef.Ref != "" && (len(gitRef.Ref) == 40 || len(gitRef.Ref) == 7) {
		worktree, err := repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("failed to get worktree: %w", err)
		}

		hash, err := repo.ResolveRevision(plumbing.Revision(gitRef.Ref))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve revision %s: %w", gitRef.Ref, err)
		}

		if err := worktree.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
			return nil, fmt.Errorf("failed to checkout commit: %w", err)
		}
	}

	// Get the HEAD commit for the digest
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}
	commitSHA := head.Hash().String()

	// Check cache with commit SHA
	cacheKey := fmt.Sprintf("git:%s:%s", gitRef.URL, commitSHA)
	if gitRef.Path != "" {
		cacheKey += ":" + gitRef.Path
	}

	if cached, err := f.cache.Get(cacheKey); err == nil {
		return &FetchResult{
			Content: cached,
			Digest:  commitSHA,
			Source:  fmt.Sprintf("git://%s (cached)", ref),
		}, nil
	}

	// Read the CUE files from the path
	modulePath := tmpDir
	if gitRef.Path != "" {
		modulePath = filepath.Join(tmpDir, gitRef.Path)
	}

	content, err := readCUEFiles(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CUE files: %w", err)
	}

	// Cache the content
	if err := f.cache.Set(cacheKey, content); err != nil {
		fmt.Printf("warning: failed to cache Git module: %v\n", err)
	}

	return &FetchResult{
		Content: content,
		Digest:  commitSHA,
		Source:  fmt.Sprintf("git://%s@%s", gitRef.URL, commitSHA[:7]),
	}, nil
}

// parseGitRef parses a Git reference string
// Format: https://github.com/org/repo.git?ref=v1.0.0&path=modules/webservice
func parseGitRef(ref string) (*GitRef, error) {
	// Parse as URL
	u, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Extract query parameters
	query := u.Query()
	gitRefValue := query.Get("ref")
	path := query.Get("path")

	// Remove query parameters to get clean URL
	u.RawQuery = ""
	cleanURL := u.String()

	// If no .git suffix and not a file:// URL, add it
	if !strings.HasSuffix(cleanURL, ".git") && u.Scheme != "file" {
		cleanURL += ".git"
	}

	return &GitRef{
		URL:  cleanURL,
		Ref:  gitRefValue,
		Path: path,
	}, nil
}

// getGitAuth builds Git authentication from a Kubernetes secret
func getGitAuth(secret *corev1.Secret) (transport.AuthMethod, error) {
	if secret == nil {
		return nil, nil
	}

	switch secret.Type {
	case corev1.SecretTypeSSHAuth:
		// SSH key authentication
		privateKey := secret.Data[corev1.SSHAuthPrivateKey]
		if len(privateKey) == 0 {
			return nil, fmt.Errorf("SSH secret missing private key")
		}

		// Get optional passphrase
		passphrase := string(secret.Data["passphrase"])

		publicKeys, err := ssh.NewPublicKeys("git", privateKey, passphrase)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %w", err)
		}

		return publicKeys, nil

	case corev1.SecretTypeBasicAuth:
		// Basic auth (token or username/password)
		username := string(secret.Data[corev1.BasicAuthUsernameKey])
		password := string(secret.Data[corev1.BasicAuthPasswordKey])

		return &http.BasicAuth{
			Username: username,
			Password: password,
		}, nil

	default:
		// Try to extract username/password or token
		if token, ok := secret.Data["token"]; ok {
			return &http.BasicAuth{
				Username: "x-access-token", // Works for GitHub, GitLab
				Password: string(token),
			}, nil
		}

		if username, ok := secret.Data["username"]; ok {
			password := secret.Data["password"]
			return &http.BasicAuth{
				Username: string(username),
				Password: string(password),
			}, nil
		}

		return nil, fmt.Errorf("unsupported secret type for Git auth: %s", secret.Type)
	}
}

// readCUEFiles reads all .cue files from a directory
func readCUEFiles(dir string) ([]byte, error) {
	var content []byte

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (like .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Read .cue files
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".cue") {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", path, err)
			}

			if len(content) > 0 {
				content = append(content, '\n')
			}
			content = append(content, data...)
		}

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
