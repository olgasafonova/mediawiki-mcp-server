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
	Password string // #nosec G117 -- config field name, not a hardcoded secret

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
	baseURL, err := loadBaseURLFromEnv()
	if err != nil {
		return nil, err
	}

	timeout, err := loadTimeoutFromEnv()
	if err != nil {
		return nil, err
	}

	maxRetries, err := loadMaxRetriesFromEnv()
	if err != nil {
		return nil, err
	}

	return &Config{
		BaseURL:    baseURL,
		Username:   os.Getenv("MEDIAWIKI_USERNAME"),
		Password:   os.Getenv("MEDIAWIKI_PASSWORD"),
		Timeout:    timeout,
		UserAgent:  loadUserAgentFromEnv(),
		MaxRetries: maxRetries,
	}, nil
}

// loadBaseURLFromEnv reads and validates MEDIAWIKI_URL, enforcing HTTPS unless
// MEDIAWIKI_ALLOW_INSECURE=true.
func loadBaseURLFromEnv() (string, error) {
	baseURL := os.Getenv("MEDIAWIKI_URL")
	if baseURL == "" {
		return "", &ConfigError{
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

	// Validate URL format and enforce HTTPS (unless MEDIAWIKI_ALLOW_INSECURE=true)
	allowInsecure, _ := strconv.ParseBool(os.Getenv("MEDIAWIKI_ALLOW_INSECURE"))
	if err := validateWikiURL(baseURL, allowInsecure); err != nil {
		return "", err
	}
	return baseURL, nil
}

// loadTimeoutFromEnv reads MEDIAWIKI_TIMEOUT, defaulting to 30 seconds.
func loadTimeoutFromEnv() (time.Duration, error) {
	t := os.Getenv("MEDIAWIKI_TIMEOUT")
	if t == "" {
		return 30 * time.Second, nil
	}
	d, err := time.ParseDuration(t)
	if err != nil {
		return 0, &ConfigError{
			Field:   "MEDIAWIKI_TIMEOUT",
			Message: fmt.Sprintf("invalid duration format: %q", t),
			Suggestion: `Use a valid Go duration string.

Examples:
  export MEDIAWIKI_TIMEOUT="30s"   # 30 seconds
  export MEDIAWIKI_TIMEOUT="2m"    # 2 minutes
  export MEDIAWIKI_TIMEOUT="1m30s" # 1 minute 30 seconds`,
		}
	}
	return d, nil
}

// loadMaxRetriesFromEnv reads MEDIAWIKI_MAX_RETRIES, defaulting to 3.
func loadMaxRetriesFromEnv() (int, error) {
	r := os.Getenv("MEDIAWIKI_MAX_RETRIES")
	if r == "" {
		return 3, nil
	}
	n, err := strconv.Atoi(r)
	if err != nil || n < 0 {
		return 0, &ConfigError{
			Field:   "MEDIAWIKI_MAX_RETRIES",
			Message: fmt.Sprintf("must be a non-negative integer, got: %q", r),
			Suggestion: `Set a non-negative integer for retry attempts.

Examples:
  export MEDIAWIKI_MAX_RETRIES="3"  # Default: 3 retries
  export MEDIAWIKI_MAX_RETRIES="0"  # No retries
  export MEDIAWIKI_MAX_RETRIES="5"  # 5 retries`,
		}
	}
	return n, nil
}

// loadUserAgentFromEnv reads MEDIAWIKI_USER_AGENT, falling back to the default.
func loadUserAgentFromEnv() string {
	if userAgent := os.Getenv("MEDIAWIKI_USER_AGENT"); userAgent != "" {
		return userAgent
	}
	return "MediaWikiMCPServer/1.0 (https://github.com/olgasafonova/mediawiki-mcp-server)"
}

// validateWikiURLScheme accepts only https, or http when allowInsecure is true.
// Without opt-in, any non-https scheme returns the HTTPS-required error — that
// covers the common case of plain hostnames where url.Parse leaves the scheme
// empty. With opt-in, schemes other than http/https still get rejected.
func validateWikiURLScheme(rawURL, scheme string, allowInsecure bool) error {
	switch {
	case scheme == "https":
		return nil
	case allowInsecure && scheme == "http":
		return nil
	case allowInsecure:
		return &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: fmt.Sprintf("URL scheme must be http or https (got %q)", scheme),
			Suggestion: `Provide a valid URL to your wiki's API endpoint.

Example:
  export MEDIAWIKI_URL="https://wiki.example.com/api.php"`,
		}
	default:
		return &ConfigError{
			Field:   "MEDIAWIKI_URL",
			Message: fmt.Sprintf("URL must use HTTPS for security (got %q scheme)", scheme),
			Suggestion: `Change the URL scheme from "http" to "https".

Your URL: ` + rawURL + `
Fixed:    ` + strings.Replace(rawURL, "http://", "https://", 1) + `

If your wiki doesn't support HTTPS, set MEDIAWIKI_ALLOW_INSECURE=true
(not recommended for production use).`,
		}
	}
}

// validateWikiURL validates the wiki URL format and enforces HTTPS unless allowInsecure is set
func validateWikiURL(rawURL string, allowInsecure bool) error {
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

	if err := validateWikiURLScheme(rawURL, parsed.Scheme, allowInsecure); err != nil {
		return err
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

// IsConfigured returns true if the wiki URL is set and the server can make API calls.
// When false, tool listing works but tool calls return an error prompting the user to configure MEDIAWIKI_URL.
func (c *Config) IsConfigured() bool {
	return c.BaseURL != ""
}

// HasCredentials returns true if authentication credentials are configured
func (c *Config) HasCredentials() bool {
	return c.Username != "" && c.Password != ""
}

// LoadConfigOrUnconfigured loads configuration from environment variables.
// Unlike LoadConfig, it does not fail when MEDIAWIKI_URL is missing; instead it returns
// an unconfigured Config that allows tool registration but rejects tool calls.
// Other invalid config (bad URL format, bad timeout) still returns an error.
func LoadConfigOrUnconfigured() (*Config, error) {
	config, err := LoadConfig()
	if err == nil {
		return config, nil
	}

	// If the only problem is a missing URL, return an unconfigured config
	if isMissingURLError(err) {
		return &Config{
			Timeout:    30 * time.Second,
			UserAgent:  "MediaWikiMCPServer/1.0 (https://github.com/olgasafonova/mediawiki-mcp-server)",
			MaxRetries: 3,
		}, nil
	}

	return nil, err
}

// isMissingURLError reports whether the error is the ConfigError raised when
// MEDIAWIKI_URL is not set at all (as opposed to set but invalid).
func isMissingURLError(err error) bool {
	configErr, ok := err.(*ConfigError)
	if !ok {
		return false
	}
	return configErr.Field == "MEDIAWIKI_URL" && configErr.Message == "environment variable is required but not set"
}
