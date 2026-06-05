package main

import "testing"

func TestNewCompareTopicCmd(t *testing.T) {
	cmd := newCompareTopicCmd()
	if cmd.Name() != "compare-topic" {
		t.Errorf("Name = %q, want compare-topic", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultInt(t, cmd, "limit", "20")
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("compare-topic: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"topic"}); err != nil {
		t.Errorf("compare-topic: unexpected error for 1 arg: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("compare-topic: expected error for 2 args")
	}
}
