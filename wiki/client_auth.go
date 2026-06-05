package wiki

import (
	"context"
	"fmt"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

func (c *Client) isLoggedIn() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loggedIn
}

// SetAuditLogger configures an audit logger for tracking write operations.
// Pass nil to disable audit logging. The audit logger records page edits,
func (c *Client) checkExistingSession(ctx context.Context) bool {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "userinfo")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return false
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return false
	}

	userinfo, ok := query["userinfo"].(map[string]interface{})
	if !ok {
		return false
	}

	// If user ID is 0, we're not logged in (anonymous)
	userID, ok := userinfo["id"].(float64)
	if !ok || userID == 0 {
		return false
	}

	// We have a valid session
	name, _ := userinfo["name"].(string)
	c.logger.Debug("Found existing session", "user", name, "id", int(userID))
	return true
}

func (c *Client) resetCookies() {
	jar, _ := cookiejar.New(nil)
	c.httpClient.Jar = jar
	c.loggedIn = false
	c.csrfToken = ""
	c.tokenExpiry = time.Time{}
	c.logger.Debug("Cookies reset for fresh login")
}

func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggedIn && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	if !c.config.HasCredentials() {
		return fmt.Errorf("no credentials configured. Set MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD environment variables")
	}

	// Check if we already have a valid session from cookies
	// This prevents the "Cannot log in when using BotPasswordSessionProvider" error
	if c.checkExistingSession(ctx) {
		c.loggedIn = true
		c.tokenExpiry = time.Now().Add(60 * time.Minute) // Trust the existing session longer
		c.logger.Info("Using existing session")
		return nil
	}

	// Get login token
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "login")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to get login token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no tokens in response")
	}

	loginToken, ok := tokens["logintoken"].(string)
	if !ok {
		return fmt.Errorf("no login token in response")
	}

	// Perform login
	params = url.Values{}
	params.Set("action", "login")
	params.Set("lgname", c.config.Username)
	params.Set("lgpassword", c.config.Password)
	params.Set("lgtoken", loginToken)

	resp, err = c.apiRequest(ctx, params)
	if err != nil {
		// Check for BotPasswordSessionProvider error and retry with fresh cookies
		if strings.Contains(err.Error(), "BotPasswordSessionProvider") {
			c.logger.Warn("BotPasswordSessionProvider conflict detected, resetting cookies")
			c.resetCookies()
			// Retry login once with fresh cookies
			return c.loginFresh(ctx)
		}
		return fmt.Errorf("login failed: %w", err)
	}

	login, ok := resp["login"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected login response")
	}

	result, ok := login["result"].(string)
	if !ok {
		return fmt.Errorf("login result not a string")
	}
	if result != "Success" {
		reason := login["reason"]
		// Check for BotPasswordSessionProvider in the reason
		if reason != nil {
			reasonStr := fmt.Sprintf("%v", reason)
			if strings.Contains(reasonStr, "BotPasswordSessionProvider") {
				c.logger.Warn("BotPasswordSessionProvider conflict in login result, resetting cookies")
				c.resetCookies()
				return c.loginFresh(ctx)
			}
			return fmt.Errorf("login failed: %s - %v", result, reason)
		}
		return fmt.Errorf("login failed: %s", result)
	}

	c.loggedIn = true
	c.tokenExpiry = time.Now().Add(60 * time.Minute) // Extended from 20 to 60 minutes

	c.logger.Info("Successfully logged in", "username", c.config.Username)

	return nil
}

func (c *Client) loginFresh(ctx context.Context) error {
	// Get login token
	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "login")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to get login token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response format")
	}

	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no tokens in response")
	}

	loginToken, ok := tokens["logintoken"].(string)
	if !ok {
		return fmt.Errorf("no login token in response")
	}

	// Perform login
	params = url.Values{}
	params.Set("action", "login")
	params.Set("lgname", c.config.Username)
	params.Set("lgpassword", c.config.Password)
	params.Set("lgtoken", loginToken)

	resp, err = c.apiRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	login, ok := resp["login"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected login response")
	}

	result, ok := login["result"].(string)
	if !ok {
		return fmt.Errorf("login result not a string")
	}
	if result != "Success" {
		reason := login["reason"]
		if reason != nil {
			return fmt.Errorf("login failed: %s - %v", result, reason)
		}
		return fmt.Errorf("login failed: %s", result)
	}

	c.loggedIn = true
	c.tokenExpiry = time.Now().Add(60 * time.Minute)

	c.logger.Info("Successfully logged in with fresh session", "username", c.config.Username)

	return nil
}

func (c *Client) getCSRFToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.csrfToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.csrfToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	if err := c.login(ctx); err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "tokens")
	params.Set("type", "csrf")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to get CSRF token: %w", err)
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing query")
	}
	tokens, ok := query["tokens"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing tokens")
	}
	csrfToken, ok := tokens["csrftoken"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format: missing csrftoken")
	}

	c.mu.Lock()
	c.csrfToken = csrfToken
	c.tokenExpiry = time.Now().Add(60 * time.Minute)
	c.mu.Unlock()

	return csrfToken, nil
}

// invalidateCSRFToken clears the cached CSRF token so the next write
// operation fetches a fresh one. MediaWiki can invalidate CSRF tokens
func (c *Client) invalidateCSRFToken() {
	c.mu.Lock()
	c.csrfToken = ""
	c.mu.Unlock()
}

func (c *Client) EnsureLoggedIn(ctx context.Context) error {
	// Anonymous access: no credentials configured, skip authentication.
	// Public wikis allow read operations without login.
	if !c.config.HasCredentials() {
		return nil
	}

	c.mu.RLock()
	loggedIn := c.loggedIn && time.Now().Before(c.tokenExpiry)
	c.mu.RUnlock()

	if loggedIn {
		return nil
	}

	return c.login(ctx)
}
