// Package cache provides an in-memory cache with TTL support.
package cache

import (
	"sync"
	"time"
)

// Entry represents a cached item with expiration.
type Entry struct {
	Value     any
	ExpiresAt time.Time
}

// Cache is a thread-safe in-memory cache with TTL.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
	ttl     time.Duration
}

// New creates a new cache with the specified TTL.
func New(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]Entry),
		ttl:     ttl,
	}
	// Start background cleanup
	go c.cleanup()
	return c
}

// Get retrieves a value from the cache.
// Returns the value and true if found and not expired, nil and false otherwise.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Value, true
}

// Set stores a value in the cache with the default TTL.
func (c *Cache) Set(key string, value any) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL stores a value in the cache with a custom TTL.
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = Entry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a value from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]Entry)
}

// Size returns the number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// cleanup periodically removes expired entries.
func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.removeExpired()
	}
}

// removeExpired removes all expired entries.
func (c *Cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}

// Stats returns cache statistics.
type Stats struct {
	Size       int
	HitCount   int64
	MissCount  int64
	HitRate    float64
}
