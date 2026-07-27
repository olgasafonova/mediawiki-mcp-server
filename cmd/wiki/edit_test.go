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
	cwFlagDefaultString(t, cmd, "dry-run", "false")
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
