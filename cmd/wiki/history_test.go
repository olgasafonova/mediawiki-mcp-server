package main

import "testing"

func TestNewHistoryCmdFlags(t *testing.T) {
	cmd := newHistoryCmd()
	if cmd.Name() != "history" {
		t.Errorf("Name = %q, want history", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "limit", "20")
	cwFlagDefaultString(t, cmd, "start", "")
	cwFlagDefaultString(t, cmd, "end", "")
	cwFlagDefaultString(t, cmd, "user", "")

	// MaximumNArgs(1)
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for 2 args")
	}
}

func TestRunHistoryRequiresTitleOrUser(t *testing.T) {
	// No title and no --user: errors before any network call.
	cmd := newHistoryCmd()
	cwGlobalFlags(cmd)
	if err := runHistory(cmd, nil); err == nil {
		t.Error("expected error when neither title nor --user provided")
	}
}
