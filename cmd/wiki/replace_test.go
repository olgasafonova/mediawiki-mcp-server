package main

import "testing"

func TestNewReplaceCmdFlags(t *testing.T) {
	cmd := newReplaceCmd()
	if cmd.Name() != "replace" {
		t.Errorf("Name = %q, want replace", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "find", "")
	cwFlagDefaultString(t, cmd, "replace", "")
	cwFlagDefaultString(t, cmd, "regex", "false")
	cwFlagDefaultString(t, cmd, "all", "false")
	cwFlagDefaultString(t, cmd, "preview", "false")
	cwFlagDefaultString(t, cmd, "bulk", "false")
	cwFlagDefaultString(t, cmd, "pages", "")
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultInt(t, cmd, "limit", "0")

	// MaximumNArgs(1)
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for 2 args")
	}
}

func TestRunReplaceRequiresTitle(t *testing.T) {
	// Non-bulk mode with no title arg errors before any network call.
	cmd := newReplaceCmd()
	cwGlobalFlags(cmd)
	if err := runReplace(cmd, nil); err == nil {
		t.Error("expected error when no title and not bulk")
	}
}
