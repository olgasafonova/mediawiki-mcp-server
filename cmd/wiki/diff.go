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

type diffSelectors struct {
	fromRev   int
	toRev     int
	fromTitle string
	toTitle   string
}

func (d diffSelectors) empty() bool {
	return d.fromRev == 0 && d.toRev == 0 && d.fromTitle == "" && d.toTitle == ""
}

func readDiffSelectors(cmd *cobra.Command) diffSelectors {
	fromRev, _ := cmd.Flags().GetInt("from")
	toRev, _ := cmd.Flags().GetInt("to")
	fromTitle, _ := cmd.Flags().GetString("from-title")
	toTitle, _ := cmd.Flags().GetString("to-title")
	return diffSelectors{fromRev, toRev, fromTitle, toTitle}
}

func runDiff(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx := context.Background()
	sel := readDiffSelectors(cmd)

	if len(args) == 1 && sel.empty() {
		return runDiffLastTwo(cmd, client, ctx, args[0])
	}

	if sel.empty() {
		return fmt.Errorf("provide a page title, --from/--to revision IDs, or --from-title/--to-title")
	}

	result, err := client.CompareRevisions(ctx, wiki.CompareRevisionsArgs{
		FromRev:   sel.fromRev,
		ToRev:     sel.toRev,
		FromTitle: sel.fromTitle,
		ToTitle:   sel.toTitle,
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

func printDiffHeader(label, title string, revID int, user, ts string) {
	fmt.Printf("%s %s (rev %d)", label, title, revID)
	if user != "" {
		fmt.Printf(" by %s", user)
	}
	if ts != "" {
		fmt.Printf(" at %s", ts)
	}
	fmt.Println()
}

func printDiffResult(cmd *cobra.Command, result wiki.CompareRevisionsResult) error {
	if isJSON(cmd) {
		return printJSON(result)
	}

	printDiffHeader("From:", result.FromTitle, result.FromRevID, result.FromUser, result.FromTimestamp)
	printDiffHeader("To:  ", result.ToTitle, result.ToRevID, result.ToUser, result.ToTimestamp)
	fmt.Println()

	if result.Diff == "" {
		fmt.Println("No differences found.")
	} else {
		fmt.Println(stripHTML(result.Diff))
	}

	return nil
}
