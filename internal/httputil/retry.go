// Package httputil provides HTTP utilities including retry logic.
package httputil

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64 // 0.0 to 1.0
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.1,
	}
}

// RetryableError indicates an error that can be retried.
type RetryableError struct {
	Err        error
	StatusCode int
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable checks if an error or status code is retryable.
func IsRetryable(err error, statusCode int) bool {
	// Network errors are retryable
	if err != nil {
		var retryable *RetryableError
		if errors.As(err, &retryable) {
			return true
		}
	}

	// Retry on server errors and rate limiting
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusBadGateway:
		return true
	}

	return false
}

// DoWithRetry executes a function with retry logic.
func DoWithRetry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Check context
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		var retryable *RetryableError
		if !errors.As(err, &retryable) {
			// Not retryable, return immediately
			return zero, err
		}

		// Don't sleep after the last attempt
		if attempt < cfg.MaxRetries {
			backoff := calculateBackoff(cfg, attempt)
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(backoff):
				// Continue to next attempt
			}
		}
	}

	return zero, lastErr
}

// calculateBackoff calculates the backoff duration for a given attempt.
func calculateBackoff(cfg RetryConfig, attempt int) time.Duration {
	backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.Multiplier, float64(attempt))

	// Apply jitter
	if cfg.Jitter > 0 {
		jitter := backoff * cfg.Jitter * (rand.Float64()*2 - 1)
		backoff += jitter
	}

	// Cap at max backoff
	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}

	return time.Duration(backoff)
}

// RateLimiter provides simple rate limiting.
type RateLimiter struct {
	interval time.Duration
	lastCall time.Time
}

// NewRateLimiter creates a rate limiter with the specified minimum interval.
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{
		interval: interval,
	}
}

// Wait blocks until it's safe to make another request.
func (r *RateLimiter) Wait(ctx context.Context) error {
	elapsed := time.Since(r.lastCall)
	if elapsed < r.interval {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.interval - elapsed):
		}
	}
	r.lastCall = time.Now()
	return nil
}
