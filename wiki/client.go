package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/text/unicode/norm"
)

// CacheEntry holds cached data with expiration and LRU tracking
type CacheEntry struct {
	Data       interface{}
	ExpiresAt  time.Time
	AccessedAt time.Time // For LRU eviction
	Key        string    // Store key for eviction
	mu         sync.Mutex
}

// Cache size limits to prevent unbounded memory growth
const (
	MaxCacheEntries      = 1000            // Maximum number of cache entries
	CacheCleanupInterval = 5 * time.Minute // How often to run cache cleanup
)

// Client handles communication with the MediaWiki API
type Client struct {
	config     *Config
	httpClient *http.Client
	logger     *slog.Logger

	// Authentication state
	mu          sync.RWMutex
	loggedIn    bool
	csrfToken   string
	tokenExpiry time.Time

	// Rate limiting - semaphore to control concurrent requests
	semaphore chan struct{}

	// Response cache with LRU eviction
	cache      sync.Map // key (string) -> *CacheEntry
	cacheTTL   map[string]time.Duration
	cacheCount int64      // Atomic counter for cache size
	cacheMu    sync.Mutex // Protects eviction operations

	// Graceful shutdown
	stopCh   chan struct{}
	stopOnce sync.Once

	// Audit logging for write operations
	auditLogger AuditLogger
}

// MaxConcurrentRequests limits parallel API calls to prevent overwhelming the server
const MaxConcurrentRequests = 3

// NewClient creates a new MediaWiki API client
func NewClient(config *Config, logger *slog.Logger) *Client {
	jar, _ := cookiejar.New(nil)

	// Initialize semaphore for rate limiting
	sem := make(chan struct{}, MaxConcurrentRequests)

	// Configure HTTP transport for better connection reuse and performance
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false, // Enable gzip compression
		ForceAttemptHTTP2:   true,  // Use HTTP/2 when available
	}

	// Initialize cache TTLs for different operations
	cacheTTL := map[string]time.Duration{
		"wiki_info":    60 * time.Minute, // Wiki info rarely changes
		"page_info":    2 * time.Minute,  // Page metadata
		"page_content": 5 * time.Minute,  // Page content
		"categories":   10 * time.Minute, // Category lists
		"search":       1 * time.Minute,  // Search results
	}

	client := &Client{
		config: config,
		httpClient: &http.Client{
			Timeout:   config.Timeout,
			Jar:       jar,
			Transport: transport,
		},
		logger:    logger,
		semaphore: sem,
		cacheTTL:  cacheTTL,
		stopCh:    make(chan struct{}),
	}

	// Start background cache cleanup routine
	go client.cacheCleanupLoop()

	return client
}

// Close gracefully shuts down the client, stopping background goroutines
func (c *Client) Close() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.logger.Debug("Client shutdown initiated")
	})
}

// SetAuditLogger configures an audit logger for tracking write operations.
// Pass nil to disable audit logging. The audit logger records page edits,
// creations, and file uploads with timestamps, content hashes, and metadata.
func (c *Client) SetAuditLogger(logger AuditLogger) {
	c.auditLogger = logger
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

// evictLRU removes the least recently used entries
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
		entries = append(entries, entryInfo{
			key:        key.(string),
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

	atomic.AddInt64(&c.cacheCount, -int64(evicted))
}

// getCached retrieves a cached value if it exists and hasn't expired
func (c *Client) getCached(key string) (interface{}, bool) {
	if entry, ok := c.cache.Load(key); ok {
		ce := entry.(*CacheEntry)
		now := time.Now()
		if now.Before(ce.ExpiresAt) {
			// Update access time for LRU tracking
			ce.mu.Lock()
			ce.AccessedAt = now
			ce.mu.Unlock()
			return ce.Data, true
		}
		// Expired, delete it
		c.cache.Delete(key)
		atomic.AddInt64(&c.cacheCount, -1)
	}
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
		if strings.HasPrefix(key.(string), prefix) {
			c.cache.Delete(key)
			deletedCount++
		}
		return true
	})
	if deletedCount > 0 {
		atomic.AddInt64(&c.cacheCount, -deletedCount)
	}
}

// apiRequest makes a request to the MediaWiki API with rate limiting
func (c *Client) apiRequest(ctx context.Context, params url.Values) (map[string]interface{}, error) {
	// Acquire semaphore slot (rate limiting)
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled while waiting for rate limiter: %w", ctx.Err())
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context error: %w", err)
	}

	params.Set("format", "json")

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with context awareness
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			}
		}

		// Create fresh request for each attempt (body is consumed on read)
		req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", c.config.UserAgent)
		// Note: Don't set Accept-Encoding manually - Go's http.Transport handles
		// compression automatically when DisableCompression is false

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			c.logger.Warn("API request failed, retrying",
				"attempt", attempt+1,
				"max_retries", c.config.MaxRetries,
				"error", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // Error ignored intentionally; body already read

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		// Handle different status codes appropriately
		if resp.StatusCode != http.StatusOK {
			// Don't retry client errors (4xx) except rate limiting (429)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return nil, fmt.Errorf("client error %d: %s", resp.StatusCode, string(body))
			}

			// Handle rate limiting with Retry-After header
			if resp.StatusCode == 429 {
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
						c.logger.Warn("Rate limited, waiting",
							"retry_after", seconds,
							"attempt", attempt+1)
						select {
						case <-time.After(time.Duration(seconds) * time.Second):
						case <-ctx.Done():
							return nil, fmt.Errorf("context cancelled during rate limit wait: %w", ctx.Err())
						}
						continue
					}
				}
			}

			lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
			c.logger.Warn("API returned non-OK status",
				"status", resp.StatusCode,
				"attempt", attempt+1)
			continue
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Check for API errors
		if errObj, ok := result["error"].(map[string]interface{}); ok {
			code, _ := errObj["code"].(string)
			info, _ := errObj["info"].(string)
			return nil, fmt.Errorf("API error [%s]: %s", code, info)
		}

		return result, nil
	}

	return nil, lastErr
}

// checkExistingSession verifies if we're already logged in via existing cookies
// Returns true if already authenticated, false otherwise
func (c *Client) checkExistingSession(ctx context.Context) bool {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "userinfo")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return false
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return false
	}

	userinfo, ok := query["userinfo"].(map[string]interface{})
	if !ok {
		return false
	}

	// If user ID is 0, we're not logged in (anonymous)
	userID, ok := userinfo["id"].(float64)
	if !ok || userID == 0 {
		return false
	}

	// We have a valid session
	name, _ := userinfo["name"].(string)
	c.logger.Debug("Found existing session", "user", name, "id", int(userID))
	return true
}

// resetCookies clears all cookies to allow fresh login
func (c *Client) resetCookies() {
	jar, _ := cookiejar.New(nil)
	c.httpClient.Jar = jar
	c.loggedIn = false
	c.csrfToken = ""
	c.tokenExpiry = time.Time{}
	c.logger.Debug("Cookies reset for fresh login")
}

// login authenticates with the wiki using bot password
func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggedIn && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	if !c.config.HasCredentials() {
		return fmt.Errorf("no credentials configured. Set MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables")
	}

	// Check if we already have a valid session from cookies
	// This prevents the "Cannot log in when using BotPasswordSessionProvider" error
	if c.checkExistingSession(ctx) {
		c.loggedIn = true
		c.tokenExpiry = time.Now().Add(60 * time.Minute) // Trust the existing session longer
		c.logger.Info("Using existing session")
		return nil
	}

	// Get login token
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "login")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to get login token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no tokens in response")
	}

	loginToken, ok := tokens["logintoken"].(string)
	if !ok {
		return fmt.Errorf("no login token in response")
	}

	// Perform login
	params = url.Values{}
	params.Set("action", "login")
	params.Set("lgname", c.config.Username)
	params.Set("lgpassword", c.config.Password)
	params.Set("lgtoken", loginToken)

	resp, err = c.apiRequest(ctx, params)
	if err != nil {
		// Check for BotPasswordSessionProvider error and retry with fresh cookies
		if strings.Contains(err.Error(), "BotPasswordSessionProvider") {
			c.logger.Warn("BotPasswordSessionProvider conflict detected, resetting cookies")
			c.resetCookies()
			// Retry login once with fresh cookies
			return c.loginFresh(ctx)
		}
		return fmt.Errorf("login failed: %w", err)
	}

	login, ok := resp["login"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected login response")
	}

	result, ok := login["result"].(string)
	if !ok {
		return fmt.Errorf("login result not a string")
	}
	if result != "Success" {
		reason := login["reason"]
		// Check for BotPasswordSessionProvider in the reason
		if reason != nil {
			reasonStr := fmt.Sprintf("%v", reason)
			if strings.Contains(reasonStr, "BotPasswordSessionProvider") {
				c.logger.Warn("BotPasswordSessionProvider conflict in login result, resetting cookies")
				c.resetCookies()
				return c.loginFresh(ctx)
			}
			return fmt.Errorf("login failed: %s - %v", result, reason)
		}
		return fmt.Errorf("login failed: %s", result)
	}

	c.loggedIn = true
	c.tokenExpiry = time.Now().Add(60 * time.Minute) // Extended from 20 to 60 minutes

	c.logger.Info("Successfully logged in", "username", c.config.Username)

	return nil
}

// loginFresh performs login with guaranteed fresh cookies (no retry)
func (c *Client) loginFresh(ctx context.Context) error {
	// Get login token
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "login")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to get login token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no tokens in response")
	}

	loginToken, ok := tokens["logintoken"].(string)
	if !ok {
		return fmt.Errorf("no login token in response")
	}

	// Perform login
	params = url.Values{}
	params.Set("action", "login")
	params.Set("lgname", c.config.Username)
	params.Set("lgpassword", c.config.Password)
	params.Set("lgtoken", loginToken)

	resp, err = c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	login, ok := resp["login"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected login response")
	}

	result, ok := login["result"].(string)
	if !ok {
		return fmt.Errorf("login result not a string")
	}
	if result != "Success" {
		reason := login["reason"]
		if reason != nil {
			return fmt.Errorf("login failed: %s - %v", result, reason)
		}
		return fmt.Errorf("login failed: %s", result)
	}

	c.loggedIn = true
	c.tokenExpiry = time.Now().Add(60 * time.Minute)

	c.logger.Info("Successfully logged in with fresh session", "username", c.config.Username)

	return nil
}

// getCSRFToken gets a CSRF token for editing
func (c *Client) getCSRFToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.csrfToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.csrfToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	if err := c.login(ctx); err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "csrf")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to get CSRF token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing query")
	}
	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing tokens")
	}
	csrfToken, ok := tokens["csrftoken"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing csrftoken")
	}

	c.mu.Lock()
	c.csrfToken = csrfToken
	c.tokenExpiry = time.Now().Add(60 * time.Minute)
	c.mu.Unlock()

	return csrfToken, nil
}

// EnsureLoggedIn ensures the client is logged in (for wikis requiring auth for read)
func (c *Client) EnsureLoggedIn(ctx context.Context) error {
	c.mu.RLock()
	loggedIn := c.loggedIn && time.Now().Before(c.tokenExpiry)
	c.mu.RUnlock()

	if loggedIn {
		return nil
	}

	return c.login(ctx)
}

// truncateContent truncates content if it exceeds the limit
func truncateContent(content string, limit int) (string, bool) {
	if len(content) <= limit {
		return content, false
	}

	truncationMsg := fmt.Sprintf(`

---
[CONTENT TRUNCATED]
Showing: %d of %d characters (%.1f%% of full content)

To get the full content:
1. Request specific sections using the 'section' parameter
2. Use mediawiki_get_page_info to check the full page size first
3. For very large pages, consider fetching in chunks`,
		limit, len(content), float64(limit)/float64(len(content))*100)

	return content[:limit] + truncationMsg, true
}

// normalizeLimit ensures limit is within bounds
func normalizeLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

// normalizeCategoryName ensures category name has proper prefix
func normalizeCategoryName(name string) string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "Category:") {
		name = "Category:" + name
	}
	return name
}

// NormalizeUnicode applies NFC normalization to a string
// This prevents Unicode-based bypass attacks where attackers use alternative
// representations of characters (e.g., combining characters, different encodings)
// to evade security checks. NFC (Canonical Decomposition, followed by Canonical Composition)
// is the standard normalization form used by MediaWiki.
func NormalizeUnicode(s string) string {
	return norm.NFC.String(s)
}

// normalizePageTitle normalizes a page title to MediaWiki conventions:
// - Applies Unicode NFC normalization
// - Trims whitespace
// - Replaces underscores with spaces
// - Capitalizes the first letter of the title and namespace prefix
// This helps handle case variations like "Module overview" vs "Module Overview"
func normalizePageTitle(title string) string {
	// Apply Unicode normalization first to prevent bypass attacks
	title = NormalizeUnicode(title)
	title = strings.TrimSpace(title)
	if title == "" {
		return title
	}

	// Replace underscores with spaces (MediaWiki convention)
	title = strings.ReplaceAll(title, "_", " ")

	// Collapse multiple spaces
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}

	// Capitalize first letter (MediaWiki default behavior)
	if colonIdx := strings.Index(title, ":"); colonIdx > 0 {
		// Has namespace prefix - capitalize both the prefix and the page title
		prefix := title[:colonIdx]
		rest := title[colonIdx+1:]

		// Capitalize the namespace prefix
		prefix = strings.ToUpper(string(prefix[0])) + prefix[1:]

		// Capitalize the first letter after the colon
		if len(rest) > 0 {
			rest = strings.ToUpper(string(rest[0])) + rest[1:]
		}
		return prefix + ":" + rest
	}

	// No namespace prefix - capitalize first letter
	return strings.ToUpper(string(title[0])) + title[1:]
}

// HTML sanitization patterns for XSS prevention - optimized with combined regexes
// Note: Go's regexp doesn't support backreferences, so we use separate patterns for each tag
var (
	// Patterns for dangerous tags with content (separate patterns since Go doesn't support backrefs)
	scriptTagRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRegex  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	iframeTagRegex = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	objectTagRegex = regexp.MustCompile(`(?is)<object[^>]*>.*?</object>`)
	embedTagRegex  = regexp.MustCompile(`(?is)<embed[^>]*>.*?</embed>`)
	appletTagRegex = regexp.MustCompile(`(?is)<applet[^>]*>.*?</applet>`)
	formTagRegex   = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)

	// Combined pattern for self-closing dangerous tags (single pass for 3 tag types)
	dangerousSelfClosingTagsRegex = regexp.MustCompile(`(?is)<(?:meta|link|base)[^>]*>`)

	// Remove event handler attributes (onclick, onerror, onload, etc.)
	eventHandlerRegex = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)

	// Combined pattern for dangerous URL schemes (javascript: and data:)
	dangerousURLRegex = regexp.MustCompile(`(?i)(href|src|action)\s*=\s*["']?\s*(?:javascript|data):[^"'>\s]*["']?`)

	// Remove style attributes that could contain expressions
	styleAttrRegex = regexp.MustCompile(`(?i)\s+style\s*=\s*(?:"[^"]*"|'[^']*')`)

	// Remove HTML comments (including MediaWiki cache comments like <!-- NewPP limit report -->)
	htmlCommentRegex = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// sanitizeHTML removes potentially dangerous HTML elements and attributes
// to prevent XSS attacks when HTML content is displayed by clients
func sanitizeHTML(html string) string {
	// Remove dangerous tags with content
	html = scriptTagRegex.ReplaceAllString(html, "")
	html = styleTagRegex.ReplaceAllString(html, "")
	html = iframeTagRegex.ReplaceAllString(html, "")
	html = objectTagRegex.ReplaceAllString(html, "")
	html = embedTagRegex.ReplaceAllString(html, "")
	html = appletTagRegex.ReplaceAllString(html, "")
	html = formTagRegex.ReplaceAllString(html, "")

	// Remove self-closing dangerous tags (meta, link, base)
	html = dangerousSelfClosingTagsRegex.ReplaceAllString(html, "")

	// Remove event handlers
	html = eventHandlerRegex.ReplaceAllString(html, "")

	// Remove dangerous URL schemes (javascript:, data:)
	html = dangerousURLRegex.ReplaceAllString(html, "$1=\"\"")

	// Remove style attributes (can contain CSS expressions)
	html = styleAttrRegex.ReplaceAllString(html, "")

	// Remove HTML comments (reduces token usage by removing MediaWiki cache comments)
	html = htmlCommentRegex.ReplaceAllString(html, "")

	return html
}

// =============================================================================
// Safe Type Assertion Helpers for API Response Parsing
// =============================================================================

// getMap safely extracts a map from an interface, returning nil if not a map
func getMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// getString safely extracts a string from an interface, returning empty string if not a string
func getString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// getFloat64 safely extracts a float64 from an interface, returning 0 if not a float64
func getFloat64(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// getInt safely extracts an int from a float64 interface (JSON numbers are float64)
func getInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

// getBool safely extracts a bool from an interface, returning false if not a bool
func getBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// getSlice safely extracts a slice from an interface, returning nil if not a slice
func getSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

// getNestedMap safely navigates nested maps using a path of keys
// Returns nil if any key is missing or value is not a map
func getNestedMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		if current == nil {
			return nil
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// getNestedString safely extracts a string from a nested path
func getNestedString(data map[string]interface{}, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return getString(data[keys[0]])
	}
	nested := getNestedMap(data, keys[:len(keys)-1]...)
	if nested == nil {
		return ""
	}
	return getString(nested[keys[len(keys)-1]])
}
