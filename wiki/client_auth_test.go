package wiki

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestLogin_TokenViaGETAndSessionCookieReused mirrors wikis that reject a
// login token obtained by POST with NeedToken.
func TestLogin_TokenViaGETAndSessionCookieReused(t *testing.T) {
	var loginSawCookie, tokenViaGET bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case q.Get("meta") == "tokens" && q.Get("type") == "login":
			tokenViaGET = r.Method == http.MethodGet
			http.SetCookie(w, &http.Cookie{Name: "wikidb_session", Value: "s1", Path: "/"})
			_, _ = w.Write([]byte(`{"query":{"tokens":{"logintoken":"tok+\\"}}}`))
		case strings.Contains(string(body), "meta=tokens") && strings.Contains(string(body), "type=login"):
			_, _ = w.Write([]byte(`{"query":{"tokens":{"logintoken":"posted+\\"}}}`))
		case strings.Contains(string(body), "action=login"):
			if r.Method != http.MethodPost || strings.Contains(r.URL.RawQuery, "lgpassword") {
				t.Errorf("credentials must be sent in a POST body")
			}
			ck, err := r.Cookie("wikidb_session")
			loginSawCookie = err == nil && ck.Value == "s1"
			if loginSawCookie && tokenViaGET {
				_, _ = w.Write([]byte(`{"login":{"result":"Success"}}`))
			} else {
				_, _ = w.Write([]byte(`{"login":{"result":"NeedToken"}}`))
			}
		default:
			_, _ = w.Write([]byte(`{"query":{"userinfo":{"id":0,"name":"anon"}}}`))
		}
	}))
	defer srv.Close()

	c := NewClient(&Config{
		BaseURL:    srv.URL + "/w/api.php",
		Username:   "User@bot",
		Password:   "secret",
		Timeout:    5 * time.Second,
		UserAgent:  "test",
		MaxRetries: 0,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer c.Close()

	if err := c.login(context.Background()); err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if !tokenViaGET {
		t.Error("login token was not requested with GET")
	}
	if !loginSawCookie {
		t.Error("login POST did not carry the session cookie from the token request")
	}
}

func TestCheckLoginResult_NoHintForNeedToken(t *testing.T) {
	c := &Client{config: &Config{Username: "TestUser"}}
	err := c.checkLoginResult(map[string]interface{}{"result": "NeedToken"})
	if err == nil || strings.Contains(err.Error(), "Special:BotPasswords") {
		t.Errorf("NeedToken must not get a username hint: %v", err)
	}
}

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
		{"user at bot name", "Ab12345@bot_one", false},
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
