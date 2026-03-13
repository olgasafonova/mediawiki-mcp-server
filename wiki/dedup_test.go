package wiki

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- RequestDeduplicator tests ---

func TestRequestDeduplicator_SingleRequest(t *testing.T) {
	d := NewRequestDeduplicator()

	result, shared, err := d.Do(context.Background(), "key1", func() (interface{}, error) {
		return "value1", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shared {
		t.Error("first request should not be shared")
	}
	if result != "value1" {
		t.Errorf("got %v, want value1", result)
	}
}

func TestRequestDeduplicator_ReturnsError(t *testing.T) {
	d := NewRequestDeduplicator()
	want := errors.New("boom")

	result, _, err := d.Do(context.Background(), "key1", func() (interface{}, error) {
		return nil, want
	})

	if !errors.Is(err, want) {
		t.Fatalf("got err %v, want %v", err, want)
	}
	if result != nil {
		t.Errorf("result should be nil on error, got %v", result)
	}
}

func TestRequestDeduplicator_CoalescesConcurrentRequests(t *testing.T) {
	d := NewRequestDeduplicator()
	var callCount atomic.Int32
	gate := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]interface{}, 5)
	shared := make([]bool, 5)
	errs := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], shared[idx], errs[idx] = d.Do(context.Background(), "same-key", func() (interface{}, error) {
				callCount.Add(1)
				<-gate // block until released
				return "shared-result", nil
			})
		}(i)
	}

	// Let goroutines start and coalesce
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	if count := callCount.Load(); count != 1 {
		t.Errorf("fn called %d times, want 1", count)
	}

	sharedCount := 0
	for i := 0; i < 5; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if results[i] != "shared-result" {
			t.Errorf("goroutine %d: got %v, want shared-result", i, results[i])
		}
		if shared[i] {
			sharedCount++
		}
	}
	if sharedCount != 4 {
		t.Errorf("got %d shared results, want 4 (one original + four shared)", sharedCount)
	}
}

func TestRequestDeduplicator_DifferentKeysRunIndependently(t *testing.T) {
	d := NewRequestDeduplicator()
	var callCount atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := string(rune('a' + idx))
			_, _, _ = d.Do(context.Background(), key, func() (interface{}, error) {
				callCount.Add(1)
				return idx, nil
			})
		}(i)
	}
	wg.Wait()

	if count := callCount.Load(); count != 3 {
		t.Errorf("fn called %d times, want 3 (one per key)", count)
	}
}

func TestRequestDeduplicator_ContextCancellation(t *testing.T) {
	d := NewRequestDeduplicator()
	gate := make(chan struct{})

	// Start a slow request
	go func() {
		_, _, _ = d.Do(context.Background(), "slow", func() (interface{}, error) {
			<-gate
			return "done", nil
		})
	}()
	time.Sleep(20 * time.Millisecond)

	// Second request with a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := d.Do(ctx, "slow", func() (interface{}, error) {
		t.Fatal("fn should not be called for canceled waiter")
		return nil, nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("got err %v, want context.Canceled", err)
	}

	close(gate)
}

func TestRequestDeduplicator_CleansUpAfterCompletion(t *testing.T) {
	d := NewRequestDeduplicator()

	_, _, _ = d.Do(context.Background(), "key1", func() (interface{}, error) {
		return "v", nil
	})

	if stats := d.Stats(); stats != 0 {
		t.Errorf("inflight count is %d after completion, want 0", stats)
	}
}

func TestRequestDeduplicator_StatsCountsInflight(t *testing.T) {
	d := NewRequestDeduplicator()
	gate := make(chan struct{})

	go func() {
		_, _, _ = d.Do(context.Background(), "a", func() (interface{}, error) {
			<-gate
			return nil, nil
		})
	}()
	go func() {
		_, _, _ = d.Do(context.Background(), "b", func() (interface{}, error) {
			<-gate
			return nil, nil
		})
	}()

	time.Sleep(30 * time.Millisecond)
	if stats := d.Stats(); stats != 2 {
		t.Errorf("inflight count is %d, want 2", stats)
	}

	close(gate)
	time.Sleep(30 * time.Millisecond)
	if stats := d.Stats(); stats != 0 {
		t.Errorf("inflight count is %d after completion, want 0", stats)
	}
}

// --- CircuitBreaker tests ---

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker()
	if s := cb.State(); s != CircuitClosed {
		t.Errorf("initial state is %v, want CircuitClosed", s)
	}
	if !cb.Allow() {
		t.Error("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(3, time.Minute, 1)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if s := cb.State(); s != CircuitOpen {
		t.Errorf("state is %v after 3 failures, want CircuitOpen", s)
	}
	if cb.Allow() {
		t.Error("open circuit should reject requests")
	}
}

func TestCircuitBreaker_StaysClosedBelowThreshold(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(5, time.Minute, 1)

	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}

	if s := cb.State(); s != CircuitClosed {
		t.Errorf("state is %v after 4 failures (threshold 5), want CircuitClosed", s)
	}
}

func TestCircuitBreaker_SuccessResetsFailCount(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(3, time.Minute, 1)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // resets
	cb.RecordFailure()
	cb.RecordFailure()

	if s := cb.State(); s != CircuitClosed {
		t.Errorf("state is %v, want CircuitClosed (success should reset counter)", s)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	if s := cb.State(); s != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %v", s)
	}

	time.Sleep(20 * time.Millisecond)

	if !cb.Allow() {
		t.Error("should allow request after reset timeout (half-open)")
	}
	if s := cb.State(); s != CircuitHalfOpen {
		t.Errorf("state is %v, want CircuitHalfOpen", s)
	}
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// First Allow() transitions open→half-open (doesn't increment halfOpenCount)
	if !cb.Allow() {
		t.Error("transition to half-open should allow request")
	}
	// halfOpenMax = 2, these two increment the counter
	if !cb.Allow() {
		t.Error("first half-open counted request should be allowed")
	}
	if !cb.Allow() {
		t.Error("second half-open counted request should be allowed")
	}
	if cb.Allow() {
		t.Error("should reject after halfOpenMax reached")
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	cb.Allow() // triggers half-open

	cb.RecordSuccess()

	if s := cb.State(); s != CircuitClosed {
		t.Errorf("state is %v, want CircuitClosed after half-open success", s)
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreakerWithConfig(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	cb.Allow() // triggers half-open

	cb.RecordFailure()

	if s := cb.State(); s != CircuitOpen {
		t.Errorf("state is %v, want CircuitOpen after half-open failure", s)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker()

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.State != "closed" {
		t.Errorf("stats state is %q, want closed", stats.State)
	}
	if stats.ConsecutiveFails != 2 {
		t.Errorf("consecutive fails is %d, want 2", stats.ConsecutiveFails)
	}
	if stats.LastFailure.IsZero() {
		t.Error("last failure should be set")
	}
}

func TestCircuitBreaker_DefaultThresholds(t *testing.T) {
	cb := NewCircuitBreaker()

	// Default threshold is 5
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}
	if s := cb.State(); s != CircuitClosed {
		t.Errorf("should stay closed at 4 failures, got %v", s)
	}

	cb.RecordFailure()
	if s := cb.State(); s != CircuitOpen {
		t.Errorf("should open at 5 failures, got %v", s)
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestErrCircuitOpen_Error(t *testing.T) {
	e := ErrCircuitOpen{
		State:    "open",
		RetryAt:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Failures: 5,
	}

	msg := e.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
	if !contains(msg, "circuit breaker is open") {
		t.Errorf("error message %q missing expected text", msg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
