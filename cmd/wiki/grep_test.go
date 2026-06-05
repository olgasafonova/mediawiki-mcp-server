package main

import (
	"context"
	"testing"
)

func TestNewGrepCmdSubcommands(t *testing.T) {
	cmd := newGrepCmd()
	if cmd.Name() != "grep" {
		t.Errorf("Name = %q, want grep", cmd.Name())
	}
	for _, sub := range []string{"page", "file"} {
		if !cwHasSubcommand(cmd, sub) {
			t.Errorf("grep missing subcommand %q", sub)
		}
	}
}

func TestNewGrepPageCmd(t *testing.T) {
	cmd := newGrepPageCmd()
	cwFlagDefaultString(t, cmd, "regex", "false")
	cwFlagDefaultInt(t, cmd, "context", "2")
	// ExactArgs(2)
	if err := cwArgsErr(cmd, []string{"only-one"}); err == nil {
		t.Error("grep page: expected error for 1 arg")
	}
	if err := cwArgsErr(cmd, []string{"title", "query"}); err != nil {
		t.Errorf("grep page: unexpected error for 2 args: %v", err)
	}
}

func TestNewGrepFileCmdArgs(t *testing.T) {
	cmd := newGrepFileCmd()
	if err := cwArgsErr(cmd, []string{"only-one"}); err == nil {
		t.Error("grep file: expected error for 1 arg")
	}
	if err := cwArgsErr(cmd, []string{"file.pdf", "query"}); err != nil {
		t.Errorf("grep file: unexpected error for 2 args: %v", err)
	}
}

func TestCmdCtx(t *testing.T) {
	if cmdCtx() == nil {
		t.Fatal("cmdCtx returned nil")
	}
	// Background context has no deadline.
	if _, ok := cmdCtx().Deadline(); ok {
		t.Error("cmdCtx should not carry a deadline")
	}
	_ = context.Background()
}
