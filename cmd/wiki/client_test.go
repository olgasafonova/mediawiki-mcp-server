package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIsJSON(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	if isJSON(cmd) {
		t.Error("isJSON should be false by default")
	}
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if !isJSON(cmd) {
		t.Error("isJSON should be true after --json set")
	}
}

func TestIsJSONMissingFlag(t *testing.T) {
	// When the flag isn't registered, GetBool returns false; isJSON must not panic.
	cmd := &cobra.Command{}
	if isJSON(cmd) {
		t.Error("isJSON on cmd without json flag should be false")
	}
}

func TestIsQuiet(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().BoolP("quiet", "q", false, "")

	if isQuiet(cmd) {
		t.Error("isQuiet should be false by default")
	}
	if err := cmd.Flags().Set("quiet", "true"); err != nil {
		t.Fatal(err)
	}
	if !isQuiet(cmd) {
		t.Error("isQuiet should be true after --quiet set")
	}
}

func TestIsQuietMissingFlag(t *testing.T) {
	cmd := &cobra.Command{}
	if isQuiet(cmd) {
		t.Error("isQuiet on cmd without quiet flag should be false")
	}
}
