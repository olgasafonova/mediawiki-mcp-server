package wiki

import (
	"strings"
	"testing"
)

func TestCheckLoginResult_BotUsernameHint(t *testing.T) {
	failed := map[string]interface{}{
		"result": "Failed",
		"reason": "The supplied credentials could not be authenticated.",
	}
	tests := []struct {
		name     string
		username string
		wantHint bool
	}{
		{"email without bot name", "Name@example.com", true},
		{"plain name without bot name", "TestUser", true},
		{"full bot login name", "Name@example.com#wiki-MCP", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{config: &Config{Username: tt.username}}
			err := c.checkLoginResult(failed)
			if err == nil {
				t.Fatal("expected error for failed login")
			}
			if !strings.Contains(err.Error(), "could not be authenticated") {
				t.Errorf("wiki reason missing from error: %v", err)
			}
			if got := strings.Contains(err.Error(), "Special:BotPasswords"); got != tt.wantHint {
				t.Errorf("hint present = %v, want %v: %v", got, tt.wantHint, err)
			}
		})
	}

	c := &Client{config: &Config{Username: "Name@example.com"}}
	if err := c.checkLoginResult(map[string]interface{}{"result": "Success"}); err != nil {
		t.Errorf("success must not error: %v", err)
	}
}

func TestCheckLoginResult_NoDoublePeriod(t *testing.T) {
	c := &Client{config: &Config{Username: "Name@example.com"}}
	err := c.checkLoginResult(map[string]interface{}{
		"result": "Failed",
		"reason": "The supplied credentials could not be authenticated.",
	})
	if err == nil || strings.Contains(err.Error(), "..") {
		t.Errorf("want single period before hint, got: %v", err)
	}
}
