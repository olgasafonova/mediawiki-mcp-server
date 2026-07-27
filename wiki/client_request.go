package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/metrics"
)

// tokenPattern matches token values in API response bodies for redaction in debug logs.
var tokenPattern = regexp.MustCompile(`"(logintoken|csrftoken)":"[^"]*"`)

// maxResponseBytes bounds a single API response read. It sits well above
// CharacterLimit (250 KB) to leave headroom for HTML output; a misbehaving or
// hostile wiki streaming more than this is rejected rather than OOMing us.
const maxResponseBytes = 16 << 20 // 16 MiB

// acquireRateLimitSlot reserves a semaphore slot for the request, blocking on
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
	if isRedirectStatus(status) {
		location := resp.Header.Get("Location")
		return false, fmt.Errorf("wiki returned redirect %d (refused; API client does not follow redirects): Location=%q", status, location)
	}
	// Don't retry client errors (4xx) except rate limiting (429)
	if isTerminalClientError(status) {
		apiErr := NewAPIError(status, body)
		c.logger.Warn("API client error response",
			"status", status,
			"body_snippet", apiErr.BodySnippet)
		return false, apiErr
	}
	// Honor Retry-After on 429
	if status == http.StatusTooManyRequests {
		if waitErr := c.waitRetryAfter(ctx, resp, attempt); waitErr != nil {
			return false, waitErr
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

// isRedirectStatus reports whether the status code is a 3xx redirect.
func isRedirectStatus(status int) bool {
	return status >= 300 && status < 400
}

// isTerminalClientError reports whether the status is a 4xx error that must
// not be retried (429 rate limiting is the retryable exception).
func isTerminalClientError(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusTooManyRequests
}

// waitRetryAfter honors a parseable Retry-After header on a 429 response,
// blocking until the wait elapses or the context is canceled.
func (c *Client) waitRetryAfter(ctx context.Context, resp *http.Response, attempt int) error {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return nil
	}
	seconds, parseErr := strconv.Atoi(retryAfter)
	if parseErr != nil {
		return nil
	}
	c.logger.Warn("Rate limited, waiting",
		"retry_after", seconds,
		"attempt", attempt+1)
	select {
	case <-time.After(time.Duration(seconds) * time.Second):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context canceled during rate limit wait: %w", ctx.Err())
	}
}

// readBoundedBody reads the response body under a hard size cap, closing the
// body and reporting an error when the cap is exceeded (read limit+1 detects
// overflow).
func readBoundedBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	_ = resp.Body.Close() // Error ignored intentionally; body already read
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("response exceeds maximum size of %d bytes", maxResponseBytes)
	}
	return body, nil
}

// apiRequest makes a request to the MediaWiki API with rate limiting and circuit breaker.
func (c *Client) apiRequest(ctx context.Context, params url.Values) (map[string]interface{}, error) {
	action := params.Get("action")
	start := time.Now()

	release, err := c.preflightAPIRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	params.Set("format", "json")

	call := apiCall{params: params, action: action, start: start}
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if err := c.waitRetryBackoff(ctx, action, attempt); err != nil {
			return nil, err
		}
		result, retry, err := c.doAPIAttempt(ctx, call, attempt)
		if err == nil {
			return result, nil
		}
		if !retry {
			return nil, err
		}
		lastErr = err
	}

	duration := time.Since(start).Seconds()
	metrics.RecordAPICall(action, duration, false, "max_retries_exceeded")
	c.circuitBreaker.RecordFailure()
	return nil, lastErr
}

// preflightAPIRequest runs the pre-request gates: configuration, circuit
// breaker, rate limiter, and context liveness. On success it returns the
// rate-limit release function, which the caller must defer.
func (c *Client) preflightAPIRequest(ctx context.Context) (release func(), err error) {
	if !c.config.IsConfigured() {
		return nil, fmt.Errorf("MEDIAWIKI_URL is not configured. Set the MEDIAWIKI_URL environment variable to your wiki's API endpoint (e.g. https://wiki.example.com/api.php)")
	}

	// Check circuit breaker before proceeding
	if !c.circuitBreaker.Allow() {
		stats := c.circuitBreaker.Stats()
		return nil, &ErrCircuitOpen{
			State:    stats.State,
			RetryAt:  stats.LastFailure.Add(30 * time.Second),
			Failures: stats.ConsecutiveFails,
		}
	}

	release, err = c.acquireRateLimitSlot(ctx)
	if err != nil {
		return nil, err
	}

	// Check context before proceeding
	if ctxErr := ctx.Err(); ctxErr != nil {
		release()
		return nil, fmt.Errorf("context error: %w", ctxErr)
	}
	return release, nil
}

// waitRetryBackoff sleeps the exponential backoff for retry attempts (no-op on
// the first attempt) and records the retry metric. Returns an error only when
// the context is canceled during the wait.
func (c *Client) waitRetryBackoff(ctx context.Context, action string, attempt int) error {
	if attempt == 0 {
		return nil
	}
	metrics.WikiAPIRetries.WithLabelValues(action).Inc()
	// Exponential backoff with context awareness
	backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
	select {
	case <-time.After(backoff):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context canceled during backoff: %w", ctx.Err())
	}
}

// apiCall carries the per-request state shared across retry attempts.
type apiCall struct {
	params url.Values
	action string
	start  time.Time
}

// newAPIRequest builds a fresh POST request for one attempt (the body reader
// is consumed on send, so each retry needs its own request).
func (c *Client) newAPIRequest(ctx context.Context, call apiCall, attempt int) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, strings.NewReader(call.params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.config.UserAgent)
	// Note: Don't set Accept-Encoding manually - Go's http.Transport handles
	// compression automatically when DisableCompression is false

	c.logger.Debug("API request",
		"action", call.action,
		"url", c.config.BaseURL,
		"attempt", attempt+1)
	return req, nil
}

// doAPIAttempt performs a single request/response cycle. The bool return is
// true when the caller may retry the attempt; false means the error (or the
// successful result) is final.
func (c *Client) doAPIAttempt(ctx context.Context, call apiCall, attempt int) (map[string]interface{}, bool, error) {
	req, err := c.newAPIRequest(ctx, call, attempt)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req) // #nosec G704 -- URL is the configured wiki API endpoint, not user-controlled
	if err != nil {
		c.logger.Warn("API request failed, retrying",
			"attempt", attempt+1,
			"max_retries", c.config.MaxRetries,
			"error", err)
		return nil, true, fmt.Errorf("request failed: %w", err)
	}

	body, err := readBoundedBody(resp)
	if err != nil {
		return nil, true, err
	}

	// Handle different status codes appropriately
	if resp.StatusCode != http.StatusOK {
		c.logger.Debug("API non-OK response",
			"status", resp.StatusCode,
			"body_preview", redactTokens(string(body[:min(len(body), 500)])))
		retryable, respErr := c.handleNonOKResponse(ctx, resp, body, attempt)
		return nil, retryable, respErr
	}

	c.logger.Debug("API response",
		"status", resp.StatusCode,
		"body_preview", redactTokens(string(body[:min(len(body), 500)])))

	result, err := c.decodeAPIResult(body, call.action, call.start)
	return result, false, err
}

// decodeAPIResult parses a 200 response body, surfaces embedded API errors,
// and records metrics plus circuit-breaker success for the completed call.
func (c *Client) decodeAPIResult(body []byte, action string, start time.Time) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	duration := time.Since(start).Seconds()
	// Check for API errors
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		code, _ := errObj["code"].(string)
		info, _ := errObj["info"].(string)
		metrics.RecordAPICall(action, duration, false, code)
		// API errors don't indicate connectivity issues, so record success for circuit breaker
		c.circuitBreaker.RecordSuccess()
		return nil, fmt.Errorf("API error [%s]: %s", code, info)
	}

	metrics.RecordAPICall(action, duration, true, "")
	c.circuitBreaker.RecordSuccess()
	return result, nil
}

// redactTokens replaces token values in a JSON response string for safe debug logging.
func redactTokens(s string) string {
	return tokenPattern.ReplaceAllString(s, `"$1":"[REDACTED]"`)
}
