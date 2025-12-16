package wiki

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds MediaWiki connection settings
type Config struct {
	// BaseURL is the wiki API endpoint (e.g., https://wiki.example.com/api.php)
	BaseURL string

	// Username for bot password authentication (optional, for editing)
	Username string

	// Password for bot password authentication (optional, for editing)
	Password string

	// Timeout for API requests
	Timeout time.Duration

	// UserAgent identifies the client to the wiki
	UserAgent string

	// MaxRetries for failed requests
	MaxRetries int
}

// ConfigError provides detailed configuration errors with recovery suggestions
type ConfigError struct {
	Field      string
	Message    string
	Suggestion string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %s\n\nTo fix this:\n%s", e.Field, e.Message, e.Suggestion)
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	baseURL := os.Getenv("MEDIAWIKI_URL")
	if baseURL == "" {
		return nil, &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: "environment variable is required but not set",
			Suggestion: `Set the MEDIAWIKI_URL environment variable to your wiki's API endpoint.

Example:
  export MEDIAWIKI_URL="https://wiki.example.com/api.php"

Or in your MCP configuration:
  "env": {
    "MEDIAWIKI_URL": "https://wiki.example.com/api.php"
  }`,
		}
	}

	// Validate URL format and enforce HTTPS
	if err := validateWikiURL(baseURL); err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	if t := os.Getenv("MEDIAWIKI_TIMEOUT"); t != "" {
		d, err := time.ParseDuration(t)
		if err != nil {
			return nil, &ConfigError{
				Field:   "MEDIAWIKI_TIMEOUT",
				Message: fmt.Sprintf("invalid duration format: %q", t),
				Suggestion: `Use a valid Go duration string.

Examples:
  export MEDIAWIKI_TIMEOUT="30s"   # 30 seconds
  export MEDIAWIKI_TIMEOUT="2m"    # 2 minutes
  export MEDIAWIKI_TIMEOUT="1m30s" # 1 minute 30 seconds`,
			}
		}
		timeout = d
	}

	maxRetries := 3
	if r := os.Getenv("MEDIAWIKI_MAX_RETRIES"); r != "" {
		n, err := strconv.Atoi(r)
		if err != nil || n < 0 {
			return nil, &ConfigError{
				Field:   "MEDIAWIKI_MAX_RETRIES",
				Message: fmt.Sprintf("must be a non-negative integer, got: %q", r),
				Suggestion: `Set a non-negative integer for retry attempts.

Examples:
  export MEDIAWIKI_MAX_RETRIES="3"  # Default: 3 retries
  export MEDIAWIKI_MAX_RETRIES="0"  # No retries
  export MEDIAWIKI_MAX_RETRIES="5"  # 5 retries`,
			}
		}
		maxRetries = n
	}

	userAgent := os.Getenv("MEDIAWIKI_USER_AGENT")
	if userAgent == "" {
		userAgent = "MediaWikiMCPServer/1.0 (https://github.com/olgasafonova/mediawiki-mcp-server)"
	}

	return &Config{
		BaseURL:    baseURL,
		Username:   os.Getenv("MEDIAWIKI_USERNAME"),
		Password:   os.Getenv("MEDIAWIKI_PASSWORD"),
		Timeout:    timeout,
		UserAgent:  userAgent,
		MaxRetries: maxRetries,
	}, nil
}

// validateWikiURL validates the wiki URL format and enforces HTTPS
func validateWikiURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: fmt.Sprintf("invalid URL format: %v", err),
			Suggestion: `Provide a valid URL to your wiki's API endpoint.

Example:
  export MEDIAWIKI_URL="https://wiki.example.com/api.php"`,
		}
	}

	// Enforce HTTPS for security (credentials are transmitted)
	if parsed.Scheme != "https" {
		return &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: fmt.Sprintf("URL must use HTTPS for security (got %q scheme)", parsed.Scheme),
			Suggestion: `Change the URL scheme from "http" to "https".

Your URL: ` + rawURL + `
Fixed:    ` + strings.Replace(rawURL, "http://", "https://", 1) + `

If your wiki doesn't support HTTPS, set MEDIAWIKI_ALLOW_INSECURE=true
(not recommended for production use).`,
		}
	}

	// Check for api.php endpoint
	if !strings.HasSuffix(parsed.Path, "api.php") && !strings.Contains(parsed.Path, "api.php") {
		return &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: "URL should point to the MediaWiki API endpoint (api.php)",
			Suggestion: `The URL should end with "api.php".

Your URL: ` + rawURL + `
Example:  https://wiki.example.com/api.php
          https://wiki.example.com/w/api.php`,
		}
	}

	return nil
}

// HasCredentials returns true if authentication credentials are configured
func (c *Config) HasCredentials() bool {
	return c.Username != "" && c.Password != ""
}
