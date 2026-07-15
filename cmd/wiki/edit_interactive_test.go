package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewEditCmdInteractiveFlag(t *testing.T) {
	cmd := newEditCmd()

	cwFlagDefaultString(t, cmd, "interactive", "false")

	if f := cmd.Flags().ShorthandLookup("i"); f == nil || f.Name != "interactive" {
		t.Error("expected -i shorthand for --interactive")
	}
}

func TestResolveEditor(t *testing.T) {
	// Resolve a real executable we know exists on every Unix test host.
	// exec.LookPath("sh") succeeds wherever /bin/sh is available, which is
	// guaranteed on the Linux/macOS CI matrix for this repo.
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("no shell available to test editor resolution: %v", err)
	}

	cases := []struct {
		name   string
		visual string
		editor string
		want   string
	}{
		{"visual wins over editor", "sh", "false", "sh"},
		{"editor used when visual unset", "", "sh", "sh"},
		{"unresolved visual falls back to editor", "definitely-not-a-binary", "sh", "sh"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VISUAL", tc.visual)
			t.Setenv("EDITOR", tc.editor)

			got, err := resolveEditor()
			if err != nil {
				t.Fatalf("resolveEditor: unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveEditor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveEditorUnset(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	_, err := resolveEditor()
	if err == nil {
		t.Fatal("resolveEditor with no VISUAL/EDITOR: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "$VISUAL") || !strings.Contains(err.Error(), "$EDITOR") {
		t.Errorf("error %q should mention both $VISUAL and $EDITOR", err)
	}
}

func TestSlugifyForTmpFile(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"plain ascii", "My Page"},
		{"already safe", "Page-Name_v1.2"},
		{"special chars", `Path / With "Quotes" & Symbols!`},
		{"empty falls back", ""},
		{"unicode", "Über die Brücke"},
	}

	// The slugifier is best-effort — the result is only used as a hint in
	// the user's `ls /tmp`. We assert invariants rather than exact output
	// so we don't bake in implementation choices that don't matter:
	//   - no spaces, slashes, or shell-special characters
	//   - output is bounded so the file name stays readable
	//   - "untitled" fallback for empty input is the caller's job
	//     (writeInteractiveBuffer substitutes it before CreateTemp), so
	//     the slugifier itself may return "" here — that's correct.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := slugifyForTmpFile(tc.input)
			if strings.ContainsAny(got, " /\\\"'?*[]{}()$&;|<>") {
				t.Errorf("slug %q contains shell-special characters", got)
			}
			if len(got) > 64 {
				t.Errorf("slug length = %d, want <= 64", len(got))
			}
		})
	}
}

func TestSlugifyForTmpFileLengthCap(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := slugifyForTmpFile(long)
	if len(got) > 64 {
		t.Errorf("slug length = %d, want <= 64", len(got))
	}
}

func TestWriteInteractiveBufferRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	const content = "= Hello =\n\nSome body text.\n"
	path, original, err := writeInteractiveBuffer("Hello", content)
	if err != nil {
		t.Fatalf("writeInteractiveBuffer: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	// Permissions must be tight: the file briefly contains wiki content
	// that may be sensitive on private wikis.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 0600", got)
	}

	// Temp file should be under TMPDIR and have a wikitext suffix for
	// editor-side syntax highlighting.
	if dir := filepath.Dir(path); dir != tmpDir {
		t.Errorf("temp dir = %q, want %q", dir, tmpDir)
	}
	if !strings.HasSuffix(path, ".wiki") {
		t.Errorf("temp path %q should end in .wiki", path)
	}

	// Content round-trips byte-for-byte so the unchanged-detection path
	// can compare later without surprise trimming.
	got, err := os.ReadFile(path) // #nosec G304 -- reading our own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("read-back = %q, want %q", got, original)
	}
	if string(got) != content {
		t.Errorf("read-back content = %q, want %q", got, content)
	}
}

// TestEditInteractiveConflicts verifies the --interactive mode refuses to
// run when another content source is also present. The error is the
// contract: a silent fallback would silently overwrite the user's piped
// content or cause confusing interaction with --file.
func TestEditInteractiveConflicts(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"with --content", []string{"Page", "-i", "--content", "= X ="}},
		{"with --file", []string{"Page", "-i", "--file", "/tmp/x"}},
		{"with --json", []string{"Page", "-i", "--json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newEditCmd()
			cmd.Flags().Bool("json", false, "") // isJSON helper reads this
			cmd.SetArgs(tc.args)

			r, w, _ := os.Pipe()
			old := os.Stdout
			os.Stdout = w

			err := cmd.Execute()

			_ = w.Close()
			os.Stdout = old
			_, _ = io.ReadAll(r)

			if err == nil {
				t.Fatalf("expected error for %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), "interactive") {
				t.Errorf("error %q should mention --interactive", err)
			}
		})
	}
}

// TestEditInteractiveConflictsWithStdin verifies the conflict with piped
// stdin. We stage a piped stdin (non-tty mode) via a process-level pipe
// replacement: writing a byte stream that readStdin() will pick up.
func TestEditInteractiveConflictsWithStdin(t *testing.T) {
	cmd := newEditCmd()
	cmd.SetArgs([]string{"Page", "-i"})

	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("piped content\n"))
		_ = w.Close()
	}()

	oldStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		_ = devNull.Close()
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when stdin is piped alongside -i, got nil")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("error %q should mention stdin", err)
	}
}

func TestIsStdoutTerminalDoesNotPanic(t *testing.T) {
	// Sanity: helper must not panic regardless of the underlying file
	// descriptor's mode (piped in tests, real terminal in CI).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("isStdoutTerminal panicked: %v", r)
		}
	}()
	_ = isStdoutTerminal()
}
