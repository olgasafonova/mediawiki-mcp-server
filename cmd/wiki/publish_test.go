package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPublishCmd(t *testing.T) {
	cmd := newPublishCmd()
	if cmd.Name() != "publish" {
		t.Errorf("Name = %q, want publish", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "summary", "")
	cwFlagDefaultString(t, cmd, "minor", "false")
	cwFlagDefaultString(t, cmd, "theme", "neutral")
	cwFlagDefaultString(t, cmd, "css", "false")
	cwFlagDefaultString(t, cmd, "preview", "false")

	// ExactArgs(2)
	if err := cwArgsErr(cmd, []string{"only-one"}); err == nil {
		t.Error("publish: expected error for 1 arg")
	}
	if err := cwArgsErr(cmd, []string{"file.md", "Title"}); err != nil {
		t.Errorf("publish: unexpected error for 2 args: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b", "c"}); err == nil {
		t.Error("publish: expected error for 3 args")
	}
}

// TestRunPublishPreview exercises the fully hermetic preview path: read a
// markdown file, convert to wikitext, print. No network is touched.
func TestRunPublishPreview(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("# Heading\n\nBody text.\n"), 0o600); err != nil {
		t.Fatalf("write temp markdown: %v", err)
	}

	cmd := newPublishCmd()
	cwGlobalFlags(cmd)
	if err := cmd.Flags().Set("preview", "true"); err != nil {
		t.Fatalf("set preview flag: %v", err)
	}

	if err := runPublish(cmd, []string{mdPath, "Some Page"}); err != nil {
		t.Fatalf("runPublish preview returned error: %v", err)
	}
}

// TestRunPublishPreviewJSON exercises the preview JSON branch (isJSON true).
func TestRunPublishPreviewJSON(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("Some **bold** text.\n"), 0o600); err != nil {
		t.Fatalf("write temp markdown: %v", err)
	}

	cmd := newPublishCmd()
	cwGlobalFlags(cmd)
	if err := cmd.Flags().Set("preview", "true"); err != nil {
		t.Fatalf("set preview flag: %v", err)
	}
	if err := cmd.PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	if err := runPublish(cmd, []string{mdPath, "Some Page"}); err != nil {
		t.Fatalf("runPublish preview JSON returned error: %v", err)
	}
}

// TestRunPublishMissingFile exercises the os.ReadFile error path, which fails
// before any network call.
func TestRunPublishMissingFile(t *testing.T) {
	cmd := newPublishCmd()
	cwGlobalFlags(cmd)
	if err := cmd.Flags().Set("preview", "true"); err != nil {
		t.Fatalf("set preview flag: %v", err)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	err := runPublish(cmd, []string{missing, "Some Page"})
	if err == nil {
		t.Fatal("expected error for missing markdown file")
	}
}
