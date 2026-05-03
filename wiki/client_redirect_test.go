package wiki

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestAPIClientRefusesCrossOriginRedirect_307 is the regression test for the
// 307/308 cross-origin credential-leak vulnerability. The bug shape:
//
//   - Client.httpClient is constructed with no CheckRedirect policy.
//   - Go's default policy follows up to 10 redirects.
//   - For HTTP 307/308 specifically, the original method AND request body
//     are preserved across origins.
//   - The login flow at client.go's loginFresh POSTs an action=login form
//     containing lgpassword=<bot-password> via apiRequest.
//   - A wiki that returns 307 Location: https://attacker/... causes Go to
//     re-POST the entire form (including the password) to the attacker.
//
// This test verifies the fix: the client must refuse all redirects on the
// API client (CheckRedirect returns http.ErrUseLastResponse) so the 3xx
// response surfaces to the caller as an error without re-POSTing.
func TestAPIClientRefusesCrossOriginRedirect_307(t *testing.T) {
	// "Attacker" server: counts requests received and records bodies.
	var attackerHits int32
	var attackerBody string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		body, _ := io.ReadAll(r.Body)
		attackerBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":{"result":"Success"}}`))
	}))
	defer attacker.Close()

	// "Wiki" server: returns 307 with Location pointing to the attacker.
	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", attacker.URL+"/exfil")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer wiki.Close()

	config := &Config{
		BaseURL:    wiki.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		UserAgent:  "TestRedirect/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	// Sensitive payload: an attacker-controllable redirect on the login leg
	// would re-POST this body (including lgpassword) to the attacker URL.
	const sentinelPassword = "SECRET_BOT_PASSWORD_DO_NOT_LEAK"
	params := url.Values{}
	params.Set("action", "login")
	params.Set("lgname", "TestBot@MCPLogin")
	params.Set("lgpassword", sentinelPassword)
	params.Set("lgtoken", "TOKEN")

	_, err := client.apiRequest(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from refused redirect, got nil")
	}

	// The attacker must NOT have received the body.
	if hits := atomic.LoadInt32(&attackerHits); hits != 0 {
		t.Errorf("attacker received %d requests; expected 0 (redirect must be refused)", hits)
	}
	if strings.Contains(attackerBody, sentinelPassword) {
		t.Errorf("attacker captured the bot password — credential leak via 307 redirect")
	}

	// The error must mention the redirect status and Location for diagnosability,
	// but must NOT echo the lgpassword (the body is not in the redirect response;
	// this is belt-and-braces against future regressions).
	if !strings.Contains(err.Error(), "redirect 307") {
		t.Errorf("error should mention refused redirect status; got: %v", err)
	}
	if strings.Contains(err.Error(), sentinelPassword) {
		t.Errorf("error message leaked the bot password: %v", err)
	}
}

// TestAPIClientRefusesCrossOriginRedirect_308 covers HTTP 308 Permanent
// Redirect, which has the same body-preservation semantics as 307.
func TestAPIClientRefusesCrossOriginRedirect_308(t *testing.T) {
	var attackerHits int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", attacker.URL)
		w.WriteHeader(http.StatusPermanentRedirect)
	}))
	defer wiki.Close()

	config := &Config{
		BaseURL:    wiki.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		UserAgent:  "TestRedirect/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	params := url.Values{}
	params.Set("action", "query")

	_, err := client.apiRequest(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from refused redirect, got nil")
	}
	if hits := atomic.LoadInt32(&attackerHits); hits != 0 {
		t.Errorf("attacker received %d requests; expected 0", hits)
	}
	if !strings.Contains(err.Error(), "redirect 308") {
		t.Errorf("error should mention refused 308 redirect; got: %v", err)
	}
}

// TestAPIClientRefusesSchemeDowngrade302 covers a weaker but still relevant
// case: 302 Found doesn't preserve POST body in browsers, but Go's default
// policy historically did for 301/302/303 in some cases. The fix is the same:
// refuse all redirects on the API client.
func TestAPIClientRefusesSchemeDowngrade302(t *testing.T) {
	var attackerHits int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	wiki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", attacker.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer wiki.Close()

	config := &Config{
		BaseURL:    wiki.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		UserAgent:  "TestRedirect/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	params := url.Values{}
	params.Set("action", "query")

	_, err := client.apiRequest(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from refused redirect, got nil")
	}
	if hits := atomic.LoadInt32(&attackerHits); hits != 0 {
		t.Errorf("attacker received %d requests; expected 0", hits)
	}
}
