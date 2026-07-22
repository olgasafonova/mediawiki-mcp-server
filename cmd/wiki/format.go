package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newFormatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "format <title> <text> <format>",
		Short: "Apply inline formatting to text on a page",
		Long: `Find text on a page and wrap it in wiki formatting.

Format is one of: strikethrough, bold, italic, underline, code, nowiki.
By default only the first occurrence is changed; use --all for every match.
Use --preview to see the change without saving.

  wiki format "Team" "John Smith" strikethrough --preview
  wiki format "Release Notes" "version 2.0" bold --all`,
		Args: cobra.ExactArgs(3),
		RunE: runFormat,
	}

	cmd.Flags().Bool("all", false, "Apply to all occurrences (default: first only)")
	cmd.Flags().Bool("preview", false, "Preview the change without saving")
	cmd.Flags().String("summary", "", "Edit summary (auto-generated if empty)")

	return cmd
}

func runFormat(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	all, _ := cmd.Flags().GetBool("all")
	preview, _ := cmd.Flags().GetBool("preview")
	summary, _ := cmd.Flags().GetString("summary")

	result, err := client.ApplyFormatting(context.Background(), wiki.ApplyFormattingArgs{
		Title:   args[0],
		Text:    args[1],
		Format:  args[2],
		All:     all,
		Preview: &preview,
		Summary: summary,
	})
	if err != nil {
		return fmt.Errorf("format failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if !result.Success {
		fmt.Printf("No change to %s: %s\n", result.Title, result.Message)
		return nil
	}
	verb := "Formatted"
	if result.Preview {
		verb = "Preview"
	}
	fmt.Printf("%s %s: %d match(es), %s applied\n", verb, result.Title, result.MatchCount, result.Format)
	if !result.Preview && result.RevisionID > 0 {
		fmt.Printf("Revision: %d\n", result.RevisionID)
	}
	return nil
}
