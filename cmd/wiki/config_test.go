package main

import (
	"testing"
)

func TestNewConfigCmd(t *testing.T) {
	cmd := newConfigCmd()
	if cmd.Name() != "config" {
		t.Errorf("Name = %q, want config", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "wiki", "false")
}

func TestRunConfigNoWikiConfigured(t *testing.T) {
	// With no MEDIAWIKI_URL and no --url, runConfig prints the help text and
	// returns nil without touching the network.
	t.Setenv("MEDIAWIKI_URL", "")
	t.Setenv("MEDIAWIKI_USERNAME", "")
	cmd := newConfigCmd()
	cwGlobalFlags(cmd)
	if err := runConfig(cmd, nil); err != nil {
		t.Errorf("runConfig with no config should return nil, got %v", err)
	}
}

func TestRunConfigShowsURLWithoutWikiFlag(t *testing.T) {
	// URL set via env, --wiki not set: prints config, no network call.
	t.Setenv("MEDIAWIKI_URL", "https://example.org/api.php")
	t.Setenv("MEDIAWIKI_USERNAME", "")
	cmd := newConfigCmd()
	cwGlobalFlags(cmd)
	if err := runConfig(cmd, nil); err != nil {
		t.Errorf("runConfig should return nil when --wiki not set, got %v", err)
	}
}
