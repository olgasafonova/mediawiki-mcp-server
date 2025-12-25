package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
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
