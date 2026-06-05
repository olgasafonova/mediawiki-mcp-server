package main

import "testing"

func TestNewLinksCmdSubcommands(t *testing.T) {
	cmd := newLinksCmd()
	if cmd.Name() != "links" {
		t.Errorf("Name = %q, want links", cmd.Name())
	}
	for _, sub := range []string{"external", "backlinks", "broken", "check", "orphaned"} {
		if !cwHasSubcommand(cmd, sub) {
			t.Errorf("links missing subcommand %q", sub)
		}
	}
}

func TestNewLinksExternalCmdArgs(t *testing.T) {
	cmd := newLinksExternalCmd()
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("external: expected error for 0 args (MinimumNArgs 1)")
	}
	if err := cwArgsErr(cmd, []string{"Page"}); err != nil {
		t.Errorf("external: unexpected error for 1 arg: %v", err)
	}
}

func TestNewLinksBacklinksCmdFlags(t *testing.T) {
	cmd := newLinksBacklinksCmd()
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultInt(t, cmd, "namespace", "0")
	cwFlagDefaultString(t, cmd, "redirects", "false")
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("backlinks: expected error for 0 args")
	}
}

func TestNewLinksBrokenCmdFlags(t *testing.T) {
	cmd := newLinksBrokenCmd()
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultInt(t, cmd, "limit", "20")
}

func TestNewLinksCheckCmd(t *testing.T) {
	cmd := newLinksCheckCmd()
	cwFlagDefaultInt(t, cmd, "timeout", "10")
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("check: expected error for 0 args")
	}
}

func TestNewLinksOrphanedCmdFlags(t *testing.T) {
	cmd := newLinksOrphanedCmd()
	cwFlagDefaultInt(t, cmd, "namespace", "0")
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultString(t, cmd, "prefix", "")
}

func TestRunLinksBrokenRequiresInput(t *testing.T) {
	// With no page args and no --category, runLinksBroken errors before any
	// network call (newWikiClient is reached only after this guard).
	cmd := newLinksBrokenCmd()
	cwGlobalFlags(cmd)
	if err := runLinksBroken(cmd, nil); err == nil {
		t.Error("expected error when neither pages nor --category provided")
	}
}
