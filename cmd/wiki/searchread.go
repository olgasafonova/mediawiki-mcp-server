package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newSearchReadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search-read <query>",
		Short: "Search the wiki and read the top results in one call",
		Long: `Search for a query and fetch the full content of the top matching pages,
saving a separate read step. Returns the page bodies plus the remaining hits.

  wiki search-read "deployment process"
  wiki search-read "onboarding" --read-count 3 --format html`,
		Args: cobra.MinimumNArgs(1),
		RunE: runSearchRead,
	}

	cmd.Flags().Int("read-count", 1, "Number of top results to read in full (max 5)")
	cmd.Flags().String("format", "wikitext", "Content format: 'wikitext' or 'html'")

	return cmd
}

func runSearchRead(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	readCount, _ := cmd.Flags().GetInt("read-count")
	format, _ := cmd.Flags().GetString("format")

	result, err := client.SearchAndRead(context.Background(), wiki.SearchAndReadArgs{
		Query:     strings.Join(args, " "),
		ReadCount: readCount,
		Format:    format,
	})
	if err != nil {
		return fmt.Errorf("search-read failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	printSearchReadResult(result)
	return nil
}

func printSearchReadResult(result wiki.SearchAndReadResult) {
	fmt.Printf("%q: %d hit(s)\n", result.Query, result.TotalHits)
	for _, p := range result.Pages {
		printSearchReadPage(p)
	}
	printOtherHits(result.OtherHits)
}

func printSearchReadPage(p wiki.SearchAndReadPage) {
	fmt.Printf("\n=== %s (rev %d) ===\n%s\n", p.Title, p.Revision, p.Content)
	if p.Truncated {
		fmt.Println("[content truncated]")
	}
}

func printOtherHits(hits []wiki.SearchHit) {
	if len(hits) == 0 {
		return
	}
	fmt.Printf("\nOther hits:\n")
	for _, h := range hits {
		if h.URL != "" {
			fmt.Printf("  %s  %s\n", h.Title, h.URL)
		} else {
			fmt.Printf("  %s\n", h.Title)
		}
	}
}
