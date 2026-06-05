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

func (c *Client) WarmCache(ctx context.Context, titles []string) error {
	if len(titles) == 0 {
		return nil
	}

	c.logger.Info("Warming cache", "pages", len(titles))

	// Use batch API to fetch multiple pages efficiently
	const batchSize = 50
	for i := 0; i < len(titles); i += batchSize {
		end := i + batchSize
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		// Fetch page info for the batch
		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))
		params.Set("prop", "info|revisions")
		params.Set("rvprop", "content|timestamp")
		params.Set("rvslots", "main")

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			c.logger.Warn("Cache warming failed for batch", "error", err)
			continue
		}

		// Cache each page result
		if query, ok := resp["query"].(map[string]interface{}); ok {
			if pages, ok := query["pages"].(map[string]interface{}); ok {
				for _, pageData := range pages {
					page, ok := pageData.(map[string]interface{})
					if !ok {
						continue
					}
					title := getString(page["title"])
					if title != "" {
						cacheKey := "page:" + normalizePageTitle(title)
						c.setCache(cacheKey, page, "page_content")
					}
				}
			}
		}
	}

	c.logger.Info("Cache warming complete", "cached", atomic.LoadInt64(&c.cacheCount))
	return nil
}

// WarmCacheWithDefaults pre-loads common wiki pages like Main_Page and help pages.
func (c *Client) WarmCacheWithDefaults(ctx context.Context) error {
	// Pre-warm wiki info cache via GetWikiInfo so the cached value
	// is a WikiInfo struct, not a raw map[string]interface{}.
	_, _ = c.GetWikiInfo(ctx, WikiInfoArgs{})

	// Try to warm cache with common pages (best effort)
	defaultPages := []string{"Main_Page", "Main Page"}
	return c.WarmCache(ctx, defaultPages)
}

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

func (c *Client) evictLRU(count int) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Collect all entries with their access times
	type entryInfo struct {
		key        string
		accessedAt time.Time
	}
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

	// Sort by access time (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].accessedAt.Before(entries[j].accessedAt)
	})

	// Evict the oldest entries
	evicted := 0
	for _, entry := range entries {
		if evicted >= count {
			break
		}
		c.cache.Delete(entry.key)
		evicted++
	}

	if evicted > 0 {
		newCount := atomic.AddInt64(&c.cacheCount, -int64(evicted))
		metrics.SetCacheSize(newCount)
		metrics.CacheEvictions.Add(float64(evicted))
	}
}

func (c *Client) getCached(key string) (interface{}, bool) {
	if entry, ok := c.cache.Load(key); ok {
		ce := entry.(*CacheEntry)
		now := time.Now()
		if now.Before(ce.ExpiresAt) {
			// Update access time for LRU tracking
			ce.mu.Lock()
			ce.AccessedAt = now
			ce.mu.Unlock()
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
