package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

// sessionMaxAge bounds how long a cached session is trusted regardless of
// what the wiki sent in cookie expiry. Defensive cap: even if the wiki
// hands out a 30-day session cookie, we re-validate after this much time.
const sessionMaxAge = 12 * time.Hour

// sessionFilePerm is the file mode for the session store. 0600 so other
// users on a shared host cannot read someone else's wiki session cookies.
const sessionFilePerm = 0o600

// sessionCacheDisabled returns true when WIKI_NO_SESSION_CACHE is set to a
// non-empty value. Lets users opt out of disk caching (CI, ephemeral
// containers, hosts with no writable home dir).
func sessionCacheDisabled() bool {
	return os.Getenv("WIKI_NO_SESSION_CACHE") != ""
}

// sessionFilePath returns the on-disk path for the session store.
// Honors XDG_CONFIG_HOME, falling back to ~/.config.
func sessionFilePath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wiki", "sessions.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "wiki", "sessions.json"), nil
}

// urlKey hashes the wiki URL into a stable key. Avoids storing the raw URL
// in the JSON file (minor privacy improvement) and keys cleanly when the
// user works against multiple wikis.
func urlKey(wikiURL string) string {
	sum := sha256.Sum256([]byte(wikiURL))
	return hex.EncodeToString(sum[:])[:16]
}

// readSessionFile loads the full session map. Returns an empty map (not
// an error) if the file doesn't exist; only returns errors for malformed
// content or unreadable files.
func readSessionFile() (map[string]wiki.SessionState, error) {
	path, err := sessionFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path derived from home dir, not user input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]wiki.SessionState{}, nil
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var out map[string]wiki.SessionState
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}
	if out == nil {
		out = map[string]wiki.SessionState{}
	}
	return out, nil
}

// writeSessionFile atomically replaces the session file. Writes to a temp
// file in the same directory and renames; this avoids partial-write
// corruption if the process is killed mid-write.
func writeSessionFile(sessions map[string]wiki.SessionState) error {
	path, err := sessionFilePath()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return fmt.Errorf("create session dir: %w", mkErr)
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessions: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "sessions-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(sessionFilePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// loadCachedSession returns the cached session for the given wiki URL,
// or (zero, false) if no cached session exists or the cached one is too
// old per sessionMaxAge.
func loadCachedSession(wikiURL string) (wiki.SessionState, bool) {
	if sessionCacheDisabled() {
		return wiki.SessionState{}, false
	}
	sessions, err := readSessionFile()
	if err != nil {
		return wiki.SessionState{}, false
	}
	s, ok := sessions[urlKey(wikiURL)]
	if !ok {
		return wiki.SessionState{}, false
	}
	if time.Since(s.SavedAt) > sessionMaxAge {
		return wiki.SessionState{}, false
	}
	return s, true
}

// saveCachedSession persists the session for the given wiki URL. Best-effort:
// errors are returned but the caller typically logs and continues so a
// disk-cache failure doesn't fail the actual CLI operation.
func saveCachedSession(wikiURL string, s wiki.SessionState) error {
	if sessionCacheDisabled() {
		return nil
	}
	sessions, err := readSessionFile()
	if err != nil {
		sessions = map[string]wiki.SessionState{}
	}
	sessions[urlKey(wikiURL)] = s
	return writeSessionFile(sessions)
}
