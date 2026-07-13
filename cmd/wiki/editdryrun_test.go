package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestEditDryRunJSONSkipsNetwork verifies that `wiki edit --dry-run --json`
// reports what would be written without creating a client (so it needs no
// credentials) and returns exit 0.
func TestEditDryRunJSONSkipsNetwork(t *testing.T) {
	// Ensure no credentials are present so a stray network/auth path would
	// fail loudly rather than pass by accident.
	t.Setenv("MEDIAWIKI_URL", "")
	t.Setenv("MEDIAWIKI_USERNAME", "")
	t.Setenv("MEDIAWIKI_PASSWORD", "")

	cmd := newEditCmd()
	// --json is a persistent flag on root in production; add it locally so
	// isJSON() resolves in this isolated command.
	cmd.Flags().Bool("json", false, "")
	cmd.SetArgs([]string{"Some Page", "--content", "= Hi =", "--dry-run", "--json"})

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	err := cmd.Execute()

	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"dry_run": true`) {
		t.Errorf("output = %q, want dry_run true", s)
	}
	if !strings.Contains(s, `"title": "Some Page"`) {
		t.Errorf("output = %q, want title Some Page", s)
	}
}
