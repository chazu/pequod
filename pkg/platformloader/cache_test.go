package platformloader

import (
	"sync"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestCache(t *testing.T) {
	cache := NewCache()
	ctx := cuecontext.New()

	// Test empty cache
	if cache.Size() != 0 {
		t.Errorf("expected empty cache, got size %d", cache.Size())
	}

	// Test Get on empty cache
	_, found := cache.Get("key1")
	if found {
		t.Error("expected not found on empty cache")
	}

	// Test Set and Get
	value1 := ctx.CompileString(`x: 1`)
	cache.Set("key1", value1)

	if cache.Size() != 1 {
		t.Errorf("expected cache size 1, got %d", cache.Size())
	}

	retrieved, found := cache.Get("key1")
	if !found {
		t.Error("expected to find key1")
	}

	// Verify the value is the same
	if retrieved.Err() != nil {
		t.Errorf("retrieved value has error: %v", retrieved.Err())
	}

	// Test overwrite
	value2 := ctx.CompileString(`x: 2`)
	cache.Set("key1", value2)

	if cache.Size() != 1 {
		t.Errorf("expected cache size 1 after overwrite, got %d", cache.Size())
	}

	// Test Delete
	cache.Delete("key1")
	if cache.Size() != 0 {
		t.Errorf("expected cache size 0 after delete, got %d", cache.Size())
	}

	_, found = cache.Get("key1")
	if found {
		t.Error("expected not found after delete")
	}

	// Test Clear
	cache.Set("key1", value1)
	cache.Set("key2", value2)
	if cache.Size() != 2 {
		t.Errorf("expected cache size 2, got %d", cache.Size())
	}

	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("expected cache size 0 after clear, got %d", cache.Size())
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache()
	ctx := cuecontext.New()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			value := ctx.CompileString(`x: 1`)
			cache.Set("key", value)
		}(i)
	}

	wg.Wait()

	// Verify cache has the key
	_, found := cache.Get("key")
	if !found {
		t.Error("expected to find key after concurrent writes")
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.Get("key")
		}()
	}

	wg.Wait()

	// Concurrent mixed operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				value := ctx.CompileString(`x: 1`)
				cache.Set("key", value)
			} else {
				_, _ = cache.Get("key")
			}
		}(i)
	}

	wg.Wait()
}
