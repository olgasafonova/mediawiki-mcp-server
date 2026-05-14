package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "wiki",
		Short: "MediaWiki CLI — search, read, edit, and audit any wiki",
		Long:  "A command-line interface for MediaWiki wikis. Shares the same API client as mediawiki-mcp-server.",
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

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitCode(err))
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print wiki CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("wiki %s\n", Version)
		},
	}
}
