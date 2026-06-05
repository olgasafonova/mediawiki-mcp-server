package main

import (
	"testing"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestNewPageCmd(t *testing.T) {
	cmd := newPageCmd()
	if cmd.Name() != "page" {
		t.Errorf("Name = %q, want page", cmd.Name())
	}

	for _, name := range []string{"summary", "sections", "info", "related", "images"} {
		cwFlagDefaultString(t, cmd, name, "false")
	}
	cwFlagDefaultInt(t, cmd, "section", "-1")
	cwFlagDefaultString(t, cmd, "format", "wikitext")

	// Args: MinimumNArgs(1) — zero args must error.
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"Title"}); err != nil {
		t.Errorf("unexpected error for 1 arg: %v", err)
	}
}

func TestPrintPageHeaderNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("printPageHeader panicked: %v", r)
		}
	}()
	printPageHeader(wiki.PageContent{Title: "Test", Revision: 7, Timestamp: "2026-01-01"})
	printPageHeader(wiki.PageContent{Title: "Trunc", Truncated: true})
	printPageHeader(wiki.PageContent{})
}
