package platformloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskCache_SetGet(t *testing.T) {
	// Create a temporary directory for the cache
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	// Test Set and Get
	key := "test-key"
	content := []byte("test content")

	if err := cache.Set(key, content); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("Get returned wrong content: got %q, want %q", got, content)
	}
}

func TestDiskCache_Miss(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	_, err = cache.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent key, got nil")
	}
}

func TestDiskCache_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	key := "test-key"
	content := []byte("test content")

	cache.Set(key, content)
	cache.Delete(key)

	_, err = cache.Get(key)
	if err == nil {
		t.Error("Expected error after delete, got nil")
	}
}

func TestDiskCache_LRUEviction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache with max 3 entries
	cache := NewDiskCacheWithConfig(tmpDir, 3, 24*time.Hour)

	// Add 4 entries
	cache.Set("key1", []byte("content1"))
	cache.Set("key2", []byte("content2"))
	cache.Set("key3", []byte("content3"))

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add key4, should evict key2 (least recently used)
	cache.Set("key4", []byte("content4"))

	// key2 should be evicted
	if _, err := cache.Get("key2"); err == nil {
		t.Error("key2 should have been evicted")
	}

	// key1 should still exist (was accessed recently)
	if _, err := cache.Get("key1"); err != nil {
		t.Errorf("key1 should still exist: %v", err)
	}
}

func TestDiskCache_TTLExpiration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache with very short TTL
	cache := NewDiskCacheWithConfig(tmpDir, 100, 1*time.Millisecond)

	cache.Set("key1", []byte("content1"))

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Entry should be expired
	_, err = cache.Get("key1")
	if err == nil {
		t.Error("Expected error for expired entry, got nil")
	}
}

func TestDiskCache_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache and add entries
	cache1 := NewDiskCache(tmpDir)
	cache1.Set("key1", []byte("content1"))
	cache1.Set("key2", []byte("content2"))

	// Create a new cache instance pointing to the same directory
	cache2 := NewDiskCache(tmpDir)

	// Entries should be loaded from disk
	got, err := cache2.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get key1 from second cache: %v", err)
	}

	if string(got) != "content1" {
		t.Errorf("Got wrong content: %q", got)
	}
}

func TestDiskCache_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	cache.Set("key1", []byte("content1"))
	cache.Set("key2", []byte("content2"))

	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if cache.Size() != 0 {
		t.Errorf("Cache size should be 0 after clear, got %d", cache.Size())
	}
}

func TestDiskCache_Stats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache := NewDiskCache(tmpDir)

	cache.Set("key1", []byte("content1"))
	cache.Set("key2", []byte("content2"))

	stats := cache.Stats()

	if stats.EntryCount != 2 {
		t.Errorf("Expected 2 entries, got %d", stats.EntryCount)
	}

	if stats.TotalSize != int64(len("content1")+len("content2")) {
		t.Errorf("Expected total size %d, got %d", len("content1")+len("content2"), stats.TotalSize)
	}
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"with.dot", "with.dot"},
		{"sha256:abc123", "sha256_abc123"},
		{"registry/repo:tag", "registry_repo_tag"},
		{"spaces and stuff", "spaces_and_stuff"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeKey(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeKey(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDiskCache_Prune(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskcache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache with short TTL
	cache := NewDiskCacheWithConfig(tmpDir, 100, 1*time.Millisecond)

	cache.Set("key1", []byte("content1"))
	cache.Set("key2", []byte("content2"))

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Prune expired entries
	if err := cache.Prune(); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	// Cache should be empty
	if cache.Size() != 0 {
		t.Errorf("Expected 0 entries after prune, got %d", cache.Size())
	}

	// Files should be removed
	files, _ := filepath.Glob(filepath.Join(tmpDir, "*.cue"))
	if len(files) > 1 { // metadata file may exist
		t.Errorf("Expected files to be removed, found %d", len(files))
	}
}
