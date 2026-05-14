package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestSessionFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("WIKI_NO_SESSION_CACHE", "")

	url := "https://example.org/api.php"
	want := wiki.SessionState{
		Cookies: []*wiki.SessionCookie{
			{Name: "MWUserID", Value: "42", Path: "/", Domain: "example.org"},
			{Name: "MWSession", Value: "abc123", Path: "/", Secure: true},
		},
		LoggedIn:    true,
		TokenExpiry: time.Now().Add(1 * time.Hour).Truncate(time.Second),
		SavedAt:     time.Now().Truncate(time.Second),
	}

	if err := saveCachedSession(url, want); err != nil {
		t.Fatalf("saveCachedSession: %v", err)
	}

	got, ok := loadCachedSession(url)
	if !ok {
		t.Fatal("loadCachedSession: expected hit, got miss")
	}
	if got.LoggedIn != want.LoggedIn {
		t.Errorf("LoggedIn: got %v want %v", got.LoggedIn, want.LoggedIn)
	}
	if !got.TokenExpiry.Equal(want.TokenExpiry) {
		t.Errorf("TokenExpiry: got %v want %v", got.TokenExpiry, want.TokenExpiry)
	}
	if len(got.Cookies) != 2 {
		t.Fatalf("Cookies: got %d want 2", len(got.Cookies))
	}
	if got.Cookies[0].Name != "MWUserID" || got.Cookies[0].Value != "42" {
		t.Errorf("first cookie: got %+v", got.Cookies[0])
	}
}

func TestSessionCacheDisabledBypass(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("WIKI_NO_SESSION_CACHE", "1")

	url := "https://example.org/api.php"
	if err := saveCachedSession(url, wiki.SessionState{LoggedIn: true, SavedAt: time.Now()}); err != nil {
		t.Fatalf("saveCachedSession: %v", err)
	}
	if _, ok := loadCachedSession(url); ok {
		t.Error("expected cache miss when WIKI_NO_SESSION_CACHE is set")
	}
}

func TestSessionStaleExpired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("WIKI_NO_SESSION_CACHE", "")

	url := "https://example.org/api.php"
	stale := wiki.SessionState{
		LoggedIn: true,
		SavedAt:  time.Now().Add(-sessionMaxAge - time.Minute),
	}
	if err := saveCachedSession(url, stale); err != nil {
		t.Fatalf("saveCachedSession: %v", err)
	}
	if _, ok := loadCachedSession(url); ok {
		t.Error("expected stale session to be rejected")
	}
}

func TestSessionFilePerm(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("WIKI_NO_SESSION_CACHE", "")

	url := "https://example.org/api.php"
	if err := saveCachedSession(url, wiki.SessionState{LoggedIn: true, SavedAt: time.Now()}); err != nil {
		t.Fatalf("saveCachedSession: %v", err)
	}
	path := filepath.Join(dir, "wiki", "sessions.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != sessionFilePerm {
		t.Errorf("session file perm: got %o want %o", mode, sessionFilePerm)
	}
}

func TestSessionPerWikiKeying(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("WIKI_NO_SESSION_CACHE", "")

	a := "https://en.wikipedia.org/api.php"
	b := "https://wiki.software-innovation.com/api.php"
	if err := saveCachedSession(a, wiki.SessionState{LoggedIn: true, SavedAt: time.Now(), Cookies: []*wiki.SessionCookie{{Name: "fromA", Value: "1"}}}); err != nil {
		t.Fatal(err)
	}
	if err := saveCachedSession(b, wiki.SessionState{LoggedIn: true, SavedAt: time.Now(), Cookies: []*wiki.SessionCookie{{Name: "fromB", Value: "1"}}}); err != nil {
		t.Fatal(err)
	}
	gotA, _ := loadCachedSession(a)
	gotB, _ := loadCachedSession(b)
	if len(gotA.Cookies) == 0 || gotA.Cookies[0].Name != "fromA" {
		t.Errorf("wiki A leaked into B: %+v", gotA.Cookies)
	}
	if len(gotB.Cookies) == 0 || gotB.Cookies[0].Name != "fromB" {
		t.Errorf("wiki B leaked into A: %+v", gotB.Cookies)
	}
}
