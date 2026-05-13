package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newSimilarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "similar <page>",
		Short: "Find pages with similar content to a source page",
		Long: `Find wiki pages whose content overlaps with the source page.

Each result includes a similarity score (0-1), the terms they share, and
whether the pages already link to each other. Useful for identifying
cross-link opportunities and potential duplicates.

  wiki similar "API Reference"
  wiki similar "Installation Guide" --limit 5 --min-score 0.3
  wiki similar "Release Notes" --category Documentation`,
		Args: cobra.ExactArgs(1),
		RunE: runSimilar,
	}

	cmd.Flags().IntP("limit", "n", 10, "Maximum similar pages to return (max 50)")
	cmd.Flags().String("category", "", "Limit search to pages in this category")
	cmd.Flags().Float64("min-score", 0.1, "Minimum similarity score 0-1")

	return cmd
}

func runSimilar(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	page := args[0]
	limit, _ := cmd.Flags().GetInt("limit")
	category, _ := cmd.Flags().GetString("category")
	minScore, _ := cmd.Flags().GetFloat64("min-score")

	result, err := client.FindSimilarPages(context.Background(), wiki.FindSimilarPagesArgs{
		Page:     page,
		Limit:    limit,
		Category: category,
		MinScore: minScore,
	})
	if err != nil {
		return fmt.Errorf("similar failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.SimilarPages) == 0 {
		fmt.Printf("No similar pages found for %q (compared %d pages).\n", result.SourcePage, result.TotalCompared)
		if result.Message != "" {
			fmt.Println(result.Message)
		}
		return nil
	}

	fmt.Printf("Similar to %q (compared %d pages)\n\n", result.SourcePage, result.TotalCompared)

	tw := table()
	fmt.Fprintf(tw, "SCORE\tTITLE\tLINKED\tBACKLINKED\tCOMMON TERMS\n")
	for _, p := range result.SimilarPages {
		linked := "no"
		if p.IsLinked {
			linked = "yes"
		}
		backlinked := "no"
		if p.LinksBack {
			backlinked = "yes"
		}
		terms := truncate(strings.Join(p.CommonTerms, ", "), 60)
		fmt.Fprintf(tw, "%.2f\t%s\t%s\t%s\t%s\n", p.SimilarityScore, p.Title, linked, backlinked, terms)
	}
	_ = tw.Flush()

	return nil
}
