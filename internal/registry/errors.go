// Package registry error types for Nordic registries.
package registry

import (
	"errors"
	"fmt"
)

// Common errors.
var (
	ErrNotFound       = errors.New("company not found")
	ErrInvalidOrgNum  = errors.New("invalid organization number")
	ErrRateLimited    = errors.New("rate limited by API")
	ErrServiceDown    = errors.New("registry service unavailable")
	ErrUnauthorized   = errors.New("unauthorized access")
	ErrInvalidCountry = errors.New("invalid or unsupported country")
)

// APIError represents an error from a registry API.
type APIError struct {
	Registry   string
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s API error (status %d): %s: %v", e.Registry, e.StatusCode, e.Message, e.Err)
	}
	return fmt.Sprintf("%s API error (status %d): %s", e.Registry, e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// IsNotFound checks if the error indicates a company was not found.
func IsNotFound(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}

// IsRateLimited checks if the error indicates rate limiting.
func IsRateLimited(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429
	}
	return false
}

// IsRetryable checks if the error is retryable.
func IsRetryable(err error) bool {
	if IsRateLimited(err) {
		return true
	}
	if errors.Is(err, ErrServiceDown) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// NewNotFoundError creates a not found error for a specific org number.
func NewNotFoundError(registry, orgNumber string) error {
	return &APIError{
		Registry:   registry,
		StatusCode: 404,
		Message:    fmt.Sprintf("company %s not found", orgNumber),
		Err:        ErrNotFound,
	}
}

// NewAPIError creates a new API error.
func NewAPIError(registry string, statusCode int, message string) *APIError {
	return &APIError{
		Registry:   registry,
		StatusCode: statusCode,
		Message:    message,
	}
}
