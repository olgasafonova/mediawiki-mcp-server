package wiki

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SessionState captures the authentication state that a CLI process can
// persist across invocations to avoid re-logging in on every command.
// Includes the cookies for the configured wiki API URL plus the in-memory
// login + token-expiry flags so EnsureLoggedIn can short-circuit on the
// next process startup.
type SessionState struct {
	Cookies     []*SessionCookie `json:"cookies"`
	LoggedIn    bool             `json:"logged_in"`
	TokenExpiry time.Time        `json:"token_expiry"`
	SavedAt     time.Time        `json:"saved_at"`
}

// SessionCookie is a JSON-safe subset of http.Cookie. The standard
// http.Cookie has fields that don't round-trip cleanly through JSON
// (Unparsed, Raw); this struct keeps only what's needed for replay.
type SessionCookie struct {
	Name    string    `json:"name"`
	Value   string    `json:"value"`
	Path    string    `json:"path,omitempty"`
	Domain  string    `json:"domain,omitempty"`
	Expires time.Time `json:"expires,omitempty"`
	Secure  bool      `json:"secure,omitempty"`
}

// SessionSnapshot returns the current session state for the configured
// wiki API URL. Returns an error only if the API URL cannot be parsed.
func (c *Client) SessionSnapshot() (SessionState, error) {
	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return SessionState{}, fmt.Errorf("invalid wiki URL: %w", err)
	}

	c.mu.RLock()
	loggedIn := c.loggedIn
	tokenExpiry := c.tokenExpiry
	c.mu.RUnlock()

	jar := c.httpClient.Jar
	if jar == nil {
		return SessionState{
			LoggedIn:    loggedIn,
			TokenExpiry: tokenExpiry,
			SavedAt:     time.Now(),
		}, nil
	}

	raw := jar.Cookies(u)
	cookies := make([]*SessionCookie, 0, len(raw))
	for _, ck := range raw {
		cookies = append(cookies, &SessionCookie{
			Name:    ck.Name,
			Value:   ck.Value,
			Path:    ck.Path,
			Domain:  ck.Domain,
			Expires: ck.Expires,
			Secure:  ck.Secure,
		})
	}

	return SessionState{
		Cookies:     cookies,
		LoggedIn:    loggedIn,
		TokenExpiry: tokenExpiry,
		SavedAt:     time.Now(),
	}, nil
}

// RestoreSession populates the cookie jar and auth flags from a previously
// captured snapshot. The session is only considered live if the token
// expiry is still in the future; otherwise login state is left as
// not-logged-in and the next EnsureLoggedIn call will perform a fresh login.
func (c *Client) RestoreSession(s SessionState) error {
	u, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid wiki URL: %w", err)
	}

	if c.httpClient.Jar != nil && len(s.Cookies) > 0 {
		raw := make([]*http.Cookie, 0, len(s.Cookies))
		for _, ck := range s.Cookies {
			//nolint:gosec // G124: HttpOnly/SameSite are server-issued attributes; we replay client-side jar cookies as-is.
			raw = append(raw, &http.Cookie{
				Name:    ck.Name,
				Value:   ck.Value,
				Path:    ck.Path,
				Domain:  ck.Domain,
				Expires: ck.Expires,
				Secure:  ck.Secure,
			})
		}
		c.httpClient.Jar.SetCookies(u, raw)
	}

	if s.LoggedIn && time.Now().Before(s.TokenExpiry) {
		c.mu.Lock()
		c.loggedIn = true
		c.tokenExpiry = s.TokenExpiry
		c.mu.Unlock()
	}

	return nil
}
