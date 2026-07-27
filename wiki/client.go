package wiki

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
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

// SetAuditLogger configures an audit logger for tracking write operations.
// Pass nil to disable audit logging. The audit logger records page edits,
// creations, and file uploads with timestamps, content hashes, and metadata.
func (c *Client) SetAuditLogger(logger AuditLogger) {
	c.auditLogger = logger
}
