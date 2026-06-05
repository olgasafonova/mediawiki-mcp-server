package main

import "testing"

func TestNewRecentCmd(t *testing.T) {
	cmd := newRecentCmd()
	if cmd.Name() != "recent" {
		t.Errorf("Name = %q, want recent", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultString(t, cmd, "type", "")
	cwFlagDefaultInt(t, cmd, "namespace", "-1")
	cwFlagDefaultString(t, cmd, "start", "")
	cwFlagDefaultString(t, cmd, "end", "")
	cwFlagDefaultString(t, cmd, "aggregate", "")
	cwFlagDefaultString(t, cmd, "continue", "")
	// NoArgs
	if err := cwArgsErr(cmd, []string{"x"}); err == nil {
		t.Error("recent: expected error for positional args (NoArgs)")
	}
}
