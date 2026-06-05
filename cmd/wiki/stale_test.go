package main

import "testing"

func TestNewStaleCmd(t *testing.T) {
	cmd := newStaleCmd()
	if cmd.Name() != "stale-pages" {
		t.Errorf("Name = %q, want stale-pages", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "days", "90")
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultInt(t, cmd, "namespace", "0")
	cwFlagDefaultInt(t, cmd, "limit", "50")
	// NoArgs
	if err := cwArgsErr(cmd, []string{"unexpected"}); err == nil {
		t.Error("stale-pages: expected error for positional args (NoArgs)")
	}
	if err := cwArgsErr(cmd, nil); err != nil {
		t.Errorf("stale-pages: unexpected error for 0 args: %v", err)
	}
}
