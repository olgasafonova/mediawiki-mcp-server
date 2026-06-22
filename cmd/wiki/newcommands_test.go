package main

import "testing"

func TestNewParseCmd(t *testing.T) {
	cmd := newParseCmd()
	if cmd.Name() != "parse" {
		t.Errorf("Name = %q, want parse", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "file", "")
	cwFlagDefaultString(t, cmd, "title", "")
	// ArbitraryArgs: any count is accepted (content can come from --file/stdin).
	if err := cwArgsErr(cmd, []string{}); err != nil {
		t.Errorf("parse: unexpected error for 0 args: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"some", "wikitext"}); err != nil {
		t.Errorf("parse: unexpected error for 2 args: %v", err)
	}
}

func TestNewFormatCmd(t *testing.T) {
	cmd := newFormatCmd()
	if cmd.Name() != "format" {
		t.Errorf("Name = %q, want format", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "all", "false")
	cwFlagDefaultString(t, cmd, "preview", "false")
	cwFlagDefaultString(t, cmd, "summary", "")
	// ExactArgs(3): title, text, format.
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("format: expected error for 2 args")
	}
	if err := cwArgsErr(cmd, []string{"Page", "text", "bold"}); err != nil {
		t.Errorf("format: unexpected error for 3 args: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b", "c", "d"}); err == nil {
		t.Error("format: expected error for 4 args")
	}
}

func TestNewSearchReadCmd(t *testing.T) {
	cmd := newSearchReadCmd()
	if cmd.Name() != "search-read" {
		t.Errorf("Name = %q, want search-read", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "read-count", "1")
	cwFlagDefaultString(t, cmd, "format", "wikitext")
	// MinimumNArgs(1).
	if err := cwArgsErr(cmd, []string{}); err == nil {
		t.Error("search-read: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"query"}); err != nil {
		t.Errorf("search-read: unexpected error for 1 arg: %v", err)
	}
}

func TestLinksExternalBatchFlag(t *testing.T) {
	cmd := newLinksExternalCmd()
	cwFlagDefaultString(t, cmd, "batch", "false")
}
