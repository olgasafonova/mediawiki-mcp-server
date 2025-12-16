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
	"strconv"
	"strings"
	"sync"
	"time"
)

// CacheEntry holds cached data with expiration
type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
}

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

	// Response cache
	cache    sync.Map // key (string) -> *CacheEntry
	cacheTTL map[string]time.Duration
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

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout:   config.Timeout,
			Jar:       jar,
			Transport: transport,
		},
		logger:    logger,
		semaphore: sem,
		cacheTTL:  cacheTTL,
	}
}

// getCached retrieves a cached value if it exists and hasn't expired
func (c *Client) getCached(key string) (interface{}, bool) {
	if entry, ok := c.cache.Load(key); ok {
		ce := entry.(*CacheEntry)
		if time.Now().Before(ce.ExpiresAt) {
			return ce.Data, true
		}
		// Expired, delete it
		c.cache.Delete(key)
	}
	return nil, false
}

// setCache stores a value in the cache with the specified TTL
func (c *Client) setCache(key string, data interface{}, ttlKey string) {
	ttl := 5 * time.Minute // default
	if t, ok := c.cacheTTL[ttlKey]; ok {
		ttl = t
	}
	c.cache.Store(key, &CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	})
}

// InvalidateCachePrefix removes all cache entries with keys starting with prefix
func (c *Client) InvalidateCachePrefix(prefix string) {
	c.cache.Range(func(key, value interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			c.cache.Delete(key)
		}
		return true
	})
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

	result := login["result"].(string)
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

	result := login["result"].(string)
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

	query := resp["query"].(map[string]interface{})
	tokens := query["tokens"].(map[string]interface{})
	csrfToken := tokens["csrftoken"].(string)

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

// normalizePageTitle normalizes a page title to MediaWiki conventions:
// - Trims whitespace
// - Replaces underscores with spaces
// - Capitalizes the first letter of the title and namespace prefix
// This helps handle case variations like "Module overview" vs "Module Overview"
func normalizePageTitle(title string) string {
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
	scriptTagRegex  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRegex   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	iframeTagRegex  = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	objectTagRegex  = regexp.MustCompile(`(?is)<object[^>]*>.*?</object>`)
	embedTagRegex   = regexp.MustCompile(`(?is)<embed[^>]*>.*?</embed>`)
	appletTagRegex  = regexp.MustCompile(`(?is)<applet[^>]*>.*?</applet>`)
	formTagRegex    = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)

	// Combined pattern for self-closing dangerous tags (single pass for 3 tag types)
	dangerousSelfClosingTagsRegex = regexp.MustCompile(`(?is)<(?:meta|link|base)[^>]*>`)

	// Remove event handler attributes (onclick, onerror, onload, etc.)
	eventHandlerRegex = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)

	// Combined pattern for dangerous URL schemes (javascript: and data:)
	dangerousURLRegex = regexp.MustCompile(`(?i)(href|src|action)\s*=\s*["']?\s*(?:javascript|data):[^"'>\s]*["']?`)

	// Remove style attributes that could contain expressions
	styleAttrRegex = regexp.MustCompile(`(?i)\s+style\s*=\s*(?:"[^"]*"|'[^']*')`)
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

	return html
}
