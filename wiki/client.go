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
	"strings"
	"sync"
	"time"
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
}

// MaxConcurrentRequests limits parallel API calls to prevent overwhelming the server
const MaxConcurrentRequests = 3

// NewClient creates a new MediaWiki API client
func NewClient(config *Config, logger *slog.Logger) *Client {
	jar, _ := cookiejar.New(nil)

	// Initialize semaphore for rate limiting
	sem := make(chan struct{}, MaxConcurrentRequests)

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Jar:     jar,
		},
		logger:    logger,
		semaphore: sem,
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
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
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
	c.tokenExpiry = time.Now().Add(20 * time.Minute)

	c.logger.Info("Successfully logged in", "username", c.config.Username)

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
	c.tokenExpiry = time.Now().Add(20 * time.Minute)
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

// HTML sanitization patterns for XSS prevention
var (
	// Remove dangerous tags entirely (including content)
	scriptTagRegex  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRegex   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	iframeTagRegex  = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	objectTagRegex  = regexp.MustCompile(`(?is)<object[^>]*>.*?</object>`)
	embedTagRegex   = regexp.MustCompile(`(?is)<embed[^>]*>.*?</embed>`)
	appletTagRegex  = regexp.MustCompile(`(?is)<applet[^>]*>.*?</applet>`)
	formTagRegex    = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)
	metaTagRegex    = regexp.MustCompile(`(?is)<meta[^>]*>`)
	linkTagRegex    = regexp.MustCompile(`(?is)<link[^>]*>`)
	baseTagRegex    = regexp.MustCompile(`(?is)<base[^>]*>`)

	// Remove event handler attributes (onclick, onerror, onload, etc.)
	eventHandlerRegex = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]*)`)

	// Remove javascript: and data: URLs in href/src attributes
	jsURLRegex   = regexp.MustCompile(`(?i)(href|src|action)\s*=\s*["']?\s*javascript:[^"'>\s]*["']?`)
	dataURLRegex = regexp.MustCompile(`(?i)(href|src)\s*=\s*["']?\s*data:[^"'>\s]*["']?`)

	// Remove style attributes that could contain expressions
	styleAttrRegex = regexp.MustCompile(`(?i)\s+style\s*=\s*("[^"]*"|'[^']*')`)
)

// sanitizeHTML removes potentially dangerous HTML elements and attributes
// to prevent XSS attacks when HTML content is displayed by clients
func sanitizeHTML(html string) string {
	// Remove dangerous tags (with their content)
	html = scriptTagRegex.ReplaceAllString(html, "")
	html = styleTagRegex.ReplaceAllString(html, "")
	html = iframeTagRegex.ReplaceAllString(html, "")
	html = objectTagRegex.ReplaceAllString(html, "")
	html = embedTagRegex.ReplaceAllString(html, "")
	html = appletTagRegex.ReplaceAllString(html, "")
	html = formTagRegex.ReplaceAllString(html, "")
	html = metaTagRegex.ReplaceAllString(html, "")
	html = linkTagRegex.ReplaceAllString(html, "")
	html = baseTagRegex.ReplaceAllString(html, "")

	// Remove event handlers
	html = eventHandlerRegex.ReplaceAllString(html, "")

	// Remove dangerous URL schemes
	html = jsURLRegex.ReplaceAllString(html, "$1=\"\"")
	html = dataURLRegex.ReplaceAllString(html, "$1=\"\"")

	// Remove style attributes (can contain CSS expressions)
	html = styleAttrRegex.ReplaceAllString(html, "")

	return html
}
