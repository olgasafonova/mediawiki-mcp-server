package wiki

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_RequiredURL(t *testing.T) {
	// Clear environment
	_ = os.Unsetenv("MEDIAWIKI_URL")
	_ = os.Unsetenv("MEDIAWIKI_USERNAME")
	_ = os.Unsetenv("MEDIAWIKI_PASSWORD")
	_ = os.Unsetenv("MEDIAWIKI_TIMEOUT")
	_ = os.Unsetenv("MEDIAWIKI_MAX_RETRIES")
	_ = os.Unsetenv("MEDIAWIKI_USER_AGENT")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when MEDIAWIKI_URL is missing")
	}
	// Check that error is a ConfigError with helpful message
	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
	if configErr.Field != "MEDIAWIKI_URL" {
		t.Errorf("Expected field MEDIAWIKI_URL, got %s", configErr.Field)
	}
}

func TestLoadConfig_ValidConfig(t *testing.T) {
	_ = os.Setenv("MEDIAWIKI_URL", "https://wiki.example.com/api.php")
	_ = os.Setenv("MEDIAWIKI_USERNAME", "TestUser")
	_ = os.Setenv("MEDIAWIKI_PASSWORD", "TestPassword")
	_ = os.Setenv("MEDIAWIKI_TIMEOUT", "60s")
	_ = os.Setenv("MEDIAWIKI_MAX_RETRIES", "5")
	_ = os.Setenv("MEDIAWIKI_USER_AGENT", "TestAgent/1.0")
	defer func() {
		_ = os.Unsetenv("MEDIAWIKI_URL")
		_ = os.Unsetenv("MEDIAWIKI_USERNAME")
		_ = os.Unsetenv("MEDIAWIKI_PASSWORD")
		_ = os.Unsetenv("MEDIAWIKI_TIMEOUT")
		_ = os.Unsetenv("MEDIAWIKI_MAX_RETRIES")
		_ = os.Unsetenv("MEDIAWIKI_USER_AGENT")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://wiki.example.com/api.php" {
		t.Errorf("Expected BaseURL 'https://wiki.example.com/api.php', got '%s'", cfg.BaseURL)
	}
	if cfg.Username != "TestUser" {
		t.Errorf("Expected Username 'TestUser', got '%s'", cfg.Username)
	}
	if cfg.Password != "TestPassword" {
		t.Errorf("Expected Password 'TestPassword', got '%s'", cfg.Password)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", cfg.Timeout)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", cfg.MaxRetries)
	}
	if cfg.UserAgent != "TestAgent/1.0" {
		t.Errorf("Expected UserAgent 'TestAgent/1.0', got '%s'", cfg.UserAgent)
	}
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	_ = os.Setenv("MEDIAWIKI_URL", "https://wiki.example.com/api.php")
	_ = os.Unsetenv("MEDIAWIKI_USERNAME")
	_ = os.Unsetenv("MEDIAWIKI_PASSWORD")
	_ = os.Unsetenv("MEDIAWIKI_TIMEOUT")
	_ = os.Unsetenv("MEDIAWIKI_MAX_RETRIES")
	_ = os.Unsetenv("MEDIAWIKI_USER_AGENT")
	defer func() { _ = os.Unsetenv("MEDIAWIKI_URL") }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check defaults
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries 3, got %d", cfg.MaxRetries)
	}
	if cfg.UserAgent == "" {
		t.Error("Expected default UserAgent to be set")
	}
}

func TestLoadConfig_InvalidTimeout(t *testing.T) {
	_ = os.Setenv("MEDIAWIKI_URL", "https://wiki.example.com/api.php")
	_ = os.Setenv("MEDIAWIKI_TIMEOUT", "invalid")
	defer func() {
		_ = os.Unsetenv("MEDIAWIKI_URL")
		_ = os.Unsetenv("MEDIAWIKI_TIMEOUT")
	}()

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid timeout")
	}
	// Check that error is a ConfigError with helpful message
	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
	if configErr.Field != "MEDIAWIKI_TIMEOUT" {
		t.Errorf("Expected field MEDIAWIKI_TIMEOUT, got %s", configErr.Field)
	}
}

func TestLoadConfig_InvalidMaxRetries(t *testing.T) {
	_ = os.Setenv("MEDIAWIKI_URL", "https://wiki.example.com/api.php")
	_ = os.Setenv("MEDIAWIKI_MAX_RETRIES", "-1")
	defer func() {
		_ = os.Unsetenv("MEDIAWIKI_URL")
		_ = os.Unsetenv("MEDIAWIKI_MAX_RETRIES")
	}()

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for negative max retries")
	}
	// Check that error is a ConfigError with helpful message
	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
	if configErr.Field != "MEDIAWIKI_MAX_RETRIES" {
		t.Errorf("Expected field MEDIAWIKI_MAX_RETRIES, got %s", configErr.Field)
	}
}

func TestLoadConfig_NonNumericMaxRetries(t *testing.T) {
	_ = os.Setenv("MEDIAWIKI_URL", "https://wiki.example.com/api.php")
	_ = os.Setenv("MEDIAWIKI_MAX_RETRIES", "abc")
	defer func() {
		_ = os.Unsetenv("MEDIAWIKI_URL")
		_ = os.Unsetenv("MEDIAWIKI_MAX_RETRIES")
	}()

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for non-numeric max retries")
	}
	// Check that error is a ConfigError with helpful message
	configErr, ok := err.(*ConfigError)
	if !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
	if configErr.Field != "MEDIAWIKI_MAX_RETRIES" {
		t.Errorf("Expected field MEDIAWIKI_MAX_RETRIES, got %s", configErr.Field)
	}
}

func TestHasCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		expected bool
	}{
		{"Both set", "user", "pass", true},
		{"Only username", "user", "", false},
		{"Only password", "", "pass", false},
		{"Neither set", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Username: tt.username,
				Password: tt.password,
			}
			if cfg.HasCredentials() != tt.expected {
				t.Errorf("HasCredentials() = %v, expected %v", cfg.HasCredentials(), tt.expected)
			}
		})
	}
}
