package main

import "testing"

func TestNewResolveCmd(t *testing.T) {
	cmd := newResolveCmd()
	if cmd.Name() != "resolve" {
		t.Errorf("Name = %q, want resolve", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "fuzzy", "false")
	cwFlagDefaultInt(t, cmd, "max-results", "5")

	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("resolve: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"Title"}); err != nil {
		t.Errorf("resolve: unexpected error for 1 arg: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("resolve: expected error for 2 args")
	}
}
