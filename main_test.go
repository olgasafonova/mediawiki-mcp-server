package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/olgasafonova/mcp-servercard-go/servercard"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	defer rl.Close()

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
	if rl.rate != 10 {
		t.Errorf("rate = %d, want 10", rl.rate)
	}
	if rl.interval != time.Minute {
		t.Errorf("interval = %v, want %v", rl.interval, time.Minute)
	}
	if rl.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)
	defer rl.Close()

	ip := "192.168.1.1"

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow(ip) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if rl.Allow(ip) {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiterMultipleIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Second)
	defer rl.Close()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Each IP should have its own bucket
	for i := 0; i < 2; i++ {
		if !rl.Allow(ip1) {
			t.Errorf("Request %d for ip1 should be allowed", i+1)
		}
		if !rl.Allow(ip2) {
			t.Errorf("Request %d for ip2 should be allowed", i+1)
		}
	}

	// Both should now be rate limited
	if rl.Allow(ip1) {
		t.Error("ip1 should be rate limited")
	}
	if rl.Allow(ip2) {
		t.Error("ip2 should be rate limited")
	}
}

func TestRateLimiterClose(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)

	// Close should not panic
	rl.Close()

	// Multiple closes should be safe
	rl.Close()
	rl.Close()
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(1, 10*time.Millisecond)
	defer rl.Close()

	ip := "192.168.1.1"

	// First request allowed
	if !rl.Allow(ip) {
		t.Error("First request should be allowed")
	}

	// Immediate second should be denied
	if rl.Allow(ip) {
		t.Error("Immediate second request should be denied")
	}

	// Wait for refill
	time.Sleep(15 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow(ip) {
		t.Error("Request after refill should be allowed")
	}
}

func TestRecoverPanic(t *testing.T) {
	// This test verifies recoverPanic properly catches panics
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Simulate panic recovery
	func() {
		defer recoverPanic(logger, "test operation")
		panic("test panic")
	}()

	// If we get here, the panic was recovered
}

// Mock handler for testing
type mockHandler struct {
	called bool
}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.called = true
	w.WriteHeader(http.StatusOK)
}

func TestSecurityMiddlewareBasic(t *testing.T) {
	// Test basic middleware functionality
	handler := &mockHandler{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := SecurityConfig{
		MaxBodySize: 1000,
	}

	sm := NewSecurityMiddleware(handler, logger, config)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	sm.ServeHTTP(w, req)

	if !handler.called {
		t.Error("Handler should have been called")
	}
}

func TestSecurityMiddlewareWithRateLimit(t *testing.T) {
	handler := &mockHandler{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := SecurityConfig{
		RateLimit:   2, // 2 requests per minute
		MaxBodySize: 1000,
	}

	sm := NewSecurityMiddleware(handler, logger, config)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		handler.called = false
		w := httptest.NewRecorder()
		sm.ServeHTTP(w, req)
		if !handler.called {
			t.Errorf("Request %d should have been allowed", i+1)
		}
	}

	// Third request should be rate limited
	handler.called = false
	w := httptest.NewRecorder()
	sm.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestIsLoopbackBind(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{":8080", false},
		{"0.0.0.0:8080", false},
		{"192.168.1.10:8080", false},
		{"example.com:8080", false},
	}
	for _, c := range cases {
		if got := isLoopbackBind(c.addr); got != c.want {
			t.Errorf("isLoopbackBind(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// TestEnforceBindSecurity locks in the fail-closed rule: a non-loopback bind
// without a token is refused; every other combination is allowed to start.
func TestEnforceBindSecurity(t *testing.T) {
	logger := testLogger()
	cases := []struct {
		name      string
		token     string
		addr      string
		wantError bool
	}{
		{"non-loopback without token refused", "", "0.0.0.0:8080", true},
		{"bare port without token refused", "", ":8080", true},
		{"external ip without token refused", "", "192.168.1.10:8080", true},
		{"non-loopback with token allowed", "secret", "0.0.0.0:8080", false},
		{"loopback without token allowed", "", "127.0.0.1:8080", false},
		{"loopback with token allowed", "secret", "127.0.0.1:8080", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := enforceBindSecurity(logger, c.token, c.addr)
			if c.wantError && err == nil {
				t.Error("expected refusal error, got nil")
			}
			if !c.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestAuxEndpointsRequireAuth proves the aux endpoints now sit behind the bearer
// auth middleware: /tools is rejected without a token and served with one.
func TestAuxEndpointsRequireAuth(t *testing.T) {
	cfg := httpServerConfig{
		Logger:    testLogger(),
		AuthToken: "secret",
	}
	secured := newSecuredHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	secured.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/tools without token = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/tools", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	secured.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/tools with token = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestHealthOpenAuxSecuredViaMux checks the outer routing: /health stays open
// even when a token is configured, while /tools routes through the auth
// middleware and is rejected without a token.
func TestHealthOpenAuxSecuredViaMux(t *testing.T) {
	cfg := httpServerConfig{
		Logger:    testLogger(),
		AuthToken: "secret",
		Card:      &servercard.ServerCard{},
	}
	secured := newSecuredHandler(cfg, nil)
	mux := buildHTTPMux(cfg, secured)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/health without token = %d, want %d (must stay open)", w.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodGet, "/tools", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/tools via mux without token = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
