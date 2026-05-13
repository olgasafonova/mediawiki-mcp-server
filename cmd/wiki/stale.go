package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newStaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stale-pages",
		Short: "Find pages not edited in the last N days",
		Long: `List wiki pages that haven't been updated in the last N days.

Defaults to 90 days. Pages are sorted oldest-first.

  wiki stale-pages
  wiki stale-pages --days 180
  wiki stale-pages --category Documentation --limit 100`,
		Args: cobra.NoArgs,
		RunE: runStale,
	}

	cmd.Flags().Int("days", 90, "Pages not edited in this many days")
	cmd.Flags().String("category", "", "Limit to pages in this category")
	cmd.Flags().Int("namespace", 0, "Namespace to check (default 0 = main)")
	cmd.Flags().IntP("limit", "n", 50, "Maximum pages to return (max 200)")

	return cmd
}

func runStale(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	days, _ := cmd.Flags().GetInt("days")
	category, _ := cmd.Flags().GetString("category")
	namespace, _ := cmd.Flags().GetInt("namespace")
	limit, _ := cmd.Flags().GetInt("limit")

	result, err := client.GetStalePages(context.Background(), wiki.GetStalePagesArgs{
		Days:      days,
		Category:  category,
		Namespace: namespace,
		Limit:     limit,
	})
	if err != nil {
		return fmt.Errorf("stale-pages failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.StaleCount == 0 {
		fmt.Printf("No pages older than %d days found (scanned %d).\n", result.Days, result.TotalScanned)
		return nil
	}

	fmt.Printf("Stale pages: %d of %d scanned (>= %d days)\n\n", result.StaleCount, result.TotalScanned, result.Days)

	tw := table()
	fmt.Fprintf(tw, "DAYS\tTITLE\tLAST EDITED\tEDITOR\n")
	for _, p := range result.StalePages {
		editor := p.Editor
		if editor == "" {
			editor = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", p.DaysStale, p.Title, p.LastEdited, editor)
	}
	_ = tw.Flush()

	return nil
}
