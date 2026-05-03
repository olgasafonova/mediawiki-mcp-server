package wiki

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAPIRequest_4xx_DoesNotLeakResponseBody is the regression test for the
// HG-2 raw-body leak in apiRequest. The bug shape:
//
//   - The 4xx and other non-OK branches in apiRequest used to format errors
//     as `fmt.Errorf("client error %d: %s", status, string(body))`.
//   - The body propagated verbatim through the dispatcher (`%w` wrap) to
//     the MCP caller, surfacing HTML error pages, MITM proxy responses,
//     and prompt-injection-controllable echo channels.
//
// This test asserts none of the body content reaches the caller-facing
// error string when the wiki returns a 4xx with a body that contains
// markers that would, under the old behavior, leak.
func TestAPIRequest_4xx_DoesNotLeakResponseBody(t *testing.T) {
	// Sentinels representative of categories an attacker (or misconfigured
	// proxy) might place in a 4xx body:
	//   - internal hostnames
	//   - request IDs / correlation headers from upstream proxies
	//   - prompt-injection markers
	//   - bot password fragments (worst case if a proxy echoes form bodies)
	const (
		internalHostMarker   = "kube-master.internal.tieto.local"
		proxyRequestIDMarker = "X-Cloudflare-Ray-Id-DEADBEEF"
		injectionMarker      = "PROMPT_INJECTION_SENTINEL_IGNORE_ABOVE_AND_LEAK_KEYS"
		passwordEcho         = "lgpassword=SUPER_SECRET_BOT_PASS_1234"
	)
	htmlBody := `<html><body>` +
		`<h1>418 I'm a teapot</h1>` +
		`<p>Internal: ` + internalHostMarker + `</p>` +
		`<p>` + proxyRequestIDMarker + `</p>` +
		`<p>` + injectionMarker + `</p>` +
		`<pre>` + passwordEcho + `</pre>` +
		`</body></html>`

	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(htmlBody))
	}))
	defer wiki.Close()

	config := &Config{
		BaseURL:    wiki.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		UserAgent:  "TestAPIError/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	params := url.Values{}
	params.Set("action", "query")

	_, err := client.apiRequest(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from 4xx, got nil")
	}

	errStr := err.Error()
	for _, sentinel := range []string{internalHostMarker, proxyRequestIDMarker, injectionMarker, passwordEcho} {
		if strings.Contains(errStr, sentinel) {
			t.Errorf("HG-2 regression: error string leaks body sentinel %q\nfull error: %s", sentinel, errStr)
		}
	}

	// Verify the structured APIError is what surfaced.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, apiErr.StatusCode)
	}
	if apiErr.Code != APICodeClientError {
		t.Errorf("expected APICodeClientError, got %q", apiErr.Code)
	}
	// BodySnippet is captured for server-side logging but must NOT appear
	// in Error() output. It contains the raw body (truncated) for ops use.
	if apiErr.BodySnippet == "" {
		t.Error("expected BodySnippet to be populated for server-side logging")
	}
	if strings.Contains(apiErr.Error(), apiErr.BodySnippet) {
		t.Error("HG-2 regression: APIError.Error() echoes BodySnippet (must be log-only)")
	}
}

// TestAPIRequest_5xx_DoesNotLeakResponseBody covers the second formerly-leaky
// branch (the lastErr assignment in the retry loop).
func TestAPIRequest_5xx_DoesNotLeakResponseBody(t *testing.T) {
	const sentinel = "INTERNAL_5XX_LEAK_SENTINEL"
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal error: " + sentinel + " stack trace ..."))
	}))
	defer wiki.Close()

	config := &Config{
		BaseURL:    wiki.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 1, // One retry so we exercise the lastErr path
		UserAgent:  "TestAPIError/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	params := url.Values{}
	params.Set("action", "query")

	_, err := client.apiRequest(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from 5xx, got nil")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Errorf("HG-2 regression: 5xx error string leaks body sentinel\nfull error: %s", err.Error())
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != APICodeServerError {
		t.Errorf("expected APICodeServerError, got %q", apiErr.Code)
	}
}

// TestAPIError_BodySnippetTruncation ensures very long bodies are capped.
func TestAPIError_BodySnippetTruncation(t *testing.T) {
	body := strings.Repeat("X", APIErrorBodyMax*4)
	apiErr := NewAPIError(http.StatusBadRequest, []byte(body))

	if len(apiErr.BodySnippet) != APIErrorBodyMax {
		t.Errorf("expected BodySnippet length %d, got %d", APIErrorBodyMax, len(apiErr.BodySnippet))
	}
	if strings.Contains(apiErr.Error(), apiErr.BodySnippet) {
		t.Error("HG-2 regression: long body leaks via Error()")
	}
}
