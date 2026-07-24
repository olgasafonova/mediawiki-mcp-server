package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newReplaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replace <title>",
		Short: "Find and replace text in wiki pages",
		Long: `Replace text in a single page or across multiple pages.

Single page:
  wiki replace "Page Title" --find "old text" --replace "new text"

Bulk mode (multiple pages):
  wiki replace --bulk --pages "Page1,Page2" --find "old" --replace "new"
  wiki replace --bulk --category "API" --find "v2" --replace "v3"`,
		Args: cobra.MaximumNArgs(1),
		RunE: runReplace,
	}

	cmd.Flags().String("find", "", "Text to find (required)")
	cmd.Flags().String("replace", "", "Replacement text (required)")
	cmd.Flags().Bool("regex", false, "Treat find pattern as regex")
	cmd.Flags().Bool("all", false, "Replace all occurrences (default: first only)")
	cmd.Flags().Bool("preview", false, "Preview changes without saving")
	cmd.Flags().String("summary", "", "Edit summary")
	cmd.Flags().Bool("minor", false, "Mark as minor edit")

	// Bulk mode flags
	cmd.Flags().Bool("bulk", false, "Enable bulk replace across multiple pages")
	cmd.Flags().String("pages", "", "Comma-separated page titles (bulk mode)")
	cmd.Flags().String("category", "", "Category to get pages from (bulk mode)")
	cmd.Flags().Int("limit", 0, "Max pages to process in bulk mode (default 10, max 50)")

	return cmd
}

func runReplace(cmd *cobra.Command, args []string) error {
	bulk, _ := cmd.Flags().GetBool("bulk")

	if bulk {
		return runBulkReplace(cmd)
	}

	if len(args) == 0 {
		return fmt.Errorf("page title is required (or use --bulk for multi-page mode)")
	}

	return runSingleReplace(cmd, args[0])
}

// requireFindReplace reads the --find/--replace flags and enforces that both
// were provided. An explicitly empty --replace (deletion) is allowed.
func requireFindReplace(cmd *cobra.Command) (find, replace string, err error) {
	find, _ = cmd.Flags().GetString("find")
	replace, _ = cmd.Flags().GetString("replace")
	if find == "" {
		return "", "", fmt.Errorf("--find is required")
	}
	if replace == "" && !cmd.Flags().Changed("replace") {
		return "", "", fmt.Errorf("--replace is required")
	}
	return find, replace, nil
}

// setupReplace runs the scaffolding shared by both replace modes: wiki
// client creation and --find/--replace validation. The caller owns closing
// the returned client.
func setupReplace(cmd *cobra.Command) (client *wiki.Client, find, replace string, err error) {
	client, err = newWikiClient(cmd)
	if err != nil {
		return nil, "", "", err
	}
	find, replace, err = requireFindReplace(cmd)
	if err != nil {
		client.Close()
		return nil, "", "", err
	}
	return client, find, replace, nil
}

func runSingleReplace(cmd *cobra.Command, title string) error {
	client, find, replace, err := setupReplace(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	useRegex, _ := cmd.Flags().GetBool("regex")
	all, _ := cmd.Flags().GetBool("all")
	preview, _ := cmd.Flags().GetBool("preview")
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")

	result, err := client.FindReplace(context.Background(), wiki.FindReplaceArgs{
		Title:    title,
		Find:     find,
		Replace:  replace,
		UseRegex: useRegex,
		All:      all,
		Preview:  &preview,
		Summary:  summary,
		Minor:    minor,
	})
	if err != nil {
		return fmt.Errorf("replace failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	printSingleReplaceResult(result, find, title, preview)
	return nil
}

// printSingleReplaceResult renders the human-readable outcome of a
// single-page replace, listing the changed lines.
func printSingleReplaceResult(result wiki.FindReplaceResult, find, title string, preview bool) {
	if result.MatchCount == 0 {
		fmt.Printf("No matches for %q in %s\n", find, title)
		return
	}

	if preview {
		fmt.Printf("Preview: %d matches in %s\n\n", result.MatchCount, title)
	} else {
		fmt.Printf("Replaced %d of %d matches in %s (rev: %d)\n\n", result.ReplaceCount, result.MatchCount, title, result.RevisionID)
	}

	for _, change := range result.Changes {
		fmt.Printf("  Line %d:\n", change.Line)
		fmt.Printf("    - %s\n", change.Before)
		fmt.Printf("    + %s\n", change.After)
	}
}

// splitPagesFlag turns the comma-separated --pages value into a trimmed,
// empty-free list of page titles.
func splitPagesFlag(pagesStr string) []string {
	var pages []string
	for _, p := range strings.Split(pagesStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			pages = append(pages, p)
		}
	}
	return pages
}

func runBulkReplace(cmd *cobra.Command) error {
	client, find, replace, err := setupReplace(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	useRegex, _ := cmd.Flags().GetBool("regex")
	preview, _ := cmd.Flags().GetBool("preview")
	summary, _ := cmd.Flags().GetString("summary")
	limit, _ := cmd.Flags().GetInt("limit")
	pagesStr, _ := cmd.Flags().GetString("pages")
	category, _ := cmd.Flags().GetString("category")

	result, err := client.BulkReplace(context.Background(), wiki.BulkReplaceArgs{
		Pages:    splitPagesFlag(pagesStr),
		Category: category,
		Find:     find,
		Replace:  replace,
		UseRegex: useRegex,
		Preview:  &preview,
		Summary:  summary,
		Limit:    limit,
	})
	if err != nil {
		return fmt.Errorf("bulk replace failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	printBulkReplaceResult(result, preview)
	return nil
}

// printBulkReplaceResult renders the human-readable summary and per-page
// table for a bulk replace.
func printBulkReplaceResult(result wiki.BulkReplaceResult, preview bool) {
	if preview {
		fmt.Printf("Preview: %d pages processed, %d with matches, %d total changes\n\n",
			result.PagesProcessed, result.PagesModified, result.TotalChanges)
	} else {
		fmt.Printf("Bulk replace: %d pages processed, %d modified, %d total changes\n\n",
			result.PagesProcessed, result.PagesModified, result.TotalChanges)
	}

	tw := table()
	fmt.Fprintf(tw, "PAGE\tMATCHES\tCHANGES\n")
	for _, r := range result.Results {
		fmt.Fprintf(tw, "%s\t%d\t%d\n", r.Title, r.MatchCount, r.ReplaceCount)
	}
	_ = tw.Flush()
}
