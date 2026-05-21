package wiki

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"
)

// TestSessionSnapshotEmpty verifies SessionSnapshot on a fresh client.
// No cookies, no auth state — fields should be zero values plus a fresh SavedAt.
func TestSessionSnapshotEmpty(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()

	before := time.Now()
	snap, err := c.SessionSnapshot()
	after := time.Now()
	if err != nil {
		t.Fatalf("SessionSnapshot returned error: %v", err)
	}

	if len(snap.Cookies) != 0 {
		t.Errorf("Expected 0 cookies on fresh client, got %d", len(snap.Cookies))
	}
	if snap.LoggedIn {
		t.Error("Expected LoggedIn=false on fresh client")
	}
	if !snap.TokenExpiry.IsZero() {
		t.Errorf("Expected zero TokenExpiry, got %v", snap.TokenExpiry)
	}
	if snap.SavedAt.Before(before) || snap.SavedAt.After(after) {
		t.Errorf("SavedAt %v not in [%v, %v]", snap.SavedAt, before, after)
	}
}

// TestSessionSnapshotInvalidBaseURL ensures SessionSnapshot reports an
// error for an unparseable wiki URL. The net/url package is liberal about
// what it accepts, so this case uses a control character that triggers
// a parse failure.
func TestSessionSnapshotInvalidBaseURL(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()
	c.config.BaseURL = "http://example.com/\x7f"

	_, err := c.SessionSnapshot()
	if err == nil {
		t.Fatal("Expected error from invalid BaseURL, got nil")
	}
}

// TestSessionRoundTrip captures the full snapshot → restore cycle.
//
// Setup: client with two cookies in its jar plus loggedIn=true and a
// future token expiry. Take a snapshot, then restore it into a fresh
// client and verify the cookies and auth flags transferred.
func TestSessionRoundTrip(t *testing.T) {
	src := createTestClient(t)
	defer src.Close()

	wikiURL, _ := url.Parse(src.config.BaseURL)
	src.httpClient.Jar.SetCookies(wikiURL, []*http.Cookie{
		{Name: "session_id", Value: "abc123", Path: "/", Secure: true},
		{Name: "csrf", Value: "tok456", Path: "/"},
	})

	expiry := time.Now().Add(30 * time.Minute)
	src.mu.Lock()
	src.loggedIn = true
	src.tokenExpiry = expiry
	src.mu.Unlock()

	snap, err := src.SessionSnapshot()
	if err != nil {
		t.Fatalf("SessionSnapshot: %v", err)
	}
	if len(snap.Cookies) != 2 {
		t.Fatalf("Expected 2 cookies in snapshot, got %d", len(snap.Cookies))
	}
	if !snap.LoggedIn || !snap.TokenExpiry.Equal(expiry) {
		t.Errorf("Snapshot auth state wrong: loggedIn=%v expiry=%v", snap.LoggedIn, snap.TokenExpiry)
	}

	dst := createTestClient(t)
	defer dst.Close()
	if err := dst.RestoreSession(snap); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}

	dstWikiURL, _ := url.Parse(dst.config.BaseURL)
	restoredCookies := dst.httpClient.Jar.Cookies(dstWikiURL)
	if len(restoredCookies) != 2 {
		t.Fatalf("Expected 2 cookies after restore, got %d", len(restoredCookies))
	}

	byName := make(map[string]string, len(restoredCookies))
	for _, ck := range restoredCookies {
		byName[ck.Name] = ck.Value
	}
	if byName["session_id"] != "abc123" || byName["csrf"] != "tok456" {
		t.Errorf("Cookie values not preserved: %+v", byName)
	}

	dst.mu.RLock()
	gotLogged := dst.loggedIn
	gotExpiry := dst.tokenExpiry
	dst.mu.RUnlock()
	if !gotLogged || !gotExpiry.Equal(expiry) {
		t.Errorf("Auth state not restored: loggedIn=%v expiry=%v want=%v", gotLogged, gotExpiry, expiry)
	}
}

// TestRestoreSessionExpiredTokenIgnoresAuth verifies that a snapshot with
// an expired token does NOT mark the client as logged in. Cookies still
// transfer (the jar replays them; the wiki decides if they're stale), but
// the auth flag stays false so the next EnsureLoggedIn forces a fresh login.
func TestRestoreSessionExpiredTokenIgnoresAuth(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()

	snap := SessionState{
		Cookies: []*SessionCookie{
			{Name: "session_id", Value: "stale", Path: "/"},
		},
		LoggedIn:    true,
		TokenExpiry: time.Now().Add(-1 * time.Hour),
		SavedAt:     time.Now().Add(-2 * time.Hour),
	}

	if err := c.RestoreSession(snap); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}

	c.mu.RLock()
	gotLogged := c.loggedIn
	c.mu.RUnlock()
	if gotLogged {
		t.Error("Expected loggedIn=false after restoring expired snapshot")
	}

	wikiURL, _ := url.Parse(c.config.BaseURL)
	if len(c.httpClient.Jar.Cookies(wikiURL)) != 1 {
		t.Error("Expected cookies to still transfer despite expired token")
	}
}

// TestSessionSnapshotNoJar covers the defensive nil-jar branch in
// SessionSnapshot. Construct a client with Jar=nil and ensure the
// snapshot returns auth state without panicking.
func TestSessionSnapshotNoJar(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()
	c.httpClient.Jar = nil

	c.mu.Lock()
	c.loggedIn = true
	c.tokenExpiry = time.Now().Add(10 * time.Minute)
	c.mu.Unlock()

	snap, err := c.SessionSnapshot()
	if err != nil {
		t.Fatalf("SessionSnapshot: %v", err)
	}
	if !snap.LoggedIn {
		t.Error("Expected LoggedIn=true")
	}
	if len(snap.Cookies) != 0 {
		t.Errorf("Expected empty cookies (nil jar), got %d", len(snap.Cookies))
	}
}

// TestRestoreSessionInvalidBaseURL ensures RestoreSession reports an
// error for an unparseable wiki URL rather than panicking when it tries
// to set cookies for a nil URL target.
func TestRestoreSessionInvalidBaseURL(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()
	c.config.BaseURL = "http://example.com/\x7f"

	err := c.RestoreSession(SessionState{
		Cookies: []*SessionCookie{{Name: "x", Value: "y"}},
	})
	if err == nil {
		t.Fatal("Expected error from invalid BaseURL, got nil")
	}
}

// TestRestoreSessionEmptySnapshotSafe verifies that restoring an empty
// snapshot (zero state) leaves the client unchanged and does not error.
func TestRestoreSessionEmptySnapshotSafe(t *testing.T) {
	c := createTestClient(t)
	defer c.Close()

	if err := c.RestoreSession(SessionState{}); err != nil {
		t.Fatalf("RestoreSession on empty snapshot: %v", err)
	}

	c.mu.RLock()
	gotLogged := c.loggedIn
	c.mu.RUnlock()
	if gotLogged {
		t.Error("Empty restore should not set loggedIn=true")
	}
}

// Ensure cookiejar (and therefore client) is reachable in tests; touched
// here to keep the import inventory honest if a future refactor drops
// the indirect use through createTestClient.
var _ = cookiejar.New
