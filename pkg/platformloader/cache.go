package platformloader

import (
	"sync"

	"cuelang.org/go/cue"
)

// Cache provides thread-safe caching of CUE values
type Cache struct {
	mu    sync.RWMutex
	items map[string]cue.Value
}

// NewCache creates a new cache instance
func NewCache() *Cache {
	return &Cache{
		items: make(map[string]cue.Value),
	}
}

// Get retrieves a value from the cache
// Returns the value and true if found, zero value and false otherwise
func (c *Cache) Get(key string) (cue.Value, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, found := c.items[key]
	return value, found
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value cue.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = value
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Clear removes all values from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]cue.Value)
}

// Size returns the number of items in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}
