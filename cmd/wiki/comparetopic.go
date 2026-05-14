package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newCompareTopicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare-topic <topic>",
		Short: "Compare how a topic is described across pages",
		Long: `Find pages mentioning a topic and surface inconsistencies in how
they describe it (different values, conflicting facts). Useful for finding
documentation drift across a wiki.

  wiki compare-topic "timeout"
  wiki compare-topic "API version" --category Documentation --limit 30`,
		Args: cobra.ExactArgs(1),
		RunE: runCompareTopic,
	}

	cmd.Flags().String("category", "", "Limit search to pages in this category")
	cmd.Flags().IntP("limit", "n", 20, "Maximum pages to compare (max 50)")

	return cmd
}

func runCompareTopic(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	topic := args[0]
	category, _ := cmd.Flags().GetString("category")
	limit, _ := cmd.Flags().GetInt("limit")

	result, err := client.CompareTopic(context.Background(), wiki.CompareTopicArgs{
		Topic:    topic,
		Category: category,
		Limit:    limit,
	})
	if err != nil {
		return fmt.Errorf("compare-topic failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.PagesFound == 0 {
		fmt.Printf("No pages mention %q\n", result.Topic)
		return nil
	}

	fmt.Printf("%q mentioned on %d page(s)\n", result.Topic, result.PagesFound)
	if result.Summary != "" {
		fmt.Println(result.Summary)
	}
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "PAGE\tMENTIONS\tLAST EDITED\n")
	for _, m := range result.PageMentions {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", m.PageTitle, m.Mentions, m.LastEdited)
	}
	_ = tw.Flush()

	if len(result.Inconsistencies) > 0 {
		fmt.Printf("\nInconsistencies (%d):\n", len(result.Inconsistencies))
		for _, inc := range result.Inconsistencies {
			fmt.Printf("  [%s] %s vs %s\n", inc.Type, inc.PageA, inc.PageB)
			fmt.Printf("    A: %s\n", truncate(strings.TrimSpace(inc.ValueA), 100))
			fmt.Printf("    B: %s\n", truncate(strings.TrimSpace(inc.ValueB), 100))
			if inc.Description != "" {
				fmt.Printf("    %s\n", inc.Description)
			}
		}
	}

	return nil
}
