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

	"github.com/olgasafonova/mediawiki-mcp-server/metrics"
)

// tokenPattern matches token values in API response bodies for redaction in debug logs.
var tokenPattern = regexp.MustCompile(`"(?:logintoken|csrftoken)":"[^"]*"`)

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

	// Request deduplication - coalesce identical in-flight requests
	dedup *RequestDeduplicator

	// Circuit breaker - fail fast when wiki is unresponsive
	circuitBreaker *CircuitBreaker

	// Graceful shutdown
	stopCh   chan struct{}
	stopOnce sync.Once

	// Audit logging for write operations
	auditLogger AuditLogger

	// allowPrivateDownloadForTest, when true, bypasses validateFileURL in
	// downloadFile so httptest servers (bound to 127.0.0.1) work. Production
	// code never sets this; it is only flipped on by tests in this package.
	allowPrivateDownloadForTest bool
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
		// Connection pool settings
		MaxIdleConns:        100,               // Total idle connections across all hosts
		MaxIdleConnsPerHost: 20,                // Idle connections per host (increased for single-host pattern)
		MaxConnsPerHost:     50,                // Maximum connections per host
		IdleConnTimeout:     120 * time.Second, // Keep idle connections longer

		// Timeouts for connection establishment and TLS
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, // Max time to wait for response headers

		// Compression and HTTP/2
		DisableCompression: false, // Enable gzip compression
		ForceAttemptHTTP2:  true,  // Use HTTP/2 when available

		// Keep-alive probe settings
		DisableKeepAlives: false, // Ensure keep-alives are enabled
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
			// SECURITY: Refuse all redirects on the API client. The client carries
			// bot credentials and CSRF tokens; an HTTP 307/308 redirect would cause
			// Go's default policy to re-POST the body (including lgpassword) to the
			// new origin. The configured wiki URL is the only legitimate target,
			// so any redirect indicates either misconfiguration or a credential-
			// exfiltration attempt. Returning http.ErrUseLastResponse short-circuits
			// the redirect and lets the caller handle the 3xx response itself.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger:         logger,
		semaphore:      sem,
		cacheTTL:       cacheTTL,
		dedup:          NewRequestDeduplicator(),
		circuitBreaker: NewCircuitBreaker(),
		stopCh:         make(chan struct{}),
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

// CircuitBreakerStatus returns the current circuit breaker state
func (c *Client) CircuitBreakerStatus() CircuitBreakerStats {
	return c.circuitBreaker.Stats()
}

// DedupStats returns request deduplication statistics
func (c *Client) DedupStats() int {
	return c.dedup.Stats()
}

// WarmCache pre-loads commonly accessed pages into the cache.
// Call this after creating the client to improve first-request latency.
// This is a convenience method that fetches typical high-traffic pages.
// HealthStatus represents the result of a connectivity check
type HealthStatus struct {
	Connected     bool          `json:"connected"`
	WikiURL       string        `json:"wiki_url"`
	SiteName      string        `json:"site_name,omitempty"`
	Generator     string        `json:"generator,omitempty"`
	ResponseTime  time.Duration `json:"response_time_ms"`
	Authenticated bool          `json:"authenticated"`
	Error         string        `json:"error,omitempty"`
}

// Ping checks connectivity to the MediaWiki API and returns health status.
// This is a lightweight check that doesn't require authentication and isn't cached.
func (c *Client) Ping(ctx context.Context) HealthStatus {
	start := time.Now()
	status := HealthStatus{
		WikiURL:       c.config.BaseURL,
		Authenticated: c.isLoggedIn(),
	}

	// Make a simple siteinfo request
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "siteinfo")
	params.Set("siprop", "general")
	params.Set("format", "json")

	resp, err := c.apiRequest(ctx, params)
	status.ResponseTime = time.Since(start)

	if err != nil {
		status.Connected = false
		status.Error = err.Error()
		return status
	}

	status.Connected = true

	// Extract basic site info
	if query, ok := resp["query"].(map[string]interface{}); ok {
		if general, ok := query["general"].(map[string]interface{}); ok {
			status.SiteName = getString(general["sitename"])
			status.Generator = getString(general["generator"])
		}
	}

	return status
}

// isLoggedIn returns the current authentication state
// creations, and file uploads with timestamps, content hashes, and metadata.
func (c *Client) SetAuditLogger(logger AuditLogger) {
	c.auditLogger = logger
}

// cacheCleanupLoop periodically cleans up expired entries and enforces size limits
// cleanupCache removes expired entries and evicts LRU entries if over limit
// evictLRU removes the least recently used entries
// getCached retrieves a cached value if it exists and hasn't expired
// setCache stores a value in the cache with the specified TTL
// InvalidateCachePrefix removes all cache entries with keys starting with prefix
// context cancellation. Returns a release function that callers must defer.
func (c *Client) acquireRateLimitSlot(ctx context.Context) (release func(), err error) {
	release = func() { <-c.semaphore }
	select {
	case c.semaphore <- struct{}{}:
		return release, nil
	default:
		// Semaphore full, we'll wait but record it
		metrics.RateLimitWaits.Inc()
		select {
		case c.semaphore <- struct{}{}:
			return release, nil
		case <-ctx.Done():
			metrics.RateLimitRejections.Inc()
			return nil, fmt.Errorf("context canceled while waiting for rate limiter: %w", ctx.Err())
		}
	}
}

// handleNonOKResponse classifies a non-200 response into either a terminal
// error or a retryable error. The bool return is true when the caller should
// retry the request; false means the error should be returned immediately.
//
// SECURITY (HG-2): never echo the raw response body to the MCP caller. Bodies
// may contain HTML error pages, MITM proxy responses, or echoed login form
// parameters. Log truncated bodies for server-side diagnostics only.
func (c *Client) handleNonOKResponse(ctx context.Context, resp *http.Response, body []byte, attempt int) (retryable bool, err error) {
	status := resp.StatusCode
	// SECURITY: 3xx responses indicate the wiki tried to redirect the
	// authenticated request. Refuse to retry or follow.
	if status >= 300 && status < 400 {
		location := resp.Header.Get("Location")
		return false, fmt.Errorf("wiki returned redirect %d (refused; API client does not follow redirects): Location=%q", status, location)
	}
	// Don't retry client errors (4xx) except rate limiting (429)
	if status >= 400 && status < 500 && status != 429 {
		apiErr := NewAPIError(status, body)
		c.logger.Warn("API client error response",
			"status", status,
			"body_snippet", apiErr.BodySnippet)
		return false, apiErr
	}
	// Honor Retry-After on 429
	if status == 429 {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
				c.logger.Warn("Rate limited, waiting",
					"retry_after", seconds,
					"attempt", attempt+1)
				select {
				case <-time.After(time.Duration(seconds) * time.Second):
				case <-ctx.Done():
					return false, fmt.Errorf("context canceled during rate limit wait: %w", ctx.Err())
				}
			}
		}
	}
	// 5xx (and 429 without parseable Retry-After) are retryable
	apiErr := NewAPIError(status, body)
	c.logger.Warn("API returned non-OK status",
		"status", status,
		"attempt", attempt+1,
		"body_snippet", apiErr.BodySnippet)
	return true, apiErr
}

func (c *Client) apiRequest(ctx context.Context, params url.Values) (map[string]interface{}, error) {
	if !c.config.IsConfigured() {
		return nil, fmt.Errorf("MEDIAWIKI_URL is not configured. Set the MEDIAWIKI_URL environment variable to your wiki's API endpoint (e.g. https://wiki.example.com/api.php)")
	}

	action := params.Get("action")
	start := time.Now()

	// Check circuit breaker before proceeding
	if !c.circuitBreaker.Allow() {
		stats := c.circuitBreaker.Stats()
		return nil, &ErrCircuitOpen{
			State:    stats.State,
			RetryAt:  stats.LastFailure.Add(30 * time.Second),
			Failures: stats.ConsecutiveFails,
		}
	}

	release, err := c.acquireRateLimitSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context error: %w", err)
	}

	params.Set("format", "json")

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			metrics.WikiAPIRetries.WithLabelValues(action).Inc()
			// Exponential backoff with context awareness
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("context canceled during backoff: %w", ctx.Err())
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

		c.logger.Debug("API request",
			"action", action,
			"url", c.config.BaseURL,
			"attempt", attempt+1)

		resp, err := c.httpClient.Do(req) // #nosec G704 -- URL is the configured wiki API endpoint, not user-controlled
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
			c.logger.Debug("API non-OK response",
				"status", resp.StatusCode,
				"body_preview", redactTokens(string(body[:min(len(body), 500)])))
			retryable, err := c.handleNonOKResponse(ctx, resp, body, attempt)
			if !retryable {
				return nil, err
			}
			lastErr = err
			continue
		}

		c.logger.Debug("API response",
			"status", resp.StatusCode,
			"body_preview", redactTokens(string(body[:min(len(body), 500)])))

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Check for API errors
		if errObj, ok := result["error"].(map[string]interface{}); ok {
			code, _ := errObj["code"].(string)
			info, _ := errObj["info"].(string)
			duration := time.Since(start).Seconds()
			metrics.RecordAPICall(action, duration, false, code)
			// API errors don't indicate connectivity issues, so record success for circuit breaker
			c.circuitBreaker.RecordSuccess()
			return nil, fmt.Errorf("API error [%s]: %s", code, info)
		}

		duration := time.Since(start).Seconds()
		metrics.RecordAPICall(action, duration, true, "")
		c.circuitBreaker.RecordSuccess()
		return result, nil
	}

	duration := time.Since(start).Seconds()
	metrics.RecordAPICall(action, duration, false, "max_retries_exceeded")
	c.circuitBreaker.RecordFailure()
	return nil, lastErr
}

// checkExistingSession verifies if we're already logged in via existing cookies
// Returns true if already authenticated, false otherwise
// resetCookies clears all cookies to allow fresh login
// login authenticates with the wiki using bot password

// loginFresh performs login with guaranteed fresh cookies (no retry)
// getCSRFToken gets a CSRF token for editing
// after use, so we must not reuse them across edit requests.
// EnsureLoggedIn ensures the client is logged in (for wikis requiring auth for read)
// truncateContent truncates content if it exceeds the limit

// redactTokens replaces token values in a JSON response string for safe debug logging.
func redactTokens(s string) string {
	return tokenPattern.ReplaceAllString(s, `"$1":"[REDACTED]"`)
}
