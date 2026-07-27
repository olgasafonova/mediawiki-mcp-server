package wiki

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/metrics"
)

// WarmCache pre-loads commonly accessed pages into the cache.
// Call this after creating the client to improve first-request latency.
func (c *Client) WarmCache(ctx context.Context, titles []string) error {
	if len(titles) == 0 {
		return nil
	}

	c.logger.Info("Warming cache", "pages", len(titles))

	// Use batch API to fetch multiple pages efficiently
	const batchSize = 50
	for _, batch := range chunkStrings(titles, batchSize) {
		c.warmCacheBatch(ctx, batch)
	}

	c.logger.Info("Cache warming complete", "cached", atomic.LoadInt64(&c.cacheCount))
	return nil
}

// warmCacheBatch fetches page info for one batch of titles and caches each
// returned page. Failures are logged and skipped (best effort).
func (c *Client) warmCacheBatch(ctx context.Context, batch []string) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(batch, "|"))
	params.Set("prop", "info|revisions")
	params.Set("rvprop", "content|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		c.logger.Warn("Cache warming failed for batch", "error", err)
		return
	}

	// Cache each page result
	for _, pageData := range getNestedMap(resp, "query", "pages") {
		c.cacheWarmedPage(pageData)
	}
}

// cacheWarmedPage stores one page object under its normalized-title cache key.
func (c *Client) cacheWarmedPage(pageData interface{}) {
	page := getMap(pageData)
	if page == nil {
		return
	}
	title := getString(page["title"])
	if title == "" {
		return
	}
	c.setCache("page:"+normalizePageTitle(title), page, "page_content")
}

// WarmCacheWithDefaults pre-loads common wiki pages like Main_Page and help pages.
// This is a convenience method that fetches typical high-traffic pages.
func (c *Client) WarmCacheWithDefaults(ctx context.Context) error {
	// Pre-warm wiki info cache via GetWikiInfo so the cached value
	// is a WikiInfo struct, not a raw map[string]interface{}.
	_, _ = c.GetWikiInfo(ctx, WikiInfoArgs{})

	// Try to warm cache with common pages (best effort)
	defaultPages := []string{"Main_Page", "Main Page"}
	return c.WarmCache(ctx, defaultPages)
}

// cacheCleanupLoop periodically cleans up expired entries and enforces size limits
func (c *Client) cacheCleanupLoop() {
	ticker := time.NewTicker(CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.logger.Debug("Cache cleanup loop stopped")
			return
		case <-ticker.C:
			c.cleanupCache()
		}
	}
}

// cleanupCache removes expired entries and evicts LRU entries if over limit
func (c *Client) cleanupCache() {
	now := time.Now()
	var expiredCount int64

	// First pass: remove expired entries
	c.cache.Range(func(key, value interface{}) bool {
		ce := value.(*CacheEntry)
		if now.After(ce.ExpiresAt) {
			c.cache.Delete(key)
			expiredCount++
		}
		return true
	})

	// Update counter for expired entries
	if expiredCount > 0 {
		atomic.AddInt64(&c.cacheCount, -expiredCount)
	}

	// Check if we need to evict for size limit
	currentCount := atomic.LoadInt64(&c.cacheCount)
	if currentCount > MaxCacheEntries {
		c.evictLRU(int(currentCount - MaxCacheEntries + MaxCacheEntries/10)) // Evict 10% extra
	}
}

// entryInfo pairs a cache key with its last access time for LRU sorting.
type entryInfo struct {
	key        string
	accessedAt time.Time
}

// evictLRU removes the least recently used entries
func (c *Client) evictLRU(count int) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	evicted := c.evictOldest(c.entriesByAge(), count)

	if evicted > 0 {
		newCount := atomic.AddInt64(&c.cacheCount, -int64(evicted))
		metrics.SetCacheSize(newCount)
		metrics.CacheEvictions.Add(float64(evicted))
	}
}

// entriesByAge returns all cache entries sorted by access time, oldest first.
func (c *Client) entriesByAge() []entryInfo {
	var entries []entryInfo
	c.cache.Range(func(key, value interface{}) bool {
		ce := value.(*CacheEntry)
		ce.mu.Lock()
		accessedAt := ce.AccessedAt
		ce.mu.Unlock()
		k, _ := key.(string)
		entries = append(entries, entryInfo{
			key:        k,
			accessedAt: accessedAt,
		})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].accessedAt.Before(entries[j].accessedAt)
	})
	return entries
}

// evictOldest deletes up to count entries (assumed sorted oldest first) and
// returns how many were removed.
func (c *Client) evictOldest(entries []entryInfo, count int) int {
	evicted := 0
	for _, entry := range entries {
		if evicted >= count {
			break
		}
		c.cache.Delete(entry.key)
		evicted++
	}
	return evicted
}

// touchEntry marks a cache entry as freshly accessed for LRU tracking.
func touchEntry(ce *CacheEntry, now time.Time) {
	ce.mu.Lock()
	ce.AccessedAt = now
	ce.mu.Unlock()
}

// getCached retrieves a cached value if it exists and hasn't expired
func (c *Client) getCached(key string) (interface{}, bool) {
	if entry, ok := c.cache.Load(key); ok {
		ce := entry.(*CacheEntry)
		now := time.Now()
		if now.Before(ce.ExpiresAt) {
			touchEntry(ce, now)
			metrics.RecordCacheAccess(true)
			return ce.Data, true
		}
		// Expired, delete it
		c.cache.Delete(key)
		newCount := atomic.AddInt64(&c.cacheCount, -1)
		metrics.SetCacheSize(newCount)
		metrics.CacheEvictions.Inc()
	}
	metrics.RecordCacheAccess(false)
	return nil, false
}

// setCache stores a value in the cache with the specified TTL
func (c *Client) setCache(key string, data interface{}, ttlKey string) {
	ttl := 5 * time.Minute // default
	if t, ok := c.cacheTTL[ttlKey]; ok {
		ttl = t
	}

	now := time.Now()

	// Check if this is a new entry or update
	_, existed := c.cache.Load(key)

	c.cache.Store(key, &CacheEntry{
		Data:       data,
		ExpiresAt:  now.Add(ttl),
		AccessedAt: now,
		Key:        key,
	})

	// Only increment count for new entries
	if !existed {
		newCount := atomic.AddInt64(&c.cacheCount, 1)
		metrics.SetCacheSize(newCount)

		// Trigger eviction if over limit (async to not block caller)
		if newCount > MaxCacheEntries {
			go c.evictLRU(int(newCount - MaxCacheEntries + MaxCacheEntries/10))
		}
	}
}

// InvalidateCachePrefix removes all cache entries with keys starting with prefix
func (c *Client) InvalidateCachePrefix(prefix string) {
	var deletedCount int64
	c.cache.Range(func(key, value interface{}) bool {
		k, _ := key.(string)
		if strings.HasPrefix(k, prefix) {
			c.cache.Delete(key)
			deletedCount++
		}
		return true
	})
	if deletedCount > 0 {
		newCount := atomic.AddInt64(&c.cacheCount, -deletedCount)
		metrics.SetCacheSize(newCount)
		metrics.CacheEvictions.Add(float64(deletedCount))
	}
}

// apiRequest makes a request to the MediaWiki API with rate limiting and circuit breaker
// acquireRateLimitSlot reserves a semaphore slot for the request, blocking on
