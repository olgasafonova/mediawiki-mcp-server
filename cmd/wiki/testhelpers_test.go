package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// cwGlobalFlags mounts the same persistent flags that main() attaches to the
// root command, so subcommand RunE/flag-reading code that calls isJSON/isQuiet
// or reads --url works under test without booting the whole root command.
func cwGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("url", "", "")
	cmd.PersistentFlags().Bool("json", false, "")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "")
}

// cwFlagDefaultString asserts a string flag exists with the expected default.
func cwFlagDefaultString(t *testing.T, cmd *cobra.Command, name, want string) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	if f == nil {
		t.Fatalf("flag %q not registered on %q", name, cmd.Name())
	}
	if f.DefValue != want {
		t.Errorf("flag %q default = %q, want %q", name, f.DefValue, want)
	}
}

// cwFlagDefaultInt asserts an int flag exists with the expected default.
func cwFlagDefaultInt(t *testing.T, cmd *cobra.Command, name, want string) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	if f == nil {
		t.Fatalf("flag %q not registered on %q", name, cmd.Name())
	}
	if f.DefValue != want {
		t.Errorf("flag %q default = %q, want %q", name, f.DefValue, want)
	}
}

// cwFlagExists asserts a flag of any type is registered.
func cwFlagExists(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	if cmd.Flags().Lookup(name) == nil {
		t.Errorf("flag %q not registered on %q", name, cmd.Name())
	}
}

// cwArgsErr runs cmd.Args validation against argv and reports whether it errored.
// It does not execute RunE, so no network is touched.
func cwArgsErr(cmd *cobra.Command, argv []string) error {
	if cmd.Args == nil {
		return nil
	}
	return cmd.Args(cmd, argv)
}

// cwHasSubcommand reports whether parent has a child with the given Use-name.
func cwHasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}
