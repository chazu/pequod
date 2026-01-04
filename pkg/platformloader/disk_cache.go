package platformloader

import (
	"container/list"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultCacheDir is the default directory for the disk cache
	DefaultCacheDir = "/tmp/pequod-module-cache"

	// DefaultMaxEntries is the default maximum number of cached entries
	DefaultMaxEntries = 100

	// DefaultTTL is the default time-to-live for cached entries
	DefaultTTL = 24 * time.Hour

	// MetadataFile is the name of the cache metadata file
	MetadataFile = "cache.json"
)

// DiskCache provides persistent, LRU-evicting cache for CUE modules
type DiskCache struct {
	mu sync.RWMutex

	// dir is the cache directory
	dir string

	// maxEntries is the maximum number of entries
	maxEntries int

	// ttl is the time-to-live for entries
	ttl time.Duration

	// lru tracks access order for eviction
	lru *list.List

	// entries maps keys to list elements
	entries map[string]*list.Element

	// metadataPath is the path to the metadata file
	metadataPath string
}

// CacheEntry represents a cached module
type CacheEntry struct {
	Key       string    `json:"key"`
	Digest    string    `json:"digest"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	AccessedAt time.Time `json:"accessedAt"`
}

// CacheMetadata contains cache state persisted to disk
type CacheMetadata struct {
	Entries []CacheEntry `json:"entries"`
	Version string       `json:"version"`
}

// NewDiskCache creates a new disk cache
func NewDiskCache(dir string) *DiskCache {
	if dir == "" {
		dir = DefaultCacheDir
	}

	cache := &DiskCache{
		dir:          dir,
		maxEntries:   DefaultMaxEntries,
		ttl:          DefaultTTL,
		lru:          list.New(),
		entries:      make(map[string]*list.Element),
		metadataPath: filepath.Join(dir, MetadataFile),
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("warning: failed to create cache directory %s: %v\n", dir, err)
		return cache
	}

	// Load existing metadata
	if err := cache.loadMetadata(); err != nil {
		fmt.Printf("warning: failed to load cache metadata: %v\n", err)
	}

	return cache
}

// NewDiskCacheWithConfig creates a disk cache with custom configuration
func NewDiskCacheWithConfig(dir string, maxEntries int, ttl time.Duration) *DiskCache {
	cache := NewDiskCache(dir)
	cache.maxEntries = maxEntries
	cache.ttl = ttl
	return cache
}

// Get retrieves an entry from the cache
func (c *DiskCache) Get(key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		RecordCacheMiss()
		return nil, fmt.Errorf("cache miss: %s", key)
	}

	entry := elem.Value.(*CacheEntry)

	// Check TTL
	if time.Since(entry.CreatedAt) > c.ttl {
		// Entry expired, remove it
		c.removeEntry(key)
		return nil, fmt.Errorf("cache entry expired: %s", key)
	}

	// Read content from disk
	contentPath := c.contentPath(key)
	content, err := os.ReadFile(contentPath)
	if err != nil {
		// File missing, remove entry
		c.removeEntry(key)
		return nil, fmt.Errorf("cache file missing: %s", key)
	}

	// Update access time and move to front
	entry.AccessedAt = time.Now()
	c.lru.MoveToFront(elem)

	RecordCacheHit()
	return content, nil
}

// Set stores an entry in the cache
func (c *DiskCache) Set(key string, content []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if elem, ok := c.entries[key]; ok {
		// Update existing entry
		entry := elem.Value.(*CacheEntry)
		entry.AccessedAt = time.Now()
		entry.Size = int64(len(content))
		c.lru.MoveToFront(elem)
	} else {
		// Create new entry
		entry := &CacheEntry{
			Key:        key,
			Digest:     key, // The key is typically the digest
			Size:       int64(len(content)),
			CreatedAt:  time.Now(),
			AccessedAt: time.Now(),
		}

		// Evict if at capacity
		for c.lru.Len() >= c.maxEntries {
			c.evictOldest()
		}

		elem := c.lru.PushFront(entry)
		c.entries[key] = elem
	}

	// Write content to disk
	contentPath := c.contentPath(key)
	if err := os.WriteFile(contentPath, content, 0644); err != nil {
		c.removeEntry(key)
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Persist metadata
	if err := c.saveMetadata(); err != nil {
		fmt.Printf("warning: failed to save cache metadata: %v\n", err)
	}

	// Update cache stats metrics
	stats := c.statsLocked()
	UpdateCacheStats(stats.EntryCount, stats.TotalSize)

	return nil
}

// Delete removes an entry from the cache
func (c *DiskCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeEntry(key)

	// Persist metadata
	if err := c.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save cache metadata: %w", err)
	}

	// Update cache stats metrics
	stats := c.statsLocked()
	UpdateCacheStats(stats.EntryCount, stats.TotalSize)

	return nil
}

// Clear removes all entries from the cache
func (c *DiskCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove all files
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(c.dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			fmt.Printf("warning: failed to remove %s: %v\n", path, err)
		}
	}

	// Reset in-memory state
	c.lru = list.New()
	c.entries = make(map[string]*list.Element)

	// Update cache stats metrics (now empty)
	UpdateCacheStats(0, 0)

	return nil
}

// Size returns the number of entries in the cache
func (c *DiskCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// Stats returns cache statistics
func (c *DiskCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statsLocked()
}

// statsLocked returns cache statistics (must hold lock)
func (c *DiskCache) statsLocked() CacheStats {
	stats := CacheStats{
		EntryCount: c.lru.Len(),
		MaxEntries: c.maxEntries,
	}

	for elem := c.lru.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*CacheEntry)
		stats.TotalSize += entry.Size
	}

	return stats
}

// CacheStats contains cache statistics
type CacheStats struct {
	EntryCount int
	MaxEntries int
	TotalSize  int64
	Hits       int64
	Misses     int64
}

// contentPath returns the file path for cached content
func (c *DiskCache) contentPath(key string) string {
	// Sanitize the key to be filesystem-safe
	safeKey := sanitizeKey(key)
	return filepath.Join(c.dir, safeKey+".cue")
}

// sanitizeKey makes a key safe for use as a filename
func sanitizeKey(key string) string {
	// Replace unsafe characters
	safe := ""
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			safe += string(r)
		case r >= 'A' && r <= 'Z':
			safe += string(r)
		case r >= '0' && r <= '9':
			safe += string(r)
		case r == '-' || r == '_' || r == '.':
			safe += string(r)
		default:
			safe += "_"
		}
	}
	return safe
}

// removeEntry removes an entry from the cache (internal, must hold lock)
func (c *DiskCache) removeEntry(key string) {
	if elem, ok := c.entries[key]; ok {
		c.lru.Remove(elem)
		delete(c.entries, key)

		// Remove file
		contentPath := c.contentPath(key)
		os.Remove(contentPath)
	}
}

// evictOldest removes the least recently used entry
func (c *DiskCache) evictOldest() {
	elem := c.lru.Back()
	if elem != nil {
		entry := elem.Value.(*CacheEntry)
		c.removeEntry(entry.Key)
		RecordCacheEviction()
	}
}

// loadMetadata loads cache metadata from disk
func (c *DiskCache) loadMetadata() error {
	data, err := os.ReadFile(c.metadataPath)
	if os.IsNotExist(err) {
		return nil // No metadata file yet
	}
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata CacheMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Rebuild LRU from metadata
	now := time.Now()
	for i := len(metadata.Entries) - 1; i >= 0; i-- {
		entry := metadata.Entries[i]

		// Skip expired entries
		if now.Sub(entry.CreatedAt) > c.ttl {
			// Remove stale file
			os.Remove(c.contentPath(entry.Key))
			continue
		}

		// Verify file exists
		contentPath := c.contentPath(entry.Key)
		if _, err := os.Stat(contentPath); os.IsNotExist(err) {
			continue
		}

		// Add to LRU (most recently accessed first)
		entryCopy := entry
		elem := c.lru.PushFront(&entryCopy)
		c.entries[entry.Key] = elem
	}

	return nil
}

// saveMetadata persists cache metadata to disk
func (c *DiskCache) saveMetadata() error {
	entries := make([]CacheEntry, 0, c.lru.Len())

	// Collect entries in access order
	for elem := c.lru.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*CacheEntry)
		entries = append(entries, *entry)
	}

	metadata := CacheMetadata{
		Entries: entries,
		Version: "v1",
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write atomically
	tmpPath := c.metadataPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := os.Rename(tmpPath, c.metadataPath); err != nil {
		return fmt.Errorf("failed to rename metadata: %w", err)
	}

	return nil
}

// Prune removes expired entries from the cache
func (c *DiskCache) Prune() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for key, elem := range c.entries {
		entry := elem.Value.(*CacheEntry)
		if now.Sub(entry.CreatedAt) > c.ttl {
			toRemove = append(toRemove, key)
		}
	}

	for _, key := range toRemove {
		c.removeEntry(key)
	}

	// Persist metadata
	if len(toRemove) > 0 {
		if err := c.saveMetadata(); err != nil {
			return fmt.Errorf("failed to save cache metadata: %w", err)
		}
	}

	return nil
}
