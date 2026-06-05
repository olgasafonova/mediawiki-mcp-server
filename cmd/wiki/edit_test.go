package main

import "testing"

func TestNewEditCmd(t *testing.T) {
	cmd := newEditCmd()
	if cmd.Name() != "edit" {
		t.Errorf("Name = %q, want edit", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "file", "")
	cwFlagDefaultString(t, cmd, "content", "")
	cwFlagDefaultString(t, cmd, "summary", "")
	cwFlagDefaultString(t, cmd, "minor", "false")
	cwFlagDefaultString(t, cmd, "bot", "false")
	cwFlagDefaultString(t, cmd, "section", "")
	// short -f alias for --file
	if f := cmd.Flags().ShorthandLookup("f"); f == nil || f.Name != "file" {
		t.Error("expected -f shorthand for --file")
	}
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("edit: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("edit: expected error for 2 args")
	}
}

func TestReadStdinNoPipe(t *testing.T) {
	// In the test harness stdin is typically a char device (no piped data),
	// so readStdin returns empty string and no error.
	got, err := readStdin()
	if err != nil {
		t.Fatalf("readStdin: unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("readStdin with no pipe = %q, want empty", got)
	}
}
