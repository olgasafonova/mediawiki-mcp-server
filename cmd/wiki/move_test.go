package main

import "testing"

func TestNewMoveCmd(t *testing.T) {
	cmd := newMoveCmd()
	if cmd.Name() != "move" {
		t.Errorf("Name = %q, want move", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "reason", "")
	cwFlagDefaultString(t, cmd, "no-redirect", "false")
	cwFlagDefaultString(t, cmd, "no-talk", "false")
	cwFlagDefaultString(t, cmd, "subpages", "false")
	// ExactArgs(2)
	if err := cwArgsErr(cmd, []string{"only-one"}); err == nil {
		t.Error("move: expected error for 1 arg")
	}
	if err := cwArgsErr(cmd, []string{"from", "to"}); err != nil {
		t.Errorf("move: unexpected error for 2 args: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b", "c"}); err == nil {
		t.Error("move: expected error for 3 args")
	}
}
