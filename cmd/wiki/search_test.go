package main

import "testing"

func TestNewSearchCmd(t *testing.T) {
	cmd := newSearchCmd()
	if cmd.Name() != "search" {
		t.Errorf("Name = %q, want search", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "limit", "20")
	cwFlagDefaultInt(t, cmd, "offset", "0")
	// MinimumNArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("search: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"query", "extra"}); err != nil {
		t.Errorf("search: unexpected error for multiple query words: %v", err)
	}
}
