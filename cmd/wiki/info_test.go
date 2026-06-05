package main

import "testing"

func TestNewInfoCmd(t *testing.T) {
	cmd := newInfoCmd()
	if cmd.Name() != "info" {
		t.Errorf("Name = %q, want info", cmd.Name())
	}
	if cmd.Short == "" {
		t.Error("info: Short description should not be empty")
	}
	// NoArgs
	if err := cwArgsErr(cmd, []string{"x"}); err == nil {
		t.Error("info: expected error for positional args (NoArgs)")
	}
	if err := cwArgsErr(cmd, nil); err != nil {
		t.Errorf("info: unexpected error for 0 args: %v", err)
	}
}
