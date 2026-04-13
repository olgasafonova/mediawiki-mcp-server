package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newRecentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recent",
		Short: "Show recent wiki changes",
		Long:  "List recent changes across the wiki, optionally filtered and aggregated.",
		Args:  cobra.NoArgs,
		RunE:  runRecent,
	}

	cmd.Flags().IntP("limit", "n", 50, "Maximum changes to return (max 500)")
	cmd.Flags().String("type", "", "Filter by type: edit, new, log")
	cmd.Flags().Int("namespace", -1, "Filter by namespace (-1 for all)")
	cmd.Flags().String("start", "", "Start timestamp (ISO 8601)")
	cmd.Flags().String("end", "", "End timestamp (ISO 8601)")
	cmd.Flags().String("aggregate", "", "Aggregate by: user, page, or type")
	cmd.Flags().String("continue", "", "Continue token for pagination")

	return cmd
}

func runRecent(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")
	rcType, _ := cmd.Flags().GetString("type")
	namespace, _ := cmd.Flags().GetInt("namespace")
	start, _ := cmd.Flags().GetString("start")
	end, _ := cmd.Flags().GetString("end")
	aggregate, _ := cmd.Flags().GetString("aggregate")
	cont, _ := cmd.Flags().GetString("continue")

	result, err := client.GetRecentChanges(context.Background(), wiki.RecentChangesArgs{
		Limit:        limit,
		Namespace:    namespace,
		Type:         rcType,
		ContinueFrom: cont,
		Start:        start,
		End:          end,
		AggregateBy:  aggregate,
	})
	if err != nil {
		return fmt.Errorf("failed to get recent changes: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Aggregated output
	if result.Aggregated != nil {
		fmt.Printf("Recent changes aggregated by %s (%d total)\n\n", result.Aggregated.By, result.Aggregated.TotalChanges)
		tw := table()
		fmt.Fprintf(tw, "KEY\tCOUNT\n")
		for _, item := range result.Aggregated.Items {
			fmt.Fprintf(tw, "%s\t%d\n", item.Key, item.Count)
		}
		_ = tw.Flush()
		return nil
	}

	// Raw changes output
	if len(result.Changes) == 0 {
		fmt.Println("No recent changes found.")
		return nil
	}

	fmt.Printf("Recent changes (%d)", len(result.Changes))
	if result.HasMore {
		fmt.Print(" (more available)")
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "TIMESTAMP\tUSER\tTYPE\tTITLE\tCOMMENT\n")
	for _, c := range result.Changes {
		comment := truncate(c.Comment, 60)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.Timestamp.Format("2006-01-02 15:04"), c.User, c.Type, c.Title, comment)
	}
	_ = tw.Flush()

	if result.HasMore && result.ContinueFrom != "" {
		fmt.Printf("\nMore results available. Use --continue %q to see next page.\n", result.ContinueFrom)
	}

	return nil
}
