package main

import "testing"

func TestNewSimilarCmd(t *testing.T) {
	cmd := newSimilarCmd()
	if cmd.Name() != "similar" {
		t.Errorf("Name = %q, want similar", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "limit", "10")
	cwFlagDefaultString(t, cmd, "category", "")
	if f := cmd.Flags().Lookup("min-score"); f == nil {
		t.Error("min-score flag not registered")
	} else if f.DefValue != "0.1" {
		t.Errorf("min-score default = %q, want 0.1", f.DefValue)
	}
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("similar: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"Page"}); err != nil {
		t.Errorf("similar: unexpected error for 1 arg: %v", err)
	}
}
