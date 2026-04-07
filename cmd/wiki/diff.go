package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [title]",
		Short: "Compare revisions or pages",
		Long: `Compare two revisions by ID, or the latest revisions of two pages.

With a title argument (no flags), compares the last two revisions of that page.
With --from/--to flags, compares specific revision IDs.
With --from-title/--to-title flags, compares latest revisions of two pages.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDiff,
	}

	cmd.Flags().Int("from", 0, "Source revision ID")
	cmd.Flags().Int("to", 0, "Target revision ID")
	cmd.Flags().String("from-title", "", "Source page title (uses latest revision)")
	cmd.Flags().String("to-title", "", "Target page title (uses latest revision)")

	return cmd
}

func runDiff(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx := context.Background()
	fromRev, _ := cmd.Flags().GetInt("from")
	toRev, _ := cmd.Flags().GetInt("to")
	fromTitle, _ := cmd.Flags().GetString("from-title")
	toTitle, _ := cmd.Flags().GetString("to-title")

	// Shortcut: wiki diff <title> — compare last two revisions
	if len(args) == 1 && fromRev == 0 && toRev == 0 && fromTitle == "" && toTitle == "" {
		return runDiffLastTwo(cmd, client, ctx, args[0])
	}

	// Explicit flags required if no title argument
	if fromRev == 0 && toRev == 0 && fromTitle == "" && toTitle == "" {
		return fmt.Errorf("provide a page title, --from/--to revision IDs, or --from-title/--to-title")
	}

	result, err := client.CompareRevisions(ctx, wiki.CompareRevisionsArgs{
		FromRev:   fromRev,
		ToRev:     toRev,
		FromTitle: fromTitle,
		ToTitle:   toTitle,
	})
	if err != nil {
		return fmt.Errorf("diff failed: %w", err)
	}

	return printDiffResult(cmd, result)
}

func runDiffLastTwo(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	revisions, err := client.GetRevisions(ctx, wiki.GetRevisionsArgs{
		Title: title,
		Limit: 2,
	})
	if err != nil {
		return fmt.Errorf("failed to get revisions for %q: %w", title, err)
	}
	if len(revisions.Revisions) < 2 {
		return fmt.Errorf("page %q has fewer than 2 revisions", title)
	}

	result, err := client.CompareRevisions(ctx, wiki.CompareRevisionsArgs{
		FromRev: revisions.Revisions[1].RevID,
		ToRev:   revisions.Revisions[0].RevID,
	})
	if err != nil {
		return fmt.Errorf("diff failed: %w", err)
	}

	return printDiffResult(cmd, result)
}

func printDiffResult(cmd *cobra.Command, result wiki.CompareRevisionsResult) error {
	if isJSON(cmd) {
		return printJSON(result)
	}

	// Header
	fmt.Printf("From: %s (rev %d)", result.FromTitle, result.FromRevID)
	if result.FromUser != "" {
		fmt.Printf(" by %s", result.FromUser)
	}
	if result.FromTimestamp != "" {
		fmt.Printf(" at %s", result.FromTimestamp)
	}
	fmt.Println()

	fmt.Printf("To:   %s (rev %d)", result.ToTitle, result.ToRevID)
	if result.ToUser != "" {
		fmt.Printf(" by %s", result.ToUser)
	}
	if result.ToTimestamp != "" {
		fmt.Printf(" at %s", result.ToTimestamp)
	}
	fmt.Println()
	fmt.Println()

	// Diff content (strip HTML tags for terminal)
	if result.Diff == "" {
		fmt.Println("No differences found.")
	} else {
		fmt.Println(stripHTML(result.Diff))
	}

	return nil
}
