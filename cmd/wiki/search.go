package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across the wiki",
		Long:  "Search wiki pages by content or title. Returns matching pages with snippet previews.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runSearch,
	}

	cmd.Flags().IntP("limit", "n", 20, "Maximum results to return (max 500)")
	cmd.Flags().Int("offset", 0, "Offset for pagination")

	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	query := strings.Join(args, " ")

	result, err := client.Search(context.Background(), wiki.SearchArgs{
		Query:  query,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Human-readable output
	if len(result.Results) == 0 {
		fmt.Printf("No results for %q\n", query)
		return nil
	}

	fmt.Printf("Found %d results for %q", result.TotalHits, query)
	if result.HasMore {
		fmt.Printf(" (showing %d)", len(result.Results))
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "TITLE\tSIZE\tSNIPPET\n")
	for _, hit := range result.Results {
		snippet := truncate(stripHTML(hit.Snippet), 80)
		fmt.Fprintf(tw, "%s\t%d\t%s\n", hit.Title, hit.Size, snippet)
	}
	_ = tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore results available. Use --offset %d to see next page.\n", result.NextOffset)
	}

	return nil
}
