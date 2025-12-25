// Package httputil provides HTTP utilities for the Nordic Registry clients.
package httputil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/olgasafonova/nordic-registry-mcp-server/internal/cache"
)

// ClientConfig holds configuration for the HTTP client.
type ClientConfig struct {
	BaseURL     string
	Timeout     time.Duration
	UserAgent   string
	CacheTTL    time.Duration
	RetryConfig RetryConfig
	RateLimit   time.Duration // Minimum time between requests
}

// Client is an HTTP client with caching, retry, and rate limiting.
type Client struct {
	config      ClientConfig
	httpClient  *http.Client
	cache       *cache.Cache
	rateLimiter *RateLimiter
	logger      *slog.Logger
	mu          sync.Mutex
	requestID   uint64
}

// NewClient creates a new HTTP client with the specified configuration.
func NewClient(config ClientConfig, logger *slog.Logger) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 1 * time.Hour
	}
	if config.RetryConfig.MaxRetries == 0 {
		config.RetryConfig = DefaultRetryConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	c := &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		cache:  cache.New(config.CacheTTL),
		logger: logger,
	}

	if config.RateLimit > 0 {
		c.rateLimiter = NewRateLimiter(config.RateLimit)
	}

	return c
}

// RequestOptions configures a single request.
type RequestOptions struct {
	CacheKey   string        // If set, result will be cached with this key
	CacheTTL   time.Duration // Override default cache TTL
	SkipCache  bool          // Skip cache lookup
	SkipRetry  bool          // Skip retry logic
}

// Get performs a GET request with optional caching and retry.
func (c *Client) Get(ctx context.Context, url string, opts RequestOptions) ([]byte, error) {
	// Generate request ID for logging
	c.mu.Lock()
	c.requestID++
	reqID := c.requestID
	c.mu.Unlock()

	logger := c.logger.With("request_id", reqID, "url", url)

	// Check cache first
	if opts.CacheKey != "" && !opts.SkipCache {
		if cached, ok := c.cache.Get(opts.CacheKey); ok {
			logger.Debug("Cache hit")
			return cached.([]byte), nil
		}
		logger.Debug("Cache miss")
	}

	// Rate limiting
	if c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	var body []byte
	var err error

	if opts.SkipRetry {
		body, err = c.doGet(ctx, url, logger)
	} else {
		body, err = DoWithRetry(ctx, c.config.RetryConfig, func() ([]byte, error) {
			return c.doGet(ctx, url, logger)
		})
	}

	if err != nil {
		return nil, err
	}

	// Cache the result
	if opts.CacheKey != "" {
		ttl := opts.CacheTTL
		if ttl == 0 {
			ttl = c.config.CacheTTL
		}
		c.cache.SetWithTTL(opts.CacheKey, body, ttl)
		logger.Debug("Cached result", "ttl", ttl)
	}

	return body, nil
}

// GetJSON performs a GET request and decodes the JSON response.
func (c *Client) GetJSON(ctx context.Context, url string, opts RequestOptions, result any) error {
	body, err := c.Get(ctx, url, opts)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}

	return nil
}

// doGet performs the actual HTTP GET request.
func (c *Client) doGet(ctx context.Context, url string, logger *slog.Logger) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if c.config.UserAgent != "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		logger.Error("Request failed", "error", err, "duration", duration)
		return nil, &RetryableError{Err: fmt.Errorf("request failed: %w", err)}
	}
	defer resp.Body.Close()

	logger.Debug("Request completed", "status", resp.StatusCode, "duration", duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		if IsRetryable(nil, resp.StatusCode) {
			return nil, &RetryableError{Err: err, StatusCode: resp.StatusCode}
		}
		return nil, err
	}

	return body, nil
}

// ClearCache clears the entire cache.
func (c *Client) ClearCache() {
	c.cache.Clear()
}

// CacheSize returns the number of items in the cache.
func (c *Client) CacheSize() int {
	return c.cache.Size()
}
