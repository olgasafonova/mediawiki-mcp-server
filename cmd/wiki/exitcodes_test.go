package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestWrapArgsAsUsageMapsToExit2 verifies that a positional-argument
// violation resolves to the documented usage exit code (2) rather than the
// generic default (1).
func TestWrapArgsAsUsageMapsToExit2(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{
		Use:  "sub",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(sub)
	wrapArgsAsUsage(root)

	err := sub.Args(sub, []string{"unexpected"})
	if err == nil {
		t.Fatal("expected NoArgs violation to error")
	}
	if got := ExitCode(err); got != exitUsage {
		t.Errorf("ExitCode = %d, want exitUsage %d", got, exitUsage)
	}
}

// TestWrapArgsAsUsageLeavesNilValidator confirms commands that opt out of
// argument validation (cobra's arbitrary-args default) are left untouched.
func TestWrapArgsAsUsageLeavesNilValidator(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "sub"} // no Args validator
	root.AddCommand(sub)
	wrapArgsAsUsage(root)

	if sub.Args != nil {
		t.Error("wrapArgsAsUsage should leave a nil Args validator untouched")
	}
}

// TestNewWikiClientMissingURLIsConfigError locks in the config-error exit
// code (10) for the most common agent failure: no MEDIAWIKI_URL configured.
func TestNewWikiClientMissingURLIsConfigError(t *testing.T) {
	t.Setenv("MEDIAWIKI_URL", "")
	t.Setenv("MEDIAWIKI_USERNAME", "")
	t.Setenv("MEDIAWIKI_PASSWORD", "")

	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().Bool("verbose", false, "")

	_, err := newWikiClient(cmd)
	if err == nil {
		t.Fatal("expected config error when MEDIAWIKI_URL is unset")
	}
	if got := ExitCode(err); got != exitConfig {
		t.Errorf("ExitCode = %d, want exitConfig %d", got, exitConfig)
	}
}
