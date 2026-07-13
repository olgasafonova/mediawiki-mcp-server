package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "wiki",
		Short: "MediaWiki CLI — search, read, edit, and audit any wiki",
		Long:  "A command-line interface for MediaWiki wikis. Shares the same API client as mediawiki-mcp-server.",
		// Setting Version enables the built-in `--version` flag (exit 0),
		// complementing the `version` subcommand.
		Version: Version,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageErr(err)
	})

	// Global flags
	root.PersistentFlags().String("url", "", "Wiki API URL (overrides MEDIAWIKI_URL)")
	root.PersistentFlags().Bool("json", false, "Output as JSON")
	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-essential output")
	root.PersistentFlags().BoolP("verbose", "v", false, "Verbose output (debug logging)")

	// Register commands
	root.AddCommand(
		newSearchCmd(),
		newPageCmd(),
		newEditCmd(),
		newReplaceCmd(),
		newLintCmd(),
		newAuditCmd(),
		newRecentCmd(),
		newHistoryCmd(),
		newDiffCmd(),
		newLinksCmd(),
		newListCmd(),
		newPublishCmd(),
		newParseCmd(),
		newFormatCmd(),
		newSearchReadCmd(),
		newSimilarCmd(),
		newStaleCmd(),
		newResolveCmd(),
		newMoveCmd(),
		newUploadCmd(),
		newCategoriesCmd(),
		newInfoCmd(),
		newGrepCmd(),
		newCompareTopicCmd(),
		newTranslationsCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)

	// Map positional-argument validation errors to exit 2 (usage). Cobra's
	// built-in Args validators return plain errors that ExitCode() can't
	// classify; wrapping them here honors the documented exit-code contract
	// without editing every command.
	wrapArgsAsUsage(root)

	if err := root.Execute(); err != nil {
		// Cobra reports an unknown subcommand as a plain, unclassified
		// error. Treat it as a usage error (exit 2) rather than the generic
		// exit 1, matching how unknown flags are already handled.
		if ExitCode(err) == exitDefault && strings.HasPrefix(err.Error(), "unknown command") {
			err = usageErr(err)
		}
		emitError(root, err)
		os.Exit(ExitCode(err))
	}
}

// emitError writes a failure to stderr. Under the global --json flag it
// emits a machine-readable {"error", "exit_code"} object so agents can parse
// failures structurally; otherwise it prints the familiar "Error: ..." line.
// stdout is left untouched so it stays reserved for command data.
func emitError(root *cobra.Command, err error) {
	if jsonOut, _ := root.PersistentFlags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{ //nolint:errcheck // best-effort diagnostic write
			"error":     err.Error(),
			"exit_code": ExitCode(err),
		})
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

// wrapArgsAsUsage recursively replaces each command's Args validator with
// one that wraps any validation failure as a usage error (exit 2). Commands
// with no Args validator (cobra's default arbitrary-args) are left alone.
func wrapArgsAsUsage(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		if inner := sub.Args; inner != nil {
			sub.Args = func(c *cobra.Command, args []string) error {
				if err := inner(c, args); err != nil {
					return usageErr(err)
				}
				return nil
			}
		}
		wrapArgsAsUsage(sub)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print wiki CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if isJSON(cmd) {
				return printJSON(map[string]string{"installed_version": Version})
			}
			fmt.Printf("wiki %s\n", Version)
			return nil
		},
	}
}
